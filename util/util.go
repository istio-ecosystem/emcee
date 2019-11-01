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

package util

import (
	"context"
	"fmt"

	"github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/pkg/log"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MeshFedVersion     = "mm.ibm.istio.io/v1"
	DefaultGatewayPort = uint32(15443)
)

func IgnoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}

func ErrorNotFound(err error) bool {
	if apierrs.IsNotFound(err) {
		return true
	}
	return false
}

func CreateIstioGateway(r istioclient.Interface, name string, namespace string, gateway istiov1alpha3.Gateway, uid types.UID) (*v1alpha3.Gateway, error) {
	gw := &v1alpha3.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind: "gateway",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1alpha3.GatewaySpec{
			Gateway: gateway,
		},
	}
	if uid != "" {
		ctrl := true
		gw.ObjectMeta.GenerateName = name + "-"
		gw.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: MeshFedVersion,
				Kind:       "MeshFedConfig",
				Name:       name,
				UID:        uid,
				Controller: &ctrl,
			},
		}
	} else {
		gw.ObjectMeta.Name = name
	}
	createdGateway, err := r.NetworkingV1alpha3().Gateways(namespace).Create(gw)
	log.Infof("create an egress gateway: <Error: %v Gateway: %v>", err, createdGateway)
	return createdGateway, err
}

func GetTlsSecret(ctx context.Context, c client.Client, tlsSelector client.MatchingLabels) (corev1.Secret, error) {
	var tlsSecretList corev1.SecretList
	var tlsSecret corev1.Secret

	if len(tlsSelector) == 0 {
		log.Infof("No tls selector")
		return tlsSecret, nil
	} else {
		if err := c.List(ctx, &tlsSecretList, tlsSelector); err != nil {
			log.Warnf("unable to fetch TLS secrets: %v", err)
			return tlsSecret, fmt.Errorf("unable to fetch TLS secrets")
		}
	}
	if len(tlsSecretList.Items) == 1 {
		return tlsSecretList.Items[0], nil
	} else {
		return tlsSecretList.Items[0], fmt.Errorf("Did not find a single secret")
	}
}
