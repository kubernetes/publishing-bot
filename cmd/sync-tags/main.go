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
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/blang/semver"
	"github.com/golang/glog"
	"github.com/renstrom/dedent"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/protocol/packp/sideband"

	"k8s.io/publishing-bot/pkg/cache"
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
	prefix := flag.String("prefix", "kubernetes-", "a string to put in front of upstream tags")
	pushScriptPath := flag.String("push-script", "", "git-push command(s) are appended to this file to push the new tags to the origin remote")
	dependencies := flag.String("dependencies", "", "comma-separated list of repo:branch pairs of dependencies")
	skipFetch := flag.Bool("skip-fetch", false, "skip fetching tags")
	generateGodeps := flag.Bool("generate-godeps", false, "regenerate Godeps.json from go.mod")
	mappingOutputFile := flag.String("mapping-output-file", "", "a file name to write the source->dest hash mapping to ({{.Tag}} is substituted with the tag name, {{.Branch}} with the local branch name)")
	publishSemverTags := flag.Bool("publish-v0-semver", false, "publish v0.x.y tag at destination repo for v1.x.y tag at the source repo")

	flag.Usage = Usage
	flag.Parse()

	if *sourceRemote == "" {
		glog.Fatalf("source-remote cannot be empty")
	}

	if *sourceBranch == "" {
		glog.Fatalf("source-branch cannot be empty")
	}

	var dependentRepos []string
	if len(*dependencies) > 0 {
		for _, pair := range strings.Split(*dependencies, ",") {
			ps := strings.Split(pair, ":")
			dependentRepos = append(dependentRepos, ps[0])
		}
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

	// delete remote tags locally
	if !*skipFetch {
		fmt.Printf("Removing all local copies of origin and %s tags.\n", *sourceRemote)
		if err := removeRemoteTags(r, "origin", *sourceRemote); err != nil {
			glog.Fatalf("Failed to iterate through tags: %v", err)
		}
	}

	// get upstream tags
	if !*skipFetch {
		fmt.Printf("Fetching tags from remote %q.\n", *sourceRemote)
		err = fetchTags(r, *sourceRemote)
		if err != nil {
			glog.Fatalf("Failed to fetch tags for %q: %v", *sourceRemote, err)
		}
	}
	kTagCommits, err := remoteTags(r, *sourceRemote)
	if err != nil {
		glog.Fatalf("Failed to iterate through %s tags: %v", *sourceRemote, err)
	}

	// get all origin tags
	if !*skipFetch {
		fmt.Printf("Fetching tags from remote %q.\n", "origin")
		err = fetchTags(r, "origin")
		if err != nil {
			glog.Fatalf("Failed to fetch tags for %q: %v", "origin", err)
		}
	}
	bTagCommits, err := remoteTags(r, "origin")
	if err != nil {
		glog.Fatalf("Failed to iterate through origin tags: %v", err)
	}

	// filter tags by source branch
	kFirstParentCommits := map[string]struct{}{}
	for _, kc := range kFirstParents {
		kFirstParentCommits[kc.Hash.String()] = struct{}{}
	}
	for name, kh := range kTagCommits {
		// ignore non-annotated tags
		tag, err := r.TagObject(kh)
		if err != nil {
			delete(kTagCommits, name)
			continue
		}

		// delete tag not on the source branch
		if _, ok := kFirstParentCommits[tag.Target.String()]; !ok {
			delete(kTagCommits, name)
		}
	}

	var sourceCommitsToDstCommits map[plumbing.Hash]plumbing.Hash

	mappingFilesWritten := map[string]bool{}

	// create or update tags from kTagCommits as local tags with the given prefix
	createdTags := []string{}
	for name, kh := range kTagCommits {
		bName := name
		if *prefix != "" {
			bName = *prefix + name[1:] // remove the v
		}

		var (
			semverTag        = ""
			publishSemverTag = false
		)
		// if we are publishing semver tags
		if *publishSemverTags {
			// and this is a valid v1... semver tag
			if _, semverErr := semver.Parse(name[1:]); semverErr == nil && strings.HasPrefix(name, "v1.") {
				publishSemverTag = true
				semverTag = "v0." + strings.TrimPrefix(name, "v1.") // replace v1.x.y with v0.x.y
			}
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

		// skip if either tag exists at origin
		_, nonSemverTagAtOrigin := bTagCommits[bName]
		_, semverTagAtOrigin := bTagCommits[semverTag]
		if nonSemverTagAtOrigin || (publishSemverTag && semverTagAtOrigin) {
			continue
		}

		// if any of the tag exists locally,
		// delete the tags, clear the cache and recreate them
		if tagExists(r, bName) {
			commit, commitTime, err := taggedCommitHashAndTime(r, bName)
			if err != nil {
				glog.Fatalf("Failed to get tag %s: %v", bName, err)
			}
			rev := commit.String()
			pseudoVersion := fmt.Sprintf("v0.0.0-%s-%s", commitTime.UTC().Format("20060102150405"), rev[:12])

			fmt.Printf("Clearing cache for local tag %s.\n", pseudoVersion)
			if err := cleanCacheForTag(pseudoVersion); err != nil {
				glog.Fatalf("Failed to clean go mod cache for %s: %v", pseudoVersion, err)
			}

			if err := deleteTag(bName); err != nil {
				glog.Fatalf("Failed to delete tag %s: %v", bName, err)
			}
		}

		if publishSemverTag && tagExists(r, semverTag) {
			fmt.Printf("Clearing cache for local tag %s.\n", semverTag)
			if err := cleanCacheForTag(semverTag); err != nil {
				glog.Fatalf("Failed to clean go mod cache for %s: %v", semverTag, err)
			}
			if err := deleteTag(semverTag); err != nil {
				glog.Fatalf("Failed to delete tag %s: %v", semverTag, err)
			}
		}

		// lazily compute kube commit map
		if sourceCommitsToDstCommits == nil {
			bRevision, err := r.ResolveRevision(plumbing.Revision(fmt.Sprintf("refs/heads/%s", localBranch)))
			if err != nil {
				glog.Fatalf("Failed to open branch %s: %v", localBranch, err)
			}
			fmt.Printf("Computing mapping from kube commits to the local branch %q at %s because %q seems to be relevant.\n", localBranch, bRevision.String(), bName)
			bHeadCommit, err := cache.CommitObject(r, *bRevision)
			if err != nil {
				glog.Fatalf("Failed to open branch %s head: %v", localBranch, err)
			}
			bFirstParents, err := git.FirstParentList(r, bHeadCommit)
			if err != nil {
				glog.Fatalf("Failed to get branch %s first-parent list: %v", localBranch, err)
			}
			sourceCommitsToDstCommits, err = git.SourceCommitToDstCommits(r, *commitMsgTag, bFirstParents, kFirstParents)
			if err != nil {
				glog.Fatalf("Failed to map upstream branch %s to HEAD: %v", *sourceBranch, err)
			}
		}

		// map kube commit to local branch
		bh, found := sourceCommitsToDstCommits[tag.Target]
		if !found {
			// this means that the tag is not on the current source branch
			continue
		}

		// store source->dest hash mapping for debugging
		if *mappingOutputFile != "" {
			fname := mappingOutputFileName(*mappingOutputFile, localBranch, bName)
			if !mappingFilesWritten[fname] {
				fmt.Printf("Writing source->dest hash mapping to %q\n", fname)
				f, err := os.Create(fname)
				if err != nil {
					glog.Fatal(f)
				}
				if err := writeKubeCommitMapping(f, r, sourceCommitsToDstCommits, kFirstParents); err != nil {
					glog.Fatal(err)
				}
				defer f.Close()

				mappingFilesWritten[fname] = true
			}
		}

		// update go.mod or Godeps.json to point to actual tagged version in the dependencies. This version might differ
		// from the one currently in go.mod  or Godeps.json because the other repo could have gotten more commit for this
		// tag, but this repo didn't. Compare https://github.com/kubernetes/publishing-bot/issues/12 for details.
		var changed, goModExists bool
		_, err = os.Stat("go.mod")
		if err == nil {
			goModExists = true
		}

		if len(dependentRepos) > 0 {
			wt := checkoutBranchTagCommit(r, bh, dependentRepos)

			// if go.mod exists, fix only go.mod and generate Godeps.json from it later
			// if it doesn't exist, check if Godeps.json exists, and update it
			if !goModExists {
				if _, err2 := os.Stat("Godeps/Godeps.json"); err2 == nil {
					fmt.Printf("Updating Godeps.json to point to %s tag.\n", bName)
					changed, err = updateGodepsJsonWithTaggedDependencies(bName, dependentRepos)
					if err != nil {
						glog.Fatalf("Failed to update Godeps.json for tag %s: %v", bName, err)
					}
				}
			} else {
				if publishSemverTag {
					changed = updateGoModAndGodeps(semverTag, dependentRepos, *generateGodeps, true)
				} else {
					changed = updateGoModAndGodeps(bName, dependentRepos, *generateGodeps, false)
				}
			}

			if changed {
				if publishSemverTag {
					bh = createCommitToFixDeps(wt, semverTag)
				} else {
					bh = createCommitToFixDeps(wt, bName)
				}
			}
		}

		// create semver annotated tag
		if publishSemverTag {
			fmt.Printf("Tagging %v as %q.\n", bh, semverTag)
			err = createAnnotatedTag(bh, semverTag, tag.Tagger.When, dedent.Dedent(fmt.Sprintf(`
			Kubernetes release %s

			Based on https://github.com/kubernetes/kubernetes/releases/tag/%s
			`, name, name)))
			if err != nil {
				glog.Fatalf("Failed to create tag %q: %v", semverTag, err)
			}
			createdTags = append(createdTags, semverTag)
		}

		// create non-semver prefixed annotated tag
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
	// we use git push --atomic because it treats
	// any existing releases which have only non-semver tags as no-ops
	// and both semver and non-semver tags are targeted in a single operation
	if *pushScriptPath != "" && len(createdTags) > 0 {
		pushScript, err := os.OpenFile(*pushScriptPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
		if err != nil {
			glog.Fatalf("Failed to open push-script %q for appending: %v", *pushScriptPath, err)
		}
		defer pushScript.Close()
		_, err = pushScript.WriteString(fmt.Sprintf("git push --atomic origin %s\n", "refs/tags/"+strings.Join(createdTags, " refs/tags/")))
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

func removeRemoteTags(r *gogit.Repository, remotes ...string) error {
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
	setUsernameCmd := exec.Command("git", "config", "user.name", publishingBot.Name)
	if err := setUsernameCmd.Run(); err != nil {
		return fmt.Errorf("Unable to set global configuration: %v", err)
	}
	setEmailCmd := exec.Command("git", "config", "user.email", publishingBot.Email)
	if err := setEmailCmd.Run(); err != nil {
		return fmt.Errorf("Unable to set global configuration: %v", err)
	}
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
	err := r.Fetch(&gogit.FetchOptions{
		RemoteName: remote,
		RefSpecs:   []config.RefSpec{config.RefSpec(fmt.Sprintf("+refs/tags/*:refs/tags/%s/*", remote))},
		Progress:   sideband.Progress(os.Stderr),
		Tags:       gogit.NoTags,
	})
	if err == gogit.NoErrAlreadyUpToDate {
		return nil
	}
	return err
}

func writeKubeCommitMapping(w io.Writer, r *gogit.Repository, m map[plumbing.Hash]plumbing.Hash, kFirstParents []*object.Commit) error {
	for _, kc := range kFirstParents {
		msg := strings.SplitN(kc.Message, "\n", 2)[0]
		var err error
		if dh, ok := m[kc.Hash]; ok {
			_, err = fmt.Fprintf(w, "%s %s %s\n", kc.Hash, dh, msg)
		} else {
			_, err = fmt.Fprintf(w, "%s <not-found> %s\n", kc.Hash, msg)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func mappingOutputFileName(fnameTpl string, branch, tag string) string {
	tpl, err := template.New("mapping-output-file").Parse(fnameTpl)
	if err != nil {
		glog.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, struct {
		Tag    string
		Branch string
	}{
		Tag:    tag,
		Branch: branch,
	}); err != nil {
		glog.Fatal(err)
	}
	return string(buf.Bytes())
}

func regenerateGodepsFromGoMod() error {
	goListCommand := exec.Command("go", "list", "-m", "-json", "all")
	goListCommand.Env = append(os.Environ(), "GO111MODULE=on", fmt.Sprintf("GOPROXY=file://%s/pkg/mod/cache/download", os.Getenv("GOPATH")))
	goMod, err := goListCommand.Output()
	if err != nil {
		return fmt.Errorf("Failed to get output of go list -m -json all: %v", err)
	}

	tmpfile, err := ioutil.TempFile("", "go-list")
	if err != nil {
		return fmt.Errorf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.Write(goMod); err != nil {
		return fmt.Errorf("Failed to write go list output to %s: %v", tmpfile.Name(), err)
	}
	if err := tmpfile.Close(); err != nil {
		return fmt.Errorf("Failed to close %s file: %v", tmpfile.Name(), err)
	}

	godepsGenCommand := exec.Command("/godeps-gen", tmpfile.Name(), "Godeps/Godeps.json")
	if err := godepsGenCommand.Run(); err != nil {
		return fmt.Errorf("Failed to run godeps-gen on file %s: %v", tmpfile.Name(), err)
	}
	return nil
}

func checkoutBranchTagCommit(r *gogit.Repository, bh plumbing.Hash, dependentRepos []string) *gogit.Worktree {
	fmt.Printf("Checking that dependencies point to the actual tags in %s.\n", strings.Join(dependentRepos, ", "))
	wt, err := r.Worktree()
	if err != nil {
		glog.Fatalf("Failed to get working tree: %v", err)
	}

	fmt.Printf("Checking out branch tag commit %s.\n", bh.String())
	if err := wt.Checkout(&gogit.CheckoutOptions{Hash: bh}); err != nil {
		glog.Fatalf("Failed to checkout %v: %v", bh, err)
	}
	return wt
}

func updateGoModAndGodeps(tag string, dependentRepos []string, generateGodeps, publishSemverTags bool) bool {
	fmt.Printf("Updating go.mod and go.sum to point to %s tag.\n", tag)
	changed, err := updateGomodWithTaggedDependencies(tag, dependentRepos, publishSemverTags)
	if err != nil {
		glog.Fatalf("Failed to update go.mod and go.sum for tag %s: %v", tag, err)
	}

	if changed && generateGodeps {
		fmt.Printf("Regenerating Godeps.json from go.mod.\n")
		if err := regenerateGodepsFromGoMod(); err != nil {
			glog.Fatalf("Failed to regenerate Godeps.json from go.mod: %v", err)
		}
	}
	return changed
}

func createCommitToFixDeps(wt *gogit.Worktree, tag string) plumbing.Hash {
	fmt.Printf("Adding extra commit to update dependencies to %s tag.\n", tag)
	publishingBotNow := publishingBot
	publishingBotNow.When = time.Now()
	bh, err := wt.Commit(fmt.Sprintf("Update dependencies to %s tag", tag), &gogit.CommitOptions{
		All:       true,
		Author:    &publishingBotNow,
		Committer: &publishingBotNow,
	})
	if err != nil {
		glog.Fatalf("Failed to commit changes to update dependencies to %s tag: %v", tag, err)
	}
	return bh
}

func deleteTag(tag string) error {
	cmd := exec.Command("git", "tag", "-d", tag)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// cleanCacheForTag deletes the .mod, .info, .zip for the tag
// and removes the tag from the list in the go mod cache dir.
func cleanCacheForTag(tag string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("unable to get current working directory: %v", err)
	}
	pkg, err := fullPackageName(dir)
	if err != nil {
		return fmt.Errorf("failed to get package at %s: %v", dir, err)
	}
	cacheDir := fmt.Sprintf("%s/pkg/mod/cache/download/%s/@v", os.Getenv("GOPATH"), pkg)

	goModFile := fmt.Sprintf("%s/%s.mod", cacheDir, tag)
	if _, err := os.Stat(goModFile); err == nil {
		if err2 := os.Remove(goModFile); err2 != nil {
			return fmt.Errorf("error deleting file %s: %v", goModFile, err2)
		}
	}

	infoFile := fmt.Sprintf("%s/%s.info", cacheDir, tag)
	if _, err := os.Stat(infoFile); err == nil {
		if err2 := os.Remove(infoFile); err2 != nil {
			return fmt.Errorf("error deleting file %s: %v", infoFile, err2)
		}
	}

	zipFile := fmt.Sprintf("%s/%s.zip", cacheDir, tag)
	if _, err := os.Stat(zipFile); err == nil {
		if err2 := os.Remove(zipFile); err2 != nil {
			return fmt.Errorf("error deleting file %s: %v", zipFile, err2)
		}
	}

	listFile := fmt.Sprintf("%s/list", cacheDir)
	if _, err := os.Stat(listFile); err == nil {
		oldContent, err2 := ioutil.ReadFile(listFile)
		if err2 != nil {
			return fmt.Errorf("error reading file %s: %v", listFile, err2)
		}

		lines := strings.Split(string(oldContent), "\n")
		newContent := []string{}
		for _, line := range lines {
			if line != tag {
				newContent = append(newContent, line)
			}
		}
		output := strings.Join(newContent, "\n")

		if err := ioutil.WriteFile(listFile, []byte(output), 0644); err != nil {
			return fmt.Errorf("error reading file %s: %v", listFile, err)
		}
	}

	fmt.Printf("Cleared go mod cache files for %s tag.\n", tag)
	return nil
}
