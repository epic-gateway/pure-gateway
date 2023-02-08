/*
Copyright 2022 Acnodal.
*/

package trueingress

import (
	"fmt"
	"net"
	"regexp"

	"github.com/go-logr/logr"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netlink/nl"
	corev1 "k8s.io/api/core/v1"
)

const (
	// ENINamePattern identifies AWS ENI interfaces. This pattern
	// doesn't match eth0 because that's the default ENI so it gets set
	// up by default. This pattern catches the other ENI
	// interfaces. Note that non-EKS systems might also have an eth1 so
	// you need to ensure that you're running on an EKS cluster before
	// using this pattern.
	ENINamePattern string = "eth[1-9]"
)

// AddrFamily returns whether lbIP is an IPV4 or IPV6 address.  The
// return value will be nl.FAMILY_V6 if the address is an IPV6
// address, nl.FAMILY_V4 if it's IPV4, or 0 if the family can't be
// determined.
func AddrFamily(lbIP net.IP) (lbIPFamily int) {
	if nil != lbIP.To16() {
		lbIPFamily = nl.FAMILY_V6
	}

	if nil != lbIP.To4() {
		lbIPFamily = nl.FAMILY_V4
	}

	return
}

// defaultInterface finds the default interface (i.e., the one with
// the default route) for the given family, which should be either
// nl.FAMILY_V6 or nl.FAMILY_V4.
func DefaultInterface(family int) (netlink.Link, error) {
	var defaultifindex int = 0
	var defaultifmetric int = 0

	rt, _ := netlink.RouteList(nil, family)
	for _, r := range rt {
		// check each route to see if it's the default (i.e., no destination)
		if r.Dst == nil && defaultifindex == 0 {
			// this is the first default route we've seen
			defaultifindex = r.LinkIndex
			defaultifmetric = r.Priority
		} else if r.Dst == nil && defaultifindex != 0 && r.Priority < defaultifmetric {
			// if there's another default route with a lower metric use it
			defaultifindex = r.LinkIndex
		}
	}

	// If none of our routes matched our criteria then we can't pick an
	// interface
	if defaultifindex == 0 {
		return nil, fmt.Errorf("No default interface can be determined")
	}

	// there's only one default route
	defaultint, err := netlink.LinkByIndex(defaultifindex)
	return defaultint, err
}

// NodeAddress finds the node's first "internal" address in the
// specified family. We used to use the downward API for this but we
// need to be able to pick a specific address family and the downward
// API doesn't allow us to do that. For example, we might need to
// ensure that we use an IPV4 address because at the moment our
// TrueIngress BPF filters only work with IPV4.
//
// If no address in the specified family can be found then the return
// value will be nil.
func NodeAddress(node corev1.Node, lbIPFamily int) net.IP {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			if ip := net.ParseIP(addr.Address); ip != nil && AddrFamily(ip) == lbIPFamily {
				return ip
			}
		}
	}

	return nil
}

// addNetwork adds lbIPNet to link.
func addNetwork(lbIPNet net.IPNet, link netlink.Link) error {
	addr, _ := netlink.ParseAddr(lbIPNet.String())
	err := netlink.AddrReplace(link, addr)
	if err != nil {
		return fmt.Errorf("could not add %v: to %v %w", addr, link, err)
	}
	return nil
}

// addDummyInterface creates a "dummy" interface whose name is
// specified by dummyint.
func addDummyInterface(name string) (netlink.Link, error) {

	// check if there's already an interface with that name
	link, err := netlink.LinkByName(name)
	if err != nil {

		// the interface doesn't exist, so we can add it
		dumint := netlink.NewLinkAttrs()
		dumint.Name = name
		link = &netlink.Dummy{LinkAttrs: dumint}
		if err = netlink.LinkAdd(link); err != nil {
			return nil, fmt.Errorf("failed adding dummy int %s: %w", name, err)
		}

	}
	// Make sure that "dummy" interface is set to up.
	netlink.LinkSetUp(link)
	return link, nil
}

// removeInterface removes link. It returns nil if everything goes
// fine, an error otherwise.
func removeInterface(link netlink.Link) error {
	if err := netlink.LinkDel(link); err != nil {
		return err
	}

	return nil
}

// deleteAddr deletes lbIP from whichever interface has it.
func deleteAddr(lbIP net.IP) error {
	hostints, _ := net.Interfaces()
	for _, hostint := range hostints {
		addrs, _ := hostint.Addrs()
		for _, ipnet := range addrs {

			ipaddr, _, _ := net.ParseCIDR(ipnet.String())

			if lbIP.Equal(ipaddr) {
				ifindex, _ := netlink.LinkByIndex(hostint.Index)
				deladdr, _ := netlink.ParseAddr(ipnet.String())
				err := netlink.AddrDel(ifindex, deladdr)
				if err != nil {
					return fmt.Errorf("could not remove %v from %v: %w", deladdr, ifindex, err)
				}
			}
		}
	}

	return nil
}

func addVirtualInt(lbIP net.IP, link netlink.Link, subnet, aggregation string) error {

	lbIPNet := net.IPNet{IP: lbIP}

	if aggregation == "default" {

		switch AddrFamily(lbIP) {
		case (nl.FAMILY_V4):

			_, poolipnet, _ := net.ParseCIDR(subnet)

			lbIPNet.Mask = poolipnet.Mask

			err := addNetwork(lbIPNet, link)
			if err != nil {
				return fmt.Errorf("could not add %v: to %v %w", lbIPNet, link, err)
			}

		case (nl.FAMILY_V6):

			_, poolipnet, _ := net.ParseCIDR(subnet)

			lbIPNet.Mask = poolipnet.Mask

			err := addNetwork(lbIPNet, link)
			if err != nil {
				return fmt.Errorf("could not add %v: to %v %w", lbIPNet, link, err)
			}
		}

	} else {

		switch AddrFamily(lbIP) {
		case (nl.FAMILY_V4):

			_, poolaggr, _ := net.ParseCIDR("0.0.0.0" + aggregation)

			lbIPNet.Mask = poolaggr.Mask

			err := addNetwork(lbIPNet, link)
			if err != nil {
				return fmt.Errorf("could not add %v: to %v %w", lbIPNet, link, err)
			}

		case (nl.FAMILY_V6):

			_, poolaggr, _ := net.ParseCIDR("::" + aggregation)

			lbIPNet.Mask = poolaggr.Mask

			err := addNetwork(lbIPNet, link)
			if err != nil {
				return fmt.Errorf("could not add %v: to %v %w", lbIPNet, link, err)
			}
		}
	}

	return nil
}

// AmazonENIInterfaces returns a slice containing this node's ENI
// network interfaces.
func AmazonENIInterfaces() ([]net.Interface, error) {
	enis := []net.Interface{}

	// Scan for ENI interfaces.
	regex, err := regexp.Compile(ENINamePattern)
	if err != nil {
		return nil, fmt.Errorf("error compiling regex \"%s\": %s", ENINamePattern, err.Error())
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, intf := range interfaces {
		if regex.Match([]byte(intf.Name)) {
			enis = append(enis, intf)
		}
	}

	return enis, nil
}

// RemoveFilters deletes our filters from the specified interface, the
// default interface, and any ENI interfaces that we can find.  We
// want to ensure that we load the PFC filter programs and
// maps. Filters survive a pod restart, but maps don't, so we delete
// the filters so they'll get reloaded during tunnel setup which will
// implicitly create the maps.
func RemoveFilters(l logr.Logger, encapName string, decapName string) error {
	// Cleanup any explicitly-specified interfaces (i.e., not "default")
	if encapName != "default" {
		unfilterInterface(l, encapName)
	}
	if decapName != "default" {
		unfilterInterface(l, decapName)
	}

	// Clean up the default interfaces, too
	default4, err := DefaultInterface(nl.FAMILY_V4)
	if err == nil {
		unfilterInterface(l, default4.Attrs().Name)
	} else {
		l.Error(err, "Determining local interface")
	}
	default6, err := DefaultInterface(nl.FAMILY_V6)
	if err == nil && default6 != nil {
		unfilterInterface(l, default6.Attrs().Name)
	}

	// Clean up any ENIs, too.
	enis, err := AmazonENIInterfaces()
	if err != nil {
		return err
	}
	for _, eni := range enis {
		unfilterInterface(l, eni.Name)
	}

	return nil
}

// unfilterInterface removes our BPF filters from an interface
func unfilterInterface(l logr.Logger, intfName string) {
	cleanupFilter(l, intfName, "ingress")
	cleanupFilter(l, intfName, "egress")
	cleanupQueueDiscipline(l, intfName)
}
