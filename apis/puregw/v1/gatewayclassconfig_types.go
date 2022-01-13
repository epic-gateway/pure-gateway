/*
Copyright 2022 Acnodal.
*/

package v1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EPIC configures the allocator to work with the Acnodal Enterprise
// Gateway.
type EPIC struct {
	Host       string `json:"gateway-hostname"`
	SvcAccount string `json:"service-account"`
	SvcKey     string `json:"service-key"`
	UserNS     string `json:"user-namespace"`
	GWTemplate string `json:"gateway-template"`
}

// GatewayClassConfigSpec configures the allocator.  For examples, see
// the "config/" directory in the PureGW source tree.
type GatewayClassConfigSpec struct {
	EPIC *EPIC `json:"epic"`
}

// EPICAPIServiceURL returns the URL to connect to the EPIC instance
// described by this ServiceGroupEPICSpec.
func (gccs *GatewayClassConfigSpec) EPICAPIServiceURL() string {
	return fmt.Sprintf("https://%s/api/epic/accounts/%s/groups/%s", gccs.EPIC.Host, gccs.EPIC.UserNS, gccs.EPIC.GWTemplate)
}

// GatewayClassConfigStatus defines the observed state of GatewayClassConfig
type GatewayClassConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// GatewayClassConfig is the Schema for the gatewayclassconfigs API
type GatewayClassConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatewayClassConfigSpec   `json:"spec,omitempty"`
	Status GatewayClassConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// GatewayClassConfigList contains a list of GatewayClassConfig
type GatewayClassConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GatewayClassConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GatewayClassConfig{}, &GatewayClassConfigList{})
}
