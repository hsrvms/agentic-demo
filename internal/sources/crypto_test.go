package sources

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAESGCMService_Roundtrip(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	svc, err := NewAESGCMService(key)
	require.NoError(t, err)

	plaintext := []byte(`{"api_key": "secret-value", "endpoint": "https://api.example.com"}`)

	ciphertext, err := svc.Encrypt(plaintext)
	require.NoError(t, err)
	require.NotEmpty(t, ciphertext)
	require.NotEqual(t, plaintext, ciphertext, "ciphertext must differ from plaintext")

	decrypted, err := svc.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, plaintext, decrypted)
}

func TestAESGCMService_EncryptNil(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	svc, err := NewAESGCMService(key)
	require.NoError(t, err)

	ciphertext, err := svc.Encrypt(nil)
	require.NoError(t, err)
	require.Nil(t, ciphertext)

	ciphertext, err = svc.Encrypt([]byte{})
	require.NoError(t, err)
	require.Nil(t, ciphertext)
}

func TestAESGCMService_DecryptNil(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	svc, err := NewAESGCMService(key)
	require.NoError(t, err)

	plaintext, err := svc.Decrypt(nil)
	require.NoError(t, err)
	require.Nil(t, plaintext)

	plaintext, err = svc.Decrypt([]byte{})
	require.NoError(t, err)
	require.Nil(t, plaintext)
}

func TestAESGCMService_InvalidKey(t *testing.T) {
	_, err := NewAESGCMService([]byte("too-short"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be 32 bytes")
}

func TestAESGCMService_TamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	svc, err := NewAESGCMService(key)
	require.NoError(t, err)

	ciphertext, err := svc.Encrypt([]byte("sensitive data"))
	require.NoError(t, err)

	// Tamper with the ciphertext.
	tampered := make([]byte, len(ciphertext))
	copy(tampered, ciphertext)
	tampered[len(tampered)-1] ^= 0xFF

	_, err = svc.Decrypt(tampered)
	require.Error(t, err)
}

func TestAESGCMService_DifferentKeyFails(t *testing.T) {
	key1 := make([]byte, 32)
	_, err := rand.Read(key1)
	require.NoError(t, err)

	key2 := make([]byte, 32)
	_, err = rand.Read(key2)
	require.NoError(t, err)

	svc1, err := NewAESGCMService(key1)
	require.NoError(t, err)

	svc2, err := NewAESGCMService(key2)
	require.NoError(t, err)

	ciphertext, err := svc1.Encrypt([]byte("data"))
	require.NoError(t, err)

	_, err = svc2.Decrypt(ciphertext)
	require.Error(t, err)
}

func TestAESGCMService_DeterministicFailure(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	svc, err := NewAESGCMService(key)
	require.NoError(t, err)

	// Same plaintext should produce different ciphertexts (unique nonce).
	c1, err := svc.Encrypt([]byte("same data"))
	require.NoError(t, err)
	c2, err := svc.Encrypt([]byte("same data"))
	require.NoError(t, err)
	require.NotEqual(t, c1, c2, "encryption must be non-deterministic (unique nonce)")

	// Both should decrypt to the original.
	d1, err := svc.Decrypt(c1)
	require.NoError(t, err)
	d2, err := svc.Decrypt(c2)
	require.NoError(t, err)
	require.Equal(t, []byte("same data"), d1)
	require.Equal(t, []byte("same data"), d2)
}
