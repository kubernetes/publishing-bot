# Kubernetes Publishing Bot

[![sig-release-publishing-bot/build](https://testgrid.k8s.io/q/summary/sig-release-publishing-bot/build/tests_status?style=svg)](https://testgrid.k8s.io/sig-release-publishing-bot#build)
[![](https://img.shields.io/uptimerobot/status/m779759348-04b1f4fd3bb5ce4a810670d2.svg?label=bot)](https://stats.uptimerobot.com/wm4Dyt8kY)
[![](https://img.shields.io/uptimerobot/status/m779759340-0a6b2cb6fee352e75f58ba16.svg?label=last%20publishing%20run)](https://github.com/kubernetes/kubernetes/issues/56876)

## Overview

The publishing bot publishes the code in `k8s.io/kubernetes/staging` to their own repositories. It guarantees that the master branches of the published repositories are compatible, i.e., if a user `go get` a published repository in a clean GOPATH, the repo is guaranteed to work.

It pulls the latest k8s.io/kubernetes changes and runs `git filter-branch` to distill the commits that affect a staging repo. Then it cherry-picks merged PRs with their feature branch commits to the target repo. It records the SHA1 of the last cherrypicked commits in `Kubernetes-sha: <sha>` lines in the commit messages.

The robot is also responsible to update the `go-mod` and the `vendor/` directory for the target repos.

## Playbook

### Publishing a new repo or a new branch

* Adapt the rules in [config/kubernetes-rules-configmap.yaml](configs/kubernetes-rules-configmap.yaml)
  * For Kubernetes, the configuration is located in the [kubernetes/kubernetes repository](https://github.com/kubernetes/kubernetes/blob/master/staging/publishing/rules.yaml)

* For a new repo, add it to the repo list in [hack/repos.sh](hack/repos.sh)

* [Test and deploy the changes](#testing-and-deploying-the-robot)

### Updating rules

#### Adapting rules for a new branch

If you're creating a new branch, you need to update the publishing-bot rules to reflect that. For Kubernetes, this means that you need to update the [`rules.yaml` file](https://github.com/kubernetes/kubernetes/blob/master/staging/publishing/rules.yaml) on the `master` branch.

For each repository, add a new branch to the `branches` stanza. If the branch is using the same Go version as the [default Go version](https://github.com/kubernetes/kubernetes/blob/489fb9bee3f626b3eeb120a5af89ad8c2b2f1c20/staging/publishing/rules.yaml#L10), you don't need to specify the Go version for the branch (otherwise you need to do that).

#### Adapting rules for a Go update

If you're updating Go version for the master or release branches, you need to adapt the [`rules.yaml` file in kubernetes/kubernetes](https://github.com/kubernetes/kubernetes/blob/master/staging/publishing/rules.yaml) on the `master` branch.

* If you're updating Go version for the master branch, you need to change the [default Go version](https://github.com/kubernetes/kubernetes/blob/489fb9bee3f626b3eeb120a5af89ad8c2b2f1c20/staging/publishing/rules.yaml#L10) to the new version.
  * If release branches that depend on the default Go version use a different (e.g. old) Go version, you need to explicitly set Go version for those branches (e.g. [like here](https://github.com/kubernetes/kubernetes/blob/489fb9bee3f626b3eeb120a5af89ad8c2b2f1c20/staging/publishing/rules.yaml#L37))
* If you're updating Go version for a previous release branch
  * if it's the same version as the default Go version, you don't need to specify the Go version for that branch
  * if it's **NOT** the same version as the default Go version, you need to explicitly specify the Go version for that branch (e.g. [like here](https://github.com/kubernetes/kubernetes/blob/489fb9bee3f626b3eeb120a5af89ad8c2b2f1c20/staging/publishing/rules.yaml#L37))
    * Examples: https://github.com/kubernetes/kubernetes/pull/93998, https://github.com/kubernetes/kubernetes/pull/101232, https://github.com/kubernetes/kubernetes/pull/104226

### Testing and deploying the robot

Currently we don't have tests for the bot. It relies on manual tests:

* Fork the repos you are going the publish.
* Run [hack/fetch-all-latest-and-push.sh](hack/fetch-all-latest-and-push.sh) from the bot root directory to update the branches of your repos. This will sync your forks with upstream. **CAUTION:** this might delete data in your forks.
* Use [hack/create-repos.sh](hack/create-repos.sh) from the bot root directory to create any missing repos in the destination github org.

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

  to run a ReplicaSet that publishes every 24h (you can change the `INTERVAL` config value for different intervals).

This will not push to your org, but runs in dry-run mode. To run with a push, add `DRYRUN=false` to your `make` command line.

### Running in Production

* Use one of the existing [configs](configs) and
* launch `make deploy CONFIG=configs/kubernetes-nightly`

**Caution:** Make sure that the bot github user CANNOT close arbitrary issues in the upstream repo. Otherwise, github will close, them triggered by `Fixes kubernetes/kubernetes#123` patterns in published commits.

**Note:**: Details about running the publishing-bot for the Kubernetes project can be found in [k8s-publishing-bot.md](k8s-publishing-bot.md).

## Contributing

Please see [CONTRIBUTING.md](CONTRIBUTING.md) for instructions on how to contribute.

## Known issues

1. Testing: currently we rely on manual testing. We should set up CI for it.
2. Automate release process (tracked at https://github.com/kubernetes/kubernetes/issues/49011): when kubernetes release, automatic update the configuration of the publishing robot. This probably means that the config must move into the Kubernetes repo, e.g. as a `.publishing.yaml` file.
