package controllers

import (
	"context"

	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// AddFinalizer safely adds "finalizerName" to the finalizers list of
// "obj". See
// https://pkg.go.dev/k8s.io/client-go/util/retry#RetryOnConflict for
// more info.
func AddFinalizer(ctx context.Context, cl client.Client, obj client.Object, finalizerName string) error {
	if !controllerutil.ContainsFinalizer(obj, finalizerName) {
		key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}

		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Fetch the resource here; you need to refetch it on every try,
			// since if you got a conflict on the last update attempt then
			// you need to get the current version before making your own
			// changes.
			if err := cl.Get(ctx, key, obj); err != nil {
				return err
			}

			// Add our finalizer
			controllerutil.AddFinalizer(obj, finalizerName)

			// Try to update
			return cl.Update(ctx, obj)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// RemoveFinalizer safely removes "finalizerName" from the finalizers
// list of "obj". See
// https://pkg.go.dev/k8s.io/client-go/util/retry#RetryOnConflict for
// more info.
func RemoveFinalizer(ctx context.Context, cl client.Client, obj client.Object, finalizerName string) error {
	if controllerutil.ContainsFinalizer(obj, finalizerName) {
		key := client.ObjectKey{Namespace: obj.GetNamespace(), Name: obj.GetName()}

		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Fetch the resource here; you need to refetch it on every try,
			// since if you got a conflict on the last update attempt then
			// you need to get the current version before making your own
			// changes.
			if err := cl.Get(ctx, key, obj); err != nil {
				return err
			}

			// Remove our finalizer
			controllerutil.RemoveFinalizer(obj, finalizerName)

			// Try to update
			return cl.Update(ctx, obj)
		})
		if err != nil {
			return err
		}
	}

	return nil
}
