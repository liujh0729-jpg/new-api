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

const (
	encryptedValuePrefixV1 = "enc:v1:"
	encryptedValuePrefixV2 = "enc:v2:"
	encryptedValuePrefix   = encryptedValuePrefixV2
	envelopeDataKeySize    = 32
)

var ErrCryptoSecretNotConfigured = errors.New("CRYPTO_SECRET must be explicitly configured with at least 32 characters")

func HasStableCryptoSecret() bool {
	secret := strings.TrimSpace(CryptoSecret)
	return CryptoSecretConfigured && len([]byte(secret)) >= 32 && secret != "random_string"
}

func deriveEncryptedSettingKeyV1() ([32]byte, error) {
	if !HasStableCryptoSecret() {
		return [32]byte{}, ErrCryptoSecretNotConfigured
	}
	return sha256.Sum256([]byte("new-api:encrypted-setting:v1\x00" + CryptoSecret)), nil
}

func deriveEncryptedSettingKEKV2() ([32]byte, error) {
	if !HasStableCryptoSecret() {
		return [32]byte{}, ErrCryptoSecretNotConfigured
	}
	return sha256.Sum256([]byte("new-api:encrypted-setting:kek:v2\x00" + CryptoSecret)), nil
}

// EncryptSensitiveValue uses per-record envelope encryption. A random data key
// encrypts the value and is itself wrapped by a key-encryption key derived from
// the deployment secret. Only the wrapped data key is stored with the payload.
func EncryptSensitiveValue(plaintext string) (string, error) {
	kek, err := deriveEncryptedSettingKEKV2()
	if err != nil {
		return "", err
	}
	kekBlock, err := aes.NewCipher(kek[:])
	if err != nil {
		return "", err
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return "", err
	}
	dataKey := make([]byte, envelopeDataKeySize)
	if _, err = io.ReadFull(rand.Reader, dataKey); err != nil {
		return "", err
	}
	defer clear(dataKey)
	dataBlock, err := aes.NewCipher(dataKey)
	if err != nil {
		return "", err
	}
	dataGCM, err := cipher.NewGCM(dataBlock)
	if err != nil {
		return "", err
	}
	keyNonce := make([]byte, kekGCM.NonceSize())
	dataNonce := make([]byte, dataGCM.NonceSize())
	if _, err = io.ReadFull(rand.Reader, keyNonce); err != nil {
		return "", err
	}
	if _, err = io.ReadFull(rand.Reader, dataNonce); err != nil {
		return "", err
	}
	wrappedKey := kekGCM.Seal(nil, keyNonce, dataKey, []byte(encryptedValuePrefixV2+"key"))
	ciphertext := dataGCM.Seal(nil, dataNonce, []byte(plaintext), []byte(encryptedValuePrefixV2+"data"))
	payload := make([]byte, 0, len(keyNonce)+len(wrappedKey)+len(dataNonce)+len(ciphertext))
	payload = append(payload, keyNonce...)
	payload = append(payload, wrappedKey...)
	payload = append(payload, dataNonce...)
	payload = append(payload, ciphertext...)
	return encryptedValuePrefixV2 + base64.RawStdEncoding.EncodeToString(payload), nil
}

func DecryptSensitiveValue(encrypted string) (string, error) {
	switch {
	case strings.HasPrefix(encrypted, encryptedValuePrefixV2):
		return decryptSensitiveValueV2(encrypted)
	case strings.HasPrefix(encrypted, encryptedValuePrefixV1):
		return decryptSensitiveValueV1(encrypted)
	default:
		return "", errors.New("unsupported encrypted setting format")
	}
}

func decryptSensitiveValueV2(encrypted string) (string, error) {
	kek, err := deriveEncryptedSettingKEKV2()
	if err != nil {
		return "", err
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encrypted, encryptedValuePrefixV2))
	if err != nil {
		return "", fmt.Errorf("decode encrypted setting: %w", err)
	}
	kekBlock, err := aes.NewCipher(kek[:])
	if err != nil {
		return "", err
	}
	kekGCM, err := cipher.NewGCM(kekBlock)
	if err != nil {
		return "", err
	}
	wrappedKeySize := envelopeDataKeySize + kekGCM.Overhead()
	minimumSize := kekGCM.NonceSize() + wrappedKeySize + 12 + 16
	if len(payload) < minimumSize {
		return "", errors.New("encrypted setting payload is truncated")
	}
	keyNonceEnd := kekGCM.NonceSize()
	wrappedKeyEnd := keyNonceEnd + wrappedKeySize
	dataKey, err := kekGCM.Open(nil, payload[:keyNonceEnd], payload[keyNonceEnd:wrappedKeyEnd], []byte(encryptedValuePrefixV2+"key"))
	if err != nil {
		return "", errors.New("decrypt encrypted setting: authentication failed")
	}
	defer clear(dataKey)
	dataBlock, err := aes.NewCipher(dataKey)
	if err != nil {
		return "", errors.New("decrypt encrypted setting: invalid data key")
	}
	dataGCM, err := cipher.NewGCM(dataBlock)
	if err != nil {
		return "", err
	}
	dataNonceEnd := wrappedKeyEnd + dataGCM.NonceSize()
	if len(payload) < dataNonceEnd+dataGCM.Overhead() {
		return "", errors.New("encrypted setting payload is truncated")
	}
	plaintext, err := dataGCM.Open(nil, payload[wrappedKeyEnd:dataNonceEnd], payload[dataNonceEnd:], []byte(encryptedValuePrefixV2+"data"))
	if err != nil {
		return "", errors.New("decrypt encrypted setting: authentication failed")
	}
	return string(plaintext), nil
}

// encryptSensitiveValueV1 exists only for compatibility tests and migrations.
// New writes always use the envelope-encrypted v2 format.
func encryptSensitiveValueV1(plaintext string) (string, error) {
	key, err := deriveEncryptedSettingKeyV1()
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
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), []byte(encryptedValuePrefixV1))
	payload := append(nonce, ciphertext...)
	return encryptedValuePrefixV1 + base64.RawStdEncoding.EncodeToString(payload), nil
}

func decryptSensitiveValueV1(encrypted string) (string, error) {
	key, err := deriveEncryptedSettingKeyV1()
	if err != nil {
		return "", err
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(encrypted, encryptedValuePrefixV1))
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
	plaintext, err := gcm.Open(nil, nonce, ciphertext, []byte(encryptedValuePrefixV1))
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
