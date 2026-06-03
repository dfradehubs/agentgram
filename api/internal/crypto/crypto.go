package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
)

// EncryptedPrefix is prepended to encrypted values to distinguish them from plaintext.
const EncryptedPrefix = "enc:"

// AESCrypto encrypts and decrypts strings using AES-256-GCM.
type AESCrypto struct {
	aead cipher.AEAD
}

// New creates an AESCrypto from a hex-encoded 32-byte key.
func New(hexKey string) (*AESCrypto, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("decode hex key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}

	return &AESCrypto{aead: aead}, nil
}

// Encrypt encrypts plaintext and returns a prefixed base64url-encoded string.
// The result has the format "enc:<base64url(nonce || ciphertext)>".
func (c *AESCrypto) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	ciphertext := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return EncryptedPrefix + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decodes a prefixed base64url string and decrypts the plaintext.
// If the value does not have the "enc:" prefix, it is returned as-is (plaintext passthrough).
func (c *AESCrypto) Decrypt(encoded string) (string, error) {
	if !IsEncrypted(encoded) {
		return encoded, nil
	}

	raw := encoded[len(EncryptedPrefix):]
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}

	nonceSize := c.aead.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return string(plaintext), nil
}

// IsEncrypted returns true if the value has the encrypted prefix.
func IsEncrypted(value string) bool {
	return len(value) > len(EncryptedPrefix) && value[:len(EncryptedPrefix)] == EncryptedPrefix
}
