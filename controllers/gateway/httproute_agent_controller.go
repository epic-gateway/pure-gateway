/*
Copyright 2022 Acnodal.
*/

package gateway

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	gatewayv1a2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	epicgwv1 "epic-gateway.org/puregw/apis/puregw/v1"
	"epic-gateway.org/puregw/controllers"
	"epic-gateway.org/puregw/internal/acnodal"
	"epic-gateway.org/puregw/internal/trueingress"
	ti "epic-gateway.org/puregw/internal/trueingress"
)

// HTTPRouteAgentReconciler reconciles a Gateway object
type HTTPRouteAgentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// SetupWithManager sets up the controller with the Manager.
func (r *HTTPRouteAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gatewayv1a2.HTTPRoute{}).
		Complete(r)
}

//+kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=httproutes/finalizers,verbs=update
//+kubebuilder:rbac:groups=gateway.networking.k8s.io,resources=gatewayclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=puregw.epic-gateway.org,resources=gatewayclassconfigs,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gateway object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *HTTPRouteAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Get the Resource that triggered this request
	route := gatewayv1a2.HTTPRoute{}
	if err := r.Get(ctx, req.NamespacedName, &route); err != nil {
		// Ignore not-found errors, since they can't be fixed by an
		// immediate requeue (we'll need to wait for a new notification),
		// and we can get them on deleted requests.
		return controllers.Done, client.IgnoreNotFound(err)
	}

	if !route.ObjectMeta.DeletionTimestamp.IsZero() {
		// This resource is marked for deletion.
		l.Info("Cleaning up")

		// Remove our finalizer to ensure that we don't block the resource
		// from being deleted.
		return controllers.Done, controllers.RemoveFinalizer(ctx, r.Client, &route, controllers.AgentFinalizerName())
	}

	// Try to get the Gateway to which we refer, and keep trying until
	// we can.
	gw := gatewayv1a2.Gateway{}
	// FIXME: need to handle multiple parents
	if err := parentGW(ctx, r.Client, route.Namespace, route.Spec.ParentRefs[0], &gw); err != nil {
		l.Info("Can't get parent, will retry", "parentRef", route.Spec.ParentRefs[0])
		return controllers.TryAgain, nil
	}

	// See if we're the chosen controller.
	config, err := getEPICConfig(ctx, r.Client, string(gw.Spec.GatewayClassName))
	if err != nil {
		return controllers.Done, err
	}
	if config == nil {
		l.V(1).Info("Not our ControllerName, will ignore")
		return controllers.Done, nil
	}

	// Figure out this node's IPV4 address
	node := corev1.Node{}
	nodeName := types.NamespacedName{Namespace: "", Name: os.Getenv("EPIC_NODE_NAME")}
	if err := r.Get(ctx, nodeName, &node); err != nil {
		return controllers.Done, err
	}

	nodeIPV4 := trueingress.NodeAddress(node, nl.FAMILY_V4)
	if nodeIPV4 == nil {
		return controllers.Done, fmt.Errorf("can't determine node's IPV4 address")
	}

	// Connect to EPIC
	epic, err := controllers.ConnectToEPIC(ctx, r.Client, &config.Namespace, config.Name)
	if err != nil {
		return controllers.Done, err
	}

	l.V(1).Info("Reconciling")

	// The resource is not being deleted, and it's our GWClass, so add
	// our finalizer.
	if err := controllers.AddFinalizer(ctx, r.Client, &route, controllers.AgentFinalizerName()); err != nil {
		return controllers.Done, err
	}

	// Setup tunnels. If EPIC hasn't yet filled in everything that we
	// need we'll back off and try again.
	slices, incomplete, err := routeSlices(ctx, r.Client, &route)
	if err != nil {
		return controllers.Done, err
	}
	if incomplete {
		return controllers.TryAgain, nil
	}
	l.Info("Referenced slices", "slices", slices)
	isEKS, err := eksCluster(ctx, r.Client)
	if err != nil {
		return controllers.Done, err
	}
	incomplete, err = setupTunnels(l, &gw, *config.Spec.TrueIngress, slices, epic, isEKS, nodeIPV4)
	if incomplete {
		return controllers.TryAgain, nil
	}
	if err != nil {
		return controllers.Done, err
	}

	l.V(1).Info("Reconcile complete, will back off and poll for changes")
	return controllers.TryAgainLater, nil
}

// Cleanup removes our finalizer from all of the HTTPRoutes in the
// system.
func (r *HTTPRouteAgentReconciler) Cleanup(l logr.Logger, ctx context.Context) error {
	routeList := gatewayv1a2.HTTPRouteList{}
	if err := r.Client.List(ctx, &routeList); err != nil {
		return err
	}
	for _, route := range routeList.Items {
		if err := controllers.RemoveFinalizer(ctx, r.Client, &route, controllers.AgentFinalizerName()); err != nil {
			l.Error(err, "removing Finalizer")
		}
	}
	return nil
}

func setupTunnels(l logr.Logger, gw *gatewayv1a2.Gateway, spec epicgwv1.TrueIngress, slices []*discoveryv1.EndpointSlice, epic acnodal.EPIC, isEKS bool, nodeIPV4 net.IP) (incomplete bool, err error) {
	// Get the service that owns this endpoint. This endpoint
	// will either re-use an existing tunnel or set up a new one
	// for this node. Tunnels belong to the service.
	svcResponse, err := epic.FetchGateway(gw.Annotations[epicgwv1.EPICLinkAnnotation])
	if err != nil {
		return false, fmt.Errorf("gateway %s not found: %s", gw.Annotations[epicgwv1.EPICLinkAnnotation], err)
	}

	// For each endpoint address on this node, set up a PFC tunnel.
	for _, slice := range slices {
		for _, ep := range slice.Endpoints {
			if ep.NodeName == nil || *ep.NodeName != os.Getenv("EPIC_NODE_NAME") {
				l.Info("DontAnnounceEndpoint", "endpoint-node", *ep.NodeName)
				continue
			}
			for _, address := range ep.Addresses {

				// See if the tunnel has been allocated by EPIC (it might
				// not be yet since it sometimes takes a while to set
				// up). If it's not there then return "incomplete" which
				// will cause a retry.
				myTunnels, exists := svcResponse.Gateway.Spec.TunnelEndpoints[nodeIPV4.String()]
				if !exists {
					l.Info("fetchTunnelConfig", "endpoints", svcResponse.Gateway)

					return true, fmt.Errorf("tunnel config not found for %s", nodeIPV4.String())
				}

				// Now that we've got the service response we have enough
				// info to set up this tunnel.
				for _, myTunnel := range myTunnels.EPICEndpoints {
					err = setupTunnel(l, spec, address, myTunnel, isEKS)
					if err != nil {
						l.Error(err, "SetupPFC")
					}
				}
			}
		}
	}

	return false, nil
}

// setupTunnel sets up the Acnodal PFC components and GUE tunnel to
// communicate with the Acnodal EPIC.
func setupTunnel(l logr.Logger, spec epicgwv1.TrueIngress, clientAddress string, epicEndpoint acnodal.TunnelEndpoint, isEKS bool) error {
	// Determine the interface to which to attach the Encap PFC
	encapIntf, err := interfaceOrDefault(spec.EncapAttachment.Interface, clientAddress)
	if err != nil {
		return err
	}
	ti.SetupNIC(l, encapIntf.Attrs().Name, "encap", spec.EncapAttachment.Direction, spec.EncapAttachment.QID, spec.EncapAttachment.Flags)

	// In a cluster running the Amazon VPC CNI we need to attach the
	// encapper to all of the ENI interfaces, not just the default.
	if isEKS {
		eniInts, err := ti.AmazonENIInterfaces()
		if err != nil {
			return err
		}
		for _, eni := range eniInts {
			// The secondary ENIs need to have the FWD(4) flag set, and we
			// add a "+ENI" suffix to the name to indicate that we changed
			// the user's flags
			ti.SetupNIC(l, eni.Name, "encap", spec.EncapAttachment.Direction, spec.EncapAttachment.QID, spec.EncapAttachment.Flags|ti.ExplicitPacketForward)
		}
	}

	// Determine the interface to which to attach the Decap PFC
	decapIntf, err := interfaceOrDefault(spec.DecapAttachment.Interface, clientAddress)
	if err != nil {
		return err
	}
	ti.SetupNIC(l, decapIntf.Attrs().Name, "decap", spec.DecapAttachment.Direction, spec.DecapAttachment.QID, spec.DecapAttachment.Flags)

	// Determine the IP address to use for this end of the tunnel. It
	// can be any address on the decap interface in the same family as
	// the address on the other end of the tunnel.
	tunnelIP := net.ParseIP(epicEndpoint.Address)
	if tunnelIP == nil {
		return fmt.Errorf("cannot parse %s as an IP address", epicEndpoint.Address)
	}
	addrs, err := netlink.AddrList(decapIntf, ti.AddrFamily(tunnelIP))
	if len(addrs) < 1 {
		return fmt.Errorf("interface %s has no addresses in family %d", decapIntf.Attrs().Name, ti.AddrFamily(tunnelIP))
	}

	// set up the GUE tunnel to the EPIC
	return ti.SetTunnel(l, epicEndpoint.TunnelID, epicEndpoint.Address, addrs[0].IP.String(), epicEndpoint.Port.Port)
}

// interfaceOrDefault returns info about an interface. If intName is
// "default" then the interface will be whichever interface has the
// least-cost default route. Otherwise, it will be the interface whose
// name is "intName". The address family to which "address" belongs is
// used to determine the default interface.
//
// If the error returned is non-nil then the netlink.Link is
// undefined.
func interfaceOrDefault(intName string, address string) (netlink.Link, error) {
	if intName == "default" {
		// figure out which interface is the default
		return ti.DefaultInterface(ti.AddrFamily(net.ParseIP(address)))
	}

	return netlink.LinkByName(intName)
}

// providerID returns this cluster's cloud provider ID.
func providerID(ctx context.Context, cl client.Client) (string, error) {
	node := corev1.Node{}
	if err := cl.Get(ctx, types.NamespacedName{Namespace: "", Name: os.Getenv("EPIC_NODE_NAME")}, &node); err != nil {
		return "", err
	}
	return node.Spec.ProviderID, nil
}

// eksCluster returns true if this is an AWS EKS cluster, based on
// this node's spec.providerID.
func eksCluster(ctx context.Context, cl client.Client) (bool, error) {
	providerID, err := providerID(ctx, cl)
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(providerID, "aws:"), nil
}
