// Licensed Materials - Property of IBM
// (C) Copyright IBM Corp. 2019. All Rights Reserved.
// US Government Users Restricted Rights - Use, duplication or
// disclosure restricted by GSA ADP Schedule Contract with IBM Corp.
// Copyright 2019 IBM Corporation

package boundary_protection

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	mmv1 "github.ibm.com/istio-research/mc2019/api/v1"
	"github.ibm.com/istio-research/mc2019/style"
	mfutil "github.ibm.com/istio-research/mc2019/util"

	istioclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	"istio.io/pkg/log"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
)

type bounderyProtection struct {
	cli      client.Client
	istioCli istioclient.Interface
}

var (
	// (compile-time check that we implement the interface)
	_ style.MeshFedConfig  = &bounderyProtection{}
	_ style.ServiceBinder  = &bounderyProtection{}
	_ style.ServiceExposer = &bounderyProtection{}
)

const (
	defaultPrefix = ".svc.cluster.local"
)

// NewBoundaryProtectionMeshFedConfig creates a "Boundary Protection" style implementation for handling MeshFedConfig
func NewBoundaryProtectionMeshFedConfig(cli client.Client, istioCli istioclient.Interface) style.MeshFedConfig {
	return &bounderyProtection{
		cli:      cli,
		istioCli: istioCli,
	}
}

// NewBoundaryProtectionServiceExposer creates a "Boundary Protection" style implementation for handling ServiceExposure
func NewBoundaryProtectionServiceExposer(cli client.Client, istioCli istioclient.Interface) style.ServiceExposer {
	return &bounderyProtection{
		cli:      cli,
		istioCli: istioCli,
	}
}

// NewBoundaryProtectionServiceBinder creates a "Boundary Protection" style implementation for handling ServiceBinding
func NewBoundaryProtectionServiceBinder(cli client.Client, istioCli istioclient.Interface) style.ServiceBinder {
	return &bounderyProtection{
		cli:      cli,
		istioCli: istioCli,
	}
}

// Implements Vadim-style
func (bp *bounderyProtection) EffectMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	// If the MeshFedConfig changes we may need to re-create all of the Istio
	// things for every ServiceBinding and ServiceExposition.  TODO Trigger
	// re-reconcile of every ServiceBinding and ServiceExposition.

	targetNamespace := mfc.GetNamespace()

	secret, err := getSecretName(ctx, mfc, bp.cli)
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

	err = bp.cli.Create(ctx, &egressSvc)
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
			"istio": "egressgateway",
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
	err = bp.cli.Create(ctx, &ingressSvc)
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

// Implements Vadim-style
func (bp *bounderyProtection) RemoveMeshFedConfig(ctx context.Context, mfc *mmv1.MeshFedConfig) error {
	return nil
}

// Implements Vadim-style
func (bp *bounderyProtection) EffectServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {

	// Create an Istio Gateway
	if mfc.Spec.UseIngressGateway {
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
		if _, err := mfutil.CreateIstioGateway(bp.istioCli, se.GetName(), se.GetNamespace(), gateway, se.GetUID()); err != nil {
			return err
		}
	} else {
		// We should never get here. Boundry implementation is with ingress gateway always.
		return fmt.Errorf("Boundry implementation requires ingress gateway")
	}

	// Create an Istio Virtual Service
	name := se.GetName()
	namespace := se.GetNamespace()
	serviceName := se.Spec.Name
	fullname := serviceName + "." + namespace + defaultPrefix
	vs := istiov1alpha3.VirtualService{
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
	if _, err := mfutil.CreateIstioVirtualService(bp.istioCli, name, namespace, vs, se.GetUID()); err != nil {
		// mfutil.DeleteIstioGateway(bp.istioCli, name, namespace)
		return err
	}

	eps, err := mfutil.GetIngressEndpoints(ctx, bp.cli, mfc.GetName(), mfc.GetNamespace(), defaultGatewayPort)
	if err != nil {
		log.Warnf("could not get endpoints %v %v", eps, err)
		return err
	}
	se.Spec.Endpoints = eps
	se.Status.Ready = true
	if err := bp.cli.Update(ctx, se); err != nil {
		return err
	}
	return nil
}

// Implements Vadim-style
func (bp *bounderyProtection) RemoveServiceExposure(ctx context.Context, se *mmv1.ServiceExposition, mfc *mmv1.MeshFedConfig) error {
	return nil
	// return fmt.Errorf("Unimplemented - service exposure delete")
}

// Implements Vadim-style
func (bp *bounderyProtection) EffectServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {

	// See https://github.com/istio-ecosystem/multi-mesh-examples/tree/master/add_hoc_limited_trust/http#consume-helloworld-v2-in-the-first-cluster

	targetNamespace := mfc.GetNamespace()

	// Create a Kubernetes service for the remote Ingress, if needed
	svcRemoteCluster := boundaryProtectionRemoteIngressService(targetNamespace, mfc)
	err := bp.cli.Create(ctx, &svcRemoteCluster)
	if logAndCheckExist(err, "Remote Cluster ingress Service", renderName(&svcRemoteCluster.ObjectMeta)) {
		return err
	}

	// Create a Kubernetes endpoint for the remote Ingress Service, if needed
	svcRemoteClusterEndpoint, err := boundaryProtectionRemoteIngressServiceEndpoint(targetNamespace, sb, mfc)
	if err != nil {
		log.Infof("Could not generate Remote Cluster ingress Service endpoint")
		return err
	}
	err = bp.cli.Create(ctx, svcRemoteClusterEndpoint)
	if logAndCheckExist(err, "Remote Cluster ingress Service endpoint", renderName(&svcRemoteClusterEndpoint.ObjectMeta)) {
		return err
	}
	// Create an Istio destination rule for the remote Ingress, if needed
	drRemoteCluster := boundaryProtectionRemoteDestinationRule(targetNamespace, mfc)
	_, err = bp.istioCli.NetworkingV1alpha3().DestinationRules(targetNamespace).Create(&drRemoteCluster)
	if logAndCheckExist(err, "Remote Cluster DestinationRule", renderName(&svcRemoteCluster.ObjectMeta)) {
		return err
	}

	svcLocalFacade := boundaryProtectionLocalServiceFacade(targetNamespace, sb, mfc)
	err = bp.cli.Create(ctx, &svcLocalFacade)
	if logAndCheckExist(err, "Local Service facade Service", renderName(&svcLocalFacade.ObjectMeta)) {
		return err
	}

	comboName := "hw-c2" // TODO This combines the service and Ingress name, do we have an algorithm to generate?
	svcLocalEgress := boundaryProtectionLocalServiceEgress(comboName, targetNamespace, sb, mfc)
	err = bp.cli.Create(ctx, &svcLocalEgress)
	if logAndCheckExist(err, "Local Service egress Service", renderName(&svcLocalEgress.ObjectMeta)) {
		return err
	}

	svcLocalGateway := boundaryProtectionLocalServiceGateway(comboName, targetNamespace, sb, mfc)
	_, err = bp.istioCli.NetworkingV1alpha3().Gateways(targetNamespace).Create(&svcLocalGateway)
	if logAndCheckExist(err, "Local Service egress Gateway", renderName(&svcLocalGateway.ObjectMeta)) {
		return err
	}

	svcLocalDR := boundaryProtectionLocalServiceDestinationRule(comboName, targetNamespace, sb, mfc)
	_, err = bp.istioCli.NetworkingV1alpha3().DestinationRules(targetNamespace).Create(&svcLocalDR)
	if logAndCheckExist(err, "Local Service egress DestinationRule", renderName(&svcLocalDR.ObjectMeta)) {
		return err
	}

	vsEgressExternal := boundaryProtectionEgressExternalVirtualService(comboName, targetNamespace, sb, mfc)
	_, err = bp.istioCli.NetworkingV1alpha3().VirtualServices(targetNamespace).Create(&vsEgressExternal)
	if logAndCheckExist(err, "Local Service departure egress VirtualService", renderName(&vsEgressExternal.ObjectMeta)) {
		return err
	}

	vsLocalToEgress := boundaryProtectionLocalToEgressVirtualService(comboName, targetNamespace, sb, mfc)
	_, err = bp.istioCli.NetworkingV1alpha3().VirtualServices(targetNamespace).Create(&vsLocalToEgress)
	if logAndCheckExist(err, "Local Service to egress VirtualService", renderName(&vsLocalToEgress.ObjectMeta)) {
		return err
	}

	return nil
}

// Implements Vadim-style
func (bp *bounderyProtection) RemoveServiceBinding(ctx context.Context, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) error {
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
	return boundaryProtectionXServiceAccount(fmt.Sprintf("istio-%s-egressgateway-service-account", name),
		namespace, owner)
}

// TODO We currently hard-code this ServiceAccount rather than using Istio Operator to create
// one congruent with user's Istio installation.  We should use Operator, but it is
// not set up to create an ingress/egress w/o control plane
func boundaryProtectionIngressServiceAccount(name, namespace string, owner *mmv1.MeshFedConfig) corev1.ServiceAccount {
	return boundaryProtectionXServiceAccount(fmt.Sprintf("istio-%s-ingressgateway-service-account", name),
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
						"heritage":                "mc2019",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "istio-proxy",
							Args:  boundaryProtectionPodArgs("istio-private-egressgateway"),
							Env:   boundaryProtectionPodEnv(labels, "istio-private-egressgateway"),
							Image: "docker.io/istio/proxyv2:1.4.0-beta.1", // TODO Get from Istio
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
						"heritage":                "mc2019",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "istio-proxy",
							Args:  boundaryProtectionPodArgs("istio-private-ingressgateway"),
							Env:   boundaryProtectionPodEnv(labels, "istio-private-ingressgateway"),
							Image: "docker.io/istio/proxyv2:1.4.0-beta.1", // TODO Get from Istio
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

func (bp *bounderyProtection) workloadMatches(ctx context.Context, namespace string, selector labels.Selector) (int, error) {
	var matches corev1.PodList
	err := bp.cli.List(ctx, &matches, &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: selector,
	})
	if err != nil {
		return 0, err
	}
	// TODO exclude terminating pods from this count?
	return len(matches.Items), nil
}

func (bp *bounderyProtection) createEgressDeployment(ctx context.Context, mfc *mmv1.MeshFedConfig, targetNamespace, secret string) error {
	egressSA := boundaryProtectionEgressServiceAccount(mfc.GetName(),
		targetNamespace, mfc)
	err := bp.cli.Create(ctx, &egressSA)
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
	err = bp.cli.Create(ctx, &egressDeployment)
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

func (bp *bounderyProtection) createIngressDeployment(ctx context.Context, mfc *mmv1.MeshFedConfig, targetNamespace, secret string) error {
	ingressSA := boundaryProtectionIngressServiceAccount(mfc.GetName()+"-ingressgateway",
		targetNamespace, mfc)
	err := bp.cli.Create(ctx, &ingressSA)
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
	err = bp.cli.Create(ctx, &ingressDeployment)
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

func boundaryProtectionRemoteIngressService(namespace string, mfc *mmv1.MeshFedConfig) corev1.Service {

	port := mfc.Spec.IngressGatewayPort

	return corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceRemoteName(mfc),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(mfc.APIVersion, mfc.Kind, mfc.ObjectMeta),
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None",
			Ports: []corev1.ServicePort{
				{
					Name: "tls-for-cross-cluster-communication",
					Port: int32(port),
				},
			},
		},
	}
}

func boundaryProtectionRemoteIngressServiceEndpoint(namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) (*corev1.Endpoints, error) {

	// Note that we use sb.Spec.Endpoint port, not mfc.Spec.IngressGatewayPort

	addresses := []corev1.EndpointAddress{}
	ports := []corev1.EndpointPort{}
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
		addresses = append(addresses, corev1.EndpointAddress{
			IP: parts[0],
		})
		ports = append(ports, corev1.EndpointPort{
			Name: fmt.Sprintf("tls-%d", port),
			Port: int32(port),
		})
	}

	return &corev1.Endpoints{
		TypeMeta: metav1.TypeMeta{
			Kind: "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceRemoteName(mfc),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(mfc.APIVersion, mfc.Kind, mfc.ObjectMeta),
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: addresses,
				Ports:     ports,
			},
		},
	}, nil
}

func serviceRemoteName(mfc *mmv1.MeshFedConfig) string {
	return fmt.Sprintf("binding-%s", mfc.GetName())
}

func renderName(om *metav1.ObjectMeta) string {
	return fmt.Sprintf("%s.%s", om.GetName(), om.GetNamespace())
}

// boundaryProtectionRemoteDestinationRule returns something like
// https://github.com/istio-ecosystem/multi-mesh-examples/tree/master/add_hoc_limited_trust/http#consume-helloworld-v2-in-the-first-cluster
func boundaryProtectionRemoteDestinationRule(namespace string, mfc *mmv1.MeshFedConfig) v1alpha3.DestinationRule {
	return v1alpha3.DestinationRule{
		TypeMeta: metav1.TypeMeta{
			Kind: "DestinationRule",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceRemoteName(mfc),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(mfc.APIVersion, mfc.Kind, mfc.ObjectMeta),
		},
		Spec: v1alpha3.DestinationRuleSpec{
			DestinationRule: istiov1alpha3.DestinationRule{
				Host:     serviceRemoteName(mfc),
				ExportTo: []string{"."},
				TrafficPolicy: &istiov1alpha3.TrafficPolicy{
					PortLevelSettings: []*istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
						&istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
							Port: &istiov1alpha3.PortSelector{
								Number: 15443,
							},
							Tls: &istiov1alpha3.TLSSettings{
								Mode:              istiov1alpha3.TLSSettings_MUTUAL,
								ClientCertificate: "/etc/istio/mesh/certs/tls.crt",
								PrivateKey:        "/etc/istio/mesh/certs/tls.key",
								CaCertificates:    "/etc/istio/mesh/example.com.crt", // TODO Where do I get this?
								Sni:               "c2.example.com",                  // TODO Where do I get this?
							},
						},
					},
				},
			},
		},
	}
}

// logAndCheckExist returns true if we failed in a bad way
func logAndCheckExist(err error, title, name string) bool {
	if err != nil && !mfutil.ErrorAlreadyExists(err) {
		log.Infof("Failed to create %s %s: %v", title, name, err)
		return true
	}
	if err == nil {
		log.Infof("Created %s %s", title, name)
	}
	return false
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
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "http",
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
							fmt.Sprintf("%s.%s.svc.cluster.local", gwSvcName, namespace),
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
				Host: fmt.Sprintf("istio-%s.%s.svc.cluster.local", mfc.GetName(), namespace),
				TrafficPolicy: &istiov1alpha3.TrafficPolicy{
					PortLevelSettings: []*istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
						&istiov1alpha3.TrafficPolicy_PortTrafficPolicy{
							Port: &istiov1alpha3.PortSelector{
								Number: 443,
							},
							Tls: &istiov1alpha3.TLSSettings{
								Mode: istiov1alpha3.TLSSettings_ISTIO_MUTUAL,
								Sni:  fmt.Sprintf("%s.%s.svc.cluster.local", gwSvcName, namespace),
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
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.VirtualServiceSpec{
			VirtualService: istiov1alpha3.VirtualService{
				Hosts:    []string{fmt.Sprintf("%s.%s.svc.cluster.local", gwSvcName, namespace)},
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
									Host: fmt.Sprintf("%s.%s.svc.cluster.local", serviceRemoteName(mfc), namespace),
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

func boundaryProtectionLocalToEgressVirtualService(gwSvcName, namespace string, sb *mmv1.ServiceBinding, mfc *mmv1.MeshFedConfig) v1alpha3.VirtualService {

	return v1alpha3.VirtualService{
		TypeMeta: metav1.TypeMeta{
			Kind: "VirtualService",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      boundLocalName(sb),
			Namespace: namespace,
			Labels: map[string]string{
				"mesh": mfc.GetName(),
			},
			OwnerReferences: ownerReference(sb.APIVersion, sb.Kind, sb.ObjectMeta),
		},
		Spec: v1alpha3.VirtualServiceSpec{
			VirtualService: istiov1alpha3.VirtualService{
				Hosts: []string{boundLocalName(sb)},
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
									Host:   fmt.Sprintf("istio-%s.%s.svc.cluster.local", mfc.GetName(), namespace),
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

func servicePath(name string) string {
	// TODO Escape / in sb.Spec.Name
	return fmt.Sprintf("/%s/", name)
}

func servicePathBinding(sb *mmv1.ServiceBinding) string {
	return servicePath(sb.Spec.Name)
}

func servicePathExposure(se *mmv1.ServiceExposition) string {
	return servicePath(se.Spec.Name)
}
