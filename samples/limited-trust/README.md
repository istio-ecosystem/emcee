# Samples

Samples to be used demonstrating mc2019

TODO: Merge with config/samples

## Developer instructions

Regenerate based on [Vadim's mutual TLS setup instructions](https://github.com/istio-ecosystem/multi-mesh-examples/tree/master/add_hoc_limited_trust/common-setup#prerequisites-for-three-clusters).

To create the secrets

```bash
cat <<EOF > secret-c1.yaml
apiVersion: v1
kind: Secret
metadata:
  name: limited-trust
  namespace: limited-trust
  labels:
    mesh: limited-trust
    secret: cluster1
type: kubernetes.io/tls
data:
  example.com.crt: `cat example.com.crt | base64`
  tls.crt: `cat c1.example.com.crt | base64`
  tls.key: `cat c1.example.com.key | base64`
EOF
cat <<EOF > secret-c2.yaml
apiVersion: v1
kind: Secret
metadata:
  name: limited-trust
  namespace: limited-trust
  labels:
    mesh: limited-trust
    secret: cluster2
type: kubernetes.io/tls
data:
  example.com.crt: `cat example.com.crt | base64`
  tls.crt: `cat c2.example.com.crt | base64`
  tls.key: `cat c2.example.com.key | base64`
EOF
```

If you need to download copies of the certs:

``` bash
kubectl --context $CLUSTER1 -n limited-trust get secret limited-trust -o jsonpath="{.data.example\.com\.crt}" | base64 -D > example.com.crt
kubectl --context $CLUSTER1 -n limited-trust get secret limited-trust -o jsonpath="{.data.tls\.crt}" | base64 -D > c1.example.com.crt
kubectl --context $CLUSTER1 -n limited-trust get secret limited-trust -o jsonpath="{.data.tls\.key}" | base64 -D > c1.example.com.key
kubectl --context $CLUSTER2 -n limited-trust get secret limited-trust -o jsonpath="{.data.tls\.crt}" | base64 -D > c2.example.com.crt
kubectl --context $CLUSTER2 -n limited-trust get secret limited-trust -o jsonpath="{.data.tls\.key}" | base64 -D > c2.example.com.key
```

To compare the keys

``` bash
INGRESS_POD=$(kubectl --context $CLUSTER2 -n limited-trust get pod -l istio=ingressgateway -o jsonpath='{.items..metadata.name}')
echo Ingress on $CLUSTER2 is $INGRESS_POD
EGRESS_POD=$(kubectl --context $CLUSTER1 -n limited-trust get pod -l istio=egressgateway -o jsonpath='{.items..metadata.name}')
echo Egress on $CLUSTER1 is $EGRESS_POD
diff <(kubectl --context $CLUSTER2 -n limited-trust exec $INGRESS_POD -- cat /etc/istio/mesh/certs/example.com.crt) example.com.crt
diff <(kubectl --context $CLUSTER1 -n limited-trust exec $EGRESS_POD -- cat /etc/istio/mesh/certs/example.com.crt) example.com.crt
```
