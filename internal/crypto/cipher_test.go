package crypto_test

import (
	"os"
	"testing"

	"envoytrade/internal/crypto"
)

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	orig := "super_secret_password_123!"
	enc, err := crypto.Encrypt(orig)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if enc == orig {
		t.Fatalf("Ciphertext equals plaintext")
	}

	dec, err := crypto.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if dec != orig {
		t.Fatalf("got %q, want %q", dec, orig)
	}
}

func TestEncrypt_EmptyPlaintext(t *testing.T) {
	enc, err := crypto.Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty string failed: %v", err)
	}
	if enc != "" {
		t.Fatalf("Encrypt empty string want empty string, got %q", enc)
	}

	dec, err := crypto.Decrypt("")
	if err != nil {
		t.Fatalf("Decrypt empty string failed: %v", err)
	}
	if dec != "" {
		t.Fatalf("Decrypt empty string want empty string, got %q", dec)
	}
}

func TestEncrypt_RandomNonces(t *testing.T) {
	orig := "same_text"
	enc1, err := crypto.Encrypt(orig)
	if err != nil {
		t.Fatalf("Encrypt 1 failed: %v", err)
	}
	enc2, err := crypto.Encrypt(orig)
	if err != nil {
		t.Fatalf("Encrypt 2 failed: %v", err)
	}
	if enc1 == enc2 {
		t.Fatalf("Encrypting same plaintext twice should produce different ciphertexts due to random nonces")
	}
}

func TestDecrypt_TamperedCiphertext(t *testing.T) {
	orig := "sensitive-totp-seed"
	enc, err := crypto.Encrypt(orig)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Corrupt first character of ciphertext
	tampered := []byte(enc)
	if tampered[0] == 'A' {
		tampered[0] = 'B'
	} else {
		tampered[0] = 'A'
	}

	_, err = crypto.Decrypt(string(tampered))
	if err == nil {
		t.Fatalf("Decrypt tampered ciphertext should fail, got nil error")
	}
}

func TestEncryptDecrypt_WithCustomEnvKey(t *testing.T) {
	customKey := "01234567890123456789012345678901" // 32 bytes
	os.Setenv("ENCRYPTION_KEY", customKey)
	defer os.Unsetenv("ENCRYPTION_KEY")

	orig := "custom_key_test"
	enc, err := crypto.Encrypt(orig)
	if err != nil {
		t.Fatalf("Encrypt with custom key failed: %v", err)
	}
	dec, err := crypto.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt with custom key failed: %v", err)
	}
	if dec != orig {
		t.Fatalf("got %q, want %q", dec, orig)
	}
}
