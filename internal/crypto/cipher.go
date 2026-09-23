package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

// defaultDevKey is used only for development and local testing when ENCRYPTION_KEY is unset.
const defaultDevKey = "envoytrade-master-dev-key-32b!!"

// getKey derives a 32-byte AES-256 key from ENCRYPTION_KEY or defaultDevKey.
func getKey() []byte {
	k := os.Getenv("ENCRYPTION_KEY")
	if k == "" {
		k = defaultDevKey
	}
	hash := sha256.Sum256([]byte(k))
	return hash[:]
}

// Encrypt encrypts plaintext using AES-256-GCM with a random 12-byte nonce
// and returns standard Base64-encoded ciphertext (nonce prepended).
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key := getKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher init: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("nonce generation: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt decodes standard Base64-encoded ciphertext and decrypts it with AES-256-GCM.
func Decrypt(encodedCiphertext string) (string, error) {
	if encodedCiphertext == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	key := getKey()
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("cipher init: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("gcm init: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}
