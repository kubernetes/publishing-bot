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
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/publishing-bot/cmd/publishing-bot/config"
)

// InstallGoVersions download and unpacks the specified Golang versions to $GOPATH/
// If the DefaultGoVersion is not specfied in rules, it defaults to the current Go release.
func InstallGoVersions(rules *config.RepositoryRules) error {
	if rules == nil {
		return nil
	}
	// Respect the default go version if set, otherwise attempt to use current stable go
	// with GOTOOLCHAIN we will fetch dynamically the branch / module specific version anyhow.
	//
	// Any version > 1.21 that supports GOTOOLCHAIN can automatically
	// fetch the correct go version for a given module if not otherwise overridden.
	defaultGoVersion := ""
	if rules.DefaultGoVersion != nil {
		defaultGoVersion = *rules.DefaultGoVersion
	} else {
		// NOTE: we only do this in the else block, so if the rules explicitly
		// specify a default, they do not depend on this endpoint
		// That means if we ever have issues with getCurrentGoRelease, a quick
		// fix is just setting the default again.
		v, err := getCurrentGoRelease()
		if err != nil {
			return err
		}
		defaultGoVersion = v
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
		return fmt.Errorf("failed to link %s to %s: %w", goLink, target, err)
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

	cmd := exec.Command("/bin/bash", "-c", fmt.Sprintf("curl -SLf https://dl.google.com/go/go%s.linux-amd64.tar.gz | tar -xz --strip 1 -C %s", v, tmpPath))
	cmd.Dir = tmpPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", strings.Join(cmd.Args, " "), err)
	}

	return os.Rename(tmpPath, pth)
}

func getCurrentGoRelease() (string, error) {
	var resp *http.Response
	var err error
	for i := 0; i < 3; i++ {
		resp, err = http.Get("https://go.dev/VERSION?m=text")
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(time.Second)
	}
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	// The response is usually multiple lines, with the first line being "goX.Y.Z"
	// go1.26.0
	// time 2026-02-10T01:22:00Z
	lines := strings.Split(string(body), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("empty response from go.dev")
	}
	return strings.TrimPrefix(lines[0], "go"), nil
}
