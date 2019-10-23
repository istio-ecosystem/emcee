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

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"

	versionedclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	"istio.io/pkg/log"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceBindingReconciler reconciles a ServiceBinding object
type ServiceBindingReconciler struct {
	client.Client
	versionedclient.Interface
}

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=servicebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=servicebindings/status,verbs=get;update;patch

func (r *ServiceBindingReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	var binding mmv1.ServiceBinding
	// your logic here
	if err := r.Get(ctx, req.NamespacedName, &binding); err != nil {
		log.Warnf("unable to fetch SB resource: %v", err)
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, ignoreNotFound(err)
	}

	mfcSelector := binding.Spec.MeshFedConfigSelector
	mfc, err := GetMeshFedConfig(ctx, r, mfcSelector)
	if mfc.ObjectMeta.Name == "" {
		log.Warnf("****************: <%v-%v>", mfc, err)
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1.ServiceBinding{}).
		Complete(r)
}
