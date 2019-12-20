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
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	MeshFedVersion = "mm.ibm.istio.io/v1"
)

func IgnoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}

func ErrorNotFound(err error) bool {
	return apierrs.IsNotFound(err)
}

func ErrorAlreadyExists(err error) bool {
	return apierrs.IsAlreadyExists(err)
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

	gw.ObjectMeta.Name = name
	if uid != "" {
		ctrl := true
		//gw.ObjectMeta.GenerateName = name + "-"
		gw.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: MeshFedVersion,
				Kind:       "MeshFedConfig",
				Name:       name,
				UID:        uid,
				Controller: &ctrl,
			},
		}
	}

	createdGateway, err := r.NetworkingV1alpha3().Gateways(namespace).Create(gw)
	// log.Infof("create an egress gateway: <Error: %v Gateway: %v>", err, createdGateway)
	if ErrorAlreadyExists(err) {
		updatedGateway, err := r.NetworkingV1alpha3().Gateways(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return updatedGateway, err
		}
		updatedGateway.Spec = gw.Spec
		updatedGateway, err = r.NetworkingV1alpha3().Gateways(namespace).Update(updatedGateway)
		return updatedGateway, err
	} else {
		return createdGateway, err
	}
}

func DeleteIstioGateway(r istioclient.Interface, name string, namespace string) error {
	if err := r.NetworkingV1alpha3().Gateways(namespace).Delete(name, nil); err != nil {
		log.Warnf("Delete an egress gateway: <Error: %v>", err)
		return err
	}
	return nil
}

func CreateIstioVirtualService(r istioclient.Interface, name string, namespace string, virtualservice istiov1alpha3.VirtualService, uid types.UID) (*v1alpha3.VirtualService, error) {
	vs := &v1alpha3.VirtualService{
		TypeMeta: metav1.TypeMeta{
			Kind: "virtualservice",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1alpha3.VirtualServiceSpec{
			VirtualService: virtualservice,
		},
	}
	vs.ObjectMeta.Name = name
	if uid != "" {
		ctrl := true
		//vs.ObjectMeta.GenerateName = name + "-"
		vs.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: MeshFedVersion,
				Kind:       "MeshFedConfig",
				Name:       name,
				UID:        uid,
				Controller: &ctrl,
			},
		}
	}
	createdVirtualService, err := r.NetworkingV1alpha3().VirtualServices(namespace).Create(vs)
	// log.Infof("create an Virtual Service: <Error: %v VS: %v>", err, createdVirtualService)
	if ErrorAlreadyExists(err) {
		updatedVirtualService, err := r.NetworkingV1alpha3().VirtualServices(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			return updatedVirtualService, err
		}
		updatedVirtualService.Spec = vs.Spec
		updatedVirtualService, err = r.NetworkingV1alpha3().VirtualServices(namespace).Update(updatedVirtualService)
		return updatedVirtualService, err
	} else {
		return createdVirtualService, err
	}
}

func DeleteIstioVirtualService(r istioclient.Interface, name string, namespace string) error {
	if err := r.NetworkingV1alpha3().VirtualServices(namespace).Delete(name, nil); err != nil {
		log.Warnf("Delete a virtual service: <Error: %v>", err)
		return err
	}
	return nil
}

func GetIngressEndpoints(ctx context.Context, c client.Client, name string, namespace string, port uint32) ([]string, error) {
	var ingressService corev1.Service
	nsn := types.NamespacedName{
		// TODO: Make a function to make this name and use it everywhere
		Name:      fmt.Sprintf("istio-%s-ingress-%d", name, port),
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
	} else {
		return nil, fmt.Errorf("Did not find a host IP")
	}
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

// GetRestConfig returns a REST configuration for talking to API server based on KUBECONFIG etc
func GetRestConfig(kubeconfig, context string) (*rest.Config, error) {
	// Normally we would just call "config.GetConfigWithContext(context)"; this
	// gives us the ability to use multiple files separated by colons.
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	loadingRules.ExplicitPath = kubeconfig
	configOverrides := &clientcmd.ConfigOverrides{
		ClusterDefaults: clientcmd.ClusterDefaults,
		CurrentContext:  context,
	}

	cfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	return cfg.ClientConfig()
}
