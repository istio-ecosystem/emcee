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

// ServiceBinder has methods for mapping ServiceBinding into mesh-specific actions
type ServiceBinder interface {
	ReconcileServiceBinding(context.Context, *mmv1.ServiceBinding) error
}

// ServiceExposer has methods for mapping ServiceBinding into mesh-specific actions
type ServiceExposer interface {
	ReconcileServiceExposure(context.Context, *mmv1.ServiceExposition) error
}
