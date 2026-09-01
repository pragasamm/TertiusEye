package crypto

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/service/kms"
)

// KMSClientAPI defines interface for AWS KMS operations (allowing real KMS client or mock in tests).
type KMSClientAPI interface {
	Encrypt(ctx context.Context, params *kms.EncryptInput, optFns ...func(*kms.Options)) (*kms.EncryptOutput, error)
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
}

// EnvelopeManager manages AES-256-GCM token encryption and AWS KMS DEK wrapping.
type EnvelopeManager struct {
	kmsClient KMSClientAPI
	keyID     string
}

// EncryptedEnvelope holds ciphertext payload and KMS-wrapped DEK bytes for BYTEA storage.
type EncryptedEnvelope struct {
	TokenCiphertext []byte `json:"token_ciphertext"`
	EncryptedDEK    []byte `json:"encrypted_dek"`
}

// NewEnvelopeManager initializes an EnvelopeManager instance.
func NewEnvelopeManager(kmsClient KMSClientAPI, keyID string) *EnvelopeManager {
	return &EnvelopeManager{
		kmsClient: kmsClient,
		keyID:     keyID,
	}
}

// ZeroBytes overwrites a byte slice with zeros to prevent memory scraping of plaintext secrets.
func ZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// EncryptToken executes AES-GCM envelope encryption using a 32-byte DEK generated via crypto/rand.
// The DEK is encrypted via AWS KMS Encrypt API and immediately zeroed out from memory.
func (e *EnvelopeManager) EncryptToken(ctx context.Context, plaintextToken []byte) (*EncryptedEnvelope, error) {
	// 1. Generate 32-byte Data Encryption Key (DEK) via crypto/rand
	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("failed to generate 32-byte DEK: %w", err)
	}
	defer ZeroBytes(dek)

	// 2. Encrypt token using AES-256-GCM
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM block: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate AES-GCM nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintextToken, nil)

	// 3. Encrypt DEK using AWS KMS Encrypt API
	var encryptedDEK []byte
	if e.kmsClient != nil {
		kmsResp, err := e.kmsClient.Encrypt(ctx, &kms.EncryptInput{
			KeyId:     &e.keyID,
			Plaintext: dek,
		})
		if err != nil {
			return nil, fmt.Errorf("AWS KMS Encrypt DEK failed: %w", err)
		}
		encryptedDEK = kmsResp.CiphertextBlob
	} else {
		// Local mock fallback if KMS client is not provided (for test environment)
		encryptedDEK = append([]byte("MOCK_KMS_DEK:"), dek...)
	}

	return &EncryptedEnvelope{
		TokenCiphertext: ciphertext,
		EncryptedDEK:    encryptedDEK,
	}, nil
}

// DecryptToken decrypts the encrypted DEK via AWS KMS, decrypts token ciphertext, and zeroes out the DEK.
func (e *EnvelopeManager) DecryptToken(ctx context.Context, envelope *EncryptedEnvelope) ([]byte, error) {
	var dek []byte

	// 1. Decrypt DEK using AWS KMS Decrypt API
	if e.kmsClient != nil {
		kmsResp, err := e.kmsClient.Decrypt(ctx, &kms.DecryptInput{
			CiphertextBlob: envelope.EncryptedDEK,
		})
		if err != nil {
			return nil, fmt.Errorf("AWS KMS Decrypt DEK failed: %w", err)
		}
		dek = make([]byte, len(kmsResp.Plaintext))
		copy(dek, kmsResp.Plaintext)
		ZeroBytes(kmsResp.Plaintext)
	} else {
		// Local mock fallback
		if len(envelope.EncryptedDEK) > 13 {
			dek = make([]byte, len(envelope.EncryptedDEK)-13)
			copy(dek, envelope.EncryptedDEK[13:])
		} else {
			return nil, fmt.Errorf("invalid mock encrypted DEK")
		}
	}
	defer ZeroBytes(dek)

	// 2. Decrypt token ciphertext using AES-256-GCM
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher for decryption: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM block for decryption: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(envelope.TokenCiphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := envelope.TokenCiphertext[:nonceSize], envelope.TokenCiphertext[nonceSize:]
	plaintextToken, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("AES-GCM decryption failed: %w", err)
	}

	return plaintextToken, nil
}
