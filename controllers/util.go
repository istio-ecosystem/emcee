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
	"strings"

	"istio.io/pkg/log"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

const (
	// DefaultGatewayPort is the port to use if port is not explicitly specified
	DefaultGatewayPort = 15443
)

func GetMeshFedConfig(ctx context.Context, reconciler interface{}, mfcSelector string) (mmv1.MeshFedConfig, error) {
	// r, _ := reconciler.(*ServiceBindingReconciler)
	s := strings.Split(mfcSelector, "=")
	var mfcList mmv1.MeshFedConfigList
	var mfc mmv1.MeshFedConfig
	if mfcSelector == "" {
		log.Infof("No configs selector. using default Selector.")
		// TODO: use Default config
	} else {
		if len(s) == 2 {
			var err error
			switch reconciler.(type) {
			case (*ServiceBindingReconciler):
				r, _ := reconciler.(*ServiceBindingReconciler)
				err = r.List(ctx, &mfcList, client.MatchingLabels{s[0]: s[1]})
			case (*ServiceExpositionReconciler):
				r, _ := reconciler.(*ServiceExpositionReconciler)
				err = r.List(ctx, &mfcList, client.MatchingLabels{s[0]: s[1]})
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
		} else {
			log.Warnf("Bad MeshFedConfig selector")
			// TODO: return error
		}
	}
	return mfc, nil
}

func GetTlsSecret(ctx context.Context, r *MeshFedConfigReconciler, tlsSelector string) (corev1.Secret, error) {
	s := strings.Split(tlsSelector, "=")
	var tlsSecretList corev1.SecretList
	var tlsSecret corev1.Secret
	if tlsSelector == "" {
		log.Infof("No tls selector.")
	} else {
		if len(s) == 2 {
			if err := r.List(ctx, &tlsSecretList, client.MatchingLabels{s[0]: s[1]}); err != nil {
				log.Warnf("unable to fetch TLS secrets: %v", err)
				return tlsSecret, ignoreNotFound(err)
			}
		} else {
			log.Warnf("Bad tls selector")
			// TODO: return error
		}

	}
	return tlsSecret, nil
}
