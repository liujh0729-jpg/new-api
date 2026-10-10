package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

const asrTestModel = "ap-funasr"

func withAIPDDCapabilityForTest(t *testing.T, capability constant.AIPDDCapability) {
	t.Helper()
	previous := constant.GetAIPDDCapabilities()
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{capability})
	t.Cleanup(func() {
		constant.SetAIPDDCapabilities(previous)
	})
}

// installBillingSettingForTest writes the runtime billing mode and task pricing the
// same way the option pipeline does: both are read through the registered config,
// not through a setter.
func installBillingSettingForTest(t *testing.T, mode string, pricing map[string]billing_setting.TaskPricingConfig) {
	t.Helper()
	billing := config.GlobalConfig.Get("billing_setting")
	if billing == nil {
		t.Fatal("billing_setting is not registered")
	}
	snapshot, err := config.ConfigToMap(billing)
	if err != nil {
		t.Fatalf("snapshot billing_setting: %v", err)
	}
	t.Cleanup(func() {
		if err := config.UpdateConfigFromMap(billing, snapshot); err != nil {
			t.Errorf("restore billing_setting: %v", err)
		}
	})

	modeJSON, err := common.Marshal(map[string]string{asrTestModel: mode})
	if err != nil {
		t.Fatalf("marshal billing mode: %v", err)
	}
	pricingJSON, err := common.Marshal(pricing)
	if err != nil {
		t.Fatalf("marshal task pricing: %v", err)
	}
	if err := config.UpdateConfigFromMap(billing, map[string]string{
		"billing_mode": string(modeJSON),
		"task_pricing": string(pricingJSON),
	}); err != nil {
		t.Fatalf("install billing_setting: %v", err)
	}
}

func asrCapability(synchronous bool, billingType constant.AIPDDBillingType) constant.AIPDDCapability {
	return constant.AIPDDCapability{
		ModelName:     asrTestModel,
		Synchronous:   synchronous,
		BillingType:   billingType,
		EndpointType:  constant.EndpointTypeAudioTranscriptions,
		ExecutionPath: "/v1/audio/transcriptions",
	}
}

func flatPerSecondPricing(unitPrice float64) map[string]billing_setting.TaskPricingConfig {
	return map[string]billing_setting.TaskPricingConfig{
		asrTestModel: {
			Unit:                      billing_setting.TaskPricingUnitSecond,
			NoReferenceVideoUnitPrice: unitPrice,
			ReferenceVideoPolicy:      billing_setting.ReferenceVideoPolicySame,
		},
	}
}

func TestSynchronousDurationUnitPrice(t *testing.T) {
	tieredPricing := map[string]billing_setting.TaskPricingConfig{
		asrTestModel: {
			Unit: billing_setting.TaskPricingUnitSecond,
			ByResolution: map[string]billing_setting.TaskPricingTier{
				"480p": {NoReferenceVideoUnitPrice: 0.002, ReferenceVideoPolicy: billing_setting.ReferenceVideoPolicySame},
			},
		},
	}

	tests := []struct {
		name        string
		capability  constant.AIPDDCapability
		mode        string
		pricing     map[string]billing_setting.TaskPricingConfig
		wantPrice   float64
		wantConfigd bool
	}{
		{
			name:        "synchronous per-second ASR",
			capability:  asrCapability(true, constant.AIPDDBillingTypeDurationSeconds),
			mode:        billing_setting.BillingModeTaskPricing,
			pricing:     flatPerSecondPricing(0.001),
			wantPrice:   0.001,
			wantConfigd: true,
		},
		{
			name:       "resolution tiers stay with the task pipeline",
			capability: asrCapability(true, constant.AIPDDBillingTypeDurationSeconds),
			mode:       billing_setting.BillingModeTaskPricing,
			pricing:    tieredPricing,
		},
		{
			name:       "async duration models stay with the task pipeline",
			capability: asrCapability(false, constant.AIPDDBillingTypeDurationSeconds),
			mode:       billing_setting.BillingModeTaskPricing,
			pricing:    flatPerSecondPricing(0.001),
		},
		{
			name:       "per-call audio is never multiplied by seconds",
			capability: asrCapability(true, constant.AIPDDBillingTypePerCall),
			mode:       billing_setting.BillingModeTaskPricing,
			pricing:    flatPerSecondPricing(0.001),
		},
		{
			name:       "an administrator ratio price wins",
			capability: asrCapability(true, constant.AIPDDBillingTypeDurationSeconds),
			mode:       billing_setting.BillingModeRatio,
			pricing:    flatPerSecondPricing(0.001),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withAIPDDCapabilityForTest(t, tt.capability)
			installBillingSettingForTest(t, tt.mode, tt.pricing)

			price, configured := SynchronousDurationUnitPriceUSD(asrTestModel)
			if configured != tt.wantConfigd || price != tt.wantPrice {
				t.Fatalf("SynchronousDurationUnitPriceUSD() = %v, %v; want %v, %v", price, configured, tt.wantPrice, tt.wantConfigd)
			}
		})
	}
}

func TestModelPriceHelperSynchronousDurationReservesOneSecond(t *testing.T) {
	withAIPDDCapabilityForTest(t, asrCapability(true, constant.AIPDDBillingTypeDurationSeconds))
	installBillingSettingForTest(t, billing_setting.BillingModeTaskPricing, flatPerSecondPricing(0.001))

	info := &relaycommon.RelayInfo{OriginModelName: asrTestModel}
	priceData, ok := modelPriceHelperSynchronousDuration(info, types.GroupRatioInfo{GroupRatio: 2})
	if !ok {
		t.Fatal("expected the synchronous duration price path")
	}
	if !priceData.UsePrice || priceData.ModelPrice != 0.001 {
		t.Fatalf("price data = %+v; want usePrice at 0.001 per second", priceData)
	}
	// Exactly one second is reserved: the measured duration arrives as an
	// "other ratio" once the upstream has answered.
	wantQuota := int(0.001 * common.QuotaPerUnit * 2)
	if priceData.QuotaToPreConsume != wantQuota {
		t.Fatalf("QuotaToPreConsume = %d, want %d", priceData.QuotaToPreConsume, wantQuota)
	}
	if len(priceData.OtherRatios) != 0 {
		t.Fatalf("pre-consume must not carry ratios, got %v", priceData.OtherRatios)
	}
	if info.PriceData.ModelPrice != priceData.ModelPrice {
		t.Fatal("price data was not stored on the relay info")
	}
}
