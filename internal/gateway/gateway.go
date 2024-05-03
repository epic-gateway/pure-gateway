package gateway

import (
	"fmt"

	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicgwv1 "epic-gateway.org/puregw/apis/puregw/v1"
	"epic-gateway.org/puregw/internal/contour/dag"
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

// GatewayAllowsHTTPRoute determines whether or not gw allows
// route. If error is nil then route is allowed, but if it's non-nil
// then gw has rejected route.
func GatewayAllowsHTTPRoute(gw gatewayv1a2.Gateway, route gatewayv1a2.HTTPRoute) error {
	if err := GatewayAllowsKind(gw, (*gatewayv1a2.Kind)(&route.Kind)); err != nil {
		return err
	}

	if err := GatewayAllowsHostnames(gw, route); err != nil {
		return err
	}

	return nil
}

// GatewayAllowsHTTPRoute determines whether or not gw allows
// route. If error is nil then route is allowed, but if it's non-nil
// then gw has rejected route.
func GatewayAllowsTCPRoute(gw gatewayv1a2.Gateway, route gatewayv1a2.TCPRoute) error {
	return GatewayAllowsKind(gw, (*gatewayv1a2.Kind)(&route.Kind))
}

// GatewayAllowsHTTPRoute determines whether or not gw allows route's
// hostnames. If error is nil then route's hostnames are allowed, but
// if it's non-nil then there's no intersection between gw's hostnames
// and route's hostnames.
func GatewayAllowsHostnames(gw gatewayv1a2.Gateway, route gatewayv1a2.HTTPRoute) error {
	for _, listener := range gw.Spec.Listeners {
		hosts, errs := dag.ComputeHosts(route.Spec.Hostnames, listener.Hostname)

		// If any of the Route's hostnames are invalid then the Route
		// can't be used
		if errs != nil {
			return errs[0]
		}

		// If there's an intersection between the Route's hostnames and
		// the Listener's hostnames then we can use the Route
		if len(hosts) != 0 {
			return nil
		}
	}

	return fmt.Errorf("No intersecting hostnames between gateway %s and route %s", gw.Name, route.Name)
}

// GatewayAllowsKind checks whether gw allows attachment by
// routeKind. If error is nil then attachment is allowed but if not
// then it isn't.
func GatewayAllowsKind(gw gatewayv1a2.Gateway, routeKind *gatewayv1a2.Kind) error {
	for _, listener := range gw.Spec.Listeners {
		if err := allowsKind(listener.AllowedRoutes, routeKind); err != nil {
			return err
		}
	}
	return nil
}

func allowsKind(allow *gatewayv1a2.AllowedRoutes, routeKind *gatewayv1a2.Kind) error {
	for _, gk := range allow.Kinds {
		if &gk.Kind != routeKind {
			return fmt.Errorf("Kind mismatch: %s vs %s", gk.Kind, *routeKind)
		}
	}
	return nil
}
