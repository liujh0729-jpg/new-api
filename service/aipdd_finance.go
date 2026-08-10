package service

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"strconv"
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

const aipddInstanceIDEnv = "AIPDD_INSTANCE_ID"

// PrepareAIPDDFinanceAttempt freezes identifiers after channel selection and before the upstream request.
func PrepareAIPDDFinanceAttempt(c *gin.Context, info *relaycommon.RelayInfo) error {
	if info == nil {
		return errors.New("relay info is required")
	}
	previousFinance := info.AIPDDFinance
	info.InitChannelMeta(c)
	if info.ChannelMeta == nil || info.ChannelType != constant.ChannelTypeAIPDD {
		if previousFinance != nil {
			// Closing a superseded attempt is best-effort: the reconciliation worker
			// re-settles stale orders, so a bookkeeping failure must not abort a relay
			// that is no longer routed to AIPDD.
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
	if info.ChannelIsMultiKey {
		// Mirrors the reconciliation worker: per-order finance requires one stable key
		// per channel, so multi-key channels relay without a finance order instead of failing.
		common.SysLog(fmt.Sprintf("skip AIPDD finance order for multi-key channel #%d; finance ownership requires one key per channel", info.ChannelId))
		info.AIPDDFinance = nil
		return nil
	}
	instanceID, err := resolveAIPDDFinanceInstanceID(info.ApiKey)
	if err != nil {
		return err
	}
	orderID := strings.TrimSpace(info.RequestId)
	if orderID == "" {
		return errors.New("request id is required for AIPDD finance order")
	}
	attemptID := fmt.Sprintf("%s:%d:%d", orderID, info.RetryIndex, info.ChannelId)
	finance := &relaycommon.AIPDDFinanceContext{
		InstanceID: instanceID, PlatformOrderID: orderID, AttemptID: attemptID,
		NewAPIUserID: strconv.Itoa(info.UserId), NewAPITokenID: strconv.Itoa(info.TokenId),
		ChannelID: info.ChannelId,
	}
	if err := model.EnsureAIPDDFinanceOrder(instanceID, orderID, attemptID,
		info.UserId, info.TokenId, info.ChannelId, info.OriginModelName); err != nil {
		return err
	}
	info.AIPDDFinance = finance
	wakeAIPDDFinanceReconciliation()
	return nil
}

func resolveAIPDDFinanceInstanceID(apiKey string) (string, error) {
	if configured := strings.TrimSpace(os.Getenv(aipddInstanceIDEnv)); configured != "" {
		if _, err := uuid.Parse(configured); err != nil {
			return "", fmt.Errorf("%s must be a UUID: %w", aipddInstanceIDEnv, err)
		}
		return configured, nil
	}
	key := strings.TrimSpace(apiKey)
	if key == "" {
		return "", errors.New("AIPDD API key is required to derive finance instance identity")
	}
	return uuid.NewHash(
		sha256.New(), uuid.NameSpaceURL, []byte("aipdd:new-api:finance-instance:"+key), 8,
	).String(), nil
}

func RecordAIPDDFinanceSettlement(info *relaycommon.RelayInfo, actualQuota int, status string) error {
	if info == nil || info.AIPDDFinance == nil {
		return nil
	}
	return recordAIPDDFinanceSettlement(info.AIPDDFinance, actualQuota, status)
}

func RecordTaskAIPDDFinanceSettlement(task *model.Task, actualQuota int, status string) error {
	if task == nil || task.PrivateData.AIPDDFinance == nil {
		return nil
	}
	return recordAIPDDFinanceSettlement(task.PrivateData.AIPDDFinance, actualQuota, status)
}

func BeginAIPDDFinanceSettlement(info *relaycommon.RelayInfo, actualQuota int) error {
	if info == nil || info.AIPDDFinance == nil {
		return nil
	}
	quota, rmbMic, _, err := aipddFinanceAmount(actualQuota)
	if err != nil {
		return err
	}
	finance := info.AIPDDFinance
	return model.BeginLocalAIPDDFinanceSettlement(
		finance.InstanceID, finance.PlatformOrderID, finance.ChannelID, quota, rmbMic)
}

func MarkAIPDDFinanceSettlementReviewRequired(info *relaycommon.RelayInfo) {
	if info == nil || info.AIPDDFinance == nil {
		return
	}
	finance := info.AIPDDFinance
	if err := model.MarkAIPDDFinanceSettlementReviewRequired(
		finance.InstanceID, finance.PlatformOrderID, finance.ChannelID); err != nil {
		common.SysLog("mark AIPDD finance settlement review failed: " + err.Error())
	}
}

func recordAIPDDFinanceSettlement(finance *relaycommon.AIPDDFinanceContext, actualQuota int, status string) error {
	quota, rmbMic, rateSnapshot, err := aipddFinanceAmount(actualQuota)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = model.RecordLocalAIPDDFinanceSettlement(
			finance.InstanceID, finance.PlatformOrderID, finance.ChannelID, quota, rmbMic, rateSnapshot, status)
		if err == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(time.Duration(50*(attempt+1)) * time.Millisecond)
		}
	}
	if err == nil {
		wakeAIPDDFinanceReconciliation()
	}
	return err
}

func aipddFinanceAmount(actualQuota int) (int64, int64, string, error) {
	quota := int64(max(0, actualQuota))
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	usdToRMB := decimal.NewFromFloat(operation_setting.USDExchangeRate)
	if quotaPerUnit.LessThanOrEqual(decimal.Zero) || usdToRMB.LessThanOrEqual(decimal.Zero) {
		return 0, 0, "", errors.New("invalid quota or currency conversion setting")
	}
	rmbMic := decimal.NewFromInt(quota).Div(quotaPerUnit).Mul(usdToRMB).
		Mul(decimal.NewFromInt(1_000_000)).Round(0).IntPart()
	snapshot, err := common.Marshal(map[string]string{
		"quota_per_unit": quotaPerUnit.String(), "usd_exchange_rate": usdToRMB.String(),
	})
	if err != nil {
		return 0, 0, "", err
	}
	return quota, rmbMic, string(snapshot), nil
}

func MarkAIPDDFinanceRefundPending(info *relaycommon.RelayInfo) {
	if info == nil || info.AIPDDFinance == nil {
		return
	}
	if err := model.MarkAIPDDFinanceRefundPending(info.AIPDDFinance.InstanceID, info.AIPDDFinance.PlatformOrderID, info.AIPDDFinance.ChannelID); err != nil {
		common.SysLog("mark AIPDD finance refund pending failed: " + err.Error())
	}
}
