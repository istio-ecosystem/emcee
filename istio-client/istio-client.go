package istioclient

import (
	"log"

	v1alpha3 "github.com/aspenmesh/istio-client-go/pkg/apis/networking/v1alpha3"
	versionedclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func GetClient() *versionedclient.Clientset {
	log.Printf("**************************")
	kubeconfig := "/Users/mb/.bluemix/plugins/container-service/clusters/istio-test-paid2/kube-config-dal13-istio-test-paid2.yml"
	namespace := "default"
	if len(kubeconfig) == 0 || len(namespace) == 0 {
		log.Fatalf("Environment variables KUBECONFIG and NAMESPACE need to be set")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Failed to create k8s rest client: %s", err)
	}

	ic, err := versionedclient.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("Failed to create istio client: %s", err)
		return nil
	}
	return ic
}

func GetVS(ic *versionedclient.Clientset) {
	vsList, err := ic.NetworkingV1alpha3().VirtualServices("default").List(metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to get VirtualService in %s namespace: %s", "default", err)
	}
	for i := range vsList.Items {
		vs := vsList.Items[i]
		log.Printf("Index: %d VirtualService Hosts: %+v\n", i, vs.Spec.GetHosts())
	}

	gw := &v1alpha3.Gateway{
		TypeMeta: metav1.TypeMeta{
			Kind: "gateway",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "namespace",
			Name:      "nameHAHAHAHA",
		},
		Spec: v1alpha3.GatewaySpec{},
	}
	a, r := ic.NetworkingV1alpha3().Gateways("default").Create(gw)
	log.Printf("/////////////////// %v %v\n", a, r)
}

func Mytest() {
	log.Printf("**************************")
	kubeconfig := "/Users/mb/.bluemix/plugins/container-service/clusters/istio-test-paid2/kube-config-dal13-istio-test-paid2.yml"
	namespace := "default"
	if len(kubeconfig) == 0 || len(namespace) == 0 {
		log.Fatalf("Environment variables KUBECONFIG and NAMESPACE need to be set")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		log.Fatalf("Failed to create k8s rest client: %s", err)
	}

	ic, err := versionedclient.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("Failed to create istio client: %s", err)
	}
	// Test VirtualServices
	vsList, err := ic.NetworkingV1alpha3().VirtualServices(namespace).List(metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to get VirtualService in %s namespace: %s", namespace, err)
	}
	for i := range vsList.Items {
		vs := vsList.Items[i]
		log.Printf("Index: %d VirtualService Hosts: %+v\n", i, vs.Spec.GetHosts())
	}

	// Test DestinationRules
	drList, err := ic.NetworkingV1alpha3().DestinationRules(namespace).List(metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to get DestinationRule in %s namespace: %s", namespace, err)
	}
	for i := range drList.Items {
		dr := drList.Items[i]
		log.Printf("Index: %d DestinationRule Host: %+v\n", i, dr.Spec.GetHost())
	}

	// Test Policies
	pList, err := ic.AuthenticationV1alpha1().Policies(namespace).List(metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to get Policy in %s namespace: %s", namespace, err)
	}
	for i := range pList.Items {
		p := pList.Items[i]
		log.Printf("Index: %d Policy Targets: %+v\n", i, p.Spec.GetTargets())
	}

	// Test MeshPolicies
	mpList, err := ic.AuthenticationV1alpha1().MeshPolicies().List(metav1.ListOptions{})
	if err != nil {
		log.Fatal("Failed to list MeshPolicies", err)
	}
	for i := range mpList.Items {
		mp := mpList.Items[i]
		log.Printf("Index: %d MeshPolicy Name: %+v\n", i, mp.ObjectMeta.Name)

		// Known broken without the custom marshal/unmarshal code
		log.Printf("Index %d MeshPolicy Value: %+v\n", i, mp.Spec.Policy.Peers)
		_, err := ic.AuthenticationV1alpha1().MeshPolicies().Get(mp.ObjectMeta.Name, metav1.GetOptions{})
		if err != nil {
			log.Fatalf("Failed to get MeshPolicy named %s", mp.ObjectMeta.Name)
		}
	}

	// Test Gateway
	log.Printf("************************** 2")
	gwList, err := ic.NetworkingV1alpha3().Gateways(namespace).List(metav1.ListOptions{})
	log.Printf("************************** 3")
	if err != nil {
		log.Printf("************************** 4")
		log.Fatalf("Failed to get Gateway in %s namespace: %s", namespace, err)
	}
	log.Printf("************************** 5")
	for i := range gwList.Items {
		gw := gwList.Items[i]
		for _, s := range gw.Spec.GetServers() {
			log.Printf("************************** 6")
			log.Printf("Index: %d Gateway servers: %+v\n", i, s)
		}
	}

	// Test ServiceEntry
	seList, err := ic.NetworkingV1alpha3().ServiceEntries(namespace).List(metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to get ServiceEntry in %s namespace: %s", namespace, err)
	}
	for i := range seList.Items {
		se := seList.Items[i]
		for _, h := range se.Spec.GetHosts() {
			log.Printf("Index: %d ServiceEntry hosts: %+v\n", i, h)
		}
	}
	// Test ServiceRoles
	srList, err := ic.RbacV1alpha1().ServiceRoles(namespace).List(metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to get ServiceRole in %s namespace: %s", namespace, err)

	}

	for i := range srList.Items {
		sr := srList.Items[i]
		log.Printf("Index: %d ServiceRole Name: %+v\n", i, sr.Name)

		for _, h := range sr.Spec.GetRules() {
			log.Printf("Index: %d ServiceRole Rules: %+v\n", i, h)
		}
	}
	// Test ServiceRoleBinding
	srbList, err := ic.RbacV1alpha1().ServiceRoleBindings(namespace).List(metav1.ListOptions{})
	if err != nil {
		log.Fatalf("Failed to get ServiceRoleBinding in %s namespace: %s", namespace, err)

	}
	for i := range srbList.Items {
		srb := srbList.Items[i]
		log.Printf("ServiceRoleBindings Name: %+v\nServiceRoleReference: %+v\n", srb.Name, srb.Spec.GetRoleRef())

	}

}
