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

	"istio.io/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

const (
	// DefaultGatewayPort is the port to use if port is not explicitly specified
	DefaultGatewayPort = 15443
)

func GetMeshFedConfig(ctx context.Context, reconciler interface{}, mfcSelector map[string]string) (mmv1.MeshFedConfig, error) {
	var mfcList mmv1.MeshFedConfigList
	var mfc mmv1.MeshFedConfig

	if len(mfcSelector) == 0 {
		log.Infof("No configs selector. using default Selector.")
		// TODO: use Default config
	} else {
		var err error
		switch reconciler.(type) {
		case (*ServiceBindingReconciler):
			r, _ := reconciler.(*ServiceBindingReconciler)
			err = r.List(ctx, &mfcList, client.MatchingLabels(mfcSelector))
		case (*ServiceExpositionReconciler):
			r, _ := reconciler.(*ServiceExpositionReconciler)
			err = r.List(ctx, &mfcList, client.MatchingLabels(mfcSelector))
		}
		if err != nil {
			log.Warnf("Unable to fetch. Error: %v", err)
			return mfc, err // <<<<<<<<<<<<
		}

		if len(mfcList.Items) == 1 {
			mfc = mfcList.Items[0]
			log.Infof("Found MeshFedConfig: '%v' ", mfc.Name)
		} else {
			log.Warnf("Mulitple configs for selector: %v", mfcSelector)
			// TODO: return error
		}
	}
	return mfc, nil
}

func GetTlsSecret(ctx context.Context, r *MeshFedConfigReconciler, tlsSelector client.MatchingLabels) (corev1.Secret, error) {
	var tlsSecretList corev1.SecretList
	var tlsSecret corev1.Secret

	if len(tlsSelector) == 0 {
		log.Infof("No tls selector.")
	} else {
		if err := r.List(ctx, &tlsSecretList, tlsSelector); err != nil {
			log.Warnf("unable to fetch TLS secrets: %v", err)
			return tlsSecret, ignoreNotFound(err)
		}
	}
	return tlsSecretList.Items[0], nil
}
