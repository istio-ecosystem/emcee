// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package passthrough

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	types "github.com/gogo/protobuf/types"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/style"
	"istio.io/pkg/log"

	"github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Passthrough has clients for k8s and Istio
type Passthrough struct {
	client.Client
	istioclient.Interface
}

var (
	// (compile-time check that we implement the interface)
	_ style.MeshFedConfig  = &Passthrough{}
	_ style.ServiceBinder  = &Passthrough{}
	_ style.ServiceExposer = &Passthrough{}
)

const (
	//	defaultPrefix      = ".svc.cluster.local"
	defaultIngressPort = 15443 // the port used at the Ingress
)

// NewPassthroughMeshFedConfig creates a "Passthrough" style implementation for handling MeshFedConfig
func NewPassthroughMeshFedConfig(cli client.Client, istioCli istioclient.Interface) style.MeshFedConfig {
	return &Passthrough{
		cli,
		istioCli,
	}
}

// NewPassthroughServiceExposer creates a "Passthrough" style implementation for handling ServiceExposure
func NewPassthroughServiceExposer(cli client.Client, istioCli istioclient.Interface) style.ServiceExposer {
	return &Passthrough{
		cli,
		istioCli,
	}
}

// NewPassthroughServiceBinder creates a "Passthrough" style implementation for handling ServiceBinding
func NewPassthroughServiceBinder(cli client.Client, istioCli istioclient.Interface) style.ServiceBinder {
	return &Passthrough{
		cli,
		istioCli,
	}
}

// ***************************
// *** EffectMeshFedConfig ***
// ***************************

// EffectMeshFedConfig does not do anything for the passthrough mode
func (pt *Passthrough) EffectMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	return nil
}

// RemoveMeshFedConfig does not do anything for the passthrough mode
func (pt *Passthrough) RemoveMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	return nil
}

// *****************************
// *** EffectServiceExposure ***
// *****************************

// EffectServiceExposure ...
func (pt *Passthrough) EffectServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {

	eps, err := GetIngressEndpointsNoPort(ctx, pt.Client, "istio-ingressgateway", "istio-system", defaultIngressPort)
	if err != nil {
		log.Warnf("could not get endpoints %v %v", eps, err)
		return err
	}
	se.Spec.Endpoints = eps

	dr := passthroughExposingDestinationRule(mfc, se)
	_, err = createDestinationRule(pt.Interface, se.GetNamespace(), dr)
	if err != nil {
		log.Warnf("Could not created the Destination Rule %v: %v", dr.GetName(), err)
	}

	gw, _ := passthroughExposingGateway(mfc, se)
	_, err = createGateway(pt.Interface, se.GetNamespace(), gw)
	if err != nil {
		log.Warnf("Could not created the Gateway %v: %v", gw.GetName(), err)
	}

	vs, _ := passthroughExposingVirtualService(mfc, se)
	_, err = createVirtualService(pt.Interface, se.GetNamespace(), vs)
	if err != nil {
		log.Warnf("Could not created the Virtual Service %v: %v", vs.GetName(), err)
	}

	se.Status.Ready = true
	if err := pt.Client.Update(ctx, se); err != nil {
		return err
	}

	return nil
}

// RemoveServiceExposure ...
func (pt *Passthrough) RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {
	return nil
}

// ****************************
// *** EffectServiceBinding ***
// ****************************

// EffectServiceBinding ...
func (pt *Passthrough) EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {

	serviceEntry := passthroughBindingServiceEntry(mfc, sb)
	_, err := createServiceEntry(pt.Interface, sb.GetNamespace(), serviceEntry)
	if err != nil {
		log.Warnf("Could not created the Service Entry %v: %v", serviceEntry.GetName(), err)
	}

	goalSvc := passthroughBindingService(sb, mfc)
	if goalSvc == nil {
		log.Infof("Could not generate Remote Cluster ingress Service")
		return fmt.Errorf("passthrough controller could not generate Remote Cluster ingress Service")
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      goalSvc.GetName(),
			Namespace: goalSvc.GetNamespace(),
		},
	}
	or, err := controllerutil.CreateOrUpdate(ctx, pt.Client, svc, func() error {
		svc.ObjectMeta.Labels = goalSvc.Labels
		svc.ObjectMeta.OwnerReferences = goalSvc.ObjectMeta.OwnerReferences
		// Update the Spec fields WITHOUT clearing svc.Spec.ClusterIP
		svc.Spec.Ports = goalSvc.Spec.Ports
		svc.Spec.SessionAffinity = goalSvc.Spec.SessionAffinity
		svc.Spec.Type = goalSvc.Spec.Type
		return nil
	})
	if err != nil {
		return err
	}

	dr := passthroughBindingDestinationRule(mfc, sb)
	_, err = createDestinationRule(pt.Interface, sb.GetNamespace(), dr)
	if err != nil {
		log.Warnf("Could not created the Destination Rule %v: %v", dr.GetName(), err)
	}

	log.Infof("%s %s %s", or,
		"Remote Cluster ingress Service",
		renderName(&svc.ObjectMeta))

	return nil
}

// RemoveServiceBinding ...
func (pt *Passthrough) RemoveServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {
	return nil
}

// *****************************
// *****************************
// *****************************

//func getIngressPort(mfc *mmv1.MeshFedConfig) uint32 {
//	if mfc.Spec.IngressGatewayPort == 0 {
//		return defaultIngressPort
//	}
//	return mfc.Spec.IngressGatewayPort
//}

func passthroughExposingDestinationRule(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceExposition) *v1alpha3.DestinationRule {
	if !mfc.Spec.UseIngressGateway {
		return nil
	}

	name := se.Spec.Name
	namespace := se.GetNamespace()
	svcName := fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace)

	return &v1alpha3.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			Kind: "DestinationRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceExposeName(mfc.GetName(), se.GetName()),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(se.APIVersion, se.Kind, se.ObjectMeta),
		},
		Spec: v1alpha3.DestinationRuleSpec{
			DestinationRule: istiov1alpha3.DestinationRule{
				Host: svcName,
				TrafficPolicy: &istiov1alpha3.TrafficPolicy{
					Tls: &istiov1alpha3.TLSSettings{
						Mode: istiov1alpha3.TLSSettings_ISTIO_MUTUAL,
					},
				},
				Subsets: []*istiov1alpha3.Subset{
					&istiov1alpha3.Subset{
						Name: "notls",
						TrafficPolicy: &istiov1alpha3.TrafficPolicy{
							Tls: &istiov1alpha3.TLSSettings{
								Mode: istiov1alpha3.TLSSettings_DISABLE,
							},
						},
					},
				},
			},
		},
	}
}

func passthroughExposingGateway(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceExposition) (*v1alpha3.Gateway, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, fmt.Errorf("passthrough requires Ingress Gateway")
	}
	portToListen := getPortfromIPPort(se.Spec.Endpoints[0])
	if portToListen == 0 {
		return nil, fmt.Errorf("passthrough requires a port number for Ingress Gateway")
	}
	return &v1alpha3.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind: "Gateway",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceExposeName(mfc.GetName(), se.GetName()),
			Namespace: se.GetNamespace(), // TODO
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(se.APIVersion, se.Kind, se.ObjectMeta),
		},
		Spec: v1alpha3.GatewaySpec{
			Gateway: istiov1alpha3.Gateway{
				Servers: []*istiov1alpha3.Server{
					&istiov1alpha3.Server{
						// Hosts: []string{fmt.Sprintf("%s.%s.svc.cluster.local", se.Spec.Name, se.GetNamespace())}, // TODO intermeshNamespace //MB
						Hosts: []string{"*.svc.cluster.local"},
						Port: &istiov1alpha3.Port{
							Number:   portToListen,
							Protocol: "TLS",
							Name:     "tls", //MB se.Spec.Name,
						},
						Tls: &istiov1alpha3.Server_TLSOptions{
							Mode: istiov1alpha3.Server_TLSOptions_AUTO_PASSTHROUGH, //MB
						},
					},
				},
				Selector: mfc.Spec.IngressGatewaySelector, // TODO: default to: map[string]string {"istio": "ingressgateway}",
			},
		},
	}, nil
}

func passthroughExposingVirtualService(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceExposition) (*v1alpha3.VirtualService, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, fmt.Errorf("passthrough requires Ingress Gateway")
	}
	portToListen := getPortfromIPPort(se.Spec.Endpoints[0])
	if portToListen == 0 {
		return nil, fmt.Errorf("passthrough requires a port number for Ingress Gateway")
	}

	return &v1alpha3.VirtualService{
		TypeMeta: metav1.TypeMeta{
			Kind: "VirtualService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("intermesh-%s-%s", se.Spec.Name, se.GetNamespace()),
			Namespace: se.GetNamespace(),
			Labels: map[string]string{
				"mesh": mfc.GetName(),
				"role": "external",
			},
			OwnerReferences: ownerReference(se.APIVersion, se.Kind, se.ObjectMeta),
		},
		Spec: v1alpha3.VirtualServiceSpec{
			VirtualService: istiov1alpha3.VirtualService{
				Hosts:    []string{"*"}, // fmt.Sprintf("%s.%s.svc.cluster.local", exposedLocalName(se), se.GetNamespace())}, // TODO Why need the "*"?
				Gateways: []string{serviceExposeName(mfc.GetName(), se.GetName())},
				Tls: []*istiov1alpha3.TLSRoute{
					{
						Match: []*istiov1alpha3.TLSMatchAttributes{
							&istiov1alpha3.TLSMatchAttributes{
								Port:     portToListen,
								SniHosts: []string{fmt.Sprintf("%s.%s.svc.cluster.local", exposedLocalName(se), se.GetNamespace())},
							},
						},
						Route: []*istiov1alpha3.RouteDestination{
							{
								Destination: &istiov1alpha3.Destination{
									Host: fmt.Sprintf("%s.%s.svc.cluster.local", se.Spec.Name, se.GetNamespace()),
									Port: &istiov1alpha3.PortSelector{
										Number: se.Spec.Port,
									},
									Subset: "notls",
								},
							},
						},
					},
				},
			},
		},
	}, nil
}

func getPortfromIPPort(ep string) uint32 {
	parts := strings.Split(ep, ":")
	numparts := len(parts)
	if numparts != 2 {
		log.Warnf("Address %q not in form ip:port", ep)
		return 0
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	return uint32(port)
}

func passthroughBindingServiceEntry(mfc *mmv1.MeshFedConfig, sb *mmv1.ServiceBinding) *v1alpha3.ServiceEntry {
	if !mfc.Spec.UseIngressGateway {
		return nil
	}
	name := boundLocalName(sb)
	namespace := sb.Spec.Namespace
	port := boundLocalPort(sb)

	parts := strings.Split(sb.Spec.Endpoints[0], ":")
	numparts := len(parts)
	if numparts != 2 {
		log.Warnf("Address %q not in form ip:port", sb.Spec.Endpoints[0])
		return nil
	}
	epPort, err := strconv.Atoi(parts[1])
	if err != nil {
		return nil
	}
	epAddress := parts[0]

	return &v1alpha3.ServiceEntry{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceEntry",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceRemoteName(mfc.GetName(), name),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.ServiceEntrySpec{
			ServiceEntry: istiov1alpha3.ServiceEntry{
				Hosts: []string{
					// Note that this must be the local name, even if a DestinationRule has altered the SNI (tested in Istio 1.4.0)
					fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace), // TODO intermeshNamespace
				},
				Ports: []*istiov1alpha3.Port{
					&istiov1alpha3.Port{
						Name:     "http",
						Number:   port,
						Protocol: "HTTP",
					},
				},
				Resolution: istiov1alpha3.ServiceEntry_STATIC,
				Location:   istiov1alpha3.ServiceEntry_MESH_INTERNAL, //MB
				Endpoints: []*istiov1alpha3.ServiceEntry_Endpoint{
					&istiov1alpha3.ServiceEntry_Endpoint{
						Address: epAddress,
						Ports: map[string]uint32{
							"http": uint32(epPort),
						},
						Locality: "us-north/007", // TODO use locality provided in discovery
						Network: "NorthStar",
					},
				},
			},
		},
	}
}

func passthroughBindingDestinationRule(mfc *mmv1.MeshFedConfig, sb *mmv1.ServiceBinding) *v1alpha3.DestinationRule {
	if !mfc.Spec.UseIngressGateway {
		return nil
	}

	// name := sb.Spec.Name //MB
	namespace := sb.Spec.Namespace
	// svcName := fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace) //MB           // TODO intermeshNamespace
	svcLocalName := fmt.Sprintf("%s.%s.svc.cluster.local", boundLocalName(sb), namespace) // TODO intermeshNamespace

	return &v1alpha3.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			Kind: "DestinationRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceRemoteName(mfc.GetName(), sb.GetName()),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.DestinationRuleSpec{
			DestinationRule: istiov1alpha3.DestinationRule{
				Host: svcLocalName,
				TrafficPolicy: &istiov1alpha3.TrafficPolicy{
					PortLevelSettings: []*istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
						&istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
							Port: &istiov1alpha3.PortSelector{
								Number: boundLocalPort(sb),
							},
							//MB
							ConnectionPool: &istiov1alpha3.ConnectionPoolSettings{
								Http: &istiov1alpha3.ConnectionPoolSettings_HTTPSettings{
									Http2MaxRequests:         1000,
									MaxRequestsPerConnection: 10,
								},
								Tcp: &istiov1alpha3.ConnectionPoolSettings_TCPSettings{
									MaxConnections: 100,
								},
							},
							OutlierDetection: &istiov1alpha3.OutlierDetection{
								BaseEjectionTime: &types.Duration{
									Seconds: 20,
								},
								ConsecutiveErrors: 2,
								Interval: &types.Duration{
									Seconds: 5,
								},
								MaxEjectionPercent: 75,
							},
							Tls: &istiov1alpha3.TLSSettings{
								Mode: istiov1alpha3.TLSSettings_ISTIO_MUTUAL, //MB
								//ClientCertificate: certificatesDir + "cert-chain.pem",
								//PrivateKey:        certificatesDir + "key.pem",
								//CaCertificates:    certificatesDir + "root-cert.pem",
								//Sni:               svcName, // intermeshNamespace ,
							},
						},
					},
				},
			},
		},
	}
}

func passthroughBindingService(sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) *corev1.Service {
	if !mfc.Spec.UseIngressGateway {
		return nil
	}
	name := boundLocalName(sb) // TODO need this? serviceIntermeshName(sb.Spec.Name)
	port := boundLocalPort(sb)
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: sb.GetNamespace(),
			Labels: map[string]string{
				"mesh": sb.Spec.Name,
				"role": "ingress-svc",
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "http",
					Port: int32(port),
					// TargetPort: intstr.FromInt(0),
				},
			},
		},
	}
}

func serviceRemoteName(mfcName, svcName string) string {
	return fmt.Sprintf("binding-%s-%s-intermesh", mfcName, svcName)
}

func serviceExposeName(mfcName, svcName string) string {
	return fmt.Sprintf("exposition-%s-%s-intermesh", mfcName, svcName)
}

//func serviceIntermeshName(name string) string {
//	return fmt.Sprintf("%s-intermesh", name)
//}

func boundLocalPort(sb *mmv1.ServiceBinding) uint32 {
	if sb.Spec.Port != 0 {
		return sb.Spec.Port
	}
	return 80
}

func renderName(om *metav1.ObjectMeta) string {
	return fmt.Sprintf("%s.%s", om.GetName(), om.GetNamespace())
}

func boundLocalName(sb *mmv1.ServiceBinding) string {
	if sb.Spec.Alias != "" {
		return sb.Spec.Alias
	}
	return sb.Spec.Name
}

func exposedLocalName(se *mmv1.ServiceExposition) string {
	if se.Spec.Alias != "" {
		return se.Spec.Alias
	}
	return se.Spec.Name
}
