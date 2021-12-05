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
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/golang/glog"
	"golang.org/x/mod/modfile"
	modzip "golang.org/x/mod/zip"
)

func Usage() {
	fmt.Fprintf(os.Stderr, `Creates a zip file at
$GOPATH/pkg/mod/cache/download/<package-name>/@v/<pseudo-version>.zip.
The zip file has the same hash as if it were created by go mod download.
This tool can be used to package modules which haven't been uploaded anywhere
yet and are only available locally.

This tool assumes that the package is already checked out at the commit
pointed by the pseudo-version.

package-name should be equal to the import path of the package.

Usage: %s --package-name <package-name> --pseudo-version <pseudo-version>
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	packageName := flag.String("package-name", "", "package to zip")
	pseudoVersion := flag.String("pseudo-version", "", "pseudoVersion to zip at")

	flag.Usage = Usage
	flag.Parse()

	if *packageName == "" {
		glog.Fatalf("package-name cannot be empty")
	}

	if *pseudoVersion == "" {
		glog.Fatalf("pseudo-version cannot be empty")
	}

	packagePath := fmt.Sprintf("%s/src/%s", os.Getenv("GOPATH"), *packageName)
	cacheDir := fmt.Sprintf("%s/pkg/mod/cache/download/%s/@v", os.Getenv("GOPATH"), *packageName)

	moduleFile, err := getModuleFile(packagePath, *pseudoVersion)
	if err != nil {
		glog.Fatalf("error getting module file: %v", err)
	}

	if err := createZipArchive(packagePath, moduleFile, cacheDir); err != nil {
		glog.Fatalf("error creating zip archive: %v", err)
	}
}

func getModuleFile(packagePath, version string) (*modfile.File, error) {
	goModPath := filepath.Join(packagePath, "go.mod")
	file, err := os.Open(goModPath)
	if err != nil {
		return nil, fmt.Errorf("error opening %s: %v", goModPath, err)
	}
	defer file.Close()

	moduleBytes, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %v", goModPath, err)
	}

	moduleFile, err := modfile.Parse(packagePath, moduleBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("error parsing module file: %v", err)
	}

	if moduleFile.Module == nil {
		return nil, fmt.Errorf("parsed module should not be nil")
	}

	moduleFile.Module.Mod.Version = version
	return moduleFile, nil
}

func createZipArchive(packagePath string, moduleFile *modfile.File, outputDirectory string) error {
	zipFilePath := filepath.Join(outputDirectory, moduleFile.Module.Mod.Version+".zip")
	var zipContents bytes.Buffer

	if err := modzip.CreateFromDir(&zipContents, moduleFile.Module.Mod, packagePath); err != nil {
		return fmt.Errorf("create zip from dir: %w", err)
	}
	if err := ioutil.WriteFile(zipFilePath, zipContents.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing zip file: %w", err)
	}
	return nil
}
