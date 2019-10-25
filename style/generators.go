// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package style

import (
	"context"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

// ServiceBinder creates/destroys the underlying mesh objects that implement a mmv1.ServiceBinding
type ServiceBinder interface {
	EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding) error
	RemoveServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding) error
}

// ServiceExposer creates/destroys the underlying mesh objects that implement a mmv1.ServiceExposition
type ServiceExposer interface {
	EffectServiceExposure(ctx context.Context, se *mmv1.ServiceExposition) error
	RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition) error
}
