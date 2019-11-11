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
kubectl --context $CLUSTER1 apply -f samples/limited-trust-c1.yaml,samples/secret-c1.yaml
kubectl --context $CLUSTER2 apply -f samples/limited-trust-c2.yaml,samples/secret-c2.yaml
```

By applying these MeshFedConfigs, the mc2019 system creates a namespace, an ingress and an egress service.

TODO It is still your job to create the Secret and Deployment.

Next, we will expose a Service

TODO start a service and expose it

Next, we will bind to the Service

``` bash
CLUSTER2_INGRESS=$(kubectl --context $CLUSTER2 get svc --selector mesh=limited-trust --output jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")
echo Using $CLUSTER2 ingress at $CLUSTER2_INGRESS:15443
cat samples/helloworld-binding.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -
```

To see the results,

``` bash
kubectl get svc binding-limited-trust -o yaml
kubectl get endpoints binding-limited-trust -o yaml
```

### Cleanup

kubectl delete servicebinding helloworld
kubectl delete meshfedconfig limited-trust
