# mc2019

A proof of concept that demonstrates high-level Istio multi-mesh.

**expose** and **bind** CRDs are defined.  A **configuration** CRD is defined.

A controller converts these CRDs into Istio CRDs using the style of
<https://github.com/istio-ecosystem/multi-mesh-examples/blob/master/add_hoc_limited_trust/README.md>

## Developer instructions

Before running, enable the new CRDs on your Kubernetes clusters:

``` bash
CLUSTER1=istio-test-paid3 # TODO change back to "..."
kubectl --context $CLUSTER1 apply -f config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml
kubectl --context $CLUSTER1 apply -f config/crd/bases/mm.ibm.istio.io_servicebindings.yaml
kubectl --context $CLUSTER1 apply -f config/crd/bases/mm.ibm.istio.io_serviceexpositions.yaml
CLUSTER2=istio-test-paid2 # TODO change back to "..."
kubectl --context $CLUSTER2 apply -f config/crd/bases/mm.ibm.istio.io_meshfedconfigs.yaml
kubectl --context $CLUSTER2 apply -f config/crd/bases/mm.ibm.istio.io_servicebindings.yaml
kubectl --context $CLUSTER2 apply -f config/crd/bases/mm.ibm.istio.io_serviceexpositions.yaml
```

Also, follow [Vadim's mutual TLS setup instructions](https://github.com/istio-ecosystem/multi-mesh-examples/tree/master/add_hoc_limited_trust/common-setup#prerequisites-for-three-clusters).

To start, do `make run` or after a `make` do a `./bin/manager`. That way, one can run the controller for two clusters on the same system by specifying the `metrics-addr` as in:
`./bin/manager  --metrics-addr \":8080\"` and `./bin/manager  --metrics-addr \":8081\"`.
TODO We need to do this twice, once for each cluster, with different contexts.

To test, we first need to tell the system what kind of security to implement:

``` bash
kubectl --context $CLUSTER1 create ns limited-trust
kubectl --context $CLUSTER2 create ns limited-trust

kubectl --context $CLUSTER1 apply -f samples/limited-trust/limited-trust-c1.yaml,samples/limited-trust/secret-c1.yaml
kubectl --context $CLUSTER2 apply -f samples/limited-trust/limited-trust-c2.yaml,samples/limited-trust/secret-c2.yaml
```

After applying these MeshFedConfigs the system creates ingress and egress services.

Next, we will expose a Service

``` bash
kubectl --context $CLUSTER2 apply -f samples/limited-trust/helloworld.yaml
kubectl --context $CLUSTER2 apply -f samples/limited-trust/helloworld-expose.yaml
```

Next, we will bind to the Service

``` bash
CLUSTER2_INGRESS=$(kubectl --context $CLUSTER2 get svc -n limited-trust --selector mesh=limited-trust,role=ingress-svc --output jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")
echo Using $CLUSTER2 ingress at $CLUSTER2_INGRESS:15443
cat samples/limited-trust/helloworld-binding.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -
```

To see the Istio resources created by the controller,

``` bash
kubectl get svc binding-limited-trust -o yaml
kubectl get endpoints binding-limited-trust -o yaml
```

### Test script

``` bash
make manager
./test/integration/bp.sh
./test/integration/cleanup-bp.sh
./test/integration/pt.sh
```

### Test interactively

First, test the exposure itself

``` bash
CLUSTER2_SECURE_INGRESS_PORT=15443
CLUSTER2_INGRESS_HOST=$CLUSTER2_INGRESS
kubectl --context $CLUSTER1 get secret c1-example-com-certs --output jsonpath="{.data.tls\.key}" | base64 -D > /tmp/c1.example.com.key
kubectl --context $CLUSTER1 get secret c1-example-com-certs --output jsonpath="{.data.tls\.crt}" | base64 -D > /tmp/c1.example.com.crt
kubectl --context $CLUSTER1 get secret c1-example-com-certs --output jsonpath="{.data.example\.com\.crt}" | base64 -D > /tmp/example.com.crt
curl --resolve c2.example.com:$CLUSTER2_SECURE_INGRESS_PORT:$CLUSTER2_INGRESS_HOST --cacert /tmp/example.com.crt --key /tmp/c1.example.com.key --cert /tmp/c1.example.com.crt https://c2.example.com:$CLUSTER2_SECURE_INGRESS_PORT/helloworld/hello -w "\nResponse code: %{http_code}\n"
```

Then test that the binding works

``` bash
SLEEP_POD=$(kubectl --context $CLUSTER1 get pods -l app=sleep -o jsonpath="{.items..metadata.name}")
Echo using Sleep pod $SLEEP_POD on $CLUSTER1
kubectl --context $CLUSTER1 exec -it $SLEEP_POD -- curl helloworld:5000/hello
```

### Troubleshooting

Try to run the test from the Egress itself

``` bash
EGRESS_POD=$(kubectl --context $CLUSTER1 -n limited-trust get pod -l istio=egressgateway -o jsonpath='{.items..metadata.name}')
echo Egress on $CLUSTER1 is $EGRESS_POD
kubectl --context $CLUSTER1 -n limited-trust exec $EGRESS_POD -- curl --resolve c2.example.com:$CLUSTER2_SECURE_INGRESS_PORT:$CLUSTER2_INGRESS_HOST --cacert /etc/istio/mesh/certs/example.com.crt --key /etc/istio/mesh/certs/tls.key --cert /etc/istio/mesh/certs/tls.crt https://c2.example.com:$CLUSTER2_SECURE_INGRESS_PORT/helloworld/hello
```

### Cleanup

kubectl --context $CLUSTER2 delete serviceexposure helloworld
kubectl --context $CLUSTER2 delete meshfedconfig limited-trust
kubectl --context $CLUSTER1 delete servicebinding helloworld
kubectl --context $CLUSTER1 delete meshfedconfig limited-trust
