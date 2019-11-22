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
	"k8s.io/apimachinery/pkg/util/intstr"
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
	defaultPrefix = ".svc.cluster.local"
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
	dr := passthroughExposingDestinationRule(mfc, se)
	_, _ = createDestinationRule(bp.istioCli, mfc.GetNamespace(), dr)

	gw, _ := passthroughExposingGateway(mfc, se)
	_, _ = createGateway(bp.istioCli, mfc.GetNamespace(), gw)

	vs, _ := passthroughExposingVirtualService(mfc, se)
	_, _ = createVirtualService(bp.istioCli, mfc.GetNamespace(), vs)

	return nil
}

// RemoveServiceExposure ...
func (bp *Passthrough) RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - service exposure delete")
}

func passthroughExposingDestinationRule(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceExposition) *v1alpha3.DestinationRule {
	if !mfc.Spec.UseIngressGateway {
		return nil
	}
	return nil
}

func passthroughExposingGateway(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceExposition) (*v1alpha3.Gateway, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, fmt.Errorf("passthrough requires Ingress Gateway")
	}
	return nil, fmt.Errorf("passthroughExposingGateway not implemented yet")
}

func passthroughExposingVirtualService(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceExposition) (*v1alpha3.VirtualService, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, fmt.Errorf("passthrough requires Ingress Gateway")
	}
	return nil, fmt.Errorf("passthroughExposingVirtualService not implemented yet")
}

// ****************************
// *** EffectServiceBinding ***
// ****************************

// EffectServiceBinding ...
func (bp *Passthrough) EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {

	serviceEntry := passthroughBindingServiceEntry(mfc, sb)
	log.Infof("******** %v", serviceEntry)
	_, _ = createServiceEntry(bp.istioCli, sb.GetNamespace(), serviceEntry)

	dr := passthroughBindingDestinationRule(mfc, sb)
	a, b := createDestinationRule(bp.istioCli, sb.GetNamespace(), dr)
	log.Infof("******************** %v %v", a, b)

	// Create a Kubernetes service
	svc := passthroughBindingService(sb, mfc)
	if svc == nil {
		log.Infof("Could not generate Remote Cluster ingress Service")
		return fmt.Errorf("passthrough controller could not generate Remote Cluster ingress Service")
	}
	err := bp.cli.Create(ctx, svc)
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
			Name:      serviceRemoteName(mfc, sb),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.ServiceEntrySpec{
			ServiceEntry: istiov1alpha3.ServiceEntry{
				Hosts: []string{
					fmt.Sprintf("%s.%s", name, namespace),
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
	svcName := fmt.Sprintf("%s.%s", name, namespace)

	return &v1alpha3.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			Kind: "DestinationRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceRemoteName(mfc, sb),
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
								Sni:               svcName,
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
	name := serviceIntermeshName(sb.Spec.Name)
	port := boundLocalPort(sb)
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("istio-%s-ingress-%d", name, port),
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
					Name:       "http",
					Port:       int32(port),
					TargetPort: intstr.FromInt(0),
				},
			},
		},
	}
}

func serviceRemoteName(mfc *mmv1.MeshFedConfig, sb *mmv1.ServiceBinding) string {
	return fmt.Sprintf("binding-%s-%s-intermesh", mfc.GetName(), sb.GetName())
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
