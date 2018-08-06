package config

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

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
	// a (full) version string like 1.10.2.
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
	Rules                 []RepositoryRule `yaml:"rules"`

	// ls-files patterns like: */BUILD *.ext pkg/foo.go Makefile
	RecursiveDeletePatterns []string `yaml:"recursive-delete-patterns"`
}

// LoadRules loads the repository rules either from the remote HTTP location or
// a local file path.
func LoadRules(ruleFile string) (*RepositoryRules, error) {
	var (
		content []byte
		err     error
	)
	if ruleUrl, err := url.ParseRequestURI(ruleFile); err == nil && len(ruleUrl.Host) > 0 {
		content, err = readFromUrl(ruleUrl)
	} else {
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
