package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestSeedanceOfferingSnapshotChangeRequiresANewPricingVersion(t *testing.T) {
	cost := int64(1_200_000)
	base := &SeedanceModelOffering{
		DisplayName: "Public video", ProviderModelID: "private-model",
		ResolutionRulesJSON: `{}`, DurationRulesJSON: `{}`,
		EnhancementProviderID: 1, EnhancementServiceCode: "private-code",
		EnhancementSpecificationJSON: `{}`, EnhancementSpecificationVersion: "spec-v1",
		ModelSaleMicroRMB: 8_000_000, ServiceChargeMicroRMB: 1_800_000,
		ProviderCostMicroRMB: &cost, VolcengineUnitCostMicroRMB: 3_000_000,
		PricingVersion: "price-v1", Enabled: true,
	}

	enabledOnly := *base
	enabledOnly.Enabled = false
	require.False(t, seedanceOfferingSnapshotChanged(base, &enabledOnly))
	require.NoError(t, validateSeedanceOfferingVersionChange(base, &enabledOnly))

	priceChanged := *base
	priceChanged.ModelSaleMicroRMB++
	require.True(t, seedanceOfferingSnapshotChanged(base, &priceChanged))
	require.Equal(t, base.PricingVersion, priceChanged.PricingVersion)
	require.ErrorContains(t, validateSeedanceOfferingVersionChange(base, &priceChanged), "pricing_version")
	priceChanged.PricingVersion = "price-v2"
	require.NoError(t, validateSeedanceOfferingVersionChange(base, &priceChanged))

	providerChanged := *base
	providerChanged.EnhancementProviderID++
	require.True(t, seedanceOfferingSnapshotChanged(base, &providerChanged))
}

func TestPublishedSeedanceOfferingOwnsNeutralMetadataAndPublicPrice(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(
		&SeedanceChannelConfig{}, &SeedanceVolcengineCredential{},
		&MediaEnhancementProvider{}, &SeedanceModelOffering{}, &SeedanceAdminAudit{},
	))
	for _, table := range []string{
		"seedance_admin_audits", "seedance_model_offerings", "media_enhancement_providers",
		"seedance_volcengine_credentials", "seedance_channel_configs",
	} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"seedance_admin_audits", "seedance_model_offerings", "media_enhancement_providers",
			"seedance_volcengine_credentials", "seedance_channel_configs",
		} {
			DB.Exec("DELETE FROM " + table)
		}
	})

	previousExchangeRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 8
	t.Cleanup(func() {
		operation_setting.USDExchangeRate = previousExchangeRate
		InvalidatePricingCache()
	})

	const modelName = "Public cinematic video"
	aipddVendor := Vendor{Name: "AIPDD", Icon: constant.AIPDDLogoPath, Status: 1}
	require.NoError(t, DB.Create(&aipddVendor).Error)
	require.NoError(t, DB.Create(&Model{
		ModelName: modelName, Description: "internal provider workflow",
		Icon: constant.AIPDDLogoPath, Tags: "AIPDD,增强", VendorID: aipddVendor.Id,
		Status: 1, SyncOfficial: 1, NameRule: NameRuleExact,
	}).Error)
	channel := Channel{
		Type: constant.ChannelTypeSeedance, Name: "字节跳动 Seedance", Key: "managed",
		Group: "default", Status: common.ChannelStatusEnabled,
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, DB.Create(&SeedanceChannelConfig{
		ChannelID: channel.Id, InstanceID: "instance-test", Status: SeedanceConfigActive,
		LastVerifiedAt: 1,
	}).Error)
	require.NoError(t, DB.Create(&SeedanceVolcengineCredential{
		ChannelID: channel.Id, Version: 1, Status: SeedanceCredentialActive,
	}).Error)
	provider := MediaEnhancementProvider{
		ProviderType: SeedanceProviderDirect, DisplayName: "private provider",
		ServiceEndpoint: "https://provider.example.test", ServiceCode: "private-code",
		Status: SeedanceConfigActive,
	}
	require.NoError(t, DB.Create(&provider).Error)
	offering := SeedanceModelOffering{
		ChannelID: channel.Id, DisplayName: modelName, ProviderModelID: "private-model-id",
		ResolutionRulesJSON: `{}`, DurationRulesJSON: `{}`,
		EnhancementProviderID: provider.ID, EnhancementServiceCode: "private-code",
		EnhancementSpecificationJSON: `{}`, EnhancementSpecificationVersion: "spec-v1",
		ModelSaleMicroRMB: 8_000_000, ServiceChargeMicroRMB: 1_800_000,
		VolcengineUnitCostMicroRMB: 3_000_000, PricingVersion: "price-v1", Enabled: true,
	}
	require.NoError(t, SaveSeedanceModelOffering(&offering, 1, "publish public offering"))

	var metadata Model
	require.NoError(t, DB.Where("model_name = ?", modelName).First(&metadata).Error)
	require.Equal(t, seedancePublicModelDescription, metadata.Description)
	require.Equal(t, seedancePublicModelTags, metadata.Tags)
	require.NotContains(t, metadata.Tags, "AIPDD")
	require.Equal(t, marshalEndpointTypes([]constant.EndpointType{constant.EndpointTypeOpenAIVideo}), metadata.Endpoints)
	var vendor Vendor
	require.NoError(t, DB.First(&vendor, metadata.VendorID).Error)
	require.Equal(t, seedancePublicVendorName, vendor.Name)

	InvalidatePricingCache()
	pricing, found := findPricingForTest(GetPricing(), modelName)
	require.True(t, found)
	require.True(t, pricing.IsIndependentSeedance)
	require.Equal(t, 1.0, pricing.ModelPrice, "8 CNY must be represented as 1 USD before the public CNY conversion")
	require.Equal(t, 1, pricing.QuotaType)
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIVideo}, pricing.SupportedEndpointTypes)
}
