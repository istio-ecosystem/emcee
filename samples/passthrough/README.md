# Samples: Passthrough

Samples to be used demonstrating mc2019

## Developer instructions

### Create Secrets

https://istio.io/docs/tasks/security/citadel-config/plugin-ca-cert/

```bash
kubectl --context $CLUSTER1 create secret generic cacerts -n istio-system --from-file=samples/certs/ca-cert.pem    --from-file=samples/certs/ca-key.pem --from-file=samples/certs/root-cert.pem --from-file=samples/certs/cert-chain.pem
istioctl --context $CLUSTER1  manifest apply --set values.global.mtls.enabled=true,values.security.selfSigned=false
kubectl --context $CLUSTER1  delete secret istio.default


kubectl --context $CLUSTER2 create secret generic cacerts -n istio-system --from-file=samples/certs/ca-cert.pem     --from-file=samples/certs/ca-key.pem --from-file=samples/certs/root-cert.pem --from-file=samples/certs/cert-chain.pem
istioctl --context $CLUSTER2  manifest apply --set values.global.mtls.enabled=true,values.security.selfSigned=false
kubectl --context $CLUSTER2  delete secret istio.default
```