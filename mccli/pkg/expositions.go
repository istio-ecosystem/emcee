// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package pkg

import (
	"context"
	"log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

// NewClient creates a client that can read mmv1 things
func NewClient(restConfig *rest.Config) (client.Client, error) {
	scheme := runtime.NewScheme()
	_ = mmv1.AddToScheme(scheme)
	cl, err := client.New(restConfig, client.Options{Scheme: scheme})
	return cl, err
}

// NewCliClient creates a client based on command-line arguments
func NewCliClient(namespace, kcontext string) (client.Client, error) {

	// See https://godoc.org/k8s.io/client-go/tools/clientcmd#BuildConfigFromFlags
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{
		ClusterDefaults: clientcmd.ClusterDefaults,
		Context: clientcmdapi.Context{
			Namespace: namespace,
		},
		CurrentContext: kcontext,
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	restConfig, err := kubeConfig.ClientConfig()

	if err != nil {
		log.Fatalf("Failed to create Kubernetes REST config: %s", err)
	}

	return NewClient(restConfig)
}

// GetExposures returns the exposures in a namespace
func GetExposures(cl client.Client, namespace string) (*[]mmv1.ServiceExposition, error) {
	ctx := context.Background()
	var expositionList mmv1.ServiceExpositionList
	err := cl.List(ctx, &expositionList)
	if err != nil {
		return nil, err
	}
	return &expositionList.Items, nil
}
