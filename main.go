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
	"io/ioutil"
	"os"

	"k8s.io/client-go/tools/clientcmd"

	v2 "github.com/metal-stack/duros-go/api/duros/v2"
	"github.com/metal-stack/v"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	storagev1 "github.com/metal-stack/duros-controller/api/v1"
	"github.com/metal-stack/duros-controller/controllers"
	duros "github.com/metal-stack/duros-go"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = storagev1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr          string
		enableLeaderElection bool
		shootKubeconfig      string
		adminToken           string
		adminKey             string
		endpoints            string
		namespace            string
	)
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&namespace, "namespace", "default", "The namespace this controller is running.")
	flag.StringVar(&shootKubeconfig, "shoot-kubeconfig", "", "The path to the kubeconfig to talk to the shoot")
	flag.StringVar(&adminToken, "admin-token", "/duros/admin-token", "The admin token file for the duros api.")
	flag.StringVar(&adminKey, "admin-key", "/duros/admin-key", "The admin key file for the duros api.")
	flag.StringVar(&endpoints, "endpoints", "", "The endpoints, in the form host:port,host:port of the duros api.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	restConfig := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "duros-controller-leader-election",
		Namespace:          namespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start duros-controller")
		os.Exit(1)
	}

	shootClient := mgr.GetClient()
	if len(shootKubeconfig) > 0 {
		shootRestConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: shootKubeconfig},
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
		if err != nil {
			setupLog.Error(err, "unable to create shoot restconfig")
			os.Exit(1)
		}
		shootClient, err = client.New(shootRestConfig, client.Options{})
		if err != nil {
			setupLog.Error(err, "unable to create shoot client")
			os.Exit(1)
		}
	}

	// connect to duros

	at, err := ioutil.ReadFile(adminToken)
	if err != nil {
		setupLog.Error(err, "unable to read admin-token from file")
		panic(err)
	}
	ak, err := ioutil.ReadFile(adminKey)
	if err != nil {
		setupLog.Error(err, "unable to read admin-key from file")
		panic(err)
	}
	ctx := context.Background()
	durosEPs := duros.MustParseCSV(endpoints)
	durosConfig := duros.DialConfig{
		Token:     string(at),
		Endpoints: durosEPs,
		Scheme:    duros.GRPCS,
		Log:       zap.NewRaw().Sugar(),
	}
	durosClient, err := duros.Dial(ctx, durosConfig)
	if err != nil {
		setupLog.Error(err, "problem running duros-controller")
		panic(err)
	}
	version, err := durosClient.GetVersion(ctx, &v2.GetVersionRequest{})
	if err != nil {
		setupLog.Error(err, "unable to connect to duros")
		panic(err)
	}
	cinfo, err := durosClient.GetClusterInfo(ctx, &v2.GetClusterRequest{})
	if err != nil {
		setupLog.Error(err, "unable to query duros api for cluster info")
		panic(err)
	}
	setupLog.Info("connected", "duros version", version.ApiVersion, "cluster", cinfo.ApiEndpoints)
	if err = (&controllers.DurosReconciler{
		Client:      mgr.GetClient(),
		Shoot:       shootClient,
		Scheme:      mgr.GetScheme(),
		Namespace:   namespace,
		DurosClient: durosClient,
		Endpoints:   durosEPs,
		AdminKey:    ak,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "LightBits")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting duros-controller", "version", v.V)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running duros-controller")
		os.Exit(1)
	}
}
