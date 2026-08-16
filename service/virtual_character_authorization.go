package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const realPersonProviderStateTTL = 2 * time.Minute

type VirtualCharacterAuthorizationError struct {
	Status  int
	Code    string
	Message string
}

func (e *VirtualCharacterAuthorizationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type VirtualCharacterAuthorizationSnapshot struct {
	CharacterID           int64    `json:"character_id"`
	UserID                int      `json:"user_id"`
	SourceType            string   `json:"source_type"`
	ProviderAccountID     int      `json:"provider_account_id"`
	ProviderGroupID       string   `json:"provider_group_id,omitempty"`
	ProviderAssetID       string   `json:"provider_asset_id"`
	AuthorizationStatus   string   `json:"authorization_status,omitempty"`
	AuthorizationFrom     int64    `json:"authorization_from,omitempty"`
	AuthorizationUntil    int64    `json:"authorization_until,omitempty"`
	CommercialUseAllowed  bool     `json:"commercial_use_allowed,omitempty"`
	Purposes              []string `json:"purposes,omitempty"`
	Regions               []string `json:"regions,omitempty"`
	Platforms             []string `json:"platforms,omitempty"`
	Industries            []string `json:"industries,omitempty"`
	AgreementVersion      string   `json:"agreement_version,omitempty"`
	AgreementReference    string   `json:"agreement_reference,omitempty"`
	ConsentReceiptHash    string   `json:"consent_receipt_hash,omitempty"`
	HolderScopeAcceptedAt int64    `json:"holder_scope_accepted_at,omitempty"`
	ProviderGroupStatus   string   `json:"provider_group_status,omitempty"`
	ProviderAssetStatus   string   `json:"provider_asset_status,omitempty"`
	ProviderCheckedAt     int64    `json:"provider_checked_at,omitempty"`
	AuthorizedAt          int64    `json:"authorized_at,omitempty"`
	CapturedAt            int64    `json:"captured_at"`
}

func SyncRealPersonVirtualCharacter(ctx context.Context, characterID int64) (*model.VirtualCharacter, error) {
	character, err := model.GetVirtualCharacterByID(characterID)
	if err != nil {
		return nil, err
	}
	if character.SourceType != model.VirtualCharacterSourceVolcRealPerson || strings.TrimSpace(character.ProviderGroupID) == "" {
		return nil, errors.New("real-person character is not ready for provider synchronization")
	}
	authorization, err := model.GetVirtualCharacterAuthorization(character.ID)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	if authorization.ValidUntil <= now {
		_ = model.ExpireRealPersonAuthorization(character.ID, "authorization expired")
		return nil, errors.New("real-person authorization expired")
	}
	account, err := providerAccountForCharacter(character)
	if err != nil {
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, err.Error())
		return nil, err
	}
	client, err := NewVolcAssetClient(account)
	if err != nil {
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, common.MaskSensitiveInfo(err.Error()))
		return nil, err
	}
	group, err := client.GetAssetGroup(ctx, character.ProviderGroupID, account.ProjectName)
	if err != nil {
		reason := common.MaskSensitiveInfo(err.Error())
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, reason)
		return nil, err
	}
	if err := validateRealPersonGroup(character, account, group); err != nil {
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, err.Error())
		return nil, err
	}
	assets, err := client.ListAssetsByGroupType(ctx, character.ProviderGroupID, model.VirtualCharacterRealPersonGroupType, account.ProjectName)
	if err != nil {
		reason := common.MaskSensitiveInfo(err.Error())
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, reason)
		return nil, err
	}
	activeImages := make([]VolcAssetResult, 0, len(assets))
	processingStatus := ""
	inactiveStatuses := make([]string, 0, len(assets))
	imageCount := 0
	for i := range assets {
		asset := assets[i]
		if !strings.EqualFold(strings.TrimSpace(asset.GroupID), strings.TrimSpace(character.ProviderGroupID)) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(asset.AssetType), "Image") {
			continue
		}
		imageCount++
		switch strings.ToLower(strings.TrimSpace(asset.Status)) {
		case "active":
			activeImages = append(activeImages, asset)
		case "processing", "creating", "pending":
			processingStatus = asset.Status
		default:
			status := strings.TrimSpace(asset.Status)
			if status == "" {
				status = "Unknown"
			}
			inactiveStatuses = append(inactiveStatuses, status)
		}
	}
	checkedAt := time.Now().Unix()
	switch len(activeImages) {
	case 0:
		if processingStatus == "" && imageCount > 0 {
			reason := "verified real-person group has no usable active image"
			if len(inactiveStatuses) > 0 {
				reason += "; provider statuses: " + strings.Join(inactiveStatuses, ", ")
			}
			_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationFailed, reason)
			return nil, errors.New(reason)
		}
		if processingStatus == "" {
			processingStatus = "Pending"
		}
		_ = model.UpdateRealPersonProviderState(character.ID, providerGroupStatus(group), processingStatus, model.VirtualCharacterAuthorizationSynchronizing, "waiting for one active real-person image", checkedAt)
		attempts := character.AssetPollAttempts + 1
		_ = model.RetryVirtualCharacterImagePoll(character.ID, attempts, time.Now().Add(virtualCharacterRetryDelay(attempts)).Unix(), "waiting for one active real-person image")
		return model.GetVirtualCharacterByID(character.ID)
	case 1:
		asset := activeImages[0]
		if err := model.ActivateRealPersonVirtualCharacter(character.ID, asset.ID, asset.Status, checkedAt); err != nil {
			return nil, err
		}
		return model.GetVirtualCharacterByID(character.ID)
	default:
		reason := fmt.Sprintf("provider group contains %d active images; exactly one is required", len(activeImages))
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationAmbiguous, reason)
		return nil, errors.New(reason)
	}
}

func AuthorizeVirtualCharacterForVideo(ctx context.Context, character *model.VirtualCharacter, userID int) (string, *VirtualCharacterAuthorizationError) {
	if character == nil || character.ID <= 0 {
		return "", authorizationError(http.StatusNotFound, "character_not_found", "character not found")
	}
	if character.Scope == model.VirtualCharacterScopePrivate && character.UserID != userID {
		return "", authorizationError(http.StatusForbidden, "character_forbidden", "character does not belong to the current user")
	}
	if character.Scope == model.VirtualCharacterScopePublic && character.Status != model.VirtualCharacterStatusActive {
		return "", authorizationError(http.StatusConflict, "character_unavailable", "character is not active")
	}
	if character.Scope == model.VirtualCharacterScopePrivate && character.Status != model.VirtualCharacterStatusActive {
		return "", authorizationError(http.StatusConflict, "character_unavailable", "character is not available for new tasks")
	}
	if strings.TrimSpace(character.ProviderAssetID) == "" {
		return "", authorizationError(http.StatusConflict, "character_unavailable", "character asset is not active")
	}
	account, err := providerAccountForCharacter(character)
	if err != nil {
		return "", authorizationError(http.StatusServiceUnavailable, "character_provider_unavailable", err.Error())
	}
	if character.SourceType != model.VirtualCharacterSourceVolcRealPerson {
		return marshalAuthorizationSnapshot(character, nil)
	}
	if !account.RealPersonEnabled {
		return "", authorizationError(http.StatusServiceUnavailable, "real_person_disabled", "real-person character support is disabled")
	}
	authorization, err := model.GetVirtualCharacterAuthorization(character.ID)
	if err != nil {
		return "", authorizationError(http.StatusConflict, "authorization_missing", "real-person authorization record is missing")
	}
	now := time.Now().Unix()
	if authorization.ValidFrom > now {
		return "", authorizationError(http.StatusConflict, "authorization_not_started", "real-person authorization is not active yet")
	}
	if authorization.ValidUntil <= now {
		_ = model.ExpireRealPersonAuthorization(character.ID, "authorization expired")
		return "", authorizationError(http.StatusConflict, "authorization_expired", "real-person authorization has expired")
	}
	if authorization.Status != model.VirtualCharacterAuthorizationActive {
		return "", authorizationError(http.StatusConflict, "authorization_inactive", "real-person authorization is not active")
	}
	if authorization.HolderScopeAcceptedAt <= 0 || strings.TrimSpace(authorization.ConsentReceiptHash) == "" {
		return "", authorizationError(http.StatusConflict, "authorization_evidence_incomplete", "real-person authorization evidence is incomplete")
	}
	if authorization.ProviderCheckedAt <= time.Now().Add(-realPersonProviderStateTTL).Unix() {
		if _, err := RefreshRealPersonVirtualCharacterState(ctx, character, account); err != nil {
			return "", authorizationError(http.StatusServiceUnavailable, "provider_state_unavailable", common.MaskSensitiveInfo(err.Error()))
		}
		authorization, err = model.GetVirtualCharacterAuthorization(character.ID)
		if err != nil || authorization.Status != model.VirtualCharacterAuthorizationActive {
			return "", authorizationError(http.StatusConflict, "authorization_inactive", "real-person authorization is not active")
		}
	}
	return marshalAuthorizationSnapshot(character, authorization)
}

func RefreshRealPersonVirtualCharacterState(ctx context.Context, character *model.VirtualCharacter, account *model.VirtualCharacterProviderAccount) (*model.VirtualCharacterAuthorization, error) {
	if character == nil || account == nil || character.SourceType != model.VirtualCharacterSourceVolcRealPerson {
		return nil, errors.New("invalid real-person provider state request")
	}
	client, err := NewVolcAssetClient(account)
	if err != nil {
		return nil, err
	}
	group, err := client.GetAssetGroup(ctx, character.ProviderGroupID, account.ProjectName)
	if err != nil {
		reason := common.MaskSensitiveInfo(err.Error())
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, reason)
		return nil, err
	}
	if err := validateRealPersonGroup(character, account, group); err != nil {
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, err.Error())
		return nil, err
	}
	asset, err := client.GetAsset(ctx, character.ProviderAssetID, account.ProjectName)
	if err != nil {
		reason := common.MaskSensitiveInfo(err.Error())
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, reason)
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(asset.ID), strings.TrimSpace(character.ProviderAssetID)) ||
		!strings.EqualFold(strings.TrimSpace(asset.GroupID), strings.TrimSpace(character.ProviderGroupID)) ||
		!strings.EqualFold(strings.TrimSpace(asset.AssetType), "Image") ||
		!strings.EqualFold(strings.TrimSpace(asset.Status), "Active") {
		reason := "real-person provider asset is no longer active or no longer belongs to its verified group"
		_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, reason)
		return nil, errors.New(reason)
	}
	checkedAt := time.Now().Unix()
	if err := model.ActivateRealPersonVirtualCharacter(character.ID, asset.ID, asset.Status, checkedAt); err != nil {
		return nil, err
	}
	return model.GetVirtualCharacterAuthorization(character.ID)
}

func GetRealPersonVirtualCharacterPreviewAsset(ctx context.Context, character *model.VirtualCharacter) (*VolcAssetResult, error) {
	account, err := providerAccountForCharacter(character)
	if err != nil {
		return nil, err
	}
	if _, err := RefreshRealPersonVirtualCharacterState(ctx, character, account); err != nil {
		return nil, err
	}
	client, err := NewVolcAssetClient(account)
	if err != nil {
		return nil, err
	}
	asset, err := client.GetAsset(ctx, character.ProviderAssetID, account.ProjectName)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(asset.Status, "Active") || strings.TrimSpace(asset.URL) == "" {
		return nil, errors.New("real-person preview asset is unavailable")
	}
	return asset, nil
}

func providerAccountForCharacter(character *model.VirtualCharacter) (*model.VirtualCharacterProviderAccount, error) {
	if character == nil || !common.HasStableCryptoSecret() {
		return nil, errors.New("virtual character provider is unavailable")
	}
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	if err != nil || account.ID != character.ProviderAccountID {
		return nil, errors.New("virtual character provider account is unavailable")
	}
	return account, nil
}

func validateRealPersonGroup(character *model.VirtualCharacter, account *model.VirtualCharacterProviderAccount, group *VolcAssetGroupResult) error {
	if group == nil || strings.TrimSpace(group.ID) == "" || !strings.EqualFold(strings.TrimSpace(group.ID), strings.TrimSpace(character.ProviderGroupID)) {
		return errors.New("verified real-person group was not found")
	}
	if !strings.EqualFold(strings.TrimSpace(group.GroupType), model.VirtualCharacterRealPersonGroupType) {
		return fmt.Errorf("provider group type %q is not %s", group.GroupType, model.VirtualCharacterRealPersonGroupType)
	}
	if group.ProjectName != "" && !strings.EqualFold(strings.TrimSpace(group.ProjectName), strings.TrimSpace(account.ProjectName)) {
		return errors.New("provider group belongs to a different project")
	}
	if !strings.EqualFold(strings.TrimSpace(group.Status), "Active") {
		return fmt.Errorf("provider group is not active: %s", group.Status)
	}
	return nil
}

func providerGroupStatus(group *VolcAssetGroupResult) string {
	if group == nil || strings.TrimSpace(group.Status) == "" {
		return "Active"
	}
	return strings.TrimSpace(group.Status)
}

func marshalAuthorizationSnapshot(character *model.VirtualCharacter, authorization *model.VirtualCharacterAuthorization) (string, *VirtualCharacterAuthorizationError) {
	snapshot := VirtualCharacterAuthorizationSnapshot{
		CharacterID: character.ID, UserID: character.UserID, SourceType: character.SourceType,
		ProviderAccountID: character.ProviderAccountID, ProviderGroupID: character.ProviderGroupID,
		ProviderAssetID: character.ProviderAssetID, CapturedAt: time.Now().Unix(),
	}
	if authorization != nil {
		snapshot.AuthorizationStatus = authorization.Status
		snapshot.AuthorizationFrom = authorization.ValidFrom
		snapshot.AuthorizationUntil = authorization.ValidUntil
		snapshot.CommercialUseAllowed = authorization.CommercialUseAllowed
		snapshot.Purposes = decodeAuthorizationScope(authorization.PurposesJSON)
		snapshot.Regions = decodeAuthorizationScope(authorization.RegionsJSON)
		snapshot.Platforms = decodeAuthorizationScope(authorization.PlatformsJSON)
		snapshot.Industries = decodeAuthorizationScope(authorization.IndustriesJSON)
		snapshot.AgreementVersion = authorization.AgreementVersion
		snapshot.AgreementReference = authorization.AgreementReference
		snapshot.ConsentReceiptHash = authorization.ConsentReceiptHash
		snapshot.HolderScopeAcceptedAt = authorization.HolderScopeAcceptedAt
		snapshot.ProviderGroupStatus = authorization.ProviderGroupStatus
		snapshot.ProviderAssetStatus = authorization.ProviderAssetStatus
		snapshot.ProviderCheckedAt = authorization.ProviderCheckedAt
		snapshot.AuthorizedAt = authorization.AuthorizedAt
	}
	payload, err := common.Marshal(snapshot)
	if err != nil {
		return "", authorizationError(http.StatusInternalServerError, "authorization_snapshot_failed", err.Error())
	}
	return string(payload), nil
}

func decodeAuthorizationScope(raw string) []string {
	var values []string
	if err := common.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return nil
	}
	return values
}

func authorizationError(status int, code, message string) *VirtualCharacterAuthorizationError {
	return &VirtualCharacterAuthorizationError{Status: status, Code: code, Message: message}
}

func IsVirtualCharacterNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
