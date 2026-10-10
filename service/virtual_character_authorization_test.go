package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAuthorizeRealPersonCharacterChecksOwnerAndWindow(t *testing.T) {
	previousDB := model.DB
	previousSecret := common.CryptoSecret
	previousConfigured := common.CryptoSecretConfigured
	db, err := gorm.Open(sqlite.Open("file:real_person_authorization?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.VirtualCharacter{}, &model.VirtualCharacterAuthorization{}, &model.VirtualCharacterProviderAccount{},
	))
	model.DB = db
	common.CryptoSecret = strings.Repeat("a", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	account := &model.VirtualCharacterProviderAccount{
		ID: 1, Enabled: true, RealPersonEnabled: true, ChannelID: 54,
		Region: "cn-beijing", ProjectName: "default",
	}
	require.NoError(t, db.Create(account).Error)
	slot := 1
	character := &model.VirtualCharacter{
		UserID: 501, RealPersonSlot: &slot, Scope: model.VirtualCharacterScopePrivate,
		SourceType: model.VirtualCharacterSourceVolcRealPerson, Name: "Actor",
		Status: model.VirtualCharacterStatusActive, ValidationStatus: model.VirtualCharacterValidationAccepted,
		ProviderAccountID: account.ID, ProviderGroupID: "group-real", ProviderAssetID: "asset-real",
	}
	require.NoError(t, db.Create(character).Error)
	now := time.Now().Unix()
	authorization := &model.VirtualCharacterAuthorization{
		CharacterID: character.ID, UserID: character.UserID, Status: model.VirtualCharacterAuthorizationActive,
		ValidFrom: now - 60, ValidUntil: now + 3600, CommercialUseAllowed: true,
		AgreementVersion: "volc-real-person-v1", ProviderGroupType: model.VirtualCharacterRealPersonGroupType,
		AgreementReference: "session-1", ConsentReceiptHash: strings.Repeat("a", 64), HolderScopeAcceptedAt: now - 120,
		ProviderGroupStatus: "Active", ProviderAssetStatus: "Active", ProviderCheckedAt: now,
	}
	require.NoError(t, db.Create(authorization).Error)

	snapshotJSON, authErr := AuthorizeVirtualCharacterForVideo(context.Background(), character, character.UserID)
	require.Nil(t, authErr)
	var snapshot VirtualCharacterAuthorizationSnapshot
	require.NoError(t, common.Unmarshal([]byte(snapshotJSON), &snapshot))
	require.Equal(t, character.ID, snapshot.CharacterID)
	require.Equal(t, authorization.AgreementReference, snapshot.AgreementReference)
	require.Equal(t, authorization.ConsentReceiptHash, snapshot.ConsentReceiptHash)

	_, authErr = AuthorizeVirtualCharacterForVideo(context.Background(), character, 999)
	require.NotNil(t, authErr)
	require.Equal(t, "character_forbidden", authErr.Code)

	require.NoError(t, db.Model(authorization).Updates(map[string]any{
		"valid_until": now - 1, "provider_checked_at": now,
	}).Error)
	_, authErr = AuthorizeVirtualCharacterForVideo(context.Background(), character, character.UserID)
	require.NotNil(t, authErr)
	require.Equal(t, "authorization_expired", authErr.Code)
	var expired model.VirtualCharacter
	require.NoError(t, db.First(&expired, character.ID).Error)
	require.Equal(t, model.VirtualCharacterStatusDeleting, expired.Status)
	require.Nil(t, expired.RealPersonSlot)
}

func TestValidateRealPersonGroupAcceptsOfficialResponseWithoutStatus(t *testing.T) {
	character := &model.VirtualCharacter{ProviderGroupID: "group-real"}
	account := &model.VirtualCharacterProviderAccount{ProjectName: "default"}
	group := &VolcAssetGroupResult{
		ID: "group-real", GroupType: model.VirtualCharacterRealPersonGroupType,
		ProjectName: "default",
	}

	require.NoError(t, validateRealPersonGroup(character, account, group))
	require.Equal(t, "Active", providerGroupStatus(group))
}

func TestProviderGroupProcessingStatusIsRetryable(t *testing.T) {
	require.False(t, isTerminalProviderGroupStatus("Processing"))
	require.False(t, isTerminalProviderGroupStatus("Creating"))
	require.True(t, isTerminalProviderGroupStatus("Failed"))
}
