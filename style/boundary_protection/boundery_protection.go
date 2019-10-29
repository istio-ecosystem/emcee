// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package boundary_protection

import (
	"context"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
	"github.ibm.com/istio-research/mc2019/style"
	"istio.io/pkg/log"

	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
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
	if len(mfc.Spec.TlsContextSelector) == 0 {
		// use the info in secret
	}
	if mfc.Spec.UseEgressGateway {
		egressGatewayPort := mfc.Spec.EgressGatewayPort
		if egressGatewayPort == 0 {
			egressGatewayPort = DefaultGatewayPort
		}
		if len(mfc.Spec.EgressGatewaySelector) != 0 {
			// use an existing gateway
			// TODO
			log.Infof("use an existing gateway..................")
		} else {
			// create an egress gateway
			// TODO
			log.Infof("create an egress gateway..................")
		}
		tlsSelector := mfc.Spec.TlsContextSelector
		GetTlsSecret(ctx, bp.cli, tlsSelector)
	}

	return nil
	// return fmt.Errorf("Unimplemented")
}

// Implements Vadim-style
func (bp *bounderyProtection) RemoveMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - MeshFedConfig delete")
}

// Implements Vadim-style
func (bp *bounderyProtection) EffectServiceExposure(ctx context.Context, se *mmv1.ServiceExposition) error {
	return nil
	// return fmt.Errorf("Unimplemented")
}

// Implements Vadim-style
func (bp *bounderyProtection) RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition) error {
	return nil
	// return fmt.Errorf("Unimplemented - service exposure delete")
}

// Implements Vadim-style
func (bp *bounderyProtection) EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding) error {
	return nil
	// return fmt.Errorf("Unimplemented")
}

// Implements Vadim-style
func (bp *bounderyProtection) RemoveServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding) error {
	return nil
	// return fmt.Errorf("Unimplemented - service binding delete")
}
