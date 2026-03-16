package main

import "testing"

func TestEncryptField(t *testing.T) {
	cfg := &Config{EncryptionKey: "field-test-key"}

	original := "user@example.com"
	enc := encryptField(cfg, original)
	if enc == original {
		t.Error("encryptField should change the value")
	}

	dec := decryptField(cfg, enc)
	if dec != original {
		t.Errorf("decryptField round-trip: got %q, want %q", dec, original)
	}
}

func TestEncryptFieldNoKey(t *testing.T) {
	cfg := &Config{}

	original := "user@example.com"
	enc := encryptField(cfg, original)
	if enc != original {
		t.Errorf("no key should pass through: got %q", enc)
	}

	dec := decryptField(cfg, enc)
	if dec != original {
		t.Errorf("no key should pass through: got %q", dec)
	}
}

func TestResolveEncryptionKey(t *testing.T) {
	// Config-level key takes priority.
	cfg := &Config{
		EncryptionKey: "config-key",
		OAuth:         OAuthConfig{EncryptionKey: "oauth-key"},
	}
	if got := resolveEncryptionKey(cfg); got != "config-key" {
		t.Errorf("should prefer config key: got %q", got)
	}

	// Fallback to OAuth key.
	cfg2 := &Config{
		OAuth: OAuthConfig{EncryptionKey: "oauth-key"},
	}
	if got := resolveEncryptionKey(cfg2); got != "oauth-key" {
		t.Errorf("should fall back to OAuth key: got %q", got)
	}

	// No key at all.
	cfg3 := &Config{}
	if got := resolveEncryptionKey(cfg3); got != "" {
		t.Errorf("should be empty: got %q", got)
	}
}
