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

	"github.com/go-logr/logr"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
)

// MeshFedReconciler reconciles a MeshFed object
type MeshFedReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=meshfeds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=meshfeds/status,verbs=get;update;patch

func (r *MeshFedReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("meshfed", req.NamespacedName)

	// your logic here
	var fed mmv1.MeshFed
	if err := r.Get(ctx, req.NamespacedName, &fed); err != nil {
		log.Error(err, "unable to fetch Resource")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, ignoreNotFound(err)
	}

	log.Info("****************")
	return ctrl.Result{}, nil
}

func (r *MeshFedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1.MeshFed{}).
		Complete(r)
}

func ignoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}
