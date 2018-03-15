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

package golang

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FullPackageName return the Golang full package name of dir inside the ${GOPATH}/src.
func FullPackageName(dir string) (string, error) {
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
