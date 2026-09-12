package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// generate 256-bit random secret
func generateWebhookSecret() (string, error) {
	b := make([]byte, 32)

	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

func generateWebhookSignature(
	secret string,
	body []byte,
) string {
	mac := hmac.New(
		sha256.New,
		[]byte(secret),
	)

	mac.Write(body)

	return hex.EncodeToString(mac.Sum(nil))
}
