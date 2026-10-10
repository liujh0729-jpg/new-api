package common

import (
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
	plaintext, err := DecryptSensitiveValue(first)
	require.NoError(t, err)
	assert.Equal(t, "merchant-private-key", plaintext)
}

func TestSensitiveValueEncryptionRejectsTamperingAndWrongSecret(t *testing.T) {
	useCryptoSecretForTest(t, "0123456789abcdef0123456789abcdef", true)

	encrypted, err := EncryptSensitiveValue("api-v3-key")
	require.NoError(t, err)

	last := encrypted[len(encrypted)-1]
	replacement := byte('A')
	if last == replacement {
		replacement = 'B'
	}
	_, err = DecryptSensitiveValue(encrypted[:len(encrypted)-1] + string(replacement))
	require.Error(t, err)

	CryptoSecret = "fedcba9876543210fedcba9876543210"
	_, err = DecryptSensitiveValue(encrypted)
	require.Error(t, err)
}
