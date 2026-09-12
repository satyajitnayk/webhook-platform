package main

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

func validateWebhookURL(rawURL string) error {
	if rawURL == "" {
		return errors.New("url is required")
	}

	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return errors.New("invalid URL")
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("URL must use http or https")
	}

	if u.Host == "" {
		return errors.New("URL must have a host")
	}

	hostname := strings.ToLower(u.Hostname())

	if hostname == "localhost" {
		return errors.New("localhost is not allowed")
	}

	// If the hostname is already an IP address,
	// reject private/reserved addresses.
	if ip := net.ParseIP(hostname); ip != nil && isBlockedIP(ip) {
		return errors.New("private or reserved IP address is not allowed")
	}

	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsUnspecified()
}

func hasDuplicateEvents(events []string) bool {
	seen := make(map[string]struct{})

	for _, event := range events {
		if _, exists := seen[event]; exists {
			return true
		}

		seen[event] = struct{}{}
	}

	return false
}
