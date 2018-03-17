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
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/renstrom/dedent"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"k8s.io/publishing-bot/pkg/cache"
	"k8s.io/publishing-bot/pkg/dependency"
	"k8s.io/publishing-bot/pkg/dependency/dep"
	"k8s.io/publishing-bot/pkg/git"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `Syncs tags between the upstream remote branch and the local checkout
of an origin branch. Tags which do not exist in origin, but in upstream
are prepended with the given prefix and then created locally to be pushed
to origin (not done by this tool).

Tags from the upstream remote are fetched as "refs/tags/<source-remote>/<tag-name>".

Usage: %s --source-remote <remote> --source-branch <source-branch>
          [--commit-message-tag <Commit-message-tag>]
          [--origin-branch <branch>]
          [--prefix <tag-prefix>]
          [--push-script <file-path>]
          [--required <pkg>,...]
          [--alternative-source <pkg>]
`, os.Args[0])
	flag.PrintDefaults()
}

const rfc2822 = "Mon Jan 02 15:04:05 -0700 2006"

var publishingBot = object.Signature{
	Name:  os.Getenv("GIT_COMMITTER_NAME"),
	Email: os.Getenv("GIT_COMMITTER_EMAIL"),
}

func main() {
	// repository flags used when the repository is not k8s.io/kubernetes
	commitMsgTag := flag.String("commit-message-tag", "Kubernetes-commit", "the git commit message tag used to point back to source commits")
	sourceRemote := flag.String("source-remote", "", "the source repo remote (e.g. upstream")
	sourceBranch := flag.String("source-branch", "", "the source repo branch (not qualified, just the name; defaults to equal <branch>)")
	publishBranch := flag.String("branch", "", "a (not qualified) branch name")
	prefix := flag.String("prefix", "kubernetes-", "a string to put in front of upstream tags")
	pushScriptPath := flag.String("push-script", "", "git-push command(s) are appended to this file to push the new tags to the origin remote")
	dependencies := flag.String("dependencies", "", "comma-separated list of repo:branch pairs of dependencies")
	required := flag.String("required", "", "comma-separated list of Golang packages that are required")
	alternativeSource := flag.String("alternative-source", "", "a package org like github.com/sttts to be used as alternative source of the dependencies")

	flag.Usage = Usage
	flag.Parse()

	if *sourceRemote == "" {
		glog.Fatalf("source-remote cannot be empty")
	}

	if *sourceBranch == "" {
		glog.Fatalf("source-branch cannot be empty")
	}

	dependentRepos, err := dependency.ParseDependencies(*dependencies)
	if err != nil {
		glog.Fatalf("Failed to parse dependencies %q: %v", *dependencies, err)
	}

	// open repo at "."
	r, err := gogit.PlainOpen(".")
	if err != nil {
		glog.Fatalf("Failed to open repo at .: %v", err)
	}

	h, err := r.Head()
	if err != nil {
		glog.Fatalf("Failed to get HEAD: %v", err)
	}
	localBranch := h.Name().Short()
	if localBranch == "" {
		glog.Fatalf("Failed to get current branch.")
	}

	if *publishBranch == "" {
		*publishBranch = localBranch
	}

	var requiredPkgs []string
	if len(*required) > 0 {
		requiredPkgs = strings.Split(*required, ",")
	}

	// get first-parent commit list of local branch
	bHead, err := git.BranchHead(r, localBranch)
	if err != nil {
		glog.Fatalf("Failed to open branch %s: %v", localBranch, err)
	}
	bFirstParents, err := git.FirstParentList(r, bHead)
	if err != nil {
		glog.Fatalf("Failed to get branch %s first-parent list: %v", localBranch, err)
	}

	// get first-parent commit list of upstream branch
	kUpdateBranch, err := r.ResolveRevision(plumbing.Revision(fmt.Sprintf("refs/remotes/%s/%s", *sourceRemote, *sourceBranch)))
	if err != nil {
		glog.Fatalf("Failed to open upstream branch %s: %v", *sourceBranch, err)
	}
	kHead, err := cache.CommitObject(r, *kUpdateBranch)
	if err != nil {
		glog.Fatalf("Failed to open upstream branch %s head: %v", *sourceBranch, err)
	}
	kFirstParents, err := git.FirstParentList(r, kHead)
	if err != nil {
		glog.Fatalf("Failed to get upstream branch %s first-parent list: %v", *sourceBranch, err)
	}

	// delete annotated remote tags locally
	fmt.Printf("Removing all local copies of origin and %s tags.\n", *sourceRemote)
	if err := removeRemoteTags(r, []string{"origin", *sourceRemote}); err != nil {
		glog.Fatalf("Failed to iterate through tags: %v", err)
	}

	// fetch tags
	fmt.Printf("Fetching tags from remote %q.\n", "origin")
	err = fetchTags(r, "origin")
	if err != nil {
		glog.Fatalf("Failed to fetch tags for %q: %v", "origin", err)
	}
	fmt.Printf("Fetching tags from remote %q.\n", *sourceRemote)
	err = fetchTags(r, *sourceRemote)
	if err != nil {
		glog.Fatalf("Failed to fetch tags for %q: %v", *sourceRemote, err)
	}

	// get all annotated tags
	bTagCommits, err := remoteTags(r, "origin")
	if err != nil {
		glog.Fatalf("Failed to iterate through origin tags: %v", err)
	}
	kTagCommits, err := remoteTags(r, *sourceRemote)
	if err != nil {
		glog.Fatalf("Failed to iterate through %s tags: %v", *sourceRemote, err)
	}

	// compute kube commit map
	fmt.Printf("Computing mapping from kube commits to the local branch.\n")
	sourceCommitsToDstCommits, err := git.SourceCommitToDstCommits(r, *commitMsgTag, bFirstParents, kFirstParents)
	if err != nil {
		glog.Fatalf("Failed to map upstream branch %s to HEAD: %v", *sourceBranch, err)
	}

	wt, err := r.Worktree()
	if err != nil {
		glog.Fatalf("Failed to get working tree: %v", err)
	}

	// create or update tags from kTagCommits as local tags with the given prefix
	createdTags := []string{}
	for name, kh := range kTagCommits {
		bName := name
		if *prefix != "" {
			bName = *prefix + name[1:] // remove the v
		}

		// ignore non-annotated tags
		tag, err := r.TagObject(kh)
		if err != nil {
			continue
		}

		// ignore old tags
		if tag.Tagger.When.Before(time.Date(2017, 9, 1, 0, 0, 0, 0, time.UTC)) {
			//fmt.Printf("Ignoring old tag origin/%s from %v\n", bName, tag.Tagger.When)
			continue
		}

		// map kube commit to local branch
		bh, found := sourceCommitsToDstCommits[tag.Target]
		if !found {
			// this means that the tag is not on the current source branch
			continue
		}

		// do not override tags (we build master first, i.e. the x.y.z-alpha.0 tag on master will not be created for feature branches)
		if tagExists(r, bName) {
			continue
		}

		// skip if it already exists in origin
		if _, found := bTagCommits[bName]; found {
			fmt.Printf("Ignoring already published tag %s.\n", bName)
			continue
		}

		// set tag in dependencies
		for i := range dependentRepos {
			dependentRepos[i].Branch = ""
			dependentRepos[i].Tag = bName
		}

		// update Godeps.json to point to actual tagged version in the dependencies. This version might differ
		// from the one currently in Godeps.json because the other repo could have gotten more commit for this
		// tag, but this repo didn't. Compare https://github.com/kubernetes/publishing-bot/issues/12 for details.
		if len(dependentRepos) > 0 {
			fmt.Printf("Checking that Godeps.json points to the actual tags in %s.\n", *dependencies)
			if err := wt.Checkout(&gogit.CheckoutOptions{Hash: bh}); err != nil {
				glog.Fatalf("Failed to checkout %v: %v", bh, err)
			}
			if _, err := updateGodepsJsonWithTaggedDependencies(r, bName, dependentRepos); err != nil {
				glog.Fatalf("Failed to update Godeps.json for tag %s: %v", bName, err)
			}
		}

		// update golang/dep Gopkg.toml
		fmt.Printf("Updating Gopkg.toml.\n")
		if err := dep.GodepToGopkg(dependentRepos, requiredPkgs, *alternativeSource); err != nil {
			glog.Fatalf("Failed to create Gopkg.toml: %v", err)
		}
		wt.Add("Gopkg.toml")

		if st, err := wt.Status(); err != nil {
			glog.Fatalf("Failed to get git status: %v", err)
		} else if !st.IsClean() {
			fmt.Printf("Adding extra commit fixing dependencies to point to %s tags.\n", bName)
			publishingBotNow := publishingBot
			publishingBotNow.When = time.Now()
			bh, err = wt.Commit(fmt.Sprintf("Fix Godeps.json to point to %s tags", bName), &gogit.CommitOptions{
				All:       true,
				Author:    &publishingBotNow,
				Committer: &publishingBotNow,
			})
			if err != nil {
				glog.Fatalf("Failed to commit Godeps/Godeps.json changes: %v", err)
			}
		}

		// create prefixed annotated tag
		fmt.Printf("Tagging %v as %q.\n", bh, bName)
		err = createAnnotatedTag(bh, bName, tag.Tagger.When, dedent.Dedent(fmt.Sprintf(`
			Kubernetes release %s

			Based on https://github.com/kubernetes/kubernetes/releases/tag/%s
			`, name, name)))
		if err != nil {
			glog.Fatalf("Failed to create tag %q: %v", bName, err)
		}
		createdTags = append(createdTags, bName)
	}

	// write push command for new tags
	if *pushScriptPath != "" && len(createdTags) > 0 {
		pushScript, err := os.OpenFile(*pushScriptPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			glog.Fatalf("Failed to open push-script %q for appending: %v", *pushScriptPath, err)
		}
		defer pushScript.Close()
		_, err = pushScript.WriteString(fmt.Sprintf("git push origin %s\n", "refs/tags/"+strings.Join(createdTags, " refs/tags/")))
		if err != nil {
			glog.Fatalf("Failed to write to push-script %q: %q", *pushScriptPath, err)
		}
	}
}

func remoteTags(r *gogit.Repository, remote string) (map[string]plumbing.Hash, error) {
	refs, err := r.Storer.IterReferences()
	if err != nil {
		glog.Fatalf("Failed to get tags: %v", err)
	}
	defer refs.Close()
	tagCommits := map[string]plumbing.Hash{}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.SymbolicReference && ref.Name().IsTag() {
			return nil
		}
		n := ref.Name().String()
		if prefix := "refs/tags/" + remote + "/"; strings.HasPrefix(n, prefix) {
			tagCommits[n[len(prefix):]] = ref.Hash()
		}
		return nil
	})
	return tagCommits, err
}

func removeRemoteTags(r *gogit.Repository, remotes []string) error {
	refs, err := r.Storer.IterReferences()
	if err != nil {
		glog.Fatalf("Failed to get tags: %v", err)
	}
	defer refs.Close()
	return refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.SymbolicReference && ref.Name().IsTag() {
			return nil
		}
		n := ref.Name().String()
		for _, remote := range remotes {
			if strings.HasPrefix(n, "refs/tags/"+remote+"/") {
				r.Storer.RemoveReference(ref.Name())
				break
			}
		}
		return nil
	})
}

func createAnnotatedTag(h plumbing.Hash, name string, date time.Time, message string) error {
	cmd := exec.Command("git", "tag", "-a", "-m", message, name, h.String())
	cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_COMMITTER_DATE=%s", date.Format(rfc2822)))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func tagExists(r *gogit.Repository, tag string) bool {
	cmd := exec.Command("git", "show-ref", fmt.Sprintf("refs/tags/%s", tag))
	return cmd.Run() == nil

	// the following does not work with go-git, for unknown reasons:
	//_, err := r.ResolveRevision(plumbing.Revision(fmt.Sprintf("refs/tags/%s", tag)))
	//return err == nil
}

func fetchTags(r *gogit.Repository, remote string) error {
	cmd := exec.Command("git", "fetch", "-q", "--no-tags", remote, fmt.Sprintf("+refs/tags/*:refs/tags/%s/*", remote))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()

	// the following with go-git does not work (yet) due to missing support for * in refspecs:
	/*
		err := r.Fetch(&gogit.FetchOptions{
			RemoteName: remote,
			RefSpecs:   []config.RefSpec{"+refs/heads/*:refs/remotes/origin/*"},
			Progress:   sideband.Progress(os.Stderr),
			Tags:       gogit.TagFollowing,
		})
		if err == gogit.NoErrAlreadyUpToDate {
			return nil
		}
		return err
	*/
}
