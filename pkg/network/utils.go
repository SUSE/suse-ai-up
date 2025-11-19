package network

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

// NetworkAddress represents a detected network address with metadata
type NetworkAddress struct {
	IP        string
	Interface string
	IsPublic  bool
	IsLocal   bool
	Priority  int // Lower number = higher priority
}

// GetAvailableAddresses detects all available IP addresses on the machine
func GetAvailableAddresses() ([]NetworkAddress, error) {
	var addresses []NetworkAddress

	// Always include localhost first (highest priority)
	addresses = append(addresses, NetworkAddress{
		IP:        "127.0.0.1",
		Interface: "localhost",
		IsPublic:  false,
		IsLocal:   true,
		Priority:  0,
	})

	addresses = append(addresses, NetworkAddress{
		IP:        "localhost",
		Interface: "localhost",
		IsPublic:  false,
		IsLocal:   true,
		Priority:  1,
	})

	// Get network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return addresses, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			// Focus on IPv4 for now
			if ip.To4() == nil {
				continue
			}

			ipStr := ip.String()

			// Skip localhost (already added)
			if ip.IsLoopback() {
				continue
			}

			// Determine if it's a public IP
			isPublic := isPublicIP(ip)
			isLocal := isPrivateIP(ip)

			priority := 3 // Default priority for private IPs
			if isPublic {
				priority = 2 // Public IPs get higher priority than other private IPs
			}

			addresses = append(addresses, NetworkAddress{
				IP:        ipStr,
				Interface: iface.Name,
				IsPublic:  isPublic,
				IsLocal:   isLocal,
				Priority:  priority,
			})
		}
	}

	// Sort by priority (lower number first)
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].Priority < addresses[j].Priority
	})

	return addresses, nil
}

// GetAvailableHosts returns a list of host strings for server binding
func GetAvailableHosts() ([]string, error) {
	addresses, err := GetAvailableAddresses()
	if err != nil {
		return nil, err
	}

	var hosts []string
	for _, addr := range addresses {
		hosts = append(hosts, addr.IP)
	}

	return hosts, nil
}

// GetPrimaryHost returns the best host to use for server binding
func GetPrimaryHost() (string, error) {
	addresses, err := GetAvailableAddresses()
	if err != nil {
		return "localhost", err
	}

	if len(addresses) > 0 {
		return addresses[0].IP, nil
	}

	return "localhost", nil
}

// isPrivateIP checks if an IP address is in a private range
func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// isPublicIP checks if an IP address is public (not private or loopback)
func isPublicIP(ip net.IP) bool {
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && ip.To4() != nil
}

// FormatHostURL formats a host with port for display
func FormatHostURL(host, port string) string {
	if strings.Contains(host, ":") {
		// IPv6 address
		return fmt.Sprintf("[%s]:%s", host, port)
	}
	return fmt.Sprintf("%s:%s", host, port)
}

// GetServerURLs returns formatted URLs for all available addresses
func GetServerURLs(port string) ([]string, error) {
	addresses, err := GetAvailableAddresses()
	if err != nil {
		return []string{fmt.Sprintf("http://localhost:%s", port)}, err
	}

	var urls []string
	for _, addr := range addresses {
		url := fmt.Sprintf("http://%s", FormatHostURL(addr.IP, port))
		urls = append(urls, url)
	}

	return urls, nil
}
