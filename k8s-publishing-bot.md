# Kubernetes Publishing Bot

The publishing-bot for the Kubernetes project is running on a CNCF sponsored
GKE cluster `development2` in the `kubernetes-public` project.

The bot is responsible for updating `go.mod`/`Godeps` and `vendor` for target repos.
To support both godeps for releases <= v1.14 and go modules for the master branch
and releases >= v1.14, we run two instances of the bot today.

The instance of the bot responsible for syncing releases <= v1.14 runs in the
`k8s-publishing-bot-godeps` namespace and the instance of the bot responsible
for syncing the master branch and releases >= v1.14 runs in the ` k8s-publishing-bot`
namespace.

The code for the former can be found in the [godeps branch] and the latter in the master
branch of the publishing-bot repo.

## Permissions

The cluster can be accessed by members of the [k8s-infra-cluster-admins]
google group. Members can be added to the group by updating [groups.yaml].

## Running the bot

Make sure you are at the root of the publishing-bot repo before running these commands.

### Populating repos

This script needs to be run whenever a new staging repo is added.

```sh
hack/fetch-all-latest-and-push.sh kubernetes
```

### Deploying the bot

```sh
make validate build-image push-image deploy CONFIG=configs/kubernetes
```

[godeps branch]: https://github.com/kubernetes/publishing-bot/tree/godeps
[k8s-infra-cluster-admins]: https://groups.google.com/forum/#!forum/k8s-infra-cluster-admins
[groups.yaml]: https://git.k8s.io/k8s.io/groups/groups.yaml
