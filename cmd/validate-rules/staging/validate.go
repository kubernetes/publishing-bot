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
	"path/filepath"

	"github.com/golang/glog"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

// globalMapBranchDirectories is a cache to avoid hitting GH limits
// key is the branch (`master` or `release-1.23`) and the value
// is the list of files/directories fetched using GH api in the
// correct directory
var globalMapBranchDirectories = make(map[string][]File)

// EnsureStagingDirectoriesExist walks through the repository rules and checks
// if the specified directories are present in the specific kubernetes branch
func EnsureStagingDirectoriesExist(rules *config.RepositoryRules) []error {
	glog.Infof("validating directories exist in the kubernetes branch")

	var errors []error
	for _, rule := range rules.Rules {
		for i := range rule.Branches {
			branchRule := rule.Branches[i]
			// ensure all the mentioned directories exist
			for _, dir := range branchRule.Source.Dirs {
				_, directory := filepath.Split(dir)
				err := checkDirectoryExistsInBranch(directory, branchRule.Source.Branch)
				if err != nil {
					errors = append(errors, err)
				}
			}

			for _, dependency := range branchRule.Dependencies {
				err := checkDirectoryExistsInBranch(dependency.Repository, dependency.Branch)
				if err != nil {
					errors = append(errors, err)
				}
			}
		}
	}
	return errors
}

func checkDirectoryExistsInBranch(directory, branch string) error {
	// Look in the cache first
	files, ok := globalMapBranchDirectories[branch]
	if !ok {
		var err error
		files, err = fetchKubernetesStagingDirectoryFiles(branch)
		if err != nil {
			globalMapBranchDirectories[branch] = []File{}
			return fmt.Errorf("error fetching directories from branch %s : %w", branch, err)
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
