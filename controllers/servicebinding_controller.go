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

package controllers

import (
	"context"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"

	istioclient "istio.io/client-go/pkg/clientset/versioned"

	"istio.io/pkg/log"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceBindingReconciler reconciles a ServiceBinding object
type ServiceBindingReconciler struct {
	client.Client
	istioclient.Interface
}

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=servicebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=servicebindings/status,verbs=get;update;patch

func (r *ServiceBindingReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	myFinalizerName := "mm.ibm.istio.io"
	var binding mmv1.ServiceBinding

	if err := r.Get(ctx, req.NamespacedName, &binding); err != nil {
		log.Warnf("unable to fetch SB resource: %v Must have been deleted", err)
		return ctrl.Result{}, ignoreNotFound(err)
	}

	mfcSelector := binding.Spec.MeshFedConfigSelector
	mfc, err := GetMeshFedConfig(ctx, r, mfcSelector)
	if (err != nil) || (mfc.ObjectMeta.Name == "") {
		if binding.ObjectMeta.DeletionTimestamp.IsZero() {
			// log.Warnf("SB did not find an mfc. will requeue the request: %v", err)
			return ctrl.Result{Requeue: true}, nil
		} else {
			// log.Warnf("SB did not find an mfc. being deleted. not requeueing: %v", err)
			binding.ObjectMeta.Finalizers = removeString(binding.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), &binding); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	styleReconciler, err := GetBindingReconciler(&mfc, r.Client, r.Interface)
	if err != nil {
		return ctrl.Result{}, err
	}

	if binding.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(binding.ObjectMeta.Finalizers, myFinalizerName) {
			binding.ObjectMeta.Finalizers = append(binding.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), &binding); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			err = styleReconciler.EffectServiceBinding(ctx, &binding, &mfc)
			return ctrl.Result{}, err
		}
	} else {
		// The object is being deleted
		if containsString(binding.ObjectMeta.Finalizers, myFinalizerName) {
			if err := styleReconciler.RemoveServiceBinding(ctx, &binding, &mfc); err != nil {
				return ctrl.Result{}, err
			}
			binding.ObjectMeta.Finalizers = removeString(binding.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), &binding); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the reconcilser with the manager
func (r *ServiceBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mmv1.ServiceBinding{}).
		Complete(r)
}
