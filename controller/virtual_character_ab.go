package controller

import (
	"context"
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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	virtualCharacterValidationTTL = 30 * time.Minute
	virtualCharacterUploadMaxBody = int64(31 << 20)
)

type virtualCharacterStagingStorage interface {
	UploadPrivateFile(ctx context.Context, filename string, reader io.Reader) (*service.AIPDDStoredFile, error)
	SignFile(ctx context.Context, fileID string) (*service.AIPDDSignedURL, error)
	DeleteFile(ctx context.Context, fileID string) error
}

// newVirtualCharacterStagingStorage is overridable in tests.
var newVirtualCharacterStagingStorage = func() (virtualCharacterStagingStorage, error) {
	return service.NewAIPDDVirtualCharacterStorage()
}

type virtualCharacterImageCreateError struct {
	Status  int
	Code    string
	Message string
}

func (e *virtualCharacterImageCreateError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type virtualCharacterGroupResponse struct {
	ID                  int64                                  `json:"id"`
	Scope               string                                 `json:"scope"`
	SourceType          string                                 `json:"source_type"`
	Name                string                                 `json:"name"`
	Description         string                                 `json:"description"`
	Tags                []string                               `json:"tags"`
	Nationality         string                                 `json:"nationality,omitempty"`
	Gender              string                                 `json:"gender,omitempty"`
	AgeMin              *int                                   `json:"age_min,omitempty"`
	AgeMax              *int                                   `json:"age_max,omitempty"`
	Occupation          string                                 `json:"occupation,omitempty"`
	Temperament         string                                 `json:"temperament,omitempty"`
	Status              string                                 `json:"status"`
	ValidationStatus    string                                 `json:"validation_status"`
	CoverURL            string                                 `json:"cover_url,omitempty"`
	ProviderAssetID     string                                 `json:"provider_asset_id,omitempty"`
	AssetUploadRequired bool                                   `json:"asset_upload_required,omitempty"`
	MimeType            string                                 `json:"mime_type,omitempty"`
	FileSize            int64                                  `json:"file_size,omitempty"`
	LastError           string                                 `json:"last_error,omitempty"`
	CreatedAt           int64                                  `json:"created_at"`
	UpdatedAt           int64                                  `json:"updated_at"`
	CatalogVersion      string                                 `json:"catalog_version,omitempty"`
	Authorization       *virtualCharacterAuthorizationResponse `json:"authorization,omitempty"`
}

type virtualCharacterAuthorizationResponse struct {
	Status              string `json:"status"`
	ProviderGroupStatus string `json:"provider_group_status,omitempty"`
	ProviderAssetStatus string `json:"provider_asset_status,omitempty"`
	ProviderCheckedAt   int64  `json:"provider_checked_at,omitempty"`
	AuthorizedAt        int64  `json:"authorized_at,omitempty"`
	RevokedAt           int64  `json:"revoked_at,omitempty"`
	LastError           string `json:"last_error,omitempty"`
}

func parseVirtualCharacterListFilter(c *gin.Context) (model.VirtualCharacterListFilter, error) {
	filter := model.VirtualCharacterListFilter{
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		Nationality: strings.TrimSpace(c.Query("nationality")),
		Gender:      strings.TrimSpace(c.Query("gender")),
		Status:      strings.TrimSpace(c.Query("status")),
		SourceType:  strings.TrimSpace(c.Query("source_type")),
	}
	if ageBand := strings.TrimSpace(c.Query("age_band")); ageBand != "" {
		ageMin, ageMax, ok := model.ParseVirtualCharacterAgeBandKey(ageBand)
		if !ok {
			return filter, errors.New("age_band must be one of 0-20, 20-40, 40-60, 60-80, 80-100")
		}
		filter.AgeMin = &ageMin
		filter.AgeMax = &ageMax
	}
	if filter.Status != "" {
		switch filter.Status {
		case model.VirtualCharacterStatusCreating, model.VirtualCharacterStatusActive,
			model.VirtualCharacterStatusBlocked, model.VirtualCharacterStatusOffline,
			model.VirtualCharacterStatusDeleting, model.VirtualCharacterStatusFailed:
		default:
			return filter, errors.New("invalid status filter")
		}
	}
	if filter.SourceType != "" {
		switch filter.SourceType {
		case model.VirtualCharacterSourceVolcPreset, model.VirtualCharacterSourceVolcAIGC, model.VirtualCharacterSourceVolcRealPerson:
		default:
			return filter, errors.New("invalid source_type filter")
		}
	}
	return filter, nil
}

func ListVirtualCharacterGroups(c *gin.Context) {
	userID := c.GetInt("id")
	scope := strings.TrimSpace(c.DefaultQuery("scope", model.VirtualCharacterScopePublic))
	if scope != model.VirtualCharacterScopePublic && scope != model.VirtualCharacterScopePrivate {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_scope", "scope must be public or private")
		return
	}
	filter, err := parseVirtualCharacterListFilter(c)
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	page := getVirtualCharacterPage(c)
	items, total, err := model.ListVirtualCharacters(userID, scope, false, filter, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	responses := make([]virtualCharacterGroupResponse, 0, len(items))
	for i := range items {
		responses = append(responses, virtualCharacterGroupToResponse(&items[i]))
	}
	page.SetTotal(int(total))
	page.SetItems(responses)
	used, _ := model.CountActivePrivateVirtualCharacters(userID)
	realUsed, _ := model.CountRealPersonVirtualCharacters(userID)
	common.ApiSuccess(c, gin.H{"page": page, "used": used, "limit": model.GetVirtualCharacterEffectiveLimit(userID), "real_person_used": realUsed, "real_person_limit": model.GetVirtualCharacterRealPersonLimit()})
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
	common.ApiSuccess(c, virtualCharacterGroupToResponse(item))
}

func AdminListVirtualCharacterGroups(c *gin.Context) {
	filter, err := parseVirtualCharacterListFilter(c)
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_filter", err.Error())
		return
	}
	page := getVirtualCharacterPage(c)
	items, total, err := model.ListVirtualCharacters(0, model.VirtualCharacterScopePublic, true, filter, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	responses := make([]virtualCharacterGroupResponse, 0, len(items))
	for i := range items {
		responses = append(responses, virtualCharacterGroupToResponse(&items[i]))
	}
	page.SetTotal(int(total))
	page.SetItems(responses)
	common.ApiSuccess(c, page)
}

func GetVirtualCharacterABConfig(c *gin.Context) {
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	libraryEnabled := err == nil && common.HasStableCryptoSecret()
	common.ApiSuccess(c, gin.H{
		"image_max_mb": 30, "task_retention_days": 90,
		"official_enabled":    libraryEnabled && account.OfficialEnabled,
		"virtual_enabled":     libraryEnabled && account.VirtualEnabled,
		"real_person_enabled": libraryEnabled && account.RealPersonEnabled,
		"real_person_limit":   model.GetVirtualCharacterRealPersonLimit(),
		"account_asset_cap":   model.GetVirtualCharacterAccountAssetCap(),
	})
}

func CreateVirtualCharacter(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		virtualCharacterError(c, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, virtualCharacterUploadMaxBody)
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_upload", "invalid or oversized multipart upload")
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	header, err := c.FormFile("file")
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "missing_file", "primary image file is required")
		return
	}
	if _, err := validateVolcCharacterImageUpload(header); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_file", err.Error())
		return
	}
	account, client, err := enabledVirtualCharacterClientForSource(model.VirtualCharacterSourceVolcAIGC)
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "virtual_disabled", err.Error())
		return
	}
	assetCount, countErr := model.CountVirtualCharacterProviderAssets()
	if countErr != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "quota_check_failed", countErr.Error())
		return
	}
	if assetCount >= int64(model.GetVirtualCharacterAccountAssetCap()) {
		virtualCharacterError(c, http.StatusConflict, "account_asset_cap_reached", "account asset capacity has been reached")
		return
	}
	metadata, tagsJSON, err := normalizeVirtualCharacterMetadata(virtualCharacterMetadataRequest{
		Name: c.PostForm("name"), Description: c.PostForm("description"), Tags: parseVirtualCharacterTags(c.PostForm("tags")),
	})
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_metadata", err.Error())
		return
	}
	item, _, err := model.CreateAIGCVirtualCharacter(userID, account.ID, metadata.Name, metadata.Description, tagsJSON)
	if err != nil {
		if strings.Contains(err.Error(), "limit reached") {
			virtualCharacterError(c, http.StatusConflict, "limit_reached", err.Error())
			return
		}
		virtualCharacterError(c, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}
	groupID, err := client.CreateAssetGroup(c.Request.Context(), metadata.Name, metadata.Description, account.ProjectName)
	if err != nil {
		message := service.LocalizeVolcAssetError(err)
		if discardErr := model.DiscardFailedAIGCVirtualCharacterUpload(item.ID, "", message); discardErr != nil {
			common.SysError(fmt.Sprintf("discard failed virtual character upload %d: %v", item.ID, discardErr))
		}
		virtualCharacterError(c, http.StatusBadGateway, "provider_group_failed", message)
		return
	}
	if err := model.AttachVirtualCharacterProviderGroup(item.ID, groupID); err != nil {
		if discardErr := model.DiscardFailedAIGCVirtualCharacterUpload(item.ID, groupID, err.Error()); discardErr != nil {
			common.SysError(fmt.Sprintf("discard failed virtual character upload %d: %v", item.ID, discardErr))
		}
		virtualCharacterError(c, http.StatusInternalServerError, "attach_group_failed", err.Error())
		return
	}
	item.ProviderGroupID = groupID
	createErr := stageAndCreateVirtualCharacterImage(c.Request.Context(), item, account, client, header, metadata.Name)
	if createErr != nil {
		if discardErr := model.DiscardFailedAIGCVirtualCharacterUpload(item.ID, groupID, createErr.Message); discardErr != nil {
			common.SysError(fmt.Sprintf("discard failed virtual character upload %d: %v", item.ID, discardErr))
		}
		virtualCharacterError(c, createErr.Status, createErr.Code, createErr.Message)
		return
	}
	item, err = model.GetVirtualCharacterByID(item.ID)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": virtualCharacterGroupToResponse(item)})
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
	used, countErr := model.CountRealPersonVirtualCharacters(userID)
	if countErr != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "count_failed", countErr.Error())
		return
	}
	if used >= int64(model.GetVirtualCharacterRealPersonLimit()) {
		virtualCharacterError(c, http.StatusConflict, "character_limit_reached", "real-person character limit reached")
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
	character, _, _, err := model.ReserveRealPersonVirtualCharacter(userID, account.ID, metadata.Name, metadata.Description, tagsJSON)
	if err != nil {
		if strings.Contains(err.Error(), "limit reached") {
			virtualCharacterError(c, http.StatusConflict, "character_limit_reached", err.Error())
			return
		}
		virtualCharacterError(c, http.StatusInternalServerError, "reservation_failed", err.Error())
		return
	}
	releaseReservation := func() {
		_ = model.DeleteRealPersonReservation(character.ID, userID)
	}
	callbackURL := base + "/api/virtual-characters/validation/callback?state=" + url.QueryEscape(state)
	providerSession, err := client.CreateVisualValidateSession(c.Request.Context(), callbackURL, account.ProjectName, language)
	if err != nil {
		releaseReservation()
		virtualCharacterError(c, http.StatusBadGateway, "provider_session_failed", common.MaskSensitiveInfo(err.Error()))
		return
	}
	encryptedToken, err := common.EncryptSensitiveValue(providerSession.BytedToken)
	if err != nil {
		releaseReservation()
		virtualCharacterError(c, http.StatusServiceUnavailable, "crypto_not_configured", err.Error())
		return
	}
	encryptedLink, err := common.EncryptSensitiveValue(providerSession.H5Link)
	if err != nil {
		releaseReservation()
		virtualCharacterError(c, http.StatusServiceUnavailable, "crypto_not_configured", err.Error())
		return
	}
	now := time.Now()
	item := &model.VirtualCharacterValidationSession{ID: sessionID, UserID: userID, ProviderAccountID: account.ID, Status: model.VirtualCharacterValidationPending, StateHash: hashValidationState(state), EncryptedBytedToken: encryptedToken, EncryptedH5Link: encryptedLink, Name: metadata.Name, Description: metadata.Description, TagsJSON: tagsJSON, Language: language, CharacterID: character.ID, ExpiresAt: now.Add(virtualCharacterValidationTTL).Unix()}
	if err := model.CreateVirtualCharacterValidationSession(item); err != nil {
		releaseReservation()
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

func CancelVirtualCharacterValidationSession(c *gin.Context) {
	cancelled, err := model.CancelReservedVirtualCharacterValidation(c.Param("id"), c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": c.Param("id"), "cancelled": cancelled})
}

func LaunchVirtualCharacterValidation(c *gin.Context) {
	state := strings.TrimSpace(c.Query("state"))
	if state == "" {
		c.AbortWithStatus(http.StatusGone)
		return
	}
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
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	c.Header("Cache-Control", "no-store")
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
		_ = model.FailReservedVirtualCharacterValidation(item.ID, model.VirtualCharacterValidationExpired, "expired", "validation session expired")
		item.Status = model.VirtualCharacterValidationExpired
		redirectValidationResult(c, item)
		return
	}
	storedToken, err := common.DecryptSensitiveValue(item.EncryptedBytedToken)
	callbackToken := strings.TrimSpace(c.Query("bytedToken"))
	if err != nil || subtle.ConstantTimeCompare([]byte(storedToken), []byte(callbackToken)) != 1 {
		_ = model.FailReservedVirtualCharacterValidation(item.ID, model.VirtualCharacterValidationFailed, "token_mismatch", "validation token mismatch")
		item.Status = model.VirtualCharacterValidationFailed
		redirectValidationResult(c, item)
		return
	}
	resultCode := strings.TrimSpace(c.Query("resultCode"))
	if resultCode != "10000" {
		_ = model.FailReservedVirtualCharacterValidation(item.ID, model.VirtualCharacterValidationFailed, resultCode, "visual validation failed")
		item.Status = model.VirtualCharacterValidationFailed
		redirectValidationResult(c, item)
		return
	}
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	if err != nil || account.ID != item.ProviderAccountID {
		_ = model.FailReservedVirtualCharacterValidation(item.ID, model.VirtualCharacterValidationFailed, resultCode, "provider account is unavailable")
		item.Status = model.VirtualCharacterValidationFailed
		redirectValidationResult(c, item)
		return
	}
	client, err := service.NewVolcAssetClient(account)
	if err == nil {
		var groupID string
		groupID, err = client.GetVisualValidateResult(c.Request.Context(), storedToken, account.ProjectName)
		if err == nil {
			var receipt string
			receipt, err = realPersonValidationReceiptHash(item, groupID)
			if err == nil {
				_, err = model.CompleteReservedVirtualCharacterValidation(item.ID, groupID, receipt)
			}
			if err != nil {
				_ = model.CreateVirtualCharacterCleanupJob(&model.VirtualCharacterCleanupJob{ProviderAccountID: account.ID, TargetType: "volc_group", TargetID: groupID})
			}
		}
	}
	if err != nil {
		_ = model.FailReservedVirtualCharacterValidation(item.ID, model.VirtualCharacterValidationFailed, resultCode, common.MaskSensitiveInfo(err.Error()))
		item.Status = model.VirtualCharacterValidationFailed
	} else {
		item.Status = model.VirtualCharacterValidationSucceeded
	}
	redirectValidationResult(c, item)
}

func realPersonValidationReceiptHash(session *model.VirtualCharacterValidationSession, groupID string) (string, error) {
	if session == nil || session.CharacterID <= 0 || session.UserID <= 0 || strings.TrimSpace(groupID) == "" {
		return "", errors.New("invalid real-person validation receipt")
	}
	receipt := struct {
		SessionID         string `json:"session_id"`
		CharacterID       int64  `json:"character_id"`
		UserID            int    `json:"user_id"`
		ProviderAccountID int    `json:"provider_account_id"`
		ProviderGroupID   string `json:"provider_group_id"`
		ResultCode        string `json:"result_code"`
	}{
		SessionID: session.ID, CharacterID: session.CharacterID, UserID: session.UserID,
		ProviderAccountID: session.ProviderAccountID, ProviderGroupID: strings.TrimSpace(groupID), ResultCode: "10000",
	}
	payload, err := common.Marshal(receipt)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:]), nil
}

func stageAndCreateVirtualCharacterImage(
	ctx context.Context,
	character *model.VirtualCharacter,
	account *model.VirtualCharacterProviderAccount,
	client service.VolcAssetClient,
	header *multipart.FileHeader,
	name string,
) *virtualCharacterImageCreateError {
	mimeType, err := validateVolcCharacterImageUpload(header)
	if err != nil {
		return &virtualCharacterImageCreateError{Status: http.StatusBadRequest, Code: "invalid_file", Message: err.Error()}
	}
	file, err := header.Open()
	if err != nil {
		return &virtualCharacterImageCreateError{Status: http.StatusBadRequest, Code: "invalid_file", Message: err.Error()}
	}
	defer file.Close()
	storage, err := newVirtualCharacterStagingStorage()
	if err != nil {
		return &virtualCharacterImageCreateError{Status: http.StatusServiceUnavailable, Code: "staging_unavailable", Message: err.Error()}
	}
	stored, err := storage.UploadPrivateFile(ctx, header.Filename, file)
	if err != nil {
		return &virtualCharacterImageCreateError{Status: http.StatusBadGateway, Code: "staging_upload_failed", Message: err.Error()}
	}
	cleanupStaging := true
	defer func() {
		if !cleanupStaging {
			return
		}
		// ctx is usually already cancelled on this path (client gone or request timed
		// out), so drop the staged file on an independent deadline and fall back to a
		// cleanup job rather than leaking an orphan file.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		if err := storage.DeleteFile(cleanupCtx, stored.FileID); err != nil {
			_ = model.CreateVirtualCharacterCleanupJob(&model.VirtualCharacterCleanupJob{
				CharacterID: character.ID, ProviderAccountID: account.ID, TargetType: "aipdd_file", TargetID: stored.FileID,
			})
		}
	}()
	signed, err := storage.SignFile(ctx, stored.FileID)
	if err != nil {
		return &virtualCharacterImageCreateError{Status: http.StatusBadGateway, Code: "staging_sign_failed", Message: err.Error()}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename))
	}
	providerAssetID, err := client.CreateAsset(ctx, character.ProviderGroupID, signed.URL, "Image", name, account.ProjectName)
	if err != nil {
		return &virtualCharacterImageCreateError{Status: http.StatusBadGateway, Code: "provider_asset_failed", Message: service.LocalizeVolcAssetError(err)}
	}
	var attachErr error
	if character.SourceType == model.VirtualCharacterSourceVolcRealPerson {
		attachErr = model.AttachRealPersonVirtualCharacterImage(character.ID, providerAssetID, stored.FileID, mimeType, header.Size)
	} else {
		attachErr = model.AttachVirtualCharacterImage(character.ID, providerAssetID, stored.FileID, mimeType, header.Size)
	}
	if attachErr != nil {
		_ = model.CreateVirtualCharacterCleanupJob(&model.VirtualCharacterCleanupJob{
			CharacterID: character.ID, ProviderAccountID: account.ID, TargetType: "volc_asset", TargetID: providerAssetID,
		})
		return &virtualCharacterImageCreateError{Status: http.StatusInternalServerError, Code: "asset_save_failed", Message: attachErr.Error()}
	}
	cleanupStaging = false
	return nil
}

func UploadRealPersonVirtualCharacterAsset(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		virtualCharacterError(c, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	characterID, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	character, err := model.GetOwnedVirtualCharacter(characterID, userID)
	if err != nil || character.SourceType != model.VirtualCharacterSourceVolcRealPerson {
		virtualCharacterLookupError(c, gorm.ErrRecordNotFound)
		return
	}
	if character.ValidationStatus != model.VirtualCharacterValidationAccepted || strings.TrimSpace(character.ProviderGroupID) == "" {
		virtualCharacterError(c, http.StatusConflict, "validation_required", "complete real-person identity validation before uploading the portrait asset")
		return
	}
	if character.Status == model.VirtualCharacterStatusDeleting {
		virtualCharacterError(c, http.StatusConflict, "character_deleting", "character is being deleted")
		return
	}
	if strings.TrimSpace(character.ProviderAssetID) != "" {
		virtualCharacterError(c, http.StatusConflict, "asset_already_uploaded", "real-person portrait asset has already been uploaded")
		return
	}
	authorization, err := model.GetVirtualCharacterAuthorization(character.ID)
	if err != nil || authorization.HolderScopeAcceptedAt <= 0 || strings.TrimSpace(authorization.ConsentReceiptHash) == "" ||
		authorization.Status == model.VirtualCharacterAuthorizationExpired || authorization.Status == model.VirtualCharacterAuthorizationRevoked {
		virtualCharacterError(c, http.StatusConflict, "authorization_invalid", "real-person authorization is not valid for asset upload")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, virtualCharacterUploadMaxBody)
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_upload", "invalid or oversized multipart upload")
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	header, err := c.FormFile("file")
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "missing_file", "real-person portrait image is required")
		return
	}
	if _, err := validateVolcCharacterImageUpload(header); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_file", err.Error())
		return
	}
	account, client, err := enabledVirtualCharacterClientForSource(model.VirtualCharacterSourceVolcRealPerson)
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "real_person_disabled", err.Error())
		return
	}
	if account.ID != character.ProviderAccountID {
		virtualCharacterError(c, http.StatusConflict, "provider_account_changed", "the provider account used for identity validation is no longer active")
		return
	}
	assetCount, err := model.CountVirtualCharacterProviderAssets()
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "quota_check_failed", err.Error())
		return
	}
	if assetCount >= int64(model.GetVirtualCharacterAccountAssetCap()) {
		virtualCharacterError(c, http.StatusConflict, "account_asset_cap_reached", "account asset capacity has been reached")
		return
	}
	if createErr := stageAndCreateVirtualCharacterImage(c.Request.Context(), character, account, client, header, character.Name); createErr != nil {
		virtualCharacterError(c, createErr.Status, createErr.Code, createErr.Message)
		return
	}
	character, err = model.GetVirtualCharacterByID(character.ID)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "asset_save_failed", err.Error())
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": virtualCharacterGroupToResponse(character)})
}

func previewVirtualCharacter(c *gin.Context) {
	characterID, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	character, err := model.GetAccessibleVirtualCharacter(characterID, c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	if character.Status == model.VirtualCharacterStatusDeleting {
		virtualCharacterError(c, http.StatusNotFound, "preview_not_found", "character preview is unavailable")
		return
	}
	previewURL := ""
	if character.SourceType == model.VirtualCharacterSourceVolcRealPerson {
		if _, authErr := service.AuthorizeVirtualCharacterForVideo(c.Request.Context(), character, c.GetInt("id")); authErr != nil {
			virtualCharacterError(c, authErr.Status, authErr.Code, authErr.Message)
			return
		}
		asset, assetErr := service.GetRealPersonVirtualCharacterPreviewAsset(c.Request.Context(), character)
		if assetErr != nil {
			virtualCharacterError(c, http.StatusBadGateway, "preview_provider_failed", common.MaskSensitiveInfo(assetErr.Error()))
			return
		}
		if !isTrustedVolcAssetPreviewURL(asset.URL) {
			virtualCharacterError(c, http.StatusBadGateway, "preview_provider_failed", "provider returned an untrusted preview URL")
			return
		}
		previewURL = asset.URL
	} else {
		if strings.TrimSpace(character.StagingFileID) == "" {
			virtualCharacterError(c, http.StatusNotFound, "preview_not_found", "character preview is unavailable")
			return
		}
		storage, storageErr := newVirtualCharacterStagingStorage()
		if storageErr != nil {
			virtualCharacterError(c, http.StatusServiceUnavailable, "storage_unavailable", storageErr.Error())
			return
		}
		signed, signErr := storage.SignFile(c.Request.Context(), character.StagingFileID)
		if signErr != nil {
			virtualCharacterError(c, http.StatusBadGateway, "preview_sign_failed", signErr.Error())
			return
		}
		previewURL = signed.URL
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, previewURL, nil)
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "preview_request_failed", err.Error())
		return
	}
	resp, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "preview_fetch_failed", err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		virtualCharacterError(c, http.StatusBadGateway, "preview_fetch_failed", fmt.Sprintf("storage returned %d", resp.StatusCode))
		return
	}
	maxBytes := virtualCharacterUploadMaxBody
	if resp.ContentLength > maxBytes {
		virtualCharacterError(c, http.StatusBadGateway, "preview_too_large", "stored preview exceeds the size limit")
		return
	}
	c.Header("Content-Type", virtualCharacterPreviewContentType(resp.Header.Get("Content-Type"), character.MimeType))
	c.Header("Cache-Control", "private, max-age=300")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	written, copyErr := io.Copy(c.Writer, io.LimitReader(resp.Body, maxBytes+1))
	if copyErr != nil || written > maxBytes {
		return
	}
}

func isTrustedVolcAssetPreviewURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return strings.HasSuffix(host, ".volces.com") || strings.HasSuffix(host, ".volcengine.com")
}

// virtualCharacterPreviewContentType keeps proxied previews on inert media types.
// Both the upstream header and the stored MIME come from user-controlled uploads,
// so anything outside the asset-type allowlist is served as an opaque download.
func virtualCharacterPreviewContentType(upstream, stored string) string {
	for _, candidate := range []string{upstream, stored} {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if index := strings.Index(candidate, ";"); index >= 0 {
			candidate = strings.TrimSpace(candidate[:index])
		}
		if strings.HasPrefix(candidate, "image/") {
			return candidate
		}
	}
	return "application/octet-stream"
}

func DeleteVirtualCharacterGroup(c *gin.Context) {
	characterID, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	item, err := model.GetOwnedVirtualCharacter(characterID, c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	if item.SourceType == model.VirtualCharacterSourceVolcRealPerson {
		err = model.RevokeRealPersonAuthorization(characterID, c.GetInt("id"), "authorization revoked by owner")
	} else {
		err = model.BeginVirtualCharacterGroupDelete(characterID, c.GetInt("id"))
	}
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"id": characterID, "status": model.VirtualCharacterStatusDeleting})
}

func SyncRealPersonVirtualCharacter(c *gin.Context) {
	characterID, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	item, err := model.GetOwnedVirtualCharacter(characterID, c.GetInt("id"))
	if err != nil || item.SourceType != model.VirtualCharacterSourceVolcRealPerson {
		virtualCharacterLookupError(c, gorm.ErrRecordNotFound)
		return
	}
	item, err = service.SyncRealPersonVirtualCharacter(c.Request.Context(), characterID)
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "real_person_sync_failed", common.MaskSensitiveInfo(err.Error()))
		return
	}
	common.ApiSuccess(c, virtualCharacterGroupToResponse(item))
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
	createAssetQPM := account.EffectiveCreateAssetQPM()
	latest, latestErr := model.GetLatestVirtualCharacterCatalogImport()
	var catalog any
	if latestErr == nil {
		catalog = latest
	}
	common.ApiSuccess(c, gin.H{
		"enabled": account.Enabled, "official_enabled": account.OfficialEnabled,
		"virtual_enabled": account.VirtualEnabled, "real_person_enabled": account.RealPersonEnabled,
		"quota_plan":        account.EffectiveQuotaPlan(),
		"create_asset_qpm":  createAssetQPM,
		"access_key_masked": maskProviderCredential(account.EncryptedAccessKey != ""), "secret_key_masked": maskProviderCredential(account.EncryptedSecretKey != ""),
		"region": account.Region, "project_name": account.ProjectName,
		"crypto_ready": common.HasStableCryptoSecret(), "last_check_status": account.LastCheckStatus, "last_check_error": account.LastCheckError,
		"last_checked_at": account.LastCheckedAt, "catalog": catalog,
		"catalog_last_synced_at": model.GetVirtualCharacterAIPDDCatalogLastSyncAt(),
		"global_limit":           model.GetVirtualCharacterGlobalLimit(),
		"real_person_limit":      model.GetVirtualCharacterRealPersonLimit(),
		"account_asset_cap":      model.GetVirtualCharacterAccountAssetCap(),
	})
}

func AdminSyncVirtualCharacterCatalogFromAIPDD(c *gin.Context) {
	force := parseVirtualCharacterDeclaration(c.Query("force"))
	var body struct {
		Force bool `json:"force"`
	}
	if err := common.DecodeJson(c.Request.Body, &body); err == nil && body.Force {
		force = true
	}
	result, err := model.SyncVirtualCharacterCatalogFromAIPDD(c.Request.Context(), nil, force, c.GetInt("id"))
	if err != nil {
		message := err.Error()
		status := http.StatusBadGateway
		code := "catalog_sync_failed"
		if strings.Contains(message, "AIPDD_API_KEY") {
			status = http.StatusServiceUnavailable
			code = "aipdd_key_missing"
		} else if strings.Contains(message, "not enabled") || strings.Contains(message, "disabled") {
			status = http.StatusServiceUnavailable
			code = "provider_unavailable"
		}
		virtualCharacterError(c, status, code, message)
		return
	}
	common.ApiSuccess(c, result)
}

func AdminUpdateVirtualCharacterABSettings(c *gin.Context) {
	var req struct {
		Enabled           bool   `json:"enabled"`
		QuotaPlan         string `json:"quota_plan"`
		CreateAssetQPM    int    `json:"create_asset_qpm"`
		AccessKey         string `json:"access_key"`
		SecretKey         string `json:"secret_key"`
		Region            string `json:"region"`
		ProjectName       string `json:"project_name"`
		GlobalLimit       int    `json:"global_limit"`
		AccountAssetCap   int    `json:"account_asset_cap"`
		RealPersonEnabled bool   `json:"real_person_enabled"`
		RealPersonLimit   int    `json:"real_person_limit"`
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
	if req.GlobalLimit <= 0 || req.GlobalLimit > 10000 {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_limit", "global limit must be between 1 and 10000")
		return
	}
	if req.RealPersonLimit == 0 {
		req.RealPersonLimit = model.GetVirtualCharacterRealPersonLimit()
	}
	if req.RealPersonLimit < 0 || req.RealPersonLimit > 1000 {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_real_person_limit", "real-person limit must be between 1 and 1000")
		return
	}
	quotaPlan, accountAssetCap, createAssetQPM := model.NormalizeVirtualCharacterQuotaPlan(req.QuotaPlan, req.AccountAssetCap, req.CreateAssetQPM)
	if accountAssetCap <= 0 || accountAssetCap > 5000000 {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_asset_cap", "account asset cap must be between 1 and 5000000")
		return
	}
	if createAssetQPM <= 0 || createAssetQPM > 300 {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_qpm", "create asset QPM must be between 1 and 300")
		return
	}
	account.Enabled = req.Enabled
	account.OfficialEnabled = req.Enabled
	account.VirtualEnabled = req.Enabled
	account.RealPersonEnabled = req.Enabled && req.RealPersonEnabled
	account.ChannelID = 0
	account.QuotaPlan = quotaPlan
	account.CreateAssetQPM = createAssetQPM
	account.Region, account.ProjectName = strings.TrimSpace(req.Region), strings.TrimSpace(req.ProjectName)
	if err := model.SaveVirtualCharacterProviderAccount(account); err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "settings_failed", err.Error())
		return
	}
	for key, value := range map[string]string{
		"VirtualCharacterLimit":           strconv.Itoa(req.GlobalLimit),
		"VirtualCharacterAccountAssetCap": strconv.Itoa(accountAssetCap),
		"VirtualCharacterRealPersonLimit": strconv.Itoa(req.RealPersonLimit),
	} {
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
		if err == nil && account.RealPersonEnabled {
			_, err = client.ListAssetGroupsByType(c.Request.Context(), model.VirtualCharacterRealPersonGroupType, account.ProjectName)
		}
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

func virtualCharacterGroupToResponse(item *model.VirtualCharacter) virtualCharacterGroupResponse {
	response := virtualCharacterGroupResponse{
		ID: item.ID, Scope: item.Scope, SourceType: item.SourceType, Name: item.Name, Description: item.Description,
		Tags: decodeVirtualCharacterTags(item.TagsJSON), Nationality: item.Nationality, Gender: item.Gender,
		AgeMin: item.AgeMin, AgeMax: item.AgeMax, Occupation: item.Occupation, Temperament: item.Temperament,
		Status: item.Status, ValidationStatus: item.ValidationStatus, CoverURL: item.CoverURL,
		ProviderAssetID: item.ProviderAssetID, MimeType: item.MimeType, FileSize: item.FileSize,
		LastError: item.LastError, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, CatalogVersion: item.CatalogVersion,
	}
	response.AssetUploadRequired = item.SourceType == model.VirtualCharacterSourceVolcRealPerson &&
		item.ValidationStatus == model.VirtualCharacterValidationAccepted &&
		strings.TrimSpace(item.ProviderGroupID) != "" && strings.TrimSpace(item.ProviderAssetID) == "" &&
		item.Status != model.VirtualCharacterStatusDeleting
	if item.SourceType == model.VirtualCharacterSourceVolcRealPerson {
		if authorization, err := model.GetVirtualCharacterAuthorization(item.ID); err == nil {
			response.Authorization = virtualCharacterAuthorizationToResponse(authorization)
		}
	}
	return response
}

func virtualCharacterAuthorizationToResponse(item *model.VirtualCharacterAuthorization) *virtualCharacterAuthorizationResponse {
	if item == nil {
		return nil
	}
	return &virtualCharacterAuthorizationResponse{
		Status: item.Status, ProviderGroupStatus: item.ProviderGroupStatus,
		ProviderAssetStatus: item.ProviderAssetStatus, ProviderCheckedAt: item.ProviderCheckedAt,
		AuthorizedAt: item.AuthorizedAt, RevokedAt: item.RevokedAt, LastError: item.LastError,
	}
}

func validationSessionResponse(item *model.VirtualCharacterValidationSession, launchURL string) gin.H {
	return gin.H{"id": item.ID, "status": item.Status, "launch_url": launchURL, "expires_at": item.ExpiresAt, "character_id": item.CharacterID, "last_error": item.LastError, "created_at": item.CreatedAt, "updated_at": item.UpdatedAt}
}

func enabledVirtualCharacterClient(realPerson bool) (*model.VirtualCharacterProviderAccount, service.VolcAssetClient, error) {
	if realPerson {
		return enabledVirtualCharacterClientForSource(model.VirtualCharacterSourceVolcRealPerson)
	}
	return enabledVirtualCharacterClientForSource(model.VirtualCharacterSourceVolcAIGC)
}

// newVolcAssetClientForVirtualCharacters is overridable in tests.
var newVolcAssetClientForVirtualCharacters = service.NewVolcAssetClient

func enabledVirtualCharacterClientForSource(sourceType string) (*model.VirtualCharacterProviderAccount, service.VolcAssetClient, error) {
	if !common.HasStableCryptoSecret() {
		return nil, nil, common.ErrCryptoSecretNotConfigured
	}
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	if err != nil {
		return nil, nil, errors.New("virtual character provider account is not enabled")
	}
	switch sourceType {
	case model.VirtualCharacterSourceVolcAIGC, model.VirtualCharacterSourceVolcPreset:
		// Follow the library master switch (account already required to be enabled).
	case model.VirtualCharacterSourceVolcRealPerson:
		if !account.RealPersonEnabled {
			return nil, nil, errors.New("real-person virtual characters are not enabled")
		}
	default:
		return nil, nil, errors.New("unsupported virtual character source")
	}
	client, err := newVolcAssetClientForVirtualCharacters(account)
	return account, client, err
}

func parseVirtualCharacterCatalog(filename string, payload []byte, fallbackVersion string) (string, []model.VirtualCharacterCatalogEntry, error) {
	type rawEntry struct {
		AssetID     string   `json:"asset_id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		CoverURL    string   `json:"cover_url"`
		Enabled     *bool    `json:"enabled"`
		Nationality string   `json:"nationality"`
		Gender      string   `json:"gender"`
		AgeMin      *int     `json:"age_min"`
		AgeMax      *int     `json:"age_max"`
		Occupation  string   `json:"occupation"`
		Temperament string   `json:"temperament"`
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
			var ageMin, ageMax *int
			if value := read("age_min"); value != "" {
				parsed, parseErr := strconv.Atoi(value)
				if parseErr != nil {
					return "", nil, fmt.Errorf("CSV catalog has invalid age_min: %s", value)
				}
				ageMin = &parsed
			}
			if value := read("age_max"); value != "" {
				parsed, parseErr := strconv.Atoi(value)
				if parseErr != nil {
					return "", nil, fmt.Errorf("CSV catalog has invalid age_max: %s", value)
				}
				ageMax = &parsed
			}
			rawEntries = append(rawEntries, rawEntry{
				AssetID: read("asset_id"), Name: read("name"), Description: read("description"), Tags: tags,
				CoverURL: read("cover_url"), Enabled: &enabled, Nationality: read("nationality"), Gender: read("gender"),
				AgeMin: ageMin, AgeMax: ageMax, Occupation: read("occupation"), Temperament: read("temperament"),
			})
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
		entry := model.VirtualCharacterCatalogEntry{
			AssetID: assetID, Name: metadata.Name, Description: metadata.Description, TagsJSON: tagsJSON,
			CoverURL: cover.String(), Enabled: enabled, Nationality: strings.TrimSpace(raw.Nationality),
			Gender: strings.TrimSpace(raw.Gender), AgeMin: raw.AgeMin, AgeMax: raw.AgeMax,
			Occupation: strings.TrimSpace(raw.Occupation), Temperament: strings.TrimSpace(raw.Temperament),
		}
		model.EnrichVirtualCharacterCatalogEntryFacets(&entry, metadata.Tags)
		if entry.AgeMin != nil && entry.AgeMax != nil && *entry.AgeMin > *entry.AgeMax {
			return "", nil, fmt.Errorf("asset %s has invalid age range", assetID)
		}
		entries = append(entries, entry)
	}
	return version, entries, nil
}

func validateVolcCharacterImageUpload(header *multipart.FileHeader) (string, error) {
	if header == nil || header.Size <= 0 {
		return "", errors.New("file is empty")
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	mimeType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	allowed := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true, ".heic": true}
	if !allowed[ext] {
		return "", errors.New("character image must be JPG, PNG, WebP, GIF, or HEIC")
	}
	if header.Size > 30<<20 {
		return "", errors.New("character image exceeds the 30 MB limit")
	}
	return mimeType, nil
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
	destination := base + "/characters?validation_session=" + url.QueryEscape(item.ID) + "&validation_status=" + url.QueryEscape(item.Status)
	c.Redirect(http.StatusFound, destination)
}

func maskProviderCredential(configured bool) string {
	if !configured {
		return ""
	}
	return "••••••••"
}
