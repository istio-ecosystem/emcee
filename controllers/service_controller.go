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

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"

	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	"istio.io/pkg/log"
	k8sapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ServiceReconciler reconciles a MeshFedConfig object
type ServiceReconciler struct {
	client.Client
	istioclient.Interface
	DiscoveryLabelKey  string
	DiscoveryLabelVal  string
	AutoExposeLabelKey string
	AutoExposeLabelVal string
	SEReconciler       *ServiceExpositionReconciler
}

type DiscoveryServer struct {
	Name      string
	Address   string
	Operation string
}

var DiscoveryChanel chan DiscoveryServer

const (
	fedConfig            = "fed-config"
	defaultMeshFedConfig = "passthrough"
)

// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mm.ibm.istio.io,resources=services/status,verbs=get;update;patch

// Functions for auto expose

func newServiceExposure(svc *k8sapi.Service, name, alias string) *mmv1.ServiceExposition {
	se := mmv1.ServiceExposition{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: mmv1.ServiceExpositionSpec{
			Name: svc.Name,
			Port: uint32(svc.Spec.Ports[0].Port), // TODO one port only?
			MeshFedConfigSelector: map[string]string{ // TODO
				"fed-config": "passthrough",
			},
		},
	}
	if alias != "" {
		se.Spec.Alias = alias
		s := strings.Split(alias, ":")
		if len(s) == 2 {
			se.Spec.MeshFedConfigSelector = map[string]string{
				fedConfig: s[0],
			}
			se.Spec.Alias = s[1]
		} else if len(s) == 1 {
			se.Spec.MeshFedConfigSelector = map[string]string{
				fedConfig: defaultMeshFedConfig,
			}
			se.Spec.Alias = s[0]
		}
	}
	return &se
}

func createServiceExposure(ser *ServiceExpositionReconciler, svc *k8sapi.Service, alias string) error {
	name := svc.GetName() + "-auto-exposed"
	nv := newServiceExposure(svc, name, alias)
	_, err := controllerutil.CreateOrUpdate(context.Background(), ser.Client, nv, func() error { return nil })
	if err != nil {
		return err
	}
	return nil
}

// Reconcile reconciles
func (r *ServiceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	var svc k8sapi.Service

	if err := r.Get(ctx, req.NamespacedName, &svc); err != nil {
		log.Warnf("unable to fetch Service resource: %v Must have been deleted %v", err, svc)
		return ctrl.Result{}, ignoreNotFound(err)
	}

	var svcAddr, svcPort string
	var s DiscoveryServer
	if r.DiscoveryLabelVal != "" && svc.ObjectMeta.Labels[r.DiscoveryLabelKey] == r.DiscoveryLabelVal {
		if len(svc.Spec.ExternalIPs) > 0 {
			svcAddr = svc.Spec.ExternalIPs[0]
		} else {
			svcAddr = svc.Spec.ClusterIP
		}
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
				s.Operation = "U"
				DiscoveryChanel <- s
			}
			return ctrl.Result{}, nil
		}

		// The object is being deleted
		if svcAddr != "" {
			s.Operation = "D"
			DiscoveryChanel <- s
		}
	} else {
		if alias, ok := svc.ObjectMeta.Labels[r.AutoExposeLabelKey]; ok {
			log.Warnf("LOOKING AT SEREVICE being Auto Exposed <%v-%v-%v-%v>", svc,
				r.AutoExposeLabelKey, r.AutoExposeLabelVal, svc.ObjectMeta.Labels[r.AutoExposeLabelKey])
			if err := createServiceExposure(r.SEReconciler, &svc, alias); err != nil {
				log.Warnf("Could not auto exposed: %v", svc, alias)
			}
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
