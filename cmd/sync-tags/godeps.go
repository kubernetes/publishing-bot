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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"strings"

	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

type GodepDependency struct {
	ImportPath string
	Comment    string `json:",omitempty"`
	Rev        string
}

type Godeps struct {
	ImportPath   string
	GoVersion    string
	GodepVersion string
	Packages     []string `json:",omitempty"`
	Deps         []GodepDependency
}

// updateGodepsJsonWithTaggedDependencies gets the dependencies at the given tag and fills Godeps.json. If anything
// is changed, it commit the changes. Returns true if Godeps.json changed.
func updateGodepsJsonWithTaggedDependencies(r *gogit.Repository, tag string, depsRepo []string) (bool, error) {
	bs, err := ioutil.ReadFile("Godeps/Godeps.json")
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}

	var godeps Godeps
	if err := json.Unmarshal(bs, &godeps); err != nil {
		return false, fmt.Errorf("failed to unmarshal Godeps.json: %v", err)
	}

	depRevisions := map[string]plumbing.Hash{}
	for _, dep := range depsRepo {
		depPath := filepath.Join("..", dep)
		dr, err := gogit.PlainOpen(depPath)
		if err != nil {
			return false, fmt.Errorf("failed to open dependency repo at %q: %v", depPath, err)
		}

		pkg, err := fullPackageName(depPath)
		if err != nil {
			return false, fmt.Errorf("failed to get Golang package at %s: %v", dep, err)
		}

		commit, err := localOrPublishedTaggedCommitHash(dr, tag)
		if err != nil {
			return false, fmt.Errorf("failed to get tag %s for %q: %v", tag, pkg, err)
		}

		depRevisions[pkg] = commit
	}

	found := map[string]bool{}
	changed := false
	for i := range godeps.Deps {
		for depPkg, depHash := range depRevisions {
			godep := &godeps.Deps[i]
			if godep.ImportPath != depPkg && !strings.HasPrefix(godep.ImportPath, depPkg+"/") {
				continue
			}

			if godep.Rev != depHash.String() {
				if !found[depPkg] {
					fmt.Printf("Bumping %s in Godeps.json from %q to %s: %q.\n", depPkg, godep.Rev, tag, depHash.String())
					changed = true
				}
				godep.Rev = depHash.String()
			}

			found[depPkg] = true

			break
		}
	}

	for dep := range depRevisions {
		if !found[dep] {
			fmt.Printf("Warning: dependency %s not found in Godeps.json.\n", dep)
		}
	}

	if !changed {
		return false, nil
	}

	bs, err = json.MarshalIndent(&godeps, "", "\t")
	if err != nil {
		return false, err
	}

	// make sure that there is a trailing newline
	if bs[len(bs)-1] != '\n' {
		bs = append(bs, '\n')
	}

	if err := ioutil.WriteFile("Godeps/Godeps.json", bs, 0644); err != nil {
		return false, fmt.Errorf("failed to write Godeps.json: %v", err)
	}

	return true, nil
}

// localOrPublishedTaggedCommitHash return the hash of the commit references by <tag> or origin/<tag>.
func localOrPublishedTaggedCommitHash(r *gogit.Repository, tag string) (plumbing.Hash, error) {
	if commit, err := taggedCommitHash(r, tag); err == nil {
		return commit, nil
	}
	return taggedCommitHash(r, "origin/"+tag)
}

// taggedCommitHash returns the hash of the commit, independently whether refs/tags/<tag> is annotated or not.
func taggedCommitHash(r *gogit.Repository, tag string) (plumbing.Hash, error) {
	ref, err := r.Reference(plumbing.ReferenceName(fmt.Sprintf("refs/tags/%s", tag)), true)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("failed to get refs/tags/%s: %v", tag, err)
	}

	tagObject, err := r.TagObject(ref.Hash())
	if err != nil {
		c, err := r.CommitObject(ref.Hash())
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("refs/tags/%s is invalid: %v", tag, err)
		}
		return c.Hash, nil
	}
	return tagObject.Target, nil
}

// fullPackageName return the Golang full package name of dir inside the ${GOPATH}/src.
func fullPackageName(dir string) (string, error) {
	gopath := os.Getenv("GOPATH")
	if len(gopath) == 0 {
		return "", fmt.Errorf("GOPATH is not set")
	}

	absGopath, err := filepath.Abs(gopath)
	if err != nil {
		return "", fmt.Errorf("failed to make GOPATH %q absolute: %v", gopath, err)
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("failed to make %q absolute: %v", dir, err)
	}

	if !strings.HasPrefix(filepath.ToSlash(absDir), filepath.ToSlash(absGopath)+"/src/") {
		return "", fmt.Errorf("path %q is no inside GOPATH %q", dir, gopath)
	}

	return absDir[len(filepath.ToSlash(absGopath)+"/src/"):], nil
}
