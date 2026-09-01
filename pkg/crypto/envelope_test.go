package crypto

import (
	"bytes"
	"context"
	"testing"
)

func TestEnvelopeEncryptionAndDecryption(t *testing.T) {
	manager := NewEnvelopeManager(nil, "arn:aws:kms:us-east-1:123456789012:key/test-key")

	plaintextToken := []byte("secret-oauth-access-token-12345")

	envelope, err := manager.EncryptToken(context.Background(), plaintextToken)
	if err != nil {
		t.Fatalf("EncryptToken failed: %v", err)
	}

	if len(envelope.TokenCiphertext) == 0 {
		t.Error("Expected non-empty token ciphertext")
	}
	if len(envelope.EncryptedDEK) == 0 {
		t.Error("Expected non-empty encrypted DEK")
	}

	decryptedToken, err := manager.DecryptToken(context.Background(), envelope)
	if err != nil {
		t.Fatalf("DecryptToken failed: %v", err)
	}

	if !bytes.Equal(decryptedToken, plaintextToken) {
		t.Errorf("Expected decrypted token '%s', got '%s'", string(plaintextToken), string(decryptedToken))
	}
}

func TestZeroBytes(t *testing.T) {
	secret := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	ZeroBytes(secret)

	for i, v := range secret {
		if v != 0 {
			t.Errorf("Expected byte at index %d to be 0, got %d", i, v)
		}
	}
}
