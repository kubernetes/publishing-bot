package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/golang/glog"
	yaml "gopkg.in/yaml.v2"

	"k8s.io/publishing-bot/cmd/publishing-bot/config"
	"k8s.io/publishing-bot/pkg/golang"
)

const (
	depCommit      = "7c44971bbb9f0ed87db40b601f2d9fe4dffb750d"
	godepVersion   = "v80-k8s-r1"
	k8sGodepCommit = "d5f2096f1a37a31b9f450d601411b8a85b64c624"
)

var (
	SystemGoPath = os.Getenv("GOPATH")
	BaseRepoPath = filepath.Join(SystemGoPath, "src", "k8s.io")
)

func Usage() {
	fmt.Fprintf(os.Stderr, `
Usage: %s [-config <config-yaml-file>] [-source-repo <repo>] [-source-org <org>] [-rules-file <file> ] [-skip-godep|skip-dep] [-target-org <org>]

Command line flags override config values.
`, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	configFilePath := flag.String("config", "", "the config file in yaml format")
	githubHost := flag.String("github-host", "", "the address of github (defaults to github.com)")
	basePackage := flag.String("base-package", "", "the name of the package base (defaults to k8s.io when source repo is kubernetes, "+
		"otherwise github-host/target-org)")
	repoName := flag.String("source-repo", "", "the name of the source repository (eg. kubernetes)")
	repoOrg := flag.String("source-org", "", "the name of the source repository organization, (eg. kubernetes)")
	rulesFile := flag.String("rules-file", "", "the file with repository rules")
	targetOrg := flag.String("target-org", "", `the target organization to publish into (e.g. "k8s-publishing-bot")`)
	skipGodep := flag.Bool("skip-godep", false, `skip godeps installation and godeps-restore`)
	skipDep := flag.Bool("skip-dep", false, `skip 'dep'' installation`)

	flag.Usage = Usage
	flag.Parse()

	cfg := config.Config{}
	if *configFilePath != "" {
		bs, err := ioutil.ReadFile(*configFilePath)
		if err != nil {
			glog.Fatalf("Failed to load config file from %q: %v", *configFilePath, err)
		}
		if err := yaml.Unmarshal(bs, &cfg); err != nil {
			glog.Fatalf("Failed to parse config file at %q: %v", *configFilePath, err)
		}
	}

	if *targetOrg != "" {
		cfg.TargetOrg = *targetOrg
	}
	if *repoName != "" {
		cfg.SourceRepo = *repoName
	}
	if *repoOrg != "" {
		cfg.SourceOrg = *repoOrg
	}
	if *githubHost != "" {
		cfg.GithubHost = *githubHost
	}
	if *basePackage != "" {
		cfg.BasePackage = *basePackage
	}

	if cfg.GithubHost == "" {
		cfg.GithubHost = "github.com"
	}
	// defaulting when base package is not specified
	if cfg.BasePackage == "" {
		if cfg.SourceRepo == "kubernetes" {
			cfg.BasePackage = "k8s.io"
		} else {
			cfg.BasePackage = filepath.Join(cfg.GithubHost, cfg.TargetOrg)
		}
	}

	BaseRepoPath = filepath.Join(SystemGoPath, "src", cfg.BasePackage)

	if *rulesFile != "" {
		cfg.RulesFile = *rulesFile
	}

	if len(cfg.SourceRepo) == 0 || len(cfg.SourceOrg) == 0 {
		glog.Fatalf("source-org and source-repo cannot be empty")
	}

	if len(cfg.TargetOrg) == 0 {
		glog.Fatalf("Target organization cannot be empty")
	}

	// If RULE_FILE_PATH is detected, check if the source repository include rules files.
	if len(os.Getenv("RULE_FILE_PATH")) > 0 {
		cfg.RulesFile = filepath.Join(BaseRepoPath, cfg.SourceRepo, os.Getenv("RULE_FILE_PATH"))
	}

	if len(cfg.RulesFile) == 0 {
		glog.Fatalf("No rules file provided")
	}
	rules, err := config.LoadRules(cfg.RulesFile)
	if err != nil {
		glog.Fatalf("Failed to load rules: %v", err)
	}
	if err := config.Validate(rules); err != nil {
		glog.Fatalf("Invalid rules: %v", err)
	}

	if err := os.MkdirAll(BaseRepoPath, os.ModePerm); err != nil {
		glog.Fatalf("Failed to create source repo directory %s: %v", BaseRepoPath, err)
	}

	if err := golang.InstallDefaultGoVersion(); err != nil {
		glog.Fatalf("Failed to install default go version: %v", err)
	}

	if !*skipGodep {
		installGodeps()
	}
	if !*skipDep {
		installDep()
	}

	cloneSourceRepo(cfg, *skipGodep)
	for _, rule := range rules.Rules {
		cloneForkRepo(cfg, rule.DestinationRepository)
	}
}

func cloneForkRepo(cfg config.Config, repoName string) {
	forkRepoLocation := fmt.Sprintf("https://%s/%s/%s", cfg.GithubHost, cfg.TargetOrg, repoName)
	repoDir := filepath.Join(BaseRepoPath, repoName)

	if _, err := os.Stat(repoDir); err == nil {
		glog.Infof("Fork repository %q already cloned to %s, resetting remote URL ...", repoName, repoDir)
		setUrlCmd := exec.Command("git", "remote", "set-url", "origin", forkRepoLocation)
		setUrlCmd.Dir = repoDir
		run(setUrlCmd)
		os.Remove(filepath.Join(repoDir, ".git", "index.lock"))
	} else {
		glog.Infof("Cloning fork repository %s ...", forkRepoLocation)
		run(exec.Command("git", "clone", forkRepoLocation))
	}

	// set user in repo because old git version (compare https://github.com/git/git/commit/92bcbb9b338dd27f0fd4245525093c4bce867f3d) still look up user ids without
	setUsernameCmd := exec.Command("git", "config", "user.name", os.Getenv("GIT_COMMITTER_NAME"))
	setUsernameCmd.Dir = repoDir
	run(setUsernameCmd)
	setEmailCmd := exec.Command("git", "config", "user.email", os.Getenv("GIT_COMMITTER_EMAIL"))
	setEmailCmd.Dir = repoDir
	run(setEmailCmd)
}

// installGodeps installs kubernetes' forked version of godep.
// We need to install the forked version because godep by default
// doesn't support bitbucket anymore, but the forked version does.
// Since the forked godep only exists until 1.14, we first checkout
// to a commit which supports it.
func installGodeps() {
	godepVersionCmd := exec.Command("godep", "version")
	version, err := godepVersionCmd.Output()
	if err == nil && string(version) == godepVersion {
		glog.Infof("Already installed godep %s", godepVersion)
		return
	}

	// clone k8s.io/kubernetes if it doesn't exist already
	if _, err := os.Stat(filepath.Join(SystemGoPath, "src", "k8s.io", "kubernetes")); err != nil {
		if err := os.MkdirAll(BaseRepoPath, os.FileMode(755)); err != nil {
			glog.Fatalf("unable to create %s directory: %v", BaseRepoPath, err)
		}

		repoLocation := "https://github.com/kubernetes/kubernetes.git"
		glog.Infof("Cloning repository %s ...", repoLocation)
		cloneCmd := exec.Command("git", "clone", repoLocation)
		cloneCmd.Dir = BaseRepoPath
		run(cloneCmd)
	}

	k8sCheckOutDir := filepath.Join(SystemGoPath, "src", "k8s.io", "kubernetes")
	k8sGodepCheckoutCmd := exec.Command("git", "checkout", k8sGodepCommit)
	k8sGodepCheckoutCmd.Dir = k8sCheckOutDir
	run(k8sGodepCheckoutCmd)

	glog.Infof("Installing k8s.io/kubernetes/third_party/forked/godep#%s ...", godepVersion)
	godepInstallCmd := exec.Command("go", "install", "k8s.io/kubernetes/third_party/forked/godep")
	run(godepInstallCmd)

	// finally, checkout to master to avoid impacting other processes later
	k8sMasterCheckoutCmd := exec.Command("git", "checkout", "master")
	k8sMasterCheckoutCmd.Dir = k8sCheckOutDir
	run(k8sMasterCheckoutCmd)
}

func installDep() {
	if _, err := exec.LookPath("dep"); err == nil {
		glog.Infof("Already installed: dep")
		return
	}
	glog.Infof("Installing github.com/golang/dep#%s ...", depCommit)
	depGoGetCmd := exec.Command("go", "get", "github.com/golang/dep")
	run(depGoGetCmd)

	depDir := filepath.Join(SystemGoPath, "src", "github.com", "golang", "dep")
	depCheckoutCmd := exec.Command("git", "checkout", depCommit)
	depCheckoutCmd.Dir = depDir
	run(depCheckoutCmd)

	depInstallCmd := exec.Command("go", "install", "./cmd/dep")
	depInstallCmd.Dir = depDir
	run(depInstallCmd)
}

// run wraps the cmd.Run() command and sets the standard output and common environment variables.
// if the c.Dir is not set, the BaseRepoPath will be used as a base directory for the command.
func run(c *exec.Cmd) {
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if len(c.Dir) == 0 {
		c.Dir = BaseRepoPath
	}
	if err := c.Run(); err != nil {
		glog.Fatalf("Command %q failed: %v", strings.Join(c.Args, " "), err)
	}
}

func cloneSourceRepo(cfg config.Config, runGodepRestore bool) {
	if _, err := os.Stat(filepath.Join(BaseRepoPath, cfg.SourceRepo)); err == nil {
		glog.Infof("Source repository %q already cloned, skipping", cfg.SourceRepo)
		return
	}

	repoLocation := fmt.Sprintf("https://%s/%s/%s", cfg.GithubHost, cfg.SourceOrg, cfg.SourceRepo)
	glog.Infof("Cloning source repository %s ...", repoLocation)
	cloneCmd := exec.Command("git", "clone", repoLocation)
	run(cloneCmd)

	if runGodepRestore {
		glog.Infof("Running hack/godep-restore.sh ...")
		restoreCmd := exec.Command("bash", "-x", "hack/godep-restore.sh")
		restoreCmd.Dir = filepath.Join(BaseRepoPath, cfg.SourceRepo)
		run(restoreCmd)
	}
}
