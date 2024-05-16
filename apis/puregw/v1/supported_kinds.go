/*
Copyright 2022 Acnodal.
*/
package v1

import (
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	// SupportedKinds contains the list of Kinds that EPIC currently
	// supports.
	SupportedKinds []gatewayv1a2.Kind = []gatewayv1a2.Kind{"HTTPRoute"}
)

// SupportedKind indicates whether the given Kind is supported by EPIC
// or not.
func SupportedKind(kind gatewayv1a2.Kind) bool {
	for _, valid := range SupportedKinds {
		if kind == valid {
			return true
		}
	}
	return false
}
