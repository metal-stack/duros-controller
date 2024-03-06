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
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/go-logr/logr"
	v2 "github.com/metal-stack/duros-go/api/duros/v2"
	"github.com/metal-stack/v"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	duroscontrollerv1 "github.com/metal-stack/duros-controller/api/v1"
	"github.com/metal-stack/duros-controller/controllers"
	"github.com/metal-stack/duros-go"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(duroscontrollerv1.AddToScheme(scheme))
	utilruntime.Must(snapshotv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		logLevel             string
		metricsAddr          string
		enableLeaderElection bool
		shootKubeconfig      string

		csiCtrlShootAccessSecretName       string
		csiCtrlGenericKubeconfigSecretName string

		adminToken string
		adminKey   string
		endpoints  string
		namespace  string
		// apiEndpoint is the duros-grpc-proxy with client cert validation
		apiEndpoint string
		apiCA       string
		apiKey      string
		apiCert     string

		// PSP disabled for k8s v1.25 migration
		pspDisabled bool
	)
	flag.StringVar(&logLevel, "log-level", "", "The log level of the controller.")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&namespace, "namespace", "default", "The namespace this controller is running.")
	flag.StringVar(&shootKubeconfig, "shoot-kubeconfig", "", "The path to the kubeconfig to talk to the shoot")
	flag.StringVar(&csiCtrlShootAccessSecretName, "lb-csi-ctrl-shoot-access-secret-name", "", "The name of the shoot access secret for the lb-csi-controller.")
	flag.StringVar(&csiCtrlGenericKubeconfigSecretName, "lb-csi-ctrl-generic-kubeconfig-secret-name", "", "The name of the generic kubeconfig secret for the lb-csi-controller.")
	flag.StringVar(&adminToken, "admin-token", "/duros/admin-token", "The admin token file for the duros api.")
	flag.StringVar(&adminKey, "admin-key", "/duros/admin-key", "The admin key file for the duros api.")
	flag.StringVar(&endpoints, "endpoints", "", "The endpoints, in the form host:port,host:port of the duros api.")

	flag.StringVar(&apiEndpoint, "api-endpoint", "", "The api endpoint, in the form host:port of the duros api")
	flag.StringVar(&apiCA, "api-ca", "", "The api endpoint ca")
	flag.StringVar(&apiCert, "api-cert", "", "The api endpoint cert")
	flag.StringVar(&apiKey, "api-key", "", "The api endpoint key")
	flag.BoolVar(&pspDisabled, "psp-disabled", false, "if set to true, deployment of PSP related objects is disabled")

	flag.Parse()

	level := slog.LevelInfo
	if len(logLevel) > 0 {
		var lvlvar slog.LevelVar
		err := lvlvar.UnmarshalText([]byte(logLevel))
		if err != nil {
			setupLog.Error(err, "can't initialize zap logger")
			os.Exit(1)
		}
		level = lvlvar.Level()
	}

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	l := slog.New(jsonHandler)

	ctrl.SetLogger(logr.FromSlogHandler(jsonHandler))

	restConfig := ctrl.GetConfigOrDie()

	disabledTimeout := time.Duration(-1) // wait for all runnables to finish before dying
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      metricsAddr,
		Port:                    9443,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "duros-controller-leader-election",
		Namespace:               namespace,
		GracefulShutdownTimeout: &disabledTimeout,
	})
	if err != nil {
		setupLog.Error(err, "unable to start duros-controller")
		os.Exit(1)
	}

	shootClient := mgr.GetClient()
	var (
		discoveryClient *discovery.DiscoveryClient
	)
	if len(shootKubeconfig) > 0 {
		shootRestConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: shootKubeconfig},
			&clientcmd.ConfigOverrides{},
		).ClientConfig()
		if err != nil {
			setupLog.Error(err, "unable to create shoot restconfig")
			os.Exit(1)
		}
		shootClient, err = client.New(shootRestConfig, client.Options{Scheme: scheme})
		if err != nil {
			setupLog.Error(err, "unable to create shoot client")
			os.Exit(1)
		}
		discoveryClient, err = discovery.NewDiscoveryClientForConfig(shootRestConfig)
		if err != nil {
			setupLog.Error(err, "unable to create shoot discovery client")
			os.Exit(1)
		}
	}

	// connect to duros

	at, err := os.ReadFile(adminToken)
	if err != nil {
		setupLog.Error(err, "unable to read admin-token from file")
		os.Exit(1)
	}
	ak, err := os.ReadFile(adminKey)
	if err != nil {
		setupLog.Error(err, "unable to read admin-key from file")
		os.Exit(1)
	}
	ctx := context.Background()
	if err := validateEndpoints(apiEndpoint); err != nil {
		setupLog.Error(err, "unable to parse api-endpoint")
		os.Exit(1)
	}
	if err := validateEndpoints(endpoints); err != nil {
		setupLog.Error(err, "unable to parse endpoints")
		os.Exit(1)
	}
	durosConfig := duros.DialConfig{
		Token:     string(at),
		Endpoint:  apiEndpoint,
		Scheme:    duros.GRPCS,
		Log:       l,
		UserAgent: "duros-controller",
	}

	if apiCA != "" && apiCert != "" && apiKey != "" {
		setupLog.Info("connecting to api with client cert", "api-endpoint", apiEndpoint)
		ac, err := os.ReadFile(apiCA)
		if err != nil {
			setupLog.Error(err, "unable to read api-ca from file")
			os.Exit(1)
		}
		ace, err := os.ReadFile(apiCert)
		if err != nil {
			setupLog.Error(err, "unable to read api-cert from file")
			os.Exit(1)
		}
		ak, err := os.ReadFile(apiKey)
		if err != nil {
			setupLog.Error(err, "unable to read api-key from file")
			os.Exit(1)
		}
		serverName, _, err := net.SplitHostPort(apiEndpoint)
		if err != nil {
			setupLog.Error(err, "unable to parse api-endpoint")
			os.Exit(1)
		}

		creds := &duros.ByteCredentials{
			CA:         ac,
			Cert:       ace,
			Key:        ak,
			ServerName: serverName,
		}
		durosConfig.ByteCredentials = creds
	}

	durosClient, err := duros.Dial(ctx, durosConfig)
	if err != nil {
		setupLog.Error(err, "problem running duros-controller")
		os.Exit(1)
	}
	version, err := durosClient.GetVersion(ctx, &v2.GetVersionRequest{})
	if err != nil {
		setupLog.Error(err, "unable to connect to duros")
		os.Exit(1)
	}
	cinfo, err := durosClient.GetClusterInfo(ctx, &v2.GetClusterRequest{})
	if err != nil {
		setupLog.Error(err, "unable to query duros api for cluster info")
		os.Exit(1)
	}
	setupLog.Info("connected", "duros version", version.GetApiVersion(), "cluster", cinfo.GetApiEndpoints())
	if err = (&controllers.DurosReconciler{
		Seed:                               mgr.GetClient(),
		Shoot:                              shootClient,
		DiscoveryClient:                    discoveryClient,
		Log:                                ctrl.Log.WithName("controllers").WithName("LightBits"),
		Namespace:                          namespace,
		DurosClient:                        durosClient,
		Endpoints:                          endpoints,
		AdminKey:                           ak,
		PSPDisabled:                        pspDisabled,
		CsiCtrlShootAccessSecretName:       csiCtrlShootAccessSecretName,
		CsiCtrlGenericKubeconfigSecretName: csiCtrlGenericKubeconfigSecretName,
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

func validateEndpoints(endpoints string) error {
	for _, endpoint := range strings.Split(endpoints, ",") {
		host, port, err := net.SplitHostPort(strings.TrimSpace(endpoint))
		if err != nil {
			return err
		}
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("invalid empty host")
		}
		if _, err = strconv.ParseUint(port, 10, 16); err != nil {
			return err
		}
	}
	return nil
}
