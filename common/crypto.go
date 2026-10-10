package common

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const encryptedValuePrefix = "enc:v1:"

var ErrCryptoSecretNotConfigured = errors.New("CRYPTO_SECRET must be explicitly configured with at least 32 characters")

func HasStableCryptoSecret() bool {
	secret := strings.TrimSpace(CryptoSecret)
	return CryptoSecretConfigured && len([]byte(secret)) >= 32 && secret != "random_string"
}

func deriveEncryptedSettingKey() ([32]byte, error) {
	if !HasStableCryptoSecret() {
		return [32]byte{}, ErrCryptoSecretNotConfigured
	}
	return sha256.Sum256([]byte("new-api:encrypted-setting:v1\x00" + CryptoSecret)), nil
}

// EncryptSensitiveValue encrypts a setting for database storage using AES-256-GCM.
// The versioned prefix allows future key derivation or cipher migrations.
func EncryptSensitiveValue(plaintext string) (string, error) {
	key, err := deriveEncryptedSettingKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), []byte(encryptedValuePrefix))
	payload := append(nonce, ciphertext...)
	return encryptedValuePrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

func DecryptSensitiveValue(encrypted string) (string, error) {
	if !strings.HasPrefix(encrypted, encryptedValuePrefix) {
		return "", errors.New("unsupported encrypted setting format")
	}
	key, err := deriveEncryptedSettingKey()
	if err != nil {
		return "", err
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encrypted, encryptedValuePrefix))
	if err != nil {
		return "", fmt.Errorf("decode encrypted setting: %w", err)
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("encrypted setting payload is truncated")
	}
	nonce, ciphertext := payload[:gcm.NonceSize()], payload[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(encryptedValuePrefix))
	if err != nil {
		return "", errors.New("decrypt encrypted setting: authentication failed")
	}
	return string(plaintext), nil
}

func GenerateHMACWithKey(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateHMAC(data string) string {
	h := hmac.New(sha256.New, []byte(CryptoSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func Password2Hash(password string) (string, error) {
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	return string(hashedPassword), err
}

func ValidatePasswordAndHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
