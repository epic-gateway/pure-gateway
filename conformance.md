EPIC Gateway Conformance
========================

Relative to specification version v1alpha2 - https://gateway-api.sigs.k8s.io/v1alpha2/references/spec/

| Feature                                                        | Optional? | Support                 | Supported? |
|----------------------------------------------------------------|-----------|-------------------------|------------|
| Gateway.spec.listeners.hostname                                | Optional  | Core                    | No         |
| Gateway.spec.listeners.tls                                     | Optional  | Core                    | No         |
| Gateway.spec.listeners.allowedRoutes.namespaces                | Optional  | Core                    | No         |
| Gateway.spec.listeners.allowedRoutes.kinds                     | Optional  | Core                    | No         |
| Gateway.spec.addresses                                         | Optional  | Extended                | No         |
| Gateway.status.addresses                                       | Optional  | Core                    | Yes        |
| Gateway.status.conditions                                      | Optional  | Core                    | No         |
| Gateway.status.listeners                                       | Optional  | Core                    | No         |
|                                                                |           |                         |            |
| HTTPRoute.spec.parentRefs to Gateway                           |           | ?                       | Yes        |
| HTTPRoute.spec.parentRefs to Other                             | Optional  | ?                       | No         |
| HTTPRoute.spec.hostnames                                       | Optional  | Core                    | Yes        |
| HTTPRoute.spec.rules.matches.path Exact                        | Optional  | Core                    | Yes        |
| HTTPRoute.spec.rules.matches.path PathPrefix                   | Optional  | Core                    | Yes        |
| HTTPRoute.spec.rules.matches.path RegularExpression            | Optional  | Custom                  | No         |
| HTTPRoute.spec.rules.matches.headers Exact                     | Optional  | Core                    | Yes        |
| HTTPRoute.spec.rules.matches.headers RegularExpression         | Optional  | Custom                  | Yes        |
| HTTPRoute.spec.rules.matches.queryParams Exact                 | Optional  | Extended                | No         |
| HTTPRoute.spec.rules.matches.queryParams RegularExpression     | Optional  | Custom                  | No         |
| HTTPRoute.spec.rules.matches.method                            | Optional  | Extended                | No         |
| HTTPRoute.spec.rules.filters.requestHeaderModifier             | Optional  | Core                    | No         |
| HTTPRoute.spec.rules.filters.requestMirror                     | Optional  | Extended                | No         |
| HTTPRoute.spec.rules.filters.requestRedirect                   | Optional  | Core                    | No         |
| HTTPRoute.spec.rules.filters.urlRewrite                        | Optional  | Extended                | No         |
| HTTPRoute.spec.rules.filters.extensionRef                      | Optional  | Implementation-specific | No         |
| HTTPRoute.spec.rules.backendRefs to Kubernetes Service         | Optional  | Core                    | Yes        |
| HTTPRoute.spec.rules.backendRefs to other resources            | Optional  | Custom                  | No         |
| HTTPRoute.spec.rules.backendRefs support for weight            | Optional  | Core                    | Yes        |
| HTTPRoute.spec.rules.backendRefs.filters.requestHeaderModifier | Optional  | Core                    | No         |
| HTTPRoute.spec.rules.backendRefs.filters.requestMirror         | Optional  | Extended                | No         |
| HTTPRoute.spec.rules.backendRefs.filters.requestRedirect       | Optional  | Core                    | No         |
| HTTPRoute.spec.rules.backendRefs.filters.urlRewrite            | Optional  | Extended                | No         |
|                                                                |           |                         |            |
| HTTPRoute.status.RouteParentStatus                             |           | Core                    | No         |
|                                                                |           |                         |            |
| TCPRoute                                                       |           |                         | No         |
| TLSRoute                                                       |           |                         | No         |
| UDPRoute                                                       |           |                         | No         |
|                                                                |           |                         |            |
| ReferenceGrant/ReferencePolicy                                 |           | Core                    | No         |
|                                                                |           |                         |            |

