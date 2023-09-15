/*
Copyright 2022 Acnodal.
*/

package puregw

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicgwv1 "epic-gateway.org/puregw/apis/puregw/v1"
	"epic-gateway.org/puregw/controllers"
	"epic-gateway.org/puregw/internal/contour/status"
	"epic-gateway.org/puregw/internal/gateway"
)

var (
	gwccAccepted metav1.Condition = metav1.Condition{
		Type:    string(gatewayv1a2.GatewayClassConditionStatusAccepted),
		Status:  metav1.ConditionTrue,
		Reason:  "Valid",
		Message: "EPIC connection succeeded",
	}

	gwccCantConnect metav1.Condition = metav1.Condition{
		Type:    string(gatewayv1a2.GatewayClassConditionStatusAccepted),
		Status:  metav1.ConditionFalse,
		Reason:  "Invalid",
		Message: "Invalid GatewayClassConfig: unable to connect to EPIC: ",
	}
)

// GatewayClassConfigReconciler reconciles a GatewayClassConfig object
type GatewayClassConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayClassConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&epicgwv1.GatewayClassConfig{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=puregw.epic-gateway.org,resources=gatewayclassconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=puregw.epic-gateway.org,resources=gatewayclassconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=puregw.epic-gateway.org,resources=gatewayclassconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the GatewayClassConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *GatewayClassConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	gwcc := epicgwv1.GatewayClassConfig{}
	if err := r.Get(ctx, req.NamespacedName, &gwcc); err != nil {
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	l.V(1).Info("Reconciling")

	epic, err := controllers.ConnectToEPIC(ctx, r.Client, &req.Namespace, req.Name)
	if err != nil {
		return controllers.Done, err
	}

	// Make a test connection to EPIC to see if this config works.
	condition := gwccAccepted
	if _, err := epic.GetAccount(); err != nil {
		condition = gwccCantConnect
		condition.Message += err.Error()
	}

	// Nudge this GWConfig's GatewayClasses. This will cause them to
	// update their Status.Conditions.
	children := []gatewayv1a2.GatewayClass{}
	if children, err = configChildren(ctx, r.Client, l, &gwcc); err != nil {
		return controllers.Done, err
	}
	l.V(1).Info("Nudging children", "childCount", len(children))
	for _, gwc := range children {
		if err = gateway.Nudge(ctx, r.Client, l, &gwc); err != nil {
			l.Error(err, "Nudging GatewayClass", "class", gwc)
		}
	}

	// Mark this GWC with the result of the test connection.
	return controllers.Done, markAcceptance(ctx, r.Client, l, &gwcc, condition)
}

// markAcceptance adds a Status Condition to indicate whether we
// accept or reject this GatewayClass.
func markAcceptance(ctx context.Context, cl client.Client, l logr.Logger, gwcc *epicgwv1.GatewayClassConfig, accepted metav1.Condition) error {
	key := client.ObjectKey{Namespace: gwcc.GetNamespace(), Name: gwcc.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, gwcc); err != nil {
			return err
		}

		gwcc.Status.Conditions = status.MergeConditions(gwcc.Status.Conditions, status.RefreshCondition(&gwcc.ObjectMeta, accepted))

		// Try to update
		return cl.Status().Update(ctx, gwcc)
	})
}

// configChildren finds the HTTPRoutes that refer to gw. It's a
// brute-force approach but will likely work up to 1000's of routes.
func configChildren(ctx context.Context, cl client.Client, l logr.Logger, gwcc *epicgwv1.GatewayClassConfig) (classes []gatewayv1a2.GatewayClass, err error) {
	// FIXME: This will not scale, but that may be OK. I expect that
	// most systems will have no more than a few classes so iterating
	// over all of them is probably OK.

	// Get the classes that belong to this service.
	classList := gatewayv1a2.GatewayClassList{}
	if err = cl.List(ctx, &classList); err != nil {
		return
	}
	l.V(1).Info("Child candidates", "count", len(classList.Items))

	gwName := types.NamespacedName{Namespace: gwcc.Namespace, Name: gwcc.Name}
	for _, gwc := range classList.Items {
		l.V(1).Info("*** Comparing", "gw", gwName, "ref", gwc.Spec.ParametersRef)

		if isRefToConfig(gwc.Spec.ParametersRef, gwName) {
			classes = append(classes, gwc)
		}
	}

	return
}

// isRefToConfig returns whether or not ref is a reference to a
// Gateway with the given namespace & name.
func isRefToConfig(ref *gatewayv1a2.ParametersReference, name types.NamespacedName) bool {
	if ref == nil {
		return false
	}

	if string(ref.Group) != epicgwv1.Group {
		return false
	}

	if ref.Kind != "GatewayClassConfig" {
		return false
	}

	if ref.Namespace == nil {
		if name.Namespace != "default" {
			return false
		}
	} else {
		if *ref.Namespace != gatewayv1a2.Namespace(name.Namespace) {
			return false
		}
	}

	return string(ref.Name) == name.Name
}
