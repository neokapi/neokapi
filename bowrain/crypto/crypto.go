// Package crypto provides authenticated encryption for secret values stored at
// rest (e.g. connector credentials). It uses AES-256-GCM under a single server
// master key supplied via configuration.
//
// A nil *Cipher is a valid pass-through: Seal returns its input and Open returns
// non-sealed input unchanged. That makes encryption opt-in (configure a key to
// enable it) and lets pre-existing plaintext rows be read and lazily re-sealed
// on their next write, with no migration step.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// sealPrefix tags a value produced by Seal so Open can distinguish sealed
// blobs from legacy plaintext. The "v1" allows a future scheme change.
const sealPrefix = "enc:v1:"

// Cipher seals and opens secret strings with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher builds a Cipher from a base64-encoded 32-byte key. An empty key
// returns (nil, nil): encryption is not configured (pass-through mode).
func NewCipher(base64Key string) (*Cipher, error) {
	if base64Key == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(base64Key))
	if err != nil {
		return nil, fmt.Errorf("decode secrets key: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("secrets key must be 32 bytes after base64 decode, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Enabled reports whether encryption is active (a non-nil keyed Cipher).
func (c *Cipher) Enabled() bool { return c != nil }

// Seal encrypts plaintext and returns a prefixed, base64-encoded blob. A nil
// Cipher returns plaintext unchanged.
func (c *Cipher) Seal(plaintext string) (string, error) {
	if c == nil {
		return plaintext, nil
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	blob := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return sealPrefix + base64.StdEncoding.EncodeToString(blob), nil
}

// Open reverses Seal. A value without the seal prefix is treated as legacy
// plaintext and returned unchanged (lazy migration). A sealed value requires a
// configured key; otherwise, or on any tamper/decrypt failure, Open errors.
func (c *Cipher) Open(value string) (string, error) {
	if !strings.HasPrefix(value, sealPrefix) {
		return value, nil // legacy plaintext / pass-through
	}
	if c == nil {
		return "", errors.New("value is encrypted but no secrets key is configured")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, sealPrefix))
	if err != nil {
		return "", fmt.Errorf("decode sealed value: %w", err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("sealed value too short")
	}
	nonce, blob := raw[:ns], raw[ns:]
	plaintext, err := c.aead.Open(nil, nonce, blob, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
