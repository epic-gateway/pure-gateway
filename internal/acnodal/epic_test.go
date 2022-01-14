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
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	puregwv1 "acnodal.io/puregw/apis/puregw/v1"
)

var (
	Listener80     = gatewayv1a2.Listener{Port: 80}
	EndpointPort80 = v1.EndpointPort{Port: 80}
	EndpointPort81 = v1.EndpointPort{Port: 81}
)

const (
	TestHarnessEPIC = "epic-ctl"
	UserNS          = "root"
	GroupName       = "samplehttp"
	ServiceAccount  = "user1"
	ServiceKey      = "password1"

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
	e, err := NewEPIC(puregwv1.GatewayClassConfigSpec{
		EPIC: &puregwv1.EPIC{
			Host:       TestHarnessEPIC,
			SvcAccount: ServiceAccount,
			SvcKey:     ServiceKey,
			UserNS:     UserNS,
			GWTemplate: GroupName,
		},
	})
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

func TestAnnouncements(t *testing.T) {
	e := MustEPIC(t)
	g := GetGroup(t, e, GroupURL)

	// announce a service
	svc, err := e.AnnounceGateway(g.Links["create-service"], ServiceName, []gatewayv1a2.Listener{Listener80})
	if err != nil {
		t.Fatal("announcing service", err)
	}
	assert.Equal(t, svc.Links["group"], GroupURL, "group url mismatch")
}
