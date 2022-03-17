/*
Copyright 2022 Acnodal.
*/

package acnodal

import (
	"fmt"
	"net/http"

	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"
)

type RouteCreate struct {
	Route Route `json:"route"`
}

type Route struct {
	ObjectMeta ObjectMeta `json:"metadata"`
	Spec       RouteSpec  `json:"spec"`
}

// RouteSpec is a container that can hold different kinds of route
// objects, for example, HTTPRoute.
type RouteSpec struct {
	// ClientRef points back to the client-side object that corresponds
	// to this one.
	ClientRef ClientRef `json:"clientRef,omitempty"`

	HTTP gatewayv1a2.HTTPRouteSpec `json:"http,omitempty"`
}

// RouteResponse is the body of the HTTP response to a request to show
// a Gateway Route.
type RouteResponse struct {
	Message string    `json:"message,omitempty"`
	Links   Links     `json:"link"`
	Route   RouteSpec `json:"route"`
}

func (e *epic) AnnounceRoute(url string, spec RouteSpec) (*RouteResponse, error) {
	spec.ClientRef.ClusterID = e.clientName

	response, err := e.http.R().
		SetBody(RouteCreate{
			Route: Route{
				Spec: spec,
			},
		}).
		SetError(RouteResponse{}).
		SetResult(RouteResponse{}).
		Post(url)
	if err != nil {
		return &RouteResponse{}, err
	}
	if response.IsError() {
		// if the response indicates that this service is already
		// announced then it's not necessarily an error. Try to fetch the
		// service and return that.
		if response.StatusCode() == http.StatusConflict {
			return e.FetchRoute(response.Header().Get(locationHeader))
		}

		// Try to find a helpful error message header, but fall back to
		// the HTTP status message
		message := response.Header().Get(errorMessageHeader)
		if message == "" {
			message = response.Status()
		}

		return &RouteResponse{}, fmt.Errorf("response code %d status \"%s\"", response.StatusCode(), response.Status())
	}
	return response.Result().(*RouteResponse), nil
}

// FetchRoute fetches the service at "url" from the EPIC.
func (n *epic) FetchRoute(url string) (*RouteResponse, error) {
	response, err := n.http.R().
		SetResult(RouteResponse{}).
		Get(url)
	if err != nil {
		return &RouteResponse{}, err
	}
	if response.IsError() {
		return &RouteResponse{}, fmt.Errorf("%s GET response code %d status \"%s\"", url, response.StatusCode(), response.Status())
	}

	srv := response.Result().(*RouteResponse)
	return srv, nil
}

func (e *epic) UpdateRoute(url string, spec RouteSpec) (*RouteResponse, error) {
	spec.ClientRef.ClusterID = e.clientName
	response, err := e.http.R().
		SetBody(RouteCreate{
			Route: Route{
				Spec: spec,
			},
		}).
		SetError(RouteResponse{}).
		SetResult(RouteResponse{}).
		Put(url)
	if err != nil {
		return &RouteResponse{}, err
	}
	if response.IsError() {
		return &RouteResponse{}, fmt.Errorf("response code %d status \"%s\"", response.StatusCode(), response.Status())
	}
	return response.Result().(*RouteResponse), nil
}
