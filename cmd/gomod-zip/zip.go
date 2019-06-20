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
	"archive/zip"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/golang/glog"
)

const (
	// MaxZipFile is the maximum size of downloaded zip file
	MaxZipFile = 500 << 20
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
	flag.Parse()

	if *packageName == "" {
		glog.Fatalf("package-name cannot be empty")
	}

	if *pseudoVersion == "" {
		glog.Fatalf("pseudo-version cannot be empty")
	}

	// create a zip file using git archive, and remove it after using it
	depPseudoVersion := fmt.Sprintf("%s@%s", *packageName, *pseudoVersion)
	zipFileName := fmt.Sprintf("%s/src/%s/%s.zip", os.Getenv("GOPATH"), *packageName, *pseudoVersion)
	prefix := fmt.Sprintf("%s/", depPseudoVersion)
	gitArchive := exec.Command("git", "archive", "--format=zip", "--prefix", prefix, "-o", zipFileName, "HEAD")
	gitArchive.Dir = fmt.Sprintf("%s/src/%s", os.Getenv("GOPATH"), *packageName)
	gitArchive.Stdout = os.Stdout
	gitArchive.Stderr = os.Stderr
	if err := gitArchive.Run(); err != nil {
		glog.Fatalf("unable to run git archive for %s at %s: %v", *packageName, *pseudoVersion, err)
	}
	defer os.Remove(zipFileName)

	archive, err := ioutil.ReadFile(zipFileName)
	if err != nil {
		glog.Fatalf("error reading zip file %s: %v", zipFileName, err)
	}

	dl := ioutil.NopCloser(bytes.NewReader(archive))
	defer dl.Close()

	// This is taken from https://github.com/golang/go/blob/b373d31c25e58d0b69cff3521b915f0c06fa6ac8/src/cmd/go/internal/modfetch/coderepo.go#L459.
	// Spool to local file.
	f, err := ioutil.TempFile("", "gomodzip-")
	if err != nil {
		dl.Close()
		glog.Fatalf("error creating temp file: %v", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()

	maxSize := int64(MaxZipFile)
	lr := &io.LimitedReader{R: dl, N: maxSize + 1}
	if _, err := io.Copy(f, lr); err != nil {
		dl.Close()
		glog.Fatalf("error reading from %s: %v", f.Name(), err)
	}

	if lr.N <= 0 {
		glog.Fatalf("downloaded zip file too large")
	}
	size := (maxSize + 1) - lr.N
	if _, err := f.Seek(0, 0); err != nil {
		glog.Fatal(err)
	}

	// The zip file created by go mod download has extra normalization over
	// the zip file created by git archive. The normalization process is done below.
	//
	// While the normalization can also be achieved via a simple zip command, the zip file
	// created by go mod download has the `00-00-1980 00:00` timestamp in the file header
	// for all files in the zip archive. This is not a valid UNIX timestamp and cannot be
	// set easily. This is, however, valid in MSDOS. The `archive/zip` package uses the
	// MSDOS version so we create the zip file using this package.
	zr, err := zip.NewReader(f, size)
	if err != nil {
		glog.Fatalf("error reading %s: %v", f.Name(), err)
	}

	packagedZipPath := fmt.Sprintf("%s/pkg/mod/cache/download/%s/@v/%s.zip", os.Getenv("GOPATH"), *packageName, *pseudoVersion)
	dst, err := os.OpenFile(packagedZipPath, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		glog.Fatalf("failed to create zip file at %s: %v", packagedZipPath, err)
	}
	defer dst.Close()
	zw := zip.NewWriter(dst)

	for _, zf := range zr.File {
		// Skip symlinks (golang.org/issue/27093)
		if !zf.FileInfo().Mode().IsRegular() {
			continue
		}
		// drop directory dummy entries
		if strings.HasSuffix(zf.Name, "/") {
			continue
		}
		// all file paths should have module@version/ prefix
		if !strings.HasPrefix(zf.Name, prefix) {
			continue
		}
		// inserted by hg archive.
		// not correct to drop from other version control systems, but too bad.
		name := strings.TrimPrefix(zf.Name, prefix)
		if name == ".hg_archival.txt" {
			continue
		}
		// don't consider vendored packages
		if isVendoredPackage(name) {
			continue
		}
		// make sure we have lower-case go.mod
		base := path.Base(name)
		if strings.ToLower(base) == "go.mod" && base != "go.mod" {
			glog.Fatalf("zip file contains %s, want all lower-case go.mod", zf.Name)
		}

		size := int64(zf.UncompressedSize64)
		if size < 0 || maxSize < size {
			glog.Fatalf("module source tree too big")
		}
		maxSize -= size

		rc, err := zf.Open()
		if err != nil {
			glog.Fatalf("unable to open file %s: %v", zf.Name, err)
		}
		w, err := zw.Create(zf.Name)
		if err != nil {
			glog.Fatal(err)
		}
		lr := &io.LimitedReader{R: rc, N: size + 1}
		if _, err := io.Copy(w, lr); err != nil {
			glog.Fatal(err)
		}
		if lr.N <= 0 {
			glog.Fatalf("individual file too large")
		}
	}

	if err := zw.Close(); err != nil {
		glog.Fatal(err)
	}
}

func isVendoredPackage(name string) bool {
	var i int
	if strings.HasPrefix(name, "vendor/") {
		i += len("vendor/")
	} else if j := strings.Index(name, "/vendor/"); j >= 0 {
		// This offset looks incorrect; this should probably be
		//
		// 	i = j + len("/vendor/")
		//
		// (See https://golang.org/issue/31562.)
		//
		// Unfortunately, we can't fix it without invalidating checksums.
		// Fortunately, the error appears to be strictly conservative: we'll retain
		// vendored packages that we should have pruned, but we won't prune
		// non-vendored packages that we should have retained.
		//
		// Since this defect doesn't seem to break anything, it's not worth fixing
		// for now.
		i += len("/vendor/")
	} else {
		return false
	}
	return strings.Contains(name[i:], "/")
}
