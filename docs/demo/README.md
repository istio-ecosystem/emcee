# Demo instructions

## Prerequisites

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

To start, do `make manager` then `tools/run-two-managers.sh`.  You will need to export environment variables `CTX_CLUSTER1` and `CTX_CLUSTER2` for two cluster contexts known to your Kubernetes configuration.  (To see if you have two contexts, `kubectl config get-contexts`).

## The boundary-protection demo

To test, we first need to tell the system what kind of security to implement:

``` bash
kubectl --context $CLUSTER1 create ns limited-trust
kubectl --context $CLUSTER2 create ns limited-trust

kubectl --context $CLUSTER1 apply -f samples/limited-trust/limited-trust-c1.yaml,samples/limited-trust/secret-c1.yaml
kubectl --context $CLUSTER2 apply -f samples/limited-trust/limited-trust-c2.yaml,samples/limited-trust/secret-c2.yaml
```

After applying these MeshFedConfigs the system creates ingress and egress services.

To demonstrate exposing a service we first need a service:

``` bash
kubectl --context $CLUSTER2 apply -f samples/limited-trust/helloworld.yaml
kubectl --context $CLUSTER2 wait --for=condition=available --timeout=60s deployment/helloworld-v1
```

Next, we will expose the service

``` bash
kubectl --context $CLUSTER2 apply -f samples/limited-trust/helloworld.yaml
kubectl --context $CLUSTER2 apply -f samples/limited-trust/helloworld-expose.yaml
```

Next, we will bind to the service

``` bash
CLUSTER2_INGRESS=$(kubectl --context $CLUSTER2 get svc -n limited-trust --selector mesh=limited-trust,role=ingress-svc --output jsonpath="{.items[0].status.loadBalancer.ingress[0].ip}")
echo Using $CLUSTER2 LIMITED-TRUST ingress at $CLUSTER2_INGRESS:15443
cat samples/limited-trust/helloworld-binding.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -
```

To verify that it works, deploy experiment consumer

``` bash
kubectl --context $CLUSTER1 run --generator=run-pod/v1 cli1 --image tutum/curl --command -- bash -c 'sleep 9999999'
```

Then run a command on the consumer:

``` bash
kubectl --context $CLUSTER1 exec -it cli1 -- curl --silent helloworld:5000/hello
```

## The passthrough demo

``` bash
kubectl --context $CLUSTER1 create namespace passthrough
kubectl --context $CLUSTER2 create namespace passthrough
kubectl --context $CLUSTER1 apply -f samples/passthrough/passthrough-c1.yaml
kubectl --context $CLUSTER2 apply -f samples/passthrough/passthrough-c2.yaml
```

We need a service to deploy

``` bash
kubectl --context $CLUSTER2 apply -f samples/passthrough/holamundo.yaml
kubectl --context $CLUSTER2 wait --for=condition=available --timeout=60s deployment/holamundo-v1
```

(We could use the same expose service for both styles, but for testing let's keep them separate).

Next, we will expose the service

``` bash
kubectl --context $CLUSTER2 apply -f samples/passthrough/holamundo-expose.yaml
```

Next, we will bind to the Service

``` bash
CLUSTER2_INGRESS=$(kubectl --context $CLUSTER2 get svc -n istio-system istio-ingressgateway --output jsonpath="{.status.loadBalancer.ingress[0].ip}")
echo Using $CLUSTER2 ISTIO ingress at $CLUSTER2_INGRESS:15443
cat samples/passthrough/holamundo-binding.yaml | sed s/9.1.2.3:5000/$CLUSTER2_INGRESS:15443/ | kubectl --context $CLUSTER1 apply -f -
```

To verify that it works, we can use the same consumer as before.  Run a command on the consumer:

``` bash
kubectl --context $CLUSTER1 exec -it cli1 -- curl --silent holaworld:5000/hola
```

### Cleanup

``` bash
kubectl --context $CLUSTER1 delete ns limited-trust
kubectl --context $CLUSTER1 delete ns passthrough
kubectl --context $CLUSTER2 delete ns limited-trust
kubectl --context $CLUSTER2 delete ns passthrough
kubectl --context $CLUSTER1 delete servicebinding helloworld
kubectl --context $CLUSTER1 delete servicebinding holaworld
```

Then shut down the two controllers.
