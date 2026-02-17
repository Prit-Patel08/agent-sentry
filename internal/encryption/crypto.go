package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

const KeyFile = "sentry.key"

var masterKey []byte

// Init loads or generates the master key.
func Init() error {
	// 1. Check Env Var
	envKey := os.Getenv("SENTRY_MASTER_KEY")
	if envKey != "" {
		key, err := hex.DecodeString(envKey)
		if err != nil {
			return fmt.Errorf("invalid SENTRY_MASTER_KEY hex: %v", err)
		}
		if len(key) != 32 {
			return fmt.Errorf("SENTRY_MASTER_KEY must be 32 bytes (64 hex chars)")
		}
		masterKey = key
		return nil
	}

	// 2. Check File
	data, err := os.ReadFile(KeyFile)
	if err == nil {
		// Found file
		// Check if it's hex encoded (64 chars) or raw bytes (32 bytes)
		// For consistency, we assume hex encoded as per WriteFile below
		if len(data) == 64 {
			key, err := hex.DecodeString(string(data))
			if err != nil {
				return fmt.Errorf("corrupt key file %s (hex): %v", KeyFile, err)
			}
			masterKey = key
			return nil
		}
		// Fallback for raw if needed? No, let's stick to hex.
	}

	// 3. Generate New
	newKey := make([]byte, 32)
	if _, err := rand.Read(newKey); err != nil {
		return err
	}
	masterKey = newKey

	// Save to file (0600 permissions)
	hexKey := hex.EncodeToString(newKey)
	if err := os.WriteFile(KeyFile, []byte(hexKey), 0600); err != nil {
		return fmt.Errorf("failed to save key file: %v", err)
	}

	fmt.Printf("[Sentry] 🔐 Generated new master key: %s\n", KeyFile)
	return nil
}

// Encrypt encrypts plain text using AES-GCM.
// Returns hex encoded string: nonce + ciphertext + tag.
func Encrypt(plaintext string) (string, error) {
	if masterKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
		// Double check if Init failed to set masterKey
		if masterKey == nil {
			return "", fmt.Errorf("encryption key not initialized")
		}
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(ciphertext), nil
}

// Decrypt decrypts hex encoded string.
func Decrypt(encryptedHex string) (string, error) {
	if masterKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}

	data, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
