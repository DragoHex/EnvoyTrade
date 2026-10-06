// ==============================================================================
// scripts/reencrypt_credentials.go
//
// Standalone utility to rotate or re-encrypt account credentials in PostgreSQL
// when the server's ENCRYPTION_KEY changes or when migrating databases between
// environments with different keys.
//
// WHEN TO USE:
//   - You changed ENCRYPTION_KEY in your .env or /etc/envoytrade/envoytrade.env.
//   - Moving a database dump from dev/staging (where a default dev key was used)
//     to production with a custom ENCRYPTION_KEY.
//   - You see error: "decrypt password: decrypt: cipher: message authentication failed"
//
// USAGE:
//   1. Test with dry-run (no changes to database):
//      go run ./scripts/reencrypt_credentials.go \
//        -old-key "old-key-here" \
//        -new-key "new-key-here" \
//        -db "postgres://envoytrade:password@localhost:5432/envoytrade" \
//        -dry-run
//
//   2. Apply re-encryption transactionally:
//      go run ./scripts/reencrypt_credentials.go \
//        -old-key "old-key-here" \
//        -new-key "new-key-here" \
//        -db "postgres://envoytrade:password@localhost:5432/envoytrade"
//
//   3. Or compile for Linux production deployment:
//      GOOS=linux GOARCH=amd64 go build -o /tmp/reencrypt_credentials ./scripts/reencrypt_credentials.go
//      scp /tmp/reencrypt_credentials user@server:/home/user/
//      ./reencrypt_credentials -old-key "..." -new-key "..." -db "..."
// ==============================================================================
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
)

// decryptWithKey decodes standard Base64-encoded ciphertext and decrypts it with AES-256-GCM
// using a 32-byte key derived from sha256(rawKey).
func decryptWithKey(rawKey, encodedCiphertext string) (string, error) {
	if encodedCiphertext == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	hash := sha256.Sum256([]byte(rawKey))
	block, err := aes.NewCipher(hash[:])
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
		return "", fmt.Errorf("gcm open: %w", err)
	}

	return string(plaintext), nil
}

// encryptWithKey encrypts plaintext with AES-256-GCM using sha256(rawKey) and prepends nonce.
func encryptWithKey(rawKey, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	hash := sha256.Sum256([]byte(rawKey))
	block, err := aes.NewCipher(hash[:])
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

func main() {
	oldKeyFlag := flag.String("old-key", "", "Old ENCRYPTION_KEY used to encrypt credentials")
	newKeyFlag := flag.String("new-key", "", "New ENCRYPTION_KEY to re-encrypt credentials with")
	dbURLFlag := flag.String("db", "", "PostgreSQL DATABASE_URL connection string")
	dryRunFlag := flag.Bool("dry-run", false, "Test decryption and encryption without updating database")
	flag.Parse()

	oldKey := *oldKeyFlag
	newKey := *newKeyFlag
	dbURL := *dbURLFlag

	if oldKey == "" {
		oldKey = os.Getenv("OLD_ENCRYPTION_KEY")
	}
	if newKey == "" {
		newKey = os.Getenv("ENCRYPTION_KEY")
	}
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}

	if oldKey == "" || newKey == "" {
		log.Fatalf("Both old-key and new-key are required (via flags or env vars OLD_ENCRYPTION_KEY and ENCRYPTION_KEY)")
	}
	if dbURL == "" {
		log.Fatalf("DATABASE_URL is required (via -db flag or env var DATABASE_URL)")
	}

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx, "SELECT id, name, encrypted_password, encrypted_totp_secret FROM accounts")
	if err != nil {
		log.Fatalf("Failed to query accounts: %v", err)
	}
	defer rows.Close()

	type accountCreds struct {
		id       string
		name     string
		origPass string
		origTotp string
		newPass  string
		newTotp  string
	}

	var toUpdate []accountCreds

	for rows.Next() {
		var id, name, encPass, encTotp string
		if err := rows.Scan(&id, &name, &encPass, &encTotp); err != nil {
			log.Fatalf("Scan row failed: %v", err)
		}

		var newPass, newTotp string

		if encPass != "" {
			plainPass, err := decryptWithKey(oldKey, encPass)
			if err != nil {
				log.Fatalf("Failed to decrypt password for account %s (%s): %v", name, id, err)
			}
			newPass, err = encryptWithKey(newKey, plainPass)
			if err != nil {
				log.Fatalf("Failed to re-encrypt password for account %s (%s): %v", name, id, err)
			}
		}

		if encTotp != "" {
			plainTotp, err := decryptWithKey(oldKey, encTotp)
			if err != nil {
				log.Fatalf("Failed to decrypt TOTP secret for account %s (%s): %v", name, id, err)
			}
			newTotp, err = encryptWithKey(newKey, plainTotp)
			if err != nil {
				log.Fatalf("Failed to re-encrypt TOTP secret for account %s (%s): %v", name, id, err)
			}
		}

		toUpdate = append(toUpdate, accountCreds{
			id:       id,
			name:     name,
			origPass: encPass,
			origTotp: encTotp,
			newPass:  newPass,
			newTotp:  newTotp,
		})
	}

	if err := rows.Err(); err != nil {
		log.Fatalf("Error iterating rows: %v", err)
	}

	log.Printf("Successfully decrypted credentials for %d account(s)", len(toUpdate))

	if *dryRunFlag {
		log.Println("[DRY RUN] Verification complete. Database was not modified.")
		return
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		log.Fatalf("Failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	for _, item := range toUpdate {
		_, err := tx.Exec(ctx,
			"UPDATE accounts SET encrypted_password = $1, encrypted_totp_secret = $2, updated_at = NOW() WHERE id = $3",
			item.newPass, item.newTotp, item.id,
		)
		if err != nil {
			log.Fatalf("Failed to update account %s (%s): %v", item.name, item.id, err)
		}
		log.Printf("Updated account %s (%s)", item.name, item.id)
	}

	if err := tx.Commit(ctx); err != nil {
		log.Fatalf("Failed to commit transaction: %v", err)
	}

	log.Println("All account credentials successfully re-encrypted in database!")
}
