package model

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func cleanupVirtualCharacterABTables(t *testing.T) {
	t.Helper()
	for _, table := range []string{"virtual_character_cleanup_jobs", "virtual_character_catalog_imports", "virtual_character_validation_sessions", "virtual_character_assets", "virtual_character_provider_accounts", "virtual_character_tasks", "virtual_character_user_limits", "virtual_characters"} {
		require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
	}
	t.Cleanup(func() {
		for _, table := range []string{"virtual_character_cleanup_jobs", "virtual_character_catalog_imports", "virtual_character_validation_sessions", "virtual_character_assets", "virtual_character_provider_accounts", "virtual_character_tasks", "virtual_character_user_limits", "virtual_characters"} {
			DB.Exec("DELETE FROM " + table)
		}
	})
}

func TestMigrateVirtualCharacterABDataOfflinesPublicAndDeletesLegacyPrivateOnly(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	publicItem := &VirtualCharacter{Scope: VirtualCharacterScopePublic, SourceType: VirtualCharacterSourceVolc, Name: "preset", Status: VirtualCharacterStatusActive, VolcAssetID: "asset-public"}
	legacyPrivate := &VirtualCharacter{UserID: 10, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceAIPDD, Name: "fictional", Status: VirtualCharacterStatusActive, AIPDDAssetID: 91, AIPDDFileID: "file-91"}
	slot := 1
	aigcSlot := 1
	realPrivate := &VirtualCharacter{UserID: 11, Slot: &slot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcRealPerson, Name: "actor", Status: VirtualCharacterStatusActive, ProviderAccountID: 3, ProviderGroupID: "group-real"}
	aigcPrivate := &VirtualCharacter{UserID: 12, Slot: &aigcSlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Name: "virtual", Status: VirtualCharacterStatusActive, ProviderAccountID: 3, ProviderGroupID: "group-aigc"}
	require.NoError(t, DB.Create(publicItem).Error)
	require.NoError(t, DB.Create(legacyPrivate).Error)
	require.NoError(t, DB.Create(realPrivate).Error)
	require.NoError(t, DB.Create(aigcPrivate).Error)
	require.NoError(t, DB.Create(&VirtualCharacterAsset{CharacterID: realPrivate.ID, ProviderAccountID: 3, ProviderAssetID: "asset-real", Name: "face", AssetType: VirtualCharacterAssetTypeImage, Status: VirtualCharacterAssetStatusActive, IsPrimary: true}).Error)
	require.NoError(t, DB.Create(&VirtualCharacterAsset{CharacterID: aigcPrivate.ID, ProviderAccountID: 3, ProviderAssetID: "asset-aigc", Name: "primary", AssetType: VirtualCharacterAssetTypeImage, Status: VirtualCharacterAssetStatusActive, IsPrimary: true}).Error)

	require.NoError(t, MigrateVirtualCharacterABData())
	var migrated VirtualCharacter
	require.NoError(t, DB.First(&migrated, publicItem.ID).Error)
	require.Equal(t, VirtualCharacterSourceVolcPreset, migrated.SourceType)
	require.Equal(t, VirtualCharacterStatusOffline, migrated.Status)
	require.NotNil(t, migrated.PrimaryAssetID)
	var asset VirtualCharacterAsset
	require.NoError(t, DB.First(&asset, *migrated.PrimaryAssetID).Error)
	require.Equal(t, "asset-public", asset.ProviderAssetID)

	var missing VirtualCharacter
	require.True(t, errors.Is(DB.Unscoped().First(&missing, legacyPrivate.ID).Error, gorm.ErrRecordNotFound))
	var keptReal VirtualCharacter
	require.NoError(t, DB.First(&keptReal, realPrivate.ID).Error)
	require.Equal(t, VirtualCharacterSourceVolcRealPerson, keptReal.SourceType)
	var keptAIGC VirtualCharacter
	require.NoError(t, DB.First(&keptAIGC, aigcPrivate.ID).Error)
	require.Equal(t, VirtualCharacterSourceVolcAIGC, keptAIGC.SourceType)
	var jobs []VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("character_id = ?", legacyPrivate.ID).Order("id ASC").Find(&jobs).Error)
	require.GreaterOrEqual(t, len(jobs), 1)

	// The migration is idempotent and must not offline manifest-managed rows,
	// wipe current private characters, or duplicate cleanup jobs.
	jobCountBefore := len(jobs)
	require.NoError(t, DB.Model(&migrated).Update("status", VirtualCharacterStatusActive).Error)
	require.NoError(t, MigrateVirtualCharacterABData())
	require.NoError(t, DB.First(&migrated, publicItem.ID).Error)
	require.Equal(t, VirtualCharacterStatusActive, migrated.Status)
	var stillAIGC VirtualCharacter
	require.NoError(t, DB.First(&stillAIGC, aigcPrivate.ID).Error)
	require.Equal(t, VirtualCharacterSourceVolcAIGC, stillAIGC.SourceType)
	require.NoError(t, DB.Where("character_id = ?", legacyPrivate.ID).Find(&jobs).Error)
	require.Equal(t, jobCountBefore, len(jobs))
}

func TestApplyVirtualCharacterCatalogAtomicallyReplacesActiveSet(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	first, err := ApplyVirtualCharacterCatalog("v1", "hash-1", []VirtualCharacterCatalogEntry{
		{AssetID: "asset-a", Name: "A", TagsJSON: "[]", CoverURL: "https://example.com/a.png", Enabled: true},
		{AssetID: "asset-b", Name: "B", TagsJSON: "[]", CoverURL: "https://example.com/b.png", Enabled: true},
	}, 1, 7)
	require.NoError(t, err)
	require.Equal(t, 2, first.Created)

	second, err := ApplyVirtualCharacterCatalog("v2", "hash-2", []VirtualCharacterCatalogEntry{
		{AssetID: "asset-b", Name: "B2", TagsJSON: "[]", CoverURL: "https://example.com/b2.png", Enabled: true},
		{AssetID: "asset-c", Name: "C", TagsJSON: "[]", CoverURL: "https://example.com/c.png", Enabled: true},
	}, 2, 7)
	require.NoError(t, err)
	require.Equal(t, 1, second.Created)
	require.Equal(t, 1, second.Updated)
	require.Equal(t, 1, second.Offlined)

	findCharacter := func(providerAssetID string) VirtualCharacter {
		var asset VirtualCharacterAsset
		require.NoError(t, DB.Where("provider_asset_id = ?", providerAssetID).First(&asset).Error)
		var character VirtualCharacter
		require.NoError(t, DB.First(&character, asset.CharacterID).Error)
		return character
	}
	a, b, c := findCharacter("asset-a"), findCharacter("asset-b"), findCharacter("asset-c")
	require.Equal(t, VirtualCharacterStatusOffline, a.Status)
	require.Equal(t, VirtualCharacterStatusActive, b.Status)
	require.Equal(t, "B2", b.Name)
	require.Equal(t, VirtualCharacterStatusActive, c.Status)
}

func TestListVirtualCharactersFiltersByStructuredFacets(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	ageAMin, ageAMax := 20, 40
	ageBMin, ageBMax := 40, 60
	require.NoError(t, DB.Create(&VirtualCharacter{
		UserID: 0, Scope: VirtualCharacterScopePublic, SourceType: VirtualCharacterSourceVolcPreset,
		Name: "CN Male", Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted,
		Nationality: "中国", Gender: "男", AgeMin: &ageAMin, AgeMax: &ageAMax, Occupation: "镖头",
		TagsJSON: `["中国","男","20-40岁"]`,
	}).Error)
	require.NoError(t, DB.Create(&VirtualCharacter{
		UserID: 0, Scope: VirtualCharacterScopePublic, SourceType: VirtualCharacterSourceVolcPreset,
		Name: "JP Female", Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted,
		Nationality: "日本", Gender: "女", AgeMin: &ageBMin, AgeMax: &ageBMax, Occupation: "教师",
		TagsJSON: `["日本","女","40-60岁"]`,
	}).Error)

	items, total, err := ListVirtualCharacters(0, VirtualCharacterScopePublic, false, VirtualCharacterListFilter{
		Nationality: "中国", Gender: "男",
	}, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, items, 1)
	require.Equal(t, "CN Male", items[0].Name)

	bandMin, bandMax := 20, 40
	items, total, err = ListVirtualCharacters(0, VirtualCharacterScopePublic, false, VirtualCharacterListFilter{
		AgeMin: &bandMin, AgeMax: &bandMax,
	}, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "CN Male", items[0].Name)

	items, total, err = ListVirtualCharacters(0, VirtualCharacterScopePublic, false, VirtualCharacterListFilter{
		Keyword: "教师",
	}, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "JP Female", items[0].Name)
}

func TestBackfillVirtualCharacterStructuredFacetsFromTags(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	character := &VirtualCharacter{
		UserID: 0, Scope: VirtualCharacterScopePublic, SourceType: VirtualCharacterSourceVolcPreset,
		Name: "Legacy", Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted,
		TagsJSON: `["美国","女","60-80岁","医生","冷静"]`,
	}
	require.NoError(t, DB.Create(character).Error)
	require.NoError(t, BackfillVirtualCharacterStructuredFacets())
	var refreshed VirtualCharacter
	require.NoError(t, DB.First(&refreshed, character.ID).Error)
	require.Equal(t, "美国", refreshed.Nationality)
	require.Equal(t, "女", refreshed.Gender)
	require.NotNil(t, refreshed.AgeMin)
	require.Equal(t, 60, *refreshed.AgeMin)
	require.NotNil(t, refreshed.AgeMax)
	require.Equal(t, 80, *refreshed.AgeMax)
}

func TestCompleteValidationCreatesOneActorGroupAndIsReplaySafe(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	setVirtualCharacterTestLimit(t, "1")
	session := &VirtualCharacterValidationSession{ID: "session-1", UserID: 55, ProviderAccountID: 3, Status: VirtualCharacterValidationPending, StateHash: "state-1", Name: "Actor", TagsJSON: "[]", ExpiresAt: time.Now().Add(time.Minute).Unix()}
	require.NoError(t, CreateVirtualCharacterValidationSession(session))
	first, err := CompleteVirtualCharacterValidation(session.ID, "group-1")
	require.NoError(t, err)
	require.Equal(t, VirtualCharacterSourceVolcRealPerson, first.SourceType)
	require.Equal(t, VirtualCharacterValidationAccepted, first.ValidationStatus)

	second, err := CompleteVirtualCharacterValidation(session.ID, "group-1")
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)
	var count int64
	require.NoError(t, DB.Model(&VirtualCharacter{}).Where("user_id = ?", 55).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestPrimaryAssetAndDeleteOutboxRemainConsistent(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	slot := 1
	character := &VirtualCharacter{UserID: 77, Slot: &slot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcRealPerson, Name: "Actor", Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted, ProviderAccountID: 4, ProviderGroupID: "group-4"}
	require.NoError(t, DB.Create(character).Error)
	first := &VirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: 4, ProviderAssetID: "asset-1", Name: "One", AssetType: VirtualCharacterAssetTypeImage, Status: VirtualCharacterAssetStatusActive}
	second := &VirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: 4, ProviderAssetID: "asset-2", Name: "Two", AssetType: VirtualCharacterAssetTypeImage, Status: VirtualCharacterAssetStatusActive}
	require.NoError(t, CreateVirtualCharacterAsset(first))
	require.NoError(t, CreateVirtualCharacterAsset(second))
	require.NoError(t, SetVirtualCharacterPrimaryAsset(character.ID, second.ID, 77))

	var primaryCount int64
	require.NoError(t, DB.Model(&VirtualCharacterAsset{}).Where("character_id = ? AND is_primary = ?", character.ID, true).Count(&primaryCount).Error)
	require.EqualValues(t, 1, primaryCount)
	require.ErrorIs(t, BeginVirtualCharacterAssetDelete(character.ID, second.ID, 77), ErrVirtualCharacterPrimaryAssetProtected)
	require.NoError(t, BeginVirtualCharacterAssetDelete(character.ID, first.ID, 77))
	var refreshed VirtualCharacter
	require.NoError(t, DB.First(&refreshed, character.ID).Error)
	require.NotNil(t, refreshed.PrimaryAssetID)
	require.Equal(t, second.ID, *refreshed.PrimaryAssetID)
	var job VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("asset_id = ? AND target_type = ?", first.ID, "volc_asset").First(&job).Error)
}
