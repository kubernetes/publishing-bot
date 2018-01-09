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
	// src branch names that are skipped
	skippedSourceBranches []string
}

// New will create a new munger.
func New(cfg *config.Config) (*PublisherMunger, error) {
	// create munger
	p := &PublisherMunger{}

	// set the baseRepoPath
	gopath := os.Getenv("GOPATH")
	// if the SourceRepo is not kubernetes, use github.com as baseRepoPath
	p.baseRepoPath = filepath.Join(gopath, "src", "github.com", cfg.TargetOrg)
	if cfg.SourceRepo == "kubernetes" {
		p.baseRepoPath = filepath.Join(gopath, "src", "k8s.io")
	}

	// NOTE: Order of the repos is sensitive!!!
	// A dependent repo needs to be published first, so that other repos can vendor its latest revision.
	rules, err := config.LoadRules(cfg)
	if err != nil {
		return nil, err
	}
	p.reposRules = *rules
	glog.Infof("publisher munger rules: %#v\n", p.reposRules)
	p.config = cfg

	// TODO: re-enable 1.5, 1.6, 1.7 after enforcing every repo branch to update once to pick up new'ish Godep targets
	// background: when we still had sync commits, we could not correlate upstream commits correctly
	p.skippedSourceBranches = rules.SkippedSourceBranches
	return p, nil
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

	// update source repo branches that are needed by other repos.
	for _, repoRules := range p.reposRules.Rules {
		for _, branchRule := range repoRules.Branches {
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
	for _, skipped := range p.skippedSourceBranches {
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

	err := exec.Command("mkdir", "-p", dst).Run()
	if err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", "--no-tags", dstURL, dst)
	return p.plog.Run(cmd)
}

// constructs all the repos, but does not push the changes to remotes.
func (p *PublisherMunger) construct() error {
	sourceRemote := filepath.Join(p.baseRepoPath, p.config.SourceRepo, ".git")
	for _, repoRules := range p.reposRules.Rules {
		if repoRules.Skip {
			continue
		}

		// clone the destination repo
		dstDir := filepath.Join(p.baseRepoPath, repoRules.DestinationRepository, "")
		dstURL := fmt.Sprintf("https://github.com/%s/%s.git", p.config.TargetOrg, repoRules.DestinationRepository)
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

		// construct branches
		formatDeps := func(deps []config.Dependency) string {
			var depStrings []string
			for _, dep := range deps {
				depStrings = append(depStrings, fmt.Sprintf("%s:%s", dep.Repository, dep.Branch))
			}
			return strings.Join(depStrings, ",")
		}

		for _, branchRule := range repoRules.Branches {
			if p.skippedBranch(branchRule.Source.Branch) {
				continue
			}
			if len(branchRule.Source.Dir) == 0 {
				branchRule.Source.Dir = "."
				p.plog.Infof("%v: 'dir' cannot be empty, defaulting to '.'", branchRule)
			}
			// TODO: Refactor this to use environment variables instead
			cmd := exec.Command(repoRules.PublishScript, branchRule.Source.Branch, branchRule.Name, formatDeps(branchRule.Dependencies), sourceRemote, branchRule.Source.Dir, p.config.SourceRepo, p.config.SourceRepo)
			if err := p.plog.Run(cmd); err != nil {
				return err
			}
			p.plog.Infof("Successfully constructed %s", branchRule.Name)
		}
	}
	return nil
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

			cmd := exec.Command("/publish_scripts/push.sh", p.config.TokenFile, branchRule.Name)
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
	p.plog = NewPublisherLog(buf)

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
