package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestGenerateWebhookSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"hello":"world"}`)

	got := generateWebhookSignature(secret, body)

	mac := hmac.New(
		sha256.New,
		[]byte(secret),
	)
	mac.Write(body)

	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal(
		[]byte(got),
		[]byte(expected),
	) {
		t.Fatalf(
			"signature mismatch: got=%s expected=%s",
			got,
			expected,
		)
	}
}

func TestGenerateWebhookSignatureChangesWithBody(t *testing.T) {
	secret := "test-secret"

	sig1 := generateWebhookSignature(
		secret,
		[]byte(`{"id":1}`),
	)

	sig2 := generateWebhookSignature(
		secret,
		[]byte(`{"id":2}`),
	)

	if sig1 == sig2 {
		t.Fatal("expected different signatures for different bodies")
	}
}

func TestGenerateWebhookSignatureChangesWithSecret(t *testing.T) {
	body := []byte(`{"id":1}`)

	sig1 := generateWebhookSignature(
		"secret-1",
		body,
	)

	sig2 := generateWebhookSignature(
		"secret-2",
		body,
	)

	if sig1 == sig2 {
		t.Fatal("expected different signatures for different secrets")
	}
}

func TestGenerateWebhookSecret(t *testing.T) {
	secret1, err := generateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}

	secret2, err := generateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}

	if secret1 == "" || secret2 == "" {
		t.Fatal("expected non-empty secrets")
	}

	if secret1 == secret2 {
		t.Fatal("expected unique secrets")
	}

	if len(secret1) != 64 {
		t.Fatalf(
			"expected 64 hex characters, got %d",
			len(secret1),
		)
	}
}