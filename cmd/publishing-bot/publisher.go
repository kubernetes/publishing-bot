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
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
	"k8s.io/publishing-bot/pkg/golang"
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

// update the local checkout of the source repository. It returns the branch heads.
func (p *PublisherMunger) updateSourceRepo() (map[string]plumbing.Hash, error) {
	repoDir := filepath.Join(p.baseRepoPath, p.config.SourceRepo)

	// fetch origin
	glog.Infof("Fetching origin at %s.", repoDir)
	r, err := gogit.PlainOpen(repoDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open repo at %s: %v", repoDir, err)
	}
	if err := r.Fetch(&gogit.FetchOptions{
		Tags:     gogit.AllTags,
		Progress: os.Stdout,
	}); err != nil && err != gogit.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("failed to fetch at %s: %v", repoDir, err)
	}

	// disable text conversion
	// TODO: remove when go-git supports text conversion to be consistent with cli git
	attrFile := filepath.Join(repoDir, ".git", "info", "attributes")
	if _, err := os.Stat(attrFile); os.IsNotExist(err) {
		glog.Infof("Disabling text conversion at %s.", repoDir)
		os.MkdirAll(filepath.Join(repoDir, ".git", "info"), 0755)
		if err := ioutil.WriteFile(attrFile, []byte(`
* -text
`), 0644); err != nil {
			return nil, fmt.Errorf("failed to create .git/info/attributes: %v", err)
		}

		fis, err := ioutil.ReadDir(repoDir)
		if err != nil {
			return nil, err
		}
		for _, fi := range fis {
			if fi.Name() != ".git" {
				if err := os.RemoveAll(filepath.Join(repoDir, fi.Name())); err != nil {
					return nil, err
				}
			}
		}
	}

	// checkout head
	glog.Infof("Checking out HEAD at %s.", repoDir)
	w, err := r.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to open worktree at %s: %v", repoDir, err)
	}
	head, err := r.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get head at %s: %v", repoDir, err)
	}
	if err := w.Checkout(&gogit.CheckoutOptions{Hash: head.Hash(), Force: true}); err != nil {
		return nil, fmt.Errorf("failed to checkout HEAD at %s: %v", repoDir, err)
	}

	// create/update local branch for all origin branches. Those are fetches into the destination repos later (as upstream/<branch>).
	refs, err := r.Storer.IterReferences()
	if err != nil {
		return nil, fmt.Errorf("failed to get branches: %v", err)
	}
	glog.Infof("Updating local branches at %s.", repoDir)
	heads := map[string]plumbing.Hash{}
	if err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()

		originPrefix := "refs/remotes/origin/"
		if !strings.Contains(name, originPrefix) || ref.Type() != plumbing.HashReference {
			return nil
		}

		shortName := strings.TrimPrefix(name, originPrefix)
		localBranch := plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/"+shortName), ref.Hash())
		if err := r.Storer.SetReference(localBranch); err != nil {
			return fmt.Errorf("failed to create reference %s pointing to %s", localBranch.Name(), localBranch.Hash().String())
		}

		heads[shortName] = localBranch.Hash()

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to process branches: %v", err)
	}

	return heads, nil
}

// update the active rules
func (p *PublisherMunger) updateRules() error {
	repoDir := filepath.Join(p.baseRepoPath, p.config.SourceRepo)

	glog.Infof("Checking out master at %s.", repoDir)
	cmd := exec.Command("git", "checkout", "master")
	cmd.Dir = repoDir
	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to checkout master: %v", err)
	}

	rules, err := config.LoadRules(p.config.RulesFile)
	if err != nil {
		return err
	}
	if err := config.Validate(rules); err != nil {
		return err
	}

	p.reposRules = *rules
	glog.Infof("Loaded %d repository rules from %s.", len(p.reposRules.Rules), p.config.RulesFile)
	return nil
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

func (p *PublisherMunger) runSmokeTests(smokeTest, oldHead, newHead string, branchEnv []string) error {
	if len(smokeTest) > 0 && string(oldHead) != string(newHead) {
		cmd := exec.Command("/bin/bash", "-xec", smokeTest)
		cmd.Env = append([]string(nil), branchEnv...) // make mutable
		cmd.Env = append(cmd.Env, "GO111MODULE=on")
		cmd.Env = append(cmd.Env, fmt.Sprintf("GOPROXY=file://%s/pkg/mod/cache/download", os.Getenv("GOPATH")))
		if err := p.plog.Run(cmd); err != nil {
			// do not clean up to allow debugging with kubectl-exec.
			return err
		}
		exec.Command("git", "reset", "--hard").Run()
		exec.Command("git", "clean", "-f", "-f", "-d").Run()
	}
	return nil
}

// constructs all the repos, but does not push the changes to remotes.
func (p *PublisherMunger) construct() error {
	sourceRemote := filepath.Join(p.baseRepoPath, p.config.SourceRepo, ".git")

	if err := golang.InstallGoVersions(&p.reposRules); err != nil {
		return err
	}

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

			// get old published hash to eventually skip cherry picking
			var lastPublishedUpstreamHash string
			bs, err := ioutil.ReadFile(path.Join(p.baseRepoPath, publishedFileName(repoRule.DestinationRepository, branchRule.Name)))
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err == nil {
				lastPublishedUpstreamHash = string(bs)
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
				lastPublishedUpstreamHash,
			)
			cmd.Env = append([]string(nil), branchEnv...) // make mutable
			if !p.reposRules.SkipGodeps {
				cmd.Env = append(cmd.Env, "PUBLISHER_BOT_GENERATE_GODEPS=true")
			}
			if p.reposRules.SkipGomod {
				cmd.Env = append(cmd.Env, "PUBLISHER_BOT_SKIP_GOMOD=true")
			}
			if err := p.plog.Run(cmd); err != nil {
				return err
			}

			newHead, _ := exec.Command("git", "rev-parse", "HEAD").Output()

			p.plog.Infof("Running branch-specific smoke tests for branch %s", branchRule.Name)
			if err := p.runSmokeTests(branchRule.SmokeTest, string(oldHead), string(newHead), branchEnv); err != nil {
				return err
			}

			p.plog.Infof("Running repo-specific smoke tests for branch %s", branchRule.Name)
			if err := p.runSmokeTests(repoRule.SmokeTest, string(oldHead), string(newHead), branchEnv); err != nil {
				return err
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
func (p *PublisherMunger) publish(newUpstreamHeads map[string]plumbing.Hash) error {
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

		p.plog.Infof("Pushing branches for %s", repoRules.DestinationRepository)
		for _, branchRule := range repoRules.Branches {
			if p.skippedBranch(branchRule.Source.Branch) {
				continue
			}

			cmd := exec.Command(p.config.BasePublishScriptPath+"/push.sh", p.config.TokenFile, branchRule.Name)
			if err := p.plog.Run(cmd); err != nil {
				return err
			}

			upstreamBranchHead, ok := newUpstreamHeads[branchRule.Source.Branch]
			if !ok {
				return fmt.Errorf("no upstream branch %q found", branchRule.Source.Branch)
			}
			if err := ioutil.WriteFile(path.Join(path.Dir(dstDir), publishedFileName(repoRules.DestinationRepository, branchRule.Name)), []byte(upstreamBranchHead.String()), 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func publishedFileName(repo, branch string) string {
	return fmt.Sprintf("published-%s-%s", repo, branch)
}

// Run constructs the repos and pushes them. It returns logs and the last master hash.
func (p *PublisherMunger) Run() (string, string, error) {
	buf := bytes.NewBuffer(nil)
	var err error
	if p.plog, err = NewPublisherLog(buf, path.Join(p.baseRepoPath, "run.log")); err != nil {
		return "", "", err
	}

	newUpstreamHeads, err := p.updateSourceRepo()
	if err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return p.plog.Logs(), "", err
	}
	if err := p.updateRules(); err != nil { // this comes after the source update because we might fetch the rules from there.
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return p.plog.Logs(), "", err
	}
	if err := p.construct(); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return p.plog.Logs(), "", err
	}
	if err := p.publish(newUpstreamHeads); err != nil {
		p.plog.Errorf("%v", err)
		p.plog.Flush()
		return p.plog.Logs(), "", err
	}

	var masterHead string
	if h, ok := newUpstreamHeads["master"]; ok {
		masterHead = h.String()
	}

	return p.plog.Logs(), masterHead, nil
}
