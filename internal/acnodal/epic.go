/*
Copyright 2022 Acnodal.
*/

//go:generate mockgen -destination internal/acnodal/epic_mock.go -package puregw.io/internal/acnodal

package acnodal

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/go-resty/resty/v2"
	v1 "k8s.io/api/core/v1"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	puregwv1 "acnodal.io/puregw/apis/puregw/v1"
)

const (
	locationHeader     = "Location"
	errorMessageHeader = "x-epic-error-message"
)

// EPIC represents one connection to an Acnodal Enterprise Gateway.
type EPIC interface {
	GetAccount() (AccountResponse, error)
	GetGroup() (GroupResponse, error)
	AnnounceGateway(url string, name string, ports []gatewayv1a2.Listener) (ServiceResponse, error)
	FetchGateway(url string) (ServiceResponse, error)
	Delete(svcUrl string) error
}

// epic represents one connection to an Acnodal Enterprise Gateway.
type epic struct {
	http      resty.Client
	groupURL  string
	authToken string
}

// Links holds a map of URL strings.
type Links map[string]string

// ObjectMeta is a shadow of the k8s ObjectMeta struct.
type ObjectMeta struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// Account is the on-the-wire representation of one EPIC Account.
type Account struct {
	ObjectMeta ObjectMeta  `json:"metadata"`
	Spec       AccountSpec `json:"spec"`
}

// AccountSpec is the on-the-wire representation of one Account
// Spec (i.e., the part that defines what the Account should look
// like).
type AccountSpec struct {
	GroupID uint16 `json:"group-id"`
}

// Group is the on-the-wire representation of one Service Group.
type Group struct {
	ObjectMeta ObjectMeta `json:"metadata"`
}

type DNSEndpoint struct {
	// The hostname of the DNS record
	DNSName string `json:"dnsName,omitempty"`
}

// ServiceSpec is the on-the-wire representation of one
// LoadBalancer Service Spec (i.e., the part that defines what the LB
// should look like).
type ServiceSpec struct {
	DisplayName string           `json:"display-name"`
	Address     string           `json:"public-address,omitempty"`
	Ports       []v1.ServicePort `json:"public-ports"`
	ServiceID   uint16           `json:"service-id"`
	TrueIngress bool             `json:"true-ingress"`

	// Endpoints are the objects that EPIC uses to trigger external-dns
	// to write DNS records. We also get the EPIC-assigned hostname from
	// them.
	Endpoints []*DNSEndpoint `json:"endpoints,omitempty"`

	// TunnelKey authenticates the client with the EPIC. It must be a
	// base64-encoded 128-bit value.
	TunnelKey string `json:"tunnel-key,omitempty"`

	// GUETunnelEndpoints is a map of maps. The outer map is from client
	// node addresses to public GUE tunnel endpoints on the EPIC. The
	// map key is a client node address and must be one of the node
	// addresses in the Spec Endpoints slice. The value is a map
	// containing TunnelEndpoints that describes the public IPs and
	// ports to which the client can send tunnel ping packets. The key
	// is the IP address of the EPIC node and the value is a
	// TunnelEndpoint.
	TunnelEndpoints map[string]EndpointMap `json:"gue-tunnel-endpoints"`
}

// EndpointMap contains a map of the EPIC endpoints that connect
// to one PureGW endpoint, keyed by Node IP address.
type EndpointMap struct {
	EPICEndpoints map[string]TunnelEndpoint `json:"epic-endpoints,omitempty"`
}

// TunnelEndpoint is an Endpoint on the EPIC.
type TunnelEndpoint struct {
	// Address is the IP address for this endpoint.
	Address string `json:"epic-address"`

	// Port is the port on which this endpoint listens.
	Port v1.EndpointPort `json:"epic-port"`

	// TunnelID distinguishes the traffic using this tunnel from the
	// traffic using other tunnels that end on the same host.
	TunnelID uint32 `json:"tunnel-id"`
}

type ServiceStatus struct {
}

// Service is the on-the-wire representation of one LoadBalancer
// Service.
type Service struct {
	ObjectMeta ObjectMeta    `json:"metadata"`
	Spec       ServiceSpec   `json:"spec"`
	Status     ServiceStatus `json:"status,omitempty"`
}

// AccountResponse is the body of the HTTP response to a request to
// show an account.
type AccountResponse struct {
	Links   Links   `json:"link"`
	Account Account `json:"account"`
}

// GroupResponse is the body of the HTTP response to a request to show
// a service group.
type GroupResponse struct {
	Message string `json:"message,omitempty"`
	Links   Links  `json:"link"`
	Group   Group  `json:"group"`
}

// ServiceCreate is the body of the HTTP request to create a load
// balancer service.
type ServiceCreate struct {
	Service Service `json:"service"`
}

// ServiceResponse is the body of the HTTP response to a request to
// show a load balancer.
type ServiceResponse struct {
	Message string  `json:"message,omitempty"`
	Links   Links   `json:"link"`
	Service Service `json:"service"`
}

// New initializes a new EPIC instance. If error is non-nil then the
// instance shouldn't be used.
func NewEPIC(config puregwv1.GatewayClassConfigSpec) (EPIC, error) {
	// Use the hostname from the service group, but reset the path.  EPIC
	// and Netbox each have their own API URL schemes so we only need
	// the protocol, host, port, credentials, etc.
	baseURL, err := url.Parse(config.EPICAPIServiceURL())
	if err != nil {
		return nil, err
	}
	baseURL.Path = "/"

	// Set up a REST client to talk to the EPIC
	r := resty.New().
		SetHostURL(baseURL.String()).
		SetHeaders(map[string]string{
			"Content-Type": "application/json",
			"accept":       "application/json",
		}).
		SetBasicAuth(config.EPIC.SvcAccount, config.EPIC.SvcKey).
		SetRetryCount(2).
		SetRetryWaitTime(time.Second).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true}). // FIXME: figure out how to *not* disable cert checks
		SetRedirectPolicy(resty.FlexibleRedirectPolicy(2))

	// Initialize the EPIC instance
	return &epic{http: *r, groupURL: config.EPICAPIServiceURL()}, nil
}

// GetAccount requests an account from the EPIC.
func (n *epic) GetAccount() (AccountResponse, error) {
	group, err := n.GetGroup()
	if err != nil {
		return AccountResponse{}, err
	}

	url := group.Links["account"]
	response, err := n.http.R().
		SetResult(AccountResponse{}).
		Get(url)
	if err != nil {
		return AccountResponse{}, err
	}
	if response.IsError() {
		return AccountResponse{}, fmt.Errorf("%s GET response code %d status \"%s\"", url, response.StatusCode(), response.Status())
	}

	srv := response.Result().(*AccountResponse)
	return *srv, nil
}

// GetGroup requests a service group from the EPIC.
func (n *epic) GetGroup() (GroupResponse, error) {
	response, err := n.http.R().
		SetResult(GroupResponse{}).
		Get(n.groupURL)
	if err != nil {
		return GroupResponse{}, err
	}
	if response.IsError() {
		return GroupResponse{}, fmt.Errorf("%s GET response code %d status \"%s\"", n.groupURL, response.StatusCode(), response.Status())
	}

	srv := response.Result().(*GroupResponse)
	return *srv, nil
}

// AnnounceService announces a service to the EPIC. url is the service
// creation URL which is a child of the service group to which this
// service will belong. name is the service name.  address is a string
// containing an IP address. ports is a slice of v1.ServicePorts.
func (n *epic) AnnounceGateway(url string, name string, sPorts []gatewayv1a2.Listener) (ServiceResponse, error) {
	// Translate from the Gateway's Listener struct to k8s/EPIC's
	// ServicePort
	ports, err := ListenersToPorts(sPorts)
	if err != nil {
		return ServiceResponse{}, err
	}

	// send the request
	response, err := n.http.R().
		SetBody(ServiceCreate{
			Service: Service{ObjectMeta: ObjectMeta{Name: name}, Spec: ServiceSpec{TrueIngress: true, DisplayName: name, Ports: ports}}}).
		SetResult(ServiceResponse{}).
		Post(url)
	if err != nil {
		return ServiceResponse{}, err
	}
	if response.IsError() {
		// if the response indicates that this service is already
		// announced then it's not necessarily an error. Try to fetch the
		// service and return that.
		if response.StatusCode() == http.StatusConflict {
			return n.FetchGateway(response.Header().Get(locationHeader))
		}

		// Try to find a helpful error message header, but fall back to
		// the HTTP status message
		message := response.Header().Get(errorMessageHeader)
		if message == "" {
			message = response.Status()
		}

		return ServiceResponse{}, fmt.Errorf("%s POST response code %d message \"%s\"", url, response.StatusCode(), message)
	}

	srv := response.Result().(*ServiceResponse)
	return *srv, nil
}

// FetchService fetches the service at "url" from the EPIC.
func (n *epic) FetchGateway(url string) (ServiceResponse, error) {
	response, err := n.http.R().
		SetResult(ServiceResponse{}).
		Get(url)
	if err != nil {
		return ServiceResponse{}, err
	}
	if response.IsError() {
		return ServiceResponse{}, fmt.Errorf("%s GET response code %d status \"%s\"", url, response.StatusCode(), response.Status())
	}

	srv := response.Result().(*ServiceResponse)
	return *srv, nil
}

// Delete tells the EPIC that this object should be deleted.
func (n *epic) Delete(url string) error {
	response, err := n.http.R().Delete(url)
	if err != nil {
		return err
	}
	if response.IsError() {
		return fmt.Errorf("%s DELETE response code %d status \"%s\"", url, response.StatusCode(), response.Status())
	}
	return nil
}

func ListenersToPorts(listeners []gatewayv1a2.Listener) ([]v1.ServicePort, error) {
	return []v1.ServicePort{}, nil
}
