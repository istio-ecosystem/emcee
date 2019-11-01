package istioclient

import (
	"log"
	"os"

	versionedclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"
	"k8s.io/client-go/tools/clientcmd"
)

// GetIstioClient creates an Istio client based on Kubernetes API server $KUBECONFIG
func GetIstioClient() *versionedclient.Clientset {
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		log.Fatalf("Environment variables KUBECONFIG must be set")
	}
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "default"
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
