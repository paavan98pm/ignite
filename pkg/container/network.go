package container

import (
	"fmt"
	"net"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/vishvananda/netlink"
	"github.com/weaveworks/ignite/pkg/constants"
	"k8s.io/apimachinery/pkg/util/wait"
)

/*
ip r list src 172.17.0.3

ip addr del "$IP" dev eth0

ip link add name br0 type bridge
ip tuntap add dev vm0 mode tap

ip link set br0 up
ip link set vm0 up

ip link set eth0 master br0
ip link set vm0 master br0
*/

// Array of container interfaces to ignore (not forward to vm)
var ignoreInterfaces = map[string]bool{
	"lo": true,
}

func SetupContainerNetworking() ([]DHCPInterface, error) {
	var dhcpIfaces []DHCPInterface
	interval := 1 * time.Second
	timeout := 1 * time.Minute

	err := wait.PollImmediate(interval, timeout, func() (bool, error) {
		// this func returns true if it's done, and optionally an error
		retry, err := networkSetup(&dhcpIfaces)
		if err == nil {
			// we're done here
			return true, nil
		}
		if retry {
			// we got an error, but let's ignore it and try again
			log.Infof("Got an error while trying to set up networking, but retrying: %v", err)
			return false, nil
		}
		// the error was fatal, return it
		return false, err
	})

	if err != nil {
		return nil, err
	}

	return dhcpIfaces, nil
}

func networkSetup(dhcpIfaces *[]DHCPInterface) (bool, error) {
	ifaces, err := net.Interfaces()
	if err != nil || ifaces == nil || len(ifaces) == 0 {
		return true, fmt.Errorf("cannot get local network interfaces: %v", err)
	}

	// interfacesCount counts the interfaces that are relevant to Ignite (in other words, not ignored)
	interfacesCount := 0
	for _, iface := range ifaces {
		// Skip the interface if it's ignored
		if ignoreInterfaces[iface.Name] {
			continue
		}

		// Try to transfer the address from the container to the DHCP server
		ipNet, _, err := takeAddress(&iface)
		if err != nil {
			// Log the problem, but don't quit the function here as there might be other good interfaces
			log.Errorf("Parsing interface %s failed: %v", iface.Name, err)
			// Try with the next interface
			continue
		}

		// Bridge the Firecracker TAP interface with the container veth interface
		dhcpIface, err := bridge(&iface)
		if err != nil {
			// Log the problem, but don't quit the function here as there might be other good interfaces
			// Don't set shouldRetry here as there is no point really with retrying with this interface
			// that seems broken/unsupported in some way.
			log.Errorf("Bridging interface %s failed: %v", iface.Name, err)
			// Try with the next interface
			continue
		}

		// Gateway for now is just x.x.x.1 TODO: Better detection
		dhcpIface.GatewayIP = &net.IP{ipNet.IP[0], ipNet.IP[1], ipNet.IP[2], 1}
		dhcpIface.VMIPNet = ipNet

		*dhcpIfaces = append(*dhcpIfaces, *dhcpIface)

		// This is an interface we care about
		interfacesCount++
	}

	// If there weren't any interfaces that were valid or active yet, retry the loop
	if interfacesCount == 0 {
		return true, fmt.Errorf("no active or valid interfaces available yet")
	}

	return false, nil
}

// bridge creates the TAP device and performs the bridging, returning the MAC address of the vm's adapter
func bridge(iface *net.Interface) (*DHCPInterface, error) {
	tapName := constants.TAP_PREFIX + iface.Name
	bridgeName := constants.BRIDGE_PREFIX + iface.Name

	handle, err := netlink.NewHandle()
	if err != nil {
		return nil, err
	}

	tuntap, err := createTAPAdapter(handle, tapName)
	if err != nil {
		return nil, err
	}

	bridge, err := createBridge(handle, bridgeName)
	if err != nil {
		return nil, err
	}

	if err = handle.LinkSetMaster(tuntap, bridge); err != nil {
		return nil, err
	}

	link, err := netlink.LinkByName(iface.Name)
	if err != nil {
		return nil, err
	}

	if err = handle.LinkSetMaster(link, bridge); err != nil {
		return nil, err
	}

	return &DHCPInterface{
		VMTAP:  tapName,
		Bridge: bridgeName,
	}, nil
}

// takeAddress removes the first address of an interface and returns it
func takeAddress(iface *net.Interface) (*net.IPNet, bool, error) {
	addrs, err := iface.Addrs()
	if err != nil || addrs == nil || len(addrs) == 0 {
		// set the bool to true so the caller knows to retry
		return nil, true, fmt.Errorf("interface %s has no address", iface.Name)
	}

	handle, err := netlink.NewHandle()
	if err != nil {
		return nil, false, errors.Wrapf(err, "failed to acquire handle on network namespace")
	}

	for _, addr := range addrs {
		var ip net.IP
		var mask net.IPMask

		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
			mask = v.Mask
		case *net.IPAddr:
			ip = v.IP
			mask = ip.DefaultMask()
		}

		if ip == nil {
			continue
		}

		ip = ip.To4()
		if ip == nil {
			continue
		}

		link, err := netlink.LinkByName(iface.Name)
		if err != nil {
			return nil, false, errors.Wrapf(err, "failed to get interface by name %s", iface.Name)
		}

		delAddr, err := netlink.ParseAddr(addr.String())
		if err != nil {
			return nil, false, errors.Wrapf(err, "failed to parse address from stringified ip %s", addr.String())
		}

		if err = handle.AddrDel(link, delAddr); err != nil {
			return nil, false, errors.Wrapf(err, "failed to remove address from interface %s", iface.Name)
		}

		log.Infof("Moving IP address %s (%s) from container to VM\n", ip.String(), fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3]))

		return &net.IPNet{
			IP:   ip,
			Mask: mask,
		}, false, nil
	}

	return nil, false, fmt.Errorf("interface %s has no valid addresses", iface.Name)
}

func createTAPAdapter(handle *netlink.Handle, tapName string) (*netlink.Tuntap, error) {
	la := netlink.NewLinkAttrs()
	la.Name = tapName
	tuntap := &netlink.Tuntap{
		LinkAttrs: la,
		Mode:      netlink.TUNTAP_MODE_TAP,
	}
	if err := netlink.LinkAdd(tuntap); err != nil {
		return nil, err
	}
	if err := netlink.LinkSetUp(tuntap); err != nil {
		return nil, err
	}
	return tuntap, nil
}

func createBridge(handle *netlink.Handle, bridgeName string) (*netlink.Bridge, error) {
	la := netlink.NewLinkAttrs()
	la.Name = bridgeName
	bridge := &netlink.Bridge{LinkAttrs: la}
	if err := netlink.LinkAdd(bridge); err != nil {
		return nil, err
	}
	if err := netlink.LinkSetUp(bridge); err != nil {
		return nil, err
	}
	return bridge, nil
}
