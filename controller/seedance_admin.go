package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type seedanceChannelConfigRequest struct {
	InstanceID                       string   `json:"instance_id"`
	AIPDDBillingBaseURL              string   `json:"aipdd_billing_base_url"`
	AIPDDBillingAPIKey               string   `json:"aipdd_billing_api_key"`
	VolcengineBillSyncEnabled        bool     `json:"volcengine_bill_sync_enabled"`
	VolcengineBillProductCodes       []string `json:"volcengine_bill_product_codes"`
	VolcengineBillConfigurationCodes []string `json:"volcengine_bill_configuration_codes"`
	DefaultEnhancementProviderID     *int64   `json:"default_enhancement_provider_id"`
	Status                           string   `json:"status"`
}

type seedanceCredentialRequest struct {
	ArkAPIKey       string `json:"ark_api_key"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
}

type seedanceProviderRequest struct {
	ID              int64  `json:"id"`
	ProviderType    string `json:"provider_type"`
	AdapterType     string `json:"adapter_type"`
	DisplayName     string `json:"display_name"`
	ServiceEndpoint string `json:"service_endpoint"`
	Credential      string `json:"credential"`
	// MediaKitAPIKey is the AI MediaKit key an administrator types into the
	// direct-connect form. It is stored in the provider's own encrypted
	// credential column and never shares a field with the Ark API key.
	MediaKitAPIKey string `json:"mediakit_api_key"`
	// ClearCredential erases the stored secret. A blank credential field means
	// "keep the current value", so removal needs an explicit flag.
	ClearCredential    bool   `json:"clear_credential"`
	ServiceCode        string `json:"service_code"`
	CapabilitiesJSON   string `json:"capabilities"`
	Status             string `json:"status"`
	TimeoutPolicyJSON  string `json:"timeout_policy"`
	RetryPolicyJSON    string `json:"retry_policy"`
	FallbackPolicyJSON string `json:"fallback_policy"`
}

// seedanceProviderView is the only provider shape the admin API returns. It
// exists so no code path can accidentally serialize the ciphertext or any value
// from which the stored API key could be recovered.
type seedanceProviderView struct {
	ID                   int64  `json:"id"`
	Version              int    `json:"version"`
	ProviderType         string `json:"provider_type"`
	AdapterType          string `json:"adapter_type"`
	DisplayName          string `json:"display_name"`
	ServiceEndpoint      string `json:"service_endpoint"`
	ServiceCode          string `json:"service_code"`
	CapabilitiesJSON     string `json:"capabilities"`
	Status               string `json:"status"`
	TimeoutPolicyJSON    string `json:"timeout_policy"`
	RetryPolicyJSON      string `json:"retry_policy"`
	FallbackPolicyJSON   string `json:"fallback_policy"`
	CredentialConfigured bool   `json:"credential_configured"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`
}

func newSeedanceProviderView(item model.MediaEnhancementProvider) seedanceProviderView {
	adapterType := strings.TrimSpace(item.AdapterType)
	if adapterType == "" {
		adapterType = model.LegacySeedanceAdapterType(item.ProviderType)
	}
	return seedanceProviderView{
		ID: item.ID, Version: item.Version, ProviderType: item.ProviderType, AdapterType: adapterType,
		DisplayName: item.DisplayName, ServiceEndpoint: item.ServiceEndpoint, ServiceCode: item.ServiceCode,
		CapabilitiesJSON: item.CapabilitiesJSON, Status: item.Status,
		TimeoutPolicyJSON: item.TimeoutPolicyJSON, RetryPolicyJSON: item.RetryPolicyJSON,
		FallbackPolicyJSON:   item.FallbackPolicyJSON,
		CredentialConfigured: strings.TrimSpace(item.CredentialEncrypted) != "",
		CreatedAt:            item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func newSeedanceProviderViews(items []model.MediaEnhancementProvider) []seedanceProviderView {
	views := make([]seedanceProviderView, 0, len(items))
	for _, item := range items {
		views = append(views, newSeedanceProviderView(item))
	}
	return views
}

type seedanceOfferingRequest struct {
	ID                              int64  `json:"id"`
	ChannelID                       int    `json:"channel_id"`
	DisplayName                     string `json:"display_name"`
	BaseModelID                     int64  `json:"base_model_id"`
	EnhancementModelID              *int64 `json:"enhancement_model_id"`
	SourceResolution                string `json:"source_resolution"`
	TargetResolution                string `json:"target_resolution"`
	OutputFPS                       int    `json:"output_fps"`
	NoReferenceUnitPriceMicroRMB    int64  `json:"no_reference_unit_price_micro_rmb"`
	ReferenceUnitPriceMicroRMB      int64  `json:"reference_unit_price_micro_rmb"`
	ProviderModelID                 string `json:"provider_model_id"`
	ResolutionRulesJSON             string `json:"resolution_rules"`
	DurationRulesJSON               string `json:"duration_rules"`
	EnhancementProviderID           int64  `json:"enhancement_provider_id"`
	EnhancementServiceCode          string `json:"enhancement_service_code"`
	EnhancementSpecificationJSON    string `json:"enhancement_specification"`
	EnhancementSpecificationVersion string `json:"enhancement_specification_version"`
	ModelSaleMicroRMB               int64  `json:"model_sale_micro_rmb"`
	ServiceChargeMicroRMB           int64  `json:"service_charge_micro_rmb"`
	ProviderCostMicroRMB            *int64 `json:"provider_cost_micro_rmb"`
	VolcengineUnitCostMicroRMB      int64  `json:"volcengine_unit_cost_micro_rmb"`
	PricingVersion                  string `json:"pricing_version"`
	Enabled                         bool   `json:"enabled"`
}

type seedanceOrderView struct {
	model.SeedanceOrder
	EnhancementStatus        string `json:"enhancement_status,omitempty"`
	EnhancementFailureReason string `json:"enhancement_failure_reason,omitempty"`
}

func newSeedanceOrderViews(items []model.SeedanceOrder) ([]seedanceOrderView, error) {
	views := make([]seedanceOrderView, 0, len(items))
	orderIDs := make([]string, 0, len(items))
	for i := range items {
		orderIDs = append(orderIDs, items[i].PlatformOrderID)
	}
	latestUsage := make(map[string]model.MediaServiceUsage, len(items))
	if len(orderIDs) > 0 {
		var usages []model.MediaServiceUsage
		if err := model.DB.Where("platform_order_id IN ?", orderIDs).Order("id desc").Find(&usages).Error; err != nil {
			return nil, err
		}
		for i := range usages {
			if _, exists := latestUsage[usages[i].PlatformOrderID]; !exists {
				latestUsage[usages[i].PlatformOrderID] = usages[i]
			}
		}
	}
	for i := range items {
		view := seedanceOrderView{SeedanceOrder: items[i]}
		if usage, ok := latestUsage[items[i].PlatformOrderID]; ok {
			view.EnhancementStatus = usage.Status
			view.EnhancementFailureReason = usage.FailureReason
			if usage.FailureReason == model.SeedanceSubmissionOutcomeUnknown {
				view.EnhancementStatus = model.SeedanceSubmissionOutcomeUnknown
			}
		}
		views = append(views, view)
	}
	return views, nil
}

type seedanceVolcengineBillImportRequest struct {
	ChannelID      int                           `json:"channel_id"`
	BillDetailID   string                        `json:"bill_detail_id"`
	Revision       int                           `json:"revision"`
	BillingPeriod  string                        `json:"billing_period"`
	ProductCode    string                        `json:"product_code"`
	CostCategory   string                        `json:"cost_category"`
	InstanceID     string                        `json:"instance_id"`
	UsageStartedAt int64                         `json:"usage_started_at"`
	UsageEndedAt   int64                         `json:"usage_ended_at"`
	AmountMicroRMB int64                         `json:"amount_micro_rmb"`
	Source         map[string]any                `json:"source"`
	Candidates     []model.SeedanceCostCandidate `json:"candidates"`
}

type seedanceVolcengineBillReconcileRequest struct {
	Candidates []model.SeedanceCostCandidate `json:"candidates"`
}

type seedanceTaskCountMetric struct {
	Status string `json:"status"`
	Model  string `json:"model"`
	Value  int64  `json:"value"`
}

type seedanceLatencyMetric struct {
	Model        string  `json:"model,omitempty"`
	ProviderType string  `json:"provider_type,omitempty"`
	ServiceCode  string  `json:"service_code,omitempty"`
	Count        int64   `json:"count"`
	Average      float64 `json:"average_seconds"`
	Maximum      int64   `json:"maximum_seconds"`
}

type seedanceFailureMetric struct {
	ProviderType string `json:"provider_type"`
	ServiceCode  string `json:"service_code"`
	Reason       string `json:"reason"`
	Value        int64  `json:"value"`
}

type seedanceUnknownSubmissionMetric struct {
	ProviderType string `json:"provider_type"`
	Value        int64  `json:"value"`
}

type seedanceStatusFailureMetric struct {
	StatusCode int   `json:"status_code"`
	Value      int64 `json:"value"`
}

func GetSeedanceAdminOverview(c *gin.Context) {
	channelID, err := strconv.Atoi(strings.TrimSpace(c.Query("channel_id")))
	if err != nil || channelID <= 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("valid channel_id is required"))
		return
	}
	config, configErr := model.GetSeedanceChannelConfig(channelID)
	if configErr != nil && !errors.Is(configErr, gorm.ErrRecordNotFound) {
		seedanceAdminFailure(c, http.StatusInternalServerError, configErr)
		return
	}
	credentials, err := model.ListSeedanceCredentials(channelID)
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	providers, err := model.ListMediaEnhancementProviders()
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	offerings, err := model.ListSeedanceModelOfferings(channelID)
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	cursors, err := model.ListSeedanceVolcengineBillCursors(channelID)
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	siteInstanceID, siteInstanceErr := service.ResolveAIPDDFinanceInstanceID()
	common.ApiSuccess(c, gin.H{
		"config":                        config,
		"configured":                    config != nil,
		"crypto_ready":                  common.HasStableCryptoSecret(),
		"billing_credential_configured": config != nil && config.AIPDDBillingCredentialEncrypted != "",
		"site_instance_id":              siteInstanceID,
		"site_instance_id_configured":   siteInstanceErr == nil,
		"credentials":                   credentials,
		"providers":                     newSeedanceProviderViews(providers),
		"offerings":                     offerings,
		"bill_cursors":                  cursors,
	})
}

// GetSeedanceAdminMetrics exposes the design's named operational metrics only
// behind administrator authentication. It intentionally returns no task input,
// secret, endpoint, external task ID or result URL.
func GetSeedanceAdminMetrics(c *gin.Context) {
	channelID, err := strconv.Atoi(strings.TrimSpace(c.Query("channel_id")))
	if err != nil || channelID <= 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("valid channel_id is required"))
		return
	}
	if err := ensureSeedanceChannel(channelID); err != nil {
		seedanceAdminFailure(c, http.StatusNotFound, err)
		return
	}
	var taskCounts []seedanceTaskCountMetric
	if err := model.DB.Model(&model.SeedanceOrder{}).
		Select("order_status AS status, model, COUNT(*) AS value").
		Where("channel_id = ?", channelID).Group("order_status, model").Scan(&taskCounts).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	var generationLatency []seedanceLatencyMetric
	if err := model.DB.Model(&model.SeedanceOrder{}).
		Select("model, COUNT(*) AS count, AVG(generation_completed_at - generation_started_at) AS average, MAX(generation_completed_at - generation_started_at) AS maximum").
		Where("channel_id = ? AND generation_completed_at > generation_started_at", channelID).
		Group("model").Scan(&generationLatency).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	usageScope := func() *gorm.DB {
		return model.DB.Table("media_service_usages").
			Joins("JOIN seedance_orders ON seedance_orders.platform_order_id = media_service_usages.platform_order_id").
			Where("seedance_orders.channel_id = ?", channelID)
	}
	var enhancementLatency []seedanceLatencyMetric
	if err := usageScope().
		Select("media_service_usages.provider_type, media_service_usages.service_code, COUNT(*) AS count, AVG(media_service_usages.completed_at - media_service_usages.started_at) AS average, MAX(media_service_usages.completed_at - media_service_usages.started_at) AS maximum").
		Where("media_service_usages.completed_at > media_service_usages.started_at").
		Group("media_service_usages.provider_type, media_service_usages.service_code").Scan(&enhancementLatency).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	var enhancementFailures []seedanceFailureMetric
	if err := usageScope().
		Select("media_service_usages.provider_type, media_service_usages.service_code, COALESCE(NULLIF(media_service_usages.failure_reason, ''), 'UNCLASSIFIED') AS reason, COUNT(*) AS value").
		Where("media_service_usages.status = ?", model.SeedanceUsageFailed).
		Group("media_service_usages.provider_type, media_service_usages.service_code, COALESCE(NULLIF(media_service_usages.failure_reason, ''), 'UNCLASSIFIED')").Scan(&enhancementFailures).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	var unknownSubmissions []seedanceUnknownSubmissionMetric
	if err := usageScope().
		Select("media_service_usages.provider_type, SUM(media_service_usages.unknown_submission_count) AS value").
		Where("media_service_usages.unknown_submission_count > 0").
		Group("media_service_usages.provider_type").Scan(&unknownSubmissions).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	outboxScope := func() *gorm.DB {
		return model.DB.Table("service_billing_outboxes").
			Joins("JOIN service_billing_events ON service_billing_events.event_id = service_billing_outboxes.event_id").
			Joins("JOIN seedance_orders ON seedance_orders.platform_order_id = service_billing_events.platform_order_id").
			Where("seedance_orders.channel_id = ?", channelID)
	}
	var outboxPending int64
	if err := outboxScope().Where("service_billing_outboxes.status <> ?", model.SeedanceSyncSynced).Count(&outboxPending).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	var oldestCreatedAt int64
	if outboxPending > 0 {
		if err := outboxScope().Where("service_billing_outboxes.status <> ?", model.SeedanceSyncSynced).
			Select("COALESCE(MIN(service_billing_outboxes.created_at), 0)").Scan(&oldestCreatedAt).Error; err != nil {
			seedanceAdminFailure(c, http.StatusInternalServerError, err)
			return
		}
	}
	var syncFailures []seedanceStatusFailureMetric
	if err := model.DB.Table("service_billing_failure_attempts").
		Joins("JOIN service_billing_events ON service_billing_events.event_id = service_billing_failure_attempts.event_id").
		Joins("JOIN seedance_orders ON seedance_orders.platform_order_id = service_billing_events.platform_order_id").
		Select("service_billing_failure_attempts.http_status AS status_code, COUNT(*) AS value").
		Where("seedance_orders.channel_id = ?", channelID).
		Group("service_billing_failure_attempts.http_status").Scan(&syncFailures).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	var costPending int64
	if err := model.DB.Model(&model.SeedanceOrder{}).Where("channel_id = ? AND volcengine_cost_status <> ?", channelID, model.SeedanceCostConfirmed).Count(&costPending).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	var reconciliationRequired int64
	if err := model.DB.Model(&model.SeedanceCostReconciliationIssue{}).Where("channel_id = ? AND status = ?", channelID, model.SeedanceReconciliationOpen).Count(&reconciliationRequired).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	var arrears int64
	if err := outboxScope().Where("service_billing_outboxes.status = ? AND service_billing_outboxes.response LIKE ?", model.SeedanceSyncSynced, "%ARREARS%").
		Distinct("service_billing_events.platform_order_id").Count(&arrears).Error; err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	oldestAge := int64(0)
	if oldestCreatedAt > 0 {
		oldestAge = max(int64(0), time.Now().Unix()-oldestCreatedAt)
	}
	common.ApiSuccess(c, gin.H{
		"seedance_tasks_total":                             taskCounts,
		"seedance_generation_latency_seconds":              generationLatency,
		"media_enhancement_latency_seconds":                enhancementLatency,
		"media_enhancement_failures_total":                 enhancementFailures,
		"media_enhancement_unknown_submissions_total":      unknownSubmissions,
		"seedance_billing_outbox_pending":                  outboxPending,
		"seedance_billing_outbox_oldest_age_seconds":       oldestAge,
		"seedance_billing_sync_failures_total":             syncFailures,
		"seedance_volcengine_cost_pending":                 costPending,
		"seedance_volcengine_cost_reconciliation_required": reconciliationRequired,
		"seedance_aipdd_arrears_total":                     arrears,
	})
}

func UpdateSeedanceChannelConfig(c *gin.Context) {
	channelID, ok := seedancePathID(c, "channel_id")
	if !ok {
		return
	}
	if err := ensureSeedanceChannel(channelID); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	var request seedanceChannelConfigRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	request.InstanceID = strings.TrimSpace(request.InstanceID)
	if request.InstanceID == "" {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("instance_id is required"))
		return
	}
	if _, err := uuid.Parse(request.InstanceID); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("instance_id must be a UUID"))
		return
	}
	if request.AIPDDBillingBaseURL != "" && !isTrustedServiceEndpoint(request.AIPDDBillingBaseURL) {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("billing endpoint must use HTTPS"))
		return
	}
	status := strings.ToUpper(strings.TrimSpace(request.Status))
	if status == "" {
		status = model.SeedanceConfigDisabled
	}
	if status != model.SeedanceConfigActive && status != model.SeedanceConfigDisabled {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("invalid config status"))
		return
	}
	productCodes := make([]string, 0, len(request.VolcengineBillProductCodes))
	seenProductCodes := make(map[string]struct{}, len(request.VolcengineBillProductCodes))
	for _, productCode := range request.VolcengineBillProductCodes {
		productCode = strings.TrimSpace(productCode)
		if productCode == "" {
			continue
		}
		if len(productCode) > 128 {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("Volcengine bill product code is too long"))
			return
		}
		if _, exists := seenProductCodes[productCode]; exists {
			continue
		}
		seenProductCodes[productCode] = struct{}{}
		productCodes = append(productCodes, productCode)
	}
	if request.VolcengineBillSyncEnabled && len(productCodes) == 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("at least one verified Volcengine bill product code is required when bill sync is enabled"))
		return
	}
	productCodesJSON, err := common.Marshal(productCodes)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	configurationCodes := make([]string, 0, len(request.VolcengineBillConfigurationCodes))
	seenConfigurationCodes := make(map[string]struct{}, len(request.VolcengineBillConfigurationCodes))
	for _, configurationCode := range request.VolcengineBillConfigurationCodes {
		configurationCode = strings.TrimSpace(configurationCode)
		if configurationCode == "" {
			continue
		}
		if len(configurationCode) > 191 {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("Volcengine bill configuration code is too long"))
			return
		}
		if _, exists := seenConfigurationCodes[configurationCode]; exists {
			continue
		}
		seenConfigurationCodes[configurationCode] = struct{}{}
		configurationCodes = append(configurationCodes, configurationCode)
	}
	if request.VolcengineBillSyncEnabled && len(configurationCodes) == 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("at least one verified Volcengine bill configuration code is required when bill sync is enabled"))
		return
	}
	configurationCodesJSON, err := common.Marshal(configurationCodes)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	if request.VolcengineBillSyncEnabled {
		credential, credentialErr := model.GetActiveSeedanceVolcengineCredential(channelID)
		if credentialErr != nil || credential.AccessKeyIDEncrypted == "" || credential.SecretAccessKeyEncrypted == "" || credential.BillingValidatedAt <= 0 {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("an active credential with validated bill read-only AK/SK is required before enabling bill sync"))
			return
		}
	}
	item := &model.SeedanceChannelConfig{
		ChannelID: channelID, InstanceID: request.InstanceID,
		AIPDDBillingBaseURL:                  strings.TrimSpace(request.AIPDDBillingBaseURL),
		VolcengineBillSyncEnabled:            request.VolcengineBillSyncEnabled,
		VolcengineBillProductCodesJSON:       string(productCodesJSON),
		VolcengineBillConfigurationCodesJSON: string(configurationCodesJSON),
		DefaultEnhancementProviderID:         request.DefaultEnhancementProviderID, Status: status,
	}
	if strings.TrimSpace(request.AIPDDBillingAPIKey) != "" {
		encrypted, err := common.EncryptSensitiveValue(strings.TrimSpace(request.AIPDDBillingAPIKey))
		if err != nil {
			seedanceAdminFailure(c, http.StatusBadRequest, err)
			return
		}
		item.AIPDDBillingCredentialEncrypted = encrypted
	}
	if err := model.SaveSeedanceChannelConfig(item, c.GetInt("id"), "updated Seedance service configuration"); err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, item)
}

func CreateSeedanceCredential(c *gin.Context) {
	channelID, ok := seedancePathID(c, "channel_id")
	if !ok {
		return
	}
	if err := ensureSeedanceChannel(channelID); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	var request seedanceCredentialRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	arkKey := strings.TrimSpace(request.ArkAPIKey)
	if arkKey == "" {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("ark_api_key is required"))
		return
	}
	arkEncrypted, err := common.EncryptSensitiveValue(arkKey)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	accessEncrypted, secretEncrypted := "", ""
	if strings.TrimSpace(request.AccessKeyID) != "" || strings.TrimSpace(request.SecretAccessKey) != "" {
		if strings.TrimSpace(request.AccessKeyID) == "" || strings.TrimSpace(request.SecretAccessKey) == "" {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("access_key_id and secret_access_key must be provided together"))
			return
		}
		accessEncrypted, err = common.EncryptSensitiveValue(strings.TrimSpace(request.AccessKeyID))
		if err == nil {
			secretEncrypted, err = common.EncryptSensitiveValue(strings.TrimSpace(request.SecretAccessKey))
		}
		if err != nil {
			seedanceAdminFailure(c, http.StatusBadRequest, err)
			return
		}
	}
	item := &model.SeedanceVolcengineCredential{
		ChannelID: channelID, ArkAPIKeyEncrypted: arkEncrypted,
		AccessKeyIDEncrypted: accessEncrypted, SecretAccessKeyEncrypted: secretEncrypted,
		Fingerprint: model.SHA256Evidence(arkKey), MaskedSuffix: maskedSuffix(arkKey), CreatedBy: c.GetInt("id"),
	}
	if err := model.CreateSeedanceCredential(item, c.GetInt("id")); err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, item)
}

func ValidateSeedanceCredential(c *gin.Context) {
	id, ok := seedancePathInt64(c, "id")
	if !ok {
		return
	}
	var credential model.SeedanceVolcengineCredential
	if err := model.DB.Where("id = ?", id).First(&credential).Error; err != nil {
		seedanceAdminFailure(c, http.StatusNotFound, err)
		return
	}
	key, err := common.DecryptSensitiveValue(credential.ArkAPIKeyEncrypted)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	var channel model.Channel
	if err := model.DB.Where("id = ? AND type = ?", credential.ChannelID, constant.ChannelTypeSeedance).First(&channel).Error; err != nil {
		seedanceAdminFailure(c, http.StatusNotFound, err)
		return
	}
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeSeedance]
	}
	requestCtx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet,
		strings.TrimRight(baseURL, "/")+"/api/v3/contents/generations/tasks?page_size=1", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Accept", "application/json")
	}
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadGateway, errors.New("Ark credential validation failed"))
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		seedanceAdminFailure(c, http.StatusBadGateway, fmt.Errorf("Ark credential validation returned HTTP %d", resp.StatusCode))
		return
	}
	billingValidated := false
	if credential.AccessKeyIDEncrypted != "" && credential.SecretAccessKeyEncrypted != "" {
		if err := service.ValidateSeedanceVolcengineBillCredential(requestCtx, &credential); err != nil {
			seedanceAdminFailure(c, http.StatusBadGateway, err)
			return
		}
		billingValidated = true
	}
	if err := model.ActivateSeedanceCredential(id, c.GetInt("id"), billingValidated); err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, gin.H{"validated": true, "credential_id": id})
}

func ListSeedanceProviders(c *gin.Context) {
	items, err := model.ListMediaEnhancementProviders()
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, newSeedanceProviderViews(items))
}

func SaveSeedanceProvider(c *gin.Context) {
	var request seedanceProviderRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	providerType := strings.ToUpper(strings.TrimSpace(request.ProviderType))
	if providerType == "" {
		providerType = model.SeedanceProviderDirect
	}
	if providerType != model.SeedanceProviderDirect {
		// provider_type is the cost attribution AIPDD finance accepts. The wire
		// protocol belongs to adapter_type instead, so this stays constrained.
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("only DIRECT_EXTERNAL is supported in this phase"))
		return
	}
	adapterType, err := model.NormalizeSeedanceAdapterType(providerType, request.AdapterType)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(request.DisplayName) == "" {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("display_name is required"))
		return
	}
	var existing *model.MediaEnhancementProvider
	if request.ID > 0 {
		existing, err = model.GetMediaEnhancementProviderForExecution(request.ID)
		if err != nil {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("enhancement provider does not exist"))
			return
		}
		if providerAdapterType(existing) != adapterType {
			// Provider credentials and policy semantics are adapter-specific. A
			// protocol change on the same row could accidentally send an existing
			// secret to a different service; create a new provider instead.
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("adapter_type cannot be changed for an existing provider"))
			return
		}
	}
	item := &model.MediaEnhancementProvider{
		ID: request.ID, ProviderType: providerType, AdapterType: adapterType,
		DisplayName:      strings.TrimSpace(request.DisplayName),
		ServiceEndpoint:  strings.TrimSpace(request.ServiceEndpoint),
		ServiceCode:      strings.TrimSpace(request.ServiceCode),
		CapabilitiesJSON: jsonOrDefault(request.CapabilitiesJSON, "{}"), Status: strings.ToUpper(strings.TrimSpace(request.Status)),
		TimeoutPolicyJSON: jsonOrDefault(request.TimeoutPolicyJSON, "{}"), RetryPolicyJSON: jsonOrDefault(request.RetryPolicyJSON, "{}"),
		FallbackPolicyJSON: jsonOrDefault(request.FallbackPolicyJSON, "{}"),
		ClearCredential:    request.ClearCredential,
	}
	credential := strings.TrimSpace(request.Credential)
	switch adapterType {
	case model.SeedanceAdapterVolcengineMediaKit:
		// The official host and service code are owned by the server so neither a
		// crafted request nor a dirty database row can point a credentialed call
		// somewhere else, and so metrics stay comparable across MediaKit nodes.
		item.ServiceEndpoint = model.SeedanceMediaKitBaseURL
		item.ServiceCode = model.SeedanceMediaKitServiceCode
		// Capabilities are decided by the built-in adapter. Accepting them here
		// would let an administrator claim idempotency or cancellation that the
		// upstream protocol does not actually provide.
		item.CapabilitiesJSON = "{}"
		if strings.TrimSpace(request.Credential) != "" {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("credential only applies to the GENERIC_HTTP adapter; use mediakit_api_key"))
			return
		}
		if mediaKitKey := strings.TrimSpace(request.MediaKitAPIKey); mediaKitKey != "" {
			credential = mediaKitKey
		}
		hasStoredCredential := existing != nil && strings.TrimSpace(existing.CredentialEncrypted) != ""
		if request.ID == 0 && credential == "" {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("mediakit_api_key is required when creating an AI MediaKit provider"))
			return
		}
		if item.Status == model.SeedanceConfigActive && credential == "" && !hasStoredCredential && !request.ClearCredential {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("mediakit_api_key is required before enabling an AI MediaKit provider"))
			return
		}
	case model.SeedanceAdapterGenericHTTP:
		if item.ServiceCode == "" {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("service_code is required"))
			return
		}
		if !isTrustedServiceEndpoint(item.ServiceEndpoint) {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("provider endpoint must use HTTPS"))
			return
		}
		if strings.TrimSpace(request.MediaKitAPIKey) != "" {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("mediakit_api_key only applies to the VOLCENGINE_MEDIAKIT adapter"))
			return
		}
	}
	if item.Status == "" {
		item.Status = model.SeedanceConfigDisabled
	}
	if item.Status != model.SeedanceConfigActive && item.Status != model.SeedanceConfigDisabled {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("invalid provider status"))
		return
	}
	if err := validateSeedanceProviderPolicies(item); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	if request.ClearCredential && credential != "" {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("clear_credential cannot be combined with a new credential"))
		return
	}
	if credential != "" {
		encrypted, err := common.EncryptSensitiveValue(credential)
		if err != nil {
			seedanceAdminFailure(c, http.StatusBadRequest, err)
			return
		}
		item.CredentialEncrypted = encrypted
	}
	if err := model.SaveMediaEnhancementProvider(item, c.GetInt("id"), "updated provider configuration and policies"); err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, newSeedanceProviderView(*item))
}

func ListSeedanceOfferings(c *gin.Context) {
	channelID, _ := strconv.Atoi(strings.TrimSpace(c.Query("channel_id")))
	items, err := model.ListSeedanceModelOfferings(channelID)
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, items)
}

func SaveSeedanceOffering(c *gin.Context) {
	var request seedanceOfferingRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	if request.ChannelID <= 0 || request.DisplayName == "" || request.BaseModelID <= 0 || strings.TrimSpace(request.PricingVersion) == "" {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("channel_id, display_name, base_model_id and pricing_version are required"))
		return
	}
	if hasForbiddenSeedancePublicName(request.DisplayName) || strings.Contains(request.DisplayName, ",") {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("public model name exposes an internal processing detail"))
		return
	}
	if request.OutputFPS == 0 {
		request.OutputFPS = 24
	}
	if _, err := model.SeedanceFPSBucket(request.OutputFPS); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	if request.NoReferenceUnitPriceMicroRMB < 0 || request.ReferenceUnitPriceMicroRMB < 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("sale unit prices must be non-negative integer micro-RMB per second"))
		return
	}
	sourceResolution, err := model.NormalizeSeedanceResolution(request.SourceResolution)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	targetResolution, err := model.NormalizeSeedanceResolution(request.TargetResolution)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	config, err := model.GetSeedanceChannelConfig(request.ChannelID)
	if err != nil || config.Status != model.SeedanceConfigActive || config.LastVerifiedAt <= 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("channel credentials must be validated before publishing models"))
		return
	}
	if _, err := model.GetActiveSeedanceVolcengineCredential(request.ChannelID); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("active Ark credential is required"))
		return
	}
	baseModel, err := model.GetSeedanceBaseModelForExecution(request.BaseModelID)
	if err != nil || !baseModel.Enabled || !baseModel.Current || baseModel.ArchivedAt > 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("active current base model revision is required"))
		return
	}
	baseNoReferenceCost, err := model.ResolveSeedanceBaseUnitCost(baseModel, sourceResolution, false)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	if _, err := model.ResolveSeedanceBaseUnitCost(baseModel, sourceResolution, true); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}

	var enhancementModel *model.SeedanceEnhancementModel
	var provider *model.MediaEnhancementProvider
	enhancementUnitCost := int64(0)
	if request.EnhancementModelID != nil && *request.EnhancementModelID > 0 {
		enhancementModel, err = model.GetSeedanceEnhancementModelForExecution(*request.EnhancementModelID)
		if err != nil || !enhancementModel.Enabled || !enhancementModel.Current || enhancementModel.ArchivedAt > 0 {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("active current enhancement model revision is required"))
			return
		}
		enhancementUnitCost, err = model.ResolveSeedanceEnhancementUnitCost(enhancementModel, targetResolution, request.OutputFPS)
		if err != nil {
			seedanceAdminFailure(c, http.StatusBadRequest, err)
			return
		}
		provider, err = model.GetMediaEnhancementProvider(enhancementModel.ProviderID)
		if err != nil {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("active enhancement provider is required"))
			return
		}
		if provider.ProviderType == model.SeedanceProviderAIPDD {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("AIPDD internal enhancement is not enabled"))
			return
		}
		if providerAdapterType(provider) == model.SeedanceAdapterVolcengineMediaKit {
			if strings.TrimSpace(provider.CredentialEncrypted) == "" {
				seedanceAdminFailure(c, http.StatusBadRequest, errors.New("active AI MediaKit provider must have an API key"))
				return
			}
			if err := service.ValidateSeedanceMediaKitSpecification(enhancementModel.SpecificationJSON); err != nil {
				seedanceAdminFailure(c, http.StatusBadRequest, err)
				return
			}
		}
	} else if sourceResolution != targetResolution {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("enhancement_model_id is required when source and target resolutions differ"))
		return
	}

	providerCost := enhancementUnitCost
	item := &model.SeedanceModelOffering{
		ID: request.ID, ChannelID: request.ChannelID, DisplayName: request.DisplayName,
		BaseModelID: request.BaseModelID, EnhancementModelID: request.EnhancementModelID,
		SourceResolution: sourceResolution, TargetResolution: targetResolution, OutputFPS: request.OutputFPS,
		NoReferenceUnitPriceMicroRMB: request.NoReferenceUnitPriceMicroRMB,
		ReferenceUnitPriceMicroRMB:   request.ReferenceUnitPriceMicroRMB,
		ProviderModelID:              baseModel.ProviderModelID,
		ResolutionRulesJSON:          "{}", DurationRulesJSON: "{}",
		ModelSaleMicroRMB:     request.NoReferenceUnitPriceMicroRMB,
		ServiceChargeMicroRMB: enhancementUnitCost, ProviderCostMicroRMB: &providerCost,
		VolcengineUnitCostMicroRMB: baseNoReferenceCost,
		PricingVersion:             strings.TrimSpace(request.PricingVersion), Enabled: request.Enabled,
	}
	if enhancementModel != nil && provider != nil {
		item.EnhancementProviderID = provider.ID
		item.EnhancementServiceCode = enhancementModel.ServiceCode
		item.EnhancementSpecificationJSON = enhancementModel.SpecificationJSON
		item.EnhancementSpecificationVersion = enhancementModel.SpecificationVersion
	} else {
		item.EnhancementSpecificationJSON = "{}"
		item.EnhancementSpecificationVersion = "none"
		item.ProviderCostMicroRMB = nil
	}
	if err := model.SaveSeedanceModelOffering(item, c.GetInt("id"), "updated offering, price and provider snapshot source"); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, model.ErrSeedanceInvalidPricing) {
			status = http.StatusBadRequest
		}
		seedanceAdminFailure(c, status, err)
		return
	}
	common.ApiSuccess(c, item)
}

func ListSeedanceOrders(c *gin.Context) {
	page := common.GetPageQuery(c)
	channelID, _ := strconv.Atoi(strings.TrimSpace(c.Query("channel_id")))
	items, total, err := model.ListSeedanceOrders(model.SeedanceOrderQuery{
		ChannelID: channelID, PlatformOrderID: strings.TrimSpace(c.Query("platform_order_id")),
		Status: strings.TrimSpace(c.Query("status")), Offset: page.GetStartIdx(), Limit: page.GetPageSize(),
	})
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	views, err := newSeedanceOrderViews(items)
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	page.SetItems(views)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}

func ArchiveSeedanceOffering(c *gin.Context) {
	id, ok := seedancePathInt64(c, "id")
	if !ok {
		return
	}
	if err := model.ArchiveSeedanceModelOffering(id, c.GetInt("id")); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "archived": true})
}

func ListSeedanceBillingOutbox(c *gin.Context) {
	page := common.GetPageQuery(c)
	channelID := 0
	if raw := strings.TrimSpace(c.Query("channel_id")); raw != "" {
		var err error
		channelID, err = strconv.Atoi(raw)
		if err != nil || channelID <= 0 {
			seedanceAdminFailure(c, http.StatusBadRequest, errors.New("channel_id must be a positive integer"))
			return
		}
	}
	items, total, err := model.ListServiceBillingOutbox(model.ServiceBillingOutboxQuery{
		ChannelID: channelID, InstanceID: strings.TrimSpace(c.Query("instance_id")),
		PlatformOrderID: strings.TrimSpace(c.Query("platform_order_id")), Status: strings.TrimSpace(c.Query("status")),
		Offset: page.GetStartIdx(), Limit: page.GetPageSize(),
	})
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}

func ReplaySeedanceBillingOutbox(c *gin.Context) {
	eventID := strings.TrimSpace(c.Param("event_id"))
	if eventID == "" {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("event_id is required"))
		return
	}
	if err := model.ReplayServiceBillingOutbox(eventID, c.GetInt("id")); err != nil {
		status := http.StatusNotFound
		if errors.Is(err, model.ErrSeedanceDeadLetterRevision) {
			status = http.StatusConflict
		}
		seedanceAdminFailure(c, status, err)
		return
	}
	common.ApiSuccess(c, gin.H{"queued": true, "event_id": eventID})
}

func ReviseSeedanceBillingOutbox(c *gin.Context) {
	eventID := strings.TrimSpace(c.Param("event_id"))
	if eventID == "" {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("event_id is required"))
		return
	}
	event, created, err := service.ReviseSeedanceDeadLetter(eventID, c.GetInt("id"))
	if err != nil {
		seedanceAdminFailure(c, http.StatusConflict, err)
		return
	}
	common.ApiSuccess(c, gin.H{"event": event, "created": created})
}

func ImportSeedanceVolcengineBill(c *gin.Context) {
	var request seedanceVolcengineBillImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	if err := ensureSeedanceChannel(request.ChannelID); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	if hasSensitiveSeedanceSourceKey(request.Source) {
		seedanceAdminFailure(c, http.StatusBadRequest, errors.New("bill source contains a sensitive field"))
		return
	}
	source, err := common.Marshal(request.Source)
	if err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	item, duplicate, err := model.ImportSeedanceVolcengineBill(model.SeedanceBillImport{
		ChannelID: request.ChannelID, BillDetailID: request.BillDetailID, Revision: request.Revision,
		BillingPeriod: request.BillingPeriod, ProductCode: request.ProductCode, CostCategory: request.CostCategory,
		InstanceID:     request.InstanceID,
		UsageStartedAt: request.UsageStartedAt, UsageEndedAt: request.UsageEndedAt,
		AmountMicroRMB: request.AmountMicroRMB, SanitizedSourceJSON: string(source), Candidates: request.Candidates,
	}, c.GetInt("id"))
	if err != nil {
		seedanceAdminFailure(c, http.StatusConflict, err)
		return
	}
	if item.AllocationStatus == model.SeedanceBillAllocated {
		if err := service.QueueSeedanceCostRevisionEvents(c.Request.Context(), item.ID); err != nil {
			seedanceAdminFailure(c, http.StatusInternalServerError, err)
			return
		}
	}
	common.ApiSuccess(c, gin.H{"item": item, "duplicate": duplicate})
}

func ListSeedanceCostReconciliation(c *gin.Context) {
	page := common.GetPageQuery(c)
	items, total, err := model.ListSeedanceCostReconciliationIssues(c.Query("status"), page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	page.SetItems(items)
	page.SetTotal(int(total))
	common.ApiSuccess(c, page)
}

func ReconcileSeedanceVolcengineBill(c *gin.Context) {
	billItemID, ok := seedancePathInt64(c, "bill_item_id")
	if !ok {
		return
	}
	var request seedanceVolcengineBillReconcileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		seedanceAdminFailure(c, http.StatusBadRequest, err)
		return
	}
	item, err := model.ReconcileSeedanceVolcengineBillItem(billItemID, request.Candidates, c.GetInt("id"))
	if err != nil {
		seedanceAdminFailure(c, http.StatusConflict, err)
		return
	}
	if err := service.QueueSeedanceCostRevisionEvents(c.Request.Context(), item.ID); err != nil {
		seedanceAdminFailure(c, http.StatusInternalServerError, err)
		return
	}
	common.ApiSuccess(c, gin.H{"item": item, "reconciled": true})
}

func ensureSeedanceChannel(channelID int) error {
	var count int64
	if err := model.DB.Model(&model.Channel{}).Where("id = ? AND type = ?", channelID, constant.ChannelTypeSeedance).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return errors.New("Seedance channel not found")
	}
	return nil
}

func seedancePathID(c *gin.Context, key string) (int, bool) {
	value, err := strconv.Atoi(strings.TrimSpace(c.Param(key)))
	if err != nil || value <= 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, fmt.Errorf("valid %s is required", key))
		return 0, false
	}
	return value, true
}

func seedancePathInt64(c *gin.Context, key string) (int64, bool) {
	value, err := strconv.ParseInt(strings.TrimSpace(c.Param(key)), 10, 64)
	if err != nil || value <= 0 {
		seedanceAdminFailure(c, http.StatusBadRequest, fmt.Errorf("valid %s is required", key))
		return 0, false
	}
	return value, true
}

func seedanceAdminFailure(c *gin.Context, status int, err error) {
	c.JSON(status, gin.H{"success": false, "message": err.Error()})
}

func maskedSuffix(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= 4 {
		return string(runes)
	}
	return string(runes[len(runes)-4:])
}

func jsonOrDefault(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func isSeedanceJSON(value string) bool {
	var decoded any
	return common.UnmarshalJsonStr(value, &decoded) == nil
}

// providerAdapterType resolves the adapter for rows stored before the column
// existed, which all spoke the generic protocol.
func providerAdapterType(provider *model.MediaEnhancementProvider) string {
	if provider == nil {
		return ""
	}
	if adapterType := strings.TrimSpace(provider.AdapterType); adapterType != "" {
		return adapterType
	}
	return model.LegacySeedanceAdapterType(provider.ProviderType)
}

func validateSeedanceProviderPolicies(item *model.MediaEnhancementProvider) error {
	if item == nil {
		return errors.New("provider configuration is required")
	}
	if _, err := decodeSeedancePolicyObject(item.CapabilitiesJSON, nil); err != nil {
		return fmt.Errorf("capabilities must be a JSON object: %w", err)
	}
	timeoutFields, err := decodeSeedancePolicyObject(item.TimeoutPolicyJSON, map[string]struct{}{"timeout_seconds": {}})
	if err != nil {
		return fmt.Errorf("timeout_policy is invalid: %w", err)
	}
	if raw, ok := timeoutFields["timeout_seconds"]; ok {
		var timeoutSeconds int
		if err := common.Unmarshal(raw, &timeoutSeconds); err != nil || timeoutSeconds < 1 || timeoutSeconds > 86_400 {
			return errors.New("timeout_policy.timeout_seconds must be an integer from 1 to 86400")
		}
	}
	retryFields, err := decodeSeedancePolicyObject(item.RetryPolicyJSON, map[string]struct{}{"mode": {}})
	if err != nil {
		return fmt.Errorf("retry_policy is invalid: %w", err)
	}
	if raw, ok := retryFields["mode"]; ok {
		var mode string
		if err := common.Unmarshal(raw, &mode); err != nil || strings.ToUpper(strings.TrimSpace(mode)) != "SAME_ATTEMPT_UNKNOWN_ONLY" {
			return errors.New("retry_policy.mode must be SAME_ATTEMPT_UNKNOWN_ONLY")
		}
	}
	fallbackFields, err := decodeSeedancePolicyObject(item.FallbackPolicyJSON, map[string]struct{}{"mode": {}})
	if err != nil {
		return fmt.Errorf("fallback_policy is invalid: %w", err)
	}
	if raw, ok := fallbackFields["mode"]; ok {
		var mode string
		if err := common.Unmarshal(raw, &mode); err != nil || strings.ToUpper(strings.TrimSpace(mode)) != "NONE" {
			return errors.New("fallback_policy.mode must be NONE in this phase")
		}
	}
	return nil
}

func decodeSeedancePolicyObject(value string, allowed map[string]struct{}) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(value, &fields); err != nil || fields == nil {
		return nil, errors.New("expected a JSON object")
	}
	if allowed != nil {
		for field := range fields {
			if _, ok := allowed[field]; !ok {
				return nil, fmt.Errorf("unsupported field %q", field)
			}
		}
	}
	return fields, nil
}

func isTrustedServiceEndpoint(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	host := strings.ToLower(parsed.Hostname())
	return parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1")
}

func hasForbiddenSeedancePublicName(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, token := range []string{"enhancement", "enhance", "super_resolution", "super-resolution", "upscale", "超分", "增强", "provider", "byok"} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	for _, field := range strings.FieldsFunc(normalized, func(r rune) bool { return r == '-' || r == '_' || r == ' ' || r == '.' }) {
		if field == "sr" {
			return true
		}
	}
	return false
}

func hasSensitiveSeedanceSourceKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			for _, token := range []string{"secret", "password", "api_key", "access_key", "authorization", "credential", "token"} {
				if strings.Contains(normalized, token) {
					return true
				}
			}
			if hasSensitiveSeedanceSourceKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasSensitiveSeedanceSourceKey(child) {
				return true
			}
		}
	}
	return false
}
