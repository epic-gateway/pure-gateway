Pure Gateway
============

This is the client implementation of the Kubernetes Gateway-SIG
Gateway v1alpha2 API. It's a client proxy that works with the EPIC
Gateway server.

https://gateway-api.sigs.k8s.io/

Installation
------------

Before installing Pure Gateway you need to install the k8s Gateway-SIG
custom resource definitions manually. Eventually they'll be bundled
into k8s but they aren't yet.

https://gateway-api.sigs.k8s.io/v1alpha2/guides/getting-started/#installing-gateway-api-crds-manually

We haven't implemented Helm charts yet so installation uses
old-fashioned yaml manifests. To install Pure Gateway, apply
`pure-gateway.yaml` from the most recent release on GitHub:
https://github.com/epic-gateway/pure-gateway/releases

You should see one instance of the manager pod running, and one
instance of the agent pod running on each node in the cluster.

There's a sample GatewayClassConfig in this project in
config/samples/puregw_v1_gatewayclassconfig.yaml that works in the
acndev environment. There's also a set of trivial samples that you can
use as a template for your Gateways.

[Sample Configurations](config/samples)
