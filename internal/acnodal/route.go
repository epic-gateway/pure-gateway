/*
Copyright 2022 Acnodal.
*/

package acnodal

import (
	"fmt"

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
		return &RouteResponse{}, fmt.Errorf("response code %d status \"%s\"", response.StatusCode(), response.Status())
	}
	return response.Result().(*RouteResponse), nil
}

func (e *epic) UpdateRoute(url string, spec RouteSpec) (*RouteResponse, error) {
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
