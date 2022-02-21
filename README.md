EPIC Gateway
============

This is the client-cluster implementation of the Kubernetes
Gateway-SIG Gateway v1alpha2 API. It's a client proxy that works with
EPIC on the server side.

https://gateway-api.sigs.k8s.io/

Installation
------------

Before installing the EPIC Gateway you need to install the k8s
Gateway-SIG custom resource definitions manually. Eventually they'll
be bundled into k8s but they aren't yet.

https://gateway-api.sigs.k8s.io/v1alpha2/guides/getting-started/#installing-gateway-api-crds-manually

Our Gitlab docker registry is private so you need to create a k8s
secret containing registry access credentials. You can use a Gitlab
"personal access token" with read-only access to this project.

```
$ kubectl create namespace epic-gateway
$ kubectl -n epic-gateway create secret docker-registry gitlab --docker-server=registry.gitlab.com --docker-username={your gitlab username} --docker-password={your gitlab access token}
```

We haven't implemented Helm charts yet so installation uses
old-fashioned yaml manifests. If you want to live on the edge then you
can install the "main-latest" version. Download and apply the newest
version of epic-gateway.yaml from
https://gitlab.com/acnodal/epic/gateway/-/packages/5252354

You should see one instance of the manager pod running, and one
instance of the agent pod running on each node in the cluster.

There's a sample GatewayClassConfig in this project in
config/samples/puregw_v1_gatewayclassconfig.yaml that works in the
acndev environment. There's also a set of trivial samples that you can
use as a template for your Gateways.

[Sample Configurations](config/samples)
