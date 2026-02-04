package clients

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// StorageCrypto handles encryption and decryption of storage data
type StorageCrypto struct {
	key []byte
}

// NewStorageCrypto creates a new StorageCrypto instance
func NewStorageCrypto(keyString string) (*StorageCrypto, error) {
	if keyString == "" {
		return nil, nil // No encryption if key is empty
	}

	// Key must be 16, 24, or 32 bytes for AES-128, AES-192, or AES-256
	// We'll hash the key if it's not the right length, or just require the user to provide a valid key.
	// For simplicity/robustness, let's decode from base64 if it looks like base64, or use as is.
	// Actually, best practice is to require a 32-byte key (AES-256).
	// Let's support flexible input by hashing it if needed, or better yet, just expect a string of sufficient length.
	// For this implementation, let's just use the bytes. If length is < 16, throw error.

	key := []byte(keyString)
	if len(key) < 16 {
		return nil, fmt.Errorf("encryption key must be at least 16 characters long")
	}

	// Pad to 32 bytes if needed for AES-256, or truncate
	if len(key) > 32 {
		key = key[:32]
	} else if len(key) < 32 {
		// Pad with zeros (simple approach, better would be KDF)
		padded := make([]byte, 32)
		copy(padded, key)
		key = padded
	}

	return &StorageCrypto{key: key}, nil
}

// Encrypt encrypts data using AES-GCM
func (sc *StorageCrypto) Encrypt(data []byte) ([]byte, error) {
	if sc == nil || len(sc.key) == 0 {
		return data, nil
	}

	block, err := aes.NewCipher(sc.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)

	// Return as base64 encoded string to make it text-file friendly
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(ciphertext)))
	base64.StdEncoding.Encode(encoded, ciphertext)

	return encoded, nil
}

// Decrypt decrypts data using AES-GCM
func (sc *StorageCrypto) Decrypt(data []byte) ([]byte, error) {
	if sc == nil || len(sc.key) == 0 {
		return data, nil
	}

	// Check if data is base64 encoded
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(data)))
	n, err := base64.StdEncoding.Decode(decoded, data)
	if err != nil {
		// If not base64, assume it's unencrypted (migration path)
		return data, nil
	}
	ciphertext := decoded[:n]

	block, err := aes.NewCipher(sc.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
