# Kubernetes Publishing Bot [![Build Status](https://travis-ci.org/tombell/travis-ci-status.svg)](https://travis-ci.org/kubernetes-nightly/publishing-bot.png)

## Overview

The publishing bot publishes the code in `k8s.io/kubernetes/staging` to their own repositories. It guarantees that the master branches of the published repositories are compatible, i.e., if a user `go get` a published repository in a clean GOPATH, the repo is guaranteed to work.

It pulls the latest k8s.io/kubernetes changes and runs `git filter-branch` to distill the commits that affect a staging repo. Then it cherry-picks merged PRs with their feature branch commits to the target repo. It records the SHA1 of the last cherrypicked commits in `Kubernetes-sha: <sha>` lines in the commit messages.

The robot is also responsible to update the `Godeps/Godeps.json` and the `vendor/` directory for the target repos.

## Playbook

### Publishing a new repo

* Create a (repoRules) in [cmd/publishing-bot/publisher.go](cmd/publishing-bot/publisher.go)

* Add a `publish_<repository_name>.sh` in [artifacts/scripts](artifacts/scripts)

* Add the repo to the repo list in [hack/fetch-all-latest-and-push.sh](hack/fetch-all-latest-and-push.sh)

* [Test and deploy the changes](#testing-and-deploying-the-robot)

### Publishing a new branch

* Update the (repoRules) in [cmd/publishing-bot/publisher.go](cmd/publishing-bot/publisher.go)

* [Test and deploy the changes](#testing-and-deploying-the-robot)

### Testing and deploying the robot

Currently we don't have tests for the bot. It relies on manual tests:

* Fork the repos you are going the publish.
* Run [hack/fetch-all-latest-and-push.sh](hack/fetch-all-latest-and-push.sh) from the bot root directory to update the branches of your repos. This will sync your forks with upstream. **CAUTION:** this might delete data in your forks.

* Change `target-org` to your github username in [artifacts/manifests/configmap.yaml](artifacts/manifests/configmap.yaml)

* Deploy the publishing bot by running make from the bot root directory, e.g.

```shell
$ make build-image push-image REPO=<your-docker-name>/k8s-publishing-bot
$ make run REPO=<your-docker-name>/k8s-publishing-bot TOKEN=<github-token>
```

  for a fire-and-forget job. Or use

```shell
$ make deploy REPO=<your-docker-name>/k8s-publishing-bot TOKEN=<github-token>
```

  to run a nightly (5:00am UTC) cronjob.

This will not push to your org, but runs in dry-run mode. To run with a push, add `DRYRUN=false` to your `make` command line.

### Running in Production

* Use one of the existing [configs](configs) and
* launch `make deploy CONFIG=configs/kubernetes-nightly`

**Caution:** Make sure that the bot github user CANNOT close arbitrary issues in the upstream repo. Otherwise, github will close, them triggered by `Fixes kubernetes/kubernetes#123` patterns in published commits.

## Known issues

1. Reporting issues: the publishing robot should file an issue and attach its logs if it meets bugs during publishing.
2. Testing: currently we rely on manual testing. We should set up CI for it.
3. Automate release process (tracked at https://github.com/kubernetes/kubernetes/issues/49011): when kubernetes release, automatic update the configuration of the publishing robot. This probably means that the config must move into the Kubernetes repo, e.g. as a `.publishing.yaml` file.
