/*
Copyright 2022 Acnodal.
*/

package gateway

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"acnodal.io/puregw/controllers"
	"acnodal.io/puregw/internal/status"
)

// GatewayClassReconciler reconciles a GatewayClass object
type GatewayClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1a2.GatewayClass{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses/finalizers,verbs=update
//+kubebuilder:rbac:groups=puregw.acnodal.io,resources=gatewayclassconfigs,verbs=get;list;watch
//+kubebuilder:rbac:groups=puregw.acnodal.io,resources=gatewayclassconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=puregw.acnodal.io,resources=gatewayclassconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GatewayClass object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *GatewayClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Get the class that caused this request
	gc := gatewayv1a2.GatewayClass{}
	if err := r.Get(ctx, req.NamespacedName, &gc); err != nil {
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	config, err := getEPICConfig(ctx, r.Client, gc.Name)
	if err != nil {
		return controllers.Done, err
	}
	if config == nil {
		l.V(1).Info("Not our ControllerName, will ignore")
		return controllers.Done, nil
	}

	epic, err := controllers.ConnectToEPIC(ctx, r.Client, &config.Namespace, config.Name)
	if err != nil {
		return controllers.Done, err
	}

	l.V(1).Info("Reconciling")

	// Make a test connection to EPIC to see if this resource and its
	// GatewayClassConfig work.
	accepted := false
	if _, err := epic.GetAccount(); err == nil {
		accepted = true
	}

	// Mark this GWC with the result of the test connection.
	if err := markAcceptance(ctx, r.Client, l, &gc, accepted); err != nil {
		return controllers.Done, err
	}

	return controllers.Done, nil
}

// markAcceptance adds a Status Condition to indicate whether we
// accept or reject this GatewayClass.
func markAcceptance(ctx context.Context, cl client.Client, l logr.Logger, gc *gatewayv1a2.GatewayClass, accepted bool) error {
	key := client.ObjectKey{Namespace: gc.GetNamespace(), Name: gc.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, gc); err != nil {
			return err
		}

		status.SetGatewayClassAccepted(ctx, cl, gc, accepted)

		// Try to update
		return cl.Status().Update(ctx, gc)
	})
}
