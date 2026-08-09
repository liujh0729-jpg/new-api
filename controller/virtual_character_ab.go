package controller

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	virtualCharacterValidationTTL = 30 * time.Minute
	virtualCharacterUploadMaxBody = int64(51 << 20)
)

type virtualCharacterAssetResponse struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	AssetType       string `json:"asset_type"`
	Status          string `json:"status"`
	IsPrimary       bool   `json:"is_primary"`
	CoverURL        string `json:"cover_url,omitempty"`
	MimeType        string `json:"mime_type,omitempty"`
	FileSize        int64  `json:"file_size,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	ProviderAssetID string `json:"provider_asset_id,omitempty"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

type virtualCharacterGroupResponse struct {
	ID               int64                           `json:"id"`
	Scope            string                          `json:"scope"`
	SourceType       string                          `json:"source_type"`
	Name             string                          `json:"name"`
	Description      string                          `json:"description"`
	Tags             []string                        `json:"tags"`
	Status           string                          `json:"status"`
	ValidationStatus string                          `json:"validation_status"`
	CoverURL         string                          `json:"cover_url,omitempty"`
	PrimaryAssetID   *int64                          `json:"primary_asset_id,omitempty"`
	Assets           []virtualCharacterAssetResponse `json:"assets"`
	LastError        string                          `json:"last_error,omitempty"`
	CreatedAt        int64                           `json:"created_at"`
	UpdatedAt        int64                           `json:"updated_at"`
	CatalogVersion   string                          `json:"catalog_version,omitempty"`
}

func ListVirtualCharacterGroups(c *gin.Context) {
	userID := c.GetInt("id")
	scope := strings.TrimSpace(c.DefaultQuery("scope", model.VirtualCharacterScopePublic))
	if scope != model.VirtualCharacterScopePublic && scope != model.VirtualCharacterScopePrivate {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_scope", "scope must be public or private")
		return
	}
	page := getVirtualCharacterPage(c)
	items, total, err := model.ListVirtualCharacters(userID, scope, false, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	responses := make([]virtualCharacterGroupResponse, 0, len(items))
	for i := range items {
		response, responseErr := virtualCharacterGroupToResponse(&items[i], scope == model.VirtualCharacterScopePrivate)
		if responseErr != nil {
			virtualCharacterError(c, http.StatusInternalServerError, "list_failed", responseErr.Error())
			return
		}
		responses = append(responses, response)
	}
	page.SetTotal(int(total))
	page.SetItems(responses)
	data := gin.H{"page": page}
	if scope == model.VirtualCharacterScopePrivate {
		used, _ := model.CountActivePrivateVirtualCharacters(userID)
		data["used"] = used
		data["limit"] = model.GetVirtualCharacterEffectiveLimit(userID)
	}
	common.ApiSuccess(c, data)
}

func GetVirtualCharacterGroup(c *gin.Context) {
	id, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	item, err := model.GetAccessibleVirtualCharacter(id, c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	response, err := virtualCharacterGroupToResponse(item, true)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "detail_failed", err.Error())
		return
	}
	common.ApiSuccess(c, response)
}

func AdminListVirtualCharacterGroups(c *gin.Context) {
	page := getVirtualCharacterPage(c)
	items, total, err := model.ListVirtualCharacters(0, model.VirtualCharacterScopePublic, true, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	responses := make([]virtualCharacterGroupResponse, 0, len(items))
	for i := range items {
		response, responseErr := virtualCharacterGroupToResponse(&items[i], true)
		if responseErr != nil {
			virtualCharacterError(c, http.StatusInternalServerError, "list_failed", responseErr.Error())
			return
		}
		responses = append(responses, response)
	}
	page.SetTotal(int(total))
	page.SetItems(responses)
	common.ApiSuccess(c, page)
}

func GetVirtualCharacterABConfig(c *gin.Context) {
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	officialEnabled, realPersonEnabled := false, false
	if err == nil && common.HasStableCryptoSecret() {
		officialEnabled, realPersonEnabled = account.OfficialEnabled, account.RealPersonEnabled
	}
	common.ApiSuccess(c, gin.H{"models": model.GetVirtualCharacterModels(), "default_model": model.GetVirtualCharacterDefaultModel(), "image_max_mb": 30, "video_max_mb": 50, "audio_max_mb": 15, "task_retention_days": 90, "official_enabled": officialEnabled, "real_person_enabled": realPersonEnabled})
}

func CreateVirtualCharacterValidationSession(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		virtualCharacterError(c, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	account, client, err := enabledVirtualCharacterClient(true)
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "real_person_disabled", err.Error())
		return
	}
	used, countErr := model.CountActivePrivateVirtualCharacters(userID)
	if countErr != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "count_failed", countErr.Error())
		return
	}
	if used >= int64(model.GetVirtualCharacterEffectiveLimit(userID)) {
		virtualCharacterError(c, http.StatusConflict, "character_limit_reached", "virtual character limit reached")
		return
	}
	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Language    string   `json:"language"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	metadata, tagsJSON, err := normalizeVirtualCharacterMetadata(virtualCharacterMetadataRequest{Name: req.Name, Description: req.Description, Tags: req.Tags})
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_metadata", err.Error())
		return
	}
	language := strings.TrimSpace(req.Language)
	if language == "" {
		language = "zh"
	}
	if language != "zh" && language != "en" {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_language", "language must be zh or en")
		return
	}
	sessionID, err := randomHex(16)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "session_failed", "failed to generate validation session")
		return
	}
	state, err := randomHex(32)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "session_failed", "failed to generate validation state")
		return
	}
	base := strings.TrimRight(strings.TrimSpace(service.GetCallbackAddress()), "/")
	if base == "" {
		virtualCharacterError(c, http.StatusServiceUnavailable, "callback_not_configured", "public callback address is not configured")
		return
	}
	callbackURL := base + "/api/virtual-characters/validation/callback?state=" + url.QueryEscape(state)
	providerSession, err := client.CreateVisualValidateSession(c.Request.Context(), callbackURL, account.ProjectName, language)
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "provider_session_failed", common.MaskSensitiveInfo(err.Error()))
		return
	}
	encryptedToken, err := common.EncryptSensitiveValue(providerSession.BytedToken)
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "crypto_not_configured", err.Error())
		return
	}
	encryptedLink, err := common.EncryptSensitiveValue(providerSession.H5Link)
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "crypto_not_configured", err.Error())
		return
	}
	now := time.Now()
	item := &model.VirtualCharacterValidationSession{ID: sessionID, UserID: userID, ProviderAccountID: account.ID, Status: model.VirtualCharacterValidationPending, StateHash: hashValidationState(state), EncryptedBytedToken: encryptedToken, EncryptedH5Link: encryptedLink, Name: metadata.Name, Description: metadata.Description, TagsJSON: tagsJSON, Language: language, ExpiresAt: now.Add(virtualCharacterValidationTTL).Unix()}
	if err := model.CreateVirtualCharacterValidationSession(item); err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "session_failed", err.Error())
		return
	}
	common.ApiSuccess(c, validationSessionResponse(item, base+"/api/virtual-characters/validation/launch/"+url.PathEscape(sessionID)+"?state="+url.QueryEscape(state)))
}

func GetVirtualCharacterValidationSession(c *gin.Context) {
	item, err := model.GetOwnedVirtualCharacterValidationSession(c.Param("id"), c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	common.ApiSuccess(c, validationSessionResponse(item, ""))
}

func LaunchVirtualCharacterValidation(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	item, err := model.GetVirtualCharacterValidationSessionByStateHash(hashValidationState(state))
	if err != nil || item.ID != c.Param("id") || item.Status != model.VirtualCharacterValidationPending || item.ExpiresAt <= time.Now().Unix() {
		c.AbortWithStatus(http.StatusGone)
		return
	}
	link, err := common.DecryptSensitiveValue(item.EncryptedH5Link)
	if err != nil {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	c.Redirect(http.StatusFound, parsed.String())
}

func VirtualCharacterValidationCallback(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	item, err := model.GetVirtualCharacterValidationSessionByStateHash(hashValidationState(state))
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if item.Status != model.VirtualCharacterValidationPending {
		redirectValidationResult(c, item)
		return
	}
	if item.ExpiresAt <= time.Now().Unix() {
		_ = model.MarkVirtualCharacterValidationExpired(item.ID)
		item.Status = model.VirtualCharacterValidationExpired
		redirectValidationResult(c, item)
		return
	}
	storedToken, err := common.DecryptSensitiveValue(item.EncryptedBytedToken)
	callbackToken := strings.TrimSpace(c.Query("bytedToken"))
	if err != nil || subtle.ConstantTimeCompare([]byte(storedToken), []byte(callbackToken)) != 1 {
		_ = model.MarkVirtualCharacterValidationFailed(item.ID, "token_mismatch", "validation token mismatch")
		item.Status = model.VirtualCharacterValidationFailed
		redirectValidationResult(c, item)
		return
	}
	resultCode := strings.TrimSpace(c.Query("resultCode"))
	if resultCode != "10000" {
		_ = model.MarkVirtualCharacterValidationFailed(item.ID, resultCode, "visual validation failed")
		item.Status = model.VirtualCharacterValidationFailed
		redirectValidationResult(c, item)
		return
	}
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	if err != nil || account.ID != item.ProviderAccountID {
		_ = model.MarkVirtualCharacterValidationFailed(item.ID, resultCode, "provider account is unavailable")
		item.Status = model.VirtualCharacterValidationFailed
		redirectValidationResult(c, item)
		return
	}
	client, err := service.NewVolcAssetClient(account)
	if err == nil {
		var groupID string
		groupID, err = client.GetVisualValidateResult(c.Request.Context(), storedToken, account.ProjectName)
		if err == nil {
			_, err = model.CompleteVirtualCharacterValidation(item.ID, groupID)
			if err != nil {
				_ = model.CreateVirtualCharacterCleanupJob(&model.VirtualCharacterCleanupJob{ProviderAccountID: account.ID, TargetType: "volc_group", TargetID: groupID})
			}
		}
	}
	if err != nil {
		_ = model.MarkVirtualCharacterValidationFailed(item.ID, resultCode, common.MaskSensitiveInfo(err.Error()))
		item.Status = model.VirtualCharacterValidationFailed
	} else {
		item.Status = model.VirtualCharacterValidationSucceeded
	}
	redirectValidationResult(c, item)
}

func UploadVirtualCharacterAsset(c *gin.Context) {
	characterID, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	character, err := model.GetOwnedVirtualCharacter(characterID, c.GetInt("id"))
	if err != nil || character.SourceType != model.VirtualCharacterSourceVolcRealPerson {
		virtualCharacterLookupError(c, gorm.ErrRecordNotFound)
		return
	}
	if character.Status != model.VirtualCharacterStatusActive || character.ValidationStatus != model.VirtualCharacterValidationAccepted {
		virtualCharacterError(c, http.StatusConflict, "character_unavailable", "character is not ready for assets")
		return
	}
	account, client, err := enabledVirtualCharacterClient(true)
	if err != nil || account.ID != character.ProviderAccountID {
		virtualCharacterError(c, http.StatusServiceUnavailable, "provider_unavailable", "virtual character provider is unavailable")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, virtualCharacterUploadMaxBody)
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_upload", "invalid or oversized multipart upload")
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "missing_file", "file is required")
		return
	}
	assetType, mimeType, err := validateVolcCharacterAssetUpload(header, c.PostForm("asset_type"))
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_file", err.Error())
		return
	}
	file, err := header.Open()
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_file", err.Error())
		return
	}
	defer file.Close()
	storage, err := service.NewAIPDDVirtualCharacterStorage()
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "staging_unavailable", err.Error())
		return
	}
	stored, err := storage.UploadPrivateFile(c.Request.Context(), header.Filename, file)
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "staging_upload_failed", err.Error())
		return
	}
	cleanupStaging := true
	defer func() {
		if cleanupStaging {
			_ = storage.DeleteFile(c.Request.Context(), stored.FileID)
		}
	}()
	signed, err := storage.SignFile(c.Request.Context(), stored.FileID)
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "staging_sign_failed", err.Error())
		return
	}
	name := strings.TrimSpace(c.PostForm("name"))
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	}
	providerAssetID, err := client.CreateAsset(c.Request.Context(), character.ProviderGroupID, signed.URL, assetType, name, account.ProjectName)
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "provider_asset_failed", common.MaskSensitiveInfo(err.Error()))
		return
	}
	asset := &model.VirtualCharacterAsset{CharacterID: character.ID, ProviderAccountID: account.ID, ProviderAssetID: providerAssetID, Name: name, AssetType: assetType, Status: model.VirtualCharacterAssetStatusProcessing, StagingFileID: stored.FileID, MimeType: mimeType, FileSize: header.Size, NextPollAt: time.Now().Add(5 * time.Second).Unix()}
	if err := model.CreateVirtualCharacterAsset(asset); err != nil {
		_ = model.CreateVirtualCharacterCleanupJob(&model.VirtualCharacterCleanupJob{CharacterID: character.ID, ProviderAccountID: account.ID, TargetType: "volc_asset", TargetID: providerAssetID})
		virtualCharacterError(c, http.StatusInternalServerError, "asset_save_failed", err.Error())
		return
	}
	cleanupStaging = false
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": virtualCharacterAssetResponse{ID: asset.ID, Name: asset.Name, AssetType: asset.AssetType, Status: asset.Status, IsPrimary: asset.IsPrimary, MimeType: asset.MimeType, FileSize: asset.FileSize, ProviderAssetID: asset.ProviderAssetID, CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt}})
}

func SetVirtualCharacterPrimaryAsset(c *gin.Context) {
	characterID, characterErr := parseVirtualCharacterID(c.Param("id"))
	assetID, assetErr := parseVirtualCharacterID(c.Param("asset_id"))
	if characterErr != nil || assetErr != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_id", "invalid character or asset id")
		return
	}
	if err := model.SetVirtualCharacterPrimaryAsset(characterID, assetID, c.GetInt("id")); err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"character_id": characterID, "asset_id": assetID})
}

func DeleteVirtualCharacterAsset(c *gin.Context) {
	characterID, characterErr := parseVirtualCharacterID(c.Param("id"))
	assetID, assetErr := parseVirtualCharacterID(c.Param("asset_id"))
	if characterErr != nil || assetErr != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_id", "invalid character or asset id")
		return
	}
	if err := model.BeginVirtualCharacterAssetDelete(characterID, assetID, c.GetInt("id")); err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": assetID, "status": model.VirtualCharacterAssetStatusDeleting})
}

func DeleteVirtualCharacterGroup(c *gin.Context) {
	characterID, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	if err := model.BeginVirtualCharacterGroupDelete(characterID, c.GetInt("id")); err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": characterID, "status": model.VirtualCharacterStatusDeleting})
}

func VirtualCharacterGone(c *gin.Context) {
	virtualCharacterError(c, http.StatusGone, "virtual_character_endpoint_removed", "this virtual character endpoint has been removed")
}

func AdminGetVirtualCharacterABSettings(c *gin.Context) {
	account, err := model.GetVirtualCharacterProviderAccount()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		account = &model.VirtualCharacterProviderAccount{Region: "cn-beijing", ProjectName: "default"}
	} else if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "settings_failed", err.Error())
		return
	}
	channels, err := virtualCharacterProviderChannels()
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "settings_failed", err.Error())
		return
	}
	latest, latestErr := model.GetLatestVirtualCharacterCatalogImport()
	var catalog any
	if latestErr == nil {
		catalog = latest
	}
	common.ApiSuccess(c, gin.H{"enabled": account.Enabled, "official_enabled": account.OfficialEnabled, "real_person_enabled": account.RealPersonEnabled, "access_key_masked": maskProviderCredential(account.EncryptedAccessKey != ""), "secret_key_masked": maskProviderCredential(account.EncryptedSecretKey != ""), "region": account.Region, "project_name": account.ProjectName, "channel_id": account.ChannelID, "crypto_ready": common.HasStableCryptoSecret(), "last_check_status": account.LastCheckStatus, "last_check_error": account.LastCheckError, "last_checked_at": account.LastCheckedAt, "channels": channels, "catalog": catalog, "global_limit": model.GetVirtualCharacterGlobalLimit(), "models": model.GetVirtualCharacterModels(), "default_model": model.GetVirtualCharacterDefaultModel()})
}

func AdminUpdateVirtualCharacterABSettings(c *gin.Context) {
	var req struct {
		Enabled           bool     `json:"enabled"`
		OfficialEnabled   bool     `json:"official_enabled"`
		RealPersonEnabled bool     `json:"real_person_enabled"`
		AccessKey         string   `json:"access_key"`
		SecretKey         string   `json:"secret_key"`
		Region            string   `json:"region"`
		ProjectName       string   `json:"project_name"`
		ChannelID         int      `json:"channel_id"`
		GlobalLimit       int      `json:"global_limit"`
		Models            []string `json:"models"`
		DefaultModel      string   `json:"default_model"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if req.Enabled && !common.HasStableCryptoSecret() {
		virtualCharacterError(c, http.StatusBadRequest, "crypto_not_configured", common.ErrCryptoSecretNotConfigured.Error())
		return
	}
	account, err := model.GetVirtualCharacterProviderAccount()
	if errors.Is(err, gorm.ErrRecordNotFound) {
		account = &model.VirtualCharacterProviderAccount{}
	} else if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "settings_failed", err.Error())
		return
	}
	if strings.TrimSpace(req.AccessKey) != "" {
		account.EncryptedAccessKey, err = common.EncryptSensitiveValue(strings.TrimSpace(req.AccessKey))
		if err != nil {
			virtualCharacterError(c, http.StatusBadRequest, "crypto_not_configured", err.Error())
			return
		}
	}
	if strings.TrimSpace(req.SecretKey) != "" {
		account.EncryptedSecretKey, err = common.EncryptSensitiveValue(strings.TrimSpace(req.SecretKey))
		if err != nil {
			virtualCharacterError(c, http.StatusBadRequest, "crypto_not_configured", err.Error())
			return
		}
	}
	if req.Enabled && (account.EncryptedAccessKey == "" || account.EncryptedSecretKey == "") {
		virtualCharacterError(c, http.StatusBadRequest, "credentials_required", "AK and SK are required before enabling the provider")
		return
	}
	if req.ChannelID > 0 {
		if _, err := validateVirtualCharacterProviderChannel(req.ChannelID, strings.TrimSpace(req.DefaultModel)); err != nil {
			virtualCharacterError(c, http.StatusBadRequest, "invalid_channel", err.Error())
			return
		}
	} else if req.Enabled {
		virtualCharacterError(c, http.StatusBadRequest, "channel_required", "a stable Volc video channel is required")
		return
	}
	models, err := normalizeVirtualCharacterModels(req.Models)
	if err != nil || (req.DefaultModel != "" && !containsString(models, req.DefaultModel)) {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_models", "default model must be one of the enabled models")
		return
	}
	if req.GlobalLimit <= 0 || req.GlobalLimit > 10000 {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_limit", "global limit must be between 1 and 10000")
		return
	}
	account.Enabled, account.OfficialEnabled, account.RealPersonEnabled = req.Enabled, req.OfficialEnabled, req.RealPersonEnabled
	account.Region, account.ProjectName, account.ChannelID = strings.TrimSpace(req.Region), strings.TrimSpace(req.ProjectName), req.ChannelID
	if err := model.SaveVirtualCharacterProviderAccount(account); err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "settings_failed", err.Error())
		return
	}
	for key, value := range map[string]string{"VirtualCharacterLimit": strconv.Itoa(req.GlobalLimit), "VirtualCharacterModels": strings.Join(models, ","), "VirtualCharacterDefaultModel": strings.TrimSpace(req.DefaultModel)} {
		if err := model.UpdateOption(key, value); err != nil {
			virtualCharacterError(c, http.StatusInternalServerError, "settings_failed", err.Error())
			return
		}
	}
	AdminGetVirtualCharacterABSettings(c)
}

func AdminTestVirtualCharacterProvider(c *gin.Context) {
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "provider_unavailable", "provider account is not enabled")
		return
	}
	client, err := service.NewVolcAssetClient(account)
	if err == nil {
		_, err = client.ListAssetGroups(c.Request.Context(), account.ProjectName)
	}
	now := time.Now().Unix()
	status, message := "ok", ""
	if err != nil {
		status, message = "failed", common.MaskSensitiveInfo(err.Error())
	}
	_ = model.DB.Model(account).Updates(map[string]any{"last_check_status": status, "last_check_error": message, "last_checked_at": now, "updated_at": now}).Error
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "connection_test_failed", message)
		return
	}
	common.ApiSuccess(c, gin.H{"status": status, "checked_at": now})
}

func AdminImportVirtualCharacterCatalog(c *gin.Context) {
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "provider_unavailable", "provider account is not enabled")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, virtualCharacterImportMax+(1<<20))
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_import", "invalid import payload")
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "missing_file", "catalog file is required")
		return
	}
	file, err := header.Open()
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_import", err.Error())
		return
	}
	defer file.Close()
	payload, err := io.ReadAll(io.LimitReader(file, virtualCharacterImportMax+1))
	if err != nil || int64(len(payload)) > virtualCharacterImportMax {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_import", "catalog file is too large")
		return
	}
	version, entries, err := parseVirtualCharacterCatalog(header.Filename, payload, c.PostForm("version"))
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_import", err.Error())
		return
	}
	hash := sha256.Sum256(payload)
	dryRun := parseVirtualCharacterDeclaration(c.PostForm("dry_run"))
	if dryRun {
		common.ApiSuccess(c, gin.H{"dry_run": true, "version": version, "content_hash": hex.EncodeToString(hash[:]), "total": len(entries)})
		return
	}
	stats, err := model.ApplyVirtualCharacterCatalog(version, hex.EncodeToString(hash[:]), entries, c.GetInt("id"), account.ID)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "catalog_import_failed", err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"dry_run": false, "version": version, "content_hash": hex.EncodeToString(hash[:]), "stats": stats})
}

func virtualCharacterGroupToResponse(item *model.VirtualCharacter, includeAssets bool) (virtualCharacterGroupResponse, error) {
	response := virtualCharacterGroupResponse{ID: item.ID, Scope: item.Scope, SourceType: item.SourceType, Name: item.Name, Description: item.Description, Tags: decodeVirtualCharacterTags(item.TagsJSON), Status: item.Status, ValidationStatus: item.ValidationStatus, CoverURL: item.CoverURL, PrimaryAssetID: item.PrimaryAssetID, LastError: item.LastError, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, CatalogVersion: item.CatalogVersion, Assets: []virtualCharacterAssetResponse{}}
	if !includeAssets {
		return response, nil
	}
	assets, err := model.ListVirtualCharacterAssets(item.ID, false)
	if err != nil {
		return response, err
	}
	for i := range assets {
		asset := &assets[i]
		response.Assets = append(response.Assets, virtualCharacterAssetResponse{ID: asset.ID, Name: asset.Name, AssetType: asset.AssetType, Status: asset.Status, IsPrimary: asset.IsPrimary, CoverURL: asset.CoverURL, MimeType: asset.MimeType, FileSize: asset.FileSize, LastError: asset.LastError, ProviderAssetID: asset.ProviderAssetID, CreatedAt: asset.CreatedAt, UpdatedAt: asset.UpdatedAt})
	}
	return response, nil
}

func validationSessionResponse(item *model.VirtualCharacterValidationSession, launchURL string) gin.H {
	return gin.H{"id": item.ID, "status": item.Status, "launch_url": launchURL, "expires_at": item.ExpiresAt, "character_id": item.CharacterID, "last_error": item.LastError, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt}
}

func enabledVirtualCharacterClient(realPerson bool) (*model.VirtualCharacterProviderAccount, service.VolcAssetClient, error) {
	if !common.HasStableCryptoSecret() {
		return nil, nil, common.ErrCryptoSecretNotConfigured
	}
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	if err != nil {
		return nil, nil, errors.New("virtual character provider account is not enabled")
	}
	if realPerson && !account.RealPersonEnabled {
		return nil, nil, errors.New("real-person virtual characters are disabled")
	}
	client, err := service.NewVolcAssetClient(account)
	return account, client, err
}

func validateVolcCharacterAssetUpload(header *multipart.FileHeader, declaredType string) (string, string, error) {
	if header == nil || header.Size <= 0 {
		return "", "", errors.New("file is empty")
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	mimeType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	typeName := strings.TrimSpace(declaredType)
	if typeName == "" {
		switch ext {
		case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".heic":
			typeName = model.VirtualCharacterAssetTypeImage
		case ".mp4", ".mov":
			typeName = model.VirtualCharacterAssetTypeVideo
		case ".mp3", ".wav":
			typeName = model.VirtualCharacterAssetTypeAudio
		}
	}
	limits := map[string]int64{model.VirtualCharacterAssetTypeImage: 30 << 20, model.VirtualCharacterAssetTypeVideo: 50 << 20, model.VirtualCharacterAssetTypeAudio: 15 << 20}
	limit, ok := limits[typeName]
	if !ok {
		return "", "", errors.New("asset_type must be Image, Video, or Audio")
	}
	allowed := map[string]map[string]bool{model.VirtualCharacterAssetTypeImage: {".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true, ".heic": true}, model.VirtualCharacterAssetTypeVideo: {".mp4": true, ".mov": true}, model.VirtualCharacterAssetTypeAudio: {".mp3": true, ".wav": true}}
	if !allowed[typeName][ext] {
		return "", "", fmt.Errorf("file extension is not supported for %s", typeName)
	}
	if header.Size > limit {
		return "", "", fmt.Errorf("%s file exceeds the %d MB limit", typeName, limit>>20)
	}
	return typeName, mimeType, nil
}

func parseVirtualCharacterCatalog(filename string, payload []byte, fallbackVersion string) (string, []model.VirtualCharacterCatalogEntry, error) {
	type rawEntry struct {
		AssetID     string   `json:"asset_id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		CoverURL    string   `json:"cover_url"`
		Enabled     *bool    `json:"enabled"`
	}
	version := strings.TrimSpace(fallbackVersion)
	rawEntries := make([]rawEntry, 0)
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".json":
		var manifest struct {
			Version string     `json:"version"`
			Items   []rawEntry `json:"items"`
		}
		if err := common.Unmarshal(payload, &manifest); err != nil {
			return "", nil, err
		}
		if strings.TrimSpace(manifest.Version) != "" {
			version = strings.TrimSpace(manifest.Version)
		}
		rawEntries = manifest.Items
	case ".csv":
		reader := csv.NewReader(strings.NewReader(string(payload)))
		records, err := reader.ReadAll()
		if err != nil || len(records) < 2 {
			return "", nil, errors.New("CSV catalog must contain a header and at least one row")
		}
		headers := make(map[string]int)
		for index, value := range records[0] {
			headers[strings.ToLower(strings.TrimSpace(value))] = index
		}
		for _, required := range []string{"asset_id", "name", "cover_url"} {
			if _, exists := headers[required]; !exists {
				return "", nil, fmt.Errorf("CSV catalog is missing %s", required)
			}
		}
		for _, row := range records[1:] {
			read := func(key string) string {
				index, ok := headers[key]
				if !ok || index >= len(row) {
					return ""
				}
				return strings.TrimSpace(row[index])
			}
			tags := []string{}
			if value := read("tags"); value != "" {
				for _, tag := range strings.Split(value, "|") {
					tags = append(tags, strings.TrimSpace(tag))
				}
			}
			enabled := true
			if value := strings.ToLower(read("enabled")); value == "false" || value == "0" || value == "no" {
				enabled = false
			}
			rawEntries = append(rawEntries, rawEntry{AssetID: read("asset_id"), Name: read("name"), Description: read("description"), Tags: tags, CoverURL: read("cover_url"), Enabled: &enabled})
		}
	default:
		return "", nil, errors.New("catalog must be a JSON or CSV file")
	}
	if version == "" {
		return "", nil, errors.New("catalog version is required")
	}
	if len(rawEntries) == 0 || len(rawEntries) > 10000 {
		return "", nil, errors.New("catalog must contain between 1 and 10000 entries")
	}
	seen := make(map[string]struct{}, len(rawEntries))
	entries := make([]model.VirtualCharacterCatalogEntry, 0, len(rawEntries))
	for _, raw := range rawEntries {
		assetID := strings.TrimPrefix(strings.TrimSpace(raw.AssetID), "asset://")
		if assetID == "" || strings.ContainsAny(assetID, " \t\r\n") {
			return "", nil, errors.New("catalog contains an invalid asset_id")
		}
		if _, exists := seen[assetID]; exists {
			return "", nil, fmt.Errorf("catalog contains duplicate asset_id %s", assetID)
		}
		seen[assetID] = struct{}{}
		metadata, tagsJSON, err := normalizeVirtualCharacterMetadata(virtualCharacterMetadataRequest{Name: raw.Name, Description: raw.Description, Tags: raw.Tags})
		if err != nil {
			return "", nil, fmt.Errorf("asset %s: %w", assetID, err)
		}
		cover, err := url.Parse(strings.TrimSpace(raw.CoverURL))
		if err != nil || cover.Host == "" || (cover.Scheme != "https" && cover.Scheme != "http") {
			return "", nil, fmt.Errorf("asset %s has an invalid cover_url", assetID)
		}
		enabled := true
		if raw.Enabled != nil {
			enabled = *raw.Enabled
		}
		entries = append(entries, model.VirtualCharacterCatalogEntry{AssetID: assetID, Name: metadata.Name, Description: metadata.Description, TagsJSON: tagsJSON, CoverURL: cover.String(), Enabled: enabled})
	}
	return version, entries, nil
}

func virtualCharacterProviderChannels() ([]gin.H, error) {
	channels, err := model.GetAllChannels(0, 10000, true, true)
	if err != nil {
		return nil, err
	}
	result := make([]gin.H, 0)
	for _, channel := range channels {
		if channel.Status == common.ChannelStatusEnabled && !channel.ChannelInfo.IsMultiKey && (channel.Type == constant.ChannelTypeVolcEngine || channel.Type == constant.ChannelTypeDoubaoVideo) {
			result = append(result, gin.H{"id": channel.Id, "name": channel.Name, "type": channel.Type, "models": channel.Models})
		}
	}
	return result, nil
}

func validateVirtualCharacterProviderChannel(channelID int, modelName string) (*model.Channel, error) {
	channel, err := model.GetChannelById(channelID, true)
	if err != nil {
		return nil, errors.New("configured channel was not found")
	}
	if channel.Status != common.ChannelStatusEnabled || channel.ChannelInfo.IsMultiKey || (channel.Type != constant.ChannelTypeVolcEngine && channel.Type != constant.ChannelTypeDoubaoVideo) {
		return nil, errors.New("configured channel must be an enabled, single-key Volc video channel")
	}
	if modelName != "" {
		for _, supported := range strings.Split(channel.Models, ",") {
			if strings.TrimSpace(supported) == modelName {
				return channel, nil
			}
		}
		return nil, errors.New("configured channel does not support the default role model")
	}
	return channel, nil
}

func randomHex(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func hashValidationState(value string) string {
	hash := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(hash[:])
}

func redirectValidationResult(c *gin.Context, item *model.VirtualCharacterValidationSession) {
	base := strings.TrimRight(strings.TrimSpace(service.GetCallbackAddress()), "/")
	if base == "" {
		c.AbortWithStatus(http.StatusOK)
		return
	}
	destination := base + "/console/virtual-characters?validation_session=" + url.QueryEscape(item.ID) + "&validation_status=" + url.QueryEscape(item.Status)
	c.Redirect(http.StatusFound, destination)
}

func maskProviderCredential(configured bool) string {
	if !configured {
		return ""
	}
	return "••••••••"
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(wanted) {
			return true
		}
	}
	return false
}
