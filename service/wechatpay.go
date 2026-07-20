package service

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
)

const (
	WechatPayCurrency             = "CNY"
	WechatPayOrderLifetime        = 15 * time.Minute
	WechatPayTestOrderLifetime    = 10 * time.Minute
	WechatPayQueryIntervalSeconds = int64(10)
)

var (
	wechatPayMchIdPattern       = regexp.MustCompile(`^[0-9]{6,32}$`)
	wechatPayPublicKeyIdPattern = regexp.MustCompile(`^PUB_KEY_ID_[A-Za-z0-9_\-]+$`)
)

type WechatPayPlainConfig struct {
	Enabled              bool
	ShowEpayWechat       bool
	AppId                string
	MchId                string
	MerchantCertificate  string
	MerchantPrivateKey   string
	ApiV3Key             string
	WechatPayPublicKeyId string
	WechatPayPublicKey   string
}

type WechatPayValidation struct {
	MerchantCertificateSerial      string
	MerchantCertificateFingerprint string
	WechatPayPublicKeyFingerprint  string
	MerchantPrivateKey             *rsa.PrivateKey
	WechatPayPublicKey             *rsa.PublicKey
}

type WechatPayConfigStatus struct {
	Configured                     bool   `json:"configured"`
	Ready                          bool   `json:"ready"`
	Enabled                        bool   `json:"enabled"`
	ShowEpayWechat                 bool   `json:"show_epay_wechat"`
	CryptoSecretConfigured         bool   `json:"crypto_secret_configured"`
	AppId                          string `json:"appid,omitempty"`
	MchId                          string `json:"mchid,omitempty"`
	MerchantCertificateSerial      string `json:"merchant_certificate_serial,omitempty"`
	MerchantCertificateFingerprint string `json:"merchant_certificate_fingerprint,omitempty"`
	WechatPayPublicKeyId           string `json:"wechatpay_public_key_id,omitempty"`
	WechatPayPublicKeyFingerprint  string `json:"wechatpay_public_key_fingerprint,omitempty"`
	HasMerchantCertificate         bool   `json:"has_merchant_certificate"`
	HasMerchantPrivateKey          bool   `json:"has_merchant_private_key"`
	HasApiV3Key                    bool   `json:"has_api_v3_key"`
	HasWechatPayPublicKey          bool   `json:"has_wechatpay_public_key"`
	ValidatedAt                    int64  `json:"validated_at,omitempty"`
	VerifiedAt                     int64  `json:"verified_at,omitempty"`
	CallbackUrl                    string `json:"callback_url"`
	Error                          string `json:"error,omitempty"`
}

type WechatPayGateway struct {
	config        *model.WechatPayConfig
	plain         WechatPayPlainConfig
	nativeService native.NativeApiService
	notifyHandler *notify.Handler
}

type WechatPayProcessResult struct {
	Test          bool
	Settled       bool
	Status        string
	UserId        int
	QuotaToAdd    int
	Money         float64
	PaymentMethod string
}

func normalizePem(value string) string {
	value = strings.TrimPrefix(value, "\ufeff")
	return strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n")) + "\n"
}

func parseMerchantPrivateKey(value string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(normalizePem(value)))
	if block == nil {
		return nil, errors.New("无法识别 apiclient_key.pem")
	}
	switch block.Type {
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("解析商户私钥失败: %w", err)
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("商户私钥不是 RSA 私钥")
		}
		return key, nil
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("解析商户 RSA 私钥失败: %w", err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("apiclient_key.pem 类型必须为 PRIVATE KEY，当前为 %s", block.Type)
	}
}

func parseWechatPayPublicKey(value string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(normalizePem(value)))
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("pub_key.pem 必须是 PUBLIC KEY 格式")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("解析微信支付公钥失败: %w", err)
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("微信支付公钥不是 RSA 公钥")
	}
	return key, nil
}

func rsaPublicKeyFingerprint(key *rsa.PublicKey) (string, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

func ValidateWechatPayPlainConfig(config WechatPayPlainConfig) (*WechatPayValidation, error) {
	config.AppId = strings.TrimSpace(config.AppId)
	config.MchId = strings.TrimSpace(config.MchId)
	config.ApiV3Key = strings.TrimSpace(config.ApiV3Key)
	config.WechatPayPublicKeyId = strings.TrimSpace(config.WechatPayPublicKeyId)
	if config.AppId == "" || len(config.AppId) > 32 || strings.ContainsAny(config.AppId, " \t\r\n") {
		return nil, errors.New("AppID 格式不正确")
	}
	if !wechatPayMchIdPattern.MatchString(config.MchId) {
		return nil, errors.New("商户号应为 6 到 32 位数字")
	}
	if len([]byte(config.ApiV3Key)) != 32 {
		return nil, errors.New("APIv3 Key 必须正好是 32 个字节")
	}
	if !wechatPayPublicKeyIdPattern.MatchString(config.WechatPayPublicKeyId) {
		return nil, errors.New("微信支付公钥 ID 格式不正确，应以 PUB_KEY_ID_ 开头")
	}

	certificate, err := utils.LoadCertificate(normalizePem(config.MerchantCertificate))
	if err != nil {
		return nil, fmt.Errorf("解析 apiclient_cert.pem 失败: %w", err)
	}
	if !utils.IsCertificateValid(*certificate, time.Now()) {
		return nil, errors.New("商户 API 证书尚未生效或已经过期")
	}
	certificatePublicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("商户 API 证书不是 RSA 证书")
	}
	privateKey, err := parseMerchantPrivateKey(config.MerchantPrivateKey)
	if err != nil {
		return nil, err
	}
	if certificatePublicKey.N.Cmp(privateKey.PublicKey.N) != 0 || certificatePublicKey.E != privateKey.PublicKey.E {
		return nil, errors.New("apiclient_cert.pem 与 apiclient_key.pem 不匹配")
	}
	publicKey, err := parseWechatPayPublicKey(config.WechatPayPublicKey)
	if err != nil {
		return nil, err
	}
	merchantFingerprint, err := rsaPublicKeyFingerprint(certificatePublicKey)
	if err != nil {
		return nil, err
	}
	wechatFingerprint, err := rsaPublicKeyFingerprint(publicKey)
	if err != nil {
		return nil, err
	}
	return &WechatPayValidation{
		MerchantCertificateSerial:      utils.GetCertificateSerialNumber(*certificate),
		MerchantCertificateFingerprint: merchantFingerprint,
		WechatPayPublicKeyFingerprint:  wechatFingerprint,
		MerchantPrivateKey:             privateKey,
		WechatPayPublicKey:             publicKey,
	}, nil
}

func decryptWechatPayConfig(record *model.WechatPayConfig) (WechatPayPlainConfig, error) {
	if record == nil {
		return WechatPayPlainConfig{}, errors.New("微信支付尚未配置")
	}
	merchantCertificate, err := common.DecryptSensitiveValue(record.MerchantCertificateEncrypted)
	if err != nil {
		return WechatPayPlainConfig{}, fmt.Errorf("解密商户证书失败: %w", err)
	}
	merchantPrivateKey, err := common.DecryptSensitiveValue(record.MerchantPrivateKeyEncrypted)
	if err != nil {
		return WechatPayPlainConfig{}, fmt.Errorf("解密商户私钥失败: %w", err)
	}
	apiV3Key, err := common.DecryptSensitiveValue(record.ApiV3KeyEncrypted)
	if err != nil {
		return WechatPayPlainConfig{}, fmt.Errorf("解密 APIv3 Key 失败: %w", err)
	}
	wechatPayPublicKey, err := common.DecryptSensitiveValue(record.WechatPayPublicKeyEncrypted)
	if err != nil {
		return WechatPayPlainConfig{}, fmt.Errorf("解密微信支付公钥失败: %w", err)
	}
	return WechatPayPlainConfig{
		Enabled: record.Enabled, ShowEpayWechat: record.ShowEpayWechat, AppId: record.AppId, MchId: record.MchId,
		MerchantCertificate: merchantCertificate, MerchantPrivateKey: merchantPrivateKey,
		ApiV3Key: apiV3Key, WechatPayPublicKeyId: record.WechatPayPublicKeyId,
		WechatPayPublicKey: wechatPayPublicKey,
	}, nil
}

func LoadWechatPayPlainConfig() (*model.WechatPayConfig, WechatPayPlainConfig, error) {
	record, err := model.GetWechatPayConfig()
	if err != nil || record == nil {
		return record, WechatPayPlainConfig{}, err
	}
	plain, err := decryptWechatPayConfig(record)
	return record, plain, err
}

func GetWechatPayCallbackUrl() (string, error) {
	base := strings.TrimRight(strings.TrimSpace(GetCallbackAddress()), "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", errors.New("请先配置有效的服务器地址或自定义回调地址")
	}
	if parsed.Scheme != "https" {
		return "", errors.New("微信支付回调地址必须使用 HTTPS")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("微信支付回调基础地址不能包含查询参数或片段")
	}
	return base + "/api/wechat-pay/notify", nil
}

func SaveWechatPayConfiguration(plain WechatPayPlainConfig) (*model.WechatPayConfig, error) {
	if !common.HasStableCryptoSecret() {
		return nil, common.ErrCryptoSecretNotConfigured
	}
	plain.AppId = strings.TrimSpace(plain.AppId)
	plain.MchId = strings.TrimSpace(plain.MchId)
	plain.ApiV3Key = strings.TrimSpace(plain.ApiV3Key)
	plain.WechatPayPublicKeyId = strings.TrimSpace(plain.WechatPayPublicKeyId)
	plain.MerchantCertificate = normalizePem(plain.MerchantCertificate)
	plain.MerchantPrivateKey = normalizePem(plain.MerchantPrivateKey)
	plain.WechatPayPublicKey = normalizePem(plain.WechatPayPublicKey)
	validation, err := ValidateWechatPayPlainConfig(plain)
	if err != nil {
		return nil, err
	}
	if plain.Enabled {
		if _, err = GetWechatPayCallbackUrl(); err != nil {
			return nil, err
		}
	}

	existing, existingPlain, loadErr := LoadWechatPayPlainConfig()
	if loadErr != nil && existing == nil {
		return nil, loadErr
	}
	materialChanged := existing == nil || loadErr != nil || existingPlain.AppId != plain.AppId || existingPlain.MchId != plain.MchId ||
		existingPlain.MerchantCertificate != plain.MerchantCertificate || existingPlain.MerchantPrivateKey != plain.MerchantPrivateKey ||
		existingPlain.ApiV3Key != plain.ApiV3Key || existingPlain.WechatPayPublicKeyId != plain.WechatPayPublicKeyId ||
		existingPlain.WechatPayPublicKey != plain.WechatPayPublicKey
	if materialChanged && existing != nil {
		pending, pendingErr := model.HasActivePendingWechatPayOrders(common.GetTimestamp())
		if pendingErr != nil {
			return nil, pendingErr
		}
		if pending {
			return nil, errors.New("存在未过期的微信支付订单，请等待订单完成或过期后再更换凭据")
		}
	}
	if plain.Enabled && (existing == nil || materialChanged || existing.VerifiedAt == 0) {
		return nil, errors.New("请先保存配置并完成 ¥0.01 真实支付测试，再启用微信支付")
	}

	merchantCertificateEncrypted, err := common.EncryptSensitiveValue(plain.MerchantCertificate)
	if err != nil {
		return nil, err
	}
	merchantPrivateKeyEncrypted, err := common.EncryptSensitiveValue(plain.MerchantPrivateKey)
	if err != nil {
		return nil, err
	}
	apiV3KeyEncrypted, err := common.EncryptSensitiveValue(plain.ApiV3Key)
	if err != nil {
		return nil, err
	}
	wechatPayPublicKeyEncrypted, err := common.EncryptSensitiveValue(plain.WechatPayPublicKey)
	if err != nil {
		return nil, err
	}
	verifiedAt := int64(0)
	createTime := int64(0)
	if existing != nil {
		createTime = existing.CreateTime
		if !materialChanged {
			verifiedAt = existing.VerifiedAt
		}
	}
	record := &model.WechatPayConfig{
		Enabled: plain.Enabled, ShowEpayWechat: plain.ShowEpayWechat, AppId: plain.AppId, MchId: plain.MchId,
		MerchantCertificateSerial:      validation.MerchantCertificateSerial,
		MerchantCertificateFingerprint: validation.MerchantCertificateFingerprint,
		WechatPayPublicKeyId:           plain.WechatPayPublicKeyId,
		WechatPayPublicKeyFingerprint:  validation.WechatPayPublicKeyFingerprint,
		MerchantCertificateEncrypted:   merchantCertificateEncrypted,
		MerchantPrivateKeyEncrypted:    merchantPrivateKeyEncrypted,
		ApiV3KeyEncrypted:              apiV3KeyEncrypted,
		WechatPayPublicKeyEncrypted:    wechatPayPublicKeyEncrypted,
		ValidatedAt:                    common.GetTimestamp(), VerifiedAt: verifiedAt, CreateTime: createTime,
	}
	if err = model.SaveWechatPayConfig(record); err != nil {
		return nil, err
	}
	return record, nil
}

func GetWechatPayConfigStatus() WechatPayConfigStatus {
	status := WechatPayConfigStatus{CryptoSecretConfigured: common.HasStableCryptoSecret()}
	callbackUrl, callbackErr := GetWechatPayCallbackUrl()
	if callbackErr == nil {
		status.CallbackUrl = callbackUrl
	}
	record, plain, err := LoadWechatPayPlainConfig()
	if record == nil {
		if err != nil {
			status.Error = err.Error()
		} else if callbackErr != nil {
			status.Error = callbackErr.Error()
		}
		return status
	}
	status.Configured = true
	status.Enabled = record.Enabled
	status.ShowEpayWechat = record.ShowEpayWechat
	status.AppId = record.AppId
	status.MchId = record.MchId
	status.MerchantCertificateSerial = record.MerchantCertificateSerial
	status.MerchantCertificateFingerprint = record.MerchantCertificateFingerprint
	status.WechatPayPublicKeyId = record.WechatPayPublicKeyId
	status.WechatPayPublicKeyFingerprint = record.WechatPayPublicKeyFingerprint
	if err != nil {
		status.Error = err.Error()
		return status
	}
	status.HasMerchantCertificate = record.MerchantCertificateEncrypted != ""
	status.HasMerchantPrivateKey = record.MerchantPrivateKeyEncrypted != ""
	status.HasApiV3Key = record.ApiV3KeyEncrypted != ""
	status.HasWechatPayPublicKey = record.WechatPayPublicKeyEncrypted != ""
	status.ValidatedAt = record.ValidatedAt
	status.VerifiedAt = record.VerifiedAt
	if _, validationErr := ValidateWechatPayPlainConfig(plain); validationErr != nil {
		status.Error = validationErr.Error()
		return status
	}
	if callbackErr != nil {
		status.Error = callbackErr.Error()
		return status
	}
	status.Ready = status.CryptoSecretConfigured
	return status
}

func NewWechatPayGateway(ctx context.Context, requireEnabled bool) (*WechatPayGateway, error) {
	record, plain, err := LoadWechatPayPlainConfig()
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, errors.New("微信支付尚未配置")
	}
	if requireEnabled && !record.Enabled {
		return nil, errors.New("微信支付尚未启用")
	}
	validation, err := ValidateWechatPayPlainConfig(plain)
	if err != nil {
		return nil, err
	}
	client, err := core.NewClient(ctx,
		option.WithWechatPayPublicKeyAuthCipher(
			plain.MchId, record.MerchantCertificateSerial, validation.MerchantPrivateKey,
			plain.WechatPayPublicKeyId, validation.WechatPayPublicKey,
		),
		option.WithHTTPClient(&http.Client{Timeout: 12 * time.Second}),
	)
	if err != nil {
		return nil, fmt.Errorf("初始化微信支付客户端失败: %w", err)
	}
	handler, err := notify.NewRSANotifyHandler(
		plain.ApiV3Key,
		verifiers.NewSHA256WithRSAPubkeyVerifier(plain.WechatPayPublicKeyId, *validation.WechatPayPublicKey),
	)
	if err != nil {
		return nil, fmt.Errorf("初始化微信支付回调处理器失败: %w", err)
	}
	return &WechatPayGateway{
		config: record, plain: plain,
		nativeService: native.NativeApiService{Client: client}, notifyHandler: handler,
	}, nil
}

func (gateway *WechatPayGateway) Prepay(ctx context.Context, tradeNo string, amountCents int64, description string, expireAt time.Time) (string, error) {
	callbackUrl, err := GetWechatPayCallbackUrl()
	if err != nil {
		return "", err
	}
	response, _, err := gateway.nativeService.Prepay(ctx, native.PrepayRequest{
		Appid: core.String(gateway.plain.AppId), Mchid: core.String(gateway.plain.MchId),
		Description: core.String(description), OutTradeNo: core.String(tradeNo),
		TimeExpire: core.Time(expireAt), NotifyUrl: core.String(callbackUrl),
		Amount: &native.Amount{Total: core.Int64(amountCents), Currency: core.String(WechatPayCurrency)},
	})
	if err != nil {
		return "", err
	}
	if response == nil || response.CodeUrl == nil || strings.TrimSpace(*response.CodeUrl) == "" {
		return "", errors.New("微信支付未返回二维码链接")
	}
	return *response.CodeUrl, nil
}

func (gateway *WechatPayGateway) Query(ctx context.Context, tradeNo string) (*payments.Transaction, error) {
	transaction, _, err := gateway.nativeService.QueryOrderByOutTradeNo(ctx, native.QueryOrderByOutTradeNoRequest{
		OutTradeNo: core.String(tradeNo), Mchid: core.String(gateway.plain.MchId),
	})
	return transaction, err
}

func (gateway *WechatPayGateway) Close(ctx context.Context, tradeNo string) error {
	_, err := gateway.nativeService.CloseOrder(ctx, native.CloseOrderRequest{
		OutTradeNo: core.String(tradeNo), Mchid: core.String(gateway.plain.MchId),
	})
	return err
}

func (gateway *WechatPayGateway) ParseNotify(ctx context.Context, request *http.Request) (*payments.Transaction, string, error) {
	transaction := new(payments.Transaction)
	notifyRequest, err := gateway.notifyHandler.ParseNotifyRequest(ctx, request, transaction)
	if err != nil {
		return nil, "", err
	}
	return transaction, notifyRequest.ID, nil
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (gateway *WechatPayGateway) validateSuccessfulTransaction(transaction *payments.Transaction) error {
	if transaction == nil || valueOrEmpty(transaction.TradeState) != "SUCCESS" {
		return errors.New("微信支付交易尚未成功")
	}
	if valueOrEmpty(transaction.Appid) != gateway.plain.AppId || valueOrEmpty(transaction.Mchid) != gateway.plain.MchId {
		return errors.New("微信支付商户身份不匹配")
	}
	if valueOrEmpty(transaction.TradeType) != "NATIVE" {
		return errors.New("微信支付交易类型不是 NATIVE")
	}
	if transaction.OutTradeNo == nil || transaction.TransactionId == nil || transaction.Amount == nil || transaction.Amount.Total == nil {
		return errors.New("微信支付交易数据不完整")
	}
	if valueOrEmpty(transaction.Amount.Currency) != WechatPayCurrency {
		return errors.New("微信支付交易币种不匹配")
	}
	return nil
}

func (gateway *WechatPayGateway) ProcessSuccessfulTransaction(transaction *payments.Transaction, notifyId string, callerIp string) (WechatPayProcessResult, error) {
	if err := gateway.validateSuccessfulTransaction(transaction); err != nil {
		return WechatPayProcessResult{}, err
	}
	tradeNo := *transaction.OutTradeNo
	transactionId := *transaction.TransactionId
	paidCents := *transaction.Amount.Total
	if strings.HasPrefix(tradeNo, "WXT") {
		completed, err := model.CompleteWechatPayTestOrder(tradeNo, transactionId, notifyId, paidCents, WechatPayCurrency)
		return WechatPayProcessResult{Test: true, Settled: completed, Status: common.TopUpStatusSuccess}, err
	}
	settlement, err := model.SettleWechatPayTopUp(tradeNo, transactionId, paidCents, WechatPayCurrency)
	if err != nil {
		return WechatPayProcessResult{}, err
	}
	if settlement.Settled {
		model.RecordTopupLog(
			settlement.UserId,
			fmt.Sprintf("微信支付充值成功，充值额度: %v，支付金额: %.2f", logger.FormatQuota(settlement.QuotaToAdd), settlement.Money),
			callerIp, settlement.PaymentMethod, model.PaymentProviderWechatPay,
		)
	}
	return WechatPayProcessResult{
		Settled: settlement.Settled, Status: common.TopUpStatusSuccess,
		UserId: settlement.UserId, QuotaToAdd: settlement.QuotaToAdd,
		Money: settlement.Money, PaymentMethod: settlement.PaymentMethod,
	}, nil
}

func (gateway *WechatPayGateway) SyncUserOrder(ctx context.Context, order *model.TopUp, callerIp string) (WechatPayProcessResult, error) {
	transaction, err := gateway.Query(ctx, order.TradeNo)
	if err != nil {
		return WechatPayProcessResult{Status: order.Status}, err
	}
	state := valueOrEmpty(transaction.TradeState)
	switch state {
	case "SUCCESS":
		return gateway.ProcessSuccessfulTransaction(transaction, "", callerIp)
	case "CLOSED", "REVOKED":
		err = model.UpdateWechatPayTopUpPendingStatus(order.TradeNo, common.TopUpStatusExpired)
		return WechatPayProcessResult{Status: common.TopUpStatusExpired}, err
	case "PAYERROR":
		err = model.UpdateWechatPayTopUpPendingStatus(order.TradeNo, common.TopUpStatusFailed)
		return WechatPayProcessResult{Status: common.TopUpStatusFailed}, err
	default:
		return WechatPayProcessResult{Status: common.TopUpStatusPending}, nil
	}
}

func (gateway *WechatPayGateway) SyncTestOrder(ctx context.Context, order *model.WechatPayTestOrder) (WechatPayProcessResult, error) {
	transaction, err := gateway.Query(ctx, order.TradeNo)
	if err != nil {
		return WechatPayProcessResult{Test: true, Status: order.Status}, err
	}
	state := valueOrEmpty(transaction.TradeState)
	switch state {
	case "SUCCESS":
		return gateway.ProcessSuccessfulTransaction(transaction, "", "")
	case "CLOSED", "REVOKED":
		err = model.UpdateWechatPayTestPendingStatus(order.TradeNo, common.TopUpStatusExpired)
		return WechatPayProcessResult{Test: true, Status: common.TopUpStatusExpired}, err
	case "PAYERROR":
		err = model.UpdateWechatPayTestPendingStatus(order.TradeNo, common.TopUpStatusFailed)
		return WechatPayProcessResult{Test: true, Status: common.TopUpStatusFailed}, err
	default:
		return WechatPayProcessResult{Test: true, Status: common.TopUpStatusPending}, nil
	}
}
