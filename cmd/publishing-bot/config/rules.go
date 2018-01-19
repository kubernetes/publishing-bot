package config

import (
	"fmt"
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

// Dependency of a piece of code
type Dependency struct {
	Repository string `yaml:"repository"`
	Branch     string `yaml:"branch"`
	// Dir from repo root
	Dir string `yaml:"dir,omitempty"`
}

func (c Dependency) String() string {
	repo := c.Repository
	if len(repo) == 0 {
		repo = "<source>"
	}
	return fmt.Sprintf("[repository %s, branch %s, subdir %s]", repo, c.Branch, c.Dir)
}

type BranchRule struct {
	Name string `yaml:"name"`
	// k8s.io/* repos the branch rule depends on
	Dependencies []Dependency `yaml:"dependencies,omitempty"`
	Source       Dependency   `yaml:"source"`
}

// a collection of publishing rules for a single destination repo
type RepositoryRule struct {
	DestinationRepository string       `yaml:"destination"`
	Branches              []BranchRule `yaml:"branches"`
	// if empty (e.g., for client-go), publisher will use its default publish script
	PublishScript string `yaml:"publish-script"`
	// not updated when true
	Skip bool `yaml:"skipped,omitempty"`
}

type RepositoryRules struct {
	SkippedSourceBranches []string         `yaml:"skip-source-branches"`
	Rules                 []RepositoryRule `yaml:"rules"`
}

// LoadRules loads the repository rules either from the remote HTTP location or
// a local file path.
func LoadRules(config *Config) (*RepositoryRules, error) {
	var (
		content []byte
		err     error
	)
	content, err = ioutil.ReadFile(config.RulesFile)
	if err != nil {
		return nil, err
	}
	var rules RepositoryRules
	if err = yaml.Unmarshal(content, &rules); err != nil {
		return nil, err
	}
	return &rules, nil
}
