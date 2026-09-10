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
