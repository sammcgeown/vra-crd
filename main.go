/*
Copyright 2022.

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
	"flag"
	"net/http"
	neturl "net/url"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	httptransport "github.com/go-openapi/runtime/client"
	"github.com/go-openapi/strfmt"
	machinev1alpha1 "github.com/sammcgeown/vra/api/v1alpha1"

	"github.com/sammcgeown/vra/controllers"
	vraclient "github.com/vmware/vra-sdk-go/pkg/client"
	"github.com/vmware/vra-sdk-go/pkg/client/login"
	"github.com/vmware/vra-sdk-go/pkg/models"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(machinev1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	// var metricsAddr string
	// var enableLeaderElection bool
	// var probeAddr string
	var configFile string
	flag.StringVar(&configFile, "config", "",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.")

	// flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	// flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	// flag.BoolVar(&enableLeaderElection, "leader-elect", false,
	// 	"Enable leader election for controller manager. "+
	// 		"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	var err error
	ctrlConfig := machinev1alpha1.ProjectConfig{}
	options := ctrl.Options{Scheme: scheme}
	if configFile != "" {
		options, err = options.AndFrom(ctrl.ConfigFile().AtPath(configFile).OfKind(&ctrlConfig))
		if err != nil {
			setupLog.Error(err, "unable to load the config file")
			os.Exit(1)
		}
	}
	ctrlConfig.RefreshToken = os.Getenv("VRA_REFRESH_TOKEN")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Get vRA Client
	vra, err := newvRAClient(ctrlConfig.URL, ctrlConfig.RefreshToken, false)
	if err != nil {
		setupLog.Error(err, "unable to create vRA client")
		os.Exit(1)
	}

	if err = (&controllers.VirtualMachineReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		VRA:    vra,
		Log:    ctrl.Log.WithName("controllers").WithName("VirtualMachine"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VirtualMachine")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func newvRAClient(url string, refreshToken string, insecure bool) (*vraclient.MulticloudIaaS, error) {
	// Get Token
	token, err := getToken(url, refreshToken, insecure)
	if err != nil {
		return nil, err
	}
	// Create vRA Client
	apiClient, err := getAPIClient(url, token, insecure)
	if err != nil {
		return nil, err
	}
	return apiClient, nil
}

// Functions below are taken from the terraform-provider-vra project
// https://github.com/vmware/terraform-provider-vra/blob/4604d8422a43fa247edfc05058d13abb2f3458fb/vra/client.go#L210
func getToken(url, refreshToken string, insecure bool) (string, error) {
	parsedURL, err := neturl.Parse(url)
	if err != nil {
		return "", err
	}
	transport := httptransport.New(parsedURL.Host, parsedURL.Path, nil)
	transport.SetDebug(false)
	transport.Transport, err = createTransport(insecure)
	if err != nil {
		return "", err
	}
	apiclient := vraclient.New(transport, strfmt.Default)

	params := login.NewRetrieveAuthTokenParams().WithBody(
		&models.CspLoginSpecification{
			RefreshToken: &refreshToken,
		},
	)
	authTokenResponse, err := apiclient.Login.RetrieveAuthToken(params)
	if err != nil || !strings.EqualFold(*authTokenResponse.Payload.TokenType, "bearer") {
		return "", err
	}

	return *authTokenResponse.Payload.Token, nil
}

func createTransport(insecure bool) (http.RoundTripper, error) {
	cfg, err := httptransport.TLSClientAuth(httptransport.TLSClientOptions{
		InsecureSkipVerify: insecure,
	})
	if err != nil {
		return nil, err
	}

	return &http.Transport{
		TLSClientConfig: cfg,
		Proxy:           http.ProxyFromEnvironment,
	}, nil
}

func getAPIClient(url string, token string, insecure bool) (*vraclient.MulticloudIaaS, error) {
	parsedURL, err := neturl.Parse(url)
	if err != nil {
		return nil, err
	}
	t := httptransport.New(parsedURL.Host, parsedURL.Path, nil)
	t.DefaultAuthentication = httptransport.APIKeyAuth("Authorization", "header", "Bearer "+token)

	apiclient := vraclient.New(t, strfmt.Default)
	return apiclient, nil
}
