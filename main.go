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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	versionedclient "github.com/aspenmesh/istio-client-go/pkg/client/clientset/versioned"

	mmv1 "github.com/istio-ecosystem/emcee/api/v1"
	"github.com/istio-ecosystem/emcee/controllers"
	"github.com/istio-ecosystem/emcee/pkg/discovery"
	mfutil "github.com/istio-ecosystem/emcee/util"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

const (
	grpcServerAddress      = ":50051"
	discoveryLabel         = "emcee:discovery"
	emceeAutoExposeLabel   = "emcee.io/expose"
	emceeAutoExposeAsLabel = "emcee.io/exposeAs"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = mmv1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr           string
		k8sContext            string
		enableLeaderElection  bool
		grpcServerAddr        string
		grpcDiscoveryLabel    string
		grpcDiscoveryLabelKey string
		grpcDiscoveryLabelVal string
		autoExposeLabel       string
		autoExposeLabelKey    string
		autoExposeAsLabel     string
		autoExposeAsLabelKey  string
	)
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&k8sContext, "context", "", "Kubernetes context")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&grpcServerAddr, "grpc-server-addr", grpcServerAddress, "The address the grpc server endpoint binds to.")
	flag.StringVar(&grpcDiscoveryLabel, "discovery-label", discoveryLabel, "The label for grpc servers to connect to.")
	flag.StringVar(&autoExposeLabel, "auto-expose-label", emceeAutoExposeLabel, "The label for auto exposing a service.")
	flag.StringVar(&autoExposeAsLabel, "exposeAs-label", emceeAutoExposeAsLabel, "The label for auto exposing a service as a different service.")
	flag.Parse()

	ctrl.SetLogger(zap.Logger(true))

	cfg, err := mfutil.GetRestConfig("", k8sContext)
	if err != nil {
		setupLog.Error(err, "unable to read config", "context", k8sContext)
		os.Exit(1)
	}
	setupLog.Info("Loaded config", "context", k8sContext)

	s := strings.Split(grpcDiscoveryLabel, ":")
	if len(s) == 2 {
		grpcDiscoveryLabelKey = s[0]
		grpcDiscoveryLabelVal = s[1]
	}

	autoExposeLabelKey = emceeAutoExposeLabel
	autoExposeAsLabelKey = emceeAutoExposeAsLabel

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	kclient := mgr.GetClient()

	istioClient, err := versionedclient.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "unable to create Istio client")
		os.Exit(1)
	}

	if err = (&controllers.MeshFedConfigReconciler{
		Client:    kclient,
		Interface: istioClient,
		//Log:    ctrl.Log.WithName("controllers").WithName("MeshFedConfig"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MeshFedConfig")
		os.Exit(1)
	}

	ser := controllers.ServiceExpositionReconciler{
		Client:    kclient,
		Interface: istioClient,
		//Log:    ctrl.Log.WithName("controllers").WithName("ServiceExposition"),
	}
	if err = (&ser).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceExposition")
		os.Exit(1)
	}

	sbr := controllers.ServiceBindingReconciler{
		Client:    kclient,
		Interface: istioClient,
		//Log:    ctrl.Log.WithName("controllers").WithName("ServiceBinding"),
	}
	if err = (&sbr).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ServiceBinding")
		os.Exit(1)
	}

	svcr := controllers.ServiceReconciler{
		Client:               kclient,
		Interface:            istioClient,
		DiscoveryLabelKey:    grpcDiscoveryLabelKey,
		DiscoveryLabelVal:    grpcDiscoveryLabelVal,
		AutoExposeLabelKey:   autoExposeLabelKey,
		AutoExposeAsLabelKey: autoExposeAsLabelKey,
		SEReconciler:         &ser,
		//Log:    ctrl.Log.WithName("controllers").WithName("Service"),
	}

	if err = (&svcr).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Service")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	ctx := context.Background()
	go discovery.Discovery(&ser, &grpcServerAddr)
	go discovery.ClientStarter(ctx, &sbr, &svcr, controllers.DiscoveryChanel)

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	fmt.Printf("Terminating Emcee manager\n")
}
