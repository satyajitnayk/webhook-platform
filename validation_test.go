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
		{"Valid HTTP URL", "http://localhost:8080/callback", false},
		{"Missing Scheme", "://example.com", true},
		{"Invalid Scheme", "ftp://example.com", true},
		{"Missing Host", "https:///path", true},
		{"Malformed Text", "not-a-url", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWebhookURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
