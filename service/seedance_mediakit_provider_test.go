package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func mediaKitFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "volcengine_mediakit", name))
	require.NoError(t, err)
	return body
}

// newTestMediaKitProvider points the adapter at a local server. The official
// host is a package constant, so the request path and headers are asserted
// through a rewriting transport instead of a configurable endpoint.
func newTestMediaKitProvider(t *testing.T, handler http.HandlerFunc) (*VolcengineMediaKitEnhancementProvider, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := *server.Client()
	client.Transport = rewriteHostTransport{target: server.URL, base: server.Client().Transport}
	return &VolcengineMediaKitEnhancementProvider{apiKey: "mediakit-secret", client: &client}, server
}

type rewriteHostTransport struct {
	target string
	base   http.RoundTripper
}

func (t rewriteHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(t.target)
	if err != nil {
		return nil, err
	}
	rewritten := req.Clone(req.Context())
	rewritten.URL.Scheme = target.Scheme
	rewritten.URL.Host = target.Host
	rewritten.Host = target.Host
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(rewritten)
}

func readAllTestBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)
	return body
}

func TestMediaKitSubmitUsesOfficialEndpointAndFields(t *testing.T) {
	var capturedPath string
	var capturedAuthorization string
	var capturedBody []byte
	provider, _ := newTestMediaKitProvider(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAuthorization = r.Header.Get("Authorization")
		capturedBody = readAllTestBody(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mediaKitFixture(t, "submit_accepted.json"))
	})

	result, err := provider.Submit(context.Background(), EnhancementSubmitRequest{
		InputURL:          "https://ark-output.example/generated-object",
		SpecificationJSON: `{"scene":"aigc","resolution":"1080p","tool_version":"standard"}`,
		IdempotencyKey:    "order-1:enhancement:1",
	})
	require.NoError(t, err)
	require.Equal(t, "mediakit-task-0001", result.ExecutionTaskID)
	require.Equal(t, model.SeedanceUsageRunning, result.Status)

	require.Equal(t, mediaKitSubmitPath, capturedPath)
	require.Equal(t, "Bearer mediakit-secret", capturedAuthorization)
	require.JSONEq(t,
		`{"video_url":"https://ark-output.example/generated-object","scene":"aigc","resolution":"1080p","tool_version":"standard","client_token":"order-1:enhancement:1"}`,
		string(capturedBody))
}

func TestMediaKitClientTokenConformsToTheOfficialIdempotencyContract(t *testing.T) {
	require.Equal(t, "order-1:enhancement:1", mediaKitClientToken("order-1:enhancement:1"))
	require.Empty(t, mediaKitClientToken(""))

	longOrUnicode := "同一个任务/" + strings.Repeat("x", 100)
	first := mediaKitClientToken(longOrUnicode)
	require.Equal(t, first, mediaKitClientToken(longOrUnicode))
	require.LessOrEqual(t, len(first), 64)
	for _, char := range []byte(first) {
		require.GreaterOrEqual(t, char, byte(0x20))
		require.LessOrEqual(t, char, byte(0x7e))
	}
}

func TestMediaKitSubmitRejectsUnsupportedSpecification(t *testing.T) {
	provider, _ := newTestMediaKitProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("an invalid specification must not reach the provider")
	})

	for _, specification := range []string{
		`{"scene":"aigc","resolution":"1080p","tool_version":"fast"}`,
		`{"scene":"aigc","resolution":"8k","tool_version":"standard"}`,
		`{"scene":"live","resolution":"1080p","tool_version":"standard"}`,
		`{"resolution":"1080p"}`,
	} {
		_, err := provider.Submit(context.Background(), EnhancementSubmitRequest{
			InputURL: "https://ark-output.example/generated-object", SpecificationJSON: specification,
		})
		require.Error(t, err, specification)
		require.True(t, isDefinitiveEnhancementFailure(err), specification)
	}
}

// The source URL comes from the generation result only. An administrator-frozen
// specification must never be able to redirect the enhancement to another video.
func TestMediaKitSpecificationCannotOverrideSourceVideo(t *testing.T) {
	payload, err := buildMediaKitSubmitRequest("https://ark-output.example/generated-object",
		`{"video_url":"https://attacker.example/other.mp4","scene":"aigc","resolution":"720p","tool_version":"professional"}`, "attempt-1")
	require.NoError(t, err)
	require.Equal(t, "https://ark-output.example/generated-object", payload.VideoURL)
}

func TestMediaKitQueryEscapesTaskIDAndMapsTerminalStates(t *testing.T) {
	var capturedPath string
	fixture := "query_processing.json"
	provider, _ := newTestMediaKitProvider(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(mediaKitFixture(t, fixture))
	})

	result, err := provider.Query(context.Background(), "task/with space")
	require.NoError(t, err)
	require.Equal(t, model.SeedanceUsageRunning, result.Status)
	require.Empty(t, result.ResultURL)
	require.Equal(t, mediaKitQueryPath+"task%2Fwith%20space", capturedPath)

	fixture = "query_succeeded.json"
	result, err = provider.Query(context.Background(), "mediakit-task-0001")
	require.NoError(t, err)
	require.Equal(t, model.SeedanceUsageSucceeded, result.Status)
	require.Equal(t, "https://mediakit-output.example/redacted-signed-object", result.ResultURL)
	require.JSONEq(t,
		`{"duration_seconds":5.12,"resolution":"4k","frame_rate":30,"tool_version":"professional","expires_at":"1777464650"}`,
		result.UsageEvidenceJSON)
	// Usage evidence is hashed into the finance event and shown to administrators,
	// so the signed output URL must stay out of it.
	require.NotContains(t, result.UsageEvidenceJSON, "redacted-signed-object")

	fixture = "query_failed.json"
	result, err = provider.Query(context.Background(), "mediakit-task-0001")
	require.NoError(t, err)
	require.Equal(t, model.SeedanceUsageFailed, result.Status)
	require.Equal(t, "DownloadFailed", result.FailureReason)
}

func TestMediaKitQueryOperationFailureIsNotMistakenForTaskFailure(t *testing.T) {
	provider, _ := newTestMediaKitProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":false,"error":{"code":"InvalidApiKey","message":"secret details"}}`)
	})

	result, err := provider.Query(context.Background(), "mediakit-task-0001")
	require.Nil(t, result)
	require.Error(t, err)
	require.True(t, isDefinitiveEnhancementFailure(err))
	require.Contains(t, err.Error(), "InvalidApiKey")
	require.NotContains(t, err.Error(), "secret details")
}

func TestMediaKitSeparatesDefinitiveFromUnknownHTTPFailures(t *testing.T) {
	status := http.StatusUnauthorized
	provider, _ := newTestMediaKitProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	})
	request := EnhancementSubmitRequest{
		InputURL:          "https://ark-output.example/generated-object",
		SpecificationJSON: `{"scene":"aigc","resolution":"1080p","tool_version":"standard"}`,
	}

	for _, definitive := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		status = definitive
		_, err := provider.Submit(context.Background(), request)
		require.Error(t, err)
		require.True(t, isDefinitiveEnhancementFailure(err), definitive)
		require.NotContains(t, err.Error(), "mediakit-secret")
		if definitive == http.StatusUnauthorized || definitive == http.StatusForbidden {
			require.Equal(t, "PROVIDER_AUTHENTICATION_FAILED", enhancementFailureReason(err, "fallback"))
		}
	}
	for _, unknown := range []int{http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway} {
		status = unknown
		_, err := provider.Submit(context.Background(), request)
		require.Error(t, err)
		require.False(t, isDefinitiveEnhancementFailure(err), unknown)
	}
}

// Cancellation is refused rather than faked, because a fake success would refund
// the customer while the provider keeps executing and billing.
func TestMediaKitReportsCancelUnsupported(t *testing.T) {
	provider := &VolcengineMediaKitEnhancementProvider{apiKey: "mediakit-secret", client: http.DefaultClient}
	capabilities := provider.Capabilities()
	require.False(t, capabilities.CancelSupported)
	require.True(t, capabilities.SubmitRetrySafe)
	require.Equal(t, 24*time.Hour, capabilities.SubmitRetryWindow)
	require.ErrorIs(t, provider.Cancel(context.Background(), "mediakit-task-0001"), ErrSeedanceRemoteCancelUnsupported)
}

func TestMediaKitAdapterPinsTheOfficialHostRegardlessOfStoredEndpoint(t *testing.T) {
	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = "0123456789abcdef0123456789abcdef"
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})
	credential, err := common.EncryptSensitiveValue("mediakit-secret")
	require.NoError(t, err)

	// A dirty row or a tampered snapshot must not be able to redirect a
	// credentialed request away from the official host.
	executor, err := newEnhancementProvider(&model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, AdapterType: model.SeedanceAdapterVolcengineMediaKit,
		ServiceEndpoint: "https://attacker.example/collect", CredentialEncrypted: credential,
		TimeoutPolicyJSON: `{}`,
	})
	require.NoError(t, err)
	adapter, ok := executor.(*VolcengineMediaKitEnhancementProvider)
	require.True(t, ok)
	require.Equal(t, "mediakit-secret", adapter.apiKey)

	_, err = newEnhancementProvider(&model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, AdapterType: model.SeedanceAdapterVolcengineMediaKit,
		TimeoutPolicyJSON: `{}`,
	})
	require.ErrorContains(t, err, "API key is not configured")
}

// Orders frozen before adapter_type existed must keep speaking the generic
// protocol they were created against.
func TestLegacyProviderSnapshotKeepsTheGenericAdapter(t *testing.T) {
	executor, err := newEnhancementProvider(&model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, ServiceEndpoint: "https://supplier.invalid/tasks",
		TimeoutPolicyJSON: `{}`,
	})
	require.NoError(t, err)
	require.IsType(t, &DirectEnhancementProvider{}, executor)
	require.True(t, enhancementCapabilities(executor).SubmitRetrySafe)
	require.True(t, enhancementCapabilities(executor).CancelSupported)
}

func TestUnsupportedAdapterTypeIsRejected(t *testing.T) {
	_, err := newEnhancementProvider(&model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, AdapterType: model.SeedanceAdapterAIPDDSuperResolution,
		ServiceEndpoint: "https://supplier.invalid/tasks", TimeoutPolicyJSON: `{}`,
	})
	require.ErrorContains(t, err, "unsupported enhancement adapter type")
}

func TestMediaKitSpecificationValidationIsSharedWithTheAdminAPI(t *testing.T) {
	require.NoError(t, ValidateSeedanceMediaKitSpecification(`{"scene":"aigc","resolution":"1080p","tool_version":"professional"}`))
	require.NoError(t, ValidateSeedanceMediaKitSpecification(`{"resolution":"720p","tool_version":"standard"}`))
	require.Error(t, ValidateSeedanceMediaKitSpecification(`{"resolution":"720p","tool_version":"fast"}`))
	require.Error(t, ValidateSeedanceMediaKitSpecification(`{}`))
}
