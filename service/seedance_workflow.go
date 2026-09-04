package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	seedanceGenericFailureMessage = "视频处理失败，请稍后重试"
	seedanceEnhancementProgress   = "80%"
	seedanceOutboxLeaseSeconds    = int64(60)
	seedanceOutboxAuthPaused      = model.SeedanceSyncAuthPaused
	seedanceCallbackTimeout       = 30 * time.Second
)

// Kept as a function variable so the crash-recovery contract can be tested by
// suppressing the best-effort immediate dispatch after the terminal commit.
var dispatchSeedanceCustomerRefund = processSeedanceCustomerRefund

type EnhancementSubmitRequest struct {
	InputURL          string
	SpecificationJSON string
	IdempotencyKey    string
}

type EnhancementResult struct {
	ExecutionTaskID   string
	Status            string
	ResultURL         string
	UsageEvidenceJSON string
	// FailureReason is an internal, non-sensitive classification. It is never
	// copied into the public task response, but it lets administrator metrics
	// distinguish a provider-declared task failure from transport failures.
	FailureReason string
}

type enhancementProviderError struct {
	cause      error
	definitive bool
	reason     string
}

func (e *enhancementProviderError) Error() string { return e.cause.Error() }
func (e *enhancementProviderError) Unwrap() error { return e.cause }

func isDefinitiveEnhancementFailure(err error) bool {
	var providerErr *enhancementProviderError
	return errors.As(err, &providerErr) && providerErr.definitive
}

func enhancementFailureReason(err error, fallback string) string {
	var providerErr *enhancementProviderError
	if errors.As(err, &providerErr) && strings.TrimSpace(providerErr.reason) != "" {
		return strings.TrimSpace(providerErr.reason)
	}
	return fallback
}

type EnhancementProvider interface {
	Submit(ctx context.Context, request EnhancementSubmitRequest) (*EnhancementResult, error)
	Query(ctx context.Context, executionTaskID string) (*EnhancementResult, error)
	Cancel(ctx context.Context, executionTaskID string) error
}

// EnhancementCapabilities describes what an adapter's wire protocol actually
// guarantees. It is static per adapter and never read from administrator input,
// because claiming an unproven guarantee is what causes duplicate billable
// remote tasks or refunds for work that keeps running upstream.
type EnhancementCapabilities struct {
	// SubmitRetrySafe means the provider guarantees that resending the same
	// idempotency key cannot create a second task.
	SubmitRetrySafe bool
	// SubmitRetryWindow bounds that guarantee when the provider documents a
	// finite deduplication lifetime. Zero means the adapter contract has no
	// shorter window than the local workflow lifetime. A retry outside a finite
	// window must stop for manual reconciliation instead of risking a second
	// billable task.
	SubmitRetryWindow time.Duration
	// CancelSupported means the provider exposes a cancellation endpoint whose
	// success proves the remote task stopped.
	CancelSupported bool
}

type capabilityAwareEnhancementProvider interface {
	Capabilities() EnhancementCapabilities
}

// ErrSeedanceSubmissionNeedsManualReview stops the poller from sending a second
// POST when the adapter has no proven idempotency contract or its documented
// deduplication window has elapsed. The attempt keeps its
// SUBMISSION_OUTCOME_UNKNOWN marker so an administrator can reconcile the
// remote task before any further request is made.
var ErrSeedanceSubmissionNeedsManualReview = errors.New("enhancement submission outcome is unknown and no currently valid idempotency guarantee permits another submission")

// ErrSeedanceRemoteCancelUnsupported reports that an accepted remote task cannot
// be cancelled. Returning success here would refund the customer while the
// provider keeps executing and billing.
var ErrSeedanceRemoteCancelUnsupported = errors.New("the upstream enhancement service does not support cancelling an accepted task")

func enhancementCapabilities(provider EnhancementProvider) EnhancementCapabilities {
	if aware, ok := provider.(capabilityAwareEnhancementProvider); ok {
		return aware.Capabilities()
	}
	// The generic external contract already requires idempotent submit and a
	// DELETE endpoint, so adapters that do not declare capabilities keep it.
	return EnhancementCapabilities{SubmitRetrySafe: true, CancelSupported: true}
}

func enhancementSubmissionRetrySafeAt(capabilities EnhancementCapabilities, usage *model.MediaServiceUsage, now int64) bool {
	if !capabilities.SubmitRetrySafe {
		return false
	}
	if capabilities.SubmitRetryWindow <= 0 {
		return true
	}
	if usage == nil {
		return false
	}
	startedAt := usage.StartedAt
	if startedAt <= 0 {
		startedAt = usage.CreatedAt
	}
	if startedAt <= 0 {
		return false
	}
	windowSeconds := int64(capabilities.SubmitRetryWindow / time.Second)
	return windowSeconds > 0 && now-startedAt < windowSeconds
}

type DirectEnhancementProvider struct {
	config     *model.MediaEnhancementProvider
	credential string
	client     *http.Client
}

type directEnhancementRequest struct {
	Input          map[string]string `json:"input"`
	Specification  any               `json:"specification"`
	IdempotencyKey string            `json:"idempotency_key"`
}

type directEnhancementResponse struct {
	ExecutionTaskID string `json:"execution_task_id"`
	TaskID          string `json:"task_id"`
	Status          string `json:"status"`
	Result          struct {
		URL string `json:"url"`
	} `json:"result"`
	URL   string `json:"url"`
	Usage any    `json:"usage"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

var enhancementProviderFactory = newEnhancementProvider

func newEnhancementProvider(config *model.MediaEnhancementProvider) (EnhancementProvider, error) {
	if config == nil {
		return nil, errors.New("enhancement provider is unavailable")
	}
	if config.ProviderType != model.SeedanceProviderDirect {
		return nil, fmt.Errorf("unsupported enhancement provider type %s", config.ProviderType)
	}
	// Orders frozen before adapter_type existed were all created against the
	// generic protocol, so resolve rather than reject them.
	adapterType := strings.TrimSpace(config.AdapterType)
	if adapterType == "" {
		adapterType = model.LegacySeedanceAdapterType(config.ProviderType)
	}
	credential := ""
	var err error
	if strings.TrimSpace(config.CredentialEncrypted) != "" {
		credential, err = common.DecryptSensitiveValue(config.CredentialEncrypted)
		if err != nil {
			return nil, err
		}
	}
	timeout := 10 * time.Minute
	var policy struct {
		TimeoutSeconds int `json:"timeout_seconds"`
	}
	if common.UnmarshalJsonStr(config.TimeoutPolicyJSON, &policy) == nil && policy.TimeoutSeconds > 0 {
		timeout = time.Duration(policy.TimeoutSeconds) * time.Second
	}
	client, err := GetHttpClientWithProxy("")
	if err != nil {
		return nil, err
	}
	cloned := *client
	cloned.Timeout = timeout
	switch adapterType {
	case model.SeedanceAdapterGenericHTTP:
		return &DirectEnhancementProvider{config: config, credential: credential, client: &cloned}, nil
	case model.SeedanceAdapterVolcengineMediaKit:
		return newVolcengineMediaKitEnhancementProvider(credential, &cloned)
	default:
		return nil, fmt.Errorf("unsupported enhancement adapter type %s", adapterType)
	}
}

func (p *DirectEnhancementProvider) Submit(ctx context.Context, request EnhancementSubmitRequest) (*EnhancementResult, error) {
	specification := any(map[string]any{})
	if strings.TrimSpace(request.SpecificationJSON) != "" {
		if err := common.UnmarshalJsonStr(request.SpecificationJSON, &specification); err != nil {
			return nil, fmt.Errorf("decode enhancement specification: %w", err)
		}
	}
	body, err := common.Marshal(directEnhancementRequest{
		Input:          map[string]string{"url": request.InputURL},
		Specification:  specification,
		IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	return p.exchange(ctx, http.MethodPost, strings.TrimRight(p.config.ServiceEndpoint, "/"), body, request.IdempotencyKey)
}

func (p *DirectEnhancementProvider) Query(ctx context.Context, executionTaskID string) (*EnhancementResult, error) {
	endpoint := strings.TrimRight(p.config.ServiceEndpoint, "/") + "/" + url.PathEscape(executionTaskID)
	return p.exchange(ctx, http.MethodGet, endpoint, nil, "")
}

func (p *DirectEnhancementProvider) Cancel(ctx context.Context, executionTaskID string) error {
	endpoint := strings.TrimRight(p.config.ServiceEndpoint, "/") + "/" + url.PathEscape(executionTaskID)
	_, err := p.exchange(ctx, http.MethodDelete, endpoint, nil, "")
	return err
}

// Capabilities restates the contract the generic protocol already required from
// custom remote services: an honoured Idempotency-Key and a DELETE endpoint.
func (p *DirectEnhancementProvider) Capabilities() EnhancementCapabilities {
	return EnhancementCapabilities{SubmitRetrySafe: true, CancelSupported: true}
}

func (p *DirectEnhancementProvider) exchange(ctx context.Context, method string, endpoint string, body []byte, idempotencyKey string) (*EnhancementResult, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.credential != "" {
		req.Header.Set("Authorization", "Bearer "+p.credential)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		definitive := resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError &&
			resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusConflict &&
			resp.StatusCode != http.StatusTooEarly && resp.StatusCode != http.StatusTooManyRequests
		return nil, &enhancementProviderError{
			cause: fmt.Errorf("enhancement provider returned HTTP %d", resp.StatusCode), definitive: definitive,
		}
	}
	if method == http.MethodDelete && len(bytes.TrimSpace(responseBody)) == 0 {
		return &EnhancementResult{Status: model.SeedanceOrderCancelled}, nil
	}
	var payload directEnhancementResponse
	if err := common.Unmarshal(responseBody, &payload); err != nil {
		return nil, err
	}
	if payload.ExecutionTaskID == "" {
		payload.ExecutionTaskID = payload.TaskID
	}
	if payload.Result.URL == "" {
		payload.Result.URL = payload.URL
	}
	usageJSON := "{}"
	if payload.Usage != nil {
		if encoded, marshalErr := common.Marshal(payload.Usage); marshalErr == nil {
			usageJSON = string(encoded)
		}
	}
	if strings.TrimSpace(payload.Error.Message) != "" {
		return nil, &enhancementProviderError{cause: errors.New("enhancement provider reported failure"), definitive: true}
	}
	return &EnhancementResult{
		ExecutionTaskID:   payload.ExecutionTaskID,
		Status:            normalizeEnhancementStatus(payload.Status),
		ResultURL:         payload.Result.URL,
		UsageEvidenceJSON: usageJSON,
	}, nil
}

func normalizeEnhancementStatus(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "succeeded", "success", "completed", "done":
		return model.SeedanceUsageSucceeded
	case "failed", "failure", "error", "cancelled", "canceled":
		return model.SeedanceUsageFailed
	case "pending", "queued", "submitted", "processing", "running", "in_progress", "":
		return model.SeedanceUsageRunning
	default:
		return model.SeedanceUsageRunning
	}
}

type seedancePricingSnapshot struct {
	PricingVersion                  string  `json:"pricing_version"`
	GroupRatio                      float64 `json:"group_ratio"`
	BaseUnitCostMicroRMB            int64   `json:"base_unit_cost_micro_rmb"`
	SuperResolutionUnitCostMicroRMB int64   `json:"super_resolution_unit_cost_micro_rmb"`
	EnhancementRequired             bool    `json:"enhancement_required"`
	ServiceChargeMicroRMB           int64   `json:"service_charge_micro_rmb"`
	ProviderType                    string  `json:"provider_type"`
	AdapterType                     string  `json:"adapter_type"`
	ProviderID                      int64   `json:"provider_id"`
	ServiceCode                     string  `json:"service_code"`
	SpecificationJSON               string  `json:"specification"`
	SpecificationVersion            string  `json:"specification_version"`
	ProviderCostMicroRMB            *int64  `json:"provider_cost_micro_rmb"`
}

func parseSeedancePricingSnapshot(order *model.SeedanceOrder) (*seedancePricingSnapshot, error) {
	if order == nil {
		return nil, errors.New("Seedance order is required")
	}
	var snapshot seedancePricingSnapshot
	if err := common.UnmarshalJsonStr(order.PricingSnapshotJSON, &snapshot); err != nil {
		return nil, fmt.Errorf("parse Seedance pricing snapshot: %w", err)
	}
	if snapshot.ProviderID <= 0 || strings.TrimSpace(snapshot.ServiceCode) == "" || strings.TrimSpace(snapshot.PricingVersion) == "" {
		return nil, errors.New("Seedance pricing snapshot is incomplete")
	}
	return &snapshot, nil
}

// ConfirmSeedanceGenerationSubmission atomically attaches the Ark task ID to
// the task and the already-durable generation attempt. It is safe to call
// again with the same upstream ID after a local response-path retry.
func ConfirmSeedanceGenerationSubmission(task *model.Task, upstreamTaskID string, responseEvidence []byte) error {
	if task == nil || strings.TrimSpace(task.TaskID) == "" || strings.TrimSpace(upstreamTaskID) == "" {
		return errors.New("Seedance generation submission result is incomplete")
	}
	upstreamTaskID = strings.TrimSpace(upstreamTaskID)
	now := time.Now().Unix()
	privateData := task.PrivateData
	privateData.UpstreamTaskID = upstreamTaskID
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var order model.SeedanceOrder
		if err := tx.Where("new_api_task_id = ?", task.TaskID).First(&order).Error; err != nil {
			return err
		}
		if order.OrderStatus == model.SeedanceOrderGenerationSubmitting {
			transition := tx.Model(&model.SeedanceOrder{}).
				Where("id = ? AND order_status = ?", order.ID, model.SeedanceOrderGenerationSubmitting).
				Updates(map[string]any{"order_status": model.SeedanceOrderGenerationProcessing, "updated_at": now})
			if transition.Error != nil {
				return transition.Error
			}
			if transition.RowsAffected != 1 {
				return errors.New("Seedance generation submission transition was lost")
			}
		} else if order.OrderStatus != model.SeedanceOrderGenerationProcessing {
			return fmt.Errorf("Seedance generation submission cannot complete from status %s", order.OrderStatus)
		}

		var attempt model.SeedanceAttempt
		if err := tx.Where("platform_order_id = ? AND stage = ?", order.PlatformOrderID, "GENERATION").First(&attempt).Error; err != nil {
			return err
		}
		if strings.TrimSpace(attempt.ExternalTaskID) != "" && attempt.ExternalTaskID != upstreamTaskID {
			return errors.New("Seedance generation attempt already belongs to another upstream task")
		}
		if err := tx.Model(&model.SeedanceAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]any{
			"external_task_id": upstreamTaskID, "status": model.SeedanceUsageRunning,
			"response_evidence_hash": model.SHA256Evidence(string(responseEvidence)), "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.Task{}).
			Where("id = ? AND status NOT IN ?", task.ID, []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure}).
			Updates(map[string]any{
				"private_data": privateData, "status": model.TaskStatusSubmitted,
				"updated_at": now,
			}).Error
	})
	if err == nil {
		task.PrivateData = privateData
		task.Status = model.TaskStatusSubmitted
		task.UpdatedAt = now
	}
	return err
}

// MarkSeedanceGenerationSubmissionOutcomeUnknown records the conservative
// boundary used when Ark may have accepted a request but no usable task ID was
// received. Ark's create-video contract has no idempotency token, so this
// attempt is never submitted automatically again.
func MarkSeedanceGenerationSubmissionOutcomeUnknown(taskID string, evidence string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("Seedance task id is required")
	}
	now := time.Now().Unix()
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var order model.SeedanceOrder
		if err := tx.Where("new_api_task_id = ?", taskID).First(&order).Error; err != nil {
			return err
		}
		if order.OrderStatus != model.SeedanceOrderGenerationSubmitting {
			return nil
		}
		if err := tx.Model(&model.SeedanceAttempt{}).
			Where("platform_order_id = ? AND stage = ? AND external_task_id = ''", order.PlatformOrderID, "GENERATION").
			Updates(map[string]any{
				"status":                 model.SeedanceSubmissionOutcomeUnknown,
				"response_evidence_hash": model.SHA256Evidence(evidence), "updated_at": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.SeedanceOrder{}).Where("id = ?", order.ID).Update("updated_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&model.Task{}).
			Where("task_id = ? AND status NOT IN ?", taskID, []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure}).
			Updates(map[string]any{"status": model.TaskStatusSubmitted, "updated_at": now}).Error
	})
}

// HandleSeedanceWorkflowPoll handles the private enhancement phase before the
// generic task poller calls Ark again. It returns handled=false while generation
// is still owned by Ark.
func HandleSeedanceWorkflowPoll(ctx context.Context, channel *model.Channel, task *model.Task) (bool, error) {
	if channel == nil || task == nil || channel.Type != constant.ChannelTypeSeedance {
		return false, nil
	}
	order, err := model.GetSeedanceOrderByTaskID(task.TaskID)
	if err != nil {
		return false, err
	}
	switch order.OrderStatus {
	case model.SeedanceOrderGenerationSubmitting, model.SeedanceOrderReceived:
		// A pre-submission intent without an Ark task ID is intentionally not sent
		// again: the prior request may have been accepted. The generic timeout
		// sweeper will close and refund it through FailSeedanceWorkflow.
		if strings.TrimSpace(task.PrivateData.UpstreamTaskID) == "" {
			return true, nil
		}
		return false, nil
	case model.SeedanceOrderGenerationProcessing:
		return false, nil
	case model.SeedanceOrderEnhancing:
		return true, pollSeedanceEnhancement(ctx, task, order)
	case model.SeedanceOrderSucceeded, model.SeedanceOrderFailed, model.SeedanceOrderCancelled:
		return true, nil
	default:
		return true, fmt.Errorf("unknown Seedance order status %s", order.OrderStatus)
	}
}

// FailSeedanceWorkflow closes either workflow phase with one compare-and-swap
// transition. It is used by upstream failures and the generic timeout sweeper
// so the public task, private order and financial outbox cannot diverge.
func FailSeedanceWorkflow(ctx context.Context, task *model.Task, evidence string) (bool, error) {
	if task == nil {
		return false, nil
	}
	order, err := model.GetSeedanceOrderByTaskID(task.TaskID)
	if model.IsRecordNotFound(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	switch order.OrderStatus {
	case model.SeedanceOrderEnhancing:
		var usage model.MediaServiceUsage
		if err := model.DB.Where("platform_order_id = ?", order.PlatformOrderID).Order("id desc").First(&usage).Error; err != nil {
			return true, err
		}
		usage.FailureReason = "WORKFLOW_FAILED"
		return true, failSeedanceOrder(ctx, task, order, &usage)
	case model.SeedanceOrderReceived, model.SeedanceOrderGenerationSubmitting, model.SeedanceOrderGenerationProcessing:
		return true, failSeedanceGeneration(ctx, task, order, evidence)
	case model.SeedanceOrderSucceeded, model.SeedanceOrderFailed, model.SeedanceOrderCancelled:
		return true, nil
	default:
		return true, fmt.Errorf("unknown Seedance order status %s", order.OrderStatus)
	}
}

// CancelSeedanceWorkflow is called only after the generation provider accepts
// cancellation, or directly while the private processing phase owns the task.
// deleteAfter matches the OpenAI DELETE contract; the official Seedance API
// keeps a visible cancelled snapshot for non-terminal tasks.
func CancelSeedanceWorkflow(ctx context.Context, task *model.Task, deleteAfter bool) error {
	if task == nil {
		return errors.New("Seedance task is required")
	}
	order, err := model.GetVisibleSeedanceOrderByTaskID(task.TaskID)
	if err != nil {
		return err
	}
	if order.OrderStatus == model.SeedanceOrderSucceeded || order.OrderStatus == model.SeedanceOrderFailed || order.OrderStatus == model.SeedanceOrderCancelled {
		return model.SoftDeleteSeedanceOrder(task.TaskID)
	}
	snapshot, err := parseSeedancePricingSnapshot(order)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	var usage model.MediaServiceUsage
	usageExists := false
	if order.OrderStatus == model.SeedanceOrderEnhancing {
		if err := model.DB.Where("platform_order_id = ?", order.PlatformOrderID).Order("id desc").First(&usage).Error; err != nil {
			return err
		}
		usageExists = true
		if strings.TrimSpace(usage.ExecutionTaskID) != "" {
			provider, providerErr := model.ResolveSeedanceProviderForOrder(order)
			if providerErr != nil {
				return providerErr
			}
			executor, providerErr := enhancementProviderFactory(provider)
			if providerErr != nil {
				return providerErr
			}
			if !enhancementCapabilities(executor).CancelSupported {
				// Settling locally would refund the customer and drop the cost
				// evidence while the provider keeps executing and billing. Report
				// the conflict and let the poller observe the real terminal state.
				return ErrSeedanceRemoteCancelUnsupported
			}
			if providerErr = executor.Cancel(ctx, usage.ExecutionTaskID); providerErr != nil {
				return providerErr
			}
		} else if usage.Status == model.SeedanceUsagePending {
			// A pending attempt without an execution ID was never confirmed as
			// accepted, so cancellation must not retain a provider execution cost.
			usage.ProviderCostMicroRMB = nil
		}
	} else {
		usage = model.MediaServiceUsage{
			ServiceLineItemID: order.PlatformOrderID + ":video-processing", PlatformOrderID: order.PlatformOrderID,
			ServiceType: model.SeedanceServiceTypeVideoSuperResolution, ProviderType: snapshot.ProviderType,
			ProviderID: snapshot.ProviderID, ServiceCode: snapshot.ServiceCode,
			SpecificationJSON: snapshot.SpecificationJSON, SpecificationVersion: snapshot.SpecificationVersion,
			AttemptID: order.PlatformOrderID + ":enhancement:not-executed", PriceVersion: snapshot.PricingVersion,
			ProviderCostMicroRMB: nil, UsageFactsJSON: "{}", Revision: 1,
			StartedAt: now, CreatedAt: now,
		}
	}
	usage.Status = model.SeedanceUsageFailed
	usage.FailureReason = "CANCELLED"
	usage.ChargeMicroRMB = 0
	usage.CompletedAt = now
	usage.UpdatedAt = now
	usage.UsageEvidenceHash = model.SHA256Evidence("cancelled")
	incurredSuperResolutionCost := int64(0)
	if order.EnhancementModelID != nil && usageExists && usage.ProviderCostMicroRMB != nil {
		incurredSuperResolutionCost = order.SuperResolutionCostMicroRMB
	}
	usage.ChargeMicroRMB = incurredSuperResolutionCost
	failureProfit, err := model.CalculateSeedanceProfit(0, incurredSuperResolutionCost, order.VolcengineEstimatedMicroRMB)
	if err != nil {
		return err
	}
	settledOrder := *order
	settledOrder.FinanceRevision++
	settledOrder.ModelSaleMicroRMB = 0
	settledOrder.SuperResolutionCostMicroRMB = incurredSuperResolutionCost
	settledOrder.ServiceChargeTotalMicroRMB = incurredSuperResolutionCost
	settledOrder.NewAPIEstimatedProfitMicroRMB = failureProfit
	cancelledData, err := seedanceCancelledTaskData(task)
	if err != nil {
		return err
	}
	event, outbox, err := buildSeedanceBillingEvent(&settledOrder, &usage, model.SeedanceOrderCancelled, "cancelled", now)
	if err != nil {
		return err
	}
	won := false
	var customerRefund *model.SeedanceCustomerRefund
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		updates := seedanceTerminalOrderUpdates(order, model.SeedanceOrderCancelled, now)
		updates["model_sale_micro_rmb"] = int64(0)
		updates["super_resolution_cost_micro_rmb"] = incurredSuperResolutionCost
		updates["service_charge_total_micro_rmb"] = incurredSuperResolutionCost
		updates["new_api_estimated_profit_micro_rmb"] = failureProfit
		if deleteAfter {
			updates["deleted_at"] = now
		}
		transition := tx.Model(&model.SeedanceOrder{}).
			Where("id = ? AND order_status IN ?", order.ID, []string{
				model.SeedanceOrderReceived, model.SeedanceOrderGenerationSubmitting,
				model.SeedanceOrderGenerationProcessing, model.SeedanceOrderEnhancing,
			}).Updates(updates)
		if transition.Error != nil || transition.RowsAffected == 0 {
			return transition.Error
		}
		won = true
		if usageExists {
			if err := tx.Model(&model.MediaServiceUsage{}).Where("id = ?", usage.ID).Updates(map[string]any{
				"status": model.SeedanceUsageFailed, "charge_micro_rmb": incurredSuperResolutionCost,
				"provider_cost_micro_rmb": usage.ProviderCostMicroRMB,
				"failure_reason":          usage.FailureReason,
				"usage_evidence_hash":     usage.UsageEvidenceHash, "completed_at": now, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&usage).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.SeedanceAttempt{}).Where("platform_order_id = ?", order.PlatformOrderID).
			Updates(map[string]any{"status": model.SeedanceOrderCancelled, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Task{}).
			Where("id = ? AND status NOT IN ?", task.ID, []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure}).
			Updates(map[string]any{
				"status": model.TaskStatusFailure, "progress": taskcommon.ProgressComplete, "finish_time": now,
				"fail_reason": "Task was cancelled", "data": cancelledData, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		if err := tx.Create(outbox).Error; err != nil {
			return err
		}
		customerRefund, err = model.QueueSeedanceCustomerRefundTx(tx, order, task, "Task was cancelled")
		return err
	})
	if err == nil && won && customerRefund != nil {
		dispatchSeedanceCustomerRefund(ctx, customerRefund.ID)
	}
	return err
}

func seedanceCancelledTaskData(task *model.Task) ([]byte, error) {
	data, err := seedancePublicTaskData(task, model.TaskStatusFailure, "")
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	payload["status"] = "cancelled"
	delete(payload, "error")
	return common.Marshal(payload)
}

func failSeedanceGeneration(ctx context.Context, task *model.Task, order *model.SeedanceOrder, evidence string) error {
	snapshot, err := parseSeedancePricingSnapshot(order)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	neutralData, err := seedancePublicTaskData(task, model.TaskStatusFailure, "")
	if err != nil {
		return err
	}
	lineItemID := order.PlatformOrderID + ":video-processing"
	usage := &model.MediaServiceUsage{
		ServiceLineItemID: lineItemID, PlatformOrderID: order.PlatformOrderID,
		ServiceType: model.SeedanceServiceTypeVideoSuperResolution, ProviderType: snapshot.ProviderType,
		ProviderID: snapshot.ProviderID, ServiceCode: snapshot.ServiceCode,
		SpecificationJSON: snapshot.SpecificationJSON, SpecificationVersion: snapshot.SpecificationVersion,
		AttemptID: order.PlatformOrderID + ":enhancement:not-executed", Status: model.SeedanceUsageFailed,
		FailureReason:  "GENERATION_FAILED",
		ChargeMicroRMB: 0, PriceVersion: snapshot.PricingVersion, ProviderCostMicroRMB: nil,
		UsageFactsJSON: "{}", UsageEvidenceHash: model.SHA256Evidence(evidence), Revision: 1,
		StartedAt: now, CompletedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	settledOrder := *order
	settledOrder.FinanceRevision++
	settledOrder.ModelSaleMicroRMB = 0
	settledOrder.ServiceChargeTotalMicroRMB = 0
	failureProfit, err := model.CalculateSeedanceFailureProfit(order.VolcengineEstimatedMicroRMB)
	if err != nil {
		return err
	}
	settledOrder.NewAPIEstimatedProfitMicroRMB = failureProfit
	event, outbox, err := buildSeedanceBillingEvent(&settledOrder, usage, model.SeedanceOrderFailed, evidence, now)
	if err != nil {
		return err
	}
	won := false
	var customerRefund *model.SeedanceCustomerRefund
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		transition := tx.Model(&model.SeedanceOrder{}).
			Where("id = ? AND order_status IN ?", order.ID, []string{
				model.SeedanceOrderReceived, model.SeedanceOrderGenerationSubmitting, model.SeedanceOrderGenerationProcessing,
			}).Updates(seedanceTerminalOrderUpdates(order, model.SeedanceOrderFailed, now, map[string]any{
			"generation_completed_at": now, "model_sale_micro_rmb": int64(0), "service_charge_total_micro_rmb": int64(0),
			"new_api_estimated_profit_micro_rmb": failureProfit,
		}))
		if transition.Error != nil || transition.RowsAffected == 0 {
			return transition.Error
		}
		won = true
		if err := tx.Model(&model.SeedanceAttempt{}).
			Where("platform_order_id = ? AND stage = ?", order.PlatformOrderID, "GENERATION").
			Updates(map[string]any{
				"status": model.SeedanceUsageFailed, "response_evidence_hash": model.SHA256Evidence(evidence),
				"completed_at": now, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Create(usage).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Task{}).
			Where("id = ? AND status NOT IN ?", task.ID, []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure}).
			Updates(map[string]any{
				"status": model.TaskStatusFailure, "progress": taskcommon.ProgressComplete, "finish_time": now,
				"fail_reason": seedanceGenericFailureMessage, "data": neutralData, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		if err := tx.Create(outbox).Error; err != nil {
			return err
		}
		customerRefund, err = model.QueueSeedanceCustomerRefundTx(tx, order, task, seedanceGenericFailureMessage)
		return err
	})
	if err == nil && won && customerRefund != nil {
		dispatchSeedanceCustomerRefund(ctx, customerRefund.ID)
	}
	return err
}

// HandleSeedanceGenerationSuccess is the accounting boundary between Ark and
// the optional private processing stage. The sale model fixes output FPS, and
// every monetary total is recomputed from the actual generated duration before
// the order can advance.
func HandleSeedanceGenerationSuccess(ctx context.Context, task *model.Task, generationResult *relaycommon.TaskInfo, generationEvidence []byte) error {
	if task == nil || generationResult == nil || strings.TrimSpace(generationResult.Url) == "" {
		return errors.New("Seedance generation result is incomplete")
	}
	order, err := model.GetSeedanceOrderByTaskID(task.TaskID)
	if err != nil {
		return err
	}
	snapshot, err := parseSeedancePricingSnapshot(order)
	if err != nil {
		return err
	}
	if generationResult.Duration <= 0 || math.IsNaN(generationResult.Duration) || math.IsInf(generationResult.Duration, 0) {
		return errors.New("Seedance upstream result is missing actual duration")
	}
	if generationResult.OutputFPS > 0 && order.OutputFPS > 0 && generationResult.OutputFPS != order.OutputFPS {
		return fmt.Errorf("Seedance upstream output FPS %d does not match the frozen sale model FPS %d", generationResult.OutputFPS, order.OutputFPS)
	}
	if err := settleSeedanceOrderForActualDuration(order, snapshot, generationResult.Duration); err != nil {
		return err
	}
	updates := map[string]any{
		"actual_duration_millis":             order.ActualDurationMillis,
		"model_sale_micro_rmb":               order.ModelSaleMicroRMB,
		"super_resolution_cost_micro_rmb":    order.SuperResolutionCostMicroRMB,
		"service_charge_total_micro_rmb":     order.ServiceChargeTotalMicroRMB,
		"volcengine_estimated_micro_rmb":     order.VolcengineEstimatedMicroRMB,
		"new_api_estimated_profit_micro_rmb": order.NewAPIEstimatedProfitMicroRMB,
		"updated_at":                         time.Now().Unix(),
	}
	if err := model.DB.Model(&model.SeedanceOrder{}).
		Where("id = ? AND order_status = ?", order.ID, model.SeedanceOrderGenerationProcessing).
		Updates(updates).Error; err != nil {
		return err
	}
	requiresEnhancement := snapshot.EnhancementRequired || order.EnhancementModelID != nil || snapshot.ProviderID > 0 || strings.TrimSpace(order.EnhancementProviderSnapshotEncrypted) != ""
	if !requiresEnhancement {
		return completeSeedanceOrderWithoutEnhancement(ctx, task, order, generationResult, generationEvidence)
	}
	return StartSeedanceEnhancement(ctx, task, generationResult, generationEvidence)
}

func settleSeedanceOrderForActualDuration(order *model.SeedanceOrder, snapshot *seedancePricingSnapshot, durationSeconds float64) error {
	if durationSeconds > float64(math.MaxInt64)/1000 {
		return errors.New("Seedance actual duration is outside the supported range")
	}
	durationMillis := int64(math.Round(durationSeconds * 1000))
	if durationMillis <= 0 {
		return errors.New("Seedance actual duration must be positive")
	}
	if order.RequestedDurationMillis == 0 {
		// Historical in-flight rows have only fixed order totals. Record the
		// observed duration, but do not reinterpret those totals as unit prices.
		order.ActualDurationMillis = durationMillis
		return nil
	}
	saleBeforeDiscount, err := model.CalculateSeedanceTimedAmount(order.SaleUnitPriceMicroRMB, durationMillis)
	if err != nil {
		return err
	}
	groupRatio := snapshot.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1
	}
	discountedSale := float64(saleBeforeDiscount) * groupRatio
	if math.IsNaN(discountedSale) || math.IsInf(discountedSale, 0) || discountedSale < 0 || discountedSale > math.MaxInt64 {
		return errors.New("Seedance discounted sale is outside the supported range")
	}
	baseUnitCost := snapshot.BaseUnitCostMicroRMB
	baseCost := order.VolcengineEstimatedMicroRMB
	if baseUnitCost > 0 {
		baseCost, err = model.CalculateSeedanceTimedAmount(baseUnitCost, durationMillis)
		if err != nil {
			return err
		}
	}
	superResolutionCost, err := model.CalculateSeedanceTimedAmount(order.SuperResolutionUnitCostMicroRMB, durationMillis)
	if err != nil {
		return err
	}
	sale := int64(math.Round(discountedSale))
	profit, err := model.CalculateSeedanceProfit(sale, superResolutionCost, baseCost)
	if err != nil {
		return err
	}
	order.ActualDurationMillis = durationMillis
	order.ModelSaleMicroRMB = sale
	order.SuperResolutionCostMicroRMB = superResolutionCost
	order.ServiceChargeTotalMicroRMB = superResolutionCost
	order.VolcengineEstimatedMicroRMB = baseCost
	order.NewAPIEstimatedProfitMicroRMB = profit
	return nil
}

func seedanceActualQuota(task *model.Task, saleMicroRMB int64) int {
	if task == nil {
		return 0
	}
	if task.PrivateData.BillingContext == nil {
		return task.Quota
	}
	context := task.PrivateData.BillingContext
	if context.USDExchangeRate <= 0 || context.QuotaPerUnit <= 0 {
		return task.Quota
	}
	quota := (float64(saleMicroRMB) / 1_000_000 / context.USDExchangeRate) * context.QuotaPerUnit
	if math.IsNaN(quota) || math.IsInf(quota, 0) || quota < 0 || quota > math.MaxInt {
		return task.Quota
	}
	return int(math.Round(quota))
}

func completeSeedanceOrderWithoutEnhancement(ctx context.Context, task *model.Task, order *model.SeedanceOrder, generationResult *relaycommon.TaskInfo, generationEvidence []byte) error {
	now := time.Now().Unix()
	neutralData, err := seedancePublicTaskData(task, model.TaskStatusSuccess, taskcommon.BuildProxyURL(task.TaskID))
	if err != nil {
		return err
	}
	publicUsage := seedancePublicUsageJSON(generationEvidence)
	neutralData, err = injectSeedancePublicUsage(neutralData, publicUsage)
	if err != nil {
		return err
	}
	privateData := task.PrivateData
	privateData.ResultURL = strings.TrimSpace(generationResult.Url)
	usage := &model.MediaServiceUsage{
		ServiceLineItemID: order.PlatformOrderID + ":video-processing", PlatformOrderID: order.PlatformOrderID,
		ServiceType: model.SeedanceServiceTypeVideoSuperResolution, ProviderType: model.SeedanceProviderDirect,
		ServiceCode: "none", SpecificationJSON: "{}", SpecificationVersion: "none",
		AttemptID: order.PlatformOrderID + ":enhancement:not-required", Status: model.SeedanceUsageSucceeded,
		ChargeMicroRMB: 0, PriceVersion: "none", UsageFactsJSON: "{}",
		UsageEvidenceHash: model.SHA256Evidence("enhancement not required"), Revision: 1,
		StartedAt: now, CompletedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	settledOrder := *order
	settledOrder.FinanceRevision++
	settledOrder.GenerationPublicUsageJSON = publicUsage
	event, outbox, err := buildSeedanceBillingEvent(&settledOrder, usage, model.SeedanceOrderSucceeded, "enhancement not required", now)
	if err != nil {
		return err
	}
	won := false
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		transition := tx.Model(&model.SeedanceOrder{}).
			Where("id = ? AND order_status = ?", order.ID, model.SeedanceOrderGenerationProcessing).
			Updates(seedanceTerminalOrderUpdates(order, model.SeedanceOrderSucceeded, now, map[string]any{
				"generation_completed_at": now, "generation_public_usage": publicUsage,
				"actual_duration_millis":          order.ActualDurationMillis,
				"model_sale_micro_rmb":            order.ModelSaleMicroRMB,
				"super_resolution_cost_micro_rmb": int64(0), "service_charge_total_micro_rmb": int64(0),
				"volcengine_estimated_micro_rmb":     order.VolcengineEstimatedMicroRMB,
				"new_api_estimated_profit_micro_rmb": order.NewAPIEstimatedProfitMicroRMB,
			}))
		if transition.Error != nil || transition.RowsAffected == 0 {
			return transition.Error
		}
		won = true
		if err := tx.Model(&model.SeedanceAttempt{}).
			Where("platform_order_id = ? AND stage = ?", order.PlatformOrderID, "GENERATION").
			Updates(map[string]any{"status": model.SeedanceUsageSucceeded, "completed_at": now, "response_evidence_hash": model.SHA256Evidence(string(generationEvidence)), "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(usage).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Task{}).Where("id = ? AND status NOT IN ?", task.ID, []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure}).Updates(map[string]any{
			"status": model.TaskStatusSuccess, "progress": taskcommon.ProgressComplete, "finish_time": now,
			"fail_reason": "", "private_data": privateData, "data": neutralData, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		return tx.Create(outbox).Error
	})
	if err == nil && won {
		RecalculateTaskQuota(ctx, task, seedanceActualQuota(task, order.ModelSaleMicroRMB), "Seedance actual duration settlement")
	}
	return err
}

func StartSeedanceEnhancement(ctx context.Context, task *model.Task, generationResult *relaycommon.TaskInfo, generationEvidence []byte) error {
	if task == nil || generationResult == nil || strings.TrimSpace(generationResult.Url) == "" {
		return errors.New("Seedance generation result is incomplete")
	}
	order, err := model.GetSeedanceOrderByTaskID(task.TaskID)
	if err != nil {
		return err
	}
	snapshot, err := parseSeedancePricingSnapshot(order)
	if err != nil {
		return err
	}
	provider, err := model.ResolveSeedanceProviderForOrder(order)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	attemptID := order.PlatformOrderID + ":enhancement:1"
	lineItemID := order.PlatformOrderID + ":video-processing"
	usageFacts, err := common.Marshal(map[string]any{
		"input_url":                generationResult.Url,
		"generation_evidence_hash": model.SHA256Evidence(string(generationEvidence)),
		"actual_duration_millis":   order.ActualDurationMillis,
		"output_fps":               order.OutputFPS,
	})
	if err != nil {
		return err
	}
	neutralData, err := seedancePublicTaskData(task, model.TaskStatusInProgress, "")
	if err != nil {
		return err
	}
	task.Data = neutralData
	publicUsage := seedancePublicUsageJSON(generationEvidence)
	charge := order.SuperResolutionCostMicroRMB
	if order.EnhancementModelID == nil && charge == 0 {
		charge = snapshot.ServiceChargeMicroRMB
	}
	providerCost := snapshot.ProviderCostMicroRMB
	if providerCost != nil && order.ActualDurationMillis > 0 {
		actualProviderCost := charge
		providerCost = &actualProviderCost
	}
	usage := &model.MediaServiceUsage{
		ServiceLineItemID:    lineItemID,
		PlatformOrderID:      order.PlatformOrderID,
		ServiceType:          model.SeedanceServiceTypeVideoSuperResolution,
		ProviderType:         provider.ProviderType,
		ProviderID:           provider.ID,
		ServiceCode:          snapshot.ServiceCode,
		SpecificationJSON:    snapshot.SpecificationJSON,
		SpecificationVersion: snapshot.SpecificationVersion,
		AttemptID:            attemptID,
		Status:               model.SeedanceUsagePending,
		ChargeMicroRMB:       charge,
		PriceVersion:         snapshot.PricingVersion,
		ProviderCostMicroRMB: providerCost,
		UsageFactsJSON:       string(usageFacts),
		Revision:             1,
		StartedAt:            now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	attempt := &model.SeedanceAttempt{
		PlatformOrderID:      order.PlatformOrderID,
		AttemptID:            attemptID,
		Stage:                "ENHANCEMENT",
		AttemptNo:            1,
		ProviderType:         provider.ProviderType,
		ProviderID:           provider.ID,
		ServiceCode:          snapshot.ServiceCode,
		SpecificationVersion: snapshot.SpecificationVersion,
		Status:               "SUBMITTING",
		RequestHash:          model.SHA256Evidence(string(usageFacts) + snapshot.SpecificationJSON),
		StartedAt:            now,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	transitioned := false
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		transition := tx.Model(&model.SeedanceOrder{}).
			Where("id = ? AND order_status = ?", order.ID, model.SeedanceOrderGenerationProcessing).
			Updates(map[string]any{
				"order_status": model.SeedanceOrderEnhancing, "generation_completed_at": now,
				"generation_public_usage": publicUsage, "updated_at": now,
			})
		if transition.Error != nil {
			return transition.Error
		}
		if transition.RowsAffected == 0 {
			return nil
		}
		transitioned = true
		if err := tx.Model(&model.SeedanceAttempt{}).
			Where("platform_order_id = ? AND stage = ?", order.PlatformOrderID, "GENERATION").
			Updates(map[string]any{"status": model.SeedanceUsageSucceeded, "completed_at": now, "response_evidence_hash": model.SHA256Evidence(string(generationEvidence)), "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(usage).Error; err != nil {
			return err
		}
		if err := tx.Create(attempt).Error; err != nil {
			return err
		}
		return tx.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]any{
			"status": model.TaskStatusInProgress, "progress": seedanceEnhancementProgress, "data": neutralData, "updated_at": now,
		}).Error
	})
	if err != nil {
		return err
	}
	if !transitioned {
		return nil
	}
	order.GenerationPublicUsageJSON = publicUsage
	return submitPendingSeedanceEnhancement(ctx, task, order, usage, provider)
}

func pollSeedanceEnhancement(ctx context.Context, task *model.Task, order *model.SeedanceOrder) error {
	var usage model.MediaServiceUsage
	if err := model.DB.Where("platform_order_id = ?", order.PlatformOrderID).Order("id desc").First(&usage).Error; err != nil {
		return err
	}
	provider, err := model.ResolveSeedanceProviderForOrder(order)
	if err != nil {
		return err
	}
	if strings.TrimSpace(usage.ExecutionTaskID) == "" {
		return submitPendingSeedanceEnhancement(ctx, task, order, &usage, provider)
	}
	executor, err := enhancementProviderFactory(provider)
	if err != nil {
		return err
	}
	result, err := executor.Query(ctx, usage.ExecutionTaskID)
	if err != nil {
		return err
	}
	return applySeedanceEnhancementResult(ctx, task, order, &usage, result)
}

func submitPendingSeedanceEnhancement(ctx context.Context, task *model.Task, order *model.SeedanceOrder, usage *model.MediaServiceUsage, providerConfig *model.MediaEnhancementProvider) error {
	var facts struct {
		InputURL string `json:"input_url"`
	}
	if err := common.UnmarshalJsonStr(usage.UsageFactsJSON, &facts); err != nil {
		return err
	}
	executor, err := enhancementProviderFactory(providerConfig)
	if err != nil {
		return err
	}
	capabilities := enhancementCapabilities(executor)
	if usage.UnknownSubmissionCount > 0 && !enhancementSubmissionRetrySafeAt(capabilities, usage, time.Now().Unix()) {
		// A previous POST may already have created a billable remote task. Without
		// a currently valid idempotency guarantee, sending it again could duplicate
		// both the work and the provider cost, so the attempt waits for manual
		// reconciliation.
		return ErrSeedanceSubmissionNeedsManualReview
	}
	result, err := executor.Submit(ctx, EnhancementSubmitRequest{
		InputURL:          facts.InputURL,
		SpecificationJSON: usage.SpecificationJSON,
		IdempotencyKey:    usage.AttemptID,
	})
	if err != nil {
		if isDefinitiveEnhancementFailure(err) {
			usage.ProviderCostMicroRMB = nil
			usage.FailureReason = enhancementFailureReason(err, "PROVIDER_REJECTED")
			usage.UsageFactsJSON = `{}`
			usage.UsageEvidenceHash = model.SHA256Evidence("provider request rejected")
			return failSeedanceOrder(ctx, task, order, usage)
		}
		// The outcome may be unknown. Keep the same attempt and idempotency key so
		// the next poll can safely retry without creating duplicate work.
		return markSeedanceSubmissionOutcomeUnknown(usage, err)
	}
	if result == nil {
		return markSeedanceSubmissionOutcomeUnknown(usage, errors.New("enhancement provider returned an empty result"))
	}
	if strings.TrimSpace(result.ExecutionTaskID) == "" && result.Status == model.SeedanceUsageFailed {
		usage.ProviderCostMicroRMB = nil
		usage.FailureReason = "PROVIDER_REPORTED_FAILURE"
		return failSeedanceOrder(ctx, task, order, usage)
	}
	if strings.TrimSpace(result.ExecutionTaskID) == "" && result.Status != model.SeedanceUsageSucceeded {
		return markSeedanceSubmissionOutcomeUnknown(usage, errors.New("enhancement provider omitted execution_task_id"))
	}
	if strings.TrimSpace(result.ExecutionTaskID) == "" {
		return applySeedanceEnhancementResult(ctx, task, order, usage, result)
	}
	now := time.Now().Unix()
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.MediaServiceUsage{}).Where("id = ?", usage.ID).Updates(map[string]any{
			"execution_task_id": result.ExecutionTaskID, "status": model.SeedanceUsageRunning,
			"failure_reason": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.SeedanceAttempt{}).Where("attempt_id = ?", usage.AttemptID).Updates(map[string]any{
			"external_task_id": result.ExecutionTaskID, "status": model.SeedanceUsageRunning, "updated_at": now,
		}).Error
	}); err != nil {
		return err
	}
	usage.ExecutionTaskID = result.ExecutionTaskID
	usage.Status = model.SeedanceUsageRunning
	usage.FailureReason = ""
	return applySeedanceEnhancementResult(ctx, task, order, usage, result)
}

func markSeedanceSubmissionOutcomeUnknown(usage *model.MediaServiceUsage, cause error) error {
	if usage == nil {
		return cause
	}
	usage.FailureReason = model.SeedanceSubmissionOutcomeUnknown
	usage.UnknownSubmissionCount++
	if usage.ID == 0 {
		return cause
	}
	now := time.Now().Unix()
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.MediaServiceUsage{}).Where("id = ?", usage.ID).Updates(map[string]any{
			"failure_reason":           model.SeedanceSubmissionOutcomeUnknown,
			"unknown_submission_count": gorm.Expr("unknown_submission_count + 1"),
			"updated_at":               now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&model.SeedanceAttempt{}).Where("attempt_id = ?", usage.AttemptID).Updates(map[string]any{
			"status": model.SeedanceSubmissionOutcomeUnknown, "updated_at": now,
		}).Error
	}); err != nil {
		return fmt.Errorf("record unknown enhancement submission outcome: %w", err)
	}
	return cause
}

func applySeedanceEnhancementResult(ctx context.Context, task *model.Task, order *model.SeedanceOrder, usage *model.MediaServiceUsage, result *EnhancementResult) error {
	if result == nil || result.Status == model.SeedanceUsageRunning || result.Status == model.SeedanceUsagePending {
		return nil
	}
	if result.Status == model.SeedanceUsageSucceeded && strings.TrimSpace(result.ResultURL) != "" {
		return completeSeedanceOrder(ctx, task, order, usage, result)
	}
	usage.FailureReason = strings.TrimSpace(result.FailureReason)
	if usage.FailureReason == "" {
		usage.FailureReason = "PROVIDER_REPORTED_FAILURE"
	}
	return failSeedanceOrder(ctx, task, order, usage)
}

func completeSeedanceOrder(ctx context.Context, task *model.Task, order *model.SeedanceOrder, usage *model.MediaServiceUsage, result *EnhancementResult) error {
	now := time.Now().Unix()
	neutralData, err := seedancePublicTaskData(task, model.TaskStatusSuccess, taskcommon.BuildProxyURL(task.TaskID))
	if err != nil {
		return err
	}
	neutralData, err = injectSeedancePublicUsage(neutralData, order.GenerationPublicUsageJSON)
	if err != nil {
		return err
	}
	privateData := task.PrivateData
	privateData.ResultURL = strings.TrimSpace(result.ResultURL)
	completedUsage := *usage
	completedUsage.Status = model.SeedanceUsageSucceeded
	if strings.TrimSpace(result.ExecutionTaskID) != "" {
		completedUsage.ExecutionTaskID = result.ExecutionTaskID
	}
	usageEvidence := result.UsageEvidenceJSON
	if strings.TrimSpace(usageEvidence) == "" {
		usageEvidence = "{}"
	}
	settledOrder := *order
	settledOrder.FinanceRevision++
	event, outbox, err := buildSeedanceBillingEvent(&settledOrder, &completedUsage, model.SeedanceOrderSucceeded, usageEvidence, now)
	if err != nil {
		return err
	}
	won := false
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		transition := tx.Model(&model.SeedanceOrder{}).
			Where("id = ? AND order_status = ?", order.ID, model.SeedanceOrderEnhancing).
			Updates(seedanceTerminalOrderUpdates(order, model.SeedanceOrderSucceeded, now))
		if transition.Error != nil || transition.RowsAffected == 0 {
			return transition.Error
		}
		won = true
		if err := tx.Model(&model.MediaServiceUsage{}).Where("id = ?", usage.ID).Updates(map[string]any{
			"status": model.SeedanceUsageSucceeded, "execution_task_id": completedUsage.ExecutionTaskID,
			"usage_evidence_hash": model.SHA256Evidence(usageEvidence), "usage_facts": usageEvidence,
			"failure_reason": "", "completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.SeedanceAttempt{}).Where("attempt_id = ?", usage.AttemptID).Updates(map[string]any{
			"status": model.SeedanceUsageSucceeded, "external_task_id": completedUsage.ExecutionTaskID,
			"response_evidence_hash": model.SHA256Evidence(usageEvidence), "completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Task{}).Where("id = ? AND status NOT IN ?", task.ID, []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure}).Updates(map[string]any{
			"status": model.TaskStatusSuccess, "progress": taskcommon.ProgressComplete, "finish_time": now,
			"fail_reason": "", "private_data": privateData, "data": neutralData, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		return tx.Create(outbox).Error
	})
	if err == nil && won {
		RecalculateTaskQuota(ctx, task, seedanceActualQuota(task, order.ModelSaleMicroRMB), "Seedance actual duration settlement")
	}
	return err
}

func failSeedanceOrder(ctx context.Context, task *model.Task, order *model.SeedanceOrder, usage *model.MediaServiceUsage) error {
	now := time.Now().Unix()
	neutralData, err := seedancePublicTaskData(task, model.TaskStatusFailure, "")
	if err != nil {
		return err
	}
	failedUsage := *usage
	incurredSuperResolutionCost := int64(0)
	if order.EnhancementModelID != nil && failedUsage.ProviderCostMicroRMB != nil {
		incurredSuperResolutionCost = order.SuperResolutionCostMicroRMB
	}
	failedUsage.ChargeMicroRMB = incurredSuperResolutionCost
	failedUsage.Status = model.SeedanceUsageFailed
	if strings.TrimSpace(failedUsage.FailureReason) == "" {
		failedUsage.FailureReason = "PROVIDER_REPORTED_FAILURE"
	}
	failureProfit, err := model.CalculateSeedanceProfit(0, incurredSuperResolutionCost, order.VolcengineEstimatedMicroRMB)
	if err != nil {
		return err
	}
	settledOrder := *order
	settledOrder.FinanceRevision++
	settledOrder.ModelSaleMicroRMB = 0
	settledOrder.SuperResolutionCostMicroRMB = incurredSuperResolutionCost
	settledOrder.ServiceChargeTotalMicroRMB = incurredSuperResolutionCost
	settledOrder.NewAPIEstimatedProfitMicroRMB = failureProfit
	event, outbox, err := buildSeedanceBillingEvent(&settledOrder, &failedUsage, model.SeedanceOrderFailed, "{}", now)
	if err != nil {
		return err
	}
	won := false
	var customerRefund *model.SeedanceCustomerRefund
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		transition := tx.Model(&model.SeedanceOrder{}).
			Where("id = ? AND order_status = ?", order.ID, model.SeedanceOrderEnhancing).
			Updates(seedanceTerminalOrderUpdates(order, model.SeedanceOrderFailed, now, map[string]any{
				"model_sale_micro_rmb": int64(0), "super_resolution_cost_micro_rmb": incurredSuperResolutionCost,
				"service_charge_total_micro_rmb":     incurredSuperResolutionCost,
				"new_api_estimated_profit_micro_rmb": failureProfit,
			}))
		if transition.Error != nil || transition.RowsAffected == 0 {
			return transition.Error
		}
		won = true
		if err := tx.Model(&model.MediaServiceUsage{}).Where("id = ?", usage.ID).Updates(map[string]any{
			"status": model.SeedanceUsageFailed, "charge_micro_rmb": incurredSuperResolutionCost,
			"provider_cost_micro_rmb": failedUsage.ProviderCostMicroRMB,
			"failure_reason":          failedUsage.FailureReason,
			"completed_at":            now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.SeedanceAttempt{}).Where("attempt_id = ?", usage.AttemptID).Updates(map[string]any{
			"status": model.SeedanceUsageFailed, "completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Task{}).Where("id = ? AND status NOT IN ?", task.ID, []model.TaskStatus{model.TaskStatusSuccess, model.TaskStatusFailure}).Updates(map[string]any{
			"status": model.TaskStatusFailure, "progress": taskcommon.ProgressComplete, "finish_time": now,
			"fail_reason": seedanceGenericFailureMessage, "data": neutralData, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(event).Error; err != nil {
			return err
		}
		if err := tx.Create(outbox).Error; err != nil {
			return err
		}
		customerRefund, err = model.QueueSeedanceCustomerRefundTx(tx, order, task, seedanceGenericFailureMessage)
		return err
	})
	if err == nil && won && customerRefund != nil {
		dispatchSeedanceCustomerRefund(ctx, customerRefund.ID)
	}
	return err
}

// ProcessSeedanceCustomerRefunds repairs the crash window after a terminal
// order transaction. The balance reversal is guarded by the durable refund
// row, while the user log has its own unique event key because LOG_DB may be a
// separate database.
func ProcessSeedanceCustomerRefunds(ctx context.Context, limit int) {
	if limit <= 0 {
		limit = 20
	}
	var refunds []model.SeedanceCustomerRefund
	if err := model.DB.Where(
		"status = ? OR (status = ? AND (log_recorded_at = 0 OR finance_settlement_recorded_at = 0))",
		model.SeedanceCustomerRefundReady,
		model.SeedanceCustomerRefundApplied,
	).Order("id ASC").Limit(limit).Find(&refunds).Error; err != nil {
		logger.LogError(ctx, "load Seedance customer refunds: "+err.Error())
		return
	}
	for i := range refunds {
		processSeedanceCustomerRefund(ctx, refunds[i].ID)
	}
}

func processSeedanceCustomerRefund(ctx context.Context, refundID int64) {
	refund, _, err := model.ApplySeedanceCustomerRefund(refundID)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("apply Seedance customer refund %d: %v", refundID, err))
		return
	}
	if refund == nil {
		return
	}
	var task model.Task
	if err := model.DB.Where("task_id = ?", refund.NewAPITaskID).First(&task).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("load task for Seedance customer refund %d: %v", refundID, err))
		return
	}
	if refund.LogRecordedAt == 0 {
		other := taskBillingOther(&task)
		other["task_id"] = task.TaskID
		other["reason"] = refund.Reason
		_, logErr := model.RecordTaskBillingLogOnce("seedance-customer-refund:"+refund.PlatformOrderID, model.RecordTaskBillingLogParams{
			UserId: task.UserId, LogType: model.LogTypeRefund, Content: "", ChannelId: task.ChannelId,
			ModelName: taskModelName(&task), Quota: refund.Quota, TokenId: task.PrivateData.TokenId,
			Group: task.Group, Other: other,
		})
		if logErr != nil {
			logger.LogError(ctx, fmt.Sprintf("record Seedance customer refund log %d: %v", refundID, logErr))
			return
		}
		if err := model.MarkSeedanceCustomerRefundLogRecorded(refundID); err != nil {
			logger.LogError(ctx, fmt.Sprintf("mark Seedance customer refund log %d: %v", refundID, err))
			return
		}
	}
	if refund.FinanceSettlementRecordedAt == 0 {
		if err := RecordTaskAIPDDFinanceSettlement(&task, 0, "REFUNDED"); err != nil {
			logger.LogWarn(ctx, "record AIPDD task refund settlement failed: "+err.Error())
			return
		}
		if err := model.MarkSeedanceCustomerRefundFinanceRecorded(refundID); err != nil {
			logger.LogError(ctx, fmt.Sprintf("mark Seedance customer refund finance settlement %d: %v", refundID, err))
		}
	}
}

func seedanceTerminalOrderUpdates(order *model.SeedanceOrder, status string, now int64, extra ...map[string]any) map[string]any {
	updates := map[string]any{
		"order_status":     status,
		"sync_status":      model.SeedanceSyncReady,
		"finance_revision": order.FinanceRevision + 1,
		"completed_at":     now,
		"updated_at":       now,
	}
	if order != nil && strings.TrimSpace(order.CallbackURLEncrypted) != "" &&
		(order.CallbackStatus == model.SeedanceCallbackWaiting || order.CallbackStatus == "") {
		updates["callback_status"] = model.SeedanceCallbackReady
		updates["callback_next_attempt_at"] = now
	}
	for _, values := range extra {
		for key, value := range values {
			updates[key] = value
		}
	}
	return updates
}

func seedancePublicTaskData(task *model.Task, status model.TaskStatus, contentURL string) ([]byte, error) {
	payload := map[string]any{}
	if task != nil && len(task.Data) > 0 {
		_ = common.Unmarshal(task.Data, &payload)
	}
	sanitizeSeedancePublicValue(payload)
	delete(payload, "usage")
	delete(payload, "output")
	delete(payload, "content")
	for _, key := range []string{
		"video_url", "videoUrl", "result_video_url", "resultVideoUrl", "download_url", "downloadUrl", "url",
	} {
		delete(payload, key)
	}
	publicStatus := "running"
	if status == model.TaskStatusSuccess {
		publicStatus = "succeeded"
	} else if status == model.TaskStatusFailure {
		publicStatus = "failed"
	}
	payload["id"] = task.TaskID
	payload["model"] = task.Properties.OriginModelName
	payload["status"] = publicStatus
	if _, ok := payload["created_at"]; !ok && task.CreatedAt > 0 {
		payload["created_at"] = task.CreatedAt
	}
	if task.UpdatedAt > 0 {
		payload["updated_at"] = task.UpdatedAt
	}
	if contentURL != "" {
		payload["content"] = map[string]any{"video_url": contentURL}
	}
	if status == model.TaskStatusFailure {
		payload["error"] = map[string]any{"code": "video_processing_failed", "message": seedanceGenericFailureMessage}
	}
	return common.Marshal(payload)
}

func seedancePublicUsageJSON(generationEvidence []byte) string {
	var payload map[string]any
	if common.Unmarshal(generationEvidence, &payload) != nil {
		return ""
	}
	usage, ok := payload["usage"].(map[string]any)
	if !ok {
		return ""
	}
	public := map[string]any{}
	for _, key := range []string{"completion_tokens", "total_tokens"} {
		if value, exists := usage[key]; exists {
			public[key] = value
		}
	}
	if len(public) == 0 {
		return ""
	}
	encoded, err := common.Marshal(public)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func injectSeedancePublicUsage(data []byte, usageJSON string) ([]byte, error) {
	if strings.TrimSpace(usageJSON) == "" {
		return data, nil
	}
	var payload map[string]any
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	var usage map[string]any
	if err := common.UnmarshalJsonStr(usageJSON, &usage); err != nil {
		return nil, err
	}
	payload["usage"] = usage
	return common.Marshal(payload)
}

func sanitizeSeedancePublicValue(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if seedancePublicTextForbidden(key) || !sanitizeSeedancePublicValue(child) {
				delete(typed, key)
			}
		}
	case []any:
		filtered := typed[:0]
		for _, child := range typed {
			if sanitizeSeedancePublicValue(child) {
				filtered = append(filtered, child)
			}
		}
		for i := len(filtered); i < len(typed); i++ {
			typed[i] = nil
		}
		typed = filtered
	case string:
		return !seedancePublicTextForbidden(typed)
	}
	return true
}

func seedancePublicTextForbidden(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, token := range []string{
		"enhance", "super_resolution", "super-resolution", "super resolution", "upscale", "provider", "byok",
		"aipdd", "credential", "api_key", "billing_", "finance_", "service_charge", "超分", "增强",
	} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}

func buildSeedanceBillingEvent(order *model.SeedanceOrder, usage *model.MediaServiceUsage, orderStatus string, usageEvidence string, now int64) (*model.ServiceBillingEvent, *model.ServiceBillingOutbox, error) {
	payload := map[string]any{
		"schema_version": 1,
		"event_id":       model.GenerateSeedanceOrderID(),
		"event_type":     "SERVICE_SETTLEMENT_UPDATED",
		"revision":       usage.Revision,
		"occurred_at":    time.Unix(now, 0).Format(time.RFC3339),
		"source_order": map[string]any{
			"source_type":                   "NEWAPI_SEEDANCE",
			"source_revision":               order.FinanceRevision,
			"platform_order_id":             order.PlatformOrderID,
			"newapi_task_id":                order.NewAPITaskID,
			"channel_id":                    order.ChannelID,
			"model":                         order.Model,
			"status":                        orderStatus,
			"model_sale_rmb":                microRMBString(order.ModelSaleMicroRMB),
			"super_resolution_cost_rmb":     microRMBString(order.SuperResolutionCostMicroRMB),
			"volcengine_cost_status":        order.VolcengineCostStatus,
			"volcengine_cost_estimated_rmb": microRMBString(order.VolcengineEstimatedMicroRMB),
			"volcengine_cost_actual_rmb":    optionalMicroRMBString(order.VolcengineActualMicroRMB),
			"newapi_profit_estimated_rmb":   microRMBString(order.NewAPIEstimatedProfitMicroRMB),
			"newapi_profit_actual_rmb":      optionalMicroRMBString(order.NewAPIActualProfitMicroRMB),
		},
		"service_usage": map[string]any{
			"service_line_item_id":  usage.ServiceLineItemID,
			"service_type":          usage.ServiceType,
			"provider_type":         usage.ProviderType,
			"service_code":          usage.ServiceCode,
			"execution_task_id":     usage.ExecutionTaskID,
			"status":                usage.Status,
			"started_at":            time.Unix(usage.StartedAt, 0).Format(time.RFC3339),
			"completed_at":          time.Unix(now, 0).Format(time.RFC3339),
			"charge_rmb":            microRMBString(usage.ChargeMicroRMB),
			"provider_cost_rmb":     optionalMicroRMBString(usage.ProviderCostMicroRMB),
			"price_version":         usage.PriceVersion,
			"pricing_snapshot_hash": order.PricingSnapshotHash,
			"usage_evidence_hash":   model.SHA256Evidence(usageEvidence),
		},
	}
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	eventID := payload["event_id"].(string)
	event := &model.ServiceBillingEvent{
		EventID: eventID, PlatformOrderID: order.PlatformOrderID, ServiceLineItemID: usage.ServiceLineItemID,
		Revision: usage.Revision, EventType: "SERVICE_SETTLEMENT_UPDATED", PayloadJSON: string(payloadBytes),
		PayloadHash: model.SHA256Evidence(string(payloadBytes)), CreatedAt: now,
	}
	outbox := &model.ServiceBillingOutbox{
		EventID: eventID, Status: model.SeedanceSyncReady, NextAttemptAt: now, CreatedAt: now, UpdatedAt: now,
	}
	return event, outbox, nil
}

// QueueSeedanceCostRevisionEvents propagates a newly confirmed bill allocation
// through the same idempotent finance outbox. The bill-item/order key prevents
// a manual import replay from creating another revision.
func QueueSeedanceCostRevisionEvents(ctx context.Context, billItemID int64) error {
	var billItem model.SeedanceVolcengineBillItem
	if err := model.DB.Where("id = ? AND allocation_status = ?", billItemID, model.SeedanceBillAllocated).First(&billItem).Error; err != nil {
		return err
	}
	var allocations []model.SeedanceCostAllocation
	if err := model.DB.Where("bill_item_id = ?", billItemID).Find(&allocations).Error; err != nil {
		return err
	}
	if len(allocations) == 0 {
		return errors.New("allocated bill item has no cost allocations")
	}
	for _, allocation := range allocations {
		sourceKey := fmt.Sprintf("volc-bill:%d:%s", billItemID, allocation.PlatformOrderID)
		err := model.DB.Transaction(func(tx *gorm.DB) error {
			var count int64
			if err := tx.Model(&model.ServiceBillingEvent{}).Where("source_revision_key = ?", sourceKey).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
			var order model.SeedanceOrder
			if err := tx.Where("platform_order_id = ?", allocation.PlatformOrderID).First(&order).Error; err != nil {
				return err
			}
			var usage model.MediaServiceUsage
			if err := tx.Where("platform_order_id = ?", allocation.PlatformOrderID).Order("id desc").First(&usage).Error; err != nil {
				return err
			}
			usage.Revision++
			now := time.Now().Unix()
			event, outbox, err := buildSeedanceBillingEvent(&order, &usage, order.OrderStatus, usage.UsageFactsJSON, now)
			if err != nil {
				return err
			}
			event.SourceRevisionKey = &sourceKey
			revisionUpdate := tx.Model(&model.MediaServiceUsage{}).Where("id = ? AND revision = ?", usage.ID, usage.Revision-1).
				Update("revision", usage.Revision)
			if revisionUpdate.Error != nil {
				return revisionUpdate.Error
			}
			if revisionUpdate.RowsAffected != 1 {
				return errors.New("service revision changed concurrently")
			}
			if err := tx.Create(event).Error; err != nil {
				return err
			}
			if err := tx.Create(outbox).Error; err != nil {
				return err
			}
			return tx.Model(&model.SeedanceOrder{}).Where("id = ?", order.ID).
				Updates(map[string]any{"sync_status": model.SeedanceSyncReady, "updated_at": now}).Error
		})
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("queue Seedance cost revision for order %s failed: %v", allocation.PlatformOrderID, err))
			return err
		}
	}
	return model.DB.Model(&model.SeedanceVolcengineBillItem{}).
		Where("id = ? AND allocation_status = ?", billItem.ID, model.SeedanceBillAllocated).
		Update("revision_event_queued_at", time.Now().Unix()).Error
}

// ProcessSeedanceCostRevisionQueue repairs the narrow crash window between a
// committed cost allocation and creation of its finance outbox revision.
// QueueSeedanceCostRevisionEvents is idempotent, so partial prior work is safe.
func ProcessSeedanceCostRevisionQueue(ctx context.Context, limit int) {
	if limit <= 0 {
		limit = 20
	}
	var items []model.SeedanceVolcengineBillItem
	if err := model.DB.Where("allocation_status = ? AND revision_event_queued_at = 0", model.SeedanceBillAllocated).
		Order("id asc").Limit(limit).Find(&items).Error; err != nil {
		logger.LogError(ctx, "load Seedance cost revision queue: "+err.Error())
		return
	}
	for i := range items {
		if err := QueueSeedanceCostRevisionEvents(ctx, items[i].ID); err != nil {
			logger.LogError(ctx, fmt.Sprintf("queue Seedance cost revision bill item %d: %v", items[i].ID, err))
		}
	}
}

func microRMBString(value int64) string {
	sign := ""
	magnitude := uint64(value)
	if value < 0 {
		sign = "-"
		magnitude = uint64(-(value + 1)) + 1
	}
	return fmt.Sprintf("%s%d.%06d", sign, magnitude/1_000_000, magnitude%1_000_000)
}

func optionalMicroRMBString(value *int64) any {
	if value == nil {
		return nil
	}
	return microRMBString(*value)
}

func ProcessSeedanceBillingOutbox(ctx context.Context, limit int) {
	if limit <= 0 {
		limit = 20
	}
	owner := uuid.NewString()
	now := time.Now().Unix()
	var candidates []model.ServiceBillingOutbox
	if err := model.DB.Table("service_billing_outboxes").Select("service_billing_outboxes.*").
		Joins("JOIN service_billing_events ON service_billing_events.event_id = service_billing_outboxes.event_id").
		Joins("JOIN seedance_orders ON seedance_orders.platform_order_id = service_billing_events.platform_order_id").
		Where("(service_billing_outboxes.status IN ? AND service_billing_outboxes.next_attempt_at <= ? AND (service_billing_outboxes.lease_until = 0 OR service_billing_outboxes.lease_until < ?)) OR (service_billing_outboxes.status = ? AND service_billing_outboxes.lease_until < ?)",
			[]string{model.SeedanceSyncReady, model.SeedanceSyncRetryWait}, now, now, model.SeedanceSyncSending, now).
		Order("service_billing_outboxes.id asc").Limit(limit).Find(&candidates).Error; err != nil {
		logger.LogError(ctx, "claim Seedance billing outbox: "+err.Error())
		return
	}
	for i := range candidates {
		item := &candidates[i]
		claim := model.DB.Model(&model.ServiceBillingOutbox{}).
			Where(`EXISTS (
				SELECT 1 FROM service_billing_events claim_events
				JOIN seedance_orders claim_orders ON claim_orders.platform_order_id = claim_events.platform_order_id
				WHERE claim_events.event_id = service_billing_outboxes.event_id
			)`).
			Where("id = ? AND ((status IN ? AND next_attempt_at <= ? AND (lease_until = 0 OR lease_until < ?)) OR (status = ? AND lease_until < ?))", item.ID,
				[]string{model.SeedanceSyncReady, model.SeedanceSyncRetryWait}, now, now, model.SeedanceSyncSending, now).
			Updates(map[string]any{"status": model.SeedanceSyncSending, "lease_owner": owner, "lease_until": now + seedanceOutboxLeaseSeconds, "updated_at": now})
		if claim.Error != nil || claim.RowsAffected == 0 {
			continue
		}
		if err := deliverSeedanceBillingOutbox(ctx, item, owner); err != nil {
			logger.LogWarn(ctx, "deliver Seedance billing event: "+err.Error())
		}
	}
}

// ReviseSeedanceDeadLetter rebuilds the latest rejected finance event from the
// authoritative order and usage rows. The old event stays immutable for audit;
// a source revision key makes repeated administrator clicks idempotent.
func ReviseSeedanceDeadLetter(eventID string, actorUserID int) (*model.ServiceBillingEvent, bool, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return nil, false, errors.New("event_id is required")
	}
	sourceRevisionKey := "manual-dead-letter:" + eventID
	var result *model.ServiceBillingEvent
	created := false
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var existing model.ServiceBillingEvent
		if err := tx.Where("source_revision_key = ?", sourceRevisionKey).First(&existing).Error; err == nil {
			result = &existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var previousEvent model.ServiceBillingEvent
		if err := tx.Where("event_id = ?", eventID).First(&previousEvent).Error; err != nil {
			return err
		}
		var previousOutbox model.ServiceBillingOutbox
		if err := tx.Where("event_id = ?", eventID).First(&previousOutbox).Error; err != nil {
			return err
		}
		if previousOutbox.Status != model.SeedanceSyncDeadLetter {
			return errors.New("only a dead-letter event can be revised")
		}
		var order model.SeedanceOrder
		if err := tx.Where("platform_order_id = ?", previousEvent.PlatformOrderID).First(&order).Error; err != nil {
			return err
		}
		var usage model.MediaServiceUsage
		if err := tx.Where("service_line_item_id = ?", previousEvent.ServiceLineItemID).First(&usage).Error; err != nil {
			return err
		}
		if usage.Revision != previousEvent.Revision {
			return errors.New("dead-letter event is no longer the latest service revision")
		}
		beforeRevision := usage.Revision
		usage.Revision++
		now := time.Now().Unix()
		nextEvent, nextOutbox, err := buildSeedanceBillingEvent(&order, &usage, order.OrderStatus, usage.UsageFactsJSON, now)
		if err != nil {
			return err
		}
		nextEvent.SourceRevisionKey = &sourceRevisionKey
		updated := tx.Model(&model.MediaServiceUsage{}).
			Where("id = ? AND revision = ?", usage.ID, beforeRevision).
			Update("revision", usage.Revision)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("service revision changed concurrently")
		}
		if err := tx.Create(nextEvent).Error; err != nil {
			return err
		}
		if err := tx.Create(nextOutbox).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "BILLING_OUTBOX", ResourceID: nextEvent.EventID,
			Action: "CREATE_REVISION", BeforeVersion: strconv.Itoa(beforeRevision), AfterVersion: strconv.Itoa(usage.Revision),
			ChangeSummary: "created a higher finance revision from a dead-letter event", CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		result = nextEvent
		created = true
		return nil
	})
	return result, created, err
}

// ProcessSeedanceCallbacks delivers the callback captured at submission only
// after the complete private workflow reaches a public terminal state. A lease
// and CAS updates make retries safe across multiple NewAPI workers.
func ProcessSeedanceCallbacks(ctx context.Context, limit int) {
	if limit <= 0 {
		limit = 20
	}
	now := time.Now().Unix()
	var orders []model.SeedanceOrder
	if err := model.DB.Where(
		"((callback_status IN ? AND callback_next_attempt_at <= ?) OR (callback_status = ? AND callback_lease_until < ?)) AND callback_url_encrypted <> ?",
		[]string{model.SeedanceCallbackReady, model.SeedanceCallbackRetryWait}, now, model.SeedanceCallbackSending, now, "",
	).Order("callback_next_attempt_at asc").Limit(limit).Find(&orders).Error; err != nil {
		logger.LogError(ctx, "load Seedance callback queue failed: "+err.Error())
		return
	}
	owner := "seedance-callback:" + uuid.NewString()
	for i := range orders {
		order := &orders[i]
		leaseUntil := time.Now().Unix() + seedanceOutboxLeaseSeconds
		claimNow := time.Now().Unix()
		claim := model.DB.Model(&model.SeedanceOrder{}).
			Where("id = ? AND ((callback_status IN ? AND callback_next_attempt_at <= ?) OR (callback_status = ? AND callback_lease_until < ?))", order.ID,
				[]string{model.SeedanceCallbackReady, model.SeedanceCallbackRetryWait}, claimNow,
				model.SeedanceCallbackSending, claimNow).
			Updates(map[string]any{
				"callback_status": model.SeedanceCallbackSending, "callback_lease_owner": owner,
				"callback_lease_until": leaseUntil, "callback_attempt_count": gorm.Expr("callback_attempt_count + 1"),
				"updated_at": time.Now().Unix(),
			})
		if claim.Error != nil || claim.RowsAffected == 0 {
			continue
		}
		order.CallbackAttemptCount++
		deliveryCtx, cancel := context.WithTimeout(ctx, seedanceCallbackTimeout)
		httpStatus, err := deliverSeedanceCallback(deliveryCtx, order)
		cancel()
		if err != nil {
			finishSeedanceCallbackFailure(ctx, order, owner, httpStatus, err)
			continue
		}
		result := model.DB.Model(&model.SeedanceOrder{}).
			Where("id = ? AND callback_status = ? AND callback_lease_owner = ?", order.ID, model.SeedanceCallbackSending, owner).
			Updates(map[string]any{
				"callback_status": model.SeedanceCallbackDelivered, "callback_last_http_status": httpStatus,
				"callback_last_error":  "",
				"callback_lease_owner": "", "callback_lease_until": 0, "updated_at": time.Now().Unix(),
			})
		if result.Error != nil {
			logger.LogError(ctx, "record Seedance callback success: "+result.Error.Error())
		} else if result.RowsAffected == 0 {
			logger.LogError(ctx, fmt.Sprintf("record Seedance callback success lost lease for order %d", order.ID))
		}
	}
}

func deliverSeedanceCallback(ctx context.Context, order *model.SeedanceOrder) (int, error) {
	callbackURL, err := common.DecryptSensitiveValue(order.CallbackURLEncrypted)
	if err != nil {
		return 0, fmt.Errorf("decrypt callback target: %w", err)
	}
	callbackURL = strings.TrimSpace(callbackURL)
	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(callbackURL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp,
		fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList,
		fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return 0, fmt.Errorf("callback target rejected: %w", err)
	}
	var task model.Task
	if err := model.DB.Where("task_id = ?", order.NewAPITaskID).First(&task).Error; err != nil {
		return 0, err
	}
	payload, err := seedanceCallbackPayload(order, &task)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NewAPI-Task-ID", task.TaskID)
	client, err := GetHttpClientWithProxy("")
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.StatusCode, fmt.Errorf("callback returned HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func seedanceCallbackPayload(order *model.SeedanceOrder, task *model.Task) ([]byte, error) {
	if order.PublicProtocol != model.SeedanceProtocolOpenAI {
		return append([]byte(nil), task.Data...), nil
	}
	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.TaskID = task.TaskID
	video.Model = task.Properties.OriginModelName
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	video.CompletedAt = task.UpdatedAt
	if task.Status == model.TaskStatusSuccess {
		video.SetMetadata("url", taskcommon.BuildProxyURL(task.TaskID))
	} else if task.Status == model.TaskStatusFailure {
		video.Error = &dto.OpenAIVideoError{Code: "video_processing_failed", Message: seedanceGenericFailureMessage}
	}
	return common.Marshal(video)
}

func finishSeedanceCallbackFailure(ctx context.Context, order *model.SeedanceOrder, owner string, httpStatus int, cause error) {
	status := model.SeedanceCallbackRetryWait
	delay := int64(30) << min(order.CallbackAttemptCount-1, 7)
	if delay > 3600 {
		delay = 3600
	}
	if order.CallbackAttemptCount >= 8 {
		status = model.SeedanceCallbackDeadLetter
		delay = 0
	}
	result := model.DB.Model(&model.SeedanceOrder{}).
		Where("id = ? AND callback_status = ? AND callback_lease_owner = ?", order.ID, model.SeedanceCallbackSending, owner).
		Updates(map[string]any{
			"callback_status": status, "callback_next_attempt_at": time.Now().Unix() + delay,
			"callback_last_http_status": httpStatus,
			"callback_last_error":       truncateSeedanceError(cause.Error()), "callback_lease_owner": "",
			"callback_lease_until": 0, "updated_at": time.Now().Unix(),
		})
	if result.Error != nil {
		logger.LogError(ctx, "record Seedance callback failure: "+result.Error.Error())
	} else if result.RowsAffected == 0 {
		logger.LogError(ctx, fmt.Sprintf("record Seedance callback failure lost lease for order %d", order.ID))
	}
}

func truncateSeedanceError(value string) string {
	const maxLength = 1024
	value = strings.TrimSpace(value)
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}

func deliverSeedanceBillingOutbox(ctx context.Context, outbox *model.ServiceBillingOutbox, owner string) error {
	var event model.ServiceBillingEvent
	if err := model.DB.Where("event_id = ?", outbox.EventID).First(&event).Error; err != nil {
		return err
	}
	var order model.SeedanceOrder
	if err := model.DB.Where("platform_order_id = ?", event.PlatformOrderID).First(&order).Error; err != nil {
		return err
	}
	if err := freezeLegacySeedanceBillingSnapshot(&order); err != nil {
		return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, order.AIPDDBillingConfigRevision, 0, err, false, 0)
	}
	baseURL := strings.TrimSpace(order.AIPDDBillingBaseURLSnapshot)
	credentialEncrypted := strings.TrimSpace(order.AIPDDBillingCredentialSnapshotEncrypted)
	instanceID := strings.TrimSpace(order.InstanceID)
	authScopeRevision := order.AIPDDBillingConfigRevision
	credential, err := common.DecryptSensitiveValue(credentialEncrypted)
	if err != nil {
		return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, 0, err, false, 0)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/finance/v1/service-usage-events"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(event.PayloadJSON))
	if err != nil {
		return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, 0, err, false, 0)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential)
	req.Header.Set("X-NewAPI-Instance-ID", instanceID)
	req.Header.Set("Idempotency-Key", fmt.Sprintf("service-usage:%s:%s:%d", event.PlatformOrderID, event.ServiceLineItemID, event.Revision))
	client, err := GetHttpClientWithProxy("")
	if err != nil {
		return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, 0, err, false, 0)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req = req.WithContext(requestCtx)
	resp, err := client.Do(req)
	if err != nil {
		return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, 0, err, false, 0)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if readErr != nil {
		return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, resp.StatusCode, readErr, false, 0)
	}
	if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
		var accepted struct {
			Accepted bool `json:"accepted"`
			Data     *struct {
				Accepted bool `json:"accepted"`
			} `json:"data"`
		}
		if err := common.Unmarshal(body, &accepted); err != nil || (!accepted.Accepted && (accepted.Data == nil || !accepted.Data.Accepted)) {
			return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, resp.StatusCode, errors.New("billing response was not accepted"), false, 0)
		}
		now := time.Now().Unix()
		return model.DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&model.ServiceBillingOutbox{}).Where("id = ? AND lease_owner = ?", outbox.ID, owner).Updates(map[string]any{
				"status": model.SeedanceSyncSynced, "response": string(body), "lease_owner": "", "lease_until": 0,
				"last_http_status": resp.StatusCode, "last_error": "", "updated_at": now,
			}).Error; err != nil {
				return err
			}
			return tx.Model(&model.SeedanceOrder{}).Where("platform_order_id = ?", event.PlatformOrderID).
				Updates(map[string]any{"sync_status": model.SeedanceSyncSynced, "updated_at": now}).Error
		})
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, resp.StatusCode, errors.New("billing credential rejected"), true, 0)
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnprocessableEntity {
		return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, resp.StatusCode, seedanceBillingValidationError(body), false, -1)
	}
	retryAfter := int64(0)
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter, _ = strconv.ParseInt(strings.TrimSpace(resp.Header.Get("Retry-After")), 10, 64)
		if retryAfter < 0 {
			retryAfter = 0
		} else if retryAfter > 3600 {
			retryAfter = 3600
		}
	}
	return finishSeedanceOutboxFailure(outbox, owner, order.ChannelID, authScopeRevision, resp.StatusCode, fmt.Errorf("billing endpoint returned HTTP %d", resp.StatusCode), false, retryAfter)
}

// freezeLegacySeedanceBillingSnapshot upgrades orders created before billing
// destination snapshots existed. The destination is committed before any
// outbound request, so a later credential rotation cannot move a retry to a
// different API-key scope. Partially populated snapshots are rejected instead
// of combining fields from different revisions.
func freezeLegacySeedanceBillingSnapshot(order *model.SeedanceOrder) error {
	if order == nil {
		return errors.New("Seedance billing order is required")
	}
	baseURL := strings.TrimSpace(order.AIPDDBillingBaseURLSnapshot)
	credentialEncrypted := strings.TrimSpace(order.AIPDDBillingCredentialSnapshotEncrypted)
	if baseURL != "" && credentialEncrypted != "" && strings.TrimSpace(order.InstanceID) != "" && order.AIPDDBillingConfigRevision > 0 {
		return nil
	}
	if baseURL != "" || credentialEncrypted != "" || order.AIPDDBillingConfigRevision > 0 {
		return errors.New("Seedance billing snapshot is incomplete")
	}

	var config model.SeedanceChannelConfig
	if err := model.DB.Where("channel_id = ?", order.ChannelID).First(&config).Error; err != nil {
		return err
	}
	baseURL = strings.TrimSpace(config.AIPDDBillingBaseURL)
	credentialEncrypted = strings.TrimSpace(config.AIPDDBillingCredentialEncrypted)
	instanceID := strings.TrimSpace(order.InstanceID)
	if instanceID == "" {
		instanceID = strings.TrimSpace(config.InstanceID)
	}
	if baseURL == "" || credentialEncrypted == "" || instanceID == "" || config.Revision <= 0 {
		return errors.New("Seedance billing configuration is incomplete")
	}

	update := model.DB.Model(&model.SeedanceOrder{}).
		Where("id = ? AND aipdd_billing_config_revision = 0 AND (aipdd_billing_base_url_snapshot = '' OR aipdd_billing_base_url_snapshot IS NULL) AND (aipdd_billing_credential_snapshot = '' OR aipdd_billing_credential_snapshot IS NULL)", order.ID).
		Updates(map[string]any{
			"aipdd_billing_config_revision":     config.Revision,
			"aipdd_billing_base_url_snapshot":   baseURL,
			"aipdd_billing_credential_snapshot": credentialEncrypted,
			"instance_id":                       gorm.Expr("CASE WHEN instance_id = '' OR instance_id IS NULL THEN ? ELSE instance_id END", instanceID),
		})
	if update.Error != nil {
		return update.Error
	}
	if err := model.DB.Where("id = ?", order.ID).First(order).Error; err != nil {
		return err
	}
	if strings.TrimSpace(order.AIPDDBillingBaseURLSnapshot) == "" ||
		strings.TrimSpace(order.AIPDDBillingCredentialSnapshotEncrypted) == "" ||
		strings.TrimSpace(order.InstanceID) == "" || order.AIPDDBillingConfigRevision <= 0 {
		return errors.New("Seedance billing snapshot could not be frozen")
	}
	return nil
}

func seedanceBillingValidationError(body []byte) error {
	var response struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Error   struct {
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return errors.New("billing event rejected")
	}
	message := strings.TrimSpace(response.Message)
	code := response.Code
	if message == "" {
		message = strings.TrimSpace(response.Error.Message)
		code = response.Error.Code
	}
	if message == "" {
		return errors.New("billing event rejected")
	}
	message = truncateSeedanceError(message)
	safeCode := seedanceSafeBillingErrorCode(code)
	if safeCode == "" {
		return fmt.Errorf("billing event rejected: %s", message)
	}
	return fmt.Errorf("billing event rejected (%s): %s", safeCode, message)
}

func seedanceSafeBillingErrorCode(value any) string {
	var code string
	switch typed := value.(type) {
	case string:
		code = typed
	case float64:
		code = strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		code = strconv.FormatBool(typed)
	default:
		return ""
	}
	return truncateSeedanceError(strings.TrimSpace(code))
}

func finishSeedanceOutboxFailure(outbox *model.ServiceBillingOutbox, owner string, channelID int, authScopeRevision int, statusCode int, cause error, authPaused bool, explicitDelay int64) error {
	now := time.Now().Unix()
	attempt := outbox.AttemptCount + 1
	status := model.SeedanceSyncRetryWait
	next := now + seedanceBackoffSeconds(attempt)
	if authPaused {
		status = seedanceOutboxAuthPaused
		next = now + 24*60*60
	} else if explicitDelay == -1 {
		status = model.SeedanceSyncDeadLetter
		next = 0
	} else if explicitDelay > 0 {
		next = now + explicitDelay
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		update := tx.Model(&model.ServiceBillingOutbox{}).Where("id = ? AND lease_owner = ?", outbox.ID, owner).Updates(map[string]any{
			"status": status, "attempt_count": attempt, "next_attempt_at": next, "lease_owner": "", "lease_until": 0,
			"last_http_status": statusCode, "last_error": truncateSeedanceError(cause.Error()), "updated_at": now,
		})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return errors.New("billing outbox lease was lost")
		}
		if err := tx.Create(&model.ServiceBillingFailureAttempt{
			EventID: outbox.EventID, AttemptNo: attempt, HTTPStatus: statusCode, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		if !authPaused {
			return nil
		}
		configUpdate := tx.Model(&model.SeedanceChannelConfig{}).Where("channel_id = ?", channelID).Updates(map[string]any{
			"billing_auth_paused_at": now, "billing_auth_last_http_status": statusCode, "updated_at": now,
		})
		if configUpdate.Error != nil {
			return configUpdate.Error
		}
		var eventIDs []string
		scopeQuery := tx.Table("service_billing_events").
			Joins("JOIN seedance_orders ON seedance_orders.platform_order_id = service_billing_events.platform_order_id").
			Where("seedance_orders.channel_id = ?", channelID)
		if authScopeRevision > 0 {
			scopeQuery = scopeQuery.Where("seedance_orders.aipdd_billing_config_revision = ?", authScopeRevision)
		} else {
			scopeQuery = scopeQuery.Where("seedance_orders.aipdd_billing_config_revision = 0")
		}
		if err := scopeQuery.Pluck("service_billing_events.event_id", &eventIDs).Error; err != nil {
			return err
		}
		if len(eventIDs) > 0 {
			if err := tx.Model(&model.ServiceBillingOutbox{}).
				Where("event_id IN ? AND status IN ?", eventIDs, []string{model.SeedanceSyncReady, model.SeedanceSyncRetryWait}).
				Updates(map[string]any{
					"status": model.SeedanceSyncAuthPaused, "next_attempt_at": next,
					"lease_owner": "", "lease_until": 0, "updated_at": now,
				}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&model.SeedanceAdminAudit{
			ActorUserID: 0, ResourceType: "CHANNEL_CONFIG", ResourceID: strconv.Itoa(channelID),
			Action: "BILLING_AUTH_PAUSE", ChangeSummary: fmt.Sprintf("paused billing credential scope revision %d after HTTP %d", authScopeRevision, statusCode), CreatedAt: now,
		}).Error
	}); err != nil {
		return err
	}
	return cause
}

func seedanceBackoffSeconds(attempt int) int64 {
	if attempt < 1 {
		attempt = 1
	}
	exponent := math.Min(float64(attempt-1), 10)
	base := int64(math.Pow(2, exponent))
	if base > 3600 {
		base = 3600
	}
	return base + rand.Int64N(max(int64(1), base/4+1))
}
