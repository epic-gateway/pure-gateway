// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dag

import (
	"fmt"
	"net"
	"strings"

	"epic-gateway.org/puregw/internal/contour/status"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	gatewayapi_v1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	KindGateway = "Gateway"
)

type Fetcher interface {
	GetSecret(name types.NamespacedName) (*v1.Secret, error)
	GetGrants(ns string) (gatewayapi_v1alpha2.ReferenceGrantList, error)
}

func isSecretRef(certificateRef gatewayapi_v1alpha2.SecretObjectReference) bool {
	return certificateRef.Group != nil && *certificateRef.Group == "" &&
		certificateRef.Kind != nil && *certificateRef.Kind == "Secret"
}

func ValidGatewayTLS(gateway gatewayapi_v1alpha2.Gateway, listenerTLS gatewayapi_v1alpha2.GatewayTLSConfig, listenerName string, gwAccessor *status.GatewayStatusUpdate, cb Fetcher) *v1.Secret {
	if len(listenerTLS.CertificateRefs) != 1 {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1alpha2.ListenerConditionReady,
			metav1.ConditionFalse,
			gatewayapi_v1alpha2.ListenerReasonInvalid,
			"Listener.TLS.CertificateRefs must contain exactly one entry",
		)
		return nil
	}

	certificateRef := listenerTLS.CertificateRefs[0]

	// Validate a v1.Secret is referenced which can be kind: secret & group: core.
	// ref: https://github.com/kubernetes-sigs/gateway-api/pull/562
	if !isSecretRef(certificateRef) {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
			metav1.ConditionFalse,
			gatewayapi_v1alpha2.ListenerReasonInvalidCertificateRef,
			fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q must contain a reference to a core.Secret", certificateRef.Name),
		)
		return nil
	}

	// If the secret is in a different namespace than the gateway, then we need to
	// check for a ReferenceGrant or ReferenceGrant that allows the reference.
	if certificateRef.Namespace != nil && string(*certificateRef.Namespace) != gateway.Namespace {
		grants, err := cb.GetGrants(gateway.Namespace)
		if err != nil {
			return nil
		}
		if !validCrossNamespaceRef(
			grants,
			crossNamespaceFrom{
				group:     gatewayapi_v1alpha2.GroupName,
				kind:      KindGateway,
				namespace: gateway.Namespace,
			},
			crossNamespaceTo{
				group:     "",
				kind:      "Secret",
				namespace: string(*certificateRef.Namespace),
				name:      string(certificateRef.Name),
			},
		) {
			gwAccessor.AddListenerCondition(
				listenerName,
				gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
				metav1.ConditionFalse,
				gatewayapi_v1alpha2.ListenerReasonInvalidCertificateRef,
				fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q namespace must match the Gateway's namespace or be covered by a ReferenceGrant", certificateRef.Name),
			)
			return nil
		}
	}

	var meta types.NamespacedName
	if certificateRef.Namespace != nil {
		meta = types.NamespacedName{Name: string(certificateRef.Name), Namespace: string(*certificateRef.Namespace)}
	} else {
		meta = types.NamespacedName{Name: string(certificateRef.Name), Namespace: gateway.Namespace}
	}

	listenerSecret, err := cb.GetSecret(meta)
	if err != nil || validTLSSecret(listenerSecret) != nil {
		gwAccessor.AddListenerCondition(
			listenerName,
			gatewayapi_v1alpha2.ListenerConditionResolvedRefs,
			metav1.ConditionFalse,
			gatewayapi_v1alpha2.ListenerReasonInvalidCertificateRef,
			fmt.Sprintf("Spec.VirtualHost.TLS.CertificateRefs %q referent is invalid: %s", certificateRef.Name, err),
		)
		return nil
	}
	return listenerSecret
}

type crossNamespaceFrom struct {
	group     string
	kind      string
	namespace string
}

type crossNamespaceTo struct {
	group     string
	kind      string
	namespace string
	name      string
}

func validCrossNamespaceRef(grants gatewayapi_v1alpha2.ReferenceGrantList, from crossNamespaceFrom, to crossNamespaceTo) bool {
	for _, grant := range grants.Items {
		// The ReferenceGrant must be defined in the namespace of
		// the "to" (the referent).
		if grant.Namespace != to.namespace {
			continue
		}

		// Check if the ReferenceGrant has a matching "from".
		var fromAllowed bool
		for _, grantFrom := range grant.Spec.From {
			if string(grantFrom.Namespace) == from.namespace && string(grantFrom.Group) == from.group && string(grantFrom.Kind) == from.kind {
				fromAllowed = true
				break
			}
		}
		if !fromAllowed {
			continue
		}

		// Check if the ReferenceGrant has a matching "to".
		var toAllowed bool
		for _, grantTo := range grant.Spec.To {
			if string(grantTo.Group) == to.group && string(grantTo.Kind) == to.kind && (grantTo.Name == nil || *grantTo.Name == "" || string(*grantTo.Name) == to.name) {
				toAllowed = true
				break
			}
		}
		if !toAllowed {
			continue
		}

		// If we got here, both the "from" and the "to" were allowed by this
		// reference policy.
		return true
	}

	// If we got here, no reference policy or reference grant allowed both the "from" and "to".
	return false
}

// ComputeHosts returns the set of hostnames to match for a route. Both the result
// and the error slice should be considered:
//   - if the set of hostnames is non-empty, it should be used for matching (may be ["*"]).
//   - if the set of hostnames is empty, there was no intersection between the listener
//     hostname and the route hostnames, and the route should be marked "Accepted: false".
//   - if the list of errors is non-empty, one or more hostnames was syntactically
//     invalid and some condition should be added to the route. This shouldn't be
//     possible because of kubebuilder+admission webhook validation but we're being
//     defensive here.
func ComputeHosts(routeHostnames []gatewayapi_v1alpha2.Hostname, rawHostname *gatewayapi_v1alpha2.Hostname) ([]gatewayapi_v1alpha2.Hostname, []error) {
	// The listener hostname is assumed to be valid because it's been run
	// through the `gatewayapi.ValidateListeners` logic, so we don't need
	// to validate it here.

	// If the Listener has a hostname then we'll use that as a default,
	// otherwise we'll use an explicit "all hosts".
	listenerHostname := "*"
	if rawHostname != nil && len(*rawHostname) > 0 {
		listenerHostname = string(*rawHostname)
	}

	// No route hostnames specified: use the listener hostname if specified,
	// or else match all hostnames.
	if len(routeHostnames) == 0 {
		return []gatewayapi_v1alpha2.Hostname{gatewayapi_v1alpha2.Hostname(listenerHostname)}, nil
	}

	hostnames := sets.NewString()
	var errs []error

	for i := range routeHostnames {
		routeHostname := string(routeHostnames[i])

		// If the route hostname is not valid, record an error and skip it.
		if err := IsValidHostname(string(routeHostname)); err != nil {
			errs = append(errs, err)
			continue
		}

		switch {
		// No listener hostname: use the route hostname.
		case len(listenerHostname) == 0:
			hostnames.Insert(routeHostname)

		// Listener hostname matches the route hostname: use it.
		case listenerHostname == routeHostname:
			hostnames.Insert(routeHostname)

		// Listener has a wildcard hostname: check if the route hostname matches.
		case strings.HasPrefix(listenerHostname, "*"):
			if hostnameMatchesWildcardHostname(routeHostname, listenerHostname) {
				hostnames.Insert(routeHostname)
			}

		// Route has a wildcard hostname: check if the listener hostname matches.
		case strings.HasPrefix(routeHostname, "*"):
			if hostnameMatchesWildcardHostname(listenerHostname, routeHostname) {
				hostnames.Insert(listenerHostname)
			}

		}
	}

	if len(hostnames) == 0 {
		return []gatewayapi_v1alpha2.Hostname{}, errs
	}

	// Flatten the set into a []Hostname to return to the caller.
	hosts := []gatewayapi_v1alpha2.Hostname{}
	for _, name := range hostnames.List() {
		hosts = append(hosts, gatewayapi_v1alpha2.Hostname(name))
	}

	return hosts, errs
}

// IsValidHostname validates that a given hostname is syntactically valid.
// It returns nil if valid and an error if not valid.
func IsValidHostname(hostname string) error {
	if net.ParseIP(hostname) != nil {
		return fmt.Errorf("invalid hostname %q: must be a DNS name, not an IP address", hostname)
	}

	if strings.Contains(hostname, "*") {
		if errs := validation.IsWildcardDNS1123Subdomain(hostname); errs != nil {
			return fmt.Errorf("invalid hostname %q: %v", hostname, errs)
		}
	} else {
		if errs := validation.IsDNS1123Subdomain(hostname); errs != nil {
			return fmt.Errorf("invalid hostname %q: %v", hostname, errs)
		}
	}

	return nil
}

// hostnameMatchesWildcardHostname returns true if hostname has the non-wildcard
// portion of wildcardHostname as a suffix, plus at least one DNS label matching the
// wildcard.
func hostnameMatchesWildcardHostname(hostname, wildcardHostname string) bool {
	if !strings.HasSuffix(hostname, strings.TrimPrefix(wildcardHostname, "*")) {
		return false
	}

	wildcardMatch := strings.TrimSuffix(hostname, strings.TrimPrefix(wildcardHostname, "*"))
	return len(wildcardMatch) > 0
}
