package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	mediaKitSubmitPath = "/api/v1/tools/enhance-video"
	mediaKitQueryPath  = "/api/v1/tasks/"
)

// Only the versions with a verified product entitlement and cost row are
// accepted. The fast version stays out until it is validated separately.
var mediaKitToolVersions = map[string]struct{}{
	"standard":     {},
	"professional": {},
}

var mediaKitScenes = map[string]struct{}{
	"aigc": {},
}

var mediaKitResolutions = map[string]struct{}{
	"480p":  {},
	"720p":  {},
	"1080p": {},
	"2k":    {},
	"4k":    {},
}

// VolcengineMediaKitEnhancementProvider speaks the official AI MediaKit quality
// enhancement protocol. It is a separate adapter rather than conditional
// branches inside the generic one because MediaKit uses distinct submit and
// query endpoints, its own request field names and its own result envelope.
type VolcengineMediaKitEnhancementProvider struct {
	apiKey string
	client *http.Client
}

type mediaKitSubmitRequest struct {
	VideoURL    string `json:"video_url"`
	Scene       string `json:"scene"`
	Resolution  string `json:"resolution"`
	ToolVersion string `json:"tool_version,omitempty"`
	ClientToken string `json:"client_token,omitempty"`
}

// mediaKitSpecification is the internal, administrator-facing shape frozen into
// an offering. It is translated into the official request rather than being
// forwarded, so a stored specification can never inject arbitrary fields.
type mediaKitSpecification struct {
	Scene       string `json:"scene"`
	Resolution  string `json:"resolution"`
	ToolVersion string `json:"tool_version"`
}

type mediaKitResponse struct {
	Success *bool  `json:"success"`
	Code    any    `json:"code"`
	Message string `json:"message"`
	TaskID  string `json:"task_id"`
	Status  string `json:"status"`
	Error   struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Data      *mediaKitPayload `json:"data"`
	Result    *mediaKitResult  `json:"result"`
	ExpiresAt any              `json:"expires_at"`
}

type mediaKitPayload struct {
	TaskID string          `json:"task_id"`
	Status string          `json:"status"`
	Result *mediaKitResult `json:"result"`
}

type mediaKitResult struct {
	VideoURL    string  `json:"video_url"`
	Duration    float64 `json:"duration"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Resolution  string  `json:"resolution"`
	FrameRate   float64 `json:"frame_rate"`
	FPS         float64 `json:"fps"`
	Format      string  `json:"format"`
	ToolVersion string  `json:"tool_version"`
}

func newVolcengineMediaKitEnhancementProvider(apiKey string, client *http.Client) (EnhancementProvider, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, errors.New("AI MediaKit API key is not configured")
	}
	return &VolcengineMediaKitEnhancementProvider{apiKey: apiKey, client: client}, nil
}

// Capabilities mirrors the current official contract. AI MediaKit documents
// client_token as an idempotency key for asynchronous submissions, including
// returning the original task for a retry. No cancellation endpoint exists for
// this task API, so accepted tasks still cannot be cancelled safely.
func (p *VolcengineMediaKitEnhancementProvider) Capabilities() EnhancementCapabilities {
	return EnhancementCapabilities{
		SubmitRetrySafe:   true,
		SubmitRetryWindow: 24 * time.Hour,
		CancelSupported:   false,
	}
}

func (p *VolcengineMediaKitEnhancementProvider) Submit(ctx context.Context, request EnhancementSubmitRequest) (*EnhancementResult, error) {
	inputURL := strings.TrimSpace(request.InputURL)
	if inputURL == "" {
		return nil, &enhancementProviderError{cause: errors.New("enhancement source video URL is empty"), definitive: true}
	}
	payload, err := buildMediaKitSubmitRequest(inputURL, request.SpecificationJSON, request.IdempotencyKey)
	if err != nil {
		return nil, &enhancementProviderError{cause: err, definitive: true}
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return p.exchange(ctx, http.MethodPost, model.SeedanceMediaKitBaseURL+mediaKitSubmitPath, body)
}

func (p *VolcengineMediaKitEnhancementProvider) Query(ctx context.Context, executionTaskID string) (*EnhancementResult, error) {
	executionTaskID = strings.TrimSpace(executionTaskID)
	if executionTaskID == "" {
		return nil, errors.New("execution task ID is required")
	}
	endpoint := model.SeedanceMediaKitBaseURL + mediaKitQueryPath + url.PathEscape(executionTaskID)
	return p.exchange(ctx, http.MethodGet, endpoint, nil)
}

// Cancel refuses instead of pretending. The caller turns this into a conflict so
// the local order is not settled while the remote task is still billable.
func (p *VolcengineMediaKitEnhancementProvider) Cancel(ctx context.Context, executionTaskID string) error {
	return ErrSeedanceRemoteCancelUnsupported
}

// ValidateSeedanceMediaKitSpecification lets the admin API reject an
// unsupported scene, resolution or tool version at publish time, using exactly
// the same whitelist the runtime adapter enforces.
func ValidateSeedanceMediaKitSpecification(specificationJSON string) error {
	_, err := buildMediaKitSubmitRequest("https://placeholder.invalid/source", specificationJSON, "")
	return err
}

func buildMediaKitSubmitRequest(inputURL string, specificationJSON string, idempotencyKey string) (*mediaKitSubmitRequest, error) {
	specification := mediaKitSpecification{}
	if strings.TrimSpace(specificationJSON) != "" {
		if err := common.UnmarshalJsonStr(specificationJSON, &specification); err != nil {
			return nil, fmt.Errorf("decode AI MediaKit specification: %w", err)
		}
	}
	scene := strings.ToLower(strings.TrimSpace(specification.Scene))
	if scene == "" {
		scene = "aigc"
	}
	if _, ok := mediaKitScenes[scene]; !ok {
		return nil, fmt.Errorf("unsupported AI MediaKit scene %q", scene)
	}
	resolution := strings.ToLower(strings.TrimSpace(specification.Resolution))
	if _, ok := mediaKitResolutions[resolution]; !ok {
		return nil, fmt.Errorf("unsupported AI MediaKit resolution %q", resolution)
	}
	toolVersion := strings.ToLower(strings.TrimSpace(specification.ToolVersion))
	if _, ok := mediaKitToolVersions[toolVersion]; !ok {
		return nil, fmt.Errorf("unsupported AI MediaKit tool version %q", toolVersion)
	}
	return &mediaKitSubmitRequest{
		VideoURL: inputURL, Scene: scene, Resolution: resolution, ToolVersion: toolVersion,
		ClientToken: mediaKitClientToken(idempotencyKey),
	}, nil
}

// mediaKitClientToken keeps the workflow attempt ID readable when it already
// satisfies the official <=64 printable-ASCII contract. Unexpected caller IDs
// are reduced to a stable digest so retries still send the same token.
func mediaKitClientToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	printableASCII := len(value) <= 64
	for _, char := range []byte(value) {
		if char < 0x20 || char > 0x7e {
			printableASCII = false
			break
		}
	}
	if printableASCII {
		return value
	}
	digest := sha256.Sum256([]byte(value))
	return "newapi-" + hex.EncodeToString(digest[:])[:56]
}

func (p *VolcengineMediaKitEnhancementProvider) exchange(ctx context.Context, method string, endpoint string, body []byte) (*EnhancementResult, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	// The API key travels only in the Authorization header so it cannot reach
	// request evidence, usage facts, logs or error text.
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
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
		return nil, &enhancementProviderError{
			cause:      fmt.Errorf("AI MediaKit returned HTTP %d", resp.StatusCode),
			definitive: isMediaKitDefinitiveStatus(resp.StatusCode),
			reason:     mediaKitHTTPFailureReason(resp.StatusCode),
		}
	}
	var payload mediaKitResponse
	if err := common.Unmarshal(responseBody, &payload); err != nil {
		return nil, err
	}
	if message := mediaKitBusinessFailure(&payload); message != "" {
		taskID, status, result := flattenMediaKitPayload(&payload)
		querySucceeded := payload.Success == nil || *payload.Success
		if method == http.MethodGet && querySucceeded && normalizeEnhancementStatus(status) == model.SeedanceUsageFailed {
			return &EnhancementResult{
				ExecutionTaskID: strings.TrimSpace(taskID), Status: model.SeedanceUsageFailed,
				UsageEvidenceJSON: mediaKitEvidenceJSON(result, payload.ExpiresAt),
				FailureReason:     mediaKitFailureCode(&payload),
			}, nil
		}
		return nil, &enhancementProviderError{
			cause: fmt.Errorf("AI MediaKit reported failure: %s", message), definitive: true,
			reason: mediaKitFailureCode(&payload),
		}
	}
	taskID, status, result := flattenMediaKitPayload(&payload)
	usageEvidence := mediaKitEvidenceJSON(result, payload.ExpiresAt)
	resultURL := ""
	if result != nil {
		resultURL = strings.TrimSpace(result.VideoURL)
	}
	return &EnhancementResult{
		ExecutionTaskID:   strings.TrimSpace(taskID),
		Status:            normalizeEnhancementStatus(status),
		ResultURL:         resultURL,
		UsageEvidenceJSON: usageEvidence,
	}, nil
}

// isMediaKitDefinitiveStatus separates "this request will never succeed" from
// "the outcome is unknown". Throttling, conflicts and server errors stay
// non-definitive so the workflow keeps the attempt open instead of refunding.
func isMediaKitDefinitiveStatus(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout, http.StatusConflict, http.StatusTooEarly, http.StatusTooManyRequests:
		return false
	default:
		return statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError
	}
}

func mediaKitHTTPFailureReason(statusCode int) string {
	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "PROVIDER_AUTHENTICATION_FAILED"
	case http.StatusTooManyRequests:
		return "PROVIDER_RATE_LIMITED"
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return "PROVIDER_TIMEOUT"
	default:
		if statusCode >= http.StatusInternalServerError {
			return "PROVIDER_UPSTREAM_ERROR"
		}
		return "PROVIDER_REQUEST_REJECTED"
	}
}

func mediaKitBusinessFailure(payload *mediaKitResponse) string {
	if payload == nil {
		return ""
	}
	if payload.Success != nil && !*payload.Success {
		return mediaKitFailureMessage(payload)
	}
	if strings.TrimSpace(payload.Error.Message) != "" {
		return mediaKitFailureMessage(payload)
	}
	return ""
}

func mediaKitFailureMessage(payload *mediaKitResponse) string {
	if code := mediaKitFailureCode(payload); code != "AI_MEDIAKIT_FAILURE" {
		return code
	}
	return "AI_MEDIAKIT_FAILURE"
}

func mediaKitFailureCode(payload *mediaKitResponse) string {
	if payload == nil {
		return "AI_MEDIAKIT_FAILURE"
	}
	for _, candidate := range []any{payload.Error.Code, payload.Code} {
		value := strings.TrimSpace(fmt.Sprint(candidate))
		if value != "" && value != "<nil>" && value != "0" {
			return truncateSeedanceError(value)
		}
	}
	return "AI_MEDIAKIT_FAILURE"
}

// flattenMediaKitPayload accepts both the bare envelope and the data-wrapped
// envelope the platform uses across its tool APIs.
func flattenMediaKitPayload(payload *mediaKitResponse) (string, string, *mediaKitResult) {
	taskID := strings.TrimSpace(payload.TaskID)
	status := strings.TrimSpace(payload.Status)
	result := payload.Result
	if payload.Data != nil {
		if taskID == "" {
			taskID = strings.TrimSpace(payload.Data.TaskID)
		}
		if status == "" {
			status = strings.TrimSpace(payload.Data.Status)
		}
		if result == nil {
			result = payload.Data.Result
		}
	}
	return taskID, status, result
}

// mediaKitUsageEvidence keeps only non-sensitive media facts. The signed result
// URL is deliberately excluded so evidence hashes and administrator views never
// carry a downloadable credential.
func mediaKitUsageEvidence(result *mediaKitResult, expiresAt any) map[string]any {
	evidence := map[string]any{}
	if result != nil {
		if result.Duration > 0 {
			evidence["duration_seconds"] = result.Duration
		}
		if result.Width > 0 && result.Height > 0 {
			evidence["width"] = result.Width
			evidence["height"] = result.Height
		}
		if resolution := strings.TrimSpace(result.Resolution); resolution != "" {
			evidence["resolution"] = resolution
		}
		frameRate := result.FrameRate
		if frameRate <= 0 {
			frameRate = result.FPS
		}
		if frameRate > 0 {
			evidence["frame_rate"] = frameRate
		}
		if format := strings.TrimSpace(result.Format); format != "" {
			evidence["format"] = format
		}
		if toolVersion := strings.TrimSpace(result.ToolVersion); toolVersion != "" {
			evidence["tool_version"] = toolVersion
		}
	}
	if value := mediaKitTimestamp(expiresAt); value != "" && value != "0" {
		evidence["expires_at"] = value
	}
	return evidence
}

func mediaKitTimestamp(value any) string {
	switch typed := value.(type) {
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	default:
		result := strings.TrimSpace(fmt.Sprint(value))
		if result == "<nil>" {
			return ""
		}
		return result
	}
}

func mediaKitEvidenceJSON(result *mediaKitResult, expiresAt any) string {
	encoded, err := common.Marshal(mediaKitUsageEvidence(result, expiresAt))
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
