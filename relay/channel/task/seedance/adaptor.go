package seedance

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	taskdoubao "github.com/QuantumNous/new-api/relay/channel/task/doubao"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// TaskAdaptor deliberately reuses the already contract-tested Ark wire adapter.
// The independent Seedance channel owns orchestration, credentials and finance;
// the embedded adapter is only the thin public/Ark protocol translation layer.
type TaskAdaptor struct {
	taskdoubao.TaskAdaptor
	credentialErr error
	offering      *model.SeedanceModelOffering
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	cloned := *info
	key, _, err := model.ResolveSeedanceArkAPIKey(info.ChannelId)
	if err == nil {
		cloned.ApiKey = key
	}
	a.credentialErr = err
	a.TaskAdaptor.Init(&cloned)
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if a.credentialErr != nil {
		return service.TaskErrorWrapperLocal(a.credentialErr, "seedance_credential_unavailable", http.StatusServiceUnavailable)
	}
	config, err := model.GetSeedanceChannelConfig(info.ChannelId)
	if err != nil || config.Status != model.SeedanceConfigActive {
		if err == nil {
			err = model.ErrSeedanceChannelUnavailable
		}
		return service.TaskErrorWrapperLocal(err, "seedance_channel_unavailable", http.StatusServiceUnavailable)
	}
	offering, err := model.GetPublishedSeedanceOffering(info.ChannelId, info.OriginModelName)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "seedance_model_not_published", http.StatusBadRequest)
	}
	if offering.EnhancementModelID != nil {
		if _, err := model.GetMediaEnhancementProvider(offering.EnhancementProviderID); err != nil {
			return service.TaskErrorWrapperLocal(err, "seedance_model_unavailable", http.StatusServiceUnavailable)
		}
	}
	if taskErr := a.TaskAdaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	requestedFPS, explicitFPS, err := seedanceRequestedFPS(req.Metadata)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if explicitFPS && requestedFPS != offering.OutputFPS {
		return service.TaskErrorWrapperLocal(
			fmt.Errorf("output FPS is fixed at %d for model %s", offering.OutputFPS, offering.DisplayName),
			"seedance_output_fps_mismatch", http.StatusBadRequest,
		)
	}
	if operation_setting.USDExchangeRate <= 0 || common.QuotaPerUnit <= 0 {
		return service.TaskErrorWrapperLocal(model.ErrSeedanceInvalidPricing, "seedance_pricing_unavailable", http.StatusServiceUnavailable)
	}
	durationSeconds := seedanceRequestedDurationSeconds(req)
	hasReferenceVideo := seedanceHasReferenceVideo(req)
	info.RequestedDuration = common.GetPointer(durationSeconds)
	info.TaskPricingFacts = &relaycommon.TaskPricingFacts{
		Quantity: durationSeconds, Resolution: offering.SourceResolution, HasReferenceVideo: hasReferenceVideo,
	}
	info.TaskPricingQuote = buildSeedanceTaskPricingQuoteForRequest(offering, info, durationSeconds, hasReferenceVideo)
	a.offering = offering
	return nil
}

func buildSeedanceTaskPricingQuote(offering *model.SeedanceModelOffering, info *relaycommon.RelayInfo) *billing_setting.TaskPricingQuote {
	return buildSeedanceTaskPricingQuoteForRequest(offering, info, 1, false)
}

func buildSeedanceTaskPricingQuoteForRequest(offering *model.SeedanceModelOffering, info *relaycommon.RelayInfo, durationSeconds float64, hasReferenceVideo bool) *billing_setting.TaskPricingQuote {
	unitPriceMicroRMB := offering.NoReferenceUnitPriceMicroRMB
	if unitPriceMicroRMB == 0 && offering.ModelSaleMicroRMB > 0 {
		unitPriceMicroRMB = offering.ModelSaleMicroRMB
	}
	variant := "no_reference_video"
	if hasReferenceVideo {
		unitPriceMicroRMB = offering.ReferenceUnitPriceMicroRMB
		if unitPriceMicroRMB == 0 && offering.ModelSaleMicroRMB > 0 {
			unitPriceMicroRMB = offering.ModelSaleMicroRMB
		}
		variant = "reference_video"
	}
	saleCNY := float64(unitPriceMicroRMB) / 1_000_000
	saleUSD := saleCNY / operation_setting.USDExchangeRate
	groupRatio, _ := ratio_setting.ResolveSeedanceGroupRatio(info.UserGroup, info.UsingGroup)
	baseUSD := saleUSD * durationSeconds
	finalSaleUSD := baseUSD * groupRatio
	quote := billing_setting.TaskPricingQuote{
		Unit:              billing_setting.TaskPricingUnitSecond,
		Variant:           offering.PricingVersion + ":" + variant,
		UnitPriceUSD:      saleUSD,
		Quantity:          durationSeconds,
		GroupRatio:        groupRatio,
		BaseUSD:           baseUSD,
		SaleUSD:           finalSaleUSD,
		Quota:             int(math.Round(finalSaleUSD * common.QuotaPerUnit)),
		HasReferenceVideo: hasReferenceVideo,
		Resolution:        offering.SourceResolution,
	}
	quote = billing_setting.ApplyMembershipToTaskPricingQuote(quote, info.MembershipRatioInfo, info.OriginModelName, common.QuotaPerUnit)
	return &quote
}

func seedanceRequestedDurationSeconds(req relaycommon.TaskSubmitReq) float64 {
	if req.Duration > 0 {
		return float64(req.Duration)
	}
	if value, err := strconv.ParseFloat(strings.TrimSpace(req.Seconds), 64); err == nil && value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
		return value
	}
	for _, key := range []string{"duration", "seconds"} {
		if value, ok := seedancePositiveNumber(req.Metadata[key]); ok {
			return value
		}
	}
	return 5
}

func seedancePositiveNumber(value any) (float64, bool) {
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		if err != nil {
			return 0, false
		}
		number = parsed
	default:
		return 0, false
	}
	return number, number > 0 && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func seedanceRequestedFPS(metadata map[string]any) (int, bool, error) {
	for _, key := range []string{"framespersecond", "frames_per_second", "fps"} {
		value, exists := metadata[key]
		if !exists || value == nil {
			continue
		}
		number, ok := seedancePositiveNumber(value)
		if !ok || number != math.Trunc(number) || number > 240 {
			return 0, true, fmt.Errorf("%s must be an integer from 1 to 240", key)
		}
		return int(number), true, nil
	}
	return 0, false, nil
}

func seedanceHasReferenceVideo(req relaycommon.TaskSubmitReq) bool {
	if strings.TrimSpace(req.InputReference) != "" {
		return true
	}
	content, ok := req.Metadata["content"].([]any)
	if !ok {
		return false
	}
	for _, item := range content {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(entry["type"])), "video_url") {
			return true
		}
		if value, exists := entry["video_url"]; exists && value != nil {
			return true
		}
	}
	return false
}

// BuildRequestBody captures the customer callback before forwarding the
// request to Ark. Ark finishing is not the public workflow finishing point for
// this channel, so forwarding callback_url would notify customers too early.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	body, err := a.TaskAdaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if callback, ok := payload["callback_url"].(string); ok {
		info.SeedanceCallbackURL = strings.TrimSpace(callback)
	}
	delete(payload, "callback_url")
	if a.offering != nil {
		payload["resolution"] = a.offering.SourceResolution
		payload["framespersecond"] = a.offering.OutputFPS
		delete(payload, "fps")
		delete(payload, "frames_per_second")
	}
	data, err = common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoResponse deliberately does not write the public response. The generic
// task controller persists the task, private order and first attempt in one
// transaction before it acknowledges an independently managed Seedance task.
func (a *TaskAdaptor) DoResponse(_ *gin.Context, resp *http.Response, _ *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()
	var upstream struct {
		ID string `json:"id"`
	}
	if err := common.Unmarshal(responseBody, &upstream); err != nil {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("decode Seedance create response: %w", err), "unmarshal_response_body_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(upstream.ID) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
	}
	return upstream.ID, responseBody, nil
}

func (a *TaskAdaptor) GetChannelName() string {
	return "seedance"
}
