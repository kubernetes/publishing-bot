package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
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
	Repository string `yaml:"repository"`
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
	GoVersion string `yaml:"go"`
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
	SkippedSourceBranches []string         `yaml:"skip-source-branches"`
	SkipGomod             bool             `yaml:"skip-gomod"`
	SkipTags              bool             `yaml:"skip-tags"`
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

	if ruleUrl, err := url.ParseRequestURI(ruleFile); err == nil && len(ruleUrl.Host) > 0 {
		glog.Infof("loading rules file from url : %s", ruleUrl)
		content, err = readFromUrl(ruleUrl)
		if err != nil {
			return nil, err
		}
	} else {
		glog.Infof("loading rules file : %s", ruleFile)
		content, err = ioutil.ReadFile(ruleFile)
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

// readFromUrl reads the rule file from provided URL.
func readFromUrl(u *url.URL) ([]byte, error) {
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
	return ioutil.ReadAll(resp.Body)
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
var goVerRegex = regexp.MustCompile(`^([0-9]+)\.([0-9]\w*)(\.[1-9]\d*\w*)*$`)

func ensureValidGoVersion(version string) error {
	if !goVerRegex.MatchString(version) {
		return fmt.Errorf("specified go version %s is invalid", version)
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
