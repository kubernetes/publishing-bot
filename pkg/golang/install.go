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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

// deprecatedDefaultGoVersion hardcodes the (old) default go version.
// The right way to define the default go version today is to specify in rules.
// TODO(nikhita): remove deprecatedDefaultGoVersion when go 1.16 is released.
var deprecatedDefaultGoVersion = "1.14.4"

// InstallGoVersions download and unpacks the specified Golang versions to $GOPATH/
// If the DefaultGoVersion is not specfied in rules, it defaults to 1.14.4.
func InstallGoVersions(rules *config.RepositoryRules) error {
	if rules == nil {
		return nil
	}

	defaultGoVersion := deprecatedDefaultGoVersion
	if rules.DefaultGoVersion != nil {
		defaultGoVersion = *rules.DefaultGoVersion
	}
	glog.Infof("Using %s as the default go version", defaultGoVersion)

	goVersions := []string{defaultGoVersion}
	for _, rule := range rules.Rules {
		for i := range rule.Branches {
			branch := rule.Branches[i]
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

func installGoVersion(v, pth string) error {
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
	tmpPath, err := os.MkdirTemp(os.Getenv("GOPATH"), "go-tmp-")
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

	return os.Rename(tmpPath, pth)
}
