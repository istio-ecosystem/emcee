# Developer and test instructions

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
