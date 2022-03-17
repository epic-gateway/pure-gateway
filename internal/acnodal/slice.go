/*
Copyright 2022 Acnodal.
*/

package acnodal

import (
	"fmt"
	"net/http"

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
	spec.ClientRef.ClusterID = n.clientName

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
		// if the response indicates that this service is already
		// announced then it's not necessarily an error. Try to fetch the
		// service and return that.
		if response.StatusCode() == http.StatusConflict {
			return n.FetchSlice(response.Header().Get(locationHeader))
		}

		// Try to find a helpful error message header, but fall back to
		// the HTTP status message
		message := response.Header().Get(errorMessageHeader)
		if message == "" {
			message = response.Status()
		}

		return &SliceResponse{}, fmt.Errorf("response code %d status \"%s\"", response.StatusCode(), response.Status())
	}
	return response.Result().(*SliceResponse), nil
}

// FetchSlice fetches the service at "url" from the EPIC.
func (n *epic) FetchSlice(url string) (*SliceResponse, error) {
	response, err := n.http.R().
		SetResult(SliceResponse{}).
		Get(url)
	if err != nil {
		return &SliceResponse{}, err
	}
	if response.IsError() {
		return &SliceResponse{}, fmt.Errorf("%s GET response code %d status \"%s\"", url, response.StatusCode(), response.Status())
	}

	srv := response.Result().(*SliceResponse)
	return srv, nil
}

func (n *epic) UpdateSlice(url string, spec SliceSpec) (*SliceResponse, error) {
	spec.ClientRef.ClusterID = n.clientName

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
