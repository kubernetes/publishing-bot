# Kubernetes Publishing Bot

[![sig-release-publishing-bot/build](https://testgrid.k8s.io/q/summary/sig-release-publishing-bot/build/tests_status?style=svg)](https://testgrid.k8s.io/sig-release-publishing-bot#build)
[![](https://img.shields.io/uptimerobot/status/m779759348-04b1f4fd3bb5ce4a810670d2.svg?label=bot)](https://stats.uptimerobot.com/wm4Dyt8kY)
[![](https://img.shields.io/uptimerobot/status/m779759340-0a6b2cb6fee352e75f58ba16.svg?label=last%20publishing%20run)](https://github.com/kubernetes/kubernetes/issues/56876)

## Overview

The publishing bot publishes the code in `k8s.io/kubernetes/staging` to their own repositories. It guarantees that the master branches of the published repositories are compatible, i.e., if a user `go get` a published repository in a clean GOPATH, the repo is guaranteed to work.

It pulls the latest k8s.io/kubernetes changes and runs `git filter-branch` to distill the commits that affect a staging repo. Then it cherry-picks merged PRs with their feature branch commits to the target repo. It records the SHA1 of the last cherrypicked commits in `Kubernetes-sha: <sha>` lines in the commit messages.

The robot is also responsible to update the `Godeps/Godeps.json` and the `vendor/` directory for the target repos.

## Playbook

### Publishing a new repo or a new branch

* Adapt the rules in [config/kubernetes-rules-configmap.yaml](configs/kubernetes-rules-configmap.yaml)

* For a new repo, add it to the repo list in [hack/fetch-all-latest-and-push.sh](hack/fetch-all-latest-and-push.sh)

* [Test and deploy the changes](#testing-and-deploying-the-robot)

### Testing and deploying the robot

Currently we don't have tests for the bot. It relies on manual tests:

* Fork the repos you are going the publish.
* Run [hack/fetch-all-latest-and-push.sh](hack/fetch-all-latest-and-push.sh) from the bot root directory to update the branches of your repos. This will sync your forks with upstream. **CAUTION:** this might delete data in your forks.

* Create a config and a corresponding ConfigMap in [configs](configs),
  - by copying [configs/example](configs/example) and [configs/example-configmap.yaml](configs/example-configmap.yaml),
  - and by changing the Makefile constants in `configs/<yourconfig>`
  - and the ConfigMap values in  `configs/<yourconfig>-configmap.yaml`.

* Create a rule config and a corresponding ConfigMap in [configs](configs),
  - by copying [configs/example-rules-configmap.yaml](configs/example-rules-configmap.yaml),
  - and by changing the Makefile constants in `configs/<yourconfig>`
  - and the ConfigMap values in  `configs/<yourconfig>-rules-configmap.yaml`.

* Deploy the publishing bot by running make from the bot root directory, e.g.

```shell
$ make build-image push-image CONFIG=configs/<yourconfig>
$ make run CONFIG=configs/<yourconfig> TOKEN=<github-token>
```

  for a fire-and-forget pod. Or use

```shell
$ make deploy CONFIG=configs/<yourconfig> TOKEN=<github-token>
```

  to run a ReplicationController that publishes every 24h (you can change the `INTERVAL` config value for different intervals).

This will not push to your org, but runs in dry-run mode. To run with a push, add `DRYRUN=false` to your `make` command line.

### Running in Production

* Use one of the existing [configs](configs) and
* launch `make deploy CONFIG=configs/kubernetes-nightly`

**Caution:** Make sure that the bot github user CANNOT close arbitrary issues in the upstream repo. Otherwise, github will close, them triggered by `Fixes kubernetes/kubernetes#123` patterns in published commits.

## Contributing

Please see [CONTRIBUTING.md](CONTRIBUTING.md) for instructions on how to contribute.

## Known issues

1. Testing: currently we rely on manual testing. We should set up CI for it.
2. Automate release process (tracked at https://github.com/kubernetes/kubernetes/issues/49011): when kubernetes release, automatic update the configuration of the publishing robot. This probably means that the config must move into the Kubernetes repo, e.g. as a `.publishing.yaml` file.
