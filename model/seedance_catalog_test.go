package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedanceCatalogCodeGeneratedWhenOmitted(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SeedanceBaseModel{}, &SeedanceAdminAudit{}))
	require.NoError(t, DB.Exec("DELETE FROM seedance_base_models").Error)
	require.NoError(t, DB.Exec("DELETE FROM seedance_admin_audits").Error)
	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM seedance_base_models").Error
		_ = DB.Exec("DELETE FROM seedance_admin_audits").Error
	})

	item := &SeedanceBaseModel{
		DisplayName:     "Seedance 2.0 Fast",
		ProviderModelID: "doubao-seedance-2-0-fast",
		CostMatrixJSON:  "[]",
		Enabled:         true,
	}
	require.NoError(t, SaveSeedanceBaseModel(item, 100))
	require.Regexp(t, `^base-[0-9a-f-]{36}$`, item.Code)
	require.Regexp(t, `^enhancement-[0-9a-f-]{36}$`, ensureSeedanceCatalogCode("", "enhancement"))

	firstID := item.ID
	originalCode := item.Code
	item.Code = ""
	item.DisplayName = "Seedance 2.0 Fast Updated"
	require.NoError(t, SaveSeedanceBaseModel(item, 100))
	require.Equal(t, originalCode, item.Code)
	require.NotEqual(t, firstID, item.ID)
	require.Equal(t, 2, item.Revision)
}

func TestSeedanceBaseModelCostChangeCreatesImmutableRevision(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&SeedanceBaseModel{}, &SeedanceAdminAudit{}))
	require.NoError(t, DB.Exec("DELETE FROM seedance_base_models").Error)
	require.NoError(t, DB.Exec("DELETE FROM seedance_admin_audits").Error)
	t.Cleanup(func() {
		_ = DB.Exec("DELETE FROM seedance_base_models").Error
		_ = DB.Exec("DELETE FROM seedance_admin_audits").Error
	})

	first := &SeedanceBaseModel{
		Code: "seedance-2-0", DisplayName: "Seedance 2.0", ProviderModelID: "doubao-seedance-2-0",
		CostMatrixJSON: `[
			{"source_resolution":"720p","has_reference_video":false,"cost_micro_rmb_per_second":1000000},
			{"source_resolution":"720p","has_reference_video":true,"cost_micro_rmb_per_second":1500000}
		]`,
		Enabled: true,
	}
	require.NoError(t, SaveSeedanceBaseModel(first, 100))
	require.Equal(t, 1, first.Revision)
	firstID := first.ID

	second := *first
	second.CostMatrixJSON = `[
		{"source_resolution":"720p","has_reference_video":false,"cost_micro_rmb_per_second":1200000},
		{"source_resolution":"720p","has_reference_video":true,"cost_micro_rmb_per_second":1700000}
	]`
	require.NoError(t, SaveSeedanceBaseModel(&second, 100))
	require.NotEqual(t, firstID, second.ID)
	require.Equal(t, 2, second.Revision)

	var storedFirst SeedanceBaseModel
	require.NoError(t, DB.First(&storedFirst, firstID).Error)
	require.False(t, storedFirst.Current)
	require.False(t, storedFirst.Enabled)
	cost, err := ResolveSeedanceBaseUnitCost(&second, "720p", true)
	require.NoError(t, err)
	require.EqualValues(t, 1_700_000, cost)
}

func TestSeedanceEstimatedCostsAreOptional(t *testing.T) {
	baseEntries, err := ValidateSeedanceBaseCostMatrix("")
	require.NoError(t, err)
	require.Empty(t, baseEntries)

	baseCost, err := ResolveSeedanceBaseUnitCost(&SeedanceBaseModel{CostMatrixJSON: "[]"}, "720p", false)
	require.NoError(t, err)
	require.Zero(t, baseCost)

	enhancementEntries, err := ValidateSeedanceEnhancementCostMatrix("[]")
	require.NoError(t, err)
	require.Empty(t, enhancementEntries)

	enhancementCost, err := ResolveSeedanceEnhancementUnitCost(&SeedanceEnhancementModel{CostMatrixJSON: "[]"}, "1080p", 30)
	require.NoError(t, err)
	require.Zero(t, enhancementCost)
}
