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

package dag

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	gatewayapi "sigs.k8s.io/gateway-api/apis/v1"
)

func TestComputeHosts(t *testing.T) {
	tests := map[string]struct {
		listenerHost gatewayapi.Hostname
		hostnames    []gatewayapi.Hostname
		want         []gatewayapi.Hostname
		wantError    []error
	}{
		"single host": {
			listenerHost: gatewayapi.Hostname(""),
			hostnames: []gatewayapi.Hostname{
				"test.projectcontour.io",
			},
			want: []gatewayapi.Hostname{
				"test.projectcontour.io",
			},
			wantError: nil,
		},
		"single DNS label hostname": {
			listenerHost: "",
			hostnames: []gatewayapi.Hostname{
				"projectcontour",
			},
			want: []gatewayapi.Hostname{
				"projectcontour",
			},
			wantError: nil,
		},
		"multiple hosts": {
			listenerHost: "",
			hostnames: []gatewayapi.Hostname{
				"test.projectcontour.io",
				"test1.projectcontour.io",
				"test2.projectcontour.io",
				"test3.projectcontour.io",
			},
			want: []gatewayapi.Hostname{
				"test.projectcontour.io",
				"test1.projectcontour.io",
				"test2.projectcontour.io",
				"test3.projectcontour.io",
			},
			wantError: nil,
		},
		"no host": {
			listenerHost: "",
			hostnames:    []gatewayapi.Hostname{},
			want:         []gatewayapi.Hostname{"*"},
			wantError:    []error(nil),
		},
		"IP in host": {
			listenerHost: "",
			hostnames: []gatewayapi.Hostname{
				"1.2.3.4",
			},
			want: []gatewayapi.Hostname{},
			wantError: []error{
				fmt.Errorf("invalid hostname \"1.2.3.4\": must be a DNS name, not an IP address"),
			},
		},
		"valid wildcard hostname": {
			listenerHost: "",
			hostnames: []gatewayapi.Hostname{
				"*.projectcontour.io",
			},
			want: []gatewayapi.Hostname{
				"*.projectcontour.io",
			},
			wantError: nil,
		},
		"invalid wildcard hostname": {
			listenerHost: "",
			hostnames: []gatewayapi.Hostname{
				"*.*.projectcontour.io",
			},
			want: []gatewayapi.Hostname{},
			wantError: []error{
				fmt.Errorf("invalid hostname \"*.*.projectcontour.io\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"invalid wildcard hostname *": {
			listenerHost: "",
			hostnames: []gatewayapi.Hostname{
				"*",
			},
			want:      []gatewayapi.Hostname{},
			wantError: []error{fmt.Errorf("invalid hostname \"*\": [a wildcard DNS-1123 subdomain must start with '*.', followed by a valid DNS subdomain, which must consist of lower case alphanumeric characters, '-' or '.' and end with an alphanumeric character (e.g. '*.example.com', regex used for validation is '\\*\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]")},
		},
		"invalid hostname": {
			listenerHost: "",
			hostnames: []gatewayapi.Hostname{
				"#projectcontour.io",
			},
			want: []gatewayapi.Hostname{},
			wantError: []error{
				fmt.Errorf("invalid hostname \"#projectcontour.io\": [a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character (e.g. 'example.com', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*')]"),
			},
		},
		"listener host & hostnames host do not exactly match": {
			listenerHost: "listener.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			want:      []gatewayapi.Hostname{},
			wantError: nil,
		},
		"listener host & hostnames host exactly match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			want: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			wantError: nil,
		},
		"listener host & multi hostnames host exactly match one host": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"http.projectcontour.io",
				"http2.projectcontour.io",
				"http3.projectcontour.io",
			},
			want:      []gatewayapi.Hostname{"http.projectcontour.io"},
			wantError: nil,
		},
		"listener host & hostnames host match wildcard host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			want: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			wantError: nil,
		},
		"listener host & hostnames host do not match wildcard host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"http.example.com",
			},
			want:      []gatewayapi.Hostname{},
			wantError: nil,
		},
		"listener host & wildcard hostnames host do not match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"*.projectcontour.io",
			},
			want: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			wantError: nil,
		},
		"listener host & wildcard hostname and matching hostname match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"*.projectcontour.io",
				"http.projectcontour.io",
			},
			want: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			wantError: nil,
		},
		"listener host & wildcard hostname and non-matching hostname don't match": {
			listenerHost: "http.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"*.projectcontour.io",
				"not.matching.io",
			},
			want: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			wantError: nil,
		},
		"listener host wildcard & wildcard hostnames host match": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"*.projectcontour.io",
			},
			want: []gatewayapi.Hostname{
				"*.projectcontour.io",
			},
			wantError: nil,
		},
		"listener host & hostname not defined match": {
			listenerHost: "http.projectcontour.io",
			hostnames:    []gatewayapi.Hostname{},
			want: []gatewayapi.Hostname{
				"http.projectcontour.io",
			},
			wantError: nil,
		},
		"listener host with many labels matches hostnames wildcard host": {
			listenerHost: "very.many.labels.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"*.projectcontour.io",
			},
			want:      []gatewayapi.Hostname{"very.many.labels.projectcontour.io"},
			wantError: nil,
		},
		"listener wildcard host matches hostnames with many labels host": {
			listenerHost: "*.projectcontour.io",
			hostnames: []gatewayapi.Hostname{
				"very.many.labels.projectcontour.io",
			},
			want:      []gatewayapi.Hostname{"very.many.labels.projectcontour.io"},
			wantError: nil,
		},
		"listener wildcard host doesn't match bare hostname": {
			listenerHost: "*.foo",
			hostnames: []gatewayapi.Hostname{
				"foo",
			},
			want:      []gatewayapi.Hostname{},
			wantError: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, gotError := ComputeHosts(tc.hostnames, &tc.listenerHost)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, tc.wantError, gotError)
		})
	}
}
