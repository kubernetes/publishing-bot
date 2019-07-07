/*
Copyright 2019 The Kubernetes Authors.

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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	gogit "gopkg.in/src-d/go-git.v4"
)

// generateVersionedImportsWithTaggedDependencies updates import paths of all
// dependencies to versioned import paths using the majorVersion.
// It also updates go.mod and go.sum to point these dependencies to the given tag.
func generateVersionedImportsWithTaggedDependencies(majorVersion, tag string, depsRepo []string) error {
	updated := map[string]bool{}

	for _, dep := range depsRepo {
		depPath := filepath.Join("..", dep)
		dr, err := gogit.PlainOpen(depPath)
		if err != nil {
			return fmt.Errorf("failed to open dependency repo at %q: %v", depPath, err)
		}

		depPkg, err := fullPackageName(depPath)
		if err != nil {
			return fmt.Errorf("failed to get package at %s: %v", depPath, err)
		}

		commit, commitTime, err := taggedCommitHashAndTime(dr, tag)
		if err != nil {
			return fmt.Errorf("failed to get tag %s for %q: %v", tag, depPkg, err)
		}
		rev := commit.String()

		// if the dependency was already published at this tag, go mod download will help
		// in avoiding packaging it up again
		downloadCommand := exec.Command("go", "mod", "download")
		downloadCommand.Env = append(os.Environ(), "GO111MODULE=on", "GOPOXY=file://${GOPATH}/pkg/mod/cache/download")
		downloadCommand.Stdout = os.Stdout
		downloadCommand.Stderr = os.Stderr
		if err := downloadCommand.Run(); err != nil {
			return fmt.Errorf("error running go mod download for %s: %v", depPkg, err)
		}

		// check if we have the tag published already. if we don't, package it up
		// and save to local mod download cache.
		if err := packageDepToGoModCache(depPath, depPkg, rev, tag, majorVersion, commitTime); err != nil {
			return fmt.Errorf("failed to package %s dependency: %v", depPkg, err)
		}

		// update import paths for the dependency
		depUpgradeCmd := exec.Command("mod", "upgrade", "-t", majorVersion, "-mod-name", depPkg)
		depUpgradeCmd.Env = append(os.Environ(), "GO111MODULE=on", "GOPOXY=file://${GOPATH}/pkg/mod/cache/download")
		depUpgradeCmd.Stdout = os.Stdout
		depUpgradeCmd.Stderr = os.Stderr
		if err := depUpgradeCmd.Run(); err != nil {
			return fmt.Errorf("unable to upgrade %s to v%s: %v", dep, majorVersion, err)
		}
		fmt.Printf("Updated import paths for %s to major version v%s.\n", dep, majorVersion)

		// update go.mod and go.sum to point the dependency at the tag
		if err := updateGoModAndGoSum(depPkg, tag, majorVersion); err != nil {
			return err
		}

		updated[dep] = true
	}

	for _, dep := range depsRepo {
		if !updated[dep] {
			fmt.Printf("Warning: dependency %s was not updated.\n", dep)
		}
	}
	return nil
}
