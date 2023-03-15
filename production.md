# Kubernetes publishing-bot production instance notes

### What is this and what does it do?

The publishing-bot for the Kubernetes project is running in the `publishing-bot` namespace  on a CNCF sponsored GKE cluster `aaa` in the `kubernetes-public` project.

### How do i get access to this?

If you need access to any of the following, please update [groups.yaml].

### GKE instance

publishing-bot is running in a GKE cluster named `aaa` in the `kubernetes-public`
- [GKE project](https://console.cloud.google.com/kubernetes/list/overview?project=kubernetes-public)
- [aaa cluster](https://console.cloud.google.com/kubernetes/clusters/details/us-central1/aaa/details?project=kubernetes-public)

The cluster can be accessed by [k8s-infra-rbac-publishing-bot@kubernetes.io].
To access the cluster, please see these [instructions].

### What images does it use?

Publishing-bot [images] can be pushed by [k8s-infra-staging-publishing-bot@kubernetes.io].

### What commands are in this repo and how/when do i use them?

Make sure you are at the root of the publishing-bot repo before running these commands.

#### Populating repos

This script needs to be run whenever a new staging repo is added in kubernetes/kubernetes

```sh
hack/fetch-all-latest-and-push.sh kubernetes
```

#### Deploying the bot

```sh
make validate build-image push-image deploy CONFIG=configs/kubernetes
```

### How to connect to the `aaa` cluster

You can use the `Activate Cloud Shell` in the GCP console above and in that console, run the following command
```
gcloud container clusters get-credentials aaa --region us-central1 --project kubernetes-public
```

then run `kubectl` commands to ensure you can see what's running in the cluster.

### What is running there?

The `publishing-bot` runs in a separate kubernetes namespace by the same name in the `aaa` cluster.
The manifests [here](https://github.com/kubernetes/publishing-bot/tree/master/artifacts/manifests) have the definitions
for these kubernetes resources. Example below:

````shell
davanum@cloudshell:~ (kubernetes-public)$ kubectl get pv,pvc,replicaset,pod -n publishing-bot
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS        CLAIM                                          STORAGECLASS   REASON   AGE
persistentvolume/pvc-084a4d52-0a57-4f70-a76a-5d2d2667429d   100Gi      RWO            Delete           Bound         publishing-bot/publisher-gopath                ssd                     8h

NAME                                     STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/publisher-gopath   Bound    pvc-084a4d52-0a57-4f70-a76a-5d2d2667429d   100Gi      RWO            ssd            8h

NAME                        DESIRED   CURRENT   READY   AGE
replicaset.apps/publisher   1         1         1       45d

NAME                  READY   STATUS    RESTARTS   AGE
pod/publisher-cdvwj   1/1     Running   0          9h
````

### How do i know if/when the bot fails?

Follow this [Kubernetes issue #56876](https://github.com/kubernetes/kubernetes/issues/56876). When the bot fails it
re-opens this issue with a fresh log. So if you are subscribed to this issue, you can see the bot open the issue
when it fails.

### How do i see what the publishing bot is doing?

you can stream the logs of the pod to see what the publishing-bot is doing
```shell
kubectl -n publishing-bot logs pod/publisher-cdvwj -f
```

### What is the persistent volume for?

To do its work the publishing-bot has to download all the repositories and performs git surgery on them. So publishing-bot
keeps the downloaded copy around and re-uses them. For example, if the pod gets killed the new pod can still work off
of the downloaded git repositories on the persistent volume. Occasionally if we suspect the downloaded git repos are
corrupted for some reason (say github flakiness), we may have to cleanup the pv/pvc. in other words, The volume is
cache only. Wiping it is not harmful in general (other than for the time it takes to recreate all the data).

### How do i clean up the pvc?

Step 1: Use the command about to find the pvc and the pod then clean them up in one shot
```shell
kubectl delete -n publishing-bot pod/publisher-cdvwj persistentvolumeclaim/publisher-gopath
```

Step 2: Re-deploy the pvc again
```shell
kubectl apply -n publishing-bot -f artifacts/manifests/pvc.yaml
```

Step 3: Watch the pod start back up from `Pending`
```shell
kubectl -n publishing-bot get pods
```

[k8s-infra-rbac-publishing-bot@kubernetes.io]: https://github.com/kubernetes/k8s.io/blob/7e72aa72f1548af9cf3dbe405f8c317fe637f361/groups/groups.yaml#L405-L418
[k8s-infra-staging-publishing-bot@kubernetes.io]: https://github.com/kubernetes/k8s.io/blob/6a6b50f4d04124b02915bc2736b468def0de96e9/groups/groups.yaml#L992-L1001
[images]: https://console.cloud.google.com/gcr/images/k8s-staging-publishing-bot/GLOBAL/k8s-publishing-bot
[groups.yaml]: https://git.k8s.io/k8s.io/groups/groups.yaml
[instructions]: https://git.k8s.io/k8s.io/running-in-community-clusters.md#access-the-cluster