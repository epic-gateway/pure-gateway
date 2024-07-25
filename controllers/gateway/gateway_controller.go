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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	epicgwv1 "epic-gateway.org/puregw/apis/puregw/v1"
	"epic-gateway.org/puregw/controllers"
	"epic-gateway.org/puregw/internal/contour/dag"
	"epic-gateway.org/puregw/internal/contour/status"
	"epic-gateway.org/puregw/internal/gateway"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayapi.Gateway{}).
		Complete(r)
}

//+kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=referencegrants,verbs=get;list;watch
//+kubebuilder:rbac:groups=puregw.epic-gateway.org,resources=gatewayclassconfigs,verbs=get;list;watch

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
	gw := gatewayapi.Gateway{}
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
		if err := controllers.RemoveFinalizer(ctx, r.Client, &gw, controllers.FinalizerName); err != nil {
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

	// The resource is not being deleted, and it's our GWClass, so add
	// our finalizer.
	if err := controllers.AddFinalizer(ctx, r.Client, &gw, controllers.FinalizerName); err != nil {
		return controllers.Done, err
	}

	l.V(1).Info("Reconciling")

	gsu := status.GatewayStatusUpdate{
		FullName:           types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name},
		Conditions:         make(map[gatewayapi.GatewayConditionType]metav1.Condition),
		ExistingConditions: nil,
		Generation:         gw.Generation,
		TransitionTime:     metav1.NewTime(time.Now()),
	}

	// Indicate that we're working on this Gateway
	gsu.AddCondition(status.ConditionProgrammedGateway, metav1.ConditionTrue, status.ReasonProgrammedGateway, "Programmed")
	gsu.AddCondition(status.ConditionAcceptedGateway, metav1.ConditionTrue, status.ReasonAcceptedGateway, "Processing")

	// Set the listener supportedKinds and update the attached routes count
	children := []gatewayapi.HTTPRoute{}
	if children, err = gatewayChildren(ctx, r.Client, l, &gw); err != nil {
		return controllers.Done, err
	}
	for _, listener := range gw.Spec.Listeners {
		setSupportedKinds(l, string(listener.Name), listener.AllowedRoutes.Kinds, &gsu)
		setAttachedRoutes(l, string(listener.Name), children, &gsu)
	}

	// Validate TLS configuration (if present)
	tlsOK := true
	for _, listener := range gw.Spec.Listeners {

		// Assume everything's OK.
		gsu.AddListenerCondition(string(listener.Name), gatewayapi.ListenerConditionAccepted, metav1.ConditionTrue, gatewayapi.ListenerReasonAccepted, "Accepted")
		gsu.AddListenerCondition(string(listener.Name), gatewayapi.ListenerConditionProgrammed, metav1.ConditionTrue, gatewayapi.ListenerReasonProgrammed, "Programmed")

		if listener.TLS != nil {
			// If we have a TLS config then we need to validate it.
			if dag.ValidGatewayTLS(gw, *listener.TLS, string(listener.Name), &gsu, r) == nil {
				tlsOK = false
				l.V(1).Info("TLS Error", "listener", listener.Name)

				// There's a TLS config problem so override the conditions we
				// set a few lines earlier.
				gsu.AddListenerCondition(string(listener.Name), gatewayapi.ListenerConditionAccepted, metav1.ConditionFalse, gatewayapi.ListenerReasonAccepted, "TLS config problem")
				gsu.AddListenerCondition(string(listener.Name), gatewayapi.ListenerConditionProgrammed, metav1.ConditionFalse, gatewayapi.ListenerReasonProgrammed, "TLS config problem")
			} else {
				l.V(1).Info("TLS OK", "listener", listener.Name)
			}
		}
	}

	// If there's something wrong with the TLS config then mark the
	// gateway and don't announce.
	if !tlsOK {
		gsu.AddCondition(status.ConditionAcceptedGateway, metav1.ConditionFalse, status.ReasonAcceptedGateway, "TLS config problem")
		gsu.AddCondition(status.ConditionProgrammedGateway, metav1.ConditionFalse, status.ReasonProgrammedGateway, "TLS config problem")
		if err := updateStatus(ctx, r.Client, l, &gw, &gsu); err != nil {
			return controllers.Done, err
		}
		return controllers.Done, nil
	}

	// Get the EPIC ServiceGroup
	group, err := epic.GetGroup()
	if err != nil {
		return controllers.Done, err
	}

	// See if we've already announced this resource.
	link, announced := gw.Annotations[epicgwv1.EPICLinkAnnotation]
	if announced {
		l.Info("Previously announced", "link", link)
		if err := updateStatus(ctx, r.Client, l, &gw, &gsu); err != nil {
			return controllers.Done, err
		}
		return controllers.Done, nil
	}

	// Announce the Gateway
	response, err := epic.AnnounceGateway(group.Links["create-proxy"], gw)
	if err != nil {
		// Tell the user that something has gone wrong
		gsu.AddCondition(status.ConditionAcceptedGateway, metav1.ConditionFalse, status.ReasonValidGateway, err.Error())
		updateStatus(ctx, r.Client, l, &gw, &gsu)
		return controllers.Done, err
	}

	// Annotate the Gateway with its URL to mark it as "announced".
	if err := r.addEpicLink(ctx, &gw, response.Links["self"], config.NamespacedName().String()); err != nil {
		return controllers.Done, err
	}
	l.Info("Announced", "self-link", response.Links["self"])

	// Add the allocated IP and hostname to the GW status.
	if err := markAddresses(ctx, r.Client, l, &gw, response.Gateway.Spec.Address, response.Gateway.Spec.Endpoints[0].DNSName); err != nil {
		return controllers.Done, err
	}

	// Tell the user that we're working on bringing up the Gateway.
	gsu.AddCondition(status.ConditionAcceptedGateway, metav1.ConditionTrue, status.ReasonAcceptedGateway, "Announced to EPIC")
	if err := updateStatus(ctx, r.Client, l, &gw, &gsu); err != nil {
		return controllers.Done, err
	}

	// Nudge this GW's HTTPRoutes. Here's the scenario: create a GW and
	// some children. Everything works. Now delete the GW and re-load
	// it. The new GW has a different UID so the EPIC GWRoutes won't
	// link to the new GWGateway. We need to nudge the children so they
	// send an update to EPIC that links to the new GWRoute.
	l.Info("Nudging children", "childCount", len(children))
	for _, route := range children {
		if err = gateway.Nudge(ctx, r.Client, l, &route); err != nil {
			l.Error(err, "Nudging HTTPRoute", "gateway", gw, "route", route)
		}
	}

	return controllers.Done, nil
}

// Cleanup removes our finalizer from all of the Gateways in the
// system.
func (r *GatewayReconciler) Cleanup(l logr.Logger, ctx context.Context) error {
	gwList := gatewayapi.GatewayList{}
	if err := r.Client.List(ctx, &gwList); err != nil {
		return err
	}
	for _, route := range gwList.Items {
		if err := controllers.RemoveFinalizer(ctx, r.Client, &route, controllers.FinalizerName); err != nil {
			l.Error(err, "removing Finalizer")
		}
	}
	return nil
}

func getEPICConfig(ctx context.Context, cl client.Client, gatewayClassName string) (*epicgwv1.GatewayClassConfig, error) {
	// Get the owning GatewayClass
	gc := gatewayapi.GatewayClass{}
	if err := cl.Get(ctx, types.NamespacedName{Name: gatewayClassName}, &gc); err != nil {
		return nil, fmt.Errorf("Unable to get GatewayClass %s", gatewayClassName)
	}

	// Check controller name - are we the right controller?
	if gc.Spec.ControllerName != controllers.GatewayController {
		return nil, nil
	}

	// Get the PureGW GatewayClassConfig referred to by the GatewayClass
	if gc.Spec.ParametersRef == nil {
		return nil, fmt.Errorf("GWC %s has no ParametersRef", gatewayClassName)
	}
	gwcName := types.NamespacedName{Namespace: "default", Name: string(gc.Spec.ParametersRef.Name)}
	if gc.Spec.ParametersRef.Namespace != nil {
		gwcName.Namespace = string(*gc.Spec.ParametersRef.Namespace)
	}
	gwc := epicgwv1.GatewayClassConfig{}
	if err := cl.Get(ctx, gwcName, &gwc); err != nil {
		return nil, fmt.Errorf("Unable to get GatewayClassConfig %s", gwcName)
	}

	return &gwc, nil
}

// addEpicLink adds an EPICLinkAnnotation annotation to gw.
func (r *GatewayReconciler) addEpicLink(ctx context.Context, gw *gatewayapi.Gateway, link string, configName string) error {
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

// GetSecret implements the dag.Fetcher GetSecret() method.
// FIXME: move this into its own class so we can use the correct context.
func (r *GatewayReconciler) GetSecret(name types.NamespacedName) (*v1.Secret, error) {
	secret := v1.Secret{}
	return &secret, r.Get(context.Background(), name, &secret)
}

// GetGrants implements the dag.Fetcher GetGrants() method.
func (r *GatewayReconciler) GetGrants(ns string) (gatewayapi_v1beta1.ReferenceGrantList, error) {
	classList := gatewayapi_v1beta1.ReferenceGrantList{}
	return classList, r.List(context.Background(), &classList)
}

// GetGrants implements the dag.Fetcher GetGrants() method.
func (r *GatewayReconciler) GetNamespace(name types.NamespacedName) (*v1.Namespace, error) {
	ns := v1.Namespace{}
	return &ns, r.Get(context.Background(), name, &ns)
}

// updateStatus uses Contour's GatewayStatusUpdate to update the
// Gateway's status.
func updateStatus(ctx context.Context, cl client.Client, l logr.Logger, gw *gatewayapi.Gateway, gsu *status.GatewayStatusUpdate) error {
	key := client.ObjectKey{Namespace: gw.GetNamespace(), Name: gw.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, gw); err != nil {
			return err
		}

		got, ok := gsu.Mutate(gw).(*gatewayapi.Gateway)
		if !ok {
			return fmt.Errorf("Failed to mutate Gateway")
		}

		// Try to update
		return cl.Status().Update(ctx, got)
	})
}

// markAddresses adds publicIP and publicHostname to gw's status.
func markAddresses(ctx context.Context, cl client.Client, l logr.Logger, gw *gatewayapi.Gateway, publicIP string, publicHostname string) error {
	key := client.ObjectKey{Namespace: gw.GetNamespace(), Name: gw.GetName()}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		if err := cl.Get(ctx, key, gw); err != nil {
			return err
		}

		// Add the IP address to GW.Status so the user can find out what
		// it is.
		gw.Status.Addresses = []gatewayapi.GatewayStatusAddress{{
			Type:  ptr.To(gatewayapi.IPAddressType),
			Value: publicIP,
		}}
		if publicHostname != "" {
			gw.Status.Addresses = append(gw.Status.Addresses, gatewayapi.GatewayStatusAddress{
				Type:  ptr.To(gatewayapi.HostnameAddressType),
				Value: publicHostname,
			})
		}

		// Try to update
		return cl.Status().Update(ctx, gw)
	})
}

// freshenStatus updates the Status Condition ObservedGeneration.
func freshenStatus(ctx context.Context, cl client.Client, l logr.Logger, key client.ObjectKey) error {
	now := metav1.NewTime(time.Now())

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		gw := gatewayapi.Gateway{}
		if err := cl.Get(ctx, key, &gw); err != nil {
			return err
		}

		// Update the Conditions' ObservedGenerations
		for i := range gw.Status.Conditions {
			gw.Status.Conditions[i].ObservedGeneration = gw.ObjectMeta.Generation
			gw.Status.Conditions[i].LastTransitionTime = now
		}
		for i, listener := range gw.Status.Listeners {
			for j := range listener.Conditions {
				gw.Status.Listeners[i].Conditions[j].ObservedGeneration = gw.ObjectMeta.Generation
				gw.Status.Listeners[i].Conditions[j].LastTransitionTime = now
			}
		}

		// Try to update
		return cl.Status().Update(ctx, &gw)
	})
}

// gatewayChildren finds the HTTPRoutes that refer to gw. It's a
// brute-force approach but will likely work up to 1000's of routes.
func gatewayChildren(ctx context.Context, cl client.Client, l logr.Logger, gw *gatewayapi.Gateway) (routes []gatewayapi.HTTPRoute, err error) {
	// FIXME: This will not scale, but that may be OK. I expect that
	// most systems will have no more than a few dozen routes so
	// iterating over all of them is probably OK.

	// Get the routes that belong to this service.
	routeList := gatewayapi.HTTPRouteList{}
	if err = cl.List(ctx, &routeList, &client.ListOptions{Namespace: ""}); err != nil {
		return
	}
	gwName := types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name}
	for _, route := range routeList.Items {
		for _, ref := range route.Spec.ParentRefs {
			if isRefToGateway(ref, gwName) {
				routes = append(routes, route)
			}
		}
	}

	l.V(1).Info("Children", "count", len(routes))

	return
}

// isRefToGateway returns whether or not ref is a reference
// to a Gateway with the given namespace & name.
func isRefToGateway(ref gatewayapi.ParentReference, gateway types.NamespacedName) bool {
	// This is copied from internal/status/routeconditions.go which
	// doesn't seem to handle "default" as a namespace.
	if ref.Group != nil && *ref.Group != gatewayapi.GroupName {
		return false
	}

	if ref.Kind != nil && *ref.Kind != dag.KindGateway {
		return false
	}

	if ref.Namespace == nil {
		if gateway.Namespace != "default" {
			return false
		}
	} else {
		if *ref.Namespace != gatewayapi.Namespace(gateway.Namespace) {
			return false
		}
	}

	return string(ref.Name) == gateway.Name
}

func setSupportedKinds(l logr.Logger, name string, rgks []gatewayapi.RouteGroupKind, gsu *status.GatewayStatusUpdate) {
	var (
		anyUnsupported bool = false
		kinds          []gatewayapi.Kind
	)

	if len(rgks) == 0 {
		// No Kinds were specified - add a default
		kinds = append(kinds, epicgwv1.SupportedKinds...)
	} else {
		// Check the user-specified Kinds to ensure that we support them
		for _, kind := range rgks {
			if epicgwv1.SupportedKind(kind.Kind) {
				kinds = append(kinds, kind.Kind)
			} else {
				anyUnsupported = true
			}
		}
	}

	if len(kinds) == 0 { // Hard error: none of the specified Kinds were supported
		l.V(1).Info("Config Error", "error", "No Valid Route Kind", "listener", name)
		gsu.AddListenerCondition(name, gatewayapi.ListenerConditionResolvedRefs, metav1.ConditionFalse, gatewayapi.ListenerReasonInvalidRouteKinds, "Listener configuration error")
		return
	}

	if anyUnsupported { // The user specified at least one unsupported Kind
		gsu.AddListenerCondition(name, gatewayapi.ListenerConditionResolvedRefs, metav1.ConditionFalse, gatewayapi.ListenerReasonInvalidRouteKinds, "Listener configuration error")
	} else {
		gsu.AddListenerCondition(name, gatewayapi.ListenerConditionResolvedRefs, metav1.ConditionTrue, gatewayapi.ListenerReasonResolvedRefs, "Resolved")
	}

	gsu.SetListenerSupportedKinds(name, kinds)
	return
}

func setAttachedRoutes(l logr.Logger, name string, children []gatewayapi.HTTPRoute, gsu *status.GatewayStatusUpdate) {
	gsu.SetListenerAttachedRoutes(name, len(children))
	return
}
