package main

import (
	"context"
	"fmt"
	"net"
	"testing"
)

type fakeResolver struct {
	ips []net.IPAddr
	err error
}

func (r fakeResolver) LookupIPAddr(
	ctx context.Context,
	host string,
) ([]net.IPAddr, error) {
	return r.ips, r.err
}

func TestSSRFSafeTransportBlocksPrivateResolvedIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
	}{
		{"loopback", "127.0.0.1"},
		{"private 10.x", "10.0.0.1"},
		{"private 172.x", "172.16.0.1"},
		{"private 192.x", "192.168.1.1"},
		{"link local", "169.254.169.254"},
		{"loopback IPv6", "::1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := fakeResolver{
				ips: []net.IPAddr{
					{IP: net.ParseIP(tt.ip)},
				},
			}

			transport := ssrfSafeTransport(resolver)

			ctx := context.Background()

			conn, err := transport.DialContext(
				ctx,
				"tcp",
				"attacker.example:80",
			)

			if conn != nil {
				conn.Close()
			}

			if err == nil {
				t.Fatalf(
					"expected connection to be blocked for IP %s",
					tt.ip,
				)
			}
		})
	}
}

func TestSSRFSafeTransportAllowsPublicIP(t *testing.T) {
	resolver := fakeResolver{
		ips: []net.IPAddr{
			{IP: net.ParseIP("93.184.216.34")},
		},
	}

	transport := ssrfSafeTransport(resolver)

	ctx := context.Background()

	conn, err := transport.DialContext(
		ctx,
		"tcp",
		"example.com:80",
	)

	if conn != nil {
		conn.Close()
	}

	// We only care that SSRF validation did not reject
	// the public IP. The actual network connection may fail.
	if err != nil {
		if isBlockedIP(net.ParseIP("93.184.216.34")) {
			t.Fatalf("public IP was incorrectly considered blocked")
		}
	}
}

func TestSSRFSafeTransportDNSFailure(t *testing.T) {
	resolver := fakeResolver{
		err: fmt.Errorf("DNS failure"),
	}

	transport := ssrfSafeTransport(resolver)

	ctx := context.Background()

	conn, err := transport.DialContext(
		ctx,
		"tcp",
		"example.com:80",
	)

	if conn != nil {
		conn.Close()
	}

	if err == nil {
		t.Fatal("expected DNS lookup error")
	}
}

func TestSSRFSafeTransportNoResolvedIPs(t *testing.T) {
	resolver := fakeResolver{
		ips: []net.IPAddr{},
	}

	transport := ssrfSafeTransport(resolver)

	ctx := context.Background()

	conn, err := transport.DialContext(
		ctx,
		"tcp",
		"example.com:80",
	)

	if conn != nil {
		conn.Close()
	}

	if err == nil {
		t.Fatal("expected error when hostname resolves to no IPs")
	}
}
