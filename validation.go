package main

import (
	"errors"
	"net/url"
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

	return nil
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
