// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package passthrough

import (
	"context"
	"fmt"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
	"github.ibm.com/istio-research/mc2019/style"
	"istio.io/pkg/log"

	corev1 "k8s.io/api/core/v1"

	"github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
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
	dr, _ := passthroughExposingDestinationRule(mfc, se)
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

func passthroughExposingDestinationRule(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceExposition) (*v1alpha3.DestinationRule, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, fmt.Errorf("passthrough requires Ingress Gateway")
	}
	return nil, fmt.Errorf("passthroughExposingGateway not implemented yet")
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

	serviceEntry, _ := passthroughBindingServiceEntry(mfc, sb)
	_, _ = createServiceEntry(bp.istioCli, mfc.GetNamespace(), serviceEntry)

	dr, _ := passthroughBindingDestinationRule(mfc, sb)
	_, _ = createDestinationRule(bp.istioCli, mfc.GetNamespace(), dr)

	// Create a Kubernetes service
	targetNamespace := "" // TODO
	svc, err := passthroughBindingService(targetNamespace, sb, mfc)
	if err != nil {
		log.Infof("Could not generate Remote Cluster ingress Service")
		return err
	}
	err = bp.cli.Create(ctx, svc)
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

func passthroughBindingDestinationRule(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceBinding) (*v1alpha3.DestinationRule, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, fmt.Errorf("passthrough requires Ingress Gateway")
	}
	return nil, fmt.Errorf("passthroughBindingDestinationRule not implemented yet")
}

func passthroughBindingServiceEntry(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceBinding) (*v1alpha3.ServiceEntry, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, fmt.Errorf("passthrough requires Ingress Gateway")
	}
	return nil, fmt.Errorf("passthroughBindingServiceEntry not implemented yet")
}

func passthroughBindingService(namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) (*corev1.Service, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, fmt.Errorf("passthrough requires Ingress Gateway")
	}
	return nil, fmt.Errorf("passthroughBindingService not implemented yet")
}
