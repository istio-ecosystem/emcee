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
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

// MeshFedConfigReconciler reconciles a MeshFedConfig object
type MeshFedConfigReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=meshfedconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=meshfedconfigs/status,verbs=get;update;patch

func (r *MeshFedConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	// your logic here
	var fed mmv1.MeshFedConfig
	if err := r.Get(ctx, req.NamespacedName, &fed); err != nil {
		log.Warnf("unable to fetch MFC resource: %v", err)
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, ignoreNotFound(err)
	}

	if len(fed.Spec.TlsContextSelector) == 0 {
		// use the info in secret
	}
	if fed.Spec.UseEgressGateway {
		egressGatewayPort := fed.Spec.EgressGatewayPort
		if egressGatewayPort == 0 {
			egressGatewayPort = DefaultGatewayPort
		}
		if len(fed.Spec.EgressGatewaySelector) == 0 {
			// use an existing fateway
			// TODO
		} else {
			// create an egress gateway
			// TODO
		}
		tlsSelector := fed.Spec.TlsContextSelector
		GetTlsSecret(ctx, r, tlsSelector)
	}

	log.Warnf("processed MFC resource: %v", fed)
	return ctrl.Result{}, nil
}

func (r *MeshFedConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1.MeshFedConfig{}).
		Complete(r)
}

func ignoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}
