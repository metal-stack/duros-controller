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
	"os"
	"time"

	"github.com/metal-stack/duros-controller/pkg/manifest"
	_ "github.com/metal-stack/duros-controller/statik"
	v2 "github.com/metal-stack/duros-go/api/duros/v2"
	"github.com/metal-stack/v"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	ctrl "sigs.k8s.io/controller-runtime"
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
		adminToken           string
		endpoints            string
		namespace            string
	)
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&namespace, "namespace", "default", "The namespace this controller is running.")
	flag.StringVar(&adminToken, "admin-token", "", "The admin token for the duros api.")
	flag.StringVar(&endpoints, "endpoints", "", "The endpoints, in the form host:port,host:port of the duros api.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	restConfig := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "99f32e9a.metal-stack.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start duros-controller")
		os.Exit(1)
	}

	// Install required CRD
	manifests, err := manifest.InstallManifests(restConfig, manifest.InstallOptions{
		UseVFS: true,
		Paths:  []string{"/crd/bases"},
	})
	if err != nil {
		setupLog.Error(err, "unable to create crds of duros-controller")
		os.Exit(1)
	}

	err = manifest.WaitForManifests(restConfig, manifests, manifest.InstallOptions{MaxTime: 500 * time.Millisecond, PollInterval: 100 * time.Millisecond})
	if err != nil {
		setupLog.Error(err, "unable to wait for created crds of duros-controller")
		os.Exit(1)
	}

	// connect to duros
	ctx := context.Background()
	durosEPs := duros.MustParseCSV(endpoints)
	durosClient, err := duros.Dial(ctx, durosEPs, duros.GRPCS, adminToken, zap.NewRaw().Sugar())
	if err != nil {
		setupLog.Error(err, "problem running duros-controller")
		panic(err)
	}
	version, err := durosClient.GetVersion(ctx, &v2.GetVersionRequest{})
	if err != nil {
		setupLog.Error(err, "unable to connect to duros")
		panic(err)
	}
	setupLog.Info("conected to duros version:%q", version.ApiVersion)
	if err = (&controllers.DurosReconciler{
		Client:      mgr.GetClient(),
		Log:         ctrl.Log.WithName("controllers").WithName("LightBits"),
		Scheme:      mgr.GetScheme(),
		Namespace:   namespace,
		DurosClient: durosClient,
		Endpoints:   durosEPs,
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
