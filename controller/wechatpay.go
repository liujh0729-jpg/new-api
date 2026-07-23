package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	wechatPayConfigBodyLimit = 256 << 10
	wechatPayNotifyBodyLimit = 64 << 10
)

type wechatPayNativeRequest struct {
	Amount        int64   `json:"amount"`
	AmountUnit    *string `json:"amount_unit,omitempty"`
	PaymentMethod string  `json:"payment_method,omitempty"`
}

func wechatPayError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"success": false, "message": message})
}

func readWechatPayPem(c *gin.Context, field string) (string, bool, error) {
	header, err := c.FormFile(field)
	if errors.Is(err, http.ErrMissingFile) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	file, err := header.Open()
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, wechatPayConfigBodyLimit+1))
	if err != nil {
		return "", false, err
	}
	if len(data) > wechatPayConfigBodyLimit {
		return "", false, errors.New("上传文件过大")
	}
	return string(data), true, nil
}

func parseOptionalBool(c *gin.Context, key string, fallback bool) (bool, error) {
	value, exists := c.GetPostForm(key)
	if !exists {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s 必须是布尔值", key)
	}
	return parsed, nil
}

func GetWechatPayConfig(c *gin.Context) {
	common.ApiSuccess(c, service.GetWechatPayConfigStatus())
}

func UpdateWechatPayConfig(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, wechatPayConfigBodyLimit)
	if err := c.Request.ParseMultipartForm(wechatPayConfigBodyLimit); err != nil {
		wechatPayError(c, http.StatusBadRequest, "配置请求过大或格式不正确")
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}

	record, plain, err := service.LoadWechatPayPlainConfig()
	credentialsNeedReplacement := err != nil && record != nil
	if err != nil && record == nil {
		wechatPayError(c, http.StatusInternalServerError, "读取微信支付配置失败")
		return
	}
	if credentialsNeedReplacement {
		plain = service.WechatPayPlainConfig{
			AppId: record.AppId, MchId: record.MchId, VerificationMode: record.VerificationMode,
			WechatPayPublicKeyId: record.WechatPayPublicKeyId,
			ShowEpayWechat:       record.ShowEpayWechat,
		}
	}
	if value, exists := c.GetPostForm("appid"); exists {
		plain.AppId = value
	}
	if value, exists := c.GetPostForm("mchid"); exists {
		plain.MchId = value
	}
	if value, exists := c.GetPostForm("api_v3_key"); exists && strings.TrimSpace(value) != "" {
		plain.ApiV3Key = value
	}
	if value, exists := c.GetPostForm("verification_mode"); exists {
		plain.VerificationMode = value
	}
	if value, exists := c.GetPostForm("wechatpay_public_key_id"); exists {
		plain.WechatPayPublicKeyId = value
	}
	plain.Enabled, err = parseOptionalBool(c, "enabled", plain.Enabled)
	if err != nil {
		wechatPayError(c, http.StatusBadRequest, err.Error())
		return
	}
	plain.ShowEpayWechat, err = parseOptionalBool(c, "show_epay_wechat", plain.ShowEpayWechat)
	if err != nil {
		wechatPayError(c, http.StatusBadRequest, err.Error())
		return
	}
	if credentialsNeedReplacement {
		// A recovered credential set must pass the real payment test before it can be enabled again.
		plain.Enabled = false
	}

	if value, provided, readErr := readWechatPayPem(c, "merchant_certificate"); readErr != nil {
		wechatPayError(c, http.StatusBadRequest, "读取 apiclient_cert.pem 失败")
		return
	} else if provided {
		plain.MerchantCertificate = value
	}
	if value, provided, readErr := readWechatPayPem(c, "merchant_private_key"); readErr != nil {
		wechatPayError(c, http.StatusBadRequest, "读取 apiclient_key.pem 失败")
		return
	} else if provided {
		plain.MerchantPrivateKey = value
	}
	if value, provided, readErr := readWechatPayPem(c, "wechatpay_public_key"); readErr != nil {
		wechatPayError(c, http.StatusBadRequest, "读取 pub_key.pem 失败")
		return
	} else if provided {
		plain.WechatPayPublicKey = value
	}

	if _, err = service.SaveWechatPayConfiguration(plain); err != nil {
		wechatPayError(c, http.StatusBadRequest, err.Error())
		return
	}
	common.ApiSuccess(c, service.GetWechatPayConfigStatus())
}

func makeWechatPayTradeNo(prefix string) string {
	return fmt.Sprintf("%s%d%s", prefix, time.Now().UnixMilli(), common.GetRandomString(8))
}

func truncateWechatPayDescription(value string, maxRunes int) string {
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}

func wechatPayOrderData(tradeNo string, codeUrl string, status string, amountCents int64, expireTime int64) gin.H {
	return gin.H{
		"trade_no": tradeNo, "code_url": codeUrl, "status": status,
		"payment_amount_cents": amountCents, "currency": service.WechatPayCurrency,
		"expires_at": expireTime,
	}
}

func prepayWechatPayTestOrder(c *gin.Context, gateway *service.WechatPayGateway, order *model.WechatPayTestOrder) {
	codeUrl, err := gateway.Prepay(
		c.Request.Context(), order.TradeNo, order.PaymentAmountCents,
		truncateWechatPayDescription(common.SystemName+" 微信支付配置测试", 60), time.Unix(order.ExpireTime, 0),
	)
	if err != nil {
		transaction, queryErr := gateway.Query(c.Request.Context(), order.TradeNo)
		if queryErr == nil && transaction != nil && transaction.TradeState != nil {
			if *transaction.TradeState == "SUCCESS" {
				if _, processErr := gateway.ProcessSuccessfulTransaction(transaction, "", c.ClientIP()); processErr == nil {
					common.ApiSuccess(c, wechatPayOrderData(order.TradeNo, "", common.TopUpStatusSuccess, 1, order.ExpireTime))
					return
				} else {
					logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 测试预下单后结算失败 trade_no=%s error=%q", order.TradeNo, processErr.Error()))
				}
			}
			if *transaction.TradeState == "NOTPAY" {
				if closeErr := gateway.Close(c.Request.Context(), order.TradeNo); closeErr == nil {
					_ = model.UpdateWechatPayTestPendingStatus(order.TradeNo, common.TopUpStatusExpired)
				}
			}
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 测试预下单失败 trade_no=%s error=%q", order.TradeNo, err.Error()))
		wechatPayError(c, http.StatusBadGateway, "微信支付预下单失败，请稍后重试")
		return
	}
	if err = model.SetWechatPayTestOrderCodeUrl(order.TradeNo, codeUrl); err != nil {
		wechatPayError(c, http.StatusInternalServerError, "保存测试订单二维码失败")
		return
	}
	common.ApiSuccess(c, wechatPayOrderData(order.TradeNo, codeUrl, common.TopUpStatusPending, 1, order.ExpireTime))
}

func CreateWechatPayTestOrder(c *gin.Context) {
	if err := model.ExpireStaleWechatPayOrders(common.GetTimestamp()); err != nil {
		wechatPayError(c, http.StatusInternalServerError, "清理过期测试订单失败")
		return
	}
	gateway, err := service.NewWechatPayGateway(c.Request.Context(), false)
	if err != nil {
		wechatPayError(c, http.StatusBadRequest, err.Error())
		return
	}
	order, err := model.FindActiveWechatPayTestOrder(common.GetTimestamp())
	if err != nil {
		wechatPayError(c, http.StatusInternalServerError, "读取测试订单失败")
		return
	}
	if order != nil && order.CodeUrl != "" {
		common.ApiSuccess(c, wechatPayOrderData(order.TradeNo, order.CodeUrl, order.Status, order.PaymentAmountCents, order.ExpireTime))
		return
	}
	if order == nil {
		tradeNo := makeWechatPayTradeNo("WXT")
		order = model.NewWechatPayTestOrder(tradeNo, time.Now().Add(service.WechatPayTestOrderLifetime).Unix())
		if err = model.InsertWechatPayTestOrder(order); err != nil {
			wechatPayError(c, http.StatusInternalServerError, "创建测试订单失败")
			return
		}
	}
	prepayWechatPayTestOrder(c, gateway, order)
}

func GetWechatPayTestOrder(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	order, err := model.GetWechatPayTestOrder(tradeNo)
	if err != nil {
		wechatPayError(c, http.StatusNotFound, "测试订单不存在")
		return
	}
	if order.Status == common.TopUpStatusPending {
		reserved, reserveErr := model.TryReserveWechatPayTestQuery(order.TradeNo, common.GetTimestamp(), service.WechatPayQueryIntervalSeconds)
		if reserveErr == nil && reserved {
			if gateway, gatewayErr := service.NewWechatPayGateway(c.Request.Context(), false); gatewayErr == nil {
				_, _ = gateway.SyncTestOrder(c.Request.Context(), order)
			}
		}
		order, _ = model.GetWechatPayTestOrder(tradeNo)
	}
	common.ApiSuccess(c, wechatPayOrderData(order.TradeNo, order.CodeUrl, order.Status, order.PaymentAmountCents, order.ExpireTime))
}

func CreateWechatPayNativeOrder(c *gin.Context) {
	request := wechatPayNativeRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		wechatPayError(c, http.StatusBadRequest, "参数错误")
		return
	}
	if request.PaymentMethod != "" && request.PaymentMethod != model.PaymentMethodWechatNative {
		wechatPayError(c, http.StatusBadRequest, "支付方式不正确")
		return
	}
	gateway, err := service.NewWechatPayGateway(c.Request.Context(), true)
	if err != nil {
		wechatPayError(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	userId := c.GetInt("id")
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		wechatPayError(c, http.StatusInternalServerError, "获取用户分组失败")
		return
	}
	quote, err := buildDomesticTopUpQuote(request.Amount, request.AmountUnit, group)
	if err != nil {
		wechatPayError(c, http.StatusBadRequest, err.Error())
		return
	}
	paymentAmountCents := quote.PaymentAmountCents
	quotaToAdd := quote.QuotaToAdd
	now := common.GetTimestamp()
	if err = model.ExpireStaleWechatPayOrders(now); err != nil {
		wechatPayError(c, http.StatusInternalServerError, "清理过期微信支付订单失败")
		return
	}
	order, err := model.FindReusableWechatPayTopUp(userId, request.Amount, paymentAmountCents, quotaToAdd, now)
	if err != nil {
		wechatPayError(c, http.StatusInternalServerError, "读取待支付订单失败")
		return
	}
	if order != nil && order.CodeUrl != "" {
		common.ApiSuccess(c, wechatPayOrderData(order.TradeNo, order.CodeUrl, order.Status, order.PaymentAmountCents, order.ExpireTime))
		return
	}
	if order == nil {
		order = &model.TopUp{
			UserId: userId, Amount: request.Amount, AmountUnit: quote.AmountUnit,
			Money: float64(paymentAmountCents) / 100, PaymentAmountCents: paymentAmountCents, QuotaToAdd: quotaToAdd,
			Currency: service.WechatPayCurrency, TradeNo: makeWechatPayTradeNo("WXU"),
			PaymentMethod: model.PaymentMethodWechatNative, PaymentProvider: model.PaymentProviderWechatPay,
			CreateTime: now, ExpireTime: time.Now().Add(service.WechatPayOrderLifetime).Unix(), Status: common.TopUpStatusPending,
		}
		if err = order.Insert(); err != nil {
			wechatPayError(c, http.StatusInternalServerError, "创建微信支付订单失败")
			return
		}
	}
	codeUrl, err := gateway.Prepay(
		c.Request.Context(), order.TradeNo, order.PaymentAmountCents,
		truncateWechatPayDescription(common.SystemName+" 账户余额充值", 60), time.Unix(order.ExpireTime, 0),
	)
	if err != nil {
		transaction, queryErr := gateway.Query(c.Request.Context(), order.TradeNo)
		if queryErr == nil && transaction != nil && transaction.TradeState != nil {
			switch *transaction.TradeState {
			case "SUCCESS":
				if _, processErr := gateway.ProcessSuccessfulTransaction(transaction, "", c.ClientIP()); processErr == nil {
					common.ApiSuccess(c, wechatPayOrderData(order.TradeNo, "", common.TopUpStatusSuccess, order.PaymentAmountCents, order.ExpireTime))
					return
				} else {
					logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 Native 预下单后结算失败 user_id=%d trade_no=%s error=%q", userId, order.TradeNo, processErr.Error()))
				}
			case "NOTPAY":
				if closeErr := gateway.Close(c.Request.Context(), order.TradeNo); closeErr == nil {
					_ = model.UpdateWechatPayTopUpPendingStatus(order.TradeNo, common.TopUpStatusExpired)
				}
			}
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("微信支付 Native 预下单失败 user_id=%d trade_no=%s error=%q", userId, order.TradeNo, err.Error()))
		wechatPayError(c, http.StatusBadGateway, "微信支付预下单失败，请稍后重试")
		return
	}
	if err = model.SetWechatPayTopUpCodeUrl(order.TradeNo, codeUrl); err != nil {
		wechatPayError(c, http.StatusInternalServerError, "保存微信支付二维码失败")
		return
	}
	common.ApiSuccess(c, wechatPayOrderData(order.TradeNo, codeUrl, common.TopUpStatusPending, order.PaymentAmountCents, order.ExpireTime))
}

func GetWechatPayNativeOrder(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	order, err := model.GetWechatPayTopUpForUser(c.GetInt("id"), tradeNo)
	if err != nil {
		wechatPayError(c, http.StatusNotFound, "微信支付订单不存在")
		return
	}
	if order.Status == common.TopUpStatusPending {
		reserved, reserveErr := model.TryReserveWechatPayTopUpQuery(order.TradeNo, common.GetTimestamp(), service.WechatPayQueryIntervalSeconds)
		if reserveErr == nil && reserved {
			if gateway, gatewayErr := service.NewWechatPayGateway(c.Request.Context(), false); gatewayErr == nil {
				_, _ = gateway.SyncUserOrder(c.Request.Context(), order, c.ClientIP())
			}
		}
		order, _ = model.GetWechatPayTopUpForUser(c.GetInt("id"), tradeNo)
	}
	common.ApiSuccess(c, wechatPayOrderData(order.TradeNo, order.CodeUrl, order.Status, order.PaymentAmountCents, order.ExpireTime))
}

func WechatPayNotify(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, wechatPayNotifyBodyLimit)
	gateway, err := service.NewWechatPayGateway(c.Request.Context(), false)
	if err != nil {
		logger.LogError(c.Request.Context(), "微信支付 webhook 初始化失败: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "server unavailable"})
		return
	}
	transaction, notifyId, err := gateway.ParseNotify(c.Request.Context(), c.Request)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("微信支付 webhook 验签或解密失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		c.JSON(http.StatusBadRequest, gin.H{"code": "FAIL", "message": "invalid notification"})
		return
	}
	_, err = gateway.ProcessSuccessfulTransaction(transaction, notifyId, c.ClientIP())
	if err != nil {
		if errors.Is(err, model.ErrTopUpNotFound) || errors.Is(err, gorm.ErrRecordNotFound) {
			logger.LogWarn(c.Request.Context(), "微信支付 webhook 收到未知订单")
			c.Status(http.StatusNoContent)
			return
		}
		logger.LogError(c.Request.Context(), "微信支付 webhook 结算失败: "+err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"code": "FAIL", "message": "settlement failed"})
		return
	}
	c.Status(http.StatusNoContent)
}
