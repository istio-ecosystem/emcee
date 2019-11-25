// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package istioclient

import (
	// TODO Why use "sigs.k8s.io/controller-runtime/pkg/internal/log" elsewhere?
	"log"

	versionedclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
)

// GetIstioClient creates an Istio client (preferring $KUBECONFIG, falling back to defaults)
func GetIstioClient() *versionedclient.Clientset {
	restConfig := ctrl.GetConfigOrDie()
	ic, err := versionedclient.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("Failed to create Istio client: %s", err)
		return nil
	}
	return ic
}
