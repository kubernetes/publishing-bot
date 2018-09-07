/*
Copyright 2016 The Kubernetes Authors.

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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/golang/glog"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

// PublisherMunger publishes content from one repository to another one.
type PublisherMunger struct {
	reposRules config.RepositoryRules
	config     *config.Config
	// plog duplicates the logs at glog and a file
	plog *plog
	// absolute path to the repos.
	baseRepoPath string
}

// New will create a new munger.
func New(cfg *config.Config, baseRepoPath string) *PublisherMunger {
	// create munger
	return &PublisherMunger{
		baseRepoPath: baseRepoPath,
		config:       cfg,
	}
}

// update the local checkout of the source repository
func (p *PublisherMunger) updateSourceRepo() (string, error) {
	repoDir := filepath.Join(p.baseRepoPath, p.config.SourceRepo)

	cmd := exec.Command("git", "fetch", "origin")
	cmd.Dir = repoDir
	if err := p.plog.Run(cmd); err != nil {
		return "", err
	}

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = repoDir
	hash, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed running %v on %q repo: %v", strings.Join(cmd.Args, " "), p.config.SourceRepo, err)
	}

	rules, err := config.LoadRules(p.config.RulesFile)
	if err != nil {
		return "", err
	}
	p.reposRules = *rules
	glog.Infof("Loaded %d repository rules from %s", len(p.reposRules.Rules), p.config.RulesFile)

	// update source repo branches that are needed by other repos.
	for _, repoRule := range p.reposRules.Rules {
		for _, branchRule := range repoRule.Branches {
			if p.skippedBranch(branchRule.Source.Branch) {
				continue
			}

			src := branchRule.Source
			// we assume src.repo is always kubernetes
			cmd := exec.Command("git", "branch", "-f", src.Branch, fmt.Sprintf("origin/%s", src.Branch))
			cmd.Dir = repoDir
			if err := p.plog.Run(cmd); err == nil {
				continue
			}
			// probably the error is because we cannot do `git branch -f` while
			// current branch is src.branch, so try `git reset --hard` instead.
			cmd = exec.Command("git", "reset", "--hard", fmt.Sprintf("origin/%s", src.Branch))
			cmd.Dir = repoDir
			if err := p.plog.Run(cmd); err != nil {
				return "", err
			}
		}
	}
	return strings.Trim(string(hash), " \t\n"), nil
}

func (p *PublisherMunger) skippedBranch(b string) bool {
	for _, skipped := range p.reposRules.SkippedSourceBranches {
		if b == skipped {
			return true
		}
	}
	return false
}

// git clone dstURL to dst if dst doesn't exist yet.
func (p *PublisherMunger) ensureCloned(dst string, dstURL string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}

	cmd := exec.Command("mkdir", "-p", dst)
	if err := p.plog.Run(cmd); err != nil {
		return err
	}
	cmd = exec.Command("git", "clone", dstURL, dst)
	if err := p.plog.Run(cmd); err != nil {
		return err
	}
	cmd = exec.Command("/bin/bash", "-c", "git tag -l | xargs git tag -d")
	cmd.Dir = dst
	return p.plog.Run(cmd)
}

// constructs all the repos, but does not push the changes to remotes.
func (p *PublisherMunger) construct() error {
	sourceRemote := filepath.Join(p.baseRepoPath, p.config.SourceRepo, ".git")
	for _, repoRule := range p.reposRules.Rules {
		if repoRule.Skip {
			continue
		}

		// clone the destination repo
		dstDir := filepath.Join(p.baseRepoPath, repoRule.DestinationRepository, "")
		dstURL := fmt.Sprintf("https://%s/%s/%s.git", p.config.GithubHost, p.config.TargetOrg, repoRule.DestinationRepository)
		if err := p.ensureCloned(dstDir, dstURL); err != nil {
			p.plog.Errorf("%v", err)
			return err
		}
		p.plog.Infof("Successfully ensured %s exists", dstDir)
		if err := os.Chdir(dstDir); err != nil {
			return err
		}

		// delete tags
		cmd := exec.Command("/bin/bash", "-c", "git tag | xargs git tag -d >/dev/null")
		if err := p.plog.Run(cmd); err != nil {
			return err
		}

		formatDeps := func(deps []config.Dependency) string {
			var depStrings []string
			for _, dep := range deps {
				depStrings = append(depStrings, fmt.Sprintf("%s:%s", dep.Repository, dep.Branch))
			}
			return strings.Join(depStrings, ",")
		}

		// construct branches
		for _, branchRule := range repoRule.Branches {
			if p.skippedBranch(branchRule.Source.Branch) {
				continue
			}
			if len(branchRule.Source.Dir) == 0 {
				branchRule.Source.Dir = "."
				p.plog.Infof("%v: 'dir' cannot be empty, defaulting to '.'", branchRule)
			}

			// get old HEAD. Ignore errors as the branch might be non-existent
			oldHead, _ := exec.Command("git", "rev-parse", fmt.Sprintf("origin/%s", branchRule.Name)).Output()

			goPath := os.Getenv("GOPATH")
			branchEnv := append([]string(nil), os.Environ()...) // make mutable
			if branchRule.GoVersion != "" {
				goRoot := filepath.Join(goPath, "go-"+branchRule.GoVersion)
				branchEnv = append(branchEnv, "GOROOT="+goRoot)
				goBin := filepath.Join(goRoot, "bin")
				branchEnv = updateEnv(branchEnv, "PATH", prependPath(goBin), goBin)
			}

			skipTags := ""
			if p.reposRules.SkipTags {
				skipTags = "true"
				p.plog.Infof("synchronizing tags is disabled")
			}

			// TODO: Refactor this to use environment variables instead
			repoPublishScriptPath := filepath.Join(p.config.BasePublishScriptPath, "construct.sh")
			cmd := exec.Command(repoPublishScriptPath,
				repoRule.DestinationRepository,
				branchRule.Source.Branch,
				branchRule.Name,
				formatDeps(branchRule.Dependencies),
				strings.Join(branchRule.RequiredPackages, ":"),
				sourceRemote,
				branchRule.Source.Dir,
				p.config.SourceRepo,
				p.config.SourceRepo,
				p.config.BasePackage,
				fmt.Sprintf("%v", repoRule.Library),
				strings.Join(p.reposRules.RecursiveDeletePatterns, " "),
				skipTags,
			)
			cmd.Env = append([]string(nil), branchEnv...) // make mutable
			if p.reposRules.SkipGodeps {
				cmd.Env = append(cmd.Env, "PUBLISHER_BOT_SKIP_GODEPS=true")
			}
			if err := p.plog.Run(cmd); err != nil {
				return err
			}

			newHead, _ := exec.Command("git", "rev-parse", fmt.Sprintf("origin/%s", branchRule.Name)).Output()
			if len(repoRule.SmokeTest) > 0 && string(oldHead) != string(newHead) {
				p.plog.Infof("Running smoke tests for branch %s", branchRule.Name)
				cmd := exec.Command("/bin/bash", "-xec", repoRule.SmokeTest)
				cmd.Env = append([]string(nil), branchEnv...) // make mutable
				if err := p.plog.Run(cmd); err != nil {
					// do not clean up to allow debugging with kubectl-exec.
					return err
				}
				exec.Command("git", "reset", "--hard").Run()
				exec.Command("git", "clean", "-f", "-f", "-d").Run()
			}

			p.plog.Infof("Successfully constructed %s", branchRule.Name)
		}
	}
	return nil
}

func updateEnv(env []string, key string, change func(string) string, val string) []string {
	for i := range env {
		if strings.HasPrefix(env[i], key+"=") {
			ss := strings.SplitN(env[i], "=", 2)
			env[i] = fmt.Sprintf("%s=%s", key, change(ss[1]))
			return env
		}
	}
	return append(env, fmt.Sprintf("%s=%s", key, val))
}

func prependPath(p string) func(string) string {
	return func(s string) string {
		if s == "" {
			return p
		}
		return p + ":" + s
	}
}

// publish to remotes.
func (p *PublisherMunger) publish() error {
	if p.config.DryRun {
		p.plog.Infof("Skipping push in dry-run mode")
		return nil
	}

	if p.config.TokenFile == "" {
		return fmt.Errorf("token cannot be empty in non-dry-run mode")
	}

	// NOTE: because some repos depend on each other, e.g., client-go depends on
	// apimachinery, they should be published atomically, but it's not supported
	// by github.
	for _, repoRules := range p.reposRules.Rules {
		if repoRules.Skip {
			continue
		}

		dstDir := filepath.Join(p.baseRepoPath, repoRules.DestinationRepository, "")
		if err := os.Chdir(dstDir); err != nil {
			return err
		}
		for _, branchRule := range repoRules.Branches {
			if p.skippedBranch(branchRule.Source.Branch) {
				continue
			}

			cmd := exec.Command(p.config.BasePublishScriptPath+"/push.sh", p.config.TokenFile, branchRule.Name)
			if err := p.plog.Run(cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

// Run constructs the repos and pushes them.
func (p *PublisherMunger) Run() (string, string, error) {
	buf := bytes.NewBuffer(nil)
	var err error
	if p.plog, err = NewPublisherLog(buf, path.Join(p.baseRepoPath, "run.log")); err != nil {
		return "", "", err
	}

	hash, err := p.updateSourceRepo()
	if err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return p.plog.Logs(), hash, err
	}
	if err := p.construct(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return p.plog.Logs(), hash, err
	}
	if err := p.publish(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return p.plog.Logs(), hash, err
	}
	return p.plog.Logs(), hash, nil
}
