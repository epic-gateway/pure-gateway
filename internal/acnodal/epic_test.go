/*
Copyright 2022 Acnodal.
*/

package acnodal

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicgwv1 "acnodal.io/puregw/apis/puregw/v1"
)

var (
	Listener80     = gatewayv1a2.Listener{Name: "listener80", Port: 80, Protocol: gatewayv1a2.HTTPProtocolType}
	EndpointPort80 = v1.EndpointPort{Port: 80}
	EndpointPort81 = v1.EndpointPort{Port: 81}
)

const (
	TestHarnessEPIC = "acndev-ctl"
	UserNS          = "root"
	GroupName       = "gatewayhttp"
	ServiceAccount  = "user1"
	ServiceKey      = "password1"
	NodeName        = "mk8s1"

	ServiceName     = "puregw-integration-test"
	EndpointName    = "test-endpoint"
	EndpointAddress = "10.42.27.42"
	GroupURL        = "/api/epic/accounts/" + UserNS + "/groups/" + GroupName
)

func TestMain(m *testing.M) {
	flag.Parse()

	if testing.Short() {
		fmt.Println("Skipping epic tests because short testing was requested.")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func MustEPIC(t *testing.T) EPIC {
	cs := epicgwv1.GatewayClassConfigSpec{
		EPIC: &epicgwv1.EPIC{
			Host:       TestHarnessEPIC,
			UserNS:     UserNS,
			GWTemplate: GroupName,
		},
	}
	u, err := cs.EPICAPIServiceURL()
	if err != nil {
		t.Fatal("formatting EPIC URL", err)
	}
	e, err := NewEPIC(u, ServiceAccount, ServiceKey, "unit-test")
	if err != nil {
		t.Fatal("initializing EPIC", err)
	}
	return e
}

func GetGroup(t *testing.T, e EPIC, url string) GroupResponse {
	g, err := e.GetGroup()
	if err != nil {
		t.Fatal("getting group", err)
	}
	return g
}

func TestGetGroup(t *testing.T) {
	e := MustEPIC(t)
	g := GetGroup(t, e, GroupURL)
	gotName := g.Group.ObjectMeta.Name
	assert.Equal(t, gotName, GroupName, "group name mismatch")
}

func TestGatewayAnnouncements(t *testing.T) {
	e := MustEPIC(t)
	g := GetGroup(t, e, GroupURL)

	gw := gatewayv1a2.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "test-ns",
			Name:      "test-name",
			UID:       "a044f348-f80c-4ac0-b911-6ed45e37994a",
		},
		Spec: gatewayv1a2.GatewaySpec{
			Listeners: []gatewayv1a2.Listener{Listener80},
			Addresses: []gatewayv1a2.GatewayAddress{},
		},
	}

	// announce a gateway
	svc, err := e.AnnounceGateway(g.Links["create-proxy"], gw)
	if err != nil {
		t.Fatal("announcing gateway", err)
	}
	assert.Equal(t, svc.Links["group"], GroupURL, "group url mismatch")
}

func TestSliceAnnouncements(t *testing.T) {
	var TCP v1.Protocol = v1.ProtocolTCP

	e := MustEPIC(t)
	a, err := e.GetAccount()
	if err != nil {
		t.Errorf("got error %+v", err)
	}

	slice := SliceSpec{
		ClientRef: ClientRef{
			ClusterID: "test",
			Namespace: "puregw",
			Name:      GroupName,
			UID:       "a044f348-f80c-4ac0-b911-6ed45e37994a",
		},
		ParentRef: ClientRef{
			Namespace: "test-ns",
			Name:      "test-service",
			UID:       "a044f348-f80c-4ac0-b911-6ed45e37994b",
		},
		EndpointSlice: discoveryv1.EndpointSlice{
			TypeMeta:    metav1.TypeMeta{},
			ObjectMeta:  metav1.ObjectMeta{},
			AddressType: "IPv4",
			Endpoints: []discoveryv1.Endpoint{{
				Addresses: []string{EndpointAddress},
				NodeName:  pointer.StringPtr("mk8s1"),
			}},
			Ports: []discoveryv1.EndpointPort{{
				Name:     pointer.StringPtr("http"),
				Protocol: &TCP,
				Port:     pointer.Int32Ptr(8080),
			}},
		},
		NodeAddresses: map[string]string{"mk8s1": "192.168.254.101"},
	}
	rt, err := e.AnnounceSlice(a.Links["create-slice"], slice)
	if err != nil {
		t.Errorf("got error %+v", err)
		return
	}

	// Delete the Slice
	if err := e.Delete(rt.Links["self"]); err != nil {
		t.Errorf("got error %+v", err)
	}
}

func TestRouteAnnouncements(t *testing.T) {
	e := MustEPIC(t)
	a, err := e.GetAccount()
	if err != nil {
		t.Errorf("got error %+v", err)
	}

	route := RouteSpec{
		ClientRef: ClientRef{
			ClusterID: "test",
			Namespace: "puregw",
			Name:      GroupName,
			UID:       "a044f348-f80c-4ac0-b911-6ed45e37994a",
		},
		HTTP: gatewayv1a2.HTTPRouteSpec{Hostnames: []gatewayv1a2.Hostname{"test-host", "other-host"}},
	}
	rt, err := e.AnnounceRoute(a.Links["create-route"], route)
	if err != nil {
		t.Errorf("got error %+v", err)
	}

	// Delete the Route
	if err := e.Delete(rt.Links["self"]); err != nil {
		t.Errorf("got error %+v", err)
	}
}
