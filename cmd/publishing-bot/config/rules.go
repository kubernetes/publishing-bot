package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
	yaml "gopkg.in/yaml.v2"
)

// Dependency of a piece of code
type Dependency struct {
	Repository string `yaml:"repository"`
	Branch     string `yaml:"branch"`
}

func (c Dependency) String() string {
	repo := c.Repository
	if repo == "" {
		repo = "<source>"
	}
	return fmt.Sprintf("[repository %s, branch %s]", repo, c.Branch)
}

// Source of a piece of code
type Source struct {
	Repository string `yaml:"repository,omitempty"`
	Branch     string `yaml:"branch"`
	// Dir from repo root
	Dir string `yaml:"dir,omitempty"`
}

func (c Source) String() string {
	repo := c.Repository
	if repo == "" {
		repo = "<source>"
	}
	return fmt.Sprintf("[repository %s, branch %s, subdir %s]", repo, c.Branch, c.Dir)
}

type BranchRule struct {
	Name string `yaml:"name"`
	// a valid go version string like 1.10.2 or 1.10
	//
	// From go 1.21 onwards there is a change in the versioning format.
	// The version displayed by `go version` should be used here:
	// 1. 1.21.0 is valid and 1.21 is invalid
	// 2. 1.21rc1 and 1.21.0rc1 are valid
	GoVersion string `yaml:"go,omitempty"`
	// k8s.io/* repos the branch rule depends on
	Dependencies     []Dependency `yaml:"dependencies,omitempty"`
	Source           Source       `yaml:"source"`
	RequiredPackages []string     `yaml:"required-packages,omitempty"`
	// SmokeTest applies only to the specific branch
	SmokeTest string `yaml:"smoke-test,omitempty"` // a multiline bash script
}

// a collection of publishing rules for a single destination repo
type RepositoryRule struct {
	DestinationRepository string       `yaml:"destination"`
	Branches              []BranchRule `yaml:"branches"`
	// SmokeTest applies to all branches
	SmokeTest string `yaml:"smoke-test,omitempty"` // a multiline bash script
	Library   bool   `yaml:"library,omitempty"`
	// not updated when true
	Skip bool `yaml:"skipped,omitempty"`
}

type RepositoryRules struct {
	SkippedSourceBranches []string         `yaml:"skip-source-branches,omitempty"`
	SkipGomod             bool             `yaml:"skip-gomod,omitempty"`
	SkipTags              bool             `yaml:"skip-tags,omitempty"`
	Rules                 []RepositoryRule `yaml:"rules"`

	// ls-files patterns like: */BUILD *.ext pkg/foo.go Makefile
	RecursiveDeletePatterns []string `yaml:"recursive-delete-patterns"`
	// a valid go version string like 1.10.2 or 1.10
	// if GoVersion is not specified in RepositoryRule,
	// DefaultGoVersion is used.
	DefaultGoVersion *string `yaml:"default-go-version,omitempty"`
}

// LoadRules loads the repository rules either from the remote HTTP location or
// a local file path.
func LoadRules(ruleFile string) (*RepositoryRules, error) {
	var content []byte

	if ruleURL, err := url.ParseRequestURI(ruleFile); err == nil && len(ruleURL.Host) > 0 {
		glog.Infof("loading rules file from url : %s", ruleURL)
		content, err = readFromURL(ruleURL)
		if err != nil {
			return nil, err
		}
	} else {
		glog.Infof("loading rules file : %s", ruleFile)
		content, err = os.ReadFile(ruleFile)
		if err != nil {
			return nil, err
		}
	}

	var rules RepositoryRules
	if err := yaml.Unmarshal(content, &rules); err != nil {
		return nil, err
	}

	return &rules, nil
}

// readFromURL reads the rule file from provided URL.
func readFromURL(u *url.URL) ([]byte, error) {
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, err
	}
	// timeout the request after 30 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func validateRepoOrder(rules *RepositoryRules) (errs []error) {
	glog.Infof("validating repository order")
	indices := map[string]int{}
	for i, r := range rules.Rules {
		indices[r.DestinationRepository] = i
	}

	for i, r := range rules.Rules {
		for _, br := range r.Branches {
			for _, d := range br.Dependencies {
				if j, ok := indices[d.Repository]; !ok {
					errs = append(errs, fmt.Errorf("unknown dependency %q in repository rules of %q", d.Repository, r.DestinationRepository))
				} else if j > i {
					errs = append(errs, fmt.Errorf("repository %q cannot depend on %q later in the rules file. Please define rules for %q above the rules for %q", r.DestinationRepository, d.Repository, d.Repository, r.DestinationRepository))
				}
			}
		}
	}
	return errs
}

// validateGoVersions validates that all specified go versions are valid.
func validateGoVersions(rules *RepositoryRules) (errs []error) {
	glog.Infof("validating go versions")
	if rules.DefaultGoVersion != nil {
		errs = append(errs, ensureValidGoVersion(*rules.DefaultGoVersion))
	}

	for _, rule := range rules.Rules {
		for _, branch := range rule.Branches {
			if branch.GoVersion != "" {
				errs = append(errs, ensureValidGoVersion(branch.GoVersion))
			}
		}
	}
	return errs
}

// goVerRegex is the regex for a valid go version.
// go versions don't follow semver. Examples:
// 1. 1.15.0 is invalid, 1.15 is valid
// 2. 1.15.0-rc.1 is invalid, 1.15rc1 is valid
//
// From go 1.21 onwards there is a change in the versioning format
// Ref: https://tip.golang.org/doc/toolchain#versions
//
// The version displayed by `go version` is what we care about and use in the config.
// This is the version in the *name of the go tool chain* (of the form goV, V is what we
// care about). For Go *language versions* >= 1.21, the following are the rules for versions
// in the go tool chain name:
// 1. 1.21 is invalid, and 1.21.0 is valid
// 2. 1.21rc1 and 1.21.0rc1 are valid
var goVerRegex = regexp.MustCompile(`^(?P<major>\d+)\.(?P<minor>\d+)(?:\.(?P<patch>\d+))?(?:(?P<pre>alpha|beta|rc)\d+)?$`)

func ensureValidGoVersion(version string) error {
	match := goVerRegex.FindStringSubmatch(version)
	if len(match) == 0 {
		return fmt.Errorf("specified go version %s is invalid", version)
	}

	var majorVersion, minorVersion, patchVersion int
	var preRelease string
	patchVersionExists := false

	majorVersion, err := strconv.Atoi(match[1])
	if err != nil {
		return fmt.Errorf("error parsing major version '%s' : %s", match[1], err)
	}
	minorVersion, err = strconv.Atoi(match[2])
	if err != nil {
		return fmt.Errorf("error parsing minor version '%s' : %s", match[2], err)
	}
	if match[3] != "" {
		patchVersion, err = strconv.Atoi(match[3])
		if err != nil {
			return fmt.Errorf("error parsing patch version '%s' : %s", match[3], err)
		}
		patchVersionExists = true
	}
	preRelease = match[4]

	// for go versions <= 1.20, patch version .0 should not exist
	if majorVersion <= 1 && minorVersion <= 20 {
		if patchVersionExists && patchVersion == 0 {
			languageVersion := fmt.Sprintf("%d.%d", majorVersion, minorVersion)
			return fmt.Errorf("go language version %s below 1.21; should not have a 0th patch release, got %s", languageVersion, version)
		}
	}

	// for go versions >= 1.21.0, patch versions should exist. If there is no patch version,
	// then it should be a prerelease
	if (majorVersion == 1 && minorVersion >= 21) || majorVersion >= 2 {
		if !patchVersionExists && preRelease == "" {
			return fmt.Errorf("patch version should always be present for go language version >= 1.21")
		}
	}

	return nil
}

func Validate(rules *RepositoryRules) error {
	errs := []error{}

	errs = append(errs, validateRepoOrder(rules)...)
	errs = append(errs, validateGoVersions(rules)...)

	msgs := []string{}
	for _, err := range errs {
		if err != nil {
			msgs = append(msgs, err.Error())
		}
	}
	if len(msgs) > 0 {
		return fmt.Errorf("validation errors:\n- %s", strings.Join(msgs, "\n- "))
	}
	return nil
}
