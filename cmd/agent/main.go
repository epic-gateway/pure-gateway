/*
Copyright 2022 Acnodal.
*/

package main

import (
	"context"
	"flag"
	"os"
	"os/exec"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicgwv1 "epic-gateway.org/puregw/apis/puregw/v1"
	ti "epic-gateway.org/puregw/internal/trueingress"

	gatewaycontrollers "epic-gateway.org/puregw/controllers/gateway"
	puregwcontrollers "epic-gateway.org/puregw/controllers/puregw"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1a2.AddToScheme(scheme))
	utilruntime.Must(epicgwv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	setupLog.Info("Hello there!")

	// Start the GUE ping utility.
	pinger := exec.Command("/opt/acnodal/bin/gue_ping_svc_auto", "25")
	err := pinger.Start()
	if err != nil {
		setupLog.Error(err, "Starting pinger")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         false,
	})
	if err != nil {
		setupLog.Error(err, "unable to start agent")
		os.Exit(1)
	}

	routeReconciler := gatewaycontrollers.HTTPRouteAgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err = routeReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "HTTPRouteAgent")
		os.Exit(1)
	}
	tcpRouteReconciler := gatewaycontrollers.TCPRouteAgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err = tcpRouteReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TCPRouteAgent")
		os.Exit(1)
	}
	gwClassConfigReconciler := puregwcontrollers.GatewayClassConfigAgentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}
	if err = gwClassConfigReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "GatewayClassConfigAgent")
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

	// This is a kludge. The proper time to remove the filters is when
	// we exit, which we also do, but we didn't always do.
	setupLog.Info("removing BPF filters")
	ti.RemoveFilters(setupLog, "default", "default")

	setupLog.Info("starting agent")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running agent")
		os.Exit(1)
	}

	// Our filters are pretty lightweight but it's polite to remove them
	// so they don't waste CPU.
	setupLog.Info("removing BPF filters")
	ti.RemoveFilters(setupLog, "default", "default")

	setupLog.Info("cleaning up finalizers")
	ctx := context.Background()
	if err := gwClassConfigReconciler.Cleanup(setupLog, ctx); err != nil {
		setupLog.Error(err, "unable to clean up GatewayClassConfigAgentReconciler")
	}
	if err := routeReconciler.Cleanup(setupLog, ctx); err != nil {
		setupLog.Error(err, "unable to clean up HTTPRouteAgentReconciler")
	}

	setupLog.Info("exiting")
	os.Exit(0)
}
