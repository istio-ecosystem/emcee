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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

// ServiceBindingReconciler reconciles a ServiceBinding object
type ServiceBindingReconciler struct {
	client.Client
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
	s := strings.Split(mfcSelector, "=")
	var mfcList mmv1.MeshFedConfigList
	var mfc mmv1.MeshFedConfig

	if mfcSelector == "" {
		log.Infof("No configs selector for SB '%v'. using default Selector.", binding.Name)
		// TODO: use Default config
	} else {
		if len(s) == 2 {
			if err := r.List(ctx, &mfcList, client.MatchingLabels{s[0]: s[1]}); err != nil {
				log.Warnf("Unable to fetch SB. Error: %v", err)
				return ctrl.Result{}, err
			}
			if len(mfcList.Items) == 1 {
				mfc = mfcList.Items[0]
				log.Infof("Processing SB '%v' and found MeshFedConfig: '%v' ", binding.Name, mfc.Name)
			} else {
				log.Warnf("Mulitple configs selected for SB '%v'. selector: %v", binding.Name, mfcSelector)
				// TODO: return error
			}
		} else {
			// TODO: return error
			log.Warnf("Bad MeshFedConfig selector for SB '%v'", binding.Name)
		}
	}

	return ctrl.Result{}, nil
}

func (r *ServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1.ServiceBinding{}).
		Complete(r)
}
