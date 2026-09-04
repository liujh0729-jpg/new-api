package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	aipddInstanceIDEnv     = "AIPDD_INSTANCE_ID"
	aipddFinanceEnabledEnv = "AIPDD_FINANCE_ENABLED"
)

// IsAIPDDFinanceEnabled reports whether NewAPI should create and settle AIPDD transit orders.
func IsAIPDDFinanceEnabled() bool {
	return common.GetEnvOrDefaultBool(aipddFinanceEnabledEnv, true)
}

// PrepareAIPDDFinanceAttempt freezes identifiers after channel selection and before the upstream request.
func PrepareAIPDDFinanceAttempt(c *gin.Context, info *relaycommon.RelayInfo) error {
	if info == nil {
		return errors.New("relay info is required")
	}
	previousFinance := info.AIPDDFinance
	info.InitChannelMeta(c)
	if !IsAIPDDFinanceEnabled() {
		// Keep channel meta for relay, but do not touch finance tables or block the request.
		info.AIPDDFinance = nil
		return nil
	}
	if info.ChannelMeta == nil || info.ChannelType != constant.ChannelTypeAIPDD {
		if previousFinance != nil {
			// Close the local pending record when retry routing leaves AIPDD.
			if err := recordAIPDDFinanceSettlement(previousFinance, 0, "NOT_CHARGED"); err != nil {
				common.SysError("close previous AIPDD finance attempt failed: " + err.Error())
			}
			info.AIPDDFinance = nil
		}
		return nil
	}
	if previousFinance != nil && previousFinance.ChannelID != info.ChannelId {
		if err := recordAIPDDFinanceSettlement(previousFinance, 0, "NOT_CHARGED"); err != nil {
			common.SysError("close previous AIPDD channel attempt failed: " + err.Error())
		}
		info.AIPDDFinance = nil
	}
	instanceID, err := ResolveAIPDDFinanceInstanceID()
	if err != nil {
		return err
	}
	orderID := strings.TrimSpace(info.RequestId)
	if orderID == "" {
		return errors.New("request id is required for AIPDD finance order")
	}
	attemptID := buildAIPDDFinanceAttemptID(orderID, info.RetryIndex, info.ChannelId)
	finance := &relaycommon.AIPDDFinanceContext{
		InstanceID: instanceID, PlatformOrderID: orderID, AttemptID: attemptID,
		NewAPIUserID: info.UserId, NewAPITokenID: info.TokenId,
		ChannelID: info.ChannelId, ChannelKeyIndex: info.ChannelMultiKeyIndex,
	}
	if err := model.EnsureAIPDDTransitOrder(instanceID, orderID, attemptID,
		info.UserId, info.TokenId, info.ChannelId, info.ChannelMultiKeyIndex, info.OriginModelName); err != nil {
		return err
	}
	info.AIPDDFinance = finance
	return nil
}

// ResolveAIPDDFinanceInstanceID returns the stable delivery-site identity
// registered with AIPDD. It is exported so administration surfaces can show
// the authoritative value instead of asking operators to enter it repeatedly.
func ResolveAIPDDFinanceInstanceID() (string, error) {
	if configured := strings.TrimSpace(os.Getenv(aipddInstanceIDEnv)); configured != "" {
		parsed, err := uuid.Parse(configured)
		if err != nil {
			return "", fmt.Errorf("%s must be a UUID: %w", aipddInstanceIDEnv, err)
		}
		return parsed.String(), nil
	}
	return "", fmt.Errorf(
		"%s is required and must match the registered delivery site's external instance ID",
		aipddInstanceIDEnv,
	)
}

func buildAIPDDFinanceAttemptID(orderID string, retryIndex, channelID int) string {
	return fmt.Sprintf("%s:%d:%d", orderID, max(0, retryIndex), channelID)
}

func RecordAIPDDFinanceSettlement(info *relaycommon.RelayInfo, actualQuota int, status string) error {
	if !IsAIPDDFinanceEnabled() || info == nil || info.AIPDDFinance == nil {
		return nil
	}
	return recordAIPDDFinanceSettlement(info.AIPDDFinance, actualQuota, status)
}

func RecordTaskAIPDDFinanceSettlement(task *model.Task, actualQuota int, status string) error {
	if !IsAIPDDFinanceEnabled() || task == nil || task.PrivateData.AIPDDFinance == nil {
		return nil
	}
	return recordAIPDDFinanceSettlement(task.PrivateData.AIPDDFinance, actualQuota, status)
}

func BeginAIPDDFinanceSettlement(info *relaycommon.RelayInfo, actualQuota int) error {
	return nil
}

func MarkAIPDDFinanceSettlementReviewRequired(info *relaycommon.RelayInfo) {
}

func recordAIPDDFinanceSettlement(finance *relaycommon.AIPDDFinanceContext, actualQuota int, status string) error {
	quota, rmbMic, err := aipddFinanceAmount(actualQuota)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = model.RecordAIPDDTransitLocalSettlement(finance.PlatformOrderID, quota, rmbMic, status)
		if err == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
		}
	}
	if err != nil || status != "CHARGED" {
		return err
	}
	return fetchAndApplyAIPDDTransitSettlement(finance)
}

func aipddFinanceAmount(actualQuota int) (int64, int64, error) {
	quota := int64(max(0, actualQuota))
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	usdToRMB := decimal.NewFromFloat(operation_setting.USDExchangeRate)
	if quotaPerUnit.LessThanOrEqual(decimal.Zero) || usdToRMB.LessThanOrEqual(decimal.Zero) {
		return 0, 0, errors.New("invalid quota or currency conversion setting")
	}
	rmbMic := decimal.NewFromInt(quota).Div(quotaPerUnit).Mul(usdToRMB).
		Mul(decimal.NewFromInt(1_000_000)).Round(0).IntPart()
	return quota, rmbMic, nil
}

func MarkAIPDDFinanceRefundPending(info *relaycommon.RelayInfo) {
	if !IsAIPDDFinanceEnabled() || info == nil || info.AIPDDFinance == nil {
		return
	}
	if err := model.RecordAIPDDTransitLocalSettlement(info.AIPDDFinance.PlatformOrderID, 0, 0, "REFUNDED"); err != nil {
		common.SysLog("mark AIPDD finance refund pending failed: " + err.Error())
	}
}
