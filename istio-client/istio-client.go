package istioclient

import (
	"log"

	versionedclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	"k8s.io/client-go/tools/clientcmd"
)

func GetIstioClient() *versionedclient.Clientset {
	// TODO: fix hardcode stuff
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
