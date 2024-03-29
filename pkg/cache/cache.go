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

package cache

import (
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

var globalCommitCache = map[plumbing.Hash]*object.Commit{}

func CommitObject(r *gogit.Repository, hash plumbing.Hash) (*object.Commit, error) {
	if c, found := globalCommitCache[hash]; found {
		if c == nil {
			return nil, plumbing.ErrObjectNotFound
		}
		return c, nil
	}

	c, err := r.CommitObject(hash)
	globalCommitCache[hash] = c
	return c, err
}
