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

package boundary_protection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/style"
	mfutil "github.com/istio-ecosystem/emcee/util"

	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/pkg/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
)

type boundaryProtection struct {
	client.Client
	istioclient.Interface
}

var (
	// (compile-time check that we implement the interface)
	_ style.MeshFedConfig  = &boundaryProtection{}
	_ style.ServiceBinder  = &boundaryProtection{}
	_ style.ServiceExposer = &boundaryProtection{}
)

const (
	defaultPrefix = ".svc.cluster.local"
)

// NewBoundaryProtectionMeshFedConfig creates a "Boundary Protection" style implementation for handling MeshFedConfig
func NewBoundaryProtectionMeshFedConfig(cli client.Client, istioCli istioclient.Interface) style.MeshFedConfig {
	return &boundaryProtection{
		cli,
		istioCli,
	}
}

// NewBoundaryProtectionServiceExposer creates a "Boundary Protection" style implementation for handling ServiceExposure
func NewBoundaryProtectionServiceExposer(cli client.Client, istioCli istioclient.Interface) style.ServiceExposer {
	return &boundaryProtection{
		cli,
		istioCli,
	}
}

// NewBoundaryProtectionServiceBinder creates a "Boundary Protection" style implementation for handling ServiceBinding
func NewBoundaryProtectionServiceBinder(cli client.Client, istioCli istioclient.Interface) style.ServiceBinder {
	return &boundaryProtection{
		cli,
		istioCli,
	}
}

// ***************************
// *** EffectMeshFedConfig ***
// ***************************
func (bp *boundaryProtection) EffectMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	// If the MeshFedConfig changes we may need to re-create all of the Istio
	// things for every ServiceBinding and ServiceExposition.  TODO Trigger
	// re-reconcile of every ServiceBinding and ServiceExposition.

	targetNamespace := mfc.GetNamespace()

	secret, err := getSecretName(ctx, mfc, bp.Client)
	if err != nil {
		log.Infof("Could not get secret name from MeshFedConfig: %v", err)
		return err
	}

	// Create Egress Service
	egressSvc := boundaryProtectionEgressService(mfc.GetName(),
		targetNamespace,
		// TODO Our EgressGatewayPort hould be int32 like ports
		int32(mfc.Spec.EgressGatewayPort),
		mfc.Spec.EgressGatewaySelector, mfc)

	err = bp.Client.Create(ctx, &egressSvc)
	if err != nil && !mfutil.ErrorAlreadyExists(err) {
		log.Infof("Failed to create Egress Service %s.%s: %v",
			egressSvc.GetName(), egressSvc.GetNamespace(), err)
		return err
	}
	if err == nil {
		log.Infof("Created Egress Service %s.%s", egressSvc.GetName(), egressSvc.GetNamespace())
	}

	// If mfc.Spec.EgressGatewaySelector is empty, default it
	if len(mfc.Spec.EgressGatewaySelector) == 0 {
		mfc.Spec.EgressGatewaySelector = map[string]string{
			style.ProjectID: "egressgateway",
		}
		log.Infof("MeshFedConfig did not specify an egress workload, using %v", mfc.Spec.EgressGatewaySelector)
		// TODO?: persist this change
	}

	nEgressPod, err := bp.workloadMatches(ctx, targetNamespace, labels.SelectorFromSet(mfc.Spec.EgressGatewaySelector))
	if err != nil {
		log.Infof("Failed to list existing Egress pods: %v", err)
		return err
	}
	if nEgressPod == 0 {
		err = bp.createEgressDeployment(ctx, mfc, targetNamespace, secret)
		if err != nil {
			log.Infof("Could not create Egress deployment: %v", err)
			return err
		}
	}

	// Create Ingress Service if it doesn't already exist
	// TODO ServicePort.Port is a uint32, IngressGatewayPort should be too
	ingressSvc := boundaryProtectionIngressService(mfc.GetName(),
		targetNamespace,
		int32(mfc.Spec.IngressGatewayPort),
		mfc.Spec.IngressGatewaySelector, mfc)
	err = bp.Client.Create(ctx, &ingressSvc)
	if err != nil && !mfutil.ErrorAlreadyExists(err) {
		log.Infof("Failed to create Ingress Service %s.%s: %v",
			ingressSvc.GetName(), ingressSvc.GetNamespace(), err)
		return err
	}
	if err == nil {
		log.Infof("Created Ingress Service %s.%s", ingressSvc.GetName(), ingressSvc.GetNamespace())
	}

	// If mfc.Spec.IngressGatewaySelector is empty, default it
	if len(mfc.Spec.IngressGatewaySelector) == 0 {
		mfc.Spec.IngressGatewaySelector = defaultIngressGatewaySelector
		log.Infof("MeshFedConfig did not specify an ingress workload, using %v", mfc.Spec.IngressGatewaySelector)
		// TODO?: persist this change
	}

	nIngressPod, err := bp.workloadMatches(ctx, targetNamespace, labels.SelectorFromSet(mfc.Spec.IngressGatewaySelector))
	if err != nil {
		log.Infof("Failed to list existing Ingress pods: %v", err)
		return err
	}
	if nIngressPod == 0 {
		err = bp.createIngressDeployment(ctx, mfc, targetNamespace, secret)
		if err != nil {
			log.Infof("Could not create Ingress deployment: %v", err)
			return err
		}
	}

	return nil
}

func (bp *boundaryProtection) RemoveMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	return nil
}

// *****************************
// *** EffectServiceExposure ***
// *****************************
func (bp *boundaryProtection) EffectServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {
	// Build an Istio Gateway and an Istio Virtual Service
	gw, vs, err := boundaryProtectionExposingGatewayAndVs(mfc, se)
	if err != nil {
		log.Warnf("could not model gateway %v %v", gw, err)
		return err
	}

	_, err = createGateway(bp.Interface, mfc.GetNamespace(), gw)

	if err != nil {
		log.Warnf("could not create gateway %v %v", gw, err)
		return err
	}
	_, err = createVirtualService(bp.Interface, mfc.GetNamespace(), vs)
	if err != nil {
		log.Warnf("could not create virtual service %v %v", vs, err)
		return err
	}

	// get the endpoints
	eps, err := mfutil.GetIngressEndpoints(ctx, bp.Client, mfc.GetName(), mfc.GetNamespace(), defaultGatewayPort)
	if err != nil {
		log.Warnf("could not get endpoints %v %v", eps, err)
		return err
	}
	se.Spec.Endpoints = eps
	se.Status.Ready = true
	if err := bp.Client.Update(ctx, se); err != nil {
		return err
	}
	return nil
}

func boundaryProtectionExposingGatewayAndVs(mfc *mmv1.MeshFedConfig, se *mmv1.ServiceExposition) (*v1alpha3.Gateway, *v1alpha3.VirtualService, error) {
	if !mfc.Spec.UseIngressGateway {
		return nil, nil, fmt.Errorf("Boundry Protection requires Ingress Gateway")
	}

	// build an Istio gateway
	ingressGatewayPort := mfc.Spec.IngressGatewayPort
	if ingressGatewayPort == 0 {
		ingressGatewayPort = defaultGatewayPort
	}

	ingressSelector := defaultIngressGatewaySelector
	if len(mfc.Spec.IngressGatewaySelector) != 0 {
		ingressSelector = mfc.Spec.IngressGatewaySelector
	}

	gateway := istiov1alpha3.Gateway{
		Selector: ingressSelector,
		Servers: []*istiov1alpha3.Server{
			{
				Port: &istiov1alpha3.Port{
					Number:   ingressGatewayPort,
					Name:     "https-meshfed-port",
					Protocol: "HTTPS",
				},
				Hosts: []string{"*"},
				Tls: &istiov1alpha3.Server_TLSOptions{
					Mode:              istiov1alpha3.Server_TLSOptions_MUTUAL,
					ServerCertificate: certificatesDir + "tls.crt",
					PrivateKey:        certificatesDir + "tls.key",
					CaCertificates:    certificatesDir + "example.com.crt",
				},
			},
		},
	}

	name := se.GetName()
	namespace := mfc.GetNamespace()
	uid := se.GetUID()
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
		gw.ObjectMeta.OwnerReferences = ownerReference(se.APIVersion, se.Kind, se.ObjectMeta)
	}

	// create vs
	namespace = se.GetNamespace()
	serviceName := se.Spec.Name
	fullname := serviceName + "." + namespace + defaultPrefix
	virtualService := istiov1alpha3.VirtualService{
		Hosts: []string{
			"*",
		},
		Gateways: []string{
			name,
		},
		Http: []*istiov1alpha3.HTTPRoute{
			{
				Name: ("route-" + name),
				Match: []*istiov1alpha3.HTTPMatchRequest{
					{
						Uri: &istiov1alpha3.StringMatch{
							MatchType: &istiov1alpha3.StringMatch_Prefix{Prefix: servicePathExposure(se)},
						},
					},
				},
				Rewrite: &istiov1alpha3.HTTPRewrite{
					Uri:       "/",
					Authority: fullname,
				},
				Route: []*istiov1alpha3.HTTPRouteDestination{
					{
						Destination: &istiov1alpha3.Destination{
							Host:   fullname,
							Subset: se.Spec.Subset,
							Port: &istiov1alpha3.PortSelector{
								Number: se.Spec.Port,
							},
						},
					},
				},
			},
		},
	}

	// CreateIstioVirtualService(bp.istioCli, name, mfc.GetNamespace(), vs, se.GetUID())
	// reateIstioVirtualService(r istioclient.Interface, name string, namespace string, virtualservice istiov1alpha3.VirtualService, uid types.UID) (*v1alpha3.VirtualService, error)
	namespace = mfc.GetNamespace()
	vs := &v1alpha3.VirtualService{
		TypeMeta: metav1.TypeMeta{
			Kind: "virtualservice",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1alpha3.VirtualServiceSpec{
			VirtualService: virtualService,
		},
	}
	vs.ObjectMeta.Name = name
	if uid != "" {
		vs.ObjectMeta.OwnerReferences = ownerReference(se.APIVersion, se.Kind, se.ObjectMeta)
	}
	return gw, vs, nil
}

func (bp *boundaryProtection) RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - service exposure delete")
}

// ****************************
// *** EffectServiceBinding ***
// ****************************
func (bp *boundaryProtection) EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {

	// See https://github.com/istio-ecosystem/multi-mesh-examples/tree/master/add_hoc_limited_trust/http#consume-helloworld-v2-in-the-first-cluster

	targetNamespace := mfc.GetNamespace()
	localNamespace := sb.GetNamespace()

	// Create a Kubernetes service for the remote Ingress, if needed
	goalSvcRemoteCluster, err := boundaryProtectionRemoteIngressService(targetNamespace, sb, mfc)
	if err != nil {
		log.Infof("Could not generate Remote Cluster ingress Service")
		return err
	}
	svcRemoteCluster := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      goalSvcRemoteCluster.GetName(),
			Namespace: goalSvcRemoteCluster.GetNamespace(),
		},
	}
	or, err := controllerutil.CreateOrUpdate(ctx, bp.Client, svcRemoteCluster, func() error {
		svcRemoteCluster.ObjectMeta.Labels = goalSvcRemoteCluster.Labels
		svcRemoteCluster.ObjectMeta.OwnerReferences = goalSvcRemoteCluster.ObjectMeta.OwnerReferences
		// Update the Spec fields WITHOUT clearing .Spec.ClusterIP
		svcRemoteCluster.Spec.Ports = goalSvcRemoteCluster.Spec.Ports
		svcRemoteCluster.Spec.SessionAffinity = goalSvcRemoteCluster.Spec.SessionAffinity
		svcRemoteCluster.Spec.Type = goalSvcRemoteCluster.Spec.Type
		svcRemoteCluster.Spec.ExternalName = goalSvcRemoteCluster.Spec.ExternalName
		return nil
	})
	if err != nil {
		return err
	}
	log.Infof("%s %s %s", or,
		"Remote Cluster ingress Service",
		renderName(&svcRemoteCluster.ObjectMeta))

	// Create an Istio destination rule for the remote Ingress, if needed
	drRemoteCluster := boundaryProtectionRemoteDestinationRule(targetNamespace, mfc, sb)
	_, err = createDestinationRule(bp.Interface, targetNamespace, &drRemoteCluster)
	if err != nil {
		log.Warnf("Failed creating/updating Istio destination rule %v: %v", drRemoteCluster.GetName(), err)
		return err
	}

	goalSvcLocalFacade := boundaryProtectionLocalServiceFacade(localNamespace, sb, mfc)
	svcLocalFacade := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      goalSvcLocalFacade.GetName(),
			Namespace: goalSvcLocalFacade.GetNamespace(),
		},
	}
	or, err = controllerutil.CreateOrUpdate(ctx, bp.Client, svcLocalFacade, func() error {
		svcLocalFacade.ObjectMeta.Labels = goalSvcLocalFacade.Labels
		svcLocalFacade.ObjectMeta.OwnerReferences = goalSvcLocalFacade.ObjectMeta.OwnerReferences
		// Update the Spec fields WITHOUT clearing .Spec.ClusterIP
		svcLocalFacade.Spec.Ports = goalSvcLocalFacade.Spec.Ports
		svcLocalFacade.Spec.SessionAffinity = goalSvcLocalFacade.Spec.SessionAffinity
		svcLocalFacade.Spec.Type = goalSvcLocalFacade.Spec.Type
		return nil
	})
	if err != nil {
		return err
	}
	log.Infof("%s %s %s", or,
		"Local Service facade Service",
		renderName(&svcLocalFacade.ObjectMeta))

	comboName := serviceIntermeshName(sb.Spec.Name)
	goalSvcLocalEgress := boundaryProtectionLocalServiceEgress(comboName, localNamespace, sb, mfc)
	svcLocalEgress := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      goalSvcLocalEgress.GetName(),
			Namespace: goalSvcLocalEgress.GetNamespace(),
		},
	}
	or, err = controllerutil.CreateOrUpdate(ctx, bp.Client, svcLocalEgress, func() error {
		svcLocalEgress.ObjectMeta.Labels = goalSvcLocalEgress.Labels
		svcLocalEgress.ObjectMeta.OwnerReferences = goalSvcLocalEgress.ObjectMeta.OwnerReferences
		// Update the Spec fields WITHOUT clearing .Spec.ClusterIP
		svcLocalEgress.Spec.Ports = goalSvcLocalEgress.Spec.Ports
		svcLocalEgress.Spec.SessionAffinity = goalSvcLocalEgress.Spec.SessionAffinity
		svcLocalEgress.Spec.Type = goalSvcLocalEgress.Spec.Type
		return nil
	})
	if err != nil {
		return err
	}
	log.Infof("%s %s %s", or,
		"Local Service egress Service",
		renderName(&svcLocalEgress.ObjectMeta))

	svcLocalGateway := boundaryProtectionLocalServiceGateway(comboName, targetNamespace, sb, mfc)
	_, err = createGateway(bp.Interface, targetNamespace, &svcLocalGateway)
	if err != nil {
		log.Warnf("Failed creating/updating Istio gateway %v: %v", svcLocalGateway.GetName(), err)
		return err
	}

	svcLocalDR := boundaryProtectionLocalServiceDestinationRule(comboName, targetNamespace, sb, mfc)
	_, err = createDestinationRule(bp.Interface, targetNamespace, &svcLocalDR)
	if err != nil {
		log.Warnf("Failed creating/updating Istio destination rule %v: %v", svcLocalDR.GetName(), err)
		return err
	}

	vsEgressExternal := boundaryProtectionEgressExternalVirtualService(comboName, targetNamespace, sb, mfc)
	_, err = createVirtualService(bp.Interface, targetNamespace, &vsEgressExternal)
	if err != nil {
		log.Warnf("Failed creating/updating Istio virtual service %v: %v", vsEgressExternal.GetName(), err)
		return err
	}

	vsLocalToEgress := boundaryProtectionLocalToEgressVirtualService(comboName, sb, mfc)
	_, err = createVirtualService(bp.Interface, localNamespace, &vsLocalToEgress)
	if err != nil {
		log.Warnf("Failed creating/updating Istio virtual service %v: %v", vsLocalToEgress.GetName(), err)
		return err
	}

	log.Infof("Successfully reconciled ServiceBinding %s/%s", sb.GetNamespace(), sb.GetName())
	return nil
}

func (bp *boundaryProtection) RemoveServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - service binding delete")
}

// TODO We currently hard-code this Service rather than using Istio Operator to create
// one congruent with user's Istio installation.  We should use Operator, but it is
// not set up to create an ingress/egress w/o control plane
func boundaryProtectionEgressService(name, namespace string, port int32, selector map[string]string, owner *mmv1.MeshFedConfig) corev1.Service {
	return corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("istio-%s-egress-%d", name, port),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": name,
				"role": "egress-svc",
			},
			OwnerReferences: ownerReference(owner.APIVersion, owner.Kind, owner.ObjectMeta),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
				}, // TODO the other ports?  How do we know which ports?  How do we know
				// the Egress port is HTTP?
			},
			Selector: selector,
		},
	}
}

// TODO We currently hard-code this Service rather than using Istio Operator to create
// one congruent with user's Istio installation.  We should use Operator, but it is
// not set up to create an ingress/egress w/o control plane
func boundaryProtectionIngressService(name, namespace string, port int32, selector map[string]string, owner *mmv1.MeshFedConfig) corev1.Service {
	return corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("istio-%s-ingress-%d", name, port),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": name,
				"role": "ingress-svc",
			},
			OwnerReferences: ownerReference(owner.APIVersion, owner.Kind, owner.ObjectMeta),
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Ports: []corev1.ServicePort{
				{
					Name: "https-for-cross-cluster-communication",
					// TODO ServicePort.Port is a uint32, IngressGatewayPort should be too
					// TODO How do we know if the IngressGatewayPort becomes the https port or the tls port?
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
				},
				{
					Name:       "tls-for-cross-cluster-communication",
					Port:       15444,
					TargetPort: intstr.FromInt(15444),
				},
				{
					Name:       "tcp-1",
					Port:       31400,
					TargetPort: intstr.FromInt(31400),
				},
				{
					Name:       "tcp-2",
					Port:       31401,
					TargetPort: intstr.FromInt(31401),
				},
			},
			Selector: selector,
		},
	}
}

// TODO We currently hard-code this ServiceAccount rather than using Istio Operator to create
// one congruent with user's Istio installation.  We should use Operator, but it is
// not set up to create an ingress/egress w/o control plane
func boundaryProtectionEgressServiceAccount(name, namespace string, owner *mmv1.MeshFedConfig) corev1.ServiceAccount {
	return boundaryProtectionXServiceAccount(egressServiceAccountName(name),
		namespace, owner)
}

// TODO We currently hard-code this ServiceAccount rather than using Istio Operator to create
// one congruent with user's Istio installation.  We should use Operator, but it is
// not set up to create an ingress/egress w/o control plane
func boundaryProtectionIngressServiceAccount(name, namespace string, owner *mmv1.MeshFedConfig) corev1.ServiceAccount {
	return boundaryProtectionXServiceAccount(ingressServiceAccountName(name),
		namespace, owner)
}

func boundaryProtectionXServiceAccount(name, namespace string, mfc *mmv1.MeshFedConfig) corev1.ServiceAccount {
	return corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(mfc.APIVersion, mfc.Kind, mfc.ObjectMeta),
		},
	}
}

// TODO We currently hard-code this Deployment rather than using Istio Operator to create
// one congruent with user's Istio installation.  We should use Operator, but it is
// not set up to create an ingress/egress w/o control plane
func boundaryProtectionEgressDeployment(name, namespace string, labels map[string]string, sa *corev1.ServiceAccount, secretName string, owner *mmv1.MeshFedConfig) appsv1.Deployment {

	return appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind: "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerReference(owner.APIVersion, owner.Kind, owner.ObjectMeta),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"sidecar.istio.io/inject": "false",
						"heritage":                "emcee",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sa.GetName(),
					Containers: []corev1.Container{
						{
							Name:  "istio-proxy",
							Args:  boundaryProtectionPodArgs("istio-private-egressgateway"),
							Env:   boundaryProtectionPodEnv(labels, "istio-private-egressgateway"),
							Image: egressImage(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "istio-certs",
									ReadOnly:  true,
									MountPath: "/etc/certs",
								},
								{
									Name:      "mesh-certs",
									ReadOnly:  true,
									MountPath: "/etc/istio/mesh/certs",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "istio-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  fmt.Sprintf("istio.%s", sa.GetName()),
									Optional:    pbool(true),
									DefaultMode: pint32(420),
								},
							},
						},
						{
							Name: "mesh-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  secretName,
									Optional:    pbool(true),
									DefaultMode: pint32(420),
								},
							},
						},
					},
				},
			},
		},
	}
}

// TODO We currently hard-code this Deployment rather than using Istio Operator to create
// one congruent with user's Istio installation.  We should use Operator, but it is
// not set up to create an ingress/egress w/o control plane
func boundaryProtectionIngressDeployment(name, namespace string, labels map[string]string, sa *corev1.ServiceAccount, secretName string, owner *mmv1.MeshFedConfig) appsv1.Deployment {

	return appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind: "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          labels,
			OwnerReferences: ownerReference(owner.APIVersion, owner.Kind, owner.ObjectMeta),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
					Annotations: map[string]string{
						"sidecar.istio.io/inject": "false",
						"heritage":                "emcee",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: sa.GetName(),
					Containers: []corev1.Container{
						{
							Name:  "istio-proxy",
							Args:  boundaryProtectionPodArgs("istio-private-ingressgateway"),
							Env:   boundaryProtectionPodEnv(labels, "istio-private-ingressgateway"),
							Image: ingressImage(),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "istio-certs",
									ReadOnly:  true,
									MountPath: "/etc/certs",
								},
								{
									Name:      "mesh-certs",
									ReadOnly:  true,
									MountPath: "/etc/istio/mesh/certs",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "istio-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  fmt.Sprintf("istio.%s", sa.GetName()),
									Optional:    pbool(true),
									DefaultMode: pint32(420),
								},
							},
						},
						{
							Name: "mesh-certs",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  secretName,
									Optional:    pbool(true),
									DefaultMode: pint32(420),
								},
							},
						},
					},
				},
			},
		},
	}
}

func pint32(i int32) *int32 {
	retval := i
	return &retval
}

func pbool(b bool) *bool {
	retval := b
	return &retval
}

func envVarFromField(apiVersion, fieldPath string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		FieldRef: &corev1.ObjectFieldSelector{
			APIVersion: apiVersion,
			FieldPath:  fieldPath,
		},
	}
}

func boundaryProtectionPodArgs(serviceCluster string) []string {
	return []string{
		"proxy",
		"router",
		"--domain", "$(POD_NAMESPACE).svc.cluster.local",
		"--log_output_level=default:info",
		"--drainDuration", "45s",
		"--parentShutdownDuration", "1m0s",
		"--connectTimeout", "10s",
		"--serviceCluster", serviceCluster,
		"--zipkinAddress", "zipkin.istio-system:9411",
		"--proxyAdminPort", "15000",
		"--statusPort", "15020",
		"--controlPlaneAuthPolicy", "NONE",
		"--discoveryAddress", "istio-pilot.istio-system:15010",
	}
}

func boundaryProtectionPodEnv(labels map[string]string, workload string) []corev1.EnvVar {
	bytes, _ := json.Marshal(labels)
	metaJSONLabels := string(bytes)

	return []corev1.EnvVar{
		{
			Name:      "NODE_NAME",
			ValueFrom: envVarFromField("v1", "spec.nodeName"),
		},
		{
			Name:      "POD_NAME",
			ValueFrom: envVarFromField("v1", "metadata.name"),
		},
		{
			Name:      "POD_NAMESPACE",
			ValueFrom: envVarFromField("v1", "metadata.namespace"),
		},
		{
			Name:      "INSTANCE_IP",
			ValueFrom: envVarFromField("v1", "status.podIP"),
		},
		{
			Name:      "HOST_IP",
			ValueFrom: envVarFromField("v1", "status.hostIP"),
		},
		{
			Name:      "SERVICE_ACCOUNT",
			ValueFrom: envVarFromField("v1", "spec.serviceAccountName"),
		},
		{
			Name:      "ISTIO_META_POD_NAME",
			ValueFrom: envVarFromField("v1", "metadata.name"),
		},
		{
			Name:      "ISTIO_META_CONFIG_NAMESPACE",
			ValueFrom: envVarFromField("v1", "metadata.namespace"),
		},
		{
			Name:  "ISTIO_METAJSON_LABELS",
			Value: metaJSONLabels,
		},
		{
			Name:  "ISTIO_META_CLUSTER_ID",
			Value: "Kubernetes",
		},
		{
			Name:  "SDS_ENABLED",
			Value: "false", // TODO Get from environment
		},
		{
			Name:  "ISTIO_META_WORKLOAD_NAME",
			Value: workload,
		},
		/* TODO
		- name: ISTIO_META_OWNER
			value: kubernetes://api/apps/v1/namespaces/istio-private-gateways/deployments/istio-private-egressgateway
		*/
	}
}

func getSecretName(ctx context.Context, mfc *mmv1.MeshFedConfig, cli client.Reader) (string, error) {
	var matches corev1.SecretList
	err := cli.List(ctx, &matches, &client.ListOptions{
		LabelSelector: labels.SelectorFromSet(mfc.Spec.TlsContextSelector),
	})
	if err != nil {
		return "", err
	}
	if len(matches.Items) == 0 {
		return "", fmt.Errorf("No secrets match %v", mfc.Spec.TlsContextSelector)
	}
	if len(matches.Items) > 1 {
		return "", fmt.Errorf("Ambiguous: %d secrets match %v", len(matches.Items), mfc.Spec.TlsContextSelector)
	}
	return matches.Items[0].GetName(), nil
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

func (bp *boundaryProtection) workloadMatches(ctx context.Context, namespace string, selector labels.Selector) (int, error) {
	var matches corev1.PodList
	err := bp.Client.List(ctx, &matches, &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: selector,
	})
	if err != nil {
		return 0, err
	}
	// TODO exclude terminating pods from this count?
	return len(matches.Items), nil
}

func (bp *boundaryProtection) createEgressDeployment(ctx context.Context, mfc *mmv1.MeshFedConfig, targetNamespace, secret string) error {
	egressSA := boundaryProtectionEgressServiceAccount(mfc.GetName(),
		targetNamespace, mfc)
	err := bp.Client.Create(ctx, &egressSA)
	if err != nil && !mfutil.ErrorAlreadyExists(err) {
		log.Infof("Failed to create Egress ServiceAccount %s.%s: %v",
			egressSA.GetName(), egressSA.GetNamespace(), err)
		return err
	}
	if err == nil {
		log.Infof("Created Egress Service Account %s.%s", egressSA.GetName(), egressSA.GetNamespace())
	}

	egressDeployment := boundaryProtectionEgressDeployment(mfc.GetName()+"-egressgateway",
		targetNamespace, mfc.Spec.EgressGatewaySelector, &egressSA, secret, mfc)
	err = bp.Client.Create(ctx, &egressDeployment)
	if err != nil && !mfutil.ErrorAlreadyExists(err) {
		log.Infof("Failed to create Egress Deployment %s.%s: %v",
			egressDeployment.GetName(), egressDeployment.GetNamespace(), err)
		return err
	}
	if err == nil {
		log.Infof("Created Egress Deployment %s.%s", egressDeployment.GetName(), egressDeployment.GetNamespace())
	}
	return err
}

func (bp *boundaryProtection) createIngressDeployment(ctx context.Context, mfc *mmv1.MeshFedConfig, targetNamespace, secret string) error {
	ingressSA := boundaryProtectionIngressServiceAccount(mfc.GetName(),
		targetNamespace, mfc)
	err := bp.Client.Create(ctx, &ingressSA)
	if err != nil && !mfutil.ErrorAlreadyExists(err) {
		log.Infof("Failed to create Ingress ServiceAccount %s.%s: %v",
			ingressSA.GetName(), ingressSA.GetNamespace(), err)
		return err
	}
	if err == nil {
		log.Infof("Created Ingress Service Account %q", ingressSA.GetName())
	}

	ingressDeployment := boundaryProtectionIngressDeployment(mfc.GetName()+"-ingressgateway",
		targetNamespace, mfc.Spec.IngressGatewaySelector, &ingressSA, secret, mfc)
	err = bp.Client.Create(ctx, &ingressDeployment)
	if err != nil && !mfutil.ErrorAlreadyExists(err) {
		log.Infof("Failed to create Ingress Deployment %s.%s: %v",
			ingressDeployment.GetName(), ingressDeployment.GetNamespace(), err)
		return err
	}
	if err == nil {
		log.Infof("Created Ingress Deployment %q", ingressDeployment.GetName())
	}
	return err
}

func boundaryProtectionRemoteIngressService(namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) (*corev1.Service, error) {

	SingleAddressPort := 0 // TODO(mb) what if there are more? if not possible, refactor the for loop
	SingleAddressIP := ""

	for _, endpoint := range sb.Spec.Endpoints {
		parts := strings.Split(endpoint, ":")
		numparts := len(parts)
		if numparts != 2 {
			return nil, fmt.Errorf("Address %q not in form ip:port", endpoint)
		}
		port, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, err
		}
		// TODO Verify parts[0] is an IPv4 or ipv6 address
		SingleAddressPort = port
		SingleAddressIP = parts[0]
	}

	svc := corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceRemoteName(mfc, sb),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
				"role": "remote-ingress-svc",
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: SingleAddressIP,
			Ports: []corev1.ServicePort{
				{
					Name: "tls-for-cross-cluster-communication",
					Port: int32(SingleAddressPort),
				},
			},
		},
	}

	return &svc, nil
}

func serviceRemoteName(mfc *mmv1.MeshFedConfig, sb *mmv1.ServiceBinding) string {
	return fmt.Sprintf("binding-%s-%s-intermesh", mfc.GetName(), sb.GetName())
}

func renderName(om *metav1.ObjectMeta) string {
	return fmt.Sprintf("%s.%s", om.GetName(), om.GetNamespace())
}

// boundaryProtectionRemoteDestinationRule returns something like
// https://github.com/istio-ecosystem/multi-mesh-examples/tree/master/add_hoc_limited_trust/http#consume-helloworld-v2-in-the-first-cluster
func boundaryProtectionRemoteDestinationRule(namespace string, mfc *mmv1.MeshFedConfig, sb *mmv1.ServiceBinding) v1alpha3.DestinationRule {
	return v1alpha3.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			Kind: "DestinationRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceRemoteName(mfc, sb),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(mfc.APIVersion, mfc.Kind, mfc.ObjectMeta),
		},
		Spec: v1alpha3.DestinationRuleSpec{
			DestinationRule: istiov1alpha3.DestinationRule{
				Host:     serviceRemoteName(mfc, sb),
				ExportTo: []string{"."},
				TrafficPolicy: &istiov1alpha3.TrafficPolicy{
					PortLevelSettings: []*istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
						&istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
							Port: &istiov1alpha3.PortSelector{
								Number: 15443,
							},
							Tls: &istiov1alpha3.TLSSettings{
								Mode:              istiov1alpha3.TLSSettings_MUTUAL,
								ClientCertificate: certificatesDir + "tls.crt",
								PrivateKey:        certificatesDir + "tls.key",
								CaCertificates:    certificatesDir + "example.com.crt", // TODO Where do I get this?
								Sni:               "c2.example.com",                    // TODO Where do I get this?
							},
						},
					},
				},
			},
		},
	}
}

func boundaryProtectionLocalServiceFacade(namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) corev1.Service {
	return corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      boundLocalName(sb),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
				"role": "local-facade",
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       int32(boundLocalPort(sb)),
					TargetPort: intstr.FromInt(int(boundLocalPort(sb))),
				},
			},
		},
	}
}

func boundaryProtectionLocalServiceEgress(gwSvcName, namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) corev1.Service {
	return corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwSvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
				"role": "local-service-egress",
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "https",
					Port: 443,
				},
			},
		},
	}
}

func boundaryProtectionLocalServiceGateway(gwSvcName, namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) v1alpha3.Gateway {
	return v1alpha3.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind: "Gateway",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("istio-%s-%s", mfc.GetName(), gwSvcName),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.GatewaySpec{
			Gateway: istiov1alpha3.Gateway{
				Selector: mfc.Spec.EgressGatewaySelector, // TODO Handle the case where we defaulted this
				Servers: []*istiov1alpha3.Server{
					{
						Port: &istiov1alpha3.Port{
							Name:     "tls",
							Number:   443,
							Protocol: "TLS",
						},
						Hosts: []string{
							fmt.Sprintf("%s.%s.svc.cluster.local", sb.Spec.Name, sb.Spec.Namespace),
						},
						Tls: &istiov1alpha3.Server_TLSOptions{
							Mode:              istiov1alpha3.Server_TLSOptions_MUTUAL,
							ServerCertificate: "/etc/certs/cert-chain.pem",
							PrivateKey:        "/etc/certs/key.pem",
							CaCertificates:    "/etc/certs/root-cert.pem",
						},
					},
				},
			},
		},
	}
}

func boundaryProtectionLocalServiceDestinationRule(gwSvcName, namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) v1alpha3.DestinationRule {
	return v1alpha3.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			Kind: "DestinationRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("istio-%s", mfc.GetName()),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.DestinationRuleSpec{
			DestinationRule: istiov1alpha3.DestinationRule{
				Host:     fmt.Sprintf("istio-%s-egress-%d.%s.svc.cluster.local", mfc.GetName(), int32(mfc.Spec.EgressGatewayPort), namespace),
				ExportTo: []string{"*"},
				Subsets: []*istiov1alpha3.Subset{
					{
						Name: serviceIntermeshName(sb.GetName()),
						TrafficPolicy: &istiov1alpha3.TrafficPolicy{
							LoadBalancer: &istiov1alpha3.LoadBalancerSettings{
								LbPolicy: &istiov1alpha3.LoadBalancerSettings_Simple{},
							},
							PortLevelSettings: []*istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
								&istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
									Port: &istiov1alpha3.PortSelector{
										Number: 443,
									},
									Tls: &istiov1alpha3.TLSSettings{
										Mode: istiov1alpha3.TLSSettings_ISTIO_MUTUAL,
										Sni:  fmt.Sprintf("%s.%s.svc.cluster.local", sb.Spec.Name, sb.Spec.Namespace),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func boundaryProtectionEgressExternalVirtualService(gwSvcName, namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) v1alpha3.VirtualService {

	return v1alpha3.VirtualService{
		TypeMeta: metav1.TypeMeta{
			Kind: "VirtualService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      gwSvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
				"role": "external",
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.VirtualServiceSpec{
			VirtualService: istiov1alpha3.VirtualService{
				Hosts:    []string{fmt.Sprintf("%s.%s.svc.cluster.local", sb.Spec.Name, sb.Spec.Namespace)},
				Gateways: []string{fmt.Sprintf("istio-%s-%s", mfc.GetName(), gwSvcName)},
				Tcp: []*istiov1alpha3.TCPRoute{
					{
						Match: []*istiov1alpha3.L4MatchAttributes{
							{
								Port: 443,
							},
						},
						Route: []*istiov1alpha3.RouteDestination{
							{
								Destination: &istiov1alpha3.Destination{
									Host: fmt.Sprintf("%s.%s.svc.cluster.local", serviceRemoteName(mfc, sb), namespace),
									Port: &istiov1alpha3.PortSelector{
										Number: 15443,
									},
									// Skip weight, it should default to 100 if left blank
								},
							},
						},
					},
				},
			},
		},
	}
}

func boundLocalName(sb *mmv1.ServiceBinding) string {
	if sb.Spec.Alias != "" {
		return sb.Spec.Alias
	}
	return sb.Spec.Name
}

func boundLocalPort(sb *mmv1.ServiceBinding) uint32 {
	if sb.Spec.Port != 0 {
		return sb.Spec.Port
	}
	return 5000 // TODO Is port mandatory?  If so, check it before calling this method
}

func boundaryProtectionLocalToEgressVirtualService(gwSvcName string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) v1alpha3.VirtualService {

	return v1alpha3.VirtualService{
		TypeMeta: metav1.TypeMeta{
			Kind: "VirtualService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      boundLocalName(sb),
			Namespace: sb.GetNamespace(),
			Labels: map[string]string{
				"mesh": mfc.GetName(),
				"role": "local-to-egress",
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.VirtualServiceSpec{
			VirtualService: istiov1alpha3.VirtualService{
				Hosts:    []string{boundLocalName(sb)},
				ExportTo: []string{"."},
				Http: []*istiov1alpha3.HTTPRoute{
					{
						Match: []*istiov1alpha3.HTTPMatchRequest{
							{
								Port: boundLocalPort(sb),
								Uri: &istiov1alpha3.StringMatch{
									MatchType: &istiov1alpha3.StringMatch_Prefix{Prefix: "/"},
								},
							},
						},
						Rewrite: &istiov1alpha3.HTTPRewrite{
							// See https://istio.io/docs/reference/config/networking/v1alpha3/virtual-service/#HTTPRewrite
							// This MUST match the ServiceExposition
							Uri: servicePathBinding(sb),
						},
						Route: []*istiov1alpha3.HTTPRouteDestination{
							{
								Destination: &istiov1alpha3.Destination{
									Host:   fmt.Sprintf("istio-%s-egress-%d.%s.svc.cluster.local", mfc.GetName(), int32(mfc.Spec.EgressGatewayPort), mfc.GetNamespace()),
									Subset: gwSvcName,
									Port: &istiov1alpha3.PortSelector{
										Number: 443,
									},
									// Skip weight, it should default to 100 if left blank
								},
							},
						},
					},
				},
			},
		},
	}
}

func servicePathBinding(sb *mmv1.ServiceBinding) string {
	return fmt.Sprintf("/%s/%s/", sb.GetNamespace(), sb.GetName())
}

func servicePathExposure(se *mmv1.ServiceExposition) string {
	if se.Spec.Alias != "" {
		return fmt.Sprintf("/%s/%s/", se.GetNamespace(), se.Spec.Alias)
	}
	return fmt.Sprintf("/%s/%s/", se.GetNamespace(), se.Spec.Name)
}

func serviceIntermeshName(name string) string {
	return fmt.Sprintf("%s-intermesh", name)
}

func egressImage() string {
	// TODO Get from Istio control plane deployment
	istioProxyImage := os.Getenv("ISTIO_PROXY_IMAGE")
	if istioProxyImage != "" {
		return istioProxyImage
	}

	return "docker.io/istio/proxyv2:1.2.5"
}

func ingressImage() string {
	// Istio uses same disk image for ingress and egress workloads
	return egressImage()
}

func egressServiceAccountName(mfcName string) string {
	return fmt.Sprintf("istio-%s-egressgateway-sa", mfcName)
}

func ingressServiceAccountName(mfcName string) string {
	return fmt.Sprintf("istio-%s-ingressgateway-sa", mfcName)
}
