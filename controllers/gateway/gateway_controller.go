/*
Copyright 2022 Acnodal.
*/

package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicgwv1 "acnodal.io/puregw/apis/puregw/v1"
	"acnodal.io/puregw/controllers"
	"acnodal.io/puregw/internal/status"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1a2.Gateway{}).
		Complete(r)
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
	const (
		finalizerName = "epic.acnodal.io/controller"
	)

	// Get the Gateway that caused this request
	gw := gatewayv1a2.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, &gw); err != nil {
		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	config, err := getEPICConfig(ctx, r.Client, string(gw.Spec.GatewayClassName))
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

	if !gw.ObjectMeta.DeletionTimestamp.IsZero() {
		// This resource is marked for deletion.
		l.Info("Cleaning up")

		// Remove our finalizer to ensure that we don't block the resource
		// from being deleted.
		if err := controllers.RemoveFinalizer(ctx, r.Client, &gw, finalizerName); err != nil {
			l.Error(err, "Removing finalizer")
			// Fall through to delete the EPIC resource
		}

		// Delete the EPIC resource
		link, announced := gw.Annotations[epicgwv1.EPICLinkAnnotation]
		if announced {
			err = epic.Delete(link)
			if err != nil {
				return controllers.Done, err
			}
		}

		return controllers.Done, nil
	}

	// See if we've already announced this resource.
	link, announced := gw.Annotations[epicgwv1.EPICLinkAnnotation]
	if announced {
		l.Info("Previously announced", "link", link)
		return controllers.Done, nil
	}

	l.V(1).Info("Reconciling")

	// The resource is not being deleted, and it's our GWClass, so add
	// our finalizer.
	if err := controllers.AddFinalizer(ctx, r.Client, &gw, finalizerName); err != nil {
		return controllers.Done, err
	}

	// Get the EPIC ServiceGroup
	group, err := epic.GetGroup()
	if err != nil {
		return controllers.Done, err
	}

	// Announce the Gateway
	response, err := epic.AnnounceGateway(group.Links["create-proxy"], gw)
	if err != nil {
		return controllers.Done, err
	}

	// Annotate the Gateway with its URL to mark it as "announced".
	if err := r.addEpicLink(ctx, &gw, response.Links["self"], config.NamespacedName().String()); err != nil {
		return controllers.Done, err
	}
	l.Info("Announced", "self-link", response.Links["self"])

	// Tell the user that we're working on bringing up the Gateway.
	if err := markReady(ctx, r.Client, l, &gw); err != nil {
		return controllers.Done, err
	}

	return controllers.Done, nil
}

func getEPICConfig(ctx context.Context, cl client.Client, gatewayClassName string) (*epicgwv1.GatewayClassConfig, error) {
	// Get the owning GatewayClass
	gc := gatewayv1a2.GatewayClass{}
	if err := cl.Get(ctx, types.NamespacedName{Name: gatewayClassName}, &gc); err != nil {
		return nil, fmt.Errorf("Unable to get GatewayClass %s", gatewayClassName)
	}

	// Check controller name - are we the right controller?
	if gc.Spec.ControllerName != controllers.ControllerName {
		return nil, nil
	}

	// Get the PureGW GatewayClassConfig referred to by the GatewayClass
	gwcName := types.NamespacedName{Name: string(gc.Spec.ParametersRef.Name)}
	if gc.Spec.ParametersRef.Namespace != nil {
		gwcName.Namespace = string(*gc.Spec.ParametersRef.Namespace)
	}
	gwc := epicgwv1.GatewayClassConfig{}
	if err := cl.Get(ctx, gwcName, &gwc); err != nil {
		return nil, fmt.Errorf("Unable to get GatewayClassConfig %s", gc.Spec.ParametersRef.Name)
	}

	return &gwc, nil
}

// addEpicLink adds an EPICLinkAnnotation annotation to gw.
func (r *GatewayReconciler) addEpicLink(ctx context.Context, gw *gatewayv1a2.Gateway, link string, configName string) error {
	var (
		patch      []map[string]interface{}
		patchBytes []byte
		err        error
	)

	if gw.Annotations == nil {
		// If this is the first annotation then we need to wrap it in an
		// object
		patch = []map[string]interface{}{{
			"op":   "add",
			"path": "/metadata/annotations",
			"value": map[string]string{
				epicgwv1.EPICLinkAnnotation:   link,
				epicgwv1.EPICConfigAnnotation: configName,
			},
		}}
	} else {
		// If there are other annotations then we can just add this one
		patch = []map[string]interface{}{
			{
				"op":    "add",
				"path":  epicgwv1.EPICLinkAnnotationPatch,
				"value": link,
			},
			{
				"op":    "add",
				"path":  epicgwv1.EPICConfigAnnotationPatch,
				"value": configName,
			},
		}
	}

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	if err := r.Patch(ctx, gw, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		return err
	}

	return nil
}

// markReady adds a Status Condition to indicate that we're
// setting up the Gateway.
func markReady(ctx context.Context, cl client.Client, l logr.Logger, gw *gatewayv1a2.Gateway) error {
	key := client.ObjectKey{Namespace: gw.GetNamespace(), Name: gw.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, gw); err != nil {
			return err
		}

		gsu := status.GatewayStatusUpdate{
			FullName:           types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name},
			Conditions:         make(map[gatewayv1a2.GatewayConditionType]metav1.Condition),
			ExistingConditions: nil,
			Generation:         gw.Generation,
			TransitionTime:     metav1.NewTime(time.Now()),
		}
		gsu.AddCondition(gatewayv1a2.GatewayConditionReady, metav1.ConditionTrue, status.ReasonValidGateway, "Announced to EPIC")

		got, ok := gsu.Mutate(gw).(*gatewayv1a2.Gateway)
		if !ok {
			return fmt.Errorf("Failed to mutate Gateway")
		}

		// Try to update
		return cl.Status().Update(ctx, got)
	})
}
