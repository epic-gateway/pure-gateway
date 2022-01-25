/*
Copyright 2022 Acnodal.
*/

package gateway

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	puregwv1 "acnodal.io/puregw/apis/puregw/v1"
	"acnodal.io/puregw/controllers"
	"acnodal.io/puregw/internal/acnodal"
)

// HTTPRouteReconciler reconciles a HTTPRoute object
type HTTPRouteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1a2.HTTPRoute{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch

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
	const (
		finalizerName = "epic.acnodal.io/controller"
	)

	// Get the HTTPRoute that triggered this request
	route := gatewayv1a2.HTTPRoute{}
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		l.Info("Can't get HTTPRoute, probably deleted", "name", req.NamespacedName)

		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	// See if we are the chosen controller class
	gw := gatewayv1a2.Gateway{}
	// FIXME: need to handle multiple parents
	if err := getParentGW(ctx, r.Client, route.Spec.ParentRefs[0], &gw); err != nil {
		l.Info("Can't get parent, will retry", "parentRef", route.Spec.ParentRefs[0])
		return controllers.TryAgain, nil
	}

	epic, err := connectToEPIC(ctx, r.Client, string(gw.Spec.GatewayClassName))
	if err != nil {
		return controllers.Done, err
	}
	if epic == nil {
		l.Info("Not our ControllerName, will ignore")
		return controllers.Done, nil
	}

	// Clean up if this resource is marked for deletion.
	if !route.ObjectMeta.DeletionTimestamp.IsZero() {
		l.Info("Cleaning up")

		// Remove our finalizer to ensure that we don't block the resource
		// from being deleted.
		if err := controllers.RemoveFinalizer(ctx, r.Client, &route, finalizerName); err != nil {
			l.Error(err, "Removing finalizer")
			// Fall through to delete the EPIC resource
		}

		// Delete the EPIC resource
		link, announced := route.Annotations[puregwv1.EPICLinkAnnotation]
		if announced {
			err = epic.Delete(link)
			if err != nil {
				return controllers.Done, err
			}
		}

		return controllers.Done, nil
	}

	l.Info("Reconciling")

	account, err := epic.GetAccount()
	if err != nil {
		return controllers.Done, err
	}

	// The resource is not being deleted, and it's our GWClass, so add
	// our finalizer.
	if err := controllers.AddFinalizer(ctx, r.Client, &route, finalizerName); err != nil {
		return controllers.Done, err
	}

	// See if we've already announced this Route
	link, announced := route.Annotations[puregwv1.EPICLinkAnnotation]
	if announced {
		l.Info("Previously announced", "link", link)
	} else {

		// Announce the route.

		// Munge the ParentRefs so they refer to the Gateways' UIDs, not
		// their names. We use UIDs on the EPIC side because they're unique.
		for i, parent := range route.Spec.ParentRefs {
			gw := gatewayv1a2.Gateway{}
			gwName := types.NamespacedName{Namespace: "default", Name: string(parent.Name)}
			if parent.Namespace != nil {
				gwName.Namespace = string(*parent.Namespace)
			}
			if err := r.Get(ctx, gwName, &gw); err != nil {
				l.Error(err, "getting parent")
				// If we can't find the parent we'll need to keep retrying until
				// we can.
				return controllers.TryAgain, nil
			}
			route.Spec.ParentRefs[i].Name = gatewayv1a2.ObjectName(gw.UID)
		}

		// Munge the ClientRefs so they refer to the services' UIDs, not
		// their names. We use UIDs on the EPIC side because they're
		// unique.
		for i, rule := range route.Spec.Rules {
			for j, ref := range rule.BackendRefs {
				svc := corev1.Service{}
				svcName := types.NamespacedName{Namespace: "default", Name: string(ref.Name)}
				if ref.Namespace != nil {
					svcName.Namespace = string(*ref.Namespace)
				}
				err := r.Get(ctx, svcName, &svc)
				if err != nil {
					return controllers.Done, err
				}
				route.Spec.Rules[i].BackendRefs[j].Name = gatewayv1a2.ObjectName(svc.UID)
			}
		}

		// Announce the Route.
		routeResp, err := epic.AnnounceRoute(account.Links["create-route"], route.Name,
			acnodal.RouteSpec{
				ClientRef: acnodal.ClientRef{
					ClusterID: "puregw", // FIXME:
					Namespace: route.Namespace,
					Name:      route.Name,
					UID:       string(route.UID),
				},
				HTTP: route.Spec,
			})
		if err != nil {
			return controllers.Done, err
		}

		// Annotate the Route to mark it as "announced".
		if err := addEpicLink(ctx, r.Client, &route, routeResp.Links["self"]); err != nil {
			return controllers.Done, err
		}
		l.Info("Announced", "epic-link", route.Annotations[puregwv1.EPICLinkAnnotation])
	}

	// Announce the EndpointSlices that the Route references
	slices, incomplete, err := referencedSlices(ctx, r.Client, &route)
	if err != nil {
		return controllers.Done, err
	}
	if incomplete {
		return controllers.TryAgain, nil
	}
	l.Info("Referenced slices", "slices", slices)

	for _, slice := range slices {
		// If this slice has been announced then we don't need to do it
		// again.
		link, announced := slice.Annotations[puregwv1.EPICLinkAnnotation]
		if announced {
			l.Info("Previously announced", "slice", slice.Name, "epic-link", link)
			continue
		}

		// Build the map of node addresses
		nodeAddrs := map[string]string{}
		for _, ep := range slice.Endpoints {
			node := corev1.Node{}
			nodeName := types.NamespacedName{Namespace: "", Name: *ep.NodeName}
			err := r.Get(ctx, nodeName, &node)
			if err != nil {
				return controllers.Done, err
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
				ClusterID: "puregw", // FIXME:
				Namespace: slice.Namespace,
				Name:      slice.Name,
				UID:       string(slice.UID),
			},
			ParentRef: acnodal.ClientRef{
				ClusterID: "puregw", // FIXME:
				Namespace: slice.Namespace,
				Name:      slice.ObjectMeta.OwnerReferences[0].Name,
				UID:       string(slice.ObjectMeta.OwnerReferences[0].UID),
			},
			EndpointSlice: *slice,
			NodeAddresses: nodeAddrs,
		}

		sliceResp, err := epic.AnnounceSlice(account.Links["create-slice"], spec)
		if err != nil {
			l.Error(err, "announcing slice")
			continue
		}

		// Annotate the Slice to mark it as "announced".
		if err := addSliceEpicLink(ctx, r.Client, slice, sliceResp.Links["self"]); err != nil {
			l.Error(err, "adding EPIC link to slice")
			continue
		}
		l.Info("Slice announced", "epic-link", slice.Annotations[puregwv1.EPICLinkAnnotation])
	}

	return controllers.Done, nil
}

// getParentGW gets the parent Gateway resource pointed to by the
// provided ParentRef.
func getParentGW(ctx context.Context, cl client.Client, ref gatewayv1a2.ParentRef, gw *gatewayv1a2.Gateway) error {
	gwName := types.NamespacedName{Namespace: "default", Name: string(ref.Name)}
	if ref.Namespace != nil {
		gwName.Namespace = string(*ref.Namespace)
	}
	return cl.Get(ctx, gwName, gw)
}

// referencedSlices returns all of the slices that belong to all of
// the services referenced by route. If incomplete is true then
// something is missing so the controller needs to back off and retry
// later. If err is non-nil then the array of EndpointSlices is
// invalid.
func referencedSlices(ctx context.Context, cl client.Client, route *gatewayv1a2.HTTPRoute) (slices []*discoveryv1.EndpointSlice, incomplete bool, err error) {
	// Check each rule in the Route.
	for _, rule := range route.Spec.Rules {

		// Get each service that this rule references.
		for _, ref := range rule.BackendRefs {

			// Get the service referenced by this ref.
			svc := corev1.Service{}
			svcName := types.NamespacedName{Namespace: "default", Name: string(ref.Name)}
			if ref.Namespace != nil {
				svcName.Namespace = string(*ref.Namespace)
			}
			err := cl.Get(ctx, svcName, &svc)
			if err != nil {
				// If the service doesn't exist yet then tell the controller
				// to back off and retry.
				if apierrors.IsNotFound(err) {
					return slices, true, nil
				}

				// If it's some other sort of error then tell the controller.
				return slices, false, err
			}

			// Get the slices that belong to this service.
			sliceList := discoveryv1.EndpointSliceList{}
			if err := cl.List(ctx, &sliceList, &client.ListOptions{
				Namespace: route.Namespace,
				LabelSelector: labels.SelectorFromSet(map[string]string{
					"kubernetes.io/service-name": svc.Name,
				}),
			}); err != nil {
				return slices, false, err
			}

			// Add each slice to the return array.
			for _, slice := range sliceList.Items {
				slices = append(slices, &slice)
			}
		}
	}

	return slices, false, nil
}

// addEpicLink adds an EPICLinkAnnotation annotation to the Route.
func addEpicLink(ctx context.Context, cl client.Client, route *gatewayv1a2.HTTPRoute, link string) error {
	var (
		patch      []map[string]interface{}
		patchBytes []byte
		err        error
	)

	if route.Annotations == nil {
		// If this is the first annotation then we need to wrap it in an
		// object
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  "/metadata/annotations",
			"value": map[string]string{puregwv1.EPICLinkAnnotation: link},
		}}
	} else {
		// If there are other annotations then we can just add this one
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  puregwv1.EPICLinkAnnotationPatch,
			"value": link,
		}}
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

// addEpicLink adds an EPICLinkAnnotation annotation to the Route.
func addSliceEpicLink(ctx context.Context, cl client.Client, slice *discoveryv1.EndpointSlice, link string) error {
	var (
		patch      []map[string]interface{}
		patchBytes []byte
		err        error
	)

	if slice.Annotations == nil {
		// If this is the first annotation then we need to wrap it in an
		// object
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  "/metadata/annotations",
			"value": map[string]string{puregwv1.EPICLinkAnnotation: link},
		}}
	} else {
		// If there are other annotations then we can just add this one
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  puregwv1.EPICLinkAnnotationPatch,
			"value": link,
		}}
	}

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	if err := cl.Patch(ctx, slice, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		return err
	}

	return nil
}
