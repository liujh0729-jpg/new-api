package seedance

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIndependentSeedanceQuoteAppliesTheDisplayedVIPRatio(t *testing.T) {
	groupRatioSnapshot := ratio_setting.GroupRatio2JSONString()
	exchangeRateSnapshot := operation_setting.USDExchangeRate
	quotaPerUnitSnapshot := common.QuotaPerUnit
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(groupRatioSnapshot))
		operation_setting.USDExchangeRate = exchangeRateSnapshot
		common.QuotaPerUnit = quotaPerUnitSnapshot
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"VIP1":0.78}`))
	operation_setting.USDExchangeRate = 8
	common.QuotaPerUnit = 500_000

	quote := buildSeedanceTaskPricingQuote(
		&model.SeedanceModelOffering{ModelSaleMicroRMB: 8_000_000, PricingVersion: "price-v1"},
		&relaycommon.RelayInfo{UserGroup: "default", UsingGroup: "VIP1"},
	)
	require.Equal(t, 1.0, quote.UnitPriceUSD)
	require.Equal(t, 0.78, quote.GroupRatio)
	require.Equal(t, 0.78, quote.SaleUSD)
	require.Equal(t, 390_000, quote.Quota)
}

func TestIndependentSeedanceQuoteUsesDurationAndReferenceVariant(t *testing.T) {
	exchangeRateSnapshot := operation_setting.USDExchangeRate
	quotaPerUnitSnapshot := common.QuotaPerUnit
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = exchangeRateSnapshot
		common.QuotaPerUnit = quotaPerUnitSnapshot
	})
	operation_setting.USDExchangeRate = 8
	common.QuotaPerUnit = 500_000

	quote := buildSeedanceTaskPricingQuoteForRequest(
		&model.SeedanceModelOffering{
			NoReferenceUnitPriceMicroRMB: 8_000_000,
			ReferenceUnitPriceMicroRMB:   12_000_000,
			SourceResolution:             "720p",
			PricingVersion:               "price-v2",
		},
		&relaycommon.RelayInfo{UserGroup: "default", UsingGroup: "default"},
		2.25,
		true,
	)
	require.Equal(t, "second", quote.Unit)
	require.Equal(t, "price-v2:reference_video", quote.Variant)
	require.Equal(t, 1.5, quote.UnitPriceUSD)
	require.Equal(t, 2.25, quote.Quantity)
	require.Equal(t, 3.375, quote.SaleUSD)
	require.Equal(t, 1_687_500, quote.Quota)
	require.True(t, quote.HasReferenceVideo)
	require.Equal(t, "720p", quote.Resolution)
}

func TestSeedanceRequestedFPSRejectsNonIntegerAndReadsAliases(t *testing.T) {
	fps, explicit, err := seedanceRequestedFPS(map[string]any{"frames_per_second": float64(24)})
	require.NoError(t, err)
	require.True(t, explicit)
	require.Equal(t, 24, fps)

	_, explicit, err = seedanceRequestedFPS(map[string]any{"fps": 23.976})
	require.True(t, explicit)
	require.Error(t, err)
}

func TestJavaGoldenFixtureAndCallbackBoundary(t *testing.T) {
	fixtureBytes, err := os.ReadFile("testdata/java-golden.json")
	require.NoError(t, err)
	var fixture map[string]any
	require.NoError(t, common.Unmarshal(fixtureBytes, &fixture))
	require.Equal(t, "83137cd59afa23dd42b7050d47c2d9de64374385", fixture["java_reference_commit"])

	official := fixture["official"].(map[string]any)
	create := official["create"].(map[string]any)
	requestBody, err := common.Marshal(create["request"])
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(string(requestBody)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		OriginModelName: "Public Seedance 2.5",
		ChannelMeta:     &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: "doubao-seedance-2-5-260628"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	require.Nil(t, relaycommon.ValidateBasicTaskRequestAllowEmptyPrompt(ctx, info, ""))
	require.Nil(t, adaptor.TaskAdaptor.ValidateRequestAndSetAction(ctx, info))
	body, err := adaptor.BuildRequestBody(ctx, info)
	require.NoError(t, err)
	forwardedBytes, err := io.ReadAll(body)
	require.NoError(t, err)
	var forwarded map[string]any
	require.NoError(t, common.Unmarshal(forwardedBytes, &forwarded))
	require.NotContains(t, forwarded, "callback_url")
	require.Equal(t, "https://client.example/callback", info.SeedanceCallbackURL)
	require.Equal(t, float64(-1), forwarded["duration"])
	require.Equal(t, false, forwarded["generate_audio"])
	require.Equal(t, false, forwarded["watermark"])
	require.Equal(t, float64(0), forwarded["priority"])
}

func TestJavaGoldenPublicSnapshotsContainNoPrivateWorkflowTerms(t *testing.T) {
	fixtureBytes, err := os.ReadFile("testdata/java-golden.json")
	require.NoError(t, err)
	normalized := strings.ToLower(string(fixtureBytes))
	for _, forbidden := range []string{"byok", "enhancement", "super_resolution", "upscale", "provider", "超分", "增强"} {
		require.NotContains(t, normalized, forbidden)
	}
}

func TestSeedancePublicDocumentationContainsNoPrivateWorkflowTerms(t *testing.T) {
	forbidden := []string{
		"byok", "enhancement", "super_resolution", "upscale", "provider",
		"attempt", "service_code", "服务代码", "超分", "增强",
	}

	openAPIBytes, err := os.ReadFile("../../../../docs/openapi/public.json")
	require.NoError(t, err)
	var openAPI struct {
		Paths      map[string]any `json:"paths"`
		Components struct {
			Schemas map[string]any `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(openAPIBytes, &openAPI))

	publicSurface := map[string]any{}
	for _, path := range []string{
		"/v1/videos", "/v1/videos/{task_id}", "/v1/videos/{task_id}/content",
		"/api/v3/contents/generations/tasks", "/api/v3/contents/generations/tasks/{task_id}",
	} {
		publicSurface[path] = openAPI.Paths[path]
	}
	for _, schema := range []string{
		"SeedanceOpenAIVideoTask", "SeedanceOfficialTask", "SeedanceOfficialUsage",
		"SeedanceTaskCreateResponse", "SeedanceCreateRequest", "SeedanceContentItem",
		"OpenAIVideoError",
	} {
		publicSurface[schema] = openAPI.Components.Schemas[schema]
	}
	publicJSON, err := json.Marshal(publicSurface)
	require.NoError(t, err)
	assertNoPrivateDocumentationTerms(t, string(publicJSON), forbidden)

	for _, name := range []string{"overview.md", "create-video.json", "get-video.json"} {
		contents, readErr := os.ReadFile("../../../../docs/apifox/seedance/" + name)
		require.NoError(t, readErr)
		assertNoPrivateDocumentationTerms(t, string(contents), forbidden)
	}
}

func assertNoPrivateDocumentationTerms(t *testing.T, contents string, forbidden []string) {
	t.Helper()
	normalized := strings.ToLower(contents)
	for _, term := range forbidden {
		require.NotContains(t, normalized, term)
	}
}

func TestJavaGoldenOfficialQuerySnapshotsMatchExactly(t *testing.T) {
	fixture := loadJavaGoldenFixture(t)
	official := fixture["official"].(map[string]any)

	for _, testCase := range []struct {
		name   string
		status model.TaskStatus
	}{
		{name: "running", status: model.TaskStatusInProgress},
		{name: "succeeded", status: model.TaskStatusSuccess},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			expected := official[testCase.name].(map[string]any)
			providerPayload := make(map[string]any, len(expected))
			for key, value := range expected {
				providerPayload[key] = value
			}
			providerPayload["id"] = "ark-private-task"
			providerPayload["model"] = "ark-private-model"

			task := &model.Task{
				TaskID: "task_public",
				Status: testCase.status,
				Properties: model.Properties{
					OriginModelName: "Public Seedance 2.5",
				},
			}
			task.SetData(providerPayload)

			actual, err := (&TaskAdaptor{}).ConvertToSeedanceOfficialTask(task)
			require.NoError(t, err)
			expectedJSON, err := common.Marshal(expected)
			require.NoError(t, err)
			require.JSONEq(t, string(expectedJSON), string(actual))
		})
	}
}

func TestJavaGoldenOpenAIQueryStatusesAndAllowlist(t *testing.T) {
	fixture := loadJavaGoldenFixture(t)
	official := fixture["official"].(map[string]any)
	openAI := fixture["openai"].(map[string]any)
	allowed := map[string]bool{
		"id": true, "task_id": true, "object": true, "model": true,
		"status": true, "progress": true, "created_at": true,
		"completed_at": true, "expires_at": true, "seconds": true,
		"size": true, "remixed_from_video_id": true, "error": true,
		"metadata": true, "usage": true,
	}

	for _, testCase := range []struct {
		name           string
		status         model.TaskStatus
		expectedStatus string
	}{
		{name: "running", status: model.TaskStatusInProgress, expectedStatus: openAI["running_status"].(string)},
		{name: "succeeded", status: model.TaskStatusSuccess, expectedStatus: openAI["completed_status"].(string)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			task := &model.Task{
				TaskID:    "task_public",
				Status:    testCase.status,
				CreatedAt: 1700000000,
				UpdatedAt: 1700000010,
				Properties: model.Properties{
					OriginModelName: "Public Seedance 2.5",
				},
			}
			task.SetData(official[testCase.name])

			actual, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
			require.NoError(t, err)
			var response map[string]any
			require.NoError(t, common.Unmarshal(actual, &response))
			require.Equal(t, testCase.expectedStatus, response["status"])
			require.Equal(t, "task_public", response["id"])
			require.Equal(t, "Public Seedance 2.5", response["model"])
			for key := range response {
				require.Truef(t, allowed[key], "unexpected OpenAI public field %q", key)
			}
			for _, forbidden := range []string{"ark-private", "provider", "enhancement", "upscale", "super_resolution"} {
				require.NotContains(t, strings.ToLower(string(actual)), forbidden)
			}
		})
	}
}

func loadJavaGoldenFixture(t *testing.T) map[string]any {
	t.Helper()
	fixtureBytes, err := os.ReadFile("testdata/java-golden.json")
	require.NoError(t, err)
	var fixture map[string]any
	require.NoError(t, common.Unmarshal(fixtureBytes, &fixture))
	return fixture
}

func TestCreateResponseIsDeferredUntilLocalOrderCommit(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	response := &http.Response{Body: io.NopCloser(strings.NewReader(`{"id":"ark-private-task"}`))}

	taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(ctx, response, &relaycommon.RelayInfo{})

	require.Nil(t, taskErr)
	require.Equal(t, "ark-private-task", taskID)
	require.JSONEq(t, `{"id":"ark-private-task"}`, string(taskData))
	require.Empty(t, recorder.Body.String())
}
