/*
Copyright 2022 Acnodal.
*/

package gateway

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink/nl"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"epic-gateway.org/puregw/controllers"
	"epic-gateway.org/puregw/internal/trueingress"
)

// TCPRouteAgentReconciler reconciles a Gateway object
type TCPRouteAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *TCPRouteAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayapi_v1alpha2.TCPRoute{}).
		Complete(r)
}

//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=tcproutes/finalizers,verbs=update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=puregw.epic-gateway.org,resources=gatewayclassconfigs,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gateway object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *TCPRouteAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Get the Resource that triggered this request
	route := gatewayapi_v1alpha2.TCPRoute{}
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		// Ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	if !route.ObjectMeta.DeletionTimestamp.IsZero() {
		// This resource is marked for deletion.
		l.Info("Cleaning up")

		// Remove our finalizer to ensure that we don't block the resource
		// from being deleted.
		return controllers.Done, controllers.RemoveFinalizer(ctx, r.Client, &route, controllers.AgentFinalizerName())
	}

	// Try to get the Gateway to which we refer, and keep trying until
	// we can.
	gw := gatewayapi.Gateway{}
	// FIXME: need to handle multiple parents
	if err := parentGW(ctx, r.Client, route.Namespace, route.Spec.ParentRefs[0], &gw); err != nil {
		l.Info("Can't get parent, will retry", "parentRef", route.Spec.ParentRefs[0])
		return controllers.TryAgain, nil
	}

	// See if we're the chosen controller.
	config, err := getEPICConfig(ctx, r.Client, string(gw.Spec.GatewayClassName))
	if err != nil {
		return controllers.Done, err
	}
	if config == nil {
		l.V(1).Info("Not our ControllerName, will ignore")
		return controllers.Done, nil
	}

	// Figure out this node's IPV4 address
	node := corev1.Node{}
	nodeName := types.NamespacedName{Namespace: "", Name: os.Getenv("EPIC_NODE_NAME")}
	if err := r.Get(ctx, nodeName, &node); err != nil {
		return controllers.Done, err
	}

	nodeIPV4 := trueingress.NodeAddress(node, nl.FAMILY_V4)
	if nodeIPV4 == nil {
		return controllers.Done, fmt.Errorf("can't determine node's IPV4 address")
	}

	// Connect to EPIC
	epic, err := controllers.ConnectToEPIC(ctx, r.Client, &config.Namespace, config.Name)
	if err != nil {
		return controllers.Done, err
	}

	l.V(1).Info("Reconciling")

	// The resource is not being deleted, and it's our GWClass, so add
	// our finalizer.
	if err := controllers.AddFinalizer(ctx, r.Client, &route, controllers.AgentFinalizerName()); err != nil {
		return controllers.Done, err
	}

	// Setup tunnels. If EPIC hasn't yet filled in everything that we
	// need we'll back off and try again.
	slices, incomplete, err := routeSlicesTCP(ctx, r.Client, &route)
	if err != nil {
		return controllers.Done, err
	}
	if incomplete {
		return controllers.TryAgain, nil
	}
	l.Info("Referenced slices", "slices", slices)
	isEKS, err := eksCluster(ctx, r.Client)
	if err != nil {
		return controllers.Done, err
	}
	incomplete, err = setupTunnels(l, &gw, *config.Spec.TrueIngress, slices, epic, isEKS, nodeIPV4)
	if incomplete {
		return controllers.TryAgain, nil
	}
	if err != nil {
		return controllers.Done, err
	}

	l.V(1).Info("Reconcile complete, will back off and poll for changes")
	return controllers.TryAgainLater, nil
}

// Cleanup removes our finalizer from all of the TCPRoutes in the
// system.
func (r *TCPRouteAgentReconciler) Cleanup(l logr.Logger, ctx context.Context) error {
	routeList := gatewayapi_v1alpha2.TCPRouteList{}
	if err := r.Client.List(ctx, &routeList); err != nil {
		return err
	}
	for _, route := range routeList.Items {
		if err := controllers.RemoveFinalizer(ctx, r.Client, &route, controllers.AgentFinalizerName()); err != nil {
			l.Error(err, "removing Finalizer")
		}
	}
	return nil
}
