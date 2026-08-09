package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/google/uuid"
)

const aipddFinanceWorkerInterval = 5 * time.Second

var aipddFinanceWake = make(chan struct{}, 1)

func StartAIPDDFinanceReconciliationTask() {
	if !common.IsMasterNode {
		return
	}
	gopool.Go(func() {
		runAIPDDFinanceReconciliation()
		ticker := time.NewTicker(aipddFinanceWorkerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
			case <-aipddFinanceWake:
			}
			runAIPDDFinanceReconciliation()
		}
	})
}

func wakeAIPDDFinanceReconciliation() {
	select {
	case aipddFinanceWake <- struct{}{}:
	default:
	}
}

func WakeAIPDDFinanceReconciliation() {
	wakeAIPDDFinanceReconciliation()
}

func runAIPDDFinanceReconciliation() {
	if err := model.MarkStaleAIPDDFinanceRefundsForReview(); err != nil {
		common.SysLog("mark stale AIPDD finance refunds failed: " + err.Error())
	}
	events, err := model.ClaimAIPDDFinanceOutbox(50)
	if err != nil {
		common.SysLog("claim AIPDD finance outbox failed: " + err.Error())
		return
	}
	for index := range events {
		if err = refreshAIPDDFinanceOrder(&events[index]); err != nil {
			_ = model.RetryAIPDDFinanceOutbox(&events[index], err)
		} else {
			_ = model.CompleteAIPDDFinanceOutbox(events[index].ID)
		}
	}
	channels, err := model.GetAIPDDChannelsForFinance()
	if err != nil {
		return
	}
	instanceID := strings.TrimSpace(os.Getenv(aipddInstanceIDEnv))
	if instanceID == "" {
		return
	}
	for index := range channels {
		if channels[index].ChannelInfo.IsMultiKey {
			common.SysLog(fmt.Sprintf("skip AIPDD finance pull for multi-key channel #%d; finance ownership requires one key per channel", channels[index].Id))
			continue
		}
		if err := pullAIPDDFinanceEvents(&channels[index], instanceID); err != nil {
			common.SysLog(fmt.Sprintf("pull AIPDD finance events for channel #%d failed: %s", channels[index].Id, err.Error()))
		}
	}
}

func refreshAIPDDFinanceOrder(event *model.AIPDDFinanceOutbox) error {
	channel, err := model.GetChannelById(event.ChannelID, true)
	if err != nil {
		return err
	}
	if channel.ChannelInfo.IsMultiKey {
		return errors.New("AIPDD finance channel must not use multi-key mode")
	}
	endpoint := aipddFinanceBaseURL(channel) + "/api/finance/v1/settlements/" + url.PathEscape(event.PlatformOrderID)
	body, err := doAIPDDFinanceGET(endpoint, channel.Key, event.InstanceID)
	if err != nil {
		return err
	}
	var envelope model.AIPDDSettlementEnvelope
	if err = common.Unmarshal(body, &envelope); err != nil {
		return err
	}
	if envelope.EventID == "" {
		envelope.EventID = uuid.NewSHA1(uuid.NameSpaceOID,
			[]byte(fmt.Sprintf("snapshot:%d:%s:%s:%d", channel.Id, event.InstanceID, event.PlatformOrderID, envelope.SettlementRevision))).String()
	}
	return model.ApplyAIPDDSettlementEnvelope(&envelope, body, channel.Id, nil)
}

func pullAIPDDFinanceEvents(channel *model.Channel, instanceID string) error {
	cursor, err := model.GetAIPDDFinanceCursor(channel.Id, instanceID)
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/api/finance/v1/settlement-events?after_sequence=%d&limit=200",
		aipddFinanceBaseURL(channel), cursor)
	body, err := doAIPDDFinanceGET(endpoint, channel.Key, instanceID)
	if err != nil {
		return err
	}
	var response struct {
		Events []json.RawMessage `json:"events"`
	}
	if err = common.Unmarshal(body, &response); err != nil {
		return err
	}
	for _, raw := range response.Events {
		var envelope model.AIPDDSettlementEnvelope
		if err = common.Unmarshal(raw, &envelope); err != nil {
			return err
		}
		if envelope.Sequence <= cursor {
			continue
		}
		sequence := envelope.Sequence
		if err = model.ApplyAIPDDSettlementEnvelope(&envelope, raw, channel.Id, &sequence); err != nil {
			return err
		}
		cursor = sequence
	}
	return nil
}

func doAIPDDFinanceGET(endpoint, apiKey, instanceID string) ([]byte, error) {
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
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("AIPDD finance endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func aipddFinanceBaseURL(channel *model.Channel) string {
	base := ""
	if channel != nil && channel.BaseURL != nil {
		base = strings.TrimSpace(*channel.BaseURL)
	}
	if base == "" {
		base = strings.TrimSpace(os.Getenv("AIPDD_BASE_URL"))
	}
	base = strings.TrimRight(base, "/")
	base = strings.TrimSuffix(base, "/v1")
	return base
}
