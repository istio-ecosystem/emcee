// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package passthrough

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
	"github.ibm.com/istio-research/mc2019/style"
	"istio.io/pkg/log"

	"github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Passthrough struct {
	cli      client.Client
	istioCli istioclient.Interface
}

var (
	// (compile-time check that we implement the interface)
	_ style.MeshFedConfig  = &Passthrough{}
	_ style.ServiceBinder  = &Passthrough{}
	_ style.ServiceExposer = &Passthrough{}
)

const (
	defaultPrefix      = ".svc.cluster.local"
	defaultIngressPort = 15443
)

// NewPassthroughMeshFedConfig creates a "Passthrough" style implementation for handling MeshFedConfig
func NewPassthroughMeshFedConfig(cli client.Client, istioCli istioclient.Interface) style.MeshFedConfig {
	return &Passthrough{
		cli:      cli,
		istioCli: istioCli,
	}
}

// NewPassthroughServiceExposer creates a "Passthrough" style implementation for handling ServiceExposure
func NewPassthroughServiceExposer(cli client.Client, istioCli istioclient.Interface) style.ServiceExposer {
	return &Passthrough{
		cli:      cli,
		istioCli: istioCli,
	}
}

// NewPassthroughServiceBinder creates a "Passthrough" style implementation for handling ServiceBinding
func NewPassthroughServiceBinder(cli client.Client, istioCli istioclient.Interface) style.ServiceBinder {
	return &Passthrough{
		cli:      cli,
		istioCli: istioCli,
	}
}

// ***************************
// *** EffectMeshFedConfig ***
// ***************************

// EffectMeshFedConfig ...
func (bp *Passthrough) EffectMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	return nil
}

// RemoveMeshFedConfig ...
func (bp *Passthrough) RemoveMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	return nil
}

// *****************************
// *** EffectServiceExposure ***
// *****************************

// EffectServiceExposure ...
func (bp *Passthrough) EffectServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {

	eps, err := GetIngressEndpointsNoPort(ctx, bp.cli, "istio-ingressgateway", "istio-system", defaultIngressPort)
	if err != nil {
		log.Warnf("could not get endpoints %v %v", eps, err)
		return err
	}
	se.Spec.Endpoints = eps

	dr := passthroughExposingDestinationRule(mfc, se)
	log.Infof("=================================== dr: %v", dr)
	a, b := createDestinationRule(bp.istioCli, se.GetNamespace(), dr)
	log.Infof("................................... dr: %v %v", a, b)

	gw, _ := passthroughExposingGateway(mfc, se)
	log.Infof("=================================== gw: %v", gw)
	c, d := createGateway(bp.istioCli, se.GetNamespace(), gw)
	log.Infof("................................... gw: %v %v", c, d)

	vs, _ := passthroughExposingVirtualService(mfc, se)
	log.Infof("=================================== vs: %v", vs)
	e, f := createVirtualService(bp.istioCli, se.GetNamespace(), vs)
	log.Infof("................................... vs: %v %v", e, f)

	// get the endpoints // TODO
	// eps, err := GetIngressEndpointsNoPort(ctx, bp.cli, mfc.GetName(), mfc.GetNamespace())

	se.Status.Ready = true
	if err := bp.cli.Update(ctx, se); err != nil {
		return err
	}

	return nil
}

// RemoveServiceExposure ...
func (bp *Passthrough) RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - service exposure delete")
}

// ****************************
// *** EffectServiceBinding ***
// ****************************

// EffectServiceBinding ...
func (bp *Passthrough) EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {

	serviceEntry := passthroughBindingServiceEntry(mfc, sb)
	log.Infof("=================================== se: %v", serviceEntry)
	a, b := createServiceEntry(bp.istioCli, sb.GetNamespace(), serviceEntry)
	log.Infof("................................... vs: %v %v", a, b)

	dr := passthroughBindingDestinationRule(mfc, sb)
	log.Infof("=================================== dr: %v", dr)
	c, d := createDestinationRule(bp.istioCli, sb.GetNamespace(), dr)
	log.Infof("................................... vs: %v %v", c, d)
	// Create a Kubernetes service
	svc := passthroughBindingService(sb, mfc)
	log.Infof("=================================== svc: %v", svc)
	if svc == nil {
		log.Infof("Could not generate Remote Cluster ingress Service")
		return fmt.Errorf("passthrough controller could not generate Remote Cluster ingress Service")
	}
	err := bp.cli.Create(ctx, svc)
	log.Infof("................................... svc: %v", err)
	if err := logAndCheckExistAndUpdate(ctx, bp, svc, err, "Remote Cluster ingress Service", "TODO"); err != nil {
		return err
	}

	return nil
}

// RemoveServiceBinding ...
func (bp *Passthrough) RemoveServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - service binding delete")
}

// *****************************
// *****************************
// *****************************

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
						Hosts: []string{fmt.Sprintf("%s.%s.svc.cluster.local", se.Spec.Name, se.GetNamespace())}, // TODO intermeshNamespace
						Port: &istiov1alpha3.Port{
							Number:   portToListen,
							Protocol: "TLS",
							Name:     se.Spec.Name,
						},
						Tls: &istiov1alpha3.Server_TLSOptions{
							Mode: istiov1alpha3.Server_TLSOptions_PASSTHROUGH,
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
				Hosts:    []string{fmt.Sprintf("%s.%s.svc.cluster.local", se.Spec.Name, se.GetNamespace())}, // TODO intermeshNamespace
				Gateways: []string{serviceExposeName(mfc.GetName(), se.GetName())},
				Tls: []*istiov1alpha3.TLSRoute{
					{
						Match: []*istiov1alpha3.TLSMatchAttributes{
							&istiov1alpha3.TLSMatchAttributes{
								Port:     portToListen,
								SniHosts: []string{fmt.Sprintf("%s.%s.svc.cluster.local", se.Spec.Name, se.GetNamespace())}, // TODO intermeshNamespace
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
	name := sb.Spec.Name
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
			Name:      serviceRemoteName(mfc.GetName(), sb.GetName()),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.ServiceEntrySpec{
			ServiceEntry: istiov1alpha3.ServiceEntry{
				Hosts: []string{
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
				Location:   istiov1alpha3.ServiceEntry_MESH_EXTERNAL,
				Endpoints: []*istiov1alpha3.ServiceEntry_Endpoint{
					&istiov1alpha3.ServiceEntry_Endpoint{
						Address: epAddress,
						Ports: map[string]uint32{
							"http": uint32(epPort),
						},
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

	name := sb.Spec.Name
	namespace := sb.Spec.Namespace
	svcName := fmt.Sprintf("%s.%s.svc.cluster.local", name, namespace) // TODO intermeshNamespace

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
				Host: svcName,
				TrafficPolicy: &istiov1alpha3.TrafficPolicy{
					PortLevelSettings: []*istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
						&istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
							Port: &istiov1alpha3.PortSelector{
								Number: boundLocalPort(sb),
							},
							Tls: &istiov1alpha3.TLSSettings{
								Mode:              istiov1alpha3.TLSSettings_MUTUAL,
								ClientCertificate: certificatesDir + "cert-chain.pem",
								PrivateKey:        certificatesDir + "key.pem",
								CaCertificates:    certificatesDir + "root-cert.pem",
								Sni:               svcName, // intermeshNamespace ,
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
	name := sb.Spec.Name // TODO need this? serviceIntermeshName(sb.Spec.Name)
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

func serviceIntermeshName(name string) string {
	return fmt.Sprintf("%s-intermesh", name)
}

func boundLocalPort(sb *mmv1.ServiceBinding) uint32 {
	if sb.Spec.Port != 0 {
		return sb.Spec.Port
	}
	return 80
}
