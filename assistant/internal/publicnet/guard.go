package publicnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"
)

type Resolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type dialContextFunc func(context.Context, string, string) (net.Conn, error)

type Guard struct {
	dial     dialContextFunc
	resolver Resolver
}

func NewGuard() *Guard {
	dialer := &net.Dialer{
		KeepAlive: 30 * time.Second,
		Timeout:   5 * time.Second,
	}
	return &Guard{
		dial:     dialer.DialContext,
		resolver: net.DefaultResolver,
	}
}

func (g *Guard) ValidateHost(ctx context.Context, host string) error {
	_, err := g.resolvePublicAddresses(ctx, host)
	return err
}

func (g *Guard) DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	if g == nil || g.resolver == nil || g.dial == nil {
		return nil, fmt.Errorf("public network guard is not configured")
	}
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("network %q is not allowed", network)
	}

	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("invalid network address: %w", err)
	}
	addresses, err := g.resolvePublicAddresses(ctx, host)
	if err != nil {
		return nil, err
	}

	var dialErrors []error
	for _, addr := range addresses {
		if network == "tcp4" && !addr.Is4() {
			continue
		}
		if network == "tcp6" && !addr.Is6() {
			continue
		}
		conn, dialErr := g.dial(ctx, network, net.JoinHostPort(addr.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		dialErrors = append(dialErrors, dialErr)
	}
	if len(dialErrors) == 0 {
		return nil, fmt.Errorf("host %q has no address for network %s", host, network)
	}
	return nil, fmt.Errorf("connect public host %q: %w", host, errors.Join(dialErrors...))
}

func (g *Guard) resolvePublicAddresses(ctx context.Context, rawHost string) ([]netip.Addr, error) {
	if g == nil || g.resolver == nil {
		return nil, fmt.Errorf("public network guard is not configured")
	}
	host := strings.TrimSpace(rawHost)
	if host == "" {
		return nil, fmt.Errorf("host is required")
	}
	if strings.Contains(host, "%") {
		return nil, fmt.Errorf("IP zone identifiers are not allowed")
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		addr = addr.Unmap()
		if !IsPublicAddr(addr) {
			return nil, fmt.Errorf("host %q resolves to a non-public IP address", host)
		}
		return []netip.Addr{addr}, nil
	}

	addresses, err := g.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve host %q: %w", host, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("host %q did not resolve to an IP address", host)
	}

	result := make([]netip.Addr, 0, len(addresses))
	seen := make(map[netip.Addr]struct{}, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if !IsPublicAddr(address) {
			return nil, fmt.Errorf("host %q resolves to a non-public IP address", host)
		}
		if _, ok := seen[address]; ok {
			continue
		}
		seen[address] = struct{}{}
		result = append(result, address)
	}
	return result, nil
}

func IsPublicAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() || addr.IsUnspecified() {
		return false
	}

	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(addr) {
			return false
		}
	}
	if addr.Is6() && !publicIPv6Prefix.Contains(addr) {
		return false
	}
	return true
}

var publicIPv6Prefix = netip.MustParsePrefix("2000::/3")

var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}
