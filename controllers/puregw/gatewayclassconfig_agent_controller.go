/*
Copyright 2022 Acnodal.
*/

package puregw

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	epicgwv1 "acnodal.io/puregw/apis/puregw/v1"
	"acnodal.io/puregw/controllers"
	ti "acnodal.io/puregw/internal/trueingress"
)

// GatewayClassConfigAgentReconciler reconciles a GatewayClassConfig object
type GatewayClassConfigAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassConfigAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicgwv1.GatewayClassConfig{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=puregw.acnodal.io,resources=gatewayclassconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=puregw.acnodal.io,resources=gatewayclassconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=puregw.acnodal.io,resources=gatewayclassconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GatewayClassConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *GatewayClassConfigAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	const (
		finalizerName = "epic.acnodal.io/controller"
	)

	// Get the config that caused this request
	gwc := epicgwv1.GatewayClassConfig{}
	if err := r.Get(ctx, req.NamespacedName, &gwc); err != nil {
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	l.V(1).Info("Reconciling")

	// Clean up to ensure that we re-load the TrueIngress components
	ti.RemoveFilters(l, gwc.Spec.TrueIngress.EncapAttachment.Interface, gwc.Spec.TrueIngress.DecapAttachment.Interface)

	return controllers.Done, nil
}
