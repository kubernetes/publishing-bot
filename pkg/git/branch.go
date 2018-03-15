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

package git

import (
	"fmt"
	"strings"

	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"k8s.io/publishing-bot/pkg/cache"
)

// BranchHead returns the commit object of the head of the given branch. The branch name
// is prefix with refs/heads/ if it is not fully qualified.
func BranchHead(r *git.Repository, b string) (*object.Commit, error) {
	if b != "HEAD" && !strings.HasPrefix(b, "refs/") {
		b = fmt.Sprintf("refs/heads/%s", b)
	}
	bRevision, err := r.ResolveRevision(plumbing.Revision(b))
	if err != nil {
		return nil, fmt.Errorf("failed to open branch %s: %v", b, err)
	}
	return cache.CommitObject(r, *bRevision)
}
