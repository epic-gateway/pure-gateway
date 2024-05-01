/*
Copyright 2024 Acnodal.
*/

package gateway

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"epic-gateway.org/puregw/controllers"
	"epic-gateway.org/puregw/internal/gateway"
)

// ReferenceGrantReconciler reconciles a Gateway object
type ReferenceGrantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReferenceGrantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1a2.ReferenceGrant{}).
		Complete(r)
}

//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gateway object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *ReferenceGrantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Get the Grant that caused this request
	grant := gatewayv1a2.ReferenceGrant{}
	if err := r.Get(ctx, req.NamespacedName, &grant); err != nil {
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	if !grant.ObjectMeta.DeletionTimestamp.IsZero() {
		// This resource is marked for deletion.
		l.Info("Cleaning up")

		// Remove our finalizer to ensure that we don't block the resource
		// from being deleted.
		if err := controllers.RemoveFinalizer(ctx, r.Client, &grant, controllers.FinalizerName); err != nil {
			l.Error(err, "Removing finalizer")
			// Fall through to delete the EPIC resource
		}

		return controllers.Done, nil
	}

	// The resource is not being deleted, so add our finalizer.
	if err := controllers.AddFinalizer(ctx, r.Client, &grant, controllers.FinalizerName); err != nil {
		return controllers.Done, err
	}

	l.V(1).Info("Reconciling")

	return controllers.Done, r.NudgeGateways(l, ctx)
}

// NudgeGateways "nudges" all of the Gateways, i.e., changes them just
// enough to trigger a reconciliation event. We do this because we
// might have Gateways that reference Grants that haven't been created
// yet, so they're invalid at first but become valid when the Grant
// appears.
//
// FIXME: we could be less brute-force, i.e., find only the Gateways
// that reference this Grant and nudge only those.
func (r *ReferenceGrantReconciler) NudgeGateways(l logr.Logger, ctx context.Context) error {
	gwList := gatewayv1a2.GatewayList{}
	if err := r.Client.List(ctx, &gwList); err != nil {
		return err
	}
	for _, route := range gwList.Items {
		l.V(1).Info("Nudging " + route.Name)
		if err := gateway.Nudge(ctx, r.Client, l, &route); err != nil {
			l.Error(err, "nudging Gateway")
			// Keep trying to nudge other Gateways
		}
	}

	return nil
}
