package common

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useCryptoSecretForTest(t *testing.T, secret string, configured bool) {
	t.Helper()
	previousSecret := CryptoSecret
	previousConfigured := CryptoSecretConfigured
	CryptoSecret = secret
	CryptoSecretConfigured = configured
	t.Cleanup(func() {
		CryptoSecret = previousSecret
		CryptoSecretConfigured = previousConfigured
	})
}

func TestSensitiveValueEncryptionRequiresExplicitStableSecret(t *testing.T) {
	useCryptoSecretForTest(t, strings.Repeat("a", 32), false)

	assert.False(t, HasStableCryptoSecret())
	_, err := EncryptSensitiveValue("secret")
	require.ErrorIs(t, err, ErrCryptoSecretNotConfigured)
}

func TestSensitiveValueEncryptionRoundTripAndRandomNonce(t *testing.T) {
	useCryptoSecretForTest(t, "0123456789abcdef0123456789abcdef", true)

	first, err := EncryptSensitiveValue("merchant-private-key")
	require.NoError(t, err)
	second, err := EncryptSensitiveValue("merchant-private-key")
	require.NoError(t, err)

	assert.NotEqual(t, first, second)
	assert.True(t, strings.HasPrefix(first, encryptedValuePrefix))
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(first, encryptedValuePrefixV2))
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "merchant-private-key")
	plaintext, err := DecryptSensitiveValue(first)
	require.NoError(t, err)
	assert.Equal(t, "merchant-private-key", plaintext)
}

func TestSensitiveValueEncryptionReadsLegacyV1Ciphertext(t *testing.T) {
	useCryptoSecretForTest(t, "0123456789abcdef0123456789abcdef", true)

	legacy, err := encryptSensitiveValueV1("legacy-private-key")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(legacy, encryptedValuePrefixV1))

	plaintext, err := DecryptSensitiveValue(legacy)
	require.NoError(t, err)
	require.Equal(t, "legacy-private-key", plaintext)
}

func TestSensitiveValueEncryptionRejectsTamperingAndWrongSecret(t *testing.T) {
	useCryptoSecretForTest(t, "0123456789abcdef0123456789abcdef", true)

	encrypted, err := EncryptSensitiveValue("api-v3-key")
	require.NoError(t, err)

	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encrypted, encryptedValuePrefixV2))
	require.NoError(t, err)
	require.NotEmpty(t, payload)
	payload[len(payload)-1] ^= 1
	tampered := encryptedValuePrefixV2 + base64.RawStdEncoding.EncodeToString(payload)
	_, err = DecryptSensitiveValue(tampered)
	require.Error(t, err)

	CryptoSecret = "fedcba9876543210fedcba9876543210"
	_, err = DecryptSensitiveValue(encrypted)
	require.Error(t, err)
}
