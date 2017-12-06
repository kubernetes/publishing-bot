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

	github "k8s.io/publishing-bot/cmd/publishing-bot/config"
)

// coordinate of a piece of code
type coordinate struct {
	repo   string
	branch string
	// dir from repo root
	dir string
}

func (c coordinate) String() string {
	return fmt.Sprintf("[repository %s, branch %s, subdir %s]", c.repo, c.branch, c.dir)
}

type branchRule struct {
	src coordinate
	dst coordinate
	// k8s.io/* repos the dst dependes on
	deps []coordinate
}

// a collection of publishing rules for a single destination repo
type repoRules struct {
	dstRepo string
	// publisher.go has assumption that src.repo is always kubernetes.
	srcToDst []branchRule
	// if empty (e.g., for client-go), publisher will use its default publish script
	publishScript string
	// not updated when true
	skipped bool
}

// PublisherMunger publishes content from one repository to another one.
type PublisherMunger struct {
	reposRules []repoRules
	config     *github.Config
	// plog duplicates the logs at glog and a file
	plog *plog
	// absolute path to the k8s repos.
	k8sIOPath string
	// src branch names that are skipped
	skippedSourceBranches []string
}

// New will create a new munger.
func New(config *github.Config) (*PublisherMunger, error) {
	// validate config
	if config.SourceRepo != "kubernetes" {
		// TODO: externalize the rules below to make it generic
		return nil, fmt.Errorf("only kubernetes as source repo supported right now")
	}
	if config.TargetOrg == "" {
		return nil, fmt.Errorf("target organization cannot be empty")
	}

	// create munger
	p := &PublisherMunger{}
	gopath := os.Getenv("GOPATH")
	p.k8sIOPath = filepath.Join(gopath, "src", "k8s.io")

	clientGo := repoRules{
		skipped: false,
		dstRepo: "client-go",
		srcToDst: []branchRule{
			{
				// rule for the client-go master branch
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/client-go"},
				dst: coordinate{repo: "client-go", branch: "master", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "master"},
					{repo: "api", branch: "master"},
				},
			},
			{
				// rule for the client-go release-2.0 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.5", dir: "staging/src/k8s.io/client-go"},
				dst: coordinate{repo: "client-go", branch: "release-2.0", dir: "./"},
			},
			{
				// rule for the client-go release-3.0 branch
				src:  coordinate{repo: config.SourceRepo, branch: "release-1.6", dir: "staging/src/k8s.io/client-go"},
				dst:  coordinate{repo: "client-go", branch: "release-3.0", dir: "./"},
				deps: []coordinate{{repo: "apimachinery", branch: "release-1.6"}},
			},
			{
				// rule for the client-go release-4.0 branch
				src:  coordinate{repo: config.SourceRepo, branch: "release-1.7", dir: "staging/src/k8s.io/client-go"},
				dst:  coordinate{repo: "client-go", branch: "release-4.0", dir: "./"},
				deps: []coordinate{{repo: "apimachinery", branch: "release-1.7"}},
			},
			{
				// rule for the client-go release-5.0 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/client-go"},
				dst: coordinate{repo: "client-go", branch: "release-5.0", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.8"},
					{repo: "api", branch: "release-1.8"},
				},
			},
			{
				// rule for the client-go release-6.0 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/client-go"},
				dst: coordinate{repo: "client-go", branch: "release-6.0", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.9"},
					{repo: "api", branch: "release-1.9"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_client_go.sh",
	}

	apimachinery := repoRules{
		skipped: false,
		dstRepo: "apimachinery",
		srcToDst: []branchRule{
			{
				// rule for the apimachinery master branch
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/apimachinery"},
				dst: coordinate{repo: "apimachinery", branch: "master", dir: "./"},
			},
			{
				// rule for the apimachinery 1.6 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.6", dir: "staging/src/k8s.io/apimachinery"},
				dst: coordinate{repo: "apimachinery", branch: "release-1.6", dir: "./"},
			},
			{
				// rule for the apimachinery 1.7 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.7", dir: "staging/src/k8s.io/apimachinery"},
				dst: coordinate{repo: "apimachinery", branch: "release-1.7", dir: "./"},
			},
			{
				// rule for the apimachinery 1.8 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/apimachinery"},
				dst: coordinate{repo: "apimachinery", branch: "release-1.8", dir: "./"},
			},
			{
				// rule for the apimachinery 1.9 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/apimachinery"},
				dst: coordinate{repo: "apimachinery", branch: "release-1.9", dir: "./"},
			},
		},
		publishScript: "/publish_scripts/publish_apimachinery.sh",
	}

	apiserver := repoRules{
		skipped: false,
		dstRepo: "apiserver",
		srcToDst: []branchRule{
			{
				// rule for the apiserver master branch
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/apiserver"},
				dst: coordinate{repo: "apiserver", branch: "master", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "master"},
					{repo: "api", branch: "master"},
					{repo: "client-go", branch: "master"},
				},
			},
			{
				// rule for the apiserver 1.6 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.6", dir: "staging/src/k8s.io/apiserver"},
				dst: coordinate{repo: "apiserver", branch: "release-1.6", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.6"},
					{repo: "client-go", branch: "release-3.0"},
				},
			},
			{
				// rule for the apiserver 1.7 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.7", dir: "staging/src/k8s.io/apiserver"},
				dst: coordinate{repo: "apiserver", branch: "release-1.7", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.7"},
					{repo: "client-go", branch: "release-4.0"},
				},
			},
			{
				// rule for the apiserver 1.8 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/apiserver"},
				dst: coordinate{repo: "apiserver", branch: "release-1.8", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.8"},
					{repo: "api", branch: "release-1.8"},
					{repo: "client-go", branch: "release-5.0"},
				},
			},
			{
				// rule for the apiserver 1.9 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/apiserver"},
				dst: coordinate{repo: "apiserver", branch: "release-1.9", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.9"},
					{repo: "api", branch: "release-1.9"},
					{repo: "client-go", branch: "release-6.0"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_apiserver.sh",
	}

	kubeAggregator := repoRules{
		skipped: false,
		dstRepo: "kube-aggregator",
		srcToDst: []branchRule{
			{
				// rule for the kube-aggregator master branch
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/kube-aggregator"},
				dst: coordinate{repo: "kube-aggregator", branch: "master", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "master"},
					{repo: "api", branch: "master"},
					{repo: "client-go", branch: "master"},
					{repo: "apiserver", branch: "master"},
				},
			},
			{
				// rule for the kube-aggregator 1.6 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.6", dir: "staging/src/k8s.io/kube-aggregator"},
				dst: coordinate{repo: "kube-aggregator", branch: "release-1.6", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.6"},
					{repo: "client-go", branch: "release-3.0"},
					{repo: "apiserver", branch: "release-1.6"},
				},
			},
			{
				// rule for the kube-aggregator 1.7 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.7", dir: "staging/src/k8s.io/kube-aggregator"},
				dst: coordinate{repo: "kube-aggregator", branch: "release-1.7", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.7"},
					{repo: "client-go", branch: "release-4.0"},
					{repo: "apiserver", branch: "release-1.7"},
				},
			},
			{
				// rule for the kube-aggregator 1.8 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/kube-aggregator"},
				dst: coordinate{repo: "kube-aggregator", branch: "release-1.8", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.8"},
					{repo: "api", branch: "release-1.8"},
					{repo: "client-go", branch: "release-5.0"},
					{repo: "apiserver", branch: "release-1.8"},
				},
			},
			{
				// rule for the kube-aggregator 1.9 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/kube-aggregator"},
				dst: coordinate{repo: "kube-aggregator", branch: "release-1.9", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.9"},
					{repo: "api", branch: "release-1.9"},
					{repo: "client-go", branch: "release-6.0"},
					{repo: "apiserver", branch: "release-1.9"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_kube_aggregator.sh",
	}

	sampleAPIServer := repoRules{
		skipped: false,
		dstRepo: "sample-apiserver",
		srcToDst: []branchRule{
			{
				// rule for the sample-apiserver master branch
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/sample-apiserver"},
				dst: coordinate{repo: "sample-apiserver", branch: "master", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "master"},
					{repo: "api", branch: "master"},
					{repo: "client-go", branch: "master"},
					{repo: "apiserver", branch: "master"},
				},
			},
			{
				// rule for the sample-apiserver 1.6 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.6", dir: "staging/src/k8s.io/sample-apiserver"},
				dst: coordinate{repo: "sample-apiserver", branch: "release-1.6", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.6"},
					{repo: "client-go", branch: "release-3.0"},
					{repo: "apiserver", branch: "release-1.6"},
				},
			},
			{
				// rule for the sample-apiserver 1.7 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.7", dir: "staging/src/k8s.io/sample-apiserver"},
				dst: coordinate{repo: "sample-apiserver", branch: "release-1.7", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.7"},
					{repo: "client-go", branch: "release-4.0"},
					{repo: "apiserver", branch: "release-1.7"},
				},
			},
			{
				// rule for the sample-apiserver 1.8 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/sample-apiserver"},
				dst: coordinate{repo: "sample-apiserver", branch: "release-1.8", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.8"},
					{repo: "api", branch: "release-1.8"},
					{repo: "client-go", branch: "release-5.0"},
					{repo: "apiserver", branch: "release-1.8"},
				},
			},
			{
				// rule for the sample-apiserver 1.9 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/sample-apiserver"},
				dst: coordinate{repo: "sample-apiserver", branch: "release-1.9", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.9"},
					{repo: "api", branch: "release-1.9"},
					{repo: "client-go", branch: "release-6.0"},
					{repo: "apiserver", branch: "release-1.9"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_sample_apiserver.sh",
	}

	sampleController := repoRules{
		skipped: false,
		dstRepo: "sample-controller",
		srcToDst: []branchRule{
			{
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/sample-controller"},
				dst: coordinate{repo: "sample-controller", branch: "master", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "master"},
					{repo: "api", branch: "master"},
					{repo: "client-go", branch: "master"},
				},
			},
			{
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/sample-controller"},
				dst: coordinate{repo: "sample-controller", branch: "release-1.9", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.9"},
					{repo: "api", branch: "release-1.9"},
					{repo: "client-go", branch: "release-6.0"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_sample_controller.sh",
	}

	apiExtensionsAPIServer := repoRules{
		dstRepo: "apiextensions-apiserver",
		srcToDst: []branchRule{
			{
				// rule for the sample-apiserver master branch
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/apiextensions-apiserver"},
				dst: coordinate{repo: "apiextensions-apiserver", branch: "master", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "master"},
					{repo: "api", branch: "master"},
					{repo: "client-go", branch: "master"},
					{repo: "apiserver", branch: "master"},
				},
			},
			{
				// rule for the sample-apiserver 1.7 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.7", dir: "staging/src/k8s.io/apiextensions-apiserver"},
				dst: coordinate{repo: "apiextensions-apiserver", branch: "release-1.7", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.7"},
					{repo: "client-go", branch: "release-4.0"},
					{repo: "apiserver", branch: "release-1.7"},
				},
			},
			{
				// rule for the sample-apiserver 1.8 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/apiextensions-apiserver"},
				dst: coordinate{repo: "apiextensions-apiserver", branch: "release-1.8", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.8"},
					{repo: "api", branch: "release-1.8"},
					{repo: "client-go", branch: "release-5.0"},
					{repo: "apiserver", branch: "release-1.8"},
				},
			},
			{
				// rule for the sample-apiserver 1.8 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/apiextensions-apiserver"},
				dst: coordinate{repo: "apiextensions-apiserver", branch: "release-1.9", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.9"},
					{repo: "api", branch: "release-1.9"},
					{repo: "client-go", branch: "release-6.0"},
					{repo: "apiserver", branch: "release-1.9"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_apiextensions_apiserver.sh",
	}

	api := repoRules{
		skipped: false,
		dstRepo: "api",
		srcToDst: []branchRule{
			{
				// rule for the api master branch
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/api"},
				dst: coordinate{repo: "api", branch: "master", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "master"},
				},
			},
			{
				// rule for the api release-1.8 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/api"},
				dst: coordinate{repo: "api", branch: "release-1.8", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.8"},
				},
			},
			{
				// rule for the api release-1.9 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/api"},
				dst: coordinate{repo: "api", branch: "release-1.9", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.9"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_api.sh",
	}

	metrics := repoRules{
		dstRepo: "metrics",
		srcToDst: []branchRule{
			{
				// rule for the metrics master branch
				src: coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/metrics"},
				dst: coordinate{repo: "metrics", branch: "master", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "master"},
					{repo: "api", branch: "master"},
					{repo: "client-go", branch: "master"},
				},
			},
			{
				// rule for the sample-apiserver 1.7 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.7", dir: "staging/src/k8s.io/metrics"},
				dst: coordinate{repo: "metrics", branch: "release-1.7", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.7"},
					{repo: "client-go", branch: "release-4.0"},
				},
			},
			{
				// rule for the sample-apiserver 1.8 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/metrics"},
				dst: coordinate{repo: "metrics", branch: "release-1.8", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.8"},
					{repo: "api", branch: "release-1.8"},
					{repo: "client-go", branch: "release-5.0"},
				},
			},
			{
				// rule for the sample-apiserver 1.9 branch
				src: coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/metrics"},
				dst: coordinate{repo: "metrics", branch: "release-1.9", dir: "./"},
				deps: []coordinate{
					{repo: "apimachinery", branch: "release-1.9"},
					{repo: "api", branch: "release-1.9"},
					{repo: "client-go", branch: "release-6.0"},
				},
			},
		},
		publishScript: "/publish_scripts/publish_metrics.sh",
	}

	codeGenerator := repoRules{
		dstRepo: "code-generator",
		srcToDst: []branchRule{
			{
				// rule for the metrics master branch
				src:  coordinate{repo: config.SourceRepo, branch: "master", dir: "staging/src/k8s.io/code-generator"},
				dst:  coordinate{repo: "code-generator", branch: "master", dir: "./"},
				deps: []coordinate{},
			},
			{
				// rule for the metrics release-1.8 branch
				src:  coordinate{repo: config.SourceRepo, branch: "release-1.8", dir: "staging/src/k8s.io/code-generator"},
				dst:  coordinate{repo: "code-generator", branch: "release-1.8", dir: "./"},
				deps: []coordinate{},
			},
			{
				// rule for the metrics release-1.9 branch
				src:  coordinate{repo: config.SourceRepo, branch: "release-1.9", dir: "staging/src/k8s.io/code-generator"},
				dst:  coordinate{repo: "code-generator", branch: "release-1.9", dir: "./"},
				deps: []coordinate{},
			},
		},
		publishScript: "/publish_scripts/publish_code_generator.sh",
	}

	// NOTE: Order of the repos is sensitive!!! A dependent repo needs to be published first, so that other repos can vendor its latest revision.
	p.reposRules = []repoRules{apimachinery, api, clientGo, apiserver, kubeAggregator, sampleAPIServer, sampleController, apiExtensionsAPIServer, metrics, codeGenerator}
	glog.Infof("publisher munger rules: %#v\n", p.reposRules)
	p.config = config

	// TODO: re-enable 1.5, 1.6, 1.7 after enforcing every repo branch to update once to pick up new'ish Godep targets
	// background: when we still had sync commits, we could not correlate upstream commits correctly
	p.skippedSourceBranches = []string{
		"release-1.5",
		"release-1.6",
		"release-1.7",
	}
	return p, nil
}

// update the local checkout of k8s.io/kubernetes
func (p *PublisherMunger) updateKubernetes() error {
	cmd := exec.Command("git", "fetch", "origin")
	cmd.Dir = filepath.Join(p.k8sIOPath, "kubernetes")
	if err := p.plog.Run(cmd); err != nil {
		return err
	}

	// update kubernetes branches that are needed by other k8s.io repos.
	for _, repoRules := range p.reposRules {
		for _, branchRule := range repoRules.srcToDst {
			if p.skippedBranch(branchRule.src.branch) {
				continue
			}

			src := branchRule.src
			// we assume src.repo is always kubernetes
			cmd := exec.Command("git", "branch", "-f", src.branch, fmt.Sprintf("origin/%s", src.branch))
			cmd.Dir = filepath.Join(p.k8sIOPath, "kubernetes")
			if err := p.plog.Run(cmd); err == nil {
				continue
			}
			// probably the error is because we cannot do `git branch -f` while
			// current branch is src.branch, so try `git reset --hard` instead.
			cmd = exec.Command("git", "reset", "--hard", fmt.Sprintf("origin/%s", src.branch))
			cmd.Dir = filepath.Join(p.k8sIOPath, "kubernetes")
			if err := p.plog.Run(cmd); err != nil {
				return err
			}
		}
	}
	return nil
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
	kubernetesRemote := filepath.Join(p.k8sIOPath, "kubernetes", ".git")
	for _, repoRules := range p.reposRules {
		if repoRules.skipped {
			continue
		}

		// clone the destination repo
		dstDir := filepath.Join(p.k8sIOPath, repoRules.dstRepo, "")
		dstURL := fmt.Sprintf("https://github.com/%s/%s.git", p.config.TargetOrg, repoRules.dstRepo)
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
		formatDeps := func(deps []coordinate) string {
			var depStrings []string
			for _, dep := range deps {
				depStrings = append(depStrings, fmt.Sprintf("%s:%s", dep.repo, dep.branch))
			}
			return strings.Join(depStrings, ",")
		}

		for _, branchRule := range repoRules.srcToDst {
			if p.skippedBranch(branchRule.src.branch) {
				continue
			}

			cmd := exec.Command(repoRules.publishScript, branchRule.src.branch, branchRule.dst.branch, formatDeps(branchRule.deps), kubernetesRemote)
			if err := p.plog.Run(cmd); err != nil {
				return err
			}
			p.plog.Infof("Successfully constructed %s", branchRule.dst)
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
	for _, repoRules := range p.reposRules {
		if repoRules.skipped {
			continue
		}

		dstDir := filepath.Join(p.k8sIOPath, repoRules.dstRepo, "")
		if err := os.Chdir(dstDir); err != nil {
			return err
		}
		for _, branchRule := range repoRules.srcToDst {
			if p.skippedBranch(branchRule.src.branch) {
				continue
			}

			cmd := exec.Command("/publish_scripts/push.sh", p.config.TokenFile, branchRule.dst.branch)
			if err := p.plog.Run(cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

// Run constructs the repos and pushes them.
func (p *PublisherMunger) Run() error {
	buf := bytes.NewBuffer(nil)
	p.plog = NewPublisherLog(buf)

	if err := p.updateKubernetes(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return err
	}
	if err := p.construct(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return err
	}
	if err := p.publish(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return err
	}
	return nil
}
