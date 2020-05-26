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

package passthrough

import (
	"context"
	"fmt"

	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	istioclient "istio.io/client-go/pkg/clientset/versioned"

	mfutil "github.com/istio-ecosystem/emcee/util"
	"istio.io/pkg/log"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	certificatesDir = "/etc/certs/"

//	defaultGatewayPort = uint32(15443)
//	intermeshNamespace = "global"
)

//var (
//	defaultIngressGatewaySelector = map[string]string{
//		"istio": "ingressgateway",
//	}
//)

func createGateway(r istioclient.Interface, namespace string, gateway *v1alpha3.Gateway) (*v1alpha3.Gateway, error) {
	createdGateway, err := r.NetworkingV1alpha3().Gateways(namespace).Create(context.TODO(), gateway, metav1.CreateOptions{})
	// log.Infof("create an egress gateway: <Error: %v Gateway: %v>", err, createdGateway)
	if mfutil.ErrorAlreadyExists(err) {
		updatedGateway, err := r.NetworkingV1alpha3().Gateways(namespace).Get(context.TODO(), gateway.GetName(), metav1.GetOptions{})
		if err != nil {
			log.Warnf("Failed updating Istio gateway %v: %v", gateway.GetName(), err)
			return updatedGateway, err
		}
		updatedGateway.Spec = gateway.Spec
		updatedGateway, err = r.NetworkingV1alpha3().Gateways(namespace).Update(context.TODO(), updatedGateway, metav1.UpdateOptions{})
		return updatedGateway, err
	}
	return createdGateway, err
}

func createVirtualService(r istioclient.Interface, namespace string, vs *v1alpha3.VirtualService) (*v1alpha3.VirtualService, error) {
	createdVirtualService, err := r.NetworkingV1alpha3().VirtualServices(namespace).Create(context.TODO(), vs, metav1.CreateOptions{})
	// log.Infof("create an egress gateway: <Error: %v Gateway: %v>", err, createdGateway)
	if mfutil.ErrorAlreadyExists(err) {
		updatedVirtualService, err := r.NetworkingV1alpha3().VirtualServices(namespace).Get(context.TODO(), vs.GetName(), metav1.GetOptions{})
		if err != nil {
			log.Warnf("Failed updating Istio virtual service %v: %v", vs.GetName(), err)
			return updatedVirtualService, err
		}
		updatedVirtualService.Spec = vs.Spec
		updatedVirtualService, err = r.NetworkingV1alpha3().VirtualServices(namespace).Update(context.TODO(), updatedVirtualService, metav1.UpdateOptions{})
		return updatedVirtualService, err
	}
	return createdVirtualService, err
}

func createDestinationRule(r istioclient.Interface, namespace string, dr *v1alpha3.DestinationRule) (*v1alpha3.DestinationRule, error) {
	createdDestinationRule, err := r.NetworkingV1alpha3().DestinationRules(namespace).Create(context.TODO(), dr, metav1.CreateOptions{})
	// log.Infof("create an egress gateway: <Error: %v Gateway: %v>", err, createdGateway)
	if mfutil.ErrorAlreadyExists(err) {
		updatedDestinationRule, err := r.NetworkingV1alpha3().DestinationRules(namespace).Get(context.TODO(), dr.GetName(), metav1.GetOptions{})
		if err != nil {
			log.Warnf("Failed updating Istio gateway %v: %v", dr.GetName(), err)
			return updatedDestinationRule, err
		}
		updatedDestinationRule.Spec = dr.Spec
		updatedDestinationRule, err = r.NetworkingV1alpha3().DestinationRules(namespace).Update(context.TODO(), updatedDestinationRule, metav1.UpdateOptions{})
		return updatedDestinationRule, err
	}
	return createdDestinationRule, err
}

func createServiceEntry(r istioclient.Interface, namespace string, dr *v1alpha3.ServiceEntry) (*v1alpha3.ServiceEntry, error) {
	createdServiceEntry, err := r.NetworkingV1alpha3().ServiceEntries(namespace).Create(context.TODO(), dr, metav1.CreateOptions{})
	// log.Infof("create an egress gateway: <Error: %v Gateway: %v>", err, createdGateway)
	if mfutil.ErrorAlreadyExists(err) {
		updatedServiceEntry, err := r.NetworkingV1alpha3().ServiceEntries(namespace).Get(context.TODO(), dr.GetName(), metav1.GetOptions{})
		if err != nil {
			log.Warnf("Failed updating Istio gateway %v: %v", dr.GetName(), err)
			return updatedServiceEntry, err
		}
		updatedServiceEntry.Spec = dr.Spec
		updatedServiceEntry, err = r.NetworkingV1alpha3().ServiceEntries(namespace).Update(context.TODO(), updatedServiceEntry, metav1.UpdateOptions{})
		return updatedServiceEntry, err
	}
	return createdServiceEntry, err
}

func ownerReference(apiVersion, kind string, owner metav1.ObjectMeta) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       owner.GetName(),
			UID:        owner.GetUID(),
		},
	}
}

// GetIngressEndpointsNoPort find the ingress endpoint
func GetIngressEndpointsNoPort(ctx context.Context, c client.Client, name string, namespace string, port uint32) ([]string, error) {
	var ingressService corev1.Service
	nsn := types.NamespacedName{
		// TODO: Make a function to make this name and use it everywhere
		Name:      name,
		Namespace: namespace,
	}

	if err := c.Get(ctx, nsn, &ingressService); err != nil {
		log.Warnf("ingress service %v not found with err: %v ", nsn, ingressService)
		return nil, err
	}
	if len(ingressService.Status.LoadBalancer.Ingress) > 0 {
		var s []string
		for _, ingress := range ingressService.Status.LoadBalancer.Ingress {
			s = append(s, fmt.Sprintf("%s:%d", ingress.IP, port))
		}
		return s, nil
	}
	return nil, fmt.Errorf("Did not find a host IP")

}
