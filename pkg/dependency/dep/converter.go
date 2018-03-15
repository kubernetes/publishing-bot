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

package dep

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/dep"
	"github.com/golang/dep/gps"

	"k8s.io/publishing-bot/pkg/dependency"
	"k8s.io/publishing-bot/pkg/dependency/godep"
	"k8s.io/publishing-bot/pkg/golang"
)

// GodepToGopkg convert Godeps/Godeps.json into Gopkg.toml. All given dependencies are
// constraint by branch or tag if set. Every other package is constraint by revision.
func GodepToGopkg(deps []dependency.Dependency, requiredPackages []string) error {
	bs, err := ioutil.ReadFile("Godeps/Godeps.json")
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	var godeps godep.Godeps
	if err := json.Unmarshal(bs, &godeps); err != nil {
		return fmt.Errorf("failed to unmarshal Godeps.json: %v", err)
	}

	m := dep.NewManifest()
	m.Required = append([]string(nil), requiredPackages...)

	dependencyPackages := map[string]dependency.Dependency{}
	for _, d := range deps {
		depPath := filepath.Join("..", d.Name)
		pkg, err := golang.FullPackageName(depPath)
		if err != nil {
			return fmt.Errorf("failed to get Golang package at %s: %v", depPath, err)
		}
		dependencyPackages[pkg] = d
	}

	// Set up a SourceManager. This manages interaction with sources (repositories).
	tempdir, _ := ioutil.TempDir("", "gps-repocache")
	sm, _ := gps.NewSourceManager(gps.SourceManagerConfig{Cachedir: filepath.Join(tempdir)})
	defer sm.Release()

	foundHashes := map[string]string{}
	for _, gd := range godeps.Deps {
		root, err := sm.DeduceProjectRoot(gd.ImportPath)
		gdRootPkg := string(root)
		if err != nil {
			return fmt.Errorf("failed to deduce root package for %q: %v", gd.ImportPath, err)
		}
		if h, found := foundHashes[gdRootPkg]; found {
			if h != gd.Rev {
				return fmt.Errorf("package %s appears with rev %s and %s in Godeps.json", gdRootPkg, foundHashes[gdRootPkg], gd.Rev)
			}
			continue
		}

		constraint := gps.ProjectProperties{
			Constraint: gps.Revision(gd.Rev),
		}

		if d, found := dependencyPackages[gdRootPkg]; found {
			if len(d.Tag) > 0 {
				constraint.Constraint = gps.NewVersion(d.Tag)
			} else if len(d.Branch) > 0 {
				constraint.Constraint = gps.NewBranch(d.Branch)
			}
		}

		m.Constraints[gps.ProjectRoot(gdRootPkg)] = constraint
	}

	bs, err = m.MarshalTOML()
	if err != nil {
		return fmt.Errorf("failed to marshal Gopkg.toml: %v", err)
	}

	// make sure that there is a trailing newline
	if bs[len(bs)-1] != '\n' {
		bs = append(bs, '\n')
	}

	if err := ioutil.WriteFile("Gopkg.toml", bs, 0644); err != nil {
		return fmt.Errorf("failed to write Gopkg.toml: %v", err)
	}

	return nil
}
