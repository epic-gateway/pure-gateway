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

	"epic-gateway.org/puregw/internal/contour/status"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
