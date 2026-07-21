package service

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupWechatPayConfigTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousCallbackAddress := operation_setting.CustomCallbackAddress
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.WechatPayConfig{}, &model.WechatPayTestOrder{}))
	model.DB = db
	model.LOG_DB = db
	operation_setting.CustomCallbackAddress = "https://wechat-pay.example.test"
	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		operation_setting.CustomCallbackAddress = previousCallbackAddress
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func useWechatPayCryptoSecretForTest(t *testing.T, secret string) {
	t.Helper()
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	common.CryptoSecret = secret
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})
}

func makeWechatPayCredentialSet(t *testing.T) WechatPayPlainConfig {
	t.Helper()
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	wechatPayKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(20260720),
		Subject:      pkix.Name{CommonName: "wechat-pay-test-merchant"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certificateDer, err := x509.CreateCertificate(rand.Reader, template, template, &merchantKey.PublicKey, merchantKey)
	require.NoError(t, err)
	privateKeyDer, err := x509.MarshalPKCS8PrivateKey(merchantKey)
	require.NoError(t, err)
	wechatPayPublicKeyDer, err := x509.MarshalPKIXPublicKey(&wechatPayKey.PublicKey)
	require.NoError(t, err)

	return WechatPayPlainConfig{
		VerificationMode:     WechatPayVerificationModePublicKey,
		AppId:                "wx1234567890abcdef",
		MchId:                "1234567890",
		ApiV3Key:             "0123456789abcdef0123456789abcdef",
		WechatPayPublicKeyId: "PUB_KEY_ID_20260720",
		MerchantCertificate:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificateDer})),
		MerchantPrivateKey:   string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyDer})),
		WechatPayPublicKey:   string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: wechatPayPublicKeyDer})),
	}
}

func TestValidateWechatPayPlainConfig(t *testing.T) {
	config := makeWechatPayCredentialSet(t)

	validation, err := ValidateWechatPayPlainConfig(config)
	require.NoError(t, err)
	assert.NotEmpty(t, validation.MerchantCertificateSerial)
	assert.Len(t, validation.MerchantCertificateFingerprint, 64)
	assert.Len(t, validation.WechatPayPublicKeyFingerprint, 64)
}

func TestValidateWechatPayPlainConfigSupportsPlatformCertificateMode(t *testing.T) {
	config := makeWechatPayCredentialSet(t)
	config.VerificationMode = WechatPayVerificationModePlatformCertificate
	config.WechatPayPublicKeyId = ""
	config.WechatPayPublicKey = ""

	validation, err := ValidateWechatPayPlainConfig(config)
	require.NoError(t, err)
	assert.NotEmpty(t, validation.MerchantCertificateSerial)
	assert.Empty(t, validation.WechatPayPublicKeyFingerprint)
	assert.Nil(t, validation.WechatPayPublicKey)
}

func TestValidateWechatPayPlainConfigRequiresPublicKeyInPublicKeyMode(t *testing.T) {
	config := makeWechatPayCredentialSet(t)
	config.WechatPayPublicKeyId = ""
	config.WechatPayPublicKey = ""

	_, err := ValidateWechatPayPlainConfig(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "公钥 ID")
}

func TestValidateWechatPayPlainConfigRejectsInvalidApiV3Key(t *testing.T) {
	config := makeWechatPayCredentialSet(t)
	config.ApiV3Key = "too-short"

	_, err := ValidateWechatPayPlainConfig(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "32")
}

func TestValidateWechatPayPlainConfigRejectsMismatchedCertificateAndKey(t *testing.T) {
	config := makeWechatPayCredentialSet(t)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	otherKeyDer, err := x509.MarshalPKCS8PrivateKey(otherKey)
	require.NoError(t, err)
	config.MerchantPrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: otherKeyDer}))

	_, err = ValidateWechatPayPlainConfig(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不匹配")
}

func TestSaveWechatPayConfigurationRequiresRealPaymentBeforeEnable(t *testing.T) {
	setupWechatPayConfigTestDB(t)
	useWechatPayCryptoSecretForTest(t, "0123456789abcdef0123456789abcdef")
	config := makeWechatPayCredentialSet(t)

	_, err := SaveWechatPayConfiguration(config)
	require.NoError(t, err)
	config.Enabled = true
	_, err = SaveWechatPayConfiguration(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "¥0.01")

	require.NoError(t, model.DB.Model(&model.WechatPayConfig{}).
		Where("scope = ?", model.WechatPayConfigScopeDefault).
		Update("verified_at", time.Now().Unix()).Error)
	_, err = SaveWechatPayConfiguration(config)
	require.NoError(t, err)
}

func TestSaveWechatPayConfigurationCanReplaceUnreadableCredentials(t *testing.T) {
	setupWechatPayConfigTestDB(t)
	useWechatPayCryptoSecretForTest(t, "0123456789abcdef0123456789abcdef")
	config := makeWechatPayCredentialSet(t)

	first, err := SaveWechatPayConfiguration(config)
	require.NoError(t, err)
	firstCiphertext := first.MerchantPrivateKeyEncrypted
	require.NoError(t, model.DB.Model(&model.WechatPayConfig{}).
		Where("scope = ?", model.WechatPayConfigScopeDefault).
		Update("verified_at", time.Now().Unix()).Error)

	common.CryptoSecret = "fedcba9876543210fedcba9876543210"
	record, _, err := LoadWechatPayPlainConfig()
	require.NotNil(t, record)
	require.Error(t, err)

	replaced, err := SaveWechatPayConfiguration(config)
	require.NoError(t, err)
	assert.NotEqual(t, firstCiphertext, replaced.MerchantPrivateKeyEncrypted)
	assert.Zero(t, replaced.VerifiedAt)
	_, plain, err := LoadWechatPayPlainConfig()
	require.NoError(t, err)
	assert.Equal(t, config.ApiV3Key, plain.ApiV3Key)
}

func TestSaveWechatPayConfigurationStoresPlatformCertificateMode(t *testing.T) {
	setupWechatPayConfigTestDB(t)
	useWechatPayCryptoSecretForTest(t, "0123456789abcdef0123456789abcdef")
	config := makeWechatPayCredentialSet(t)
	config.VerificationMode = WechatPayVerificationModePlatformCertificate

	record, err := SaveWechatPayConfiguration(config)
	require.NoError(t, err)
	assert.Equal(t, WechatPayVerificationModePlatformCertificate, record.VerificationMode)
	assert.Empty(t, record.WechatPayPublicKeyId)
	assert.Empty(t, record.WechatPayPublicKeyEncrypted)
	assert.Empty(t, record.WechatPayPublicKeyFingerprint)

	_, plain, err := LoadWechatPayPlainConfig()
	require.NoError(t, err)
	assert.Equal(t, WechatPayVerificationModePlatformCertificate, plain.VerificationMode)
	assert.Empty(t, plain.WechatPayPublicKeyId)
	assert.Empty(t, plain.WechatPayPublicKey)

	status := GetWechatPayConfigStatus()
	assert.Equal(t, WechatPayVerificationModePlatformCertificate, status.VerificationMode)
	assert.False(t, status.HasWechatPayPublicKey)
	assert.True(t, status.Ready)
	assert.Empty(t, status.Error)
}
