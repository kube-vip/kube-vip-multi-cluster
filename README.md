# kube-vip-multi-cluster

My obsession with [Sun Cluster](https://en.wikipedia.org/wiki/Solaris_Cluster) is complete!

I'll potentially write a blog post about this, but "yonks" ago I used to manage multiple HA applications across big sun systems. If one system failed, then the cluster software would immediately start it on the other system within the cluster.. so why not do the same across mulitple Kubernetes clusters.

**NOTE** this is a proof of concept at the moment, so if you run this to power your finance, telco or interconntinental crime organisation I can't help you.

## Description

Each cluster will require a multi-cluster controller running, the controller will watch for objects of type `MultiCluster` being created. A `LoadBalancer` is required for each multi cluster controller and should use the correct selector so that it will expose access to the multi-cluster controller once it starts!

```
kubectl expose deployment -n kube-vip-multi-cluster-system kube-vip-multi-cluster-controller-manager  \
       --port=9900 \
       --selector='control-plane=controller-manager' \
       --type=LoadBalancer \
       --name=multi-cluster-service \
       --load-balancer-ip=x.x.x.x
```

**Note** A different port is required for each multi cluster configuration, so if your nodes are part of multiple "multi cluster" configurations you'll need to add additional services (choose a load balancer that supports multiple ports per IP... like kube-vip)

Depoying the controller is as simple as dropping a manifest like this `kubectl apply -f https://raw.githubusercontent.com/kube-vip/kube-vip-multi-cluster/main/dist/install.yaml`. 

Finally, you'll need to create a `MulitCluster` manifest like the ones in `/testing/manifests`, the main parts are all within the spec section:

### Multi Cluster Specification

#### Port

The port defines which port the multicluster will listen on for incoming connections as part of the leaderElection algorithim.

`port: 9990`

#### CallSign

The callsign is a simple key/string used to designate all members of the cluster should be speaking to one another, if someone joins an existing cluster with a different callsign they wont be allowed to participate.

`callsign: "federation"`

#### Rank

This is used to designate the rank of a cluster member, the higher the ranking the more likely they are to be selected as the leader of the group.

`rank: 5`

#### Ready

This should be set to true on the first member of the cluster, so that an initial leader is selected and so that the member will begin listening for additional members.
`ready: true`
  
#### Service Address

This should be taken from the IP address assigned to the service being used to expose the member of the multi-cluster. This address is then sent to all other members during the lifecycle of the cluster, so that they can conect back to this node as membership changes.

`serviceAddress: 172.18.100.100`

#### Payload

The payload contains the various Kubernetes manifests that are **applied** when a member becomes a leader, and **deleted** when that leader becomes demoted.


```
  payload: |
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: nginx-deployment2
    spec:
```

## Getting Started

### Create two test clusters with loadbalancer services (uses kind, but the binary isn't required)

```
go run ./testing/clusters/ \
       -name newyork \
       -svcAddress 172.18.100.100; \
go run ./testing/clusters/ \
       -name sheffield \
       -svcAddress 172.18.100.200
```

### Delete the clusters 

```
go run ./testing/clusters/ \
       -name newyork \
       -forceClean; \
go run ./testing/clusters/ \
       -name sheffield \
       -forceClean
```

### Apply two multicluster manifests

```
kubectl --context kind-newyork apply -f ./testing/manifests/newyork.yaml
```

```
kubectl --context kind-sheffield apply -f ./testing/manifests/sheffield.yaml
```

## Rebuild & reload

Really quick rebuild:

This will rebuild the image, push it to the two clusters and recreate all the configurations.

```
make docker-build-quick
```

#### Debug
```
tmux new-session \; send-keys 'kubectl logs -f -n kube-vip-multi-cluster-system  deployment/kube-vip-multi-cluster-controller-manager --context=kind-sheffield' C-m \; split-window -v\; send-keys 'kubectl logs -f -n kube-vip-multi-cluster-system deployment/kube-vip-multi-cluster-controller-manager --context=kind-newyork' Enter
 ```

## For Development work, follow the details below

### Prerequisites
- go version v1.21.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.

### To Deploy on the cluster
**Build and push your image to the location specified by `IMG`:**

```sh
make docker-build docker-push IMG=<some-registry>/kube-vip-multi-cluster:tag
```

**NOTE:** This image ought to be published in the personal registry you specified. 
And it is required to have access to pull the image from the working environment. 
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=<some-registry>/kube-vip-multi-cluster:tag
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin 
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/kube-vip-multi-cluster:tag
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/kube-vip-multi-cluster/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

