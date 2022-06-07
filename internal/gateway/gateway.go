package gateway

import (
	"fmt"

	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicgwv1 "acnodal.io/puregw/apis/puregw/v1"
)

// GatewayEPICUID returns this Gateway's EPIC UID. This can be either
// the local resource's UID (if it's not a shared Gateway) or the
// value of the sharing key annotation (if it is a shared gateway).
func GatewayEPICUID(gw gatewayv1a2.Gateway) string {
	// Assume that this GW will be non-shared
	uid := string(gw.UID)

	// if there's a sharing key then use that so the web service will
	// connect us to the existing one.
	if key, hasKey := gw.Annotations[epicgwv1.EPICSharingKeyAnnotation]; hasKey {
		uid = key
	}

	return uid
}

// GatewayAllowsRoute checks whether gw allows attachment by route. If
// error is nil then attachment is allowed but if not then it isn't.
func GatewayAllowsRoute(gw gatewayv1a2.Gateway, route gatewayv1a2.HTTPRoute) error {
	for _, listener := range gw.Spec.Listeners {
		if err := allowsRoute(listener.AllowedRoutes, &route); err != nil {
			return err
		}
	}
	return nil
}

func allowsRoute(allow *gatewayv1a2.AllowedRoutes, route *gatewayv1a2.HTTPRoute) error {
	for _, gk := range allow.Kinds {
		if gk.Kind != gatewayv1a2.Kind(route.Kind) {
			return fmt.Errorf("Kind mismatch: %s vs %s", gk.Kind, route.Kind)
		}
	}
	return nil
}
