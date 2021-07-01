# Kubernetes Publishing Bot

The publishing-bot for the Kubernetes project is running in the
`publishing-bot` namespace  on a CNCF sponsored GKE cluster
`aaa` in the `kubernetes-public` project.

The bot is responsible for updating `go.mod` for target repos.

## Permissions

If you need access to any of the following, please update [groups.yaml].

### Cluster

The cluster can be accessed by [k8s-infra-rbac-publishing-bot@kubernetes.io].
To access the cluster, please see these [instructions].

### Images

Publishing-bot [images] can be pushed by [k8s-infra-staging-publishing-bot@kubernetes.io].

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

[k8s-infra-rbac-publishing-bot@kubernetes.io]: https://github.com/kubernetes/k8s.io/blob/7e72aa72f1548af9cf3dbe405f8c317fe637f361/groups/groups.yaml#L405-L418
[k8s-infra-staging-publishing-bot@kubernetes.io]: https://github.com/kubernetes/k8s.io/blob/6a6b50f4d04124b02915bc2736b468def0de96e9/groups/groups.yaml#L992-L1001
[images]: https://console.cloud.google.com/gcr/images/k8s-staging-publishing-bot/GLOBAL/k8s-publishing-bot
[groups.yaml]: https://git.k8s.io/k8s.io/groups/groups.yaml
[instructions]: https://github.com/kubernetes/k8s.io/blob/main/running-in-community-clusters.md#access-the-cluster
