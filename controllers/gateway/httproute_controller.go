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
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1beta1 "sigs.k8s.io/gateway-api/apis/v1beta1"

	epicgwv1 "epic-gateway.org/puregw/apis/puregw/v1"
	"epic-gateway.org/puregw/controllers"
	"epic-gateway.org/puregw/internal/acnodal"
	"epic-gateway.org/puregw/internal/contour/dag"
	"epic-gateway.org/puregw/internal/contour/status"
	"epic-gateway.org/puregw/internal/gateway"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayapi.HTTPRoute{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=puregw.epic-gateway.org,resources=endpointsliceshadows,verbs=get;list;watch;create;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HTTPRoute object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *HTTPRouteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	l.V(1).Info("Reconciling")

	var (
		config *epicgwv1.GatewayClassConfig
	)

	// Get the HTTPRoute that triggered this request
	route := gatewayapi.HTTPRoute{}
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		l.Info("Can't get HTTPRoute, probably deleted", "name", req.NamespacedName)

		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	// Clean up if this resource is marked for deletion.
	if !route.ObjectMeta.DeletionTimestamp.IsZero() {
		l.Info("Cleaning up")

		// Remove our finalizer to ensure that we don't block the resource
		// from being deleted.
		if err := controllers.RemoveFinalizer(ctx, r.Client, &route, controllers.FinalizerName); err != nil {
			l.Error(err, "Removing finalizer")
			// Fall through to delete the EPIC resource
		}

		// Delete the EPIC resource if it was announced.
		if err := maybeDelete(ctx, r.Client, &route); err != nil {
			return controllers.Done, err
		}

		// FIXME: clean up slices. Need to delete the slices that are
		// referenced only by this route

		return controllers.Done, nil
	}

	// The resource is not being deleted, and it's our GWClass, so add
	// our finalizer.
	if err := controllers.AddFinalizer(ctx, r.Client, &route, controllers.FinalizerName); err != nil {
		return controllers.Done, err
	}

	// We'll accumulate our Route Status in this RSU and apply it when
	// we're done.
	rsu := status.RouteStatusUpdate{
		FullName:          req.NamespacedName,
		GatewayController: controllers.GatewayController,
		TransitionTime:    metav1.NewTime(time.Now()),
		Generation:        route.Generation,
	}
	for _, rps := range route.Status.Parents {
		rsu.RouteParentStatuses = append(rsu.RouteParentStatuses, &rps)
	}

	// Make a copy of the Route that we'll send to the mothership.
	announcedRoute := route.DeepCopy()

	// Try to get the Gateways to which we refer.
	for i, parent := range route.Spec.ParentRefs {
		parentUpdate := rsu.StatusUpdateFor(parent)
		parentUpdate.AddCondition(gatewayapi.RouteConditionAccepted, metav1.ConditionTrue, gatewayapi.RouteReasonAccepted, "Accepted by PureGW")

		gw := gatewayapi.Gateway{}
		if parentConfig, err := parentGW(ctx, r.Client, route.Namespace, parent, &gw); err != nil {
			l.Info("Can't get parent, will retry", "parentRef", parent, "message", err)
		} else {
			// Make sure that the Gateway will allow this Route to attach
			if cond := gateway.GatewayAllowsHTTPRoute(parent, gw, route, r); cond != nil {
				l.V(1).Info("Gateway rejected parentRef", "gw", gw.Name, "parent", parent)
				parentUpdate.AddCondition(gatewayapi.RouteConditionType(cond.Type), cond.Status, gatewayapi.RouteConditionReason(cond.Reason), cond.Message)
				parentUpdate.AddCondition(gatewayapi.RouteConditionResolvedRefs, metav1.ConditionTrue, gatewayapi.RouteReasonResolvedRefs, "Reference not allowed by parent")

				// Tell the mothership that this parent is invalid.
				announcedRoute.Spec.ParentRefs[i].Name = "INVALID"
			} else {

				// Add conditions for all of the rules relative to this parent
				r.addRuleConditions(l, ctx, parentUpdate, route, announcedRoute)

				// Munge the ParentRefs so they refer to the Gateways' UIDs, not
				// their names. We use UIDs on the EPIC side because they're unique.
				announcedRoute.Spec.ParentRefs[i].Name = gatewayapi.ObjectName(gateway.GatewayEPICUID(gw))

				// FIXME: this assumes that we're only working with one
				// upstream EPIC cluster, but in theory one parent GW could be
				// on one cluster and a different parent GW could be on
				// another cluster.
				config = parentConfig

				// // FIXME: Not sure what GatewayRef is for: Routes have no
				// // single "Gateway", but they have 0 or more parent references
				// // that can be to Gateways. If I don't set this, then each
				// // time I call Mutate() I get another instance of the parent
				// // reference in the Route Status.
				// rsu.GatewayRef = types.NamespacedName{Namespace: gw.Namespace, Name: gw.Name}
			}
		}

		parentUpdate.AddCondition(gatewayapi.RouteConditionResolvedRefs, metav1.ConditionTrue, gatewayapi.RouteReasonResolvedRefs, "References resolved")
	}

	if err := markRouteConditions(ctx, r.Client, l, client.ObjectKey{Namespace: route.GetNamespace(), Name: route.GetName()}, rsu); err != nil {
		return controllers.Done, err
	}

	// The HTTPRoute object doesn't have enough info to connect to EPIC
	// since it doesn't reference a ConfigClass. We kludge around this
	// by "borrowing" one of the parents' ConfigClasses, but if there
	// are no valid parents, then we can't connect to EPIC so we can't
	// announce.
	if config == nil {
		l.Info("No valid parents, cannot announce")
		return controllers.Done, nil
	}

	epic, err := controllers.ConnectToEPIC(ctx, r.Client, &config.Namespace, config.Name)
	if err != nil {
		return controllers.Done, err
	}

	account, err := epic.GetAccount()
	if err != nil {
		return controllers.Done, err
	}

	// Announce the Slices that the Route references. We do this first
	// so the slices will be in place when the route is announced. The
	// slices need the route to be able to allocate tunnel IDs.
	if err := announceSlices(ctx, r.Client, l, account.Links["create-slice"], epic, config.NamespacedName().String(), &route); err != nil {
		return controllers.Done, err
	}

	// See if we've already announced this Route
	if link, announced := route.Annotations[epicgwv1.EPICLinkAnnotation]; announced {

		l.Info("Previously announced, will update", "link", link)

		// Update the Route.
		_, err := epic.UpdateRoute(link,
			acnodal.RouteSpec{
				ClientRef: acnodal.ClientRef{
					Namespace: announcedRoute.Namespace,
					Name:      announcedRoute.Name,
					UID:       string(announcedRoute.UID),
				},
				HTTP: &announcedRoute.Spec,
			})
		if err != nil {
			return controllers.Done, err
		}

	} else {
		// Announce the route to EPIC.
		routeResp, err := epic.AnnounceRoute(account.Links["create-route"],
			acnodal.RouteSpec{
				ClientRef: acnodal.ClientRef{
					Namespace: announcedRoute.Namespace,
					Name:      announcedRoute.Name,
					UID:       string(announcedRoute.UID),
				},
				HTTP: &announcedRoute.Spec,
			})
		if err != nil {
			return controllers.Done, err
		}

		// Annotate the Route to mark it as "announced".
		if err := addEpicLink(ctx, r.Client, &route, routeResp.Links["self"], config.NamespacedName().String()); err != nil {
			return controllers.Done, err
		}

		l.Info("Announced", "epic-link", route.Annotations[epicgwv1.EPICLinkAnnotation])
	}

	return controllers.Done, nil
}

func (r *HTTPRouteReconciler) addRuleConditions(l logr.Logger, ctx context.Context, update *status.RouteParentStatusUpdate, route gatewayapi.HTTPRoute, announcedRoute *gatewayapi.HTTPRoute) {
	// Munge the ClientRefs so they refer to the services' UIDs, not
	// their names. We use UIDs on the EPIC side because they're
	// unique.
	for i, rule := range route.Spec.Rules {
		l.Info("Processing rule", "rule", rule)
		for j, ref := range rule.BackendRefs {
			switch *ref.Kind {
			case gatewayapi.Kind("Service"):
				// Validate the backend ref
				condition := dag.ValidateBackendRef(ref.BackendRef, route.Kind, route.Namespace, r)
				if condition != nil {
					announcedRoute.Spec.Rules[i].BackendRefs[j].Name = "INVALID"
					update.AddCondition(gatewayapi.RouteConditionType(condition.Type), condition.Status, gatewayapi.RouteConditionReason(condition.Reason), condition.Message)
					break
				}

				// We think that the ref is OK, so try to find the service to
				// which it refers.
				svc := corev1.Service{}
				if err := r.Get(ctx, namespacedNameOfHTTPBackendRef(ref.BackendRef, route.Namespace), &svc); err != nil {
					announcedRoute.Spec.Rules[i].BackendRefs[j].Name = "INVALID"
					update.AddCondition(gatewayapi.RouteConditionResolvedRefs, metav1.ConditionFalse, gatewayapi.RouteReasonBackendNotFound, "Backend Ref Not Found")
				} else {
					announcedRoute.Spec.Rules[i].BackendRefs[j].Name = gatewayapi.ObjectName(svc.UID)
				}

			default:
				update.AddCondition(gatewayapi.RouteConditionResolvedRefs, metav1.ConditionFalse, gatewayapi.RouteReasonInvalidKind, "Backend Ref invalid kind: "+string(*ref.Kind))
			}
		}
	}
}

// Cleanup removes our finalizer from all of the HTTPRoutes in the
// system.
func (r *HTTPRouteReconciler) Cleanup(l logr.Logger, ctx context.Context) error {
	routeList := gatewayapi.HTTPRouteList{}
	if err := r.Client.List(ctx, &routeList); err != nil {
		return err
	}
	for _, route := range routeList.Items {
		if err := controllers.RemoveFinalizer(ctx, r.Client, &route, controllers.FinalizerName); err != nil {
			l.Error(err, "removing Finalizer")
		}
	}
	return nil
}

// GetSecret implements the dag.Fetcher GetSecret() method.
// FIXME: move this into its own class so we can use the correct context.
func (r *HTTPRouteReconciler) GetSecret(name types.NamespacedName) (*corev1.Secret, error) {
	secret := corev1.Secret{}
	return &secret, r.Get(context.Background(), name, &secret)
}

// GetGrants implements the dag.Fetcher GetGrants() method.
func (r *HTTPRouteReconciler) GetGrants(ns string) (gatewayapi_v1beta1.ReferenceGrantList, error) {
	classList := gatewayapi_v1beta1.ReferenceGrantList{}
	return classList, r.List(context.Background(), &classList)
}

// GetGrants implements the dag.Fetcher GetGrants() method.
func (r *HTTPRouteReconciler) GetNamespace(name types.NamespacedName) (*corev1.Namespace, error) {
	ns := corev1.Namespace{}
	return &ns, r.Get(context.Background(), name, &ns)
}

// announceSlices announces the slices that this HTTPRoute
// references.If the error return value is non-nil them something has
// gone wrong.
func announceSlices(ctx context.Context, cl client.Client, l logr.Logger, sliceURL string, epic acnodal.EPIC, configName string, route *gatewayapi.HTTPRoute) error {
	// Get the set of EndpointSlices that this Route references.
	slices, incomplete, err := routeSlices(ctx, cl, route)
	if err != nil {
		return err
	}
	if incomplete {
		l.Info("Incomplete backend slice info, will back off and retry")
		return nil
	}
	l.V(1).Info("Referenced slices", "slices", slices)

	// Announce the EndpointSlices that the Route references.
	for _, slice := range slices {
		// If this slice has been announced then we don't need to do it
		// again. We don't need to update slices - the slice controller
		// will take care of that.
		if hasBeen, _ := hasBeenAnnounced(ctx, cl, slice); hasBeen {
			l.Info("Slice previously announced", "slice", slice.Namespace+"/"+slice.Name)
			continue
		}

		// Build the map of node addresses
		nodeAddrs := map[string]string{}
		for _, ep := range slice.Endpoints {
			node := corev1.Node{}
			nodeName := types.NamespacedName{Namespace: "", Name: *ep.NodeName}
			err := cl.Get(ctx, nodeName, &node)
			if err != nil {
				return err
			}
			for _, addr := range node.Status.Addresses {
				if addr.Type == corev1.NodeInternalIP {
					nodeAddrs[*ep.NodeName] = addr.Address
				}
			}
		}

		// Announce slice
		spec := acnodal.SliceSpec{
			ClientRef: acnodal.ClientRef{
				Namespace: slice.Namespace,
				Name:      slice.Name,
				UID:       string(slice.UID),
			},
			ParentRef: acnodal.ClientRef{
				Namespace: slice.Namespace,
				Name:      slice.ObjectMeta.OwnerReferences[0].Name,
				UID:       string(slice.ObjectMeta.OwnerReferences[0].UID),
			},
			EndpointSlice: *slice,
			NodeAddresses: nodeAddrs,
		}

		// Fix the null endpoints if the service has no replicas. Null
		// endpoints will cause the announcement to fail.
		if spec.EndpointSlice.Endpoints == nil {
			spec.EndpointSlice.Endpoints = []discoveryv1.Endpoint{}
		}

		sliceResp, err := epic.AnnounceSlice(sliceURL, spec)
		if err != nil {
			l.Error(err, "announcing slice")
			continue
		}

		// Annotate the Slice to mark it as "announced".
		if err := addSliceEpicLink(ctx, cl, slice, sliceResp.Links["self"], configName, route); err != nil {
			l.Error(err, "adding EPIC link to slice")
			continue
		}
		l.Info("Slice announced", "epic-link", sliceResp.Links["self"], "slice", slice.Namespace+"/"+slice.Name)
	}

	return nil
}

// parentGW gets the parent Gateway resource pointed to by the
// provided ParentRef, and returns it in *gw. defaultNS is the
// namespace of the object that contains the ref, which means that
// it's the default namespace if the ref doesn't have one.
func parentGW(ctx context.Context, cl client.Client, defaultNS string, ref gatewayapi.ParentReference, gw *gatewayapi.Gateway) (*epicgwv1.GatewayClassConfig, error) {

	// FIXME: fail if the parent is not a Gateway

	gwName := namespacedNameOfParentRef(ref, defaultNS)
	if err := cl.Get(ctx, gwName, gw); err != nil {
		return nil, err
	}

	// See if we're the chosen controller
	if config, err := getEPICConfig(ctx, cl, string(gw.Spec.GatewayClassName)); err != nil {
		return nil, err
	} else {
		if config == nil {
			return nil, fmt.Errorf("Not our ControllerName, will ignore Gateway %v", gwName)
		}

		return config, nil
	}
}

// routeSlices returns all of the slices that belong to all of
// the services referenced by route. If incomplete is true then
// something is missing so the controller needs to back off and retry
// later. If err is non-nil then the array of EndpointSlices is
// invalid.
func routeSlices(ctx context.Context, cl client.Client, route *gatewayapi.HTTPRoute) (slices []*discoveryv1.EndpointSlice, incomplete bool, err error) {
	// Assume that we can reach all of our services.
	incomplete = false

	// Check each rule in the Route.
	for _, rule := range route.Spec.Rules {

		// Get each service that this rule references.
		for _, ref := range rule.BackendRefs {

			// Get the service referenced by this ref.
			svc := corev1.Service{}
			err = cl.Get(ctx, namespacedNameOfHTTPBackendRef(ref.BackendRef, route.Namespace), &svc)
			if err != nil {
				// If the service doesn't exist yet then tell the controller
				// to back off and retry.
				if apierrors.IsNotFound(err) {
					incomplete = true
				} else {
					// If it's some other sort of error then tell the controller.
					return
				}
			}

			// Get the slices that belong to this service.
			sliceList := discoveryv1.EndpointSliceList{}
			if err = cl.List(ctx, &sliceList, &client.ListOptions{
				Namespace: svc.Namespace,
				LabelSelector: labels.SelectorFromSet(map[string]string{
					"kubernetes.io/service-name": svc.Name,
				}),
			}); err != nil {
				return
			}

			// Add each slice to the return array.
			for _, slice := range sliceList.Items {
				slices = append(slices, &slice)
			}
		}
	}

	return
}

// addEpicLink adds our annotations that indicate that the route has
// been announced.
func addEpicLink(ctx context.Context, cl client.Client, route client.Object, link string, configName string) error {
	var (
		patch      []map[string]interface{}
		patchBytes []byte
		err        error
	)

	if route.GetAnnotations() == nil {
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
	if err := cl.Patch(ctx, route, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		return err
	}

	return nil
}

// removeEpicLink removes our annotations that indicate that the route
// has been announced.
func removeEpicLink(ctx context.Context, cl client.Client, route client.Object) error {
	var (
		patch      []map[string]interface{}
		patchBytes []byte
		err        error
	)

	// Remove our annotations, if present.
	for annKey := range route.GetAnnotations() {
		if annKey == epicgwv1.EPICLinkAnnotation {
			patch = append(patch, map[string]interface{}{
				"op":   "remove",
				"path": epicgwv1.EPICLinkAnnotationPatch,
			})
		} else if annKey == epicgwv1.EPICConfigAnnotation {
			patch = append(patch, map[string]interface{}{
				"op":   "remove",
				"path": epicgwv1.EPICConfigAnnotationPatch,
			})
		}
	}

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	if err := cl.Patch(ctx, route, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		return err
	}

	return nil
}

// addSliceEpicLink adds our annotations that indicate that the slice
// has been announced.
func addSliceEpicLink(ctx context.Context, cl client.Client, slice *discoveryv1.EndpointSlice, link string, configName string, route client.Object) error {
	kind := gatewayapi.Kind("HTTPRoute")
	ns := gatewayapi.Namespace(route.GetNamespace())
	name := gatewayapi.ObjectName(route.GetName())

	shadow := epicgwv1.EndpointSliceShadow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slice.Name,
			Namespace: slice.Namespace,
		},
		Spec: epicgwv1.EndpointSliceShadowSpec{
			EPICConfigName: configName,
			EPICLink:       link,
			ParentRoutes: []gatewayapi.ParentReference{{
				Kind:      &kind,
				Namespace: &ns,
				Name:      name,
			}},
		},
	}

	return cl.Create(ctx, &shadow)
}

func hasBeenAnnounced(ctx context.Context, cl client.Client, slice *discoveryv1.EndpointSlice) (bool, error) {
	shadow := epicgwv1.EndpointSliceShadow{}
	name := types.NamespacedName{Namespace: slice.Namespace, Name: slice.Name}
	if err := cl.Get(ctx, name, &shadow); err != nil {
		return false, err
	}
	return true, nil
}

func maybeDelete(ctx context.Context, cl client.Client, route client.Object) error {
	annotations := route.GetAnnotations()
	link, announced := annotations[epicgwv1.EPICLinkAnnotation]
	if announced {
		// Get cached config name
		configName, err := controllers.SplitNSName(annotations[epicgwv1.EPICConfigAnnotation])
		if err != nil {
			return err
		}
		epic, err := controllers.ConnectToEPIC(ctx, cl, &configName.Namespace, configName.Name)
		if err != nil {
			return err
		}
		err = epic.Delete(link)
		if err != nil {
			return err
		}
	}

	return nil
}

// markRouteConditions adds a Status Condition to the route.
func markRouteConditions(ctx context.Context, cl client.Client, l logr.Logger, routeKey client.ObjectKey, rsu status.RouteStatusUpdate) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Fetch the resource here; you need to refetch it on every try,
		// since if you got a conflict on the last update attempt then
		// you need to get the current version before making your own
		// changes.
		route := gatewayapi.HTTPRoute{}
		if err := cl.Get(ctx, routeKey, &route); err != nil {
			return err
		}

		// Try to update
		route2 := rsu.Mutate(&route)
		return cl.Status().Update(ctx, route2)
	})
}

// namespacedNameOfParentRef returns the NamespacedName of a
// ParentRef.
func namespacedNameOfParentRef(ref gatewayapi.ParentReference, defaultNS string) types.NamespacedName {
	name := types.NamespacedName{Namespace: defaultNS, Name: string(ref.Name)}
	if ref.Namespace != nil {
		name.Namespace = string(*ref.Namespace)
	}
	return name
}

// namespacedNameOfHTTPBackendRef returns the NamespacedName of an
// HTTPBackendRef.
func namespacedNameOfHTTPBackendRef(ref gatewayapi.BackendRef, defaultNS string) types.NamespacedName {
	name := types.NamespacedName{Namespace: defaultNS, Name: string(ref.Name)}
	if ref.Namespace != nil {
		name.Namespace = string(*ref.Namespace)
	}
	return name
}
