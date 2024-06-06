// Copyright Project Contour Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package dag provides a data model, in the form of a directed acyclic graph,
// of the relationship between Kubernetes Ingress, Service, and Secret objects.
package dag

import (
	core_v1 "k8s.io/api/core/v1"
)

// Secret represents a K8s Secret for TLS usage as a DAG Vertex. A Secret is
// a leaf in the DAG.
type Secret struct {
	Object         *core_v1.Secret
	ValidTLSSecret *SecretValidationStatus
	ValidCASecret  *SecretValidationStatus
	ValidCRLSecret *SecretValidationStatus
}

func (s *Secret) Name() string      { return s.Object.Name }
func (s *Secret) Namespace() string { return s.Object.Namespace }

// Data returns the contents of the backing secret's map.
func (s *Secret) Data() map[string][]byte {
	return s.Object.Data
}

// Cert returns the secret's tls certificate
func (s *Secret) Cert() []byte {
	return s.Object.Data[core_v1.TLSCertKey]
}

// PrivateKey returns the secret's tls private key
func (s *Secret) PrivateKey() []byte {
	return s.Object.Data[core_v1.TLSPrivateKeyKey]
}

type SecretValidationStatus struct {
	Error error
}
