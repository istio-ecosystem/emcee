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
	DEFAULT_PREFIX = ".svc.cluster.local"
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
		mfc.Spec.IngressGatewaySelector = map[string]string{
			"istio": "ingressgateway",
		}
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
			ingressGatewayPort = mfutil.DefaultGatewayPort
		}
		if len(mfc.Spec.IngressGatewaySelector) != 0 {
			gateway := istiov1alpha3.Gateway{
				Selector: mfc.Spec.IngressGatewaySelector,
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
							ServerCertificate: CERT_DIR + "tls.crt",
							PrivateKey:        CERT_DIR + "tls.key",
							CaCertificates:    CERT_DIR + "example.com.crt",
						},
					},
				},
			}
			if _, err := mfutil.CreateIstioGateway(bp.istioCli, se.GetName(), se.GetNamespace(), gateway, se.GetUID()); err != nil {
				return err
			}
		} else {
			// use an existing gateway
			// TODO
			return fmt.Errorf("unimplemented. Gateway proxy is not specified. Currently this is not supported.")
		}
	} else {
		// We should never get here. Boundry implementation is with ingress gateway always.
		return fmt.Errorf("Boundry implementation requires ingress gateway")
	}

	// Create an Istio Virtual Service
	name := se.GetName()
	namespace := se.GetNamespace()
	serviceName := se.Spec.Name
	fullname := serviceName + "." + namespace + DEFAULT_PREFIX
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
							MatchType: &istiov1alpha3.StringMatch_Prefix{Prefix: namespace + "/" + serviceName},
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

	// TODO: Get the gateway endpoints
	eps, err := mfutil.GetIngressEndpoints(ctx, bp.cli, mfc.GetName(), mfc.GetNamespace(), mfutil.DefaultGatewayPort)
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
	return nil
	// return fmt.Errorf("Unimplemented")
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
			Name:            fmt.Sprintf("istio-%s-egress-%d", name, port),
			Namespace:       namespace,
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
			Name:            fmt.Sprintf("istio-%s-ingress-%d", name, port),
			Namespace:       namespace,
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

func boundaryProtectionXServiceAccount(name, namespace string, owner *mmv1.MeshFedConfig) corev1.ServiceAccount {
	return corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind: "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: ownerReference(owner.APIVersion, owner.Kind, owner.ObjectMeta),
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
