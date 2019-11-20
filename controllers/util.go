/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
	"github.ibm.com/istio-research/mc2019/style"
	"github.ibm.com/istio-research/mc2019/style/boundary_protection"
	"github.ibm.com/istio-research/mc2019/style/passthrough"

	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	"istio.io/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultGatewayPort is the port to use if port is not explicitly specified
	DefaultGatewayPort = 15443
)

func GetMeshFedConfig(ctx context.Context, r client.Client, mfcSelector map[string]string) (mmv1.MeshFedConfig, error) {
	var mfcList mmv1.MeshFedConfigList
	var mfc mmv1.MeshFedConfig
	var err error

	if len(mfcSelector) == 0 {
		log.Infof("No configs selector. using default Selector.")
		// TODO: use Default config
	} else {
		err = r.List(ctx, &mfcList, client.MatchingLabels(mfcSelector))

		if err != nil {
			log.Warnf("Unable to fetch. Error: %v", err)
			return mfc, err
		}

		if len(mfcList.Items) == 0 {
			return mfc, fmt.Errorf("Did not Find MeshFedConfig")
		} else if len(mfcList.Items) == 1 {
			mfc = mfcList.Items[0]
			log.Infof("Found MeshFedConfig: '%v' ", mfc.Name)
		} else {
			log.Warnf("Mulitple configs for selector: %v %v", mfcSelector, mfcList.Items)
			return mfc, fmt.Errorf("Mulitple configs for selector")
		}
	}
	return mfc, err
}

// GetMeshFedConfigReconciler creates a MeshFedConfig implementation specific to the MeshFedStyle
func GetMeshFedConfigReconciler(mfc *mmv1.MeshFedConfig, cli client.Client, istioCli istioclient.Interface) (style.MeshFedConfig, error) {
	// TODO: Detect if mfc refers to a Vadim-style reconciler
	if mfc.Spec.UseEgressGateway {
		return boundary_protection.NewBoundaryProtectionMeshFedConfig(cli, istioCli), nil
	} else if mfc.Spec.UseIngressGateway {
		return passthrough.NewPassthroughMeshFedConfig(cli, istioCli), nil
	}
	return nil, fmt.Errorf("No handler for %v style", mfc)
}

// GetBindingReconciler creates a ServiceBinding implementation specific to the MeshFedStyle
func GetBindingReconciler(mfc *mmv1.MeshFedConfig, cli client.Client, istioCli istioclient.Interface) (style.ServiceBinder, error) {
	// TODO: Detect if mfc refers to a Vadim-style reconciler
	if mfc.Spec.UseEgressGateway {
		return boundary_protection.NewBoundaryProtectionServiceBinder(cli, istioCli), nil
	} else if mfc.Spec.UseIngressGateway {
		return passthrough.NewPassthroughServiceBinder(cli, istioCli), nil
	}
	return nil, fmt.Errorf("No handler for %v style", mfc)
}

// GetExposureReconciler creates a ServiceExposure implementation specific to the MeshFedStyle
func GetExposureReconciler(mfc *mmv1.MeshFedConfig, cli client.Client, istioCli istioclient.Interface) (style.ServiceExposer, error) {
	// TODO: Detect if mfc refers to a Vadim-style reconciler
	if mfc.Spec.UseEgressGateway {
		return boundary_protection.NewBoundaryProtectionServiceExposer(cli, istioCli), nil
	} else if mfc.Spec.UseIngressGateway {
		return passthrough.NewPassthroughServiceExposer(cli, istioCli), nil
	}
	return nil, fmt.Errorf("No handler for %v style", mfc)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
