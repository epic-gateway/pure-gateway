/*
Copyright 2022 Acnodal.
*/
package v1

import (
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

var (
	SupportedKinds []gatewayv1a2.Kind = []gatewayv1a2.Kind{"HTTPRoute"}
)
