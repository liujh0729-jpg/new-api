package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	epayEnabled := isEpayTopUpEnabled()
	wechatPayStatus := service.GetWechatPayConfigStatus()
	wechatPayEnabled := wechatPayStatus.Enabled && wechatPayStatus.Ready && wechatPayStatus.VerifiedAt > 0
	payMethods := make([]map[string]string, 0, len(operation_setting.PayMethods)+4)
	if epayEnabled || !wechatPayEnabled {
		for _, method := range operation_setting.PayMethods {
			if wechatPayEnabled && !wechatPayStatus.ShowEpayWechat && method["type"] == "wxpay" {
				continue
			}
			methodCopy := make(map[string]string, len(method))
			for key, value := range method {
				methodCopy[key] = value
			}
			methodCopy["amount_unit"] = getConfiguredDomesticTopUpAmountUnit()
			payMethods = append(payMethods, methodCopy)
		}
	}
	if wechatPayEnabled {
		payMethods = append(payMethods, map[string]string{
			"name": "微信支付（官方）", "type": model.PaymentMethodWechatNative,
			"min_topup": strconv.FormatInt(getMinTopup(), 10), "amount_unit": getConfiguredDomesticTopUpAmountUnit(),
		})
	}

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if isStripeTopUpEnabled() {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":        "Stripe",
				"type":        "stripe",
				"color":       "rgba(var(--semi-purple-5), 1)",
				"min_topup":   strconv.Itoa(setting.StripeMinTopUp),
				"amount_unit": model.TopUpAmountUnitProvider,
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	// 如果启用了 Waffo 支付，添加到支付方法列表
	enableWaffo := isWaffoTopUpEnabled()
	if enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffo {
				hasWaffo = true
				break
			}
		}

		if !hasWaffo {
			waffoMethod := map[string]string{
				"name":        "Waffo (Global Payment)",
				"type":        model.PaymentMethodWaffo,
				"color":       "rgba(var(--semi-blue-5), 1)",
				"min_topup":   strconv.Itoa(setting.WaffoMinTopUp),
				"amount_unit": model.TopUpAmountUnitProvider,
			}
			payMethods = append(payMethods, waffoMethod)
		}
	}

	enableWaffoPancake := isWaffoPancakeTopUpEnabled()
	if enableWaffoPancake {
		hasWaffoPancake := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffoPancake {
				hasWaffoPancake = true
				break
			}
		}

		if !hasWaffoPancake {
			payMethods = append(payMethods, map[string]string{
				"name":        "Waffo Pancake",
				"type":        model.PaymentMethodWaffoPancake,
				"color":       "rgba(var(--semi-orange-5), 1)",
				"min_topup":   strconv.Itoa(setting.WaffoPancakeMinTopUp),
				"amount_unit": model.TopUpAmountUnitProvider,
			})
		}
	}

	data := gin.H{
		"enable_online_topup":        epayEnabled || wechatPayEnabled,
		"enable_wechatpay_topup":     wechatPayEnabled,
		"enable_stripe_topup":        isStripeTopUpEnabled(),
		"enable_creem_topup":         isCreemTopUpEnabled(),
		"enable_waffo_topup":         enableWaffo,
		"enable_waffo_pancake_topup": enableWaffoPancake,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products":          setting.CreemProducts,
		"pay_methods":             payMethods,
		"min_topup":               operation_setting.MinTopUp,
		"stripe_min_topup":        setting.StripeMinTopUp,
		"waffo_min_topup":         setting.WaffoMinTopUp,
		"waffo_pancake_min_topup": setting.WaffoPancakeMinTopUp,
		"amount_options":          operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                operation_setting.GetPaymentSetting().AmountDiscount,
		"topup_link":              common.TopUpLink,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64   `json:"amount"`
	AmountUnit    *string `json:"amount_unit,omitempty"`
	PaymentMethod string  `json:"payment_method"`
}

type AmountRequest struct {
	Amount     int64   `json:"amount"`
	AmountUnit *string `json:"amount_unit,omitempty"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

type domesticTopUpQuote struct {
	AmountUnit         string
	CreditUSD          decimal.Decimal
	PaymentAmount      decimal.Decimal
	PaymentAmountCents int64
	QuotaToAdd         int64
}

func getConfiguredDomesticTopUpAmountUnit() string {
	switch operation_setting.GetQuotaDisplayType() {
	case operation_setting.QuotaDisplayTypeCNY:
		return model.TopUpAmountUnitCNY
	case operation_setting.QuotaDisplayTypeTokens:
		return model.TopUpAmountUnitTokens
	default:
		return model.TopUpAmountUnitUSD
	}
}

func resolveDomesticTopUpAmountUnit(requested *string) (string, error) {
	if requested == nil || strings.TrimSpace(*requested) == "" {
		// Backward compatibility: CNY used to be display-only, so legacy clients
		// still submit USD credit quantities. Token mode already used token units.
		if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
			return model.TopUpAmountUnitTokens, nil
		}
		return model.TopUpAmountUnitUSD, nil
	}

	unit := strings.ToUpper(strings.TrimSpace(*requested))
	switch unit {
	case model.TopUpAmountUnitUSD, model.TopUpAmountUnitCNY, model.TopUpAmountUnitTokens:
	default:
		return "", errors.New("不支持的充值单位")
	}
	if unit != getConfiguredDomesticTopUpAmountUnit() {
		return "", errors.New("充值单位已变化，请刷新页面")
	}
	return unit, nil
}

func getMinTopupForAmountUnit(unit string) int64 {
	minTopup := operation_setting.MinTopUp
	if unit == model.TopUpAmountUnitTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func getMinTopup() int64 {
	return getMinTopupForAmountUnit(getConfiguredDomesticTopUpAmountUnit())
}

func buildDomesticTopUpQuote(amount int64, requestedUnit *string, group string) (domesticTopUpQuote, error) {
	unit, err := resolveDomesticTopUpAmountUnit(requestedUnit)
	if err != nil {
		return domesticTopUpQuote{}, err
	}
	if amount < getMinTopupForAmountUnit(unit) {
		return domesticTopUpQuote{}, fmt.Errorf("充值数量不能小于 %d", getMinTopupForAmountUnit(unit))
	}
	if common.QuotaPerUnit <= 0 || operation_setting.Price <= 0 {
		return domesticTopUpQuote{}, errors.New("充值配置无效")
	}

	requestedAmount := decimal.NewFromInt(amount)
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	creditUSD := requestedAmount
	quota := requestedAmount.Mul(quotaPerUnit)

	switch unit {
	case model.TopUpAmountUnitCNY:
		if operation_setting.USDExchangeRate <= 0 {
			return domesticTopUpQuote{}, errors.New("人民币展示汇率无效")
		}
		creditUSD = requestedAmount.Div(decimal.NewFromFloat(operation_setting.USDExchangeRate))
		quota = creditUSD.Mul(quotaPerUnit)
	case model.TopUpAmountUnitTokens:
		creditUSD = requestedAmount.Div(quotaPerUnit)
		quota = requestedAmount
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	discount := 1.0
	if configured, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok && configured > 0 {
		discount = configured
	}

	paymentAmount := creditUSD.
		Mul(decimal.NewFromFloat(operation_setting.Price)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(discount)).
		Round(2)
	paymentCents := paymentAmount.Mul(decimal.NewFromInt(100))
	maxInt64 := int64(^uint64(0) >> 1)
	maxValue := decimal.NewFromInt(maxInt64)
	if paymentCents.LessThan(decimal.NewFromInt(1)) || paymentCents.GreaterThan(maxValue) {
		return domesticTopUpQuote{}, errors.New("充值金额过低或超出范围")
	}
	if quota.LessThan(decimal.NewFromInt(1)) || quota.GreaterThan(maxValue) {
		return domesticTopUpQuote{}, errors.New("充值额度无效")
	}

	return domesticTopUpQuote{
		AmountUnit:         unit,
		CreditUSD:          creditUSD,
		PaymentAmount:      paymentAmount,
		PaymentAmountCents: paymentCents.IntPart(),
		QuotaToAdd:         quota.IntPart(),
	}, nil
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	quote, err := buildDomesticTopUpQuote(req.Amount, req.AmountUnit, group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/log")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          quote.PaymentAmount.StringFixed(2),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 拉起支付失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	topUp := &model.TopUp{
		UserId:             id,
		Amount:             req.Amount,
		AmountUnit:         quote.AmountUnit,
		Money:              quote.PaymentAmount.InexactFloat64(),
		PaymentAmountCents: quote.PaymentAmountCents,
		QuotaToAdd:         quote.QuotaToAdd,
		Currency:           model.TopUpAmountUnitCNY,
		TradeNo:            tradeNo,
		PaymentMethod:      req.PaymentMethod,
		PaymentProvider:    model.PaymentProviderEpay,
		CreateTime:         time.Now().Unix(),
		Status:             common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 创建充值订单失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值订单创建成功 user_id=%d trade_no=%s payment_method=%s amount=%d amount_unit=%s quota_to_add=%d money=%.2f uri=%q params=%q", id, tradeNo, req.PaymentMethod, req.Amount, quote.AmountUnit, quote.QuotaToAdd, quote.PaymentAmount.InexactFloat64(), uri, common.GetJsonString(params)))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

func EpayNotify(c *gin.Context) {
	if !isEpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s params=%q", c.Request.RequestURI, c.ClientIP(), c.Request.Method, common.GetJsonString(params)))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	client := GetEpayClient()
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.RequestURI, c.ClientIP()))
		}
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签成功 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))

	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		money, parseErr := decimal.NewFromString(strings.TrimSpace(verifyInfo.Money))
		if parseErr != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调金额无效 trade_no=%s money=%q client_ip=%s error=%q", verifyInfo.ServiceTradeNo, verifyInfo.Money, c.ClientIP(), parseErr.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		paidCents := money.Round(2).Mul(decimal.NewFromInt(100)).IntPart()
		topUp, quotaToAdd, newlySettled, settleErr := model.SettleEpayTopUp(verifyInfo.ServiceTradeNo, verifyInfo.Type, paidCents)
		if settleErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 结算失败 trade_no=%s callback_type=%s money=%q client_ip=%s error=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.Money, c.ClientIP(), settleErr.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		if newlySettled {
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值成功 trade_no=%s user_id=%d client_ip=%s quota_to_add=%d money=%.2f topup=%q", topUp.TradeNo, topUp.UserId, c.ClientIP(), quotaToAdd, topUp.Money, common.GetJsonString(topUp)))
			model.RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(int(quotaToAdd)), topUp.Money), c.ClientIP(), topUp.PaymentMethod, "epay")
		}
	} else {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))
	}
	if _, writeErr := c.Writer.Write([]byte("success")); writeErr != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), writeErr.Error()))
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	quote, err := buildDomesticTopUpQuote(req.Amount, req.AmountUnit, group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": quote.PaymentAmount.StringFixed(2)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
