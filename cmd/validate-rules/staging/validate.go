/*
Copyright 2021 The Kubernetes Authors.

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

package staging

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

// globalMapBranchDirectories is a cache to avoid hitting GH limits
// key is the branch (`master` or `release-1.23`) and the value
// is the list of files/directories fetched using GH api in the
// correct directory.
var globalMapBranchDirectories = make(map[string][]File)

const defaultBranch = "master"

// EnsureStagingDirectoriesExist walks through the repository rules and checks
// if the specified directories are present in the specific kubernetes branch.
func EnsureStagingDirectoriesExist(rules *config.RepositoryRules, baseBranch, rulesFile string) []error {
	glog.Infof("Validating directories exist in the Kubernetes branches")

	if baseBranch == "" {
		baseBranch = defaultBranch
	}
	glog.Infof("Testing only rules matching the base branch: %s", baseBranch)
	kubeRoot := filepath.Clean(filepath.Join(filepath.Dir(rulesFile), "..", ".."))

	var errors []error
	for _, rule := range rules.Rules {
		for i := range rule.Branches {
			branchRule := rule.Branches[i]
			// ensure all the mentioned directories exist
			for _, dir := range branchRule.Source.Dirs {
				_, directory := filepath.Split(dir)
				if baseBranch != branchRule.Source.Branch {
					glog.Infof("Skipping branch %q for repository %q", branchRule.Source.Branch, directory)
					continue
				}
				err := checkDirectoryExists(directory, branchRule.Source.Branch, baseBranch, kubeRoot)
				if err != nil {
					errors = append(errors, err)
				}
			}

			for _, dependency := range branchRule.Dependencies {
				if baseBranch != dependency.Branch {
					glog.Infof("Skipping branch %q for dependency %q", dependency.Branch, dependency.Repository)
					continue
				}
				err := checkDirectoryExists(dependency.Repository, dependency.Branch, baseBranch, kubeRoot)
				if err != nil {
					errors = append(errors, err)
				}
			}
		}
	}
	return errors
}

func checkDirectoryExists(directory, branch, baseBranch, kubeRoot string) error {
	if branch == baseBranch {
		localPath := filepath.Join(kubeRoot, "staging", "src", "k8s.io", directory)
		if info, err := os.Stat(localPath); err == nil && info.IsDir() {
			return nil
		}
	}
	return checkDirectoryExistsInBranch(directory, branch)
}

func checkDirectoryExistsInBranch(directory, branch string) error {
	glog.Infof("Check if directory %q exists in branch %q", directory, branch)

	// Look in the cache first
	files, ok := globalMapBranchDirectories[branch]
	if !ok {
		var err error
		files, err = fetchKubernetesStagingDirectoryFiles(branch)
		if err != nil {
			globalMapBranchDirectories[branch] = []File{}
			return fmt.Errorf("error fetching directories from branch %s: %w", branch, err)
		}
		globalMapBranchDirectories[branch] = files
	}

	for _, file := range files {
		// check the name and that it is a directory!
		if file.Name == directory && file.Type == "dir" {
			return nil
		}
	}
	return fmt.Errorf("%s not found in branch %s", directory, branch)
}
