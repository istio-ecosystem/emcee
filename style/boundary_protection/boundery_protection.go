// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package boundary_protection

import (
	"context"
	"fmt"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
	"github.ibm.com/istio-research/mc2019/style"
	mfutil "github.ibm.com/istio-research/mc2019/util"

	"github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/pkg/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type bounderyProtection struct {
	cli      client.Client
	istioCli istioclient.Interface
}

var (
	// (compile-time check that we implement the interface)
	_ style.MeshFedConfig  = &bounderyProtection{}
	_ style.ServiceBinder  = &bounderyProtection{}
	_ style.ServiceExposer = &bounderyProtection{}
)

const (
	DEFAULT_PREFIX = ".svc.cluster.local"
)

// NewBoundaryProtectionMeshFedConfig creates a "Boundary Protection" style implementation for handling MeshFedConfig
func NewBoundaryProtectionMeshFedConfig(cli client.Client, istioCli istioclient.Interface) style.MeshFedConfig {
	return &bounderyProtection{
		cli:      cli,
		istioCli: istioCli,
	}
}

// NewBoundaryProtectionServiceExposer creates a "Boundary Protection" style implementation for handling ServiceExposure
func NewBoundaryProtectionServiceExposer(cli client.Client, istioCli istioclient.Interface) style.ServiceExposer {
	return &bounderyProtection{
		cli:      cli,
		istioCli: istioCli,
	}
}

// NewBoundaryProtectionServiceBinder creates a "Boundary Protection" style implementation for handling ServiceBinding
func NewBoundaryProtectionServiceBinder(cli client.Client, istioCli istioclient.Interface) style.ServiceBinder {
	return &bounderyProtection{
		cli:      cli,
		istioCli: istioCli,
	}
}

// Implements Vadim-style
func (bp *bounderyProtection) EffectMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	// If the MeshFedConfig changes we may need to re-create all of the Istio
	// things for every ServiceBinding and ServiceExposition.  TODO Trigger
	// re-reconcile of every ServiceBinding and ServiceExposition.

	nsMC := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind: "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("istio-%s", mfc.GetName()),
		},
	}

	// Don't try to create the namespace if it already exists
	var nsDummy corev1.Namespace
	err := bp.cli.Get(ctx, client.ObjectKey{Name: nsMC.GetName()}, &nsDummy)
	if err != nil {
		err := bp.cli.Create(ctx, &nsMC)
		if err == nil {
			log.Infof("Created Namespace %q to hold Ingress/Egress", nsMC.GetName())
		} else {
			log.Infof("Failed to create Namespace %q: %v", nsMC.GetName(), err)
			return err
		}
	}

	// TODO Create Secrets.  HOW?  I lack the data to create them with
	log.Warnf("TODO: Create Secrets")

	// TODO Do I need to check UseEgressGateway?  This handler should not get any MeshFedConfig without it...
	if mfc.Spec.UseEgressGateway {
		// Create Egress Service if it doesn't already exist
		egressSvc := corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind: "Service",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("istio-%s-egress-%d", mfc.GetName(), mfc.Spec.EgressGatewayPort),
				Namespace: nsMC.GetName(),
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						// TODO ServicePort.Port is a uint32, EgressGatewayPort should be too?
						Port:       int32(mfc.Spec.EgressGatewayPort),
						TargetPort: intstr.FromInt(int(mfc.Spec.EgressGatewayPort)),
					}, // TODO the other ports?  How do we know which ports?  How do we know
					// the Egress port is HTTP?
				},
				Selector: mfc.Spec.EgressGatewaySelector,
			},
		}
		err = mfutil.ApplyService(ctx, bp.cli, &egressSvc)
		if err != nil {
			return err
		}

		// TODO Create Egress ServiceAccount
		// TODO Create Egress Deployment
	}

	// TODO Do I need to check UseIngressGateway?  This handler should not get any MeshFedConfig without it...
	if mfc.Spec.UseIngressGateway {
		// Create Ingress Service if it doesn't already exist
		ingressSvc := corev1.Service{
			TypeMeta: metav1.TypeMeta{
				Kind: "Service",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("istio-%s-ingress-%d", mfc.GetName(), mfc.Spec.IngressGatewayPort),
				Namespace: nsMC.GetName(),
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{
					{
						Name: "https-for-cross-cluster-communication",
						// TODO ServicePort.Port is a uint32, IngressGatewayPort should be too
						// TODO How do we know if the IngressGatewayPort becomes the https port or the tls port?
						Port:       int32(mfc.Spec.IngressGatewayPort),
						TargetPort: intstr.FromInt(int(mfc.Spec.IngressGatewayPort)),
					},
					{
						Name:       "tls-for-cross-cluster-communication",
						Port:       15444,
						TargetPort: intstr.FromInt(15443),
					},
					{
						Name:       "tcp-1",
						Port:       31400,
						TargetPort: intstr.FromInt(31400),
					},
					{
						Name:       "tcp-2",
						Port:       31401,
						TargetPort: intstr.FromInt(31401),
					},
				},
				Selector: mfc.Spec.IngressGatewaySelector,
			},
		}
		err = mfutil.ApplyService(ctx, bp.cli, &ingressSvc)
		if err != nil {
			return err
		}

		// TODO Create Ingress ServiceAccount
		// TODO Create Ingress Deployment
	}

	return err
}

// Implements Vadim-style
func (bp *bounderyProtection) RemoveMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	nsMC := corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind: "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("istio-%s", mfc.GetName()),
		},
	}

	// Don't try to delete the namespace if it doesn't exist
	var nsDummy corev1.Namespace
	err := bp.cli.Get(ctx, client.ObjectKey{Name: nsMC.GetName()}, &nsDummy)
	if err != nil {
		log.Infof("Skipped delete of Namespace %q", nsMC.GetName())
		return nil
	}

	err = bp.cli.Delete(ctx, &nsMC)
	if err == nil {
		log.Infof("Deleted Namespace %q", nsMC.GetName())
	} else {
		log.Infof("Failed to delete Namespace %q: %v (a %T)", nsMC.GetName(), err, err)
	}

	return err
}

// Implements Vadim-style
func (bp *bounderyProtection) EffectServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {
	// if len(mfc.Spec.TlsContextSelector) == 0 {
	// 	return fmt.Errorf("Unimplemented. tls selector is not specified. Currently this is not supported.")
	// } else {
	// 	tlsSelector := mfc.Spec.TlsContextSelector
	// 	if tls, err := mfutil.GetTlsSecret(ctx, bp.cli, tlsSelector); err != nil {
	// 		return err
	// 	} else {
	// 		log.Infof("Retrieved the tls info: %v", tls)
	// 	}
	// }

	// Create an Istio Gateway
	var CreatedGW *v1alpha3.Gateway
	var err error
	if mfc.Spec.UseEgressGateway {
		egressGatewayPort := mfc.Spec.EgressGatewayPort
		if egressGatewayPort == 0 {
			egressGatewayPort = mfutil.DefaultGatewayPort
		}
		if len(mfc.Spec.EgressGatewaySelector) != 0 {
			gateway := istiov1alpha3.Gateway{
				Selector: mfc.Spec.EgressGatewaySelector,
				Servers: []*istiov1alpha3.Server{
					{
						Port: &istiov1alpha3.Port{
							Number:   egressGatewayPort,
							Name:     "https-meshfed-port",
							Protocol: "HTTPS",
						},
						Hosts: []string{"*"},
						Tls: &istiov1alpha3.Server_TLSOptions{
							Mode:              istiov1alpha3.Server_TLSOptions_MUTUAL,
							ServerCertificate: "/etc/istio/certs/tls.crt",
							PrivateKey:        "/etc/istio/certs/tls.key",
							CaCertificates:    "/etc/istio/certs/example.com.crt",
						},
					},
				},
			}
			CreatedGW, err = mfutil.CreateIstioGateway(bp.istioCli, se.GetName(), se.GetNamespace(), gateway, se.GetUID())
			if err != nil {
				return err
			}
			log.Infof("Here is the GW: %v", CreatedGW)
		} else {
			// use an existing gateway
			// TODO
			return fmt.Errorf("Unimplemented. Gateway proxy is not specified. Currently this is not supported.")
		}
	} else {
		// We should never get here. Boundry implementation is with egress gateway always.
		return fmt.Errorf("Boundry implementation requires egress gateway")
	}

	// Create an Istio Virtual Service
	name := se.GetName()
	namespace := se.GetNamespace()
	fullname := name + "." + namespace + DEFAULT_PREFIX
	vs := istiov1alpha3.VirtualService{
		Hosts: []string{
			"*",
		},
		Gateways: []string{
			name,
		},
		Http: []*istiov1alpha3.HTTPRoute{
			{
				Name: ("route-" + name),
				Match: []*istiov1alpha3.HTTPMatchRequest{
					{
						Uri: &istiov1alpha3.StringMatch{
							MatchType: &istiov1alpha3.StringMatch_Prefix{Prefix: namespace + "/" + name}, // <--
						},
					},
				},
				Rewrite: &istiov1alpha3.HTTPRewrite{
					Uri:       "/",
					Authority: fullname,
				},
				Route: []*istiov1alpha3.HTTPRouteDestination{
					{
						Destination: &istiov1alpha3.Destination{
							Host:   fullname,
							Subset: se.Spec.Subset,
							Port: &istiov1alpha3.PortSelector{
								Number: se.Spec.Port,
							},
						},
					},
				},
			},
		},
	}
	log.Infof("Here is the VS: %v", vs)
	if _, err := mfutil.CreateIstioVirtualService(bp.istioCli, name, namespace, vs, se.GetUID()); err != nil {
		mfutil.DeleteIstioGateway(bp.istioCli, name, namespace)
		return err
	}

	se.Spec.Endpoints = []string{
		"yello",
		"mello",
	}
	if err := bp.cli.Update(ctx, se); err != nil {
		return err
	}
	return nil
}

// Implements Vadim-style
func (bp *bounderyProtection) RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - service exposure delete")
}

// Implements Vadim-style
func (bp *bounderyProtection) EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented")
}

// Implements Vadim-style
func (bp *bounderyProtection) RemoveServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - service binding delete")
}
