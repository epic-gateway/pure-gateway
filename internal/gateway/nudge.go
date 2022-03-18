package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// NudgeRoute "nudges" route, i.e., causes its reconciler to fire, by
// adding a random annotation.
func NudgeRoute(ctx context.Context, cl client.Client, l logr.Logger, route *gatewayv1a2.HTTPRoute) error {
	var (
		err        error
		patch      []map[string]interface{}
		patchBytes []byte
		raw        []byte = make([]byte, 8, 8)
	)

	_, _ = rand.Read(raw)

	// Prepare the patch with the new annotation.
	if route.Annotations == nil {
		// If this is the first annotation then we need to wrap it in a
		// JSON object
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  "/metadata/annotations",
			"value": map[string]string{"nudge": hex.EncodeToString(raw)},
		}}
	} else {
		// If there are other annotations then we can just add this one
		patch = []map[string]interface{}{{
			"op":    "add",
			"path":  "/metadata/annotations/nudge",
			"value": hex.EncodeToString(raw),
		}}
	}

	// apply the patch
	if patchBytes, err = json.Marshal(patch); err != nil {
		return err
	}
	if err := cl.Patch(ctx, route, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		l.Error(err, "patching", "route", route)
		return err
	}
	l.Info("patched", "route", route)

	return nil
}
