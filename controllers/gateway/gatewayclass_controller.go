/*
Copyright 2022 Acnodal.
*/

package gateway

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"epic-gateway.org/puregw/controllers"
	"epic-gateway.org/puregw/internal/contour/status"
)

var (
	accepted metav1.Condition = metav1.Condition{
		Type:    string(gatewayv1a2.GatewayClassConditionStatusAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  "Valid",
		Message: "EPIC connection succeeded",
	}

	gwccInvalid metav1.Condition = metav1.Condition{
		Type:   string(gatewayv1a2.GatewayClassConditionStatusAccepted),
		Status: metav1.ConditionFalse,
		Reason: string(gatewayv1a2.GatewayClassReasonInvalidParameters),
	}

	gwccCantConnect metav1.Condition = metav1.Condition{
		Type:    string(gatewayv1a2.GatewayClassConditionStatusAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  "Invalid",
		Message: "Invalid GatewayClassConfig: unable to connect to EPIC: ",
	}
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

	l.V(1).Info("Reconciling")

	config, err := getEPICConfig(ctx, r.Client, gc.Name)
	if err != nil {
		condition := gwccInvalid
		condition.Message = err.Error()
		return controllers.Done, markAcceptance(ctx, r.Client, l, &gc, condition)
	}
	if config == nil {
		l.V(1).Info("Not our ControllerName, will ignore")
		return controllers.Done, nil
	}

	epic, err := controllers.ConnectToEPIC(ctx, r.Client, &config.Namespace, config.Name)
	if err != nil {
		return controllers.Done, err
	}

	// Make a test connection to EPIC to see if this resource and its
	// GatewayClassConfig work.
	condition := accepted
	if _, err := epic.GetAccount(); err != nil {
		condition = gwccCantConnect
		condition.Message += err.Error()
	}

	// Mark this GWC with the result of the test connection.
	if err := markAcceptance(ctx, r.Client, l, &gc, condition); err != nil {
		return controllers.Done, err
	}

	return controllers.Done, nil
}

// markAcceptance adds a Status Condition to indicate whether we
// accept or reject this GatewayClass.
func markAcceptance(ctx context.Context, cl client.Client, l logr.Logger, gc *gatewayv1a2.GatewayClass, accepted metav1.Condition) error {
	key := client.ObjectKey{Namespace: gc.GetNamespace(), Name: gc.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, gc); err != nil {
			return err
		}

		gc.Status.Conditions = status.MergeConditions(gc.Status.Conditions, status.RefreshCondition(&gc.ObjectMeta, accepted))

		// Try to update
		return cl.Status().Update(ctx, gc)
	})
}
