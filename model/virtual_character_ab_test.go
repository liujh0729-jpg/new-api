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

func TestMigrateVirtualCharacterABDataOfflinesPublicAndDeletesLegacyPrivate(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	publicItem := &VirtualCharacter{Scope: VirtualCharacterScopePublic, SourceType: VirtualCharacterSourceVolc, Name: "preset", Status: VirtualCharacterStatusActive, VolcAssetID: "asset-public"}
	privateItem := &VirtualCharacter{UserID: 10, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceAIPDD, Name: "fictional", Status: VirtualCharacterStatusActive, AIPDDAssetID: 91, AIPDDFileID: "file-91"}
	require.NoError(t, DB.Create(publicItem).Error)
	require.NoError(t, DB.Create(privateItem).Error)

	require.NoError(t, MigrateVirtualCharacterABData())
	var migrated VirtualCharacter
	require.NoError(t, DB.First(&migrated, publicItem.ID).Error)
	require.Equal(t, VirtualCharacterSourceVolcPreset, migrated.SourceType)
	require.Equal(t, VirtualCharacterStatusOffline, migrated.Status)
	require.NotNil(t, migrated.PrimaryAssetID)
	var asset VirtualCharacterAsset
	require.NoError(t, DB.First(&asset, *migrated.PrimaryAssetID).Error)
	require.Equal(t, "asset-public", asset.ProviderAssetID)

	var deleted VirtualCharacter
	require.True(t, errors.Is(DB.Unscoped().First(&deleted, privateItem.ID).Error, gorm.ErrRecordNotFound))
	var jobs []VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("character_id = ?", privateItem.ID).Order("id ASC").Find(&jobs).Error)
	require.Len(t, jobs, 2)
	require.Equal(t, "aipdd_asset", jobs[0].TargetType)
	require.Equal(t, "aipdd_file", jobs[1].TargetType)

	// The migration is idempotent and must not offline manifest-managed rows.
	require.NoError(t, DB.Model(&migrated).Update("status", VirtualCharacterStatusActive).Error)
	require.NoError(t, MigrateVirtualCharacterABData())
	require.NoError(t, DB.First(&migrated, publicItem.ID).Error)
	require.Equal(t, VirtualCharacterStatusActive, migrated.Status)
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
	require.NoError(t, BeginVirtualCharacterAssetDelete(character.ID, second.ID, 77))
	var refreshed VirtualCharacter
	require.NoError(t, DB.First(&refreshed, character.ID).Error)
	require.NotNil(t, refreshed.PrimaryAssetID)
	require.Equal(t, first.ID, *refreshed.PrimaryAssetID)
	var job VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("asset_id = ? AND target_type = ?", second.ID, "volc_asset").First(&job).Error)
}
