package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blang/semver"
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
	if len(repo) == 0 {
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
	if len(repo) == 0 {
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
}

// a collection of publishing rules for a single destination repo
type RepositoryRule struct {
	DestinationRepository string       `yaml:"destination"`
	Branches              []BranchRule `yaml:"branches"`
	SmokeTest             string       `yaml:"smoke-test,omitempty"` // a multiline bash script
	Library               bool         `yaml:"library,omitempty"`
	// not updated when true
	Skip bool `yaml:"skipped,omitempty"`
}

type RepositoryRules struct {
	SkippedSourceBranches []string         `yaml:"skip-source-branches"`
	SkipGodeps            bool             `yaml:"skip-godeps"`
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
	var (
		content []byte
		err     error
	)
	if ruleUrl, err := url.ParseRequestURI(ruleFile); err == nil && len(ruleUrl.Host) > 0 {
		glog.Infof("loading rules file from url : %s", ruleUrl)
		content, err = readFromUrl(ruleUrl)
	} else {
		glog.Infof("loading rules file : %s", ruleFile)
		content, err = ioutil.ReadFile(ruleFile)
		if err != nil {
			return nil, err
		}

	}

	var rules RepositoryRules
	if err = yaml.Unmarshal(content, &rules); err != nil {
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

func ensureValidGoVersion(version string) error {
	s := version
	parts := strings.SplitN(s, ".", 3)

	// go uses 1.14 instead of 1.14.0 for its versions
	if len(parts) == 3 && version[len(version)-2:] == ".0" {
		return fmt.Errorf("go version %s should not contain the .0 suffix", version)
	}

	// the semver library requires a patch version, so append a .0
	// to be able to validate the major/minor versions
	if len(parts) == 2 {
		s = s + ".0"
	}
	if _, err := semver.Parse(s); err != nil {
		return fmt.Errorf("specified go version %s must be a valid go version: %v", version, err)
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
