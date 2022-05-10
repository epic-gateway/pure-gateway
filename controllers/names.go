package controllers

import (
	"os"

	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

const (
	GatewayController = gatewayv1a2.GatewayController("acnodal.io/epic")

	agentFinalizerPrefix = "epic.acnodal.io/agent_"
	FinalizerName        = "epic.acnodal.io/controller"
)

// AgentFinalizerName returns the finalizer name for the given
// nodeName.
func AgentFinalizerName() string {
	return agentFinalizerPrefix + os.Getenv("EPIC_NODE_NAME")
}
