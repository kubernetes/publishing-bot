# Kubernetes Publishing Bot

The publishing-bot for the Kubernetes project is running in the
`k8s-publishing-bot` namespace  on a CNCF sponsored GKE cluster
`development2` in the `kubernetes-public` project.

The bot is responsible for updating `go.mod`/`Godeps` for target repos.

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

[k8s-infra-cluster-admins]: https://groups.google.com/forum/#!forum/k8s-infra-cluster-admins
[groups.yaml]: https://git.k8s.io/k8s.io/groups/groups.yaml
