package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigLoadAndValidation(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	content := `{
		"tenant_id": "tenant-abc-123",
		"device_uuid": "device-uuid-999",
		"cert_path": "cert.pem",
		"key_path": "key.pem",
		"scan_interval": "10s",
		"max_jitter": "2s",
		"software_scan_paths": ["/tmp"]
	}`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.TenantID != "tenant-abc-123" {
		t.Errorf("Expected TenantID 'tenant-abc-123', got '%s'", cfg.TenantID)
	}
	if cfg.DeviceUUID != "device-uuid-999" {
		t.Errorf("Expected DeviceUUID 'device-uuid-999', got '%s'", cfg.DeviceUUID)
	}
	if cfg.ScanInterval != 10*time.Second {
		t.Errorf("Expected ScanInterval 10s, got %v", cfg.ScanInterval)
	}
	if cfg.MaxJitter != 2*time.Second {
		t.Errorf("Expected MaxJitter 2s, got %v", cfg.MaxJitter)
	}
}

func TestConfigMissingRequiredFields(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.json")

	content := `{
		"tenant_id": "",
		"device_uuid": "device-uuid-999"
	}`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Error("Expected LoadConfig to fail for missing tenant_id, but it succeeded")
	}
}

func TestLoadTLSKeyPair(t *testing.T) {
	tempDir := t.TempDir()
	certPath, keyPath, err := generateTestCertKey(tempDir)
	if err != nil {
		t.Fatalf("Failed to generate test cert keypair: %v", err)
	}

	cfg := &Config{
		CertPath: certPath,
		KeyPath:  keyPath,
	}

	cert, err := cfg.LoadTLSKeyPair()
	if err != nil {
		t.Fatalf("LoadTLSKeyPair failed: %v", err)
	}

	if len(cert.Certificate) == 0 {
		t.Error("Expected non-empty X.509 certificate payload")
	}
}

func generateTestCertKey(dir string) (string, string, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"TertiusEye Test"},
		},
		NotBefore: time.Now(),
		NotAfter:  time.Now().Add(1 * time.Hour),
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	certPath := filepath.Join(dir, "cert.pem")
	certFile, err := os.Create(certPath)
	if err != nil {
		return "", "", err
	}
	defer certFile.Close()
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	keyPath := filepath.Join(dir, "key.pem")
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return "", "", err
	}
	defer keyFile.Close()
	pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certPath, keyPath, nil
}
