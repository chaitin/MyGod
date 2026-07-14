package publicnet

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
)

type resolverFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f resolverFunc) LookupNetIP(ctx context.Context, network string, host string) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestIsPublicAddr(t *testing.T) {
	tests := []struct {
		address string
		want    bool
	}{
		{address: "8.8.8.8", want: true},
		{address: "1.1.1.1", want: true},
		{address: "2606:4700:4700::1111", want: true},
		{address: "127.0.0.1"},
		{address: "10.0.0.1"},
		{address: "100.64.0.1"},
		{address: "169.254.169.254"},
		{address: "172.16.0.1"},
		{address: "192.168.0.1"},
		{address: "192.0.2.1"},
		{address: "198.18.0.1"},
		{address: "203.0.113.1"},
		{address: "0.0.0.0"},
		{address: "::1"},
		{address: "fc00::1"},
		{address: "fe80::1"},
		{address: "2001:db8::1"},
		{address: "::ffff:127.0.0.1"},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if got := IsPublicAddr(netip.MustParseAddr(test.address)); got != test.want {
				t.Fatalf("IsPublicAddr(%s) = %v, want %v", test.address, got, test.want)
			}
		})
	}
}

func TestGuardRejectsMixedPublicAndPrivateDNSAnswers(t *testing.T) {
	guard := &Guard{
		resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{
				netip.MustParseAddr("93.184.216.34"),
				netip.MustParseAddr("10.0.0.1"),
			}, nil
		}),
	}
	if err := guard.ValidateHost(context.Background(), "example.com"); err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("ValidateHost() error = %v, want non-public rejection", err)
	}
}

func TestGuardRejectsResolverFailureAndEmptyAnswers(t *testing.T) {
	for _, test := range []struct {
		name     string
		resolver Resolver
	}{
		{
			name: "failure",
			resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
				return nil, errors.New("lookup failed")
			}),
		},
		{
			name: "empty",
			resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
				return nil, nil
			}),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			guard := &Guard{resolver: test.resolver}
			if err := guard.ValidateHost(context.Background(), "example.com"); err == nil {
				t.Fatal("ValidateHost() error = nil")
			}
		})
	}
}

func TestGuardDialContextPinsValidatedAddress(t *testing.T) {
	var dialedAddress string
	guard := &Guard{
		resolver: resolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
		}),
		dial: func(_ context.Context, network string, address string) (net.Conn, error) {
			if network != "tcp" {
				t.Fatalf("network = %q, want tcp", network)
			}
			dialedAddress = address
			client, server := net.Pipe()
			_ = server.Close()
			return client, nil
		},
	}

	conn, err := guard.DialContext(context.Background(), "tcp", "example.com:443")
	if err != nil {
		t.Fatalf("DialContext() error = %v", err)
	}
	_ = conn.Close()
	if dialedAddress != "93.184.216.34:443" {
		t.Fatalf("dialed address = %q, want pinned public IP", dialedAddress)
	}
}

func TestGuardDialContextRejectsPrivateLiteral(t *testing.T) {
	guard := NewGuard()
	if _, err := guard.DialContext(context.Background(), "tcp", "127.0.0.1:80"); err == nil {
		t.Fatal("DialContext() error = nil")
	}
}
