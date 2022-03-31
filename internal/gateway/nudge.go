package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Nudge "nudges" nudgee, i.e., causes its reconciler to fire, by
// adding a random annotation.
func Nudge(ctx context.Context, cl client.Client, l logr.Logger, nudgee client.Object) error {
	var (
		err        error
		patch      []map[string]interface{}
		patchBytes []byte
		raw        []byte = make([]byte, 8, 8)
	)

	_, _ = rand.Read(raw)

	// Prepare the patch with the new annotation.
	if nudgee.GetAnnotations() == nil {
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
	if err := cl.Patch(ctx, nudgee, client.RawPatch(types.JSONPatchType, patchBytes)); err != nil {
		l.Error(err, "patching", "route", nudgee)
		return err
	}
	l.Info("patched", "route", nudgee)

	return nil
}
