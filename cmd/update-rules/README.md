Update branch rules
===================

Command line tooling to manage following operations for branch rules of existing destination repo:
 - add a new branch rule
    - with go version
    - without go version (sets blank "" go version)
 - update an existing branch rule with given go version

For new branch rule, it refers the 'master' branch rule for each destination repository and appends the new
branch rule to configured branches.

For existing branch rule, it updates the given go version for each destination repository.

### Build:

To build the `update-rules` CLI binary, run:

##### Linux:

```
make build
```

##### macOS:

```
GOOS=darwin make build
```

The generated binary will be located at `_output/update-rules`.

##### Container Image:

To build the container image, run:

```
make build-image
```

`update-rules` binary will be available at the root `/` in the image and can be invoked as:

```
docker run -t gcr.io/k8s-staging-publishing-bot/publishing-bot:latest /update-rules
```

### Usage:

Run the command line as:
```
  update-rules -h

  Usage: update-rules --branch BRANCH --rules PATHorURL [--go VERSION | -o PATH]

  Examples:
  # Update rules for branch release-1.23 with go version 1.16.4
  update-rules -branch release-1.23 -go 1.16.4 -rules /go/src/k8s.io/kubernetes/staging/publishing/rules.yaml

  # Update rules using URL to input rules file
  update-rules -branch release-1.23 -go 1.16.4 -rules https://raw.githubusercontent.com/kubernetes/kubernetes/master/staging/publishing/rules.yaml

  # Update rules and export to /tmp/rules.yaml
  update-rules -branch release-1.24 -go 1.17.1 -o /tmp/rules.yaml -rules /go/src/k8s.io/kubernetes/staging/publishing/rules.yaml

  -alsologtostderr
    	log to standard error as well as files
  -branch string
    	[required] Branch to update rules for, e.g. --branch release-x.yy
  -go string
    	Golang version to pin for this branch, e.g. --go 1.16.1
  -log_backtrace_at value
    	when logging hits line file:N, emit a stack trace
  -log_dir string
    	If non-empty, write log files in this directory
  -logtostderr
    	log to standard error instead of files
  -o string
    	Path to export the updated rules to, e.g. -o /tmp/rules.yaml
  -rules string
    	[required] URL or Path of the rules file to update rules for, e.g. --rules path/or/url/to/rules/file.yaml
  -stderrthreshold value
    	logs at or above this threshold go to stderr
  -v value
    	log level for V logs
  -vmodule value
    	comma-separated list of pattern=N settings for file-filtered logging
```

#### Required flags:

- `-rules` flag with value is required for processing input rules file
- `-branch` flag with value is required for adding/updating rules for all destination repos

#### Optional flags:

- `-go` flag refers to golang version which should be pinned for given branch, if not given an empty string is set
- `-o` flag refers to output file where the processed rules should be exported, otherwise rules are printed on stdout
