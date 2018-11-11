/*
Copyright 2017 The Kubernetes Authors.

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
	"io/ioutil"
	"os"

	"github.com/golang/glog"
	"gopkg.in/yaml.v2"

	"strings"

	"time"

	"path/filepath"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `
Usage: %s [-config <config-yaml-file>] [-dry-run] [-token-file <token-file>] [-interval <sec>]
          [-source-repo <repo>] [-target-org <org>]

Command line flags override config values.
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	configFilePath := flag.String("config", "", "the config file in yaml format")
	githubHost := flag.String("github-host", "", "the address of github (defaults to github.com)")
	basePackage := flag.String("base-package", "", "the name of the package base (defaults to k8s.io when source repo is kubernetes, "+
		"otherwise github-host/target-org)")
	dryRun := flag.Bool("dry-run", false, "do not push anything to github")
	tokenFile := flag.String("token-file", "", "the file with the github token")
	rulesFile := flag.String("rules-file", "", "the file or URL with repository rules")
	// TODO: make absolute
	repoName := flag.String("source-repo", "", "the name of the source repository (eg. kubernetes)")
	repoOrg := flag.String("source-org", "", "the name of the source repository organization, (eg. kubernetes)")
	targetOrg := flag.String("target-org", "", `the target organization to publish into (e.g. "k8s-publishing-bot")`)
	basePublishScriptPath := flag.String("base-publish-script-path", "./publish_scripts", `the base path in source repo where bot will look for publishing scripts`)
	interval := flag.Uint("interval", 0, "loop with the given seconds of wait in between")
	serverPort := flag.Int("server-port", 0, "start a webserver on the given port listening on 0.0.0.0")

	flag.Usage = Usage
	flag.Parse()

	cfg := config.Config{}
	if *configFilePath != "" {
		bs, err := ioutil.ReadFile(*configFilePath)
		if err != nil {
			glog.Fatalf("Failed to load config file from %q: %v", *configFilePath, err)
		}
		if err := yaml.Unmarshal(bs, &cfg); err != nil {
			glog.Fatalf("Failed to parse config file at %q: %v", *configFilePath, err)
		}
	}

	// override with flags
	if *dryRun {
		cfg.DryRun = true
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
	if *tokenFile != "" {
		cfg.TokenFile = *tokenFile
	}
	if *rulesFile != "" {
		cfg.RulesFile = *rulesFile
	}
	if *basePublishScriptPath != "" {
		cfg.BasePublishScriptPath = *basePublishScriptPath
	}
	if *githubHost != "" {
		cfg.GithubHost = *githubHost
	}
	if *basePackage != "" {
		cfg.BasePackage = *basePackage
	}

	// defaulting to github.com when it is not specified.
	if cfg.GithubHost == "" {
		cfg.GithubHost = "github.com"
	}

	var err error
	cfg.BasePublishScriptPath, err = filepath.Abs(cfg.BasePublishScriptPath)
	if err != nil {
		glog.Fatalf("Failed to get absolute path for base-publish-script-path %q: %v", cfg.BasePublishScriptPath, err)
	}

	if len(cfg.SourceRepo) == 0 || len(cfg.SourceOrg) == 0 {
		glog.Fatalf("source-org and source-repo cannot be empty")
	}

	if len(cfg.TargetOrg) == 0 {
		glog.Fatalf("Target organization cannot be empty")
	}

	// set the baseRepoPath
	gopath := os.Getenv("GOPATH")
	// defaulting when base package is not specified
	if cfg.BasePackage == "" {
		if cfg.SourceRepo == "kubernetes" {
			cfg.BasePackage = "k8s.io"
		} else {
			cfg.BasePackage = filepath.Join(cfg.GithubHost, cfg.TargetOrg)
		}
	}
	baseRepoPath := fmt.Sprintf("%s/%s/%s", gopath, "src", cfg.BasePackage)

	// If RULE_FILE_PATH is detected, check if the source repository include rules files.
	if len(os.Getenv("RULE_FILE_PATH")) > 0 {
		cfg.RulesFile = filepath.Join(baseRepoPath, cfg.SourceRepo, os.Getenv("RULE_FILE_PATH"))
	}

	if len(cfg.RulesFile) == 0 {
		glog.Fatalf("No rules file provided")
	}

	runChan := make(chan bool, 1)

	// start server
	server := Server{
		Issue:   cfg.GithubIssue,
		config:  cfg,
		RunChan: runChan,
	}
	if *serverPort != 0 {
		if err := server.Run(*serverPort); err != nil {
			glog.Fatalf("Failed to run healthz server: %v", err)
		}
	}

	githubIssueErrorf := glog.Fatalf
	if *interval != 0 {
		githubIssueErrorf = glog.Errorf
	}

	for {
		last := time.Now()
		publisher := New(&cfg, baseRepoPath)

		if cfg.TokenFile != "" && cfg.GithubIssue != 0 && !cfg.DryRun {
			// load token
			bs, err := ioutil.ReadFile(cfg.TokenFile)
			if err != nil {
				glog.Fatalf("Failed to load token file from %q: %v", cfg.TokenFile, err)
			}
			token := strings.Trim(string(bs), " \t\n")

			// run
			logs, healthz, err := publisher.Run()
			server.SetHealth(err == nil, healthz)
			if err != nil {
				glog.Infof("Failed to run publisher: %v", err)
				if err := ReportOnIssue(err, logs, token, cfg.TargetOrg, cfg.SourceRepo, cfg.GithubIssue); err != nil {
					githubIssueErrorf("Failed to report logs on github issue: %v", err)
					server.SetHealth(false, healthz)
				}
			} else if err := CloseIssue(token, cfg.TargetOrg, cfg.SourceRepo, cfg.GithubIssue); err != nil {
				githubIssueErrorf("Failed to close issue: %v", err)
				server.SetHealth(false, healthz)
			}
		} else {
			// run
			_, hash, err := publisher.Run()
			server.SetHealth(err == nil, hash)
			if err != nil {
				glog.Infof("Failed to run publisher: %v", err)
			}
		}

		if *interval == 0 {
			break
		}

		select {
		case <-runChan:
		case <-time.After(time.Duration(int(*interval)-int(time.Since(last).Seconds())) * time.Second):
		}
	}
}
