/*
Copyright 2022 Acnodal.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

// EndpointSliceShadowSpec defines the desired state of EndpointSliceShadow
type EndpointSliceShadowSpec struct {
	// EPICConfigName is the name of the GatewayClassConfig that was
	// used to announce this slice to EPIC.
	EPICConfigName string `json:"epicConfigName"`

	// EPICLink is this slice's URL on the EPIC system.
	EPICLink string `json:"epicLink"`

	// ParentRoutes provides an efficient way to link back to the
	// HTTPRoutes that reference this slice.
	ParentRoutes []gatewayv1a2.ParentReference `json:"parentRoutes"`
}

// EndpointSliceShadowStatus defines the observed state of EndpointSliceShadow
type EndpointSliceShadowStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=epshadow;epshadows
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".spec.epicConfigName",name=Config Name,type=string
//+kubebuilder:printcolumn:JSONPath=".spec.epicLink",name=EPIC Link,type=string

// EndpointSliceShadow is a kludge because of a difference in the way
// that EndpointSlice works compared to basically every other k8s
// object. When you scale a service down to zero endpoints the
// EndpointSlice controller BLOWS AWAY THE SLICE ANNOTATIONS AND
// FINALIZERS. This is incredibly hostile, but when I opened an issue
// the devs doubled down and indicated that this was as intended.
//
// Since we can't mark the EndpointSlice in the usual way we have this
// "shadow" resource that holds the extra data that we need.
type EndpointSliceShadow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EndpointSliceShadowSpec   `json:"spec,omitempty"`
	Status EndpointSliceShadowStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// EndpointSliceShadowList contains a list of EndpointSliceShadow
type EndpointSliceShadowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EndpointSliceShadow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EndpointSliceShadow{}, &EndpointSliceShadowList{})
}
