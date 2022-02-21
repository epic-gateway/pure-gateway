Sample Configs
==============

You can install these in this order to get a trivial EPIC Gateway in the acndev environment.

* [GatewayClassConfig](puregw_v1_gatewayclassconfig.yaml) - an Acnodal GatewayClassConfig that works with EPIC in the acndev environment
* [GatewayClass](gateway_v1a2_gatewayclass.yaml) - one GatewayClass for EPIC, and one "not EPIC" class for testing
* [KUARD Deployment](apps_v1_deployment.yaml)
* [KUARD Service](core_v1_service.yaml) - lets the sample HTTPRoute find the KUARD endpoints
* [Gateway](gateway_v1a2_gateway.yaml) - a trivial EPIC Gateway, and a "not EPIC" gateway for testing
* [HTTPRoute](gateway_v1a2_httproute.yaml) - a trivial route that binds the Gateway to the Service
