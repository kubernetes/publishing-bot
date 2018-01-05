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

package git

import (
	"fmt"
	"strings"

	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

// SourceHash extracts kube commit from commit message
// The baseRepoName default to "kubernetes".
// TODO: Refactor so we take the commitMsgTag as argument and don't need to
// construct the ancientSyncCommitSubjectPrefix or sourceCommitPrefix
func SourceHash(c *object.Commit, baseRepoOrg, baseRepoName string) plumbing.Hash {
	lines := strings.Split(c.Message, "\n")
	sourceCommitPrefix := strings.Title(baseRepoName) + "-commit: "
	ancientSyncCommitSubjectPrefix := fmt.Sprintf("sync(%s/%s)", baseRepoOrg, baseRepoName)
	for _, line := range lines {
		if strings.HasPrefix(line, sourceCommitPrefix) {
			return plumbing.NewHash(strings.TrimSpace(line[len(sourceCommitPrefix):]))
		}
	}

	if strings.HasPrefix(lines[0], ancientSyncCommitSubjectPrefix) {
		return plumbing.NewHash(strings.TrimSpace(lines[0][len(ancientSyncCommitSubjectPrefix):]))
	}

	return plumbing.ZeroHash
}
