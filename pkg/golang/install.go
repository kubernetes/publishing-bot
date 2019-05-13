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

package golang

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

const defaultGoVersion = "1.12.5"

// installGoVersions download and unpacks the specified Golang versions to $GOPATH/
func InstallDefaultGoVersion() error {
	var empty config.RepositoryRules
	return InstallGoVersions(&empty)
}

// installGoVersions download and unpacks the specified Golang versions to $GOPATH/
func InstallGoVersions(rules *config.RepositoryRules) error {
	goVersions := []string{defaultGoVersion}
	for _, rule := range rules.Rules {
		for _, branch := range rule.Branches {
			if branch.GoVersion != "" {
				found := false
				for _, v := range goVersions {
					if v == branch.GoVersion {
						found = true
					}
				}
				if !found {
					goVersions = append(goVersions, branch.GoVersion)
				}
			}
		}
	}
	systemGoPath := os.Getenv("GOPATH")
	for _, v := range goVersions {
		if err := installGoVersion(v, filepath.Join(systemGoPath, "go-"+v)); err != nil {
			return err
		}
	}
	goLink, target := filepath.Join(systemGoPath, "go"), filepath.Join(systemGoPath, "go-"+defaultGoVersion)
	os.Remove(goLink)
	if err := os.Symlink(target, goLink); err != nil {
		return fmt.Errorf("failed to link %s to %s: %s", goLink, target, err)
	}

	return nil
}

func installGoVersion(v string, pth string) error {
	if s, err := os.Stat(pth); err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		if s.IsDir() {
			glog.Infof("Found existing go %s at %s", v, pth)
			return nil
		}
		return fmt.Errorf("expected %s to be a directory", pth)
	}

	glog.Infof("Installing go %s to %s", v, pth)
	tmpPath, err := ioutil.TempDir(os.Getenv("GOPATH"), "go-tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpPath)

	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf("curl -SLf https://storage.googleapis.com/golang/go%s.linux-amd64.tar.gz | tar -xz --strip 1 -C %s", v, tmpPath))
	cmd.Dir = tmpPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %v", strings.Join(cmd.Args, " "), err)
	}
	if err := os.Rename(tmpPath, pth); err != nil {
		return err
	}

	return nil
}
