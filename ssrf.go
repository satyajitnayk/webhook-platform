package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

type IPResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type netResolver struct{}

func (netResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

func ssrfSafeTransport(resolver IPResolver) *http.Transport {
	return &http.Transport{
		// Do not use HTTP_PROXY/HTTPS_PROXY.
		// Otherwise the request could be routed through a proxy
		// and bypass our destination IP validation.
		Proxy: nil,

		DialContext: func(
			ctx context.Context,
			network string,
			address string,
		) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, fmt.Errorf("invalid address: %w", err)
			}

			ips, err := resolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("DNS lookup failed: %w", err)
			}

			if len(ips) == 0 {
				return nil, fmt.Errorf("hostname resolved to no IP addresses")
			}

			// Check ALL resolved IPs.
			// If any address is private/reserved, reject the request.
			for _, ip := range ips {
				if isBlockedIP(ip.IP) {
					return nil, fmt.Errorf(
						"connection to private or reserved IP %s is blocked",
						ip.IP,
					)
				}
			}

			dialer := &net.Dialer{
				Timeout: 5 * time.Second,
			}

			// Connect to the IP we just validated rather than
			// resolving the hostname again.
			targetIP := ips[0].IP.String()

			return dialer.DialContext(
				ctx,
				network,
				net.JoinHostPort(targetIP, port),
			)
		},
	}
}
