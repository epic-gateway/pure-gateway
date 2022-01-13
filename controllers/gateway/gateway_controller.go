/*
Copyright 2022 Acnodal.
*/

package gateway

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	puregwv1 "acnodal.io/puregw/apis/puregw/v1"
	"acnodal.io/puregw/internal/acnodal"
	"acnodal.io/puregw/internal/controllers"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=puregw.acnodal.io,resources=gatewayclassconfigs,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gateway object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Get the Gateway that caused this request
	gw := gatewayv1a2.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, &gw); err != nil {
		l.Info("can't get Gateway, probably deleted", "name", req.NamespacedName)

		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	// Get the owning GatewayClass
	gc := gatewayv1a2.GatewayClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: string(gw.Spec.GatewayClassName)}, &gc); err != nil {
		l.Info("No GatewayClass, will retry", "name", gw.Spec.GatewayClassName)
		return controllers.TryAgain, nil
	}

	// Check controller name - are we the right controller?
	if gc.Spec.ControllerName != "acnodal.io/puregw" {
		l.Info("Not our ControllerName, will ignore", "request", req)
		return controllers.Done, nil
	}

	// Get the PureGW GatewayClassConfig referred to by the GatewayClass
	gwcName := types.NamespacedName{Name: string(gc.Spec.ParametersRef.Name)}
	if gc.Spec.ParametersRef.Namespace != nil {
		gwcName.Namespace = string(*gc.Spec.ParametersRef.Namespace)
	}
	gwc := puregwv1.GatewayClassConfig{}
	if err := r.Get(ctx, gwcName, &gwc); err != nil {
		l.Info("No GatewayClassConfig, will retry", "name", gwcName)
		return controllers.TryAgain, nil
	}

	l.Info("processing request", "request", req, "config", gwc)

	// Connect to EPIC
	epic, err := acnodal.NewEPIC(gwc.Spec)
	if err != nil {
		return controllers.Done, err
	}

	// Get the EPIC ServiceGroup
	group, err := epic.GetGroup()
	if err != nil {
		return controllers.Done, err
	}

	// Announce the Gateway
	_, err = epic.AnnounceGateway(group.Links["create-service"], gw.Name, string(gw.ObjectMeta.UID), gw.Spec.Listeners)
	if err != nil {
		return controllers.Done, err
	}

	return controllers.Done, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1a2.Gateway{}).
		Complete(r)
}
