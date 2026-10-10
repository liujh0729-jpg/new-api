package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
// Upstream compute must stay reachable even when the accounting preconditions are not met, so every
// finance-only failure degrades to "no local order for this attempt" instead of returning an error.
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
			if err := recordAIPDDFinanceSettlement(previousFinance, 0, "NOT_CHARGED", AIPDDCostBasis{}); err != nil {
				common.SysError("close previous AIPDD finance attempt failed: " + err.Error())
			}
			info.AIPDDFinance = nil
		}
		return nil
	}
	if previousFinance != nil && previousFinance.ChannelID != info.ChannelId {
		if err := recordAIPDDFinanceSettlement(previousFinance, 0, "NOT_CHARGED", AIPDDCostBasis{}); err != nil {
			common.SysError("close previous AIPDD channel attempt failed: " + err.Error())
		}
		info.AIPDDFinance = nil
	}
	orderID := strings.TrimSpace(info.RequestId)
	if orderID == "" {
		warnAIPDDFinanceDisabled("AIPDD finance skipped: request id is missing")
		return nil
	}
	instanceID, err := resolveAIPDDFinanceInstanceID()
	if err != nil {
		// A malformed identity degrades to "no site identity" instead of dropping
		// the ledger too: the customer charge and cost estimate are still recorded.
		warnAIPDDFinanceDisabled("AIPDD source settlement skipped: " + err.Error())
		instanceID = ""
	}
	if instanceID == "" {
		warnAIPDDFinanceDisabled("AIPDD source settlement skipped: " + aipddInstanceIDEnv +
			" is not configured; local orders and cost estimates are still recorded")
	}
	attemptID := buildAIPDDFinanceAttemptID(orderID, info.RetryIndex, info.ChannelId)
	finance := &relaycommon.AIPDDFinanceContext{
		InstanceID: instanceID, PlatformOrderID: orderID, AttemptID: attemptID,
		NewAPIUserID: info.UserId, NewAPITokenID: info.TokenId,
		ChannelID: info.ChannelId, ChannelKeyIndex: info.ChannelMultiKeyIndex,
	}
	if err := model.EnsureAIPDDTransitOrder(instanceID, orderID, attemptID,
		info.UserId, info.TokenId, info.ChannelId, info.ChannelMultiKeyIndex, info.OriginModelName); err != nil {
		warnAIPDDFinanceDisabled("create AIPDD transit order failed: " + err.Error())
		return nil
	}
	info.AIPDDFinance = finance
	return nil
}

var aipddFinanceWarnings sync.Map

// warnAIPDDFinanceDisabled logs each distinct accounting degradation once per process so a
// misconfigured deployment cannot flood the log with one line per relay request.
func warnAIPDDFinanceDisabled(message string) {
	if _, loaded := aipddFinanceWarnings.LoadOrStore(message, struct{}{}); loaded {
		return
	}
	common.SysLog(message + " (relay is unaffected; logged once)")
}

func resolveAIPDDFinanceInstanceID() (string, error) {
	configured := strings.TrimSpace(os.Getenv(aipddInstanceIDEnv))
	if configured == "" {
		// Unconfigured is a supported steady state, not an error: NewAPI keeps its
		// own ledger and cost estimates, it just cannot ask AIPDD for the settled
		// amount, and it must not send an unregistered identity upstream.
		return "", nil
	}
	parsed, err := uuid.Parse(configured)
	if err != nil {
		return "", fmt.Errorf("%s must be a UUID: %w", aipddInstanceIDEnv, err)
	}
	return parsed.String(), nil
}

func buildAIPDDFinanceAttemptID(orderID string, retryIndex, channelID int) string {
	return fmt.Sprintf("%s:%d:%d", orderID, max(0, retryIndex), channelID)
}

func RecordAIPDDFinanceSettlement(info *relaycommon.RelayInfo, actualQuota int, status string) error {
	if !IsAIPDDFinanceEnabled() || info == nil || info.AIPDDFinance == nil {
		return nil
	}
	// Only a charged request carries cost evidence worth reading; refund and
	// not-charged paths must not touch relay state that may be absent.
	var costBasis AIPDDCostBasis
	if appliesAIPDDCostEstimate(status, actualQuota) {
		costBasis = aipddRelayCostBasis(info, actualQuota)
	}
	return recordAIPDDFinanceSettlement(info.AIPDDFinance, actualQuota, status, costBasis)
}

func RecordTaskAIPDDFinanceSettlement(task *model.Task, actualQuota int, status string) error {
	if !IsAIPDDFinanceEnabled() || task == nil || task.PrivateData.AIPDDFinance == nil {
		return nil
	}
	var costBasis AIPDDCostBasis
	if appliesAIPDDCostEstimate(status, actualQuota) {
		costBasis = aipddTaskCostBasis(task, actualQuota)
	}
	return recordAIPDDFinanceSettlement(task.PrivateData.AIPDDFinance, actualQuota, status, costBasis)
}

func BeginAIPDDFinanceSettlement(info *relaycommon.RelayInfo, actualQuota int) error {
	return nil
}

func MarkAIPDDFinanceSettlementReviewRequired(info *relaycommon.RelayInfo) {
}

func recordAIPDDFinanceSettlement(
	finance *relaycommon.AIPDDFinanceContext,
	actualQuota int,
	status string,
	costBasis AIPDDCostBasis,
) error {
	quota, rmbMic, err := aipddFinanceAmount(actualQuota)
	if err != nil {
		return err
	}
	var estimatedAWCoin, estimatedRMBMic *int64
	if appliesAIPDDCostEstimate(status, actualQuota) {
		if estimate, ok := EstimateAIPDDSourceCost(costBasis); ok {
			estimatedAWCoin = &estimate.AWCoin
			estimatedRMBMic = &estimate.RMBMic
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = model.RecordAIPDDTransitLocalSettlement(
			finance.PlatformOrderID, quota, rmbMic, status, estimatedAWCoin, estimatedRMBMic)
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
	if err := model.RecordAIPDDTransitLocalSettlement(info.AIPDDFinance.PlatformOrderID, 0, 0, "REFUNDED", nil, nil); err != nil {
		common.SysLog("mark AIPDD finance refund pending failed: " + err.Error())
	}
}
