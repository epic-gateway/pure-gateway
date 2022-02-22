/*
Copyright 2022 Acnodal.
*/

package discovery

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	puregwv1 "acnodal.io/puregw/apis/puregw/v1"
	"acnodal.io/puregw/controllers"
	"acnodal.io/puregw/internal/acnodal"
)

// EndpointSliceReconciler reconciles a EndpointSlice object
type EndpointSliceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *EndpointSliceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&discoveryv1.EndpointSlice{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices/finalizers,verbs=update
//+kubebuilder:rbac:groups=puregw.acnodal.io,resources=endpointsliceshadows,verbs=get;list;watch;delete

// Reconcile updates EPIC with the current EndpointSlice contents.
func (r *EndpointSliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	const (
		finalizerName = "epic.acnodal.io/controller"
	)

	// See if the HTTPRoute controller has announced this slice. The
	// route can tell whether we're interesting to EPIC or not, but we
	// can't. This logic is a little backward - we usually get the
	// object that caused the event first, but in this case we need the
	// shadow to update and to clean up.
	shadow := puregwv1.EndpointSliceShadow{}
	sliceName := types.NamespacedName{Namespace: req.Namespace, Name: req.Name}
	if err := r.Get(ctx, sliceName, &shadow); err != nil {
		l.Info("Not announced, will ignore")
		return controllers.Done, client.IgnoreNotFound(err)
	}

	// Connect to EPIC using the info in the shadow.
	configName, err := controllers.SplitNSName(shadow.Spec.EPICConfigName)
	if err != nil {
		return controllers.Done, err
	}
	epic, err := controllers.ConnectToEPIC(ctx, r.Client, &configName.Namespace, configName.Name)
	if err != nil {
		return controllers.Done, err
	}

	// Get the Slice that caused this request
	slice := discoveryv1.EndpointSlice{}
	if err := r.Get(ctx, req.NamespacedName, &slice); err != nil {
		if apierrors.IsNotFound(err) {
			l.Info("Slice deleted, will clean up")

			// Delete the EPIC-side resource.
			if err := epic.Delete(shadow.Spec.EPICLink); err != nil {
				return controllers.Done, err
			}

			// Delete the shadow.
			return controllers.Done, r.Delete(ctx, &shadow)
		}

		return controllers.Done, err
	}

	l.Info("Reconciling")

	// The resource is not being deleted, and it's interesting to
	// EPIC. It changed so update its EPIC-side resource.

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
		EndpointSlice: slice,
		NodeAddresses: nodeAddrs,
	}

	// Fix the null endpoints if the service has no replicas. Null
	// endpoints will cause the announcement to fail.
	if spec.EndpointSlice.Endpoints == nil {
		spec.EndpointSlice.Endpoints = []discoveryv1.Endpoint{}
	}

	_, err = epic.UpdateSlice(shadow.Spec.EPICLink, spec)
	return controllers.Done, err
}
