/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	yaml "gopkg.in/yaml.v2"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

var (
	SystemGoPath = os.Getenv("GOPATH")
	BaseRepoPath = filepath.Join(SystemGoPath, "src", "k8s.io")
)

func Usage() {
	fmt.Fprintf(os.Stderr, `
Usage: %s [-config <config-yaml-file>] [-source-repo <repo>] [-source-org <org>] [-rules-file <file> ] [-target-org <org>]

Command line flags override config values.
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	configFilePath := flag.String("config", "", "the config file in yaml format")
	githubHost := flag.String("github-host", "", "the address of github (defaults to github.com)")
	basePackage := flag.String("base-package", "", "the name of the package base (defaults to k8s.io when source repo is kubernetes, "+
		"otherwise github-host/target-org)")
	repoName := flag.String("source-repo", "", "the name of the source repository (eg. kubernetes)")
	repoOrg := flag.String("source-org", "", "the name of the source repository organization, (eg. kubernetes)")
	rulesFile := flag.String("rules-file", "", "the file with repository rules")
	targetOrg := flag.String("target-org", "", `the target organization to publish into (e.g. "k8s-publishing-bot")`)

	flag.Usage = Usage
	flag.Parse()

	cfg := &config.Config{}
	if *configFilePath != "" {
		bs, err := os.ReadFile(*configFilePath)
		if err != nil {
			glog.Fatalf("Failed to load config file from %q: %v", *configFilePath, err)
		}
		if err := yaml.Unmarshal(bs, &cfg); err != nil {
			glog.Fatalf("Failed to parse config file at %q: %v", *configFilePath, err)
		}
	}

	if *targetOrg != "" {
		cfg.TargetOrg = *targetOrg
	}
	if *repoName != "" {
		cfg.SourceRepo = *repoName
	}
	if *repoOrg != "" {
		cfg.SourceOrg = *repoOrg
	}
	if *githubHost != "" {
		cfg.GithubHost = *githubHost
	}
	if *basePackage != "" {
		cfg.BasePackage = *basePackage
	}

	if cfg.GithubHost == "" {
		cfg.GithubHost = "github.com"
	}

	if cfg.GitDefaultBranch == "" {
		cfg.GitDefaultBranch = "master"
	}

	// defaulting when base package is not specified
	if cfg.BasePackage == "" {
		if cfg.SourceRepo == "kubernetes" {
			cfg.BasePackage = "k8s.io"
		} else {
			cfg.BasePackage = filepath.Join(cfg.GithubHost, cfg.TargetOrg)
		}
	}

	BaseRepoPath = filepath.Join(SystemGoPath, "src", cfg.BasePackage)

	if *rulesFile != "" {
		cfg.RulesFile = *rulesFile
	}

	if cfg.SourceRepo == "" || cfg.SourceOrg == "" {
		glog.Fatalf("source-org and source-repo cannot be empty")
	}

	if cfg.TargetOrg == "" {
		glog.Fatalf("Target organization cannot be empty")
	}

	// If RULE_FILE_PATH is detected, check if the source repository include rules files.
	if len(os.Getenv("RULE_FILE_PATH")) > 0 {
		cfg.RulesFile = filepath.Join(BaseRepoPath, cfg.SourceRepo, os.Getenv("RULE_FILE_PATH"))
	}

	if cfg.RulesFile == "" {
		glog.Fatalf("No rules file provided")
	}
	rules, err := config.LoadRules(cfg.RulesFile)
	if err != nil {
		glog.Fatalf("Failed to load rules: %v", err)
	}
	if err := config.Validate(rules); err != nil {
		glog.Fatalf("Invalid rules: %v", err)
	}

	if err := os.MkdirAll(BaseRepoPath, os.ModePerm); err != nil {
		glog.Fatalf("Failed to create source repo directory %s: %v", BaseRepoPath, err)
	}

	cloneSourceRepo(cfg)
	for _, rule := range rules.Rules {
		cloneForkRepo(cfg, rule.DestinationRepository)
	}
}

func cloneForkRepo(cfg *config.Config, repoName string) {
	forkRepoLocation := fmt.Sprintf("https://%s/%s/%s", cfg.GithubHost, cfg.TargetOrg, repoName)
	repoDir := filepath.Join(BaseRepoPath, repoName)

	if _, err := os.Stat(repoDir); err == nil {
		glog.Infof("Fork repository %q already cloned to %s, resetting remote URL ...", repoName, repoDir)
		setURLCmd := exec.Command("git", "remote", "set-url", "origin", forkRepoLocation)
		setURLCmd.Dir = repoDir
		run(setURLCmd)
		os.Remove(filepath.Join(repoDir, ".git", "index.lock"))
	} else {
		glog.Infof("Cloning fork repository %s ...", forkRepoLocation)
		run(exec.Command("git", "clone", forkRepoLocation))
	}

	// set user in repo because old git version (compare https://github.com/git/git/commit/92bcbb9b338dd27f0fd4245525093c4bce867f3d) still look up user ids without
	setUsernameCmd := exec.Command("git", "config", "user.name", os.Getenv("GIT_COMMITTER_NAME"))
	setUsernameCmd.Dir = repoDir
	run(setUsernameCmd)
	setEmailCmd := exec.Command("git", "config", "user.email", os.Getenv("GIT_COMMITTER_EMAIL"))
	setEmailCmd.Dir = repoDir
	run(setEmailCmd)
}

// run wraps the cmd.Run() command and sets the standard output and common environment variables.
// if the c.Dir is not set, the BaseRepoPath will be used as a base directory for the command.
func run(c *exec.Cmd) {
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if c.Dir == "" {
		c.Dir = BaseRepoPath
	}
	if err := c.Run(); err != nil {
		glog.Fatalf("Command %q failed: %v", strings.Join(c.Args, " "), err)
	}
}

func cloneSourceRepo(cfg *config.Config) {
	repoLocation := fmt.Sprintf("https://%s/%s/%s", cfg.GithubHost, cfg.SourceOrg, cfg.SourceRepo)

	if _, err := os.Stat(filepath.Join(BaseRepoPath, cfg.SourceRepo)); err == nil {
		glog.Infof("Source repository %q already cloned, only setting remote", cfg.SourceRepo)
		remoteCmd := exec.Command("git", "remote", "set-url", "origin", repoLocation)
		remoteCmd.Dir = filepath.Join(BaseRepoPath, cfg.SourceRepo)
		run(remoteCmd)
		return
	}

	glog.Infof("Cloning source repository %s ...", repoLocation)
	cloneCmd := exec.Command("git", "clone", repoLocation)
	run(cloneCmd)
}
