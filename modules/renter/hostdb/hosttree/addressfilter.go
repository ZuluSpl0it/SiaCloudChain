package hosttree

import (
	"fmt"
	"net"

	"gitlab.com/NebulousLabs/Sia/build"
)

const (
	ipv4FilterRange = 24
	ipv6FilterRange = 54
)

// addressFilter filters host addresses which belong to the same subnet to
// avoid selecting hosts from the same region.
type addressFilter struct {
	filter   map[string]struct{}
	lookupIP func(string) ([]net.IP, error)
	testing  bool // true in a testing build, false otherwise
}

// newAddressFilter creates a new addressFilter object.
func newAddressFilter(lookupIP func(string) ([]net.IP, error)) *addressFilter {
	return &addressFilter{
		filter:   make(map[string]struct{}),
		lookupIP: lookupIP,
		testing:  build.Release == "testing",
	}
}

// Add adds the addresses from a host to the filter preventing addresses from
// the same subnets from being selected.
func (af *addressFilter) Add(host *hostEntry) {
	// Translate the hostname to one or multiple IPs. If the argument is an IP
	// address LookupIP will just return that IP.
	addresses, err := af.lookupIP(host.NetAddress.Host())
	if err != nil {
		return
	}
	// If any of the addresses is blocked we ignore the host.
	for _, ip := range addresses {
		// Set the filterRange according to the type of IP address.
		var filterRange int
		if len(ip) == net.IPv4len {
			filterRange = ipv4FilterRange
		} else {
			filterRange = ipv6FilterRange
		}
		// Get the subnet.
		_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), filterRange))
		if err != nil {
			continue
		}
		// Add the subnet to the map.
		af.filter[ipnet.String()] = struct{}{}
	}
}

// Filtered returns true if an address is supposed to be filtered and therefore
// not selected by the hosttree.
func (af *addressFilter) Filtered(host *hostEntry) bool {
	// During testing we don't filter addresses.
	if af.testing {
		return false
	}
	// Translate the hostname to one or multiple IPs. If the argument is an IP
	// address LookupIP will just return that IP.
	addresses, err := af.lookupIP(host.NetAddress.Host())
	if err != nil {
		return true
	}
	// If the hostname is associated with more than 2 addresses we filter it
	if len(addresses) > 2 {
		return true
	}
	// If the hostname is associated with 2 addresses of the same type, we
	// filter it.
	if (len(addresses) == 2) && (len(addresses[0]) == len(addresses[1])) {
		return true
	}
	// If any of the addresses is blocked we ignore the host.
	for _, ip := range addresses {
		// Set the filterRange according to the type of IP address.
		var filterRange int
		if len(ip) == net.IPv4len {
			filterRange = ipv4FilterRange
		} else {
			filterRange = ipv6FilterRange
		}
		// Get the subnet.
		_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), filterRange))
		if err != nil {
			continue
		}
		// Check if the subnet is in the map. If it is, we filter the host.
		if _, exists := af.filter[ipnet.String()]; exists {
			return true
		}
	}
	return false
}
