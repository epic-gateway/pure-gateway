/*
Copyright 2022 Acnodal.
*/

package discovery

import (
	"context"
	"encoding/json"

	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	puregwv1 "acnodal.io/puregw/apis/puregw/v1"
	"acnodal.io/puregw/controllers"
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

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EndpointSlice object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *EndpointSliceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	const (
		finalizerName = "epic.acnodal.io/controller"
	)

	// Get the Slice that caused this request
	slice := discoveryv1.EndpointSlice{}
	if err := r.Get(ctx, req.NamespacedName, &slice); err != nil {
		l.Info("Can't get Slice, probably deleted", "name", req.NamespacedName)

		// ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	// See if the HTTPRoute controller has announced this slice. It has
	// enough data to be able to tell whether we're interesting to EPIC
	// or not, but we don't.
	link, announced := slice.Annotations[puregwv1.EPICLinkAnnotation]
	if !announced {
		// We don't know if this belongs to a service that we care about
		return controllers.Done, nil
	}

	// Clean up if this resource is marked for deletion.
	if !slice.ObjectMeta.DeletionTimestamp.IsZero() {
		l.Info("Cleaning up")

		// Remove our finalizer to ensure that we don't block the resource
		// from being deleted.
		if err := controllers.RemoveFinalizer(ctx, r.Client, &slice, finalizerName); err != nil {
			l.Error(err, "Removing finalizer")
			// Fall through to delete the EPIC resource
		}

		// Delete the EPIC resource.
		configName, err := controllers.SplitNSName(slice.Annotations[puregwv1.EPICConfigAnnotation])
		if err != nil {
			return controllers.Done, err
		}
		epic, err := controllers.ConnectToEPIC(ctx, r.Client, &configName.Namespace, configName.Name)
		if err != nil {
			return controllers.Done, err
		}
		return controllers.Done, epic.Delete(link)
	}

	l.Info("Reconciling")

	// The resource is not being deleted, and it's interesting to EPIC,
	// so add our finalizer.
	if err := controllers.AddFinalizer(ctx, r.Client, &slice, finalizerName); err != nil {
		return controllers.Done, err
	}

	// Update slice
	l.Info("FIXME: Update slice", "slice", link)

	return ctrl.Result{}, nil
}

// addEpicSliceLink adds an EPICLinkAnnotation annotation to slice.
func addEpicSliceLink(ctx context.Context, cl client.Client, slice *discoveryv1.EndpointSlice, link string) error {
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
