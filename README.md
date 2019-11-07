# mc2019

A proof of concept that demonstrates high-level Istio multi-mesh.

**expose** and **bind** CRDs are defined.  A **configuration** CRD is defined.

A controller converts these CRDs into Istio CRDs using the style of
https://github.com/istio-ecosystem/multi-mesh-examples/blob/master/add_hoc_limited_trust/README.md

## Developer instructions

Before running, enable the new CRDs on your Kubernetes clusters:

``` bash
CLUSTER1=...
kubectl --context $CLUSTER1 apply -f config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml
kubectl --context $CLUSTER1 apply -f config/crd/bases/mm.ibm.istio.io_servicebindings.yaml
kubectl --context $CLUSTER1 apply -f config/crd/bases/mm.ibm.istio.io_serviceexpositions.yaml
CLUSTER2=...
kubectl --context $CLUSTER2 apply -f config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml
kubectl --context $CLUSTER2 apply -f config/crd/bases/mm.ibm.istio.io_servicebindings.yaml
kubectl --context $CLUSTER2 apply -f config/crd/bases/mm.ibm.istio.io_serviceexpositions.yaml
```

Also, follow [Vadim's mutual TLS setup instructions](https://github.com/istio-ecosystem/multi-mesh-examples/tree/master/add_hoc_limited_trust/common-setup#prerequisites-for-three-clusters).

To start, do `make run`.  TODO We need to do this twice, once for each cluster, with different contexts.

To test, we first need to tell the system what kind of security to implement:

``` bash
kubectl --context $CLUSTER1 apply -f samples/limited-trust-c1.yaml
kubectl --context $CLUSTER2 apply -f samples/limited-trust-c2.yaml
```

By applying these MeshFedConfigs, the mc2019 system creates a namespace, an ingress and an egress service.

TODO It is still your job to create the Secret and Deployment.

Next, we will expose a Service

TODO start a service and expose it
