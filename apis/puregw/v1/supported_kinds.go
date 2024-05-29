/*
Copyright 2022 Acnodal.
*/
package v1

import (
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

var (
	// SupportedKinds contains the list of Kinds that EPIC currently
	// supports.
	SupportedKinds []gatewayapi.Kind = []gatewayapi.Kind{"HTTPRoute"}
)

// SupportedKind indicates whether the given Kind is supported by EPIC
// or not.
func SupportedKind(kind gatewayapi.Kind) bool {
	for _, valid := range SupportedKinds {
		if kind == valid {
			return true
		}
	}
	return false
}
