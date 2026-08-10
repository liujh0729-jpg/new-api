package controller

import (
	"context"
	"encoding/csv"
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
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	virtualCharacterImageMaxBytes = int64(30 << 20)
	virtualCharacterNameMaxRunes  = 191
	virtualCharacterDescMaxRunes  = 2000
	virtualCharacterTagMaxCount   = 20
	virtualCharacterTagMaxRunes   = 32
	virtualCharacterImportMax     = int64(4 << 20)
)

type virtualCharacterResponse struct {
	ID               int64    `json:"id"`
	Scope            string   `json:"scope"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Tags             []string `json:"tags"`
	Status           string   `json:"status"`
	ValidationStatus string   `json:"validation_status"`
	CoverURL         string   `json:"cover_url"`
	MimeType         string   `json:"mime_type,omitempty"`
	FileSize         int64    `json:"file_size,omitempty"`
	CreatedAt        int64    `json:"created_at"`
	UpdatedAt        int64    `json:"updated_at"`
	LastError        string   `json:"last_error,omitempty"`
}

type virtualCharacterMetadataRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type publicVirtualCharacterRequest struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Tags            []string `json:"tags"`
	CoverURL        string   `json:"cover_url"`
	AssetID         string   `json:"asset_id"`
	PublicChannelID int      `json:"public_channel_id"`
	Status          string   `json:"status"`
}

func ListVirtualCharacters(c *gin.Context) {
	userID := c.GetInt("id")
	scope := strings.TrimSpace(c.DefaultQuery("scope", model.VirtualCharacterScopePrivate))
	if scope != model.VirtualCharacterScopePrivate && scope != model.VirtualCharacterScopePublic {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_scope", "scope must be private or public")
		return
	}
	page := getVirtualCharacterPage(c)
	items, total, err := model.ListVirtualCharacters(userID, scope, false, model.VirtualCharacterListFilter{}, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	responses := make([]virtualCharacterResponse, 0, len(items))
	for i := range items {
		responses = append(responses, virtualCharacterToResponse(&items[i]))
	}
	page.SetTotal(int(total))
	page.SetItems(responses)
	data := gin.H{"page": page}
	if scope == model.VirtualCharacterScopePrivate {
		used, countErr := model.CountActivePrivateVirtualCharacters(userID)
		if countErr != nil {
			virtualCharacterError(c, http.StatusInternalServerError, "count_failed", countErr.Error())
			return
		}
		data["used"] = used
		data["limit"] = model.GetVirtualCharacterEffectiveLimit(userID)
	}
	common.ApiSuccess(c, data)
}

func GetVirtualCharacter(c *gin.Context) {
	item, err := getAccessibleVirtualCharacterParam(c, c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	common.ApiSuccess(c, virtualCharacterToResponse(item))
}

func UploadVirtualCharacter(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		virtualCharacterError(c, http.StatusUnauthorized, "unauthorized", "authentication is required")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, virtualCharacterImageMaxBytes+(1<<20))
	// Keep the accepted image in memory until it is streamed to AIPDD. A lower
	// multipart threshold would create a temporary local material file.
	if err := c.Request.ParseMultipartForm(virtualCharacterImageMaxBytes + (1 << 20)); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_multipart", "invalid upload form or request is too large")
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if !parseVirtualCharacterDeclaration(c.PostForm("non_real_person")) {
		virtualCharacterError(c, http.StatusBadRequest, "declaration_required", "confirm that the image depicts a non-real fictional character")
		return
	}
	metadata, tagsJSON, err := normalizeVirtualCharacterMetadata(virtualCharacterMetadataRequest{
		Name:        c.PostForm("name"),
		Description: c.PostForm("description"),
		Tags:        parseVirtualCharacterTags(c.PostForm("tags")),
	})
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_metadata", err.Error())
		return
	}
	header, err := c.FormFile("file")
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "file_required", "image file is required")
		return
	}
	mimeType, err := validateVirtualCharacterUpload(header)
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_image", err.Error())
		return
	}

	item, limit, err := model.ReservePrivateVirtualCharacter(userID, metadata.Name, metadata.Description, tagsJSON, mimeType, header.Size)
	if err != nil {
		if strings.Contains(err.Error(), "limit reached") {
			virtualCharacterError(c, http.StatusConflict, "limit_reached", fmt.Sprintf("private character limit reached (%d)", limit))
			return
		}
		virtualCharacterError(c, http.StatusInternalServerError, "reserve_failed", err.Error())
		return
	}

	storage, err := service.NewAIPDDVirtualCharacterStorage()
	if err != nil {
		_ = model.BeginVirtualCharacterDelete(item, err.Error())
		virtualCharacterError(c, http.StatusServiceUnavailable, "storage_unavailable", err.Error())
		return
	}
	file, err := header.Open()
	if err != nil {
		_ = model.BeginVirtualCharacterDelete(item, err.Error())
		virtualCharacterError(c, http.StatusBadRequest, "open_file_failed", err.Error())
		return
	}
	uploaded, uploadErr := storage.UploadPrivateFile(c.Request.Context(), header.Filename, io.LimitReader(file, virtualCharacterImageMaxBytes+1))
	_ = file.Close()
	if uploadErr != nil {
		_ = model.BeginVirtualCharacterDelete(item, uploadErr.Error())
		virtualCharacterError(c, http.StatusBadGateway, "upload_failed", uploadErr.Error())
		return
	}
	if err := model.MarkVirtualCharacterStorage(item.ID, uploaded.FileID, 0); err != nil {
		cleanupAIPDDVirtualCharacter(storage, 0, uploaded.FileID)
		_ = model.BeginVirtualCharacterDelete(item, err.Error())
		virtualCharacterError(c, http.StatusInternalServerError, "persist_upload_failed", err.Error())
		return
	}
	asset, err := storage.CreateDigitalAsset(c.Request.Context(), metadata.Name, uploaded.FileID, header.Size)
	if err != nil {
		_ = model.BeginVirtualCharacterDelete(item, err.Error())
		virtualCharacterError(c, http.StatusBadGateway, "asset_create_failed", err.Error())
		return
	}
	if err := model.MarkVirtualCharacterStorage(item.ID, uploaded.FileID, asset.ID); err != nil {
		cleanupAIPDDVirtualCharacter(storage, asset.ID, uploaded.FileID)
		_ = model.BeginVirtualCharacterDelete(item, err.Error())
		virtualCharacterError(c, http.StatusInternalServerError, "persist_asset_failed", err.Error())
		return
	}
	if err := model.ActivateVirtualCharacter(item.ID); err != nil {
		_ = model.BeginVirtualCharacterDelete(item, err.Error())
		virtualCharacterError(c, http.StatusInternalServerError, "activate_failed", err.Error())
		return
	}
	item, err = model.GetOwnedVirtualCharacter(item.ID, userID)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "read_after_create_failed", err.Error())
		return
	}
	common.ApiSuccess(c, virtualCharacterToResponse(item))
}

func UpdateVirtualCharacter(c *gin.Context) {
	item, err := getOwnedVirtualCharacterParam(c, c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	if item.Status == model.VirtualCharacterStatusDeleting {
		virtualCharacterError(c, http.StatusConflict, "character_deleting", "character is being deleted")
		return
	}
	var req virtualCharacterMetadataRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	metadata, tagsJSON, err := normalizeVirtualCharacterMetadata(req)
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_metadata", err.Error())
		return
	}
	if err := model.UpdateVirtualCharacterMetadata(item, metadata.Name, metadata.Description, tagsJSON); err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}
	if strings.TrimSpace(item.ProviderGroupID) != "" {
		if account, client, clientErr := enabledVirtualCharacterClientForSource(item.SourceType); clientErr == nil && account.ID == item.ProviderAccountID {
			_ = client.UpdateAssetGroup(c.Request.Context(), item.ProviderGroupID, metadata.Name, metadata.Description, account.ProjectName)
		}
	}
	item.Name, item.Description, item.TagsJSON = metadata.Name, metadata.Description, tagsJSON
	item.UpdatedAt = time.Now().Unix()
	response, responseErr := virtualCharacterGroupToResponse(item, true)
	if responseErr != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "update_failed", responseErr.Error())
		return
	}
	common.ApiSuccess(c, response)
}

func DeleteVirtualCharacter(c *gin.Context) {
	item, err := getOwnedVirtualCharacterParam(c, c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	if item.Status != model.VirtualCharacterStatusDeleting {
		if err := model.BeginVirtualCharacterDelete(item, ""); err != nil {
			virtualCharacterError(c, http.StatusInternalServerError, "delete_failed", err.Error())
			return
		}
	}
	common.ApiSuccess(c, gin.H{"id": item.ID, "status": model.VirtualCharacterStatusDeleting})
}

func PreviewVirtualCharacter(c *gin.Context) {
	item, err := getAccessibleVirtualCharacterParam(c, c.GetInt("id"))
	if err != nil {
		virtualCharacterLookupError(c, err)
		return
	}
	if item.Scope != model.VirtualCharacterScopePrivate || strings.TrimSpace(item.AIPDDFileID) == "" || item.Status == model.VirtualCharacterStatusDeleting {
		virtualCharacterError(c, http.StatusNotFound, "preview_not_found", "character preview is unavailable")
		return
	}
	storage, err := service.NewAIPDDVirtualCharacterStorage()
	if err != nil {
		virtualCharacterError(c, http.StatusServiceUnavailable, "storage_unavailable", err.Error())
		return
	}
	signed, err := storage.SignFile(c.Request.Context(), item.AIPDDFileID)
	if err != nil {
		virtualCharacterError(c, http.StatusBadGateway, "preview_sign_failed", err.Error())
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, signed.URL, nil)
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
	if resp.ContentLength > virtualCharacterImageMaxBytes {
		virtualCharacterError(c, http.StatusBadGateway, "preview_too_large", "stored preview exceeds 30 MB")
		return
	}
	mimeType := item.MimeType
	if value := strings.TrimSpace(resp.Header.Get("Content-Type")); value != "" {
		mimeType = value
	}
	c.Header("Content-Type", mimeType)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	written, copyErr := io.Copy(c.Writer, io.LimitReader(resp.Body, virtualCharacterImageMaxBytes+1))
	if copyErr != nil || written > virtualCharacterImageMaxBytes {
		return
	}
}

func GetVirtualCharacterTaskHistory(c *gin.Context) {
	page := getVirtualCharacterPage(c)
	cutoff := time.Now().Add(-90 * 24 * time.Hour).Unix()
	links, total, err := model.ListVirtualCharacterTaskLinks(c.GetInt("id"), page.GetStartIdx(), page.GetPageSize(), cutoff)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "history_failed", err.Error())
		return
	}
	taskIDs := make([]string, 0, len(links))
	for i := range links {
		taskIDs = append(taskIDs, links[i].TaskID)
	}
	taskMap, err := model.GetVirtualCharacterTasksByIDs(c.GetInt("id"), taskIDs)
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "history_failed", err.Error())
		return
	}
	items := make([]gin.H, 0, len(links))
	for i := range links {
		link := &links[i]
		entry := gin.H{
			"task_id": link.TaskID, "character_id": link.CharacterID, "character_name": link.CharacterName,
			"character_scope": link.CharacterScope, "character_asset_id": link.CharacterAssetID,
			"character_asset_name": link.CharacterAssetName, "provider_asset_id": link.ProviderAssetID,
			"link_status": link.Status, "created_at": link.CreatedAt,
		}
		if task, exists := taskMap[link.TaskID]; exists {
			entry["task"] = relayTaskToSafeDTO(task)
		} else if link.LastError != "" {
			entry["error"] = link.LastError
		}
		items = append(items, entry)
	}
	page.SetTotal(int(total))
	page.SetItems(items)
	common.ApiSuccess(c, gin.H{
		"page":           page,
		"retention_days": 90,
		"output_notice":  "Generated video URLs are temporary upstream results and may expire after about 24 hours.",
	})
}

func AdminListVirtualCharacters(c *gin.Context) {
	page := getVirtualCharacterPage(c)
	items, total, err := model.ListVirtualCharacters(0, model.VirtualCharacterScopePublic, true, model.VirtualCharacterListFilter{}, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	responses := make([]gin.H, 0, len(items))
	for i := range items {
		responses = append(responses, adminVirtualCharacterToResponse(&items[i]))
	}
	page.SetTotal(int(total))
	page.SetItems(responses)
	common.ApiSuccess(c, page)
}

func AdminCreateVirtualCharacter(c *gin.Context) {
	var req publicVirtualCharacterRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := normalizePublicVirtualCharacter(req)
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_public_character", err.Error())
		return
	}
	if err := model.CreatePublicVirtualCharacter(item); err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "create_failed", err.Error())
		return
	}
	common.ApiSuccess(c, adminVirtualCharacterToResponse(item))
}

func AdminUpdateVirtualCharacter(c *gin.Context) {
	id, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	existing, err := model.GetVirtualCharacterByID(id)
	if err != nil || existing.Scope != model.VirtualCharacterScopePublic {
		virtualCharacterLookupError(c, err)
		return
	}
	var req publicVirtualCharacterRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	item, err := normalizePublicVirtualCharacter(req)
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_public_character", err.Error())
		return
	}
	item.ID = id
	item.Scope = model.VirtualCharacterScopePublic
	if err := model.UpdatePublicVirtualCharacter(item); err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "update_failed", err.Error())
		return
	}
	common.ApiSuccess(c, adminVirtualCharacterToResponse(item))
}

func AdminDeleteVirtualCharacter(c *gin.Context) {
	id, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}
	if err := model.DeletePublicVirtualCharacter(id); err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "delete_failed", err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"id": id, "status": model.VirtualCharacterStatusOffline})
}

func AdminImportVirtualCharacters(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, virtualCharacterImportMax+(1<<20))
	header, err := c.FormFile("file")
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "file_required", "JSON or CSV import file is required")
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if header.Size <= 0 || header.Size > virtualCharacterImportMax {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_import", "import file must be between 1 byte and 4 MB")
		return
	}
	file, err := header.Open()
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_import", err.Error())
		return
	}
	defer file.Close()
	defaultChannelID, _ := strconv.Atoi(c.PostForm("public_channel_id"))
	records, err := parsePublicVirtualCharacterImport(header.Filename, file, defaultChannelID)
	if err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_import", err.Error())
		return
	}
	if len(records) > 2000 {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_import", "an import may contain at most 2000 records")
		return
	}
	items := make([]*model.VirtualCharacter, 0, len(records))
	for index, req := range records {
		item, normalizeErr := normalizePublicVirtualCharacter(req)
		if normalizeErr != nil {
			virtualCharacterError(c, http.StatusBadRequest, "invalid_import", fmt.Sprintf("row %d: %v", index+1, normalizeErr))
			return
		}
		items = append(items, item)
	}
	if err := model.UpsertPublicVirtualCharacters(items); err != nil {
		virtualCharacterError(c, http.StatusInternalServerError, "import_failed", err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"processed": len(items)})
}

func AdminSetVirtualCharacterUserLimit(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userID <= 0 {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_user", "invalid user id")
		return
	}
	if _, err := model.GetUserById(userID, false); err != nil {
		virtualCharacterError(c, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	var req struct {
		Limit int `json:"limit"`
	}
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if err := model.SetVirtualCharacterUserLimit(userID, req.Limit); err != nil {
		virtualCharacterError(c, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{"user_id": userID, "limit": model.GetVirtualCharacterEffectiveLimit(userID), "overridden": req.Limit > 0})
}

func virtualCharacterToResponse(item *model.VirtualCharacter) virtualCharacterResponse {
	response := virtualCharacterResponse{
		ID: item.ID, Scope: item.Scope, Name: item.Name, Description: item.Description,
		Tags: decodeVirtualCharacterTags(item.TagsJSON), Status: item.Status, ValidationStatus: item.ValidationStatus,
		MimeType: item.MimeType, FileSize: item.FileSize, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		LastError: item.LastError,
	}
	if item.Scope == model.VirtualCharacterScopePublic {
		response.CoverURL = item.CoverURL
	} else if item.Status != model.VirtualCharacterStatusDeleting && strings.TrimSpace(item.AIPDDFileID) != "" {
		response.CoverURL = "/api/virtual-characters/" + strconv.FormatInt(item.ID, 10) + "/preview"
	}
	return response
}

func adminVirtualCharacterToResponse(item *model.VirtualCharacter) gin.H {
	response := virtualCharacterToResponse(item)
	payload := gin.H{
		"id": response.ID, "scope": response.Scope, "name": response.Name,
		"description": response.Description, "tags": response.Tags, "status": response.Status,
		"validation_status": response.ValidationStatus, "cover_url": response.CoverURL,
		"mime_type": response.MimeType, "file_size": response.FileSize, "created_at": response.CreatedAt,
		"updated_at": response.UpdatedAt, "asset_id": item.VolcAssetID,
		"public_channel_id": item.PublicChannelID,
	}
	if response.LastError != "" {
		payload["last_error"] = response.LastError
	}
	return payload
}

func relayTaskToSafeDTO(task *model.Task) any {
	if task == nil {
		return nil
	}
	return tasksToDto([]*model.Task{task}, false)[0]
}

func normalizeVirtualCharacterMetadata(req virtualCharacterMetadataRequest) (virtualCharacterMetadataRequest, string, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" || utf8.RuneCountInString(req.Name) > virtualCharacterNameMaxRunes {
		return req, "", fmt.Errorf("name is required and must not exceed %d characters", virtualCharacterNameMaxRunes)
	}
	if utf8.RuneCountInString(req.Description) > virtualCharacterDescMaxRunes {
		return req, "", fmt.Errorf("description must not exceed %d characters", virtualCharacterDescMaxRunes)
	}
	tags, err := normalizeVirtualCharacterTags(req.Tags)
	if err != nil {
		return req, "", err
	}
	req.Tags = tags
	payload, err := common.Marshal(tags)
	return req, string(payload), err
}

func normalizeVirtualCharacterTags(tags []string) ([]string, error) {
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{})
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > virtualCharacterTagMaxRunes {
			return nil, fmt.Errorf("each tag must not exceed %d characters", virtualCharacterTagMaxRunes)
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
		if len(result) > virtualCharacterTagMaxCount {
			return nil, fmt.Errorf("at most %d tags are allowed", virtualCharacterTagMaxCount)
		}
	}
	return result, nil
}

func parseVirtualCharacterTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var tags []string
	if strings.HasPrefix(raw, "[") && common.UnmarshalJsonStr(raw, &tags) == nil {
		return tags
	}
	return strings.Split(raw, ",")
}

func decodeVirtualCharacterTags(raw string) []string {
	var tags []string
	if common.UnmarshalJsonStr(raw, &tags) != nil || tags == nil {
		return []string{}
	}
	return tags
}

func validateVirtualCharacterUpload(header *multipart.FileHeader) (string, error) {
	if header == nil || header.Size <= 0 {
		return "", errors.New("image file is empty")
	}
	if header.Size > virtualCharacterImageMaxBytes {
		return "", errors.New("image file must not exceed 30 MB")
	}
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()
	probe := make([]byte, 512)
	n, err := io.ReadFull(file, probe)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	mimeType := http.DetectContentType(probe[:n])
	allowed := map[string]bool{"image/jpeg": true, "image/png": true, "image/webp": true}
	if !allowed[mimeType] {
		return "", errors.New("only JPG, PNG, and WebP images are supported")
	}
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		return "", errors.New("image filename must end in .jpg, .jpeg, .png, or .webp")
	}
	return mimeType, nil
}

func normalizePublicVirtualCharacter(req publicVirtualCharacterRequest) (*model.VirtualCharacter, error) {
	metadata, tagsJSON, err := normalizeVirtualCharacterMetadata(virtualCharacterMetadataRequest{Name: req.Name, Description: req.Description, Tags: req.Tags})
	if err != nil {
		return nil, err
	}
	assetID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(req.AssetID), "asset://"))
	if assetID == "" || strings.ContainsAny(assetID, "\r\n\t ") {
		return nil, errors.New("a valid Volc asset ID is required")
	}
	coverURL := strings.TrimSpace(req.CoverURL)
	parsed, err := url.Parse(coverURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, errors.New("cover_url must be a public HTTP(S) URL")
	}
	channel, err := model.GetChannelById(req.PublicChannelID, false)
	if err != nil {
		return nil, errors.New("public channel not found")
	}
	if channel.Status != common.ChannelStatusEnabled || channel.ChannelInfo.IsMultiKey ||
		(channel.Type != constant.ChannelTypeVolcEngine && channel.Type != constant.ChannelTypeDoubaoVideo) {
		return nil, errors.New("public channel must be an enabled single-key VolcEngine or DoubaoVideo channel")
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = model.VirtualCharacterStatusActive
	}
	if status != model.VirtualCharacterStatusActive && status != model.VirtualCharacterStatusOffline {
		return nil, errors.New("status must be active or offline")
	}
	return &model.VirtualCharacter{
		Name: metadata.Name, Description: metadata.Description, TagsJSON: tagsJSON, CoverURL: coverURL,
		VolcAssetID: assetID, PublicChannelID: req.PublicChannelID, Status: status,
	}, nil
}

func parsePublicVirtualCharacterImport(filename string, reader io.Reader, defaultChannelID int) ([]publicVirtualCharacterRequest, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".json" {
		payload, err := io.ReadAll(io.LimitReader(reader, virtualCharacterImportMax+1))
		if err != nil {
			return nil, err
		}
		if int64(len(payload)) > virtualCharacterImportMax {
			return nil, errors.New("import file exceeds 4 MB")
		}
		var records []publicVirtualCharacterRequest
		if err := common.Unmarshal(payload, &records); err != nil {
			return nil, err
		}
		for i := range records {
			if records[i].PublicChannelID == 0 {
				records[i].PublicChannelID = defaultChannelID
			}
		}
		return records, nil
	}
	if ext != ".csv" {
		return nil, errors.New("import file must be .json or .csv")
	}
	csvReader := csv.NewReader(io.LimitReader(reader, virtualCharacterImportMax+1))
	csvReader.ReuseRecord = false
	rows, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, errors.New("CSV must include a header and at least one row")
	}
	header := make(map[string]int)
	for index, value := range rows[0] {
		header[strings.ToLower(strings.TrimSpace(value))] = index
	}
	required := []string{"name", "cover_url", "asset_id"}
	for _, name := range required {
		if _, exists := header[name]; !exists {
			return nil, fmt.Errorf("CSV is missing %s column", name)
		}
	}
	valueAt := func(row []string, name string) string {
		index, exists := header[name]
		if !exists || index >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[index])
	}
	records := make([]publicVirtualCharacterRequest, 0, len(rows)-1)
	for _, row := range rows[1:] {
		channelID := defaultChannelID
		if value := valueAt(row, "public_channel_id"); value != "" {
			channelID, _ = strconv.Atoi(value)
		}
		records = append(records, publicVirtualCharacterRequest{
			Name: valueAt(row, "name"), Description: valueAt(row, "description"), Tags: parseVirtualCharacterTags(valueAt(row, "tags")),
			CoverURL: valueAt(row, "cover_url"), AssetID: valueAt(row, "asset_id"), PublicChannelID: channelID, Status: valueAt(row, "status"),
		})
	}
	return records, nil
}

func getAccessibleVirtualCharacterParam(c *gin.Context, userID int) (*model.VirtualCharacter, error) {
	id, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		return nil, err
	}
	return model.GetAccessibleVirtualCharacter(id, userID)
}

func getOwnedVirtualCharacterParam(c *gin.Context, userID int) (*model.VirtualCharacter, error) {
	id, err := parseVirtualCharacterID(c.Param("id"))
	if err != nil {
		return nil, err
	}
	return model.GetOwnedVirtualCharacter(id, userID)
}

func parseVirtualCharacterID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid character id")
	}
	return id, nil
}

func virtualCharacterLookupError(c *gin.Context, err error) {
	if err == nil || errors.Is(err, gorm.ErrRecordNotFound) {
		virtualCharacterError(c, http.StatusNotFound, "character_not_found", "character not found")
		return
	}
	virtualCharacterError(c, http.StatusInternalServerError, "character_lookup_failed", err.Error())
}

func virtualCharacterError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{"success": false, "message": message, "error": gin.H{"code": code, "message": message}})
}

func parseVirtualCharacterDeclaration(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "true" || value == "1" || value == "yes"
}

func cleanupAIPDDVirtualCharacter(storage *service.AIPDDVirtualCharacterStorage, assetID int64, fileID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	_ = storage.DeleteDigitalAsset(ctx, assetID)
	_ = storage.DeleteFile(ctx, fileID)
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func getVirtualCharacterPage(c *gin.Context) *common.PageInfo {
	page := common.GetPageQuery(c)
	if page.Page < 1 {
		page.Page = 1
	}
	if page.PageSize < 1 {
		page.PageSize = 20
	}
	return page
}
