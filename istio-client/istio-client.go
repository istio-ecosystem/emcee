// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
