/*
Copyright 2022 Acnodal.
*/

package acnodal

import (
	"fmt"

	discoveryv1 "k8s.io/api/discovery/v1"
)

type SliceCreate struct {
	Slice Slice `json:"slice"`
}

type Slice struct {
	Spec SliceSpec `json:"spec"`
}

type SliceSpec struct {
	// ClientRef points back to the client-side object that corresponds
	// to this one.
	ClientRef ClientRef `json:"clientRef"`

	// ParentRef points to the client-side service that owns this slice.
	ParentRef ClientRef `json:"parentRef,omitempty"`

	// Slice holds the client-side EndpointSlice contents.
	discoveryv1.EndpointSlice `json:",inline"`

	// Map of node addresses. Key is node name, value is IP address
	// represented as string.
	NodeAddresses map[string]string `json:"nodeAddresses"`
}

// SliceResponse is the body of the HTTP response to a request to show
// an EndpointSlice.
type SliceResponse struct {
	Message string    `json:"message,omitempty"`
	Links   Links     `json:"link"`
	Slice   SliceSpec `json:"slice"`
}

func (n *epic) AnnounceSlice(url string, spec SliceSpec) (*SliceResponse, error) {
	response, err := n.http.R().
		SetBody(SliceCreate{
			Slice: Slice{
				Spec: spec,
			},
		}).
		SetError(SliceResponse{}).
		SetResult(SliceResponse{}).
		Post(url)
	if err != nil {
		return &SliceResponse{}, err
	}
	if response.IsError() {
		return &SliceResponse{}, fmt.Errorf("response code %d status \"%s\"", response.StatusCode(), response.Status())
	}
	return response.Result().(*SliceResponse), nil
}

func (n *epic) UpdateSlice(url string, spec SliceSpec) (*SliceResponse, error) {
	response, err := n.http.R().
		SetBody(SliceCreate{
			Slice: Slice{
				Spec: spec,
			},
		}).
		SetError(SliceResponse{}).
		SetResult(SliceResponse{}).
		Put(url)
	if err != nil {
		return &SliceResponse{}, err
	}
	if response.IsError() {
		return &SliceResponse{}, fmt.Errorf("response code %d status \"%s\"", response.StatusCode(), response.Status())
	}
	return response.Result().(*SliceResponse), nil
}
