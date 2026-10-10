package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

// The production catalog fixture is the inner `data` object of the live
// https://api.aipdd.work/v1/new-api/catalog response, so these tests pin the
// eight-category contract against what upstream actually publishes.
const aipddProductionFixturePath = "../pkg/aipddcatalog/testdata/aipdd_catalog.json"

const (
	aipddFixtureRMBPerAWCoin    = 0.0001
	aipddFixtureUSDExchangeRate = 7.3
	aipddFixturePriceTolerance  = 1e-12
)

func aipddFixtureUSD(awcoin float64) float64 {
	return awcoin * aipddFixtureRMBPerAWCoin / aipddFixtureUSDExchangeRate
}

func loadAIPDDProductionFixture(t *testing.T) ([]byte, aipddcatalog.AtomicCatalog) {
	t.Helper()
	raw, err := os.ReadFile(aipddProductionFixturePath)
	require.NoError(t, err)
	catalog, err := aipddcatalog.UnmarshalAtomic(raw)
	require.NoError(t, err)
	require.NoError(t, catalog.Validate())
	return raw, catalog
}

// aipddProductionCatalogServer serves the fixture the way upstream does: the
// catalog is wrapped in the code/message/data envelope, so the handler below
// exercises FetchAtomic's envelope decode as well as the apply path.
func aipddProductionCatalogServer(t *testing.T, rawFixture []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":` + string(rawFixture) + `}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func aipddProductionFixtureCleanup(t *testing.T) {
	t.Helper()
	previousExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = aipddFixtureUSDExchangeRate
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = previousExchangeRate
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
	})
}

func aipddStoredEndpoints(t *testing.T, item Model) []constant.EndpointType {
	t.Helper()
	var endpoints []constant.EndpointType
	require.NoError(t, common.Unmarshal([]byte(item.Endpoints), &endpoints))
	return endpoints
}

func TestSyncAIPDDCatalogFromProductionFixtureInitializesEightCategories(t *testing.T) {
	truncateTables(t)
	aipddProductionFixtureCleanup(t)
	rawFixture, catalog := loadAIPDDProductionFixture(t)
	server := aipddProductionCatalogServer(t, rawFixture)

	result, err := SyncAIPDDCatalog(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)
	require.False(t, result.UsedSnapshot)
	require.Equal(t, catalog.Revision, result.Revision)
	require.InDelta(t, aipddFixtureRMBPerAWCoin, result.RMBPerAWCoin, 1e-12)
	require.Len(t, catalog.ModelNames(), result.AddedModels)

	var managed Channel
	require.NoError(t, DB.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, aipddEnvChannelName).First(&managed).Error)
	require.Equal(t, common.ChannelStatusEnabled, managed.Status)
	require.Equal(t, "default", managed.Group)
	require.NotNil(t, managed.BaseURL)
	require.Equal(t, server.URL, *managed.BaseURL)
	require.ElementsMatch(t, catalog.ModelNames(), managed.GetModels())

	var models []Model
	require.NoError(t, DB.Find(&models).Error)
	byName := make(map[string]Model, len(models))
	for _, item := range models {
		byName[item.ModelName] = item
	}
	require.Len(t, models, len(catalog.ModelNames()))

	// One case per category. Membership is read back from the stored endpoint
	// column, the same source the pricing page and the relay guards use, so a
	// model cannot be listed under a category it does not answer on.
	categoryCases := []struct {
		endpoint constant.EndpointType
		tag      string
		models   []string
	}{
		{endpoint: constant.EndpointTypeOpenAI, tag: "文字模型", models: []string{
			"ap-chat-3.8-flash", "ap-deepseek-v4-flash", "ap-deepseek-v4-pro",
			"ap-glm-5.2", "ap-glm-5.3", "ap-glm-5.3-flash", "ap-qwen3.8-max",
		}},
		{endpoint: constant.EndpointTypeEmbeddings, tag: "向量模型", models: []string{"ap-qwen3-embedding:0.6b"}},
		{endpoint: constant.EndpointTypeImageGeneration, tag: "图片生成", models: []string{
			"ap-image-2.0", "ap-image-fast-2.0", "ap-image-flare-2.5", "ap-image-lite-2.0",
			"ap-image-sunburst-2.5", "ap-jimeng-image-4.0", "ap-nano-2.1",
		}},
		{endpoint: constant.EndpointTypeImageEdits, tag: "图生图", models: []string{
			"ap-image-2.0", "ap-image-flare-2.5", "ap-image-lite-2.0",
			"ap-image-sunburst-2.5", "ap-nano-2.1", "ap-qwen-image-edit",
		}},
		{endpoint: constant.EndpointTypeOpenAIVideo, tag: "视频生成", models: []string{"ap-ltx-2.5", "ap-minimax-h3"}},
		{endpoint: constant.EndpointTypeAudioSpeech, tag: "语音合成", models: []string{"ap-indextts-2"}},
		{endpoint: constant.EndpointTypeAudioTranscriptions, tag: "语音识别", models: []string{"ap-funasr"}},
		// P0 is still open: jev is not published upstream, so the decision
		// category has a route and no synchronisable model yet.
		{endpoint: constant.EndpointTypeSystemOne, tag: "决策模型", models: nil},
	}
	for _, testCase := range categoryCases {
		var stored []string
		for _, item := range models {
			if slices.Contains(aipddStoredEndpoints(t, item), testCase.endpoint) {
				stored = append(stored, item.ModelName)
			}
		}
		require.ElementsMatch(t, testCase.models, stored, testCase.tag)
		for _, modelName := range testCase.models {
			item, ok := byName[modelName]
			require.True(t, ok, modelName)
			require.Contains(t, item.Tags, testCase.tag, modelName)
			require.Equal(t, 1, item.Status, modelName)
			require.Equal(t, 1, item.SyncOfficial, modelName)
			require.Equal(t, NameRuleExact, item.NameRule, modelName)
			require.NotEmpty(t, item.Description, modelName)
		}
	}

	// Execution mode the relay reads: the four synchronous families answer
	// directly, video and voice cloning still submit a task.
	for modelName, expected := range map[string]struct {
		endpoint    constant.EndpointType
		synchronous bool
	}{
		"ap-funasr":          {constant.EndpointTypeAudioTranscriptions, true},
		"ap-qwen-image-edit": {constant.EndpointTypeImageEdits, true},
		"ap-image-2.0":       {constant.EndpointTypeImageGeneration, true},
		"ap-indextts-2":      {constant.EndpointTypeAudioSpeech, false},
		"ap-minimax-h3":      {constant.EndpointTypeOpenAIVideo, false},
	} {
		capability, ok := constant.GetAIPDDCapability(modelName)
		require.True(t, ok, modelName)
		require.Equal(t, expected.endpoint, capability.EndpointType, modelName)
		require.Equal(t, expected.synchronous, capability.Synchronous, modelName)
	}

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", managed.Id).Find(&abilities).Error)
	require.Len(t, abilities, len(catalog.ModelNames()))
	for _, ability := range abilities {
		require.Equal(t, "default", ability.Group, ability.Model)
		require.True(t, ability.Enabled, ability.Model)
	}

	var legacyCount int64
	require.NoError(t, DB.Model(&Model{}).Where("model_name IN ?", legacyAIPDDModelNames).Count(&legacyCount).Error)
	require.Zero(t, legacyCount)
	for _, modelName := range legacyAIPDDModelNames {
		require.False(t, slices.Contains(managed.GetModels(), modelName), modelName)
	}
}

func TestSyncAIPDDCatalogFromProductionFixtureImportsEveryPublishedPrice(t *testing.T) {
	truncateTables(t)
	aipddProductionFixtureCleanup(t)
	rawFixture, catalog := loadAIPDDProductionFixture(t)
	server := aipddProductionCatalogServer(t, rawFixture)

	_, err := SyncAIPDDCatalog(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)

	// Fixed per-call prices, keyed by the upstream AW-coin amount so the
	// converted USD price is derived instead of guessed.
	capabilityByID := make(map[string]aipddcatalog.AtomicCapability, len(catalog.Capabilities))
	for _, capability := range catalog.Capabilities {
		capabilityByID[capability.ID] = capability
	}
	expectedFixed := map[string]float64{
		"ap-image-2.0":          2000,
		"ap-image-fast-2.0":     4000,
		"ap-image-flare-2.5":    2000,
		"ap-image-lite-2.0":     2400,
		"ap-image-sunburst-2.5": 3000,
		"ap-jimeng-image-4.0":   1500,
		"ap-nano-2.1":           5000,
		"ap-indextts-2":         1000,
		"ap-qwen-image-edit":    10000,
	}
	for modelName, awcoin := range expectedFixed {
		capability, ok := capabilityByID[modelName]
		require.True(t, ok, modelName)
		require.InDelta(t, awcoin, aipddcatalog.TaskAWCoinPrice(capability.Pricing), 1e-9, modelName)
		price, found := ratio_setting.GetModelPrice(modelName, false)
		require.True(t, found, modelName)
		require.InDelta(t, aipddFixtureUSD(awcoin), price, aipddFixturePriceTolerance, modelName)
		require.NotEqual(t, billing_setting.BillingModeTaskPricing, billing_setting.GetBillingMode(modelName), modelName)
	}

	// ap-qwen-image-edit is the regression guard: it publishes its per-call
	// display price through token_market_shared, and a missing import left it
	// on the unknown-model token ratio, which undercharges a 10000-awcoin edit.
	editPrice, found := ratio_setting.GetModelPrice("ap-qwen-image-edit", false)
	require.True(t, found, "token_market_shared per-call prices must be imported")
	require.InDelta(t, 0.136986301369863, editPrice, 1e-12)

	var storedPrices map[string]float64
	var priceOption Option
	require.NoError(t, DB.Where("key = ?", aipddModelPriceOptionKey).First(&priceOption).Error)
	require.NoError(t, common.UnmarshalJsonStr(priceOption.Value, &storedPrices))
	require.Len(t, storedPrices, len(expectedFixed))
	for modelName := range storedPrices {
		require.Contains(t, expectedFixed, modelName)
	}

	// Duration prices: ASR is flat per second, video is per resolution.
	funasr, ok := billing_setting.GetTaskPricing("ap-funasr")
	require.True(t, ok)
	require.Equal(t, billing_setting.TaskPricingUnitSecond, funasr.Unit)
	require.InDelta(t, aipddFixtureUSD(100), funasr.NoReferenceVideoUnitPrice, aipddFixturePriceTolerance)
	require.Equal(t, billing_setting.BillingModeTaskPricing, billing_setting.GetBillingMode("ap-funasr"))

	minimax, ok := billing_setting.GetTaskPricing("ap-minimax-h3")
	require.True(t, ok)
	require.Empty(t, minimax.NoReferenceVideoUnitPrice)
	for resolution, awcoinPerSecond := range map[string]float64{"480p": 900, "768p": 1200, "1080p": 1500} {
		tier, found := minimax.ByResolution[resolution]
		require.True(t, found, resolution)
		require.InDelta(t, aipddFixtureUSD(awcoinPerSecond), tier.NoReferenceVideoUnitPrice, aipddFixturePriceTolerance, resolution)
		require.Equal(t, billing_setting.ReferenceVideoPolicySame, tier.ReferenceVideoPolicy, resolution)
	}

	// The catalog never writes local token ratios: per-token prices reach the
	// gateway through the upstream ratio sync, not through these options.
	var ratioCount int64
	require.NoError(t, DB.Model(&Option{}).Where("key = ?", "ModelRatio").Count(&ratioCount).Error)
	require.Zero(t, ratioCount)

	// Display: every available model is priced, and the unavailable upstream
	// model (ap-ltx-2.5) is hidden from /v1/models and from the pricing page.
	require.True(t, aipddcatalog.IsHiddenFromV1ModelsList("ap-ltx-2.5"))
	for _, modelName := range catalog.ModelNames() {
		if modelName == "ap-ltx-2.5" {
			continue
		}
		require.False(t, aipddcatalog.IsHiddenFromV1ModelsList(modelName), modelName)
	}
	InvalidatePricingCache()
	pricing := GetPricing()
	byName := make(map[string]Pricing, len(pricing))
	for _, item := range pricing {
		byName[item.ModelName] = item
	}
	require.Len(t, pricing, len(catalog.ModelNames())-1)
	require.NotContains(t, byName, "ap-ltx-2.5")
	for modelName, item := range byName {
		if modelName == "ap-funasr" || modelName == "ap-minimax-h3" {
			require.Equal(t, billing_setting.BillingModeTaskPricing, item.BillingMode, modelName)
			require.Equal(t, 1, item.QuotaType, modelName)
			continue
		}
		if _, ok := expectedFixed[modelName]; ok {
			require.Equal(t, 1, item.QuotaType, modelName)
			require.Positive(t, item.ModelPrice, modelName)
			continue
		}
		require.Equal(t, 0, item.QuotaType, modelName)
	}
}

func TestSyncAIPDDCatalogFromProductionFixtureIsIdempotent(t *testing.T) {
	truncateTables(t)
	aipddProductionFixtureCleanup(t)
	rawFixture, catalog := loadAIPDDProductionFixture(t)
	server := aipddProductionCatalogServer(t, rawFixture)

	first, err := SyncAIPDDCatalog(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)
	require.Positive(t, first.AddedModels)

	second, err := SyncAIPDDCatalog(context.Background(), server.Client(), server.URL, "sk-test")
	require.NoError(t, err)
	require.Equal(t, catalog.Revision, second.Revision)
	require.Zero(t, second.AddedModels)
	require.Zero(t, second.RemovedModels)
	require.Zero(t, second.UpdatedPrices)
	require.False(t, second.UsedSnapshot)

	var managed Channel
	require.NoError(t, DB.Where("type = ? AND name = ?", constant.ChannelTypeAIPDD, aipddEnvChannelName).First(&managed).Error)
	require.ElementsMatch(t, catalog.ModelNames(), managed.GetModels())
	var modelCount, abilityCount, snapshotCount int64
	require.NoError(t, DB.Model(&Model{}).Count(&modelCount).Error)
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", managed.Id).Count(&abilityCount).Error)
	require.NoError(t, DB.Model(&AIPDDCatalogSnapshot{}).Count(&snapshotCount).Error)
	require.EqualValues(t, len(catalog.ModelNames()), modelCount)
	require.EqualValues(t, len(catalog.ModelNames()), abilityCount)
	require.EqualValues(t, 1, snapshotCount)

	var snapshot AIPDDCatalogSnapshot
	require.NoError(t, DB.First(&snapshot, aipddCatalogSnapshotID).Error)
	require.Equal(t, catalog.Revision, snapshot.Revision)
	require.Equal(t, server.URL, snapshot.SourceBaseURL)
	require.Contains(t, snapshot.Payload, "ap-qwen-image-edit")
}

func TestIsTokenMarketPerCallCapabilityCoversBothTaskAdapters(t *testing.T) {
	perCall := aipddcatalog.AtomicPricing{PricingModel: "per_call", Currency: "awcoin", Enabled: true}
	perSecond := aipddcatalog.AtomicPricing{PricingModel: "per_second", Currency: "awcoin", Enabled: true}
	require.True(t, isTokenMarketPerCallCapability(aipddcatalog.AtomicCapability{
		AdapterCode: "token_market_media", Pricing: perCall,
	}))
	require.True(t, isTokenMarketPerCallCapability(aipddcatalog.AtomicCapability{
		AdapterCode: "token_market_shared", Pricing: perCall,
	}))
	require.False(t, isTokenMarketPerCallCapability(aipddcatalog.AtomicCapability{
		AdapterCode: "token_market_shared", Pricing: perSecond,
	}))
	require.False(t, isTokenMarketPerCallCapability(aipddcatalog.AtomicCapability{
		AdapterCode: "comfyui", Pricing: perCall,
	}))
}
