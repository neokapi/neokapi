package crypto

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKey is a valid base64-encoded 32-byte key.
var testKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher(testKey)
	require.NoError(t, err)
	require.True(t, c.Enabled())

	plaintext := `{"username":"editor","password":"hunter2"}`
	sealed, err := c.Seal(plaintext)
	require.NoError(t, err)
	assert.Contains(t, sealed, sealPrefix)
	assert.NotContains(t, sealed, "hunter2", "ciphertext must not leak the secret")

	// Two seals of the same plaintext differ (random nonce) but both open.
	sealed2, err := c.Seal(plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, sealed, sealed2)

	opened, err := c.Open(sealed)
	require.NoError(t, err)
	assert.Equal(t, plaintext, opened)
}

func TestNilCipherIsPassThrough(t *testing.T) {
	var c *Cipher // empty key => NewCipher returns nil
	assert.False(t, c.Enabled())

	s, err := c.Seal("plain")
	require.NoError(t, err)
	assert.Equal(t, "plain", s)

	o, err := c.Open("plain")
	require.NoError(t, err)
	assert.Equal(t, "plain", o)
}

func TestOpenLegacyPlaintext(t *testing.T) {
	c, err := NewCipher(testKey)
	require.NoError(t, err)
	// A pre-encryption row has no seal prefix and must read back unchanged.
	legacy := `{"url":"https://wp.example.com"}`
	o, err := c.Open(legacy)
	require.NoError(t, err)
	assert.Equal(t, legacy, o)
}

func TestOpenSealedWithoutKeyErrors(t *testing.T) {
	c, err := NewCipher(testKey)
	require.NoError(t, err)
	sealed, err := c.Seal("secret")
	require.NoError(t, err)

	var noKey *Cipher
	_, err = noKey.Open(sealed)
	require.Error(t, err, "a sealed value must not silently pass through without a key")
}

func TestTamperIsDetected(t *testing.T) {
	c, err := NewCipher(testKey)
	require.NoError(t, err)
	sealed, err := c.Seal("secret")
	require.NoError(t, err)

	tampered := sealed[:len(sealed)-3] + "AAA"
	_, err = c.Open(tampered)
	require.Error(t, err)
}

func TestNewCipherKeyValidation(t *testing.T) {
	// Empty key => no encryption (nil cipher, no error).
	c, err := NewCipher("")
	require.NoError(t, err)
	assert.Nil(t, c)

	// Not base64.
	_, err = NewCipher("not base64 !!!")
	require.Error(t, err)

	// Valid base64 but wrong length.
	_, err = NewCipher(base64.StdEncoding.EncodeToString([]byte("too-short")))
	require.Error(t, err)
}
