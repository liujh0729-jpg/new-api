package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/shopspring/decimal"
)

var errAIPDDTransitSettlementPending = errors.New("AIPDD transit settlement is pending")

type aipddTransitSettlementEnvelope struct {
	Code int `json:"code"`
	Data struct {
		PlatformOrderID string `json:"platform_order_id"`
		Settlement      struct {
			Status        string `json:"status"`
			ChargedPoints *int64 `json:"charged_points"`
			ChargedRMB    string `json:"charged_rmb"`
		} `json:"settlement"`
	} `json:"data"`
}

type aipddTransitSettlementPayload struct {
	Status        string `json:"status"`
	ChargedPoints *int64 `json:"charged_points"`
	ChargedRMB    string `json:"charged_rmb"`
}

// ApplyAIPDDTransitSettlementResponse consumes the settlement embedded by
// AIPDD in an asynchronous task's terminal response. Unknown or pending
// response shapes are intentionally ignored so normal task parsing is not
// coupled to accounting.
func ApplyAIPDDTransitSettlementResponse(
	finance *relaycommon.AIPDDFinanceContext,
	body []byte,
) error {
	if finance == nil || len(body) == 0 {
		return nil
	}
	var response struct {
		Settlement *aipddTransitSettlementPayload `json:"settlement"`
		Data       *struct {
			Settlement *aipddTransitSettlementPayload `json:"settlement"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return nil
	}
	settlement := response.Settlement
	if settlement == nil && response.Data != nil {
		settlement = response.Data.Settlement
	}
	if settlement == nil || !strings.EqualFold(strings.TrimSpace(settlement.Status), "settled") {
		return nil
	}
	return applyAIPDDTransitSettlementPayload(finance.PlatformOrderID, settlement)
}

func fetchAndApplyAIPDDTransitSettlement(finance *relaycommon.AIPDDFinanceContext) error {
	if finance == nil {
		return nil
	}
	if order, err := model.GetAIPDDTransitOrder(finance.PlatformOrderID); err == nil && order.Status == model.AIPDDTransitSettled {
		return nil
	}
	var lastErr error
	for attempt, delay := range []time.Duration{0, 150 * time.Millisecond, 400 * time.Millisecond, 900 * time.Millisecond} {
		if delay > 0 {
			time.Sleep(delay)
		}
		lastErr = fetchAIPDDTransitSettlement(finance)
		if lastErr == nil {
			return nil
		}
		if !errors.Is(lastErr, errAIPDDTransitSettlementPending) && attempt >= 1 {
			break
		}
	}
	return lastErr
}

func fetchAIPDDTransitSettlement(finance *relaycommon.AIPDDFinanceContext) error {
	channel, err := model.GetChannelById(finance.ChannelID, true)
	if err != nil {
		return err
	}
	apiKey, err := aipddTransitAPIKey(channel, finance)
	if err != nil {
		return err
	}
	endpoint := aipddTransitBaseURL(channel) + "/api/transit/v1/orders/" +
		url.PathEscape(finance.PlatformOrderID) + "/settlement"
	body, err := doAIPDDTransitGET(endpoint, apiKey, finance.InstanceID)
	if err != nil {
		return err
	}
	var envelope aipddTransitSettlementEnvelope
	if err = common.Unmarshal(body, &envelope); err != nil {
		return err
	}
	if envelope.Code != 0 {
		return fmt.Errorf("AIPDD transit settlement returned code %d", envelope.Code)
	}
	settlement := aipddTransitSettlementPayload{
		Status:        envelope.Data.Settlement.Status,
		ChargedPoints: envelope.Data.Settlement.ChargedPoints,
		ChargedRMB:    envelope.Data.Settlement.ChargedRMB,
	}
	if !strings.EqualFold(strings.TrimSpace(settlement.Status), "settled") {
		return errAIPDDTransitSettlementPending
	}
	return applyAIPDDTransitSettlementPayload(finance.PlatformOrderID, &settlement)
}

// SyncAIPDDTaskFinanceFromUpstream applies an embedded Seedance/AIPDD settlement
// and, once the task is terminal, fetches Java settlement if NewAPI is still PENDING.
// GET /v1/videos realtime fetch can persist SUCCESS before the background poller
// runs; without this, the poller skips billing and aipdd_transit_order stays PENDING.
func SyncAIPDDTaskFinanceFromUpstream(task *model.Task, responseBody []byte, taskResult *relaycommon.TaskInfo) {
	if task == nil || taskResult == nil || !IsAIPDDFinanceEnabled() {
		return
	}
	finance := task.PrivateData.AIPDDFinance
	if finance == nil {
		return
	}
	if err := ApplyAIPDDTransitSettlementResponse(finance, responseBody); err != nil {
		common.SysError("apply AIPDD realtime settlement failed: " + err.Error())
	}
	if taskResult.Status != model.TaskStatusSuccess && taskResult.Status != model.TaskStatusFailure {
		return
	}
	if order, err := model.GetAIPDDTransitOrder(finance.PlatformOrderID); err == nil && order.Status == model.AIPDDTransitSettled {
		return
	}
	status := "CHARGED"
	quota := task.Quota
	if taskResult.Status == model.TaskStatusFailure {
		status = "REFUNDED"
		quota = 0
	}
	if err := RecordTaskAIPDDFinanceSettlement(task, quota, status); err != nil {
		common.SysError("record AIPDD realtime settlement failed: " + err.Error())
	}
}

func applyAIPDDTransitSettlementPayload(platformOrderID string, settlement *aipddTransitSettlementPayload) error {
	if settlement == nil || settlement.ChargedPoints == nil || strings.TrimSpace(settlement.ChargedRMB) == "" {
		return errors.New("AIPDD transit settlement is missing charged amount")
	}
	rmb, err := decimal.NewFromString(strings.TrimSpace(settlement.ChargedRMB))
	if err != nil || rmb.IsNegative() {
		return errors.New("AIPDD transit settlement returned invalid charged_rmb")
	}
	rmbMic := rmb.Mul(decimal.NewFromInt(1_000_000)).Round(0).IntPart()
	return model.ApplyAIPDDTransitSourceSettlement(
		platformOrderID, max(0, *settlement.ChargedPoints), rmbMic)
}

func aipddTransitAPIKey(channel *model.Channel, finance *relaycommon.AIPDDFinanceContext) (string, error) {
	if channel == nil {
		return "", errors.New("AIPDD channel is required")
	}
	keys := channel.GetKeys()
	if finance.ChannelKeyIndex >= 0 && finance.ChannelKeyIndex < len(keys) {
		candidate := strings.TrimSpace(keys[finance.ChannelKeyIndex])
		if candidate != "" {
			return candidate, nil
		}
	}
	// Site-scoped keys are intentionally rotatable. If the original multi-key
	// slot disappeared, any remaining key for the same configured site can read
	// the order history identified by AIPDD_INSTANCE_ID.
	for _, candidate := range keys {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate, nil
		}
	}
	return "", errors.New("selected AIPDD channel key is no longer available")
}

func aipddTransitBaseURL(channel *model.Channel) string {
	base := ""
	if channel != nil && channel.BaseURL != nil {
		base = strings.TrimSpace(*channel.BaseURL)
	}
	if base == "" {
		base = strings.TrimSpace(os.Getenv("AIPDD_BASE_URL"))
	}
	if base == "" {
		base = constant.ChannelBaseURLs[constant.ChannelTypeAIPDD]
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	return strings.TrimSuffix(base, "/v1")
}

func doAIPDDTransitGET(endpoint, apiKey, instanceID string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("X-AIPDD-Instance-ID", instanceID)
	client := GetHttpClient()
	if client == nil {
		return nil, errors.New("HTTP client is not initialized")
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("AIPDD transit settlement returned HTTP %d", response.StatusCode)
	}
	return body, nil
}
