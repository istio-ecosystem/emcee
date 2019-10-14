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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

// ServiceExpositionReconciler reconciles a ServiceExposition object
type ServiceExpositionReconciler struct {
	client.Client
}

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=serviceexpositions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=serviceexpositions/status,verbs=get;update;patch

func (r *ServiceExpositionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	var exposition mmv1.ServiceExposition
	// your logic here
	if err := r.Get(ctx, req.NamespacedName, &exposition); err != nil {
		log.Warnf("unable to fetch SE resource: %v", err)
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, ignoreNotFound(err)
	}

	log.Warnf("processed SE resource: %v", exposition)
	return ctrl.Result{}, nil
}

func (r *ServiceExpositionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1.ServiceExposition{}).
		Complete(r)
}
