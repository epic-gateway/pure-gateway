package controllers

import (
	"os"

	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

const (
	GatewayController = gatewayapi.GatewayController("epic-gateway.org/puregw")

	agentFinalizerPrefix = "puregw.epic-gateway.org/agent_"
	FinalizerName        = "puregw.epic-gateway.org/manager"
)

// AgentFinalizerName returns the finalizer name for the given
// nodeName.
func AgentFinalizerName() string {
	return agentFinalizerPrefix + os.Getenv("EPIC_NODE_NAME")
}
