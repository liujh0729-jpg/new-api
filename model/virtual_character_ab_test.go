package model

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func cleanupVirtualCharacterABTables(t *testing.T) {
	t.Helper()
	for _, table := range []string{"virtual_character_task_references", "virtual_character_authorizations", "virtual_character_cleanup_jobs", "virtual_character_catalog_imports", "virtual_character_validation_sessions", "virtual_character_assets", "virtual_character_provider_accounts", "virtual_character_tasks", "virtual_character_user_limits", "virtual_characters"} {
		if DB.Migrator().HasTable(table) {
			require.NoError(t, DB.Exec("DELETE FROM "+table).Error)
		}
	}
	t.Cleanup(func() {
		for _, table := range []string{"virtual_character_task_references", "virtual_character_authorizations", "virtual_character_cleanup_jobs", "virtual_character_catalog_imports", "virtual_character_validation_sessions", "virtual_character_assets", "virtual_character_provider_accounts", "virtual_character_tasks", "virtual_character_user_limits", "virtual_characters"} {
			if DB.Migrator().HasTable(table) {
				DB.Exec("DELETE FROM " + table)
			}
		}
	})
}

func TestNormalizeLegacyRealPeopleBlocksOnlyRowsWithoutAuthorization(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	require.NoError(t, DB.AutoMigrate(&VirtualCharacterAuthorization{}))
	legacySlot := 1
	legacy := &VirtualCharacter{UserID: 501, Slot: &legacySlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcRealPerson, Name: "Legacy", Status: VirtualCharacterStatusActive}
	activeSlot := 1
	active := &VirtualCharacter{UserID: 502, RealPersonSlot: &activeSlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcRealPerson, Name: "Active", Status: VirtualCharacterStatusActive}
	require.NoError(t, DB.Create(legacy).Error)
	require.NoError(t, DB.Create(active).Error)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&VirtualCharacterAuthorization{CharacterID: active.ID, UserID: active.UserID, Status: VirtualCharacterAuthorizationActive, ValidFrom: now - 60, ValidUntil: now + 3600}).Error)

	require.NoError(t, NormalizeLegacyRealPersonSlots())
	require.NoError(t, NormalizeLegacyRealPersonSlots())

	var blocked VirtualCharacter
	require.NoError(t, DB.First(&blocked, legacy.ID).Error)
	require.Equal(t, VirtualCharacterStatusBlocked, blocked.Status)
	require.Nil(t, blocked.Slot)
	require.Nil(t, blocked.RealPersonSlot)
	var legacyAuthorization VirtualCharacterAuthorization
	require.NoError(t, DB.Where("character_id = ?", legacy.ID).First(&legacyAuthorization).Error)
	require.Equal(t, VirtualCharacterAuthorizationFailed, legacyAuthorization.Status)

	var unchanged VirtualCharacter
	require.NoError(t, DB.First(&unchanged, active.ID).Error)
	require.Equal(t, VirtualCharacterStatusActive, unchanged.Status)
	require.NotNil(t, unchanged.RealPersonSlot)
}

func TestMigrateVirtualCharacterABDataOfflinesPublicAndDeletesLegacyPrivateOnly(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	publicItem := &VirtualCharacter{Scope: VirtualCharacterScopePublic, SourceType: VirtualCharacterSourceVolc, Name: "preset", Status: VirtualCharacterStatusActive, VolcAssetID: "asset-public"}
	legacyPrivate := &VirtualCharacter{UserID: 10, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceAIPDD, Name: "fictional", Status: VirtualCharacterStatusActive, AIPDDAssetID: 91, AIPDDFileID: "file-91"}
	slot := 1
	aigcSlot := 1
	realPrivate := &VirtualCharacter{UserID: 11, Slot: &slot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcRealPerson, Name: "actor", Status: VirtualCharacterStatusActive, ProviderAccountID: 3, ProviderGroupID: "group-real", ProviderAssetID: "asset-real"}
	aigcPrivate := &VirtualCharacter{UserID: 12, Slot: &aigcSlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Name: "virtual", Status: VirtualCharacterStatusActive, ProviderAccountID: 3, ProviderGroupID: "group-aigc", ProviderAssetID: "asset-aigc"}
	require.NoError(t, DB.Create(publicItem).Error)
	require.NoError(t, DB.Create(legacyPrivate).Error)
	require.NoError(t, DB.Create(realPrivate).Error)
	require.NoError(t, DB.Create(aigcPrivate).Error)

	require.NoError(t, MigrateVirtualCharacterABData())
	var migrated VirtualCharacter
	require.NoError(t, DB.First(&migrated, publicItem.ID).Error)
	require.Equal(t, VirtualCharacterSourceVolcPreset, migrated.SourceType)
	require.Equal(t, VirtualCharacterStatusOffline, migrated.Status)
	require.Equal(t, "asset-public", migrated.ProviderAssetID)

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
		var character VirtualCharacter
		require.NoError(t, DB.Where("provider_asset_id = ?", providerAssetID).First(&character).Error)
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

func TestListVirtualCharactersFiltersByAssetType(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	slotImage, slotVideo, slotEmpty := 1, 2, 3
	require.NoError(t, DB.Create(&VirtualCharacter{
		UserID: 91, Slot: &slotImage, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC,
		Name: "Image material", Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted,
		AssetType: VirtualCharacterAssetTypeImage,
	}).Error)
	require.NoError(t, DB.Create(&VirtualCharacter{
		UserID: 91, Slot: &slotVideo, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC,
		Name: "Video material", Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted,
		AssetType: VirtualCharacterAssetTypeVideo,
	}).Error)
	require.NoError(t, DB.Create(&VirtualCharacter{
		UserID: 91, Slot: &slotEmpty, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC,
		Name: "Legacy material", Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted,
	}).Error)

	items, total, err := ListVirtualCharacters(91, VirtualCharacterScopePrivate, false, VirtualCharacterListFilter{
		AssetType: VirtualCharacterAssetTypeVideo,
	}, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "Video material", items[0].Name)

	items, total, err = ListVirtualCharacters(91, VirtualCharacterScopePrivate, false, VirtualCharacterListFilter{
		AssetType: VirtualCharacterAssetTypeImage,
	}, 0, 20)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	names := []string{items[0].Name, items[1].Name}
	require.Contains(t, names, "Image material")
	require.Contains(t, names, "Legacy material")
}

func TestEffectiveVirtualCharacterAssetTypeDefaultsToImage(t *testing.T) {
	require.Equal(t, VirtualCharacterAssetTypeImage, EffectiveVirtualCharacterAssetType(""))
	require.Equal(t, VirtualCharacterAssetTypeImage, EffectiveVirtualCharacterAssetType("image"))
	require.Equal(t, VirtualCharacterAssetTypeVideo, EffectiveVirtualCharacterAssetType("VIDEO"))
	require.Equal(t, VirtualCharacterAssetTypeAudio, EffectiveVirtualCharacterAssetType("audio"))
	require.Equal(t, VirtualCharacterAssetTypeImage, EffectiveVirtualCharacterAssetType("unknown"))
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

func TestPendingRealPersonReservationIsHiddenAndCancellationRemovesIt(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	character, authorization, _, err := ReserveRealPersonVirtualCharacter(56, 3, "Pending actor", "", "[]")
	require.NoError(t, err)
	require.Zero(t, authorization.ValidUntil)
	session := &VirtualCharacterValidationSession{
		ID: "session-cancel", UserID: 56, ProviderAccountID: 3, CharacterID: character.ID,
		Status: VirtualCharacterValidationPending, StateHash: "state-cancel", Name: character.Name,
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}
	require.NoError(t, CreateVirtualCharacterValidationSession(session))

	items, total, err := ListVirtualCharacters(56, VirtualCharacterScopePrivate, false, VirtualCharacterListFilter{
		SourceType: VirtualCharacterSourceVolcRealPerson,
	}, 0, 20)
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, items)

	cancelled, err := CancelReservedVirtualCharacterValidation(session.ID, session.UserID)
	require.NoError(t, err)
	require.True(t, cancelled)

	var storedSession VirtualCharacterValidationSession
	require.NoError(t, DB.First(&storedSession, "id = ?", session.ID).Error)
	require.Equal(t, VirtualCharacterValidationCancelled, storedSession.Status)
	var characterCount, authorizationCount int64
	require.NoError(t, DB.Unscoped().Model(&VirtualCharacter{}).Where("id = ?", character.ID).Count(&characterCount).Error)
	require.NoError(t, DB.Model(&VirtualCharacterAuthorization{}).Where("character_id = ?", character.ID).Count(&authorizationCount).Error)
	require.Zero(t, characterCount)
	require.Zero(t, authorizationCount)
}

func TestOfficialH5CompletionActivatesReservedVerificationEvidence(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	character, _, _, err := ReserveRealPersonVirtualCharacter(57, 4, "Verified actor", "", "[]")
	require.NoError(t, err)
	session := &VirtualCharacterValidationSession{
		ID: "session-official-h5", UserID: 57, ProviderAccountID: 4, CharacterID: character.ID,
		Status: VirtualCharacterValidationPending, StateHash: "state-official-h5", Name: character.Name,
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}
	require.NoError(t, CreateVirtualCharacterValidationSession(session))

	completed, err := CompleteReservedVirtualCharacterValidation(session.ID, "group-official", strings.Repeat("a", 64))
	require.NoError(t, err)
	require.Equal(t, VirtualCharacterValidationAccepted, completed.ValidationStatus)

	authorization, err := GetVirtualCharacterAuthorization(character.ID)
	require.NoError(t, err)
	require.Equal(t, VirtualCharacterAuthorizationSynchronizing, authorization.Status)
	require.Equal(t, VirtualCharacterProviderAssetAwaitingUpload, authorization.ProviderAssetStatus)
	require.Greater(t, authorization.HolderScopeAcceptedAt, int64(0))
	require.Zero(t, authorization.ValidUntil)
	stored, err := GetVirtualCharacterByID(character.ID)
	require.NoError(t, err)
	require.Empty(t, stored.ProviderAssetID)
	require.Zero(t, stored.AssetNextPollAt)
	due, err := ListVirtualCharactersToPoll(time.Now().Unix(), 10)
	require.NoError(t, err)
	require.Empty(t, due)
}

func TestFailedRealPersonValidationRemovesReservedCharacter(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	character, _, _, err := ReserveRealPersonVirtualCharacter(58, 5, "Failed actor", "", "[]")
	require.NoError(t, err)
	session := &VirtualCharacterValidationSession{
		ID: "session-failed", UserID: 58, ProviderAccountID: 5, CharacterID: character.ID,
		Status: VirtualCharacterValidationPending, StateHash: "state-failed", Name: character.Name,
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
	}
	require.NoError(t, CreateVirtualCharacterValidationSession(session))
	require.NoError(t, FailReservedVirtualCharacterValidation(session.ID, VirtualCharacterValidationFailed, "rejected", "validation failed"))

	var storedSession VirtualCharacterValidationSession
	require.NoError(t, DB.First(&storedSession, "id = ?", session.ID).Error)
	require.Equal(t, VirtualCharacterValidationFailed, storedSession.Status)
	var characterCount int64
	require.NoError(t, DB.Unscoped().Model(&VirtualCharacter{}).Where("id = ?", character.ID).Count(&characterCount).Error)
	require.Zero(t, characterCount)
}

func TestCollapseAssetsKeepsOneImageAndQueuesEveryExtra(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	require.NoError(t, DB.AutoMigrate(&legacyVirtualCharacterAsset{}))
	if !DB.Migrator().HasColumn(&legacyVirtualCharacterPrimary{}, "PrimaryAssetID") {
		require.NoError(t, DB.Exec("ALTER TABLE virtual_characters ADD COLUMN primary_asset_id integer").Error)
	}
	slot := 1
	character := &VirtualCharacter{UserID: 77, Slot: &slot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Name: "Actor", Status: VirtualCharacterStatusActive, ValidationStatus: VirtualCharacterValidationAccepted, ProviderAccountID: 4, ProviderGroupID: "group-4"}
	require.NoError(t, DB.Create(character).Error)
	publicPreset := &VirtualCharacter{Scope: VirtualCharacterScopePublic, SourceType: VirtualCharacterSourceVolcPreset, Name: "Preset", Status: VirtualCharacterStatusActive, ProviderAssetID: "catalog-asset"}
	require.NoError(t, DB.Create(publicPreset).Error)
	noImageSlot := 2
	noImage := &VirtualCharacter{UserID: 78, Slot: &noImageSlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Name: "Audio only", Status: VirtualCharacterStatusActive, ProviderAccountID: 4, ProviderGroupID: "group-audio-only"}
	require.NoError(t, DB.Create(noImage).Error)
	processingSlot := 3
	processingCharacter := &VirtualCharacter{UserID: 79, Slot: &processingSlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Name: "Processing", Status: VirtualCharacterStatusActive, ProviderAccountID: 4, ProviderGroupID: "group-processing"}
	require.NoError(t, DB.Create(processingCharacter).Error)
	failedSlot := 4
	failedCharacter := &VirtualCharacter{UserID: 80, Slot: &failedSlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Name: "Failed", Status: VirtualCharacterStatusActive, ProviderAccountID: 4, ProviderGroupID: "group-failed"}
	require.NoError(t, DB.Create(failedCharacter).Error)
	video := &legacyVirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: 4, ProviderAssetID: "video-primary", AssetType: "Video", Status: "Active", IsPrimary: true, StagingFileID: "file-video"}
	activeImage := &legacyVirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: 4, ProviderAssetID: "image-keep", AssetType: "Image", Status: "Active", StagingFileID: "file-keep", MimeType: "image/png", FileSize: 123}
	deletedImage := &legacyVirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: 4, ProviderAssetID: "image-deleted", AssetType: "Image", Status: "Active", StagingFileID: "file-deleted", DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}}
	require.NoError(t, DB.Create(video).Error)
	require.NoError(t, DB.Create(activeImage).Error)
	require.NoError(t, DB.Create(deletedImage).Error)
	require.NoError(t, DB.Create(&legacyVirtualCharacterAsset{CharacterID: noImage.ID, ProviderAccountID: 4, ProviderAssetID: "audio-extra", AssetType: "Audio", Status: "Active", StagingFileID: "file-audio"}).Error)
	require.NoError(t, DB.Create(&legacyVirtualCharacterAsset{CharacterID: processingCharacter.ID, ProviderAccountID: 4, ProviderAssetID: "image-processing", AssetType: "Image", Status: "Processing", NextPollAt: 123}).Error)
	require.NoError(t, DB.Create(&legacyVirtualCharacterAsset{CharacterID: failedCharacter.ID, ProviderAccountID: 4, ProviderAssetID: "image-failed", AssetType: "Image", Status: "Failed", LastError: "provider rejected"}).Error)
	require.NoError(t, DB.Model(&legacyVirtualCharacterPrimary{}).Where("id = ?", character.ID).Update("primary_asset_id", video.ID).Error)

	stats, err := GetVirtualCharacterCollapsePreflightStats()
	require.NoError(t, err)
	require.EqualValues(t, 6, stats.LegacyAssets)
	require.EqualValues(t, 4, stats.CharactersWithLegacyAssets)
	require.EqualValues(t, 2, stats.MinimumExtraAssets)
	require.EqualValues(t, 1, stats.SoftDeletedAssets)
	require.NoError(t, MigrateVirtualCharacterCollapseAssets())
	require.False(t, DB.Migrator().HasTable(&legacyVirtualCharacterAsset{}))
	var migratedPreset VirtualCharacter
	require.NoError(t, DB.First(&migratedPreset, publicPreset.ID).Error)
	require.Equal(t, "catalog-asset", migratedPreset.ProviderAssetID)
	require.Equal(t, VirtualCharacterStatusOffline, migratedPreset.Status)
	var migratedNoImage VirtualCharacter
	require.NoError(t, DB.First(&migratedNoImage, noImage.ID).Error)
	require.Equal(t, VirtualCharacterStatusFailed, migratedNoImage.Status)
	require.Nil(t, migratedNoImage.Slot)
	require.Contains(t, migratedNoImage.LastError, "no usable image")
	var migratedProcessing VirtualCharacter
	require.NoError(t, DB.First(&migratedProcessing, processingCharacter.ID).Error)
	require.Equal(t, VirtualCharacterStatusCreating, migratedProcessing.Status)
	require.EqualValues(t, 123, migratedProcessing.AssetNextPollAt)
	var migratedFailed VirtualCharacter
	require.NoError(t, DB.First(&migratedFailed, failedCharacter.ID).Error)
	require.Equal(t, VirtualCharacterStatusFailed, migratedFailed.Status)
	require.Nil(t, migratedFailed.Slot)
	require.Equal(t, "provider rejected", migratedFailed.LastError)
	var failedJobs []VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("character_id = ?", failedCharacter.ID).Find(&failedJobs).Error)
	failedTargets := make(map[string]bool, len(failedJobs))
	for _, job := range failedJobs {
		failedTargets[job.TargetType+":"+job.TargetID] = true
	}
	require.True(t, failedTargets["volc_asset:image-failed"])
	require.True(t, failedTargets["volc_group:group-failed"])
	var refreshed VirtualCharacter
	require.NoError(t, DB.First(&refreshed, character.ID).Error)
	require.Equal(t, "image-keep", refreshed.ProviderAssetID)
	require.Equal(t, "file-keep", refreshed.StagingFileID)
	require.Equal(t, "image/png", refreshed.MimeType)
	require.EqualValues(t, 123, refreshed.FileSize)
	require.Equal(t, VirtualCharacterStatusActive, refreshed.Status)
	require.NotNil(t, refreshed.Slot)

	var jobs []VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("character_id = ?", character.ID).Order("id ASC").Find(&jobs).Error)
	targets := make(map[string]bool, len(jobs))
	for _, job := range jobs {
		targets[job.TargetType+":"+job.TargetID] = true
	}
	require.True(t, targets["volc_asset:video-primary"])
	require.True(t, targets["aipdd_file:file-video"])
	require.True(t, targets["volc_asset:image-deleted"])
	require.True(t, targets["aipdd_file:file-deleted"])
	require.False(t, targets["volc_asset:image-keep"])
	var noImageJobs []VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("character_id = ?", noImage.ID).Find(&noImageJobs).Error)
	noImageTargets := make(map[string]bool, len(noImageJobs))
	for _, job := range noImageJobs {
		noImageTargets[job.TargetType+":"+job.TargetID] = true
	}
	require.True(t, noImageTargets["volc_asset:audio-extra"])
	require.True(t, noImageTargets["aipdd_file:file-audio"])
	require.True(t, noImageTargets["volc_group:group-audio-only"])

	// A repeated startup is a no-op after the old table has been removed.
	require.NoError(t, MigrateVirtualCharacterCollapseAssets())
}

func TestSelectLegacyVirtualCharacterAssetHandlesDanglingAndFailedPrimary(t *testing.T) {
	failedID := int64(2)
	danglingID := int64(99)
	assets := []legacyVirtualCharacterAsset{
		{ID: 1, ProviderAssetID: "processing", AssetType: "Image", Status: "Processing"},
		{ID: failedID, ProviderAssetID: "failed", AssetType: "Image", Status: "Failed"},
		{ID: 3, ProviderAssetID: "unknown", AssetType: "Image", Status: "Unknown"},
	}
	require.Equal(t, int64(1), selectLegacyVirtualCharacterAsset(assets, &danglingID).ID)
	require.Equal(t, failedID, selectLegacyVirtualCharacterAsset(assets, &failedID).ID)
}

func TestBeginVirtualCharacterGroupDeleteQueuesImageFileBeforeGroup(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	slot := 1
	character := &VirtualCharacter{
		UserID: 88, Slot: &slot, Scope: VirtualCharacterScopePrivate,
		SourceType: VirtualCharacterSourceVolcAIGC, Status: VirtualCharacterStatusActive,
		ProviderAccountID: 5, ProviderGroupID: "group-delete", ProviderAssetID: "image-delete",
		AIPDDAssetID: 701, AIPDDChannelID: 9, StagingFileID: "file-delete",
	}
	require.NoError(t, DB.Create(character).Error)
	require.NoError(t, BeginVirtualCharacterGroupDelete(character.ID, character.UserID))

	var refreshed VirtualCharacter
	require.NoError(t, DB.First(&refreshed, character.ID).Error)
	require.Equal(t, VirtualCharacterStatusDeleting, refreshed.Status)
	require.Nil(t, refreshed.Slot)
	var jobs []VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("character_id = ?", character.ID).Order("id ASC").Find(&jobs).Error)
	require.Len(t, jobs, 4)
	require.Equal(t, "volc_asset", jobs[0].TargetType)
	require.Equal(t, "aipdd_asset", jobs[1].TargetType)
	require.Equal(t, "701", jobs[1].TargetID)
	require.Equal(t, 9, jobs[1].AIPDDChannelID)
	require.Equal(t, "aipdd_file", jobs[2].TargetType)
	require.Equal(t, 9, jobs[2].AIPDDChannelID)
	require.Equal(t, "volc_group", jobs[3].TargetType)
	pending, err := HasIncompleteVirtualCharacterCleanupJobs(character.ID, jobs[3].ID)
	require.NoError(t, err)
	require.True(t, pending)
	require.NoError(t, CompleteVirtualCharacterCleanupJob(jobs[0].ID))
	require.NoError(t, CompleteVirtualCharacterCleanupJob(jobs[1].ID))
	require.NoError(t, CompleteVirtualCharacterCleanupJob(jobs[2].ID))
	pending, err = HasIncompleteVirtualCharacterCleanupJobs(character.ID, jobs[3].ID)
	require.NoError(t, err)
	require.False(t, pending)
}

func TestDiscardFailedAIGCVirtualCharacterUploadHidesEmptyCharacterAndQueuesCleanup(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	slot := 1
	character := &VirtualCharacter{
		UserID: 90, Slot: &slot, Scope: VirtualCharacterScopePrivate,
		SourceType: VirtualCharacterSourceVolcAIGC, Status: VirtualCharacterStatusCreating,
		ProviderAccountID: 5, ProviderGroupID: "group-failed", ProviderAssetID: "asset-failed",
		AIPDDAssetID: 702, AIPDDChannelID: 10, StagingFileID: "file-failed",
	}
	require.NoError(t, DB.Create(character).Error)
	require.NoError(t, DiscardFailedAIGCVirtualCharacterUpload(character.ID, "", "素材上传失败"))

	var hidden VirtualCharacter
	require.ErrorIs(t, DB.First(&hidden, character.ID).Error, gorm.ErrRecordNotFound)
	require.NoError(t, DB.Unscoped().First(&hidden, character.ID).Error)
	require.Equal(t, VirtualCharacterStatusDeleting, hidden.Status)
	require.Nil(t, hidden.Slot)
	require.True(t, hidden.DeletedAt.Valid)
	require.Equal(t, "素材上传失败", hidden.LastError)

	var jobs []VirtualCharacterCleanupJob
	require.NoError(t, DB.Where("character_id = ?", character.ID).Find(&jobs).Error)
	require.Len(t, jobs, 4)
	targets := make(map[string]string, len(jobs))
	for _, job := range jobs {
		targets[job.TargetType] = job.TargetID
	}
	require.Equal(t, "asset-failed", targets["volc_asset"])
	require.Equal(t, "702", targets["aipdd_asset"])
	require.Equal(t, "file-failed", targets["aipdd_file"])
	require.Equal(t, "group-failed", targets["volc_group"])

	noTargetSlot := 2
	noTarget := &VirtualCharacter{
		UserID: 90, Slot: &noTargetSlot, Scope: VirtualCharacterScopePrivate,
		SourceType: VirtualCharacterSourceVolcAIGC, Status: VirtualCharacterStatusCreating,
		ProviderAccountID: 5,
	}
	require.NoError(t, DB.Create(noTarget).Error)
	require.NoError(t, DiscardFailedAIGCVirtualCharacterUpload(noTarget.ID, "", "素材组创建失败"))
	require.ErrorIs(t, DB.Unscoped().First(&VirtualCharacter{}, noTarget.ID).Error, gorm.ErrRecordNotFound)
}

func TestVirtualCharacterImageTerminalTransitionsReleaseFailedSlot(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	activeSlot := 1
	active := &VirtualCharacter{UserID: 89, Slot: &activeSlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Status: VirtualCharacterStatusCreating, ProviderAssetID: "image-active"}
	require.NoError(t, DB.Create(active).Error)
	require.NoError(t, MarkVirtualCharacterImageTerminal(active.ID, true, "ignored"))
	require.NoError(t, DB.First(active, active.ID).Error)
	require.Equal(t, VirtualCharacterStatusActive, active.Status)
	require.NotNil(t, active.Slot)
	require.Empty(t, active.LastError)

	failedSlot := 2
	failed := &VirtualCharacter{UserID: 89, Slot: &failedSlot, Scope: VirtualCharacterScopePrivate, SourceType: VirtualCharacterSourceVolcAIGC, Status: VirtualCharacterStatusCreating, ProviderAssetID: "image-failed"}
	require.NoError(t, DB.Create(failed).Error)
	require.NoError(t, MarkVirtualCharacterImageTerminal(failed.ID, false, "provider rejected"))
	require.NoError(t, DB.First(failed, failed.ID).Error)
	require.Equal(t, VirtualCharacterStatusFailed, failed.Status)
	require.Nil(t, failed.Slot)
	require.Equal(t, "provider rejected", failed.LastError)
}
