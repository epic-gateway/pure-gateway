/*
Copyright 2022 Acnodal.
*/

package v1

import (
	"fmt"
	"net/url"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// EPIC configures the allocator to work with the Acnodal Enterprise
// Gateway.
type EPIC struct {
	// ClientName is used to tag resources on the EPIC server. It's
	// purely informative but it's helpful to have each client cluster
	// have a different name.
	// +kubebuilder:default="epic-gateway"
	ClientName string `json:"client-name,omitempty"`

	Host       string `json:"gateway-hostname"`
	SvcAccount string `json:"service-account"`
	SvcKey     string `json:"service-key"`
	UserNS     string `json:"user-namespace"`
	GWTemplate string `json:"gateway-template"`
}

// Attachment contains the configuration data that lets us attach
// our packet forwarding components to a network interface.
type Attachment struct {
	// Interface is the name of the interface.
	Interface string `json:"interface"`

	// Direction is either "ingress" or "egress".
	Direction string `json:"direction"`

	// Flags configure the PFC component's behavior.
	Flags int `json:"flags"`

	// QID is a magic parameter that the PFC needs.
	QID int `json:"qid"`
}

// TrueIngress configures the announcers to announce service
// addresses to the Acnodal Enterprise GateWay.
type TrueIngress struct {
	// EncapAttachment configures how the agent will attach the Packet
	// Forwarding Components for packet encapsulation.
	EncapAttachment Attachment `json:"encapAttachment"`

	// DecapAttachment configures how the agent will attach the Packet
	// Forwarding Components for packet decapsulation.
	DecapAttachment Attachment `json:"decapAttachment"`
}

// GatewayClassConfigSpec configures the EPIC Gateway client.  It has
// two parts: one for communication with the EPIC server, and one for
// configuration of the TrueIngress components on the client
// cluster. For examples, see the "config/" directory in the PureGW
// source tree.
type GatewayClassConfigSpec struct {
	EPIC        *EPIC        `json:"epic"`
	TrueIngress *TrueIngress `json:"trueIngress"`
}

// EPICAPIServiceURL returns the URL to connect to the EPIC instance
// described by this ServiceGroupEPICSpec.
func (gccs *GatewayClassConfigSpec) EPICAPIServiceURL() (*url.URL, error) {
	return url.Parse(fmt.Sprintf("https://%s/api/epic/accounts/%s/groups/%s", gccs.EPIC.Host, gccs.EPIC.UserNS, gccs.EPIC.GWTemplate))
}

// GatewayClassConfigStatus defines the observed state of GatewayClassConfig
type GatewayClassConfigStatus struct {
	// Conditions is the current status from the controller for
	// this GatewayClassConfig.
	//
	// +optional
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:default={{type: "Accepted", status: "Unknown", message: "Waiting for controller", reason: "Waiting", lastTransitionTime: "1970-01-01T00:00:00Z"}}
	Conditions []metav1.Condition `json:"conditions,omitempty"`
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

func (gcc *GatewayClassConfig) NamespacedName() types.NamespacedName {
	return types.NamespacedName{Namespace: gcc.Namespace, Name: gcc.Name}
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
