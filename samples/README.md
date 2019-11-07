# Samples

Samples to be used demonstrating mc2019

TODO: Merge with config/samples

## Developer instructions

Regenerate based on [Vadim's mutual TLS setup instructions](https://github.com/istio-ecosystem/multi-mesh-examples/tree/master/add_hoc_limited_trust/common-setup#prerequisites-for-three-clusters).

To create the secrets

``` bash
kubectl create secret generic limited-trust --from-file example.com.crt=<(cat example.com.crt | base64),tls.crt=<(cat c1.example.com.crt | base64),tls.key=<(cat c1.example.com.key | base64) --dry-run=true --output=yaml > secret-c1.yaml
kubectl create secret generic limited-trust --from-file example.com.crt=<(cat example.com.crt | base64),tls.crt=<(cat c2.example.com.crt | base64),tls.key=<(cat c2.example.com.key | base64) --dry-run=true --output=yaml > secret-c2.yaml
```
