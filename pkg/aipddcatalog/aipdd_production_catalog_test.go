package aipddcatalog

import (
	"os"
	"slices"
	"sort"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdata/aipdd_catalog.json is the `data` object of the production
// https://api.aipdd.work/v1/new-api/catalog response, captured on 2026-10-10
// (schemaVersion 2, revision 7494d4ff009fab75cbbe44d0f9d7d7a078094b949dff3d5a7347c25fd72acd2e).
// It is the only place the eight endpoint categories are specified end to end,
// so these tests read it instead of restating a synthetic catalog.
func loadFixtureCatalog(t *testing.T) AtomicCatalog {
	t.Helper()
	data, err := os.ReadFile("testdata/aipdd_catalog.json")
	require.NoError(t, err)
	catalog, err := UnmarshalAtomic(data)
	require.NoError(t, err, "production catalog must unmarshal")
	require.NoError(t, catalog.Validate(), "production catalog must validate")
	return catalog
}

func TestFixtureCatalogKeepsProductionShape(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	assert.Equal(t, 2, catalog.SchemaVersion)
	assert.Len(t, catalog.Capabilities, 12)
	assert.Len(t, catalog.Models, 8)
	assert.Len(t, catalog.ModelNames(), 20)
}

type aipddCategoryCase struct {
	name     string
	endpoint constant.EndpointType
	models   []string
	// nonPrimary lists models that must answer on this endpoint but keep another
	// endpoint first: the generation models that gain an edit entry stay on
	// /v1/images/generations as their primary route.
	nonPrimary []string
}

// The eight categories the default AIPDD integration must cover.
func aipddCategoryCases() []aipddCategoryCase {
	return []aipddCategoryCase{
		{
			name:     "文字模型",
			endpoint: constant.EndpointTypeOpenAI,
			models: []string{
				"ap-chat-3.8-flash",
				"ap-deepseek-v4-flash",
				"ap-deepseek-v4-pro",
				"ap-glm-5.2",
				"ap-glm-5.3",
				"ap-glm-5.3-flash",
				"ap-qwen3.8-max",
			},
		},
		{
			name:     "向量模型",
			endpoint: constant.EndpointTypeEmbeddings,
			models:   []string{"ap-qwen3-embedding:0.6b"},
		},
		{
			name:     "图片生成",
			endpoint: constant.EndpointTypeImageGeneration,
			models: []string{
				"ap-image-2.0",
				"ap-image-fast-2.0",
				"ap-image-flare-2.5",
				"ap-image-lite-2.0",
				"ap-image-sunburst-2.5",
				"ap-jimeng-image-4.0",
				"ap-nano-2.1",
			},
		},
		{
			// ap-qwen-image-edit answers here directly; the other five are
			// generation models that accept reference images, so the same model
			// is reachable on both /v1/images/generations and /v1/images/edits.
			// ap-image-fast-2.0 and ap-jimeng-image-4.0 publish no edit support.
			name:     "图生图",
			endpoint: constant.EndpointTypeImageEdits,
			models: []string{
				"ap-image-2.0",
				"ap-image-flare-2.5",
				"ap-image-lite-2.0",
				"ap-image-sunburst-2.5",
				"ap-nano-2.1",
				"ap-qwen-image-edit",
			},
			nonPrimary: []string{
				"ap-image-2.0",
				"ap-image-flare-2.5",
				"ap-image-lite-2.0",
				"ap-image-sunburst-2.5",
				"ap-nano-2.1",
			},
		},
		{
			name:     "视频生成",
			endpoint: constant.EndpointTypeOpenAIVideo,
			models:   []string{"ap-ltx-2.5", "ap-minimax-h3"},
		},
		{
			name:     "音频模型",
			endpoint: constant.EndpointTypeAudioSpeech,
			models:   []string{"ap-indextts-2"},
		},
		{
			name:     "语音识别",
			endpoint: constant.EndpointTypeAudioTranscriptions,
			models:   []string{"ap-funasr"},
		},
		{
			// P0 is still open: jev is not published in the upstream catalog yet,
			// so /v1/systemone has a route and no synchronizable model.
			name:     "决策模型",
			endpoint: constant.EndpointTypeSystemOne,
			models:   nil,
		},
	}
}

func TestFixtureCatalogCoversTheEightEndpointCategories(t *testing.T) {
	catalog := loadFixtureCatalog(t)
	endpoints := catalog.ModelEndpointTypes()

	for _, testCase := range aipddCategoryCases() {
		t.Run(testCase.name, func(t *testing.T) {
			var answers []string
			for model, modelEndpoints := range endpoints {
				if containsEndpointType(modelEndpoints, testCase.endpoint) {
					answers = append(answers, model)
				}
			}
			sort.Strings(answers)
			expected := append([]string(nil), testCase.models...)
			sort.Strings(expected)
			if len(expected) == 0 {
				assert.Empty(t, answers, "no catalog model may claim "+string(testCase.endpoint)+" before P0 lands")
				return
			}
			assert.Equal(t, expected, answers)

			for _, model := range expected {
				require.Contains(t, endpoints, model)
				if slices.Contains(testCase.nonPrimary, model) {
					assert.NotEqual(t, testCase.endpoint, endpoints[model][0],
						"%s must keep its generation endpoint first; %s is only appended", model, testCase.endpoint)
					continue
				}
				assert.Equal(t, testCase.endpoint, endpoints[model][0],
					"%s must answer on %s as its primary endpoint", model, testCase.endpoint)
			}
		})
	}
}

func TestFixtureCatalogSynchronousAndBillingDecisions(t *testing.T) {
	catalog := loadFixtureCatalog(t)
	capabilities := map[string]constant.AIPDDCapability{}
	for _, capability := range catalog.RuntimeCapabilities() {
		capabilities[capability.ModelName] = capability
	}

	cases := []struct {
		model         string
		taskKind      string
		billingType   constant.AIPDDBillingType
		taskCost      float64
		synchronous   bool
		execution     string
		executionPath string
	}{
		{
			model:         "ap-funasr",
			taskKind:      "speech_to_text",
			billingType:   constant.AIPDDBillingTypeDurationSeconds,
			taskCost:      100,
			synchronous:   true,
			execution:     "openai",
			executionPath: "/v1/audio/transcriptions",
		},
		{
			model:         "ap-qwen-image-edit",
			taskKind:      "image_to_image",
			billingType:   constant.AIPDDBillingTypePerCall,
			taskCost:      10000,
			synchronous:   true,
			execution:     "openai",
			executionPath: "/v1/images/edits",
		},
		{
			model:       "ap-image-2.0",
			taskKind:    "image_generation",
			billingType: constant.AIPDDBillingTypePerCall,
			taskCost:    2000,
			synchronous: true,
			execution:   "token_market_image",
		},
		{
			model:       "ap-indextts-2",
			taskKind:    "voice_clone",
			billingType: constant.AIPDDBillingTypePerCall,
			taskCost:    1000,
			synchronous: false,
			execution:   "token_market_audio",
		},
		{
			model:       "ap-ltx-2.5",
			taskKind:    "video_generation",
			billingType: constant.AIPDDBillingTypeDurationSeconds,
			taskCost:    4000,
			synchronous: false,
			execution:   "shared_task",
		},
		{
			model:       "ap-minimax-h3",
			taskKind:    "video_generation",
			billingType: constant.AIPDDBillingTypeDurationSeconds,
			taskCost:    900,
			synchronous: false,
			execution:   "token_market_video",
		},
	}

	for _, expected := range cases {
		t.Run(expected.model, func(t *testing.T) {
			capability, ok := capabilities[expected.model]
			require.True(t, ok, "catalog must publish %s", expected.model)
			assert.Equal(t, expected.taskKind, capability.TaskKind)
			assert.Equal(t, expected.billingType, capability.BillingType)
			assert.InDelta(t, expected.taskCost, capability.TaskCost, 0.01)
			assert.Equal(t, expected.synchronous, capability.Synchronous)
			assert.Equal(t, expected.execution, capability.ExecutionProtocol)
			if expected.executionPath != "" {
				assert.Equal(t, expected.executionPath, capability.ExecutionPath)
			}
		})
	}

	// The gateway may only pre-charge one second for synchronous per-second
	// media; asynchronous video keeps the task pricing pipeline instead.
	assert.Equal(t, map[string]float64{"ap-funasr": 100}, catalog.SynchronousPerSecondPrices())

	// ap-qwen-image-edit is declared image-generation by upstream but is an edit
	// model, so the primary endpoint comes from the task kind, not upstream.
	assert.Equal(t, constant.EndpointTypeImageEdits, capabilities["ap-qwen-image-edit"].EndpointType)
}

func TestFixtureCatalogMinimaxH3ResolutionPricing(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	var pricing *constant.AIPDDSeedancePricing
	for _, capability := range catalog.RuntimeCapabilities() {
		if capability.ModelName == "ap-minimax-h3" {
			pricing = capability.SeedancePricing
		}
	}
	require.NotNil(t, pricing)

	for resolution, amount := range map[string]float64{"480p": 900, "768p": 1200, "1080p": 1500} {
		item, ok := pricing.ByResolution[resolution]
		require.True(t, ok, "ap-minimax-h3 must publish a %s tier", resolution)
		assert.InDelta(t, amount, item.AmountAWCoinPerSecond, 0.01)
	}
}

// The imagePricing undercharge fix, asserted against the published tiers:
// chargeConfig.amountAwcoin only carries the cheapest tier, so a 4K request used
// to be billed at the 1K price.
func TestFixtureCatalogImagePricingTiers(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	pricings := map[string]*constant.AIPDDImagePricing{}
	for _, capability := range catalog.RuntimeCapabilities() {
		if capability.ImagePricing != nil {
			pricings[capability.ModelName] = capability.ImagePricing
		}
	}

	for _, model := range []string{
		"ap-image-2.0",
		"ap-image-flare-2.5",
		"ap-image-lite-2.0",
		"ap-image-sunburst-2.5",
		"ap-nano-2.1",
	} {
		require.Contains(t, pricings, model, "%s must carry tiered image pricing", model)
	}
	// ap-image-fast-2.0 and ap-jimeng-image-4.0 publish a flat per-call price.
	assert.NotContains(t, pricings, "ap-image-fast-2.0")
	assert.NotContains(t, pricings, "ap-jimeng-image-4.0")

	cases := []struct {
		model   string
		size    string
		wantRMB float64
	}{
		{"ap-image-2.0", "1024x1024", 1.5},
		{"ap-image-2.0", "1024x1536", 1.5},
		{"ap-image-2.0", "2048x2048", 2},
		{"ap-image-2.0", "4096x4096", 3},
		{"ap-image-2.0", "3840x2160", 3},
		// flare-2.5 keys its tiers by quality class only, so an unlisted pixel
		// size rounds up to its class instead of falling back to the base price.
		{"ap-image-flare-2.5", "1024x1536", 2},
		{"ap-image-flare-2.5", "4096x4096", 3},
		{"ap-image-lite-2.0", "1024x1024", 1},
		{"ap-image-lite-2.0", "1024x1536", 1.25},
		{"ap-image-lite-2.0", "4096x4096", 1.5},
		{"ap-image-sunburst-2.5", "1024x1024", 4.0 / 3.0},
		{"ap-image-sunburst-2.5", "4096x4096", 7.0 / 3.0},
		// nano-2.1 lists 1K below its base price, so the base is not the minimum.
		{"ap-nano-2.1", "1024x1024", 0.8},
		{"ap-nano-2.1", "2048x2048", 1},
	}

	for _, testCase := range cases {
		t.Run(testCase.model+" "+testCase.size, func(t *testing.T) {
			pricing, ok := pricings[testCase.model]
			require.True(t, ok)
			assert.InDelta(t, testCase.wantRMB, pricing.RatioFor("", testCase.size), 0.001)
		})
	}
}

func TestFixtureCatalogRoundTrips(t *testing.T) {
	catalog := loadFixtureCatalog(t)

	data, err := MarshalAtomic(catalog)
	require.NoError(t, err)

	var reparsed AtomicCatalog
	require.NoError(t, common.Unmarshal(data, &reparsed))
	assert.Equal(t, catalog.AWCoinRate, reparsed.AWCoinRate)
	assert.Equal(t, catalog.Revision, reparsed.Revision)
	assert.Equal(t, catalog.ModelNames(), reparsed.ModelNames())
}

func containsEndpointType(values []constant.EndpointType, want constant.EndpointType) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
