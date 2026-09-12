package main

import "testing"

func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
	}{
		{"Empty URL", "", true},

		{"Valid HTTPS URL", "https://example.com", false},
		{"Valid HTTP URL", "http://example.com/callback", false},

		{"Missing Scheme", "://example.com", true},
		{"Invalid Scheme", "ftp://example.com", true},
		{"Missing Host", "https:///path", true},
		{"Malformed Text", "not-a-url", true},

		// SSRF protection
		{"Localhost", "http://localhost:8080/callback", true},
		{"Loopback IPv4", "http://127.0.0.1:8080/callback", true},
		{"Loopback IPv6", "http://[::1]:8080/callback", true},
		{"Private 10.x IP", "http://10.0.0.1/callback", true},
		{"Private 172.16.x IP", "http://172.16.0.1/callback", true},
		{"Private 192.168.x IP", "http://192.168.1.1/callback", true},
		{"Link Local IP", "http://169.254.169.254/callback", true},
		{"Unspecified IPv4", "http://0.0.0.0:8080/callback", true},
		{"Unspecified IPv6", "http://[::]:8080/callback", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.rawURL)

			if (err != nil) != tt.wantErr {
				t.Errorf(
					"validateWebhookURL() error = %v, wantErr %v",
					err,
					tt.wantErr,
				)
			}
		})
	}
}
