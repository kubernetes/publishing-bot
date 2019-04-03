package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var debug = false

func main() {
	if len(os.Args) < 2 || len(os.Args) > 4 {
		fmt.Fprintln(os.Stderr, "This tool generates a Godeps.json file based on an input file containing dependency information.")
		fmt.Fprintln(os.Stderr, "usage: gen-godeps <input-file> [<output-file>]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "  <input-file> should contain the result of running 'GO111MODULE=on go list -m -json all'")
		fmt.Fprintln(os.Stderr, "  <output-file> is a Godeps.json file. if <output-file> is omitted, content is written to stdout")
		os.Exit(1)
	}

	inputFile := os.Args[1]

	f, err := os.Open(inputFile)
	checkErr(err)
	defer f.Close()

	decoder := json.NewDecoder(f)
	deps := []GoListDep{}
	for {
		dep := &GoListDep{}
		err := decoder.Decode(dep)
		if err == nil {
			deps = append(deps, *dep)
			continue
		}
		if err == io.EOF {
			break
		}
		checkErr(err)
	}

	sort.SliceStable(deps, func(i, j int) bool { return deps[i].Path < deps[j].Path })

	godeps := Godeps{
		GoVersion:    "unknown",
		GodepVersion: "gen-godeps",
		Packages:     []string{"./..."},
	}
	for _, dep := range deps {
		if dep.Main {
			godeps.ImportPath = dep.Path
			continue
		}

		version := dep.Version
		if len(dep.Replace.Path) > 0 {
			switch {
			case dep.Replace.Path == dep.Path:
				// pinned replacement, use the replaced version
				if debug {
					fmt.Fprintf(os.Stderr, "use replaced version for %q\n", dep.Path)
				}
				version = dep.Replace.Version
			case (strings.HasPrefix(dep.Replace.Path, "./") || strings.HasPrefix(dep.Replace.Path, "../")) && len(dep.Replace.Version) == 0:
				// relative path, use the required version
				if debug {
					fmt.Fprintf(os.Stderr, "use required version for relative %q\n", dep.Path)
				}
			default:
				// replacement path != source path, we can't generate a usable godeps.json
				checkErr(fmt.Errorf("dependency %q was replaced with %q, cannot generate godeps", dep.Path, dep.Replace.Path))
			}
		} else {
			if debug {
				fmt.Fprintf(os.Stderr, "use required version for %q\n", dep.Path)
			}
		}
		rev, err := versionToRev(dep.Path, version)
		checkErr(err)
		godeps.Deps = append(godeps.Deps, GodepDep{ImportPath: dep.Path, Rev: rev})
	}

	godepJSON, err := json.MarshalIndent(godeps, "", "\t")
	checkErr(err)

	if len(os.Args) > 2 {
		outputFile := os.Args[2]
		checkErr(os.MkdirAll(filepath.Dir(outputFile), os.FileMode(755)))
		checkErr(ioutil.WriteFile(outputFile, godepJSON, os.FileMode(0644)))
	} else {
		fmt.Println(string(godepJSON))
	}
}

var (
	// https://tip.golang.org/cmd/go/#hdr-Pseudo_versions
	pseudoVersion = regexp.MustCompile(`(-0\.|\.0\.|-)\d{14}-([0-9a-f]{12})(\+incompatible)?$`)
)

func versionToRev(path, version string) (string, error) {
	switch {
	// pseudo version (v0.0.0-20180207000608-0eeff89b0690)
	case pseudoVersion.FindStringSubmatch(version) != nil:
		sha := pseudoVersion.FindStringSubmatch(version)[2]
		if sha == "000000000000" {
			return "", fmt.Errorf("unknown version sha: %q: %q", path, version)
		}
		return sha, nil

	default:
		if version == "v0.0.0" {
			return "", fmt.Errorf("unknown version tag: %q: %q", path, version)
		}
		// https://tip.golang.org/cmd/go/#hdr-Module_compatibility_and_semantic_versioning
		return strings.TrimSuffix(version, "+incompatible"), nil
	}
}

func checkErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type GoListDep struct {
	Path    string
	Version string
	Main    bool
	Replace GoListReplace
}
type GoListReplace struct {
	Path    string
	Version string
}

type Godeps struct {
	ImportPath   string
	GoVersion    string
	GodepVersion string
	Packages     []string
	Deps         []GodepDep
}

type GodepDep struct {
	ImportPath string
	Rev        string
}
