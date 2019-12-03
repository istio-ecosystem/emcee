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

	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	"istio.io/pkg/log"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// Without this (seemingly) unneeded import, fails with 'panic: No Auth Provider found for name "oidc"' on IKS
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

// ServiceExpositionReconciler reconciles a ServiceExposition object
type ServiceExpositionReconciler struct {
	client.Client
	istioclient.Interface
}

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=serviceexpositions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=serviceexpositions/status,verbs=get;update;patch

func (r *ServiceExpositionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	myFinalizerName := "mm.ibm.istio.io"
	var exposition mmv1.ServiceExposition

	if err := r.Get(ctx, req.NamespacedName, &exposition); err != nil {
		log.Warnf("Unable to fetch SE resource: %v Must have been deleted", err)
		return ctrl.Result{}, ignoreNotFound(err)
	}

	mfcSelector := exposition.Spec.MeshFedConfigSelector
	mfc, err := GetMeshFedConfig(ctx, r.Client, mfcSelector)
	if (err != nil) || (mfc.ObjectMeta.Name == "") {
		if exposition.ObjectMeta.DeletionTimestamp.IsZero() {
			// log.Warnf("SE did not find an mfc. will requeue the request: %v", err)
			return ctrl.Result{Requeue: true}, nil
		} else {
			// log.Warnf("SE did not find an mfc. being deleted. not requeueing: %v", err)
			exposition.ObjectMeta.Finalizers = removeString(exposition.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), &exposition); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	styleReconciler, err := GetExposureReconciler(&mfc, r.Client, r.Interface)
	if err != nil {
		return ctrl.Result{}, err
	}

	if exposition.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(exposition.ObjectMeta.Finalizers, myFinalizerName) {
			exposition.ObjectMeta.Finalizers = append(exposition.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), &exposition); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			err = styleReconciler.EffectServiceExposure(ctx, &exposition, &mfc)
			return ctrl.Result{}, nil
		}
	} else {
		// The object is being deleted
		if containsString(exposition.ObjectMeta.Finalizers, myFinalizerName) {
			if err := styleReconciler.RemoveServiceExposure(ctx, &exposition, &mfc); err != nil {
				return ctrl.Result{}, err
			}
			exposition.ObjectMeta.Finalizers = removeString(exposition.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), &exposition); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, err
}

func (r *ServiceExpositionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1.ServiceExposition{}).
		Complete(r)
}
