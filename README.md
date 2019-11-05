# mc2019

A proof of concept that demonstrates high-level Istio multi-mesh.

**expose** and **bind** CRDs are defined.  A **configuration** CRD is defined.

A controller converts these CRDs into Istio CRDs using the style of
https://github.com/istio-ecosystem/multi-mesh-examples/blob/master/add_hoc_limited_trust/README.md

## Developer instructions

Before running, enable the new CRDs on your Kubernetes cluster:

``` bash
kubectl apply -f config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml
kubectl apply -f config/crd/bases/mm.ibm.istio.io_servicebindings.yaml
kubectl apply -f config/crd/bases/mm.ibm.istio.io_serviceexpositions.yaml
```

To start, do `make run`

To test, we first need to tell the system what kind of security to implement:

``` bash
kubectl apply -f samples/limited-trust.yaml
```

By applying this MeshFedConfig, the mc2019 system creates a namespace, an ingress and an egress service.

TODO It is still your job to create the Secret and Deployment.

Next, we will expose a Service

TODO start a service and expose it
