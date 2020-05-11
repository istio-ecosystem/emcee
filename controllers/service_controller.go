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
	"strconv"
	"strings"

	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	"istio.io/pkg/log"
	k8sapi "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServiceReconciler reconciles a MeshFedConfig object
type ServiceReconciler struct {
	client.Client
	istioclient.Interface
	DiscoveryLabelKey string
	DiscoveryLabelVal string
}

type DiscoveryServer struct {
	Name      string
	Address   string
	Operation string
}

var DiscoveryChanel chan DiscoveryServer

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=services/status,verbs=get;update;patch

func (r *ServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	var svc k8sapi.Service

	if err := r.Get(ctx, req.NamespacedName, &svc); err != nil {
		log.Warnf("unable to fetch Service resource: %v Must have been deleted %v", err, svc)
		return ctrl.Result{}, ignoreNotFound(err)
	}

	var svcAddr, svcPort string
	var s DiscoveryServer
	log.Warnf("LOOKING AT SEREVICE %v", svc)
	if svc.ObjectMeta.Labels[r.DiscoveryLabelKey] == r.DiscoveryLabelVal {
		log.Warnf("+++ LOOKING AT SEREVICE %v", svc)
		if len(svc.Spec.ExternalIPs) > 0 {
			svcAddr = svc.Spec.ExternalIPs[0]
		} else {
			svcAddr = svc.Spec.ClusterIP
		}
		log.Warnf("LOOKING AT SEREVICE Address %v", svcAddr)
		svcPort = strconv.Itoa(int(svc.Spec.Ports[0].Port))
		s = DiscoveryServer{
			Name:      svc.GetNamespace() + "/" + svc.GetName(),
			Address:   svcAddr + ":" + svcPort,
			Operation: "add",
		}

		// TODO: For early testing only. Fix.
		if strings.EqualFold(svcAddr, "9.9.9.9") {
			s.Address = "127.0.0.1" + ":" + svcPort
		}

		if svc.ObjectMeta.DeletionTimestamp.IsZero() {
			if svcAddr != "" {
				log.Warnf("LOOKING AT SEREVICE wrote UPDATE %v", s)
				s.Operation = "U"
				DiscoveryChanel <- s
			}
			return ctrl.Result{}, nil
		}

		// The object is being deleted
		if svcAddr != "" {
			log.Warnf("LOOKING AT SEREVICE wrote DELETE %v", s)
			s.Operation = "D"
			DiscoveryChanel <- s
		}
	}
	return ctrl.Result{}, nil

}

func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	DiscoveryChanel = make(chan DiscoveryServer, 100)
	return ctrl.NewControllerManagedBy(mgr).
		For(&k8sapi.Service{}).
		Complete(r)
}
