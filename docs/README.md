# Emcee: A "master of ceremonies" for Istio multi mesh

The goal of Emcee is to provide a high level way to expose and consume microservices
in an Istio environment.

The design goal of Emcee is to be able to enter into multiple relationships with
other meshes, with independent security configuration, and selectively expose,
consume, and discover service endpoints.

## Data plane styles

Eventually, we intend to support four different data plane styles.  As of December 2019
we support the 'compliance' and 'ingress passthrough' styles.

![Data Plane Styles](data-plane.png?raw=true "Data Plane Styles")

## Multi-mesh relationship characteristics

Some multi-mesh relationships are fast and automatic.  Others involve off-line manual processes.

![Mesh Connection Styles](connection.png?raw=true "Mesh Connection Styles")

## Service Discovery

Within a mesh, emcee “announces” exposed endpoints.  This is highly selective.

![selective discoverability](discoverability.png?raw=true "Selective discoverability")

- Emcee knows that M2 exposes using terminating Ingress
- Emcee knows that “reviews” is public
- Emcee sends descriptions of “reviews” and “emcess” to clients, probably using the mechanisms of Nathan Mittler’s Service Discovery API RFC
  - an EndpointSlice to describe how to talk to reviews and emcee.
    - Alternative: OpenAPI
  - Wrapped using Envoy incremental XDS but with EndpointSlice Delta instead of Cluster Delta
    - Alternative: Istio MCP
- Emcess doesn’t announce some services, e.g. debugging httpbin, even though it has a VirtualService on the multi-mesh Ingress

## Multiple mesh relationships

![multiple relationships](n-meshes.png?raw=true "Multiple relationships")

- Microservice on m1 consumes from m2 on the blue group and m4 on the yellow group.
- The blue group uses an isolation style
- The yellow group uses flat networking
- Blue and yellow might be on different physical networks
- A cluster may have >1 multimesh Ingress
  - This doesn’t come up in passthrough, because all clients need the same TLS credentials.  In the terminating case it adds flexibility with different trust relationships.

## Three concepts: mesh-config, expose, bind

![Three concepts](3-concepts.png?raw=true "Three concepts")

### Mesh-config experience

(Boundary Protection Style)

User creates namespace, MeshFedConfig, and Secret (typically by using a script that creates a pair of Secrets).

Emcee controller creates Ingress and Egress if necessary.  The user can select an existing Ingress and Egress if customization is needed.

``` YAML
apiVersion: v1
kind: Namespace
metadata:
name: limited-trust
```

``` YAML
apiVersion: mm.ibm.istio.io/v1
kind: MeshFedConfig
metadata:
  name: limited-trust
  namespace: limited-trust
spec:
  mode: ”BOUNDARY”
  tls_context_selector:
    mesh: limited-trust
    secret: cluster1
  use_egress_gateway: true
  egress_gateway_selector:
    istio: egressgateway
  egress_gateway_port: 443
  use_ingress_gateway: true
  ingress_gateway_selector:
    istio: ingressgateway
  ingress_gateway_port: 15443
```

``` YAML
apiVersion: v1
kind: Secret
metadata:
  name: c1-example-com-certs
  namespace: limited-trust
  labels:
    mesh: limited-trust
    secret: cluster1
type: kubernetes.io/tls
data:
  # Public key
  # Issuer: O=example Inc., CN=example.com
  # Subject: O=example Inc., CN=c1.example.com
  # Expires Nov  6 16:11:14 2020 GMT
  tls.crt: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JS…
  # Private key matching the above
  tls.key: LS0tLS1CRUdJTiBQUklWQVRFIEt0tLS0tCk1JSLY5Lb…  # Root cert
  # Issuer: O=example Inc., CN=example.com
  # Subject: O=example Inc., CN=example.com
  # Expires Nov  6 16:11:14 2020 GMT
  example.com.crt: LS0tLS1CRUdJTiBDRVJUSUZJQ0FUR…
```

### Expose experience

``` YAML
apiVersion: mm.ibm.istio.io/v1
kind: ServiceExposition
metadata:
  name: se1
spec:
  mesh_fed_config_selector:
    fed-config: limited-trust
  name:	helloworld
  subset: … (optional)
  alias: … (optional)
  port:	5000
  directory: true (optional)
```

### Bind experience

``` YAML
apiVersion: mm.ibm.istio.io/v1
kind: ServiceBinding
metadata:
  name: helloworld
spec:
  name: helloworld
  namespace: default
  mesh_fed_config_selector:
    fed-config: limited-trust
  endpoints:
  - "9.1.2.3:5000"  # Can come through discovery
```
