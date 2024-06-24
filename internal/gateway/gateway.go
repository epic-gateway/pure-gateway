package gateway

import (
	"k8s.io/apimachinery/pkg/labels"

	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicgwv1 "epic-gateway.org/puregw/apis/puregw/v1"
	"epic-gateway.org/puregw/internal/contour/dag"
)

// GatewayEPICUID returns this Gateway's EPIC UID. This can be either
// the local resource's UID (if it's not a shared Gateway) or the
// value of the sharing key annotation (if it is a shared gateway).
func GatewayEPICUID(gw gatewayapi.Gateway) string {
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
// route. If Condition is nil then route is allowed, but if it's
// non-nil then gw has rejected route.
func GatewayAllowsHTTPRoute(gw gatewayapi.Gateway, route gatewayapi.HTTPRoute, fetcher dag.Fetcher) *meta_v1.Condition {
	if !GatewayAllowsKind(gw, (*gatewayapi.Kind)(&route.Kind)) {
		return &meta_v1.Condition{
			Type:    string(gatewayapi.RouteConditionAccepted),
			Reason:  string(gatewayapi.RouteReasonInvalidKind),
			Status:  meta_v1.ConditionFalse,
			Message: "Reference Kind not allowed by parent",
		}
	}

	if !GatewayAllowsHostnames(gw, route) {
		return &meta_v1.Condition{
			Type:    string(gatewayapi.RouteConditionAccepted),
			Reason:  string(gatewayapi.RouteReasonNoMatchingListenerHostname),
			Status:  meta_v1.ConditionFalse,
			Message: "Reference Hostname not allowed by parent",
		}
	}

	listenerAllows := false
	for _, listener := range gw.Spec.Listeners {
		if dag.NamespaceMatches(gw.Namespace, listener.AllowedRoutes.Namespaces, labels.Everything(), route.Namespace, fetcher) {
			listenerAllows = true
		}
	}
	if !listenerAllows {
		return &meta_v1.Condition{
			Type:    string(gatewayapi.RouteConditionAccepted),
			Reason:  string(gatewayapi.RouteReasonNotAllowedByListeners),
			Status:  meta_v1.ConditionFalse,
			Message: "Reference Listener not allowed by parent",
		}
	}

	return nil
}

// GatewayAllowsHTTPRoute determines whether or not gw allows
// route. If error is nil then route is allowed, but if it's non-nil
// then gw has rejected route.
func GatewayAllowsTCPRoute(gw gatewayapi.Gateway, route gatewayapi_v1alpha2.TCPRoute) bool {
	return GatewayAllowsKind(gw, (*gatewayapi.Kind)(&route.Kind))
}

// GatewayAllowsHTTPRoute determines whether or not gw allows route's
// hostnames. If error is nil then route's hostnames are allowed, but
// if it's non-nil then there's no intersection between gw's hostnames
// and route's hostnames.
func GatewayAllowsHostnames(gw gatewayapi.Gateway, route gatewayapi.HTTPRoute) bool {
	for _, listener := range gw.Spec.Listeners {
		hosts, errs := dag.ComputeHosts(route.Spec.Hostnames, listener.Hostname)

		// If any of the Route's hostnames are invalid then the Route
		// can't be used
		if errs != nil {
			return false
		}

		// If there's an intersection between the Route's hostnames and
		// the Listener's hostnames then we can use the Route
		if len(hosts) != 0 {
			return true
		}
	}

	return false
}

// GatewayAllowsKind checks whether gw allows attachment by
// routeKind. If error is nil then attachment is allowed but if not
// then it isn't.
func GatewayAllowsKind(gw gatewayapi.Gateway, routeKind *gatewayapi.Kind) bool {
	for _, listener := range gw.Spec.Listeners {
		if kindsIntersect(listener.AllowedRoutes, routeKind) {
			return true
		}
	}
	return false
}

func kindsIntersect(allowedKinds *gatewayapi.AllowedRoutes, routeKind *gatewayapi.Kind) bool {
	allowed := allowedKinds.Kinds
	if len(allowedKinds.Kinds) == 0 {
		allowed = []gatewayapi.RouteGroupKind{{Kind: "HTTPRoute"}}
	}
	for _, gk := range allowed {
		if gk.Kind == *routeKind {
			return true
		}
	}
	return false
}
