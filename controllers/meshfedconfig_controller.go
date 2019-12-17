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

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"

	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	"istio.io/pkg/log"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MeshFedConfigReconciler reconciles a MeshFedConfig object
type MeshFedConfigReconciler struct {
	client.Client
	istioclient.Interface
}

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=meshfedconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=meshfedconfigs/status,verbs=get;update;patch

func (r *MeshFedConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	myFinalizerName := "mm.ibm.istio.io"
	var mfc mmv1.MeshFedConfig

	if err := r.Get(ctx, req.NamespacedName, &mfc); err != nil {
		log.Warnf("unable to fetch MFC resource: %v Must have been deleted", err)
		return ctrl.Result{}, ignoreNotFound(err)
	}

	styleReconciler, err := GetMeshFedConfigReconciler(&mfc, r.Client, r.Interface)
	if err != nil {
		return ctrl.Result{}, err
	}

	if mfc.ObjectMeta.DeletionTimestamp.IsZero() {
		if !containsString(mfc.ObjectMeta.Finalizers, myFinalizerName) {
			mfc.ObjectMeta.Finalizers = append(mfc.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), &mfc); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			err = styleReconciler.EffectMeshFedConfig(ctx, &mfc)
			return ctrl.Result{}, err
		}
	} else {
		// The object is being deleted
		if containsString(mfc.ObjectMeta.Finalizers, myFinalizerName) {
			if err := styleReconciler.RemoveMeshFedConfig(ctx, &mfc); err != nil {
				return ctrl.Result{}, err
			}
			mfc.ObjectMeta.Finalizers = removeString(mfc.ObjectMeta.Finalizers, myFinalizerName)
			if err := r.Update(context.Background(), &mfc); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, err
	}
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

//func errorNotFound(err error) bool {
//	return apierrs.IsNotFound(err)
//}
