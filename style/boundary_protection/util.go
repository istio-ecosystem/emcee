// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package boundary_protection

import (
	"context"

	mfutil "github.ibm.com/istio-research/mc2019/util"

	"istio.io/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultGatewayPort is the port to use if port is not explicitly specified
	DefaultGatewayPort = 15443
)

func GetTlsSecret(ctx context.Context, c client.Client, tlsSelector client.MatchingLabels) (corev1.Secret, error) {
	var tlsSecretList corev1.SecretList
	var tlsSecret corev1.Secret

	if len(tlsSelector) == 0 {
		log.Infof("No tls selector.")
		return tlsSecret, nil
	} else {
		if err := c.List(ctx, &tlsSecretList, tlsSelector); err != nil {
			log.Warnf("unable to fetch TLS secrets: %v", err)
			return tlsSecret, mfutil.IgnoreNotFound(err)
		}
	}
	return tlsSecretList.Items[0], nil
}
