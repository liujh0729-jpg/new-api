package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

const (
	VirtualCharacterContextKey      = "virtual_character"
	VirtualCharacterAssetContextKey = "virtual_character_asset"
	VirtualCharacterTaskIDKey       = "virtual_character_task_id"
	VirtualCharacterTaskClaimedKey  = "virtual_character_task_claimed"
	virtualCharacterTaskFailureKey  = "virtual_character_task_failure"
)

func BindVirtualCharacter() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req relaycommon.TaskSubmitReq
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if req.CharacterID == nil {
			if req.CharacterAssetID != nil {
				abortVirtualCharacterBinding(c, http.StatusBadRequest, "character_id_required", "character_asset_id requires character_id")
				return
			}
			c.Next()
			return
		}
		if *req.CharacterID <= 0 {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_character_id", "character_id must be a positive integer")
			return
		}
		if req.CharacterAssetID != nil && *req.CharacterAssetID <= 0 {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_character_asset_id", "character_asset_id must be a positive integer")
			return
		}
		if !model.IsVirtualCharacterSeedanceModel(strings.TrimSpace(req.Model)) {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "character_model_not_allowed", "character video requires a Seedance model")
			return
		}
		if virtualCharacterRequestHasExternalReferences(req) {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "character_reference_conflict", "character_id cannot be combined with other reference media")
			return
		}
		item, err := model.GetAccessibleVirtualCharacter(*req.CharacterID, c.GetInt("id"))
		if err != nil {
			abortVirtualCharacterBinding(c, http.StatusNotFound, "character_not_found", "character not found")
			return
		}
		if item.Status != model.VirtualCharacterStatusActive {
			abortVirtualCharacterBinding(c, http.StatusConflict, "character_unavailable", "character is not available for new tasks")
			return
		}
		assetID := int64(0)
		if req.CharacterAssetID != nil {
			assetID = *req.CharacterAssetID
		}
		asset, item, err := model.GetVirtualCharacterAssetForUser(item.ID, assetID, c.GetInt("id"))
		if err != nil {
			abortVirtualCharacterBinding(c, http.StatusNotFound, "character_asset_not_found", "character asset not found")
			return
		}
		if asset.Status != model.VirtualCharacterAssetStatusActive || strings.TrimSpace(asset.ProviderAssetID) == "" {
			abortVirtualCharacterBinding(c, http.StatusConflict, "character_asset_unavailable", "character asset is not active")
			return
		}
		account, err := model.GetEnabledVirtualCharacterProviderAccount()
		if err != nil || !common.HasStableCryptoSecret() || account.ID != item.ProviderAccountID {
			abortVirtualCharacterBinding(c, http.StatusServiceUnavailable, "character_provider_unavailable", "virtual character provider is unavailable")
			return
		}
		switch item.SourceType {
		case model.VirtualCharacterSourceVolcAIGC, model.VirtualCharacterSourceVolcPreset:
			// Official presets and user-created virtual characters follow the library master switch.
		case model.VirtualCharacterSourceVolcRealPerson:
			abortVirtualCharacterBinding(c, http.StatusServiceUnavailable, "character_feature_disabled", "real-person virtual characters are not available yet")
			return
		default:
			abortVirtualCharacterBinding(c, http.StatusServiceUnavailable, "character_feature_disabled", "unsupported virtual character source")
			return
		}

		taskID := model.GenerateTaskID()
		if err := model.CreateVirtualCharacterTaskLink(&model.VirtualCharacterTask{
			TaskID: taskID, UserID: c.GetInt("id"), CharacterID: item.ID,
			CharacterName: item.Name, CharacterScope: item.Scope, CharacterAssetID: asset.ID,
			CharacterAssetName: asset.Name, ProviderAssetID: asset.ProviderAssetID,
		}); err != nil {
			abortVirtualCharacterBinding(c, http.StatusInternalServerError, "character_task_link_failed", err.Error())
			return
		}
		c.Set(VirtualCharacterTaskIDKey, taskID)
		defer func() {
			if c.GetBool(VirtualCharacterTaskClaimedKey) {
				return
			}
			reason := c.GetString(virtualCharacterTaskFailureKey)
			if reason == "" {
				reason = "request did not reach the task relay"
			}
			_ = model.MarkVirtualCharacterTaskFailed(taskID, reason)
		}()

		// Re-check after registering the in-flight task. A delete/offline action
		// that won the race blocks this request; a later action sees the link and
		// waits for the task to reach a terminal state before source cleanup.
		latestAsset, latest, err := model.GetVirtualCharacterAssetForUser(item.ID, asset.ID, c.GetInt("id"))
		if err != nil || latest.Status != model.VirtualCharacterStatusActive || latestAsset.Status != model.VirtualCharacterAssetStatusActive ||
			latest.Scope != item.Scope || latestAsset.ProviderAssetID != asset.ProviderAssetID || (latest.Scope == model.VirtualCharacterScopePrivate && latest.UserID != c.GetInt("id")) {
			abortVirtualCharacterBinding(c, http.StatusConflict, "character_unavailable", "character is not available for new tasks")
			return
		}
		item, asset = latest, latestAsset
		referenceURL := "asset://" + strings.TrimPrefix(strings.TrimSpace(asset.ProviderAssetID), "asset://")

		req.Images = []string{referenceURL}
		req.Image = ""
		req.ImageTail = ""
		req.FirstFrame = ""
		req.LastFrame = ""
		req.InputReference = ""
		payload, err := common.Marshal(req)
		if err != nil {
			abortVirtualCharacterBinding(c, http.StatusInternalServerError, "character_bind_failed", err.Error())
			return
		}
		if err := common.ReplaceRequestBody(c, payload); err != nil {
			abortVirtualCharacterBinding(c, http.StatusInternalServerError, "character_bind_failed", err.Error())
			return
		}
		c.Set(VirtualCharacterContextKey, item)
		c.Set(VirtualCharacterAssetContextKey, asset)
		c.Next()
	}
}

func GetBoundVirtualCharacterAsset(c *gin.Context) (*model.VirtualCharacterAsset, bool) {
	value, exists := c.Get(VirtualCharacterAssetContextKey)
	if !exists {
		return nil, false
	}
	item, ok := value.(*model.VirtualCharacterAsset)
	return item, ok && item != nil
}

func GetBoundVirtualCharacter(c *gin.Context) (*model.VirtualCharacter, bool) {
	value, exists := c.Get(VirtualCharacterContextKey)
	if !exists {
		return nil, false
	}
	item, ok := value.(*model.VirtualCharacter)
	return item, ok && item != nil
}

func GetVirtualCharacterTaskID(c *gin.Context) (string, bool) {
	value := strings.TrimSpace(c.GetString(VirtualCharacterTaskIDKey))
	return value, value != ""
}

func virtualCharacterRequestHasExternalReferences(req relaycommon.TaskSubmitReq) bool {
	if len(req.Images) > 0 || strings.TrimSpace(req.Image) != "" || strings.TrimSpace(req.ImageTail) != "" ||
		strings.TrimSpace(req.FirstFrame) != "" || strings.TrimSpace(req.LastFrame) != "" || strings.TrimSpace(req.InputReference) != "" {
		return true
	}
	if req.Metadata == nil {
		return false
	}
	content, exists := req.Metadata["content"]
	if !exists || content == nil {
		return false
	}
	items, ok := content.([]interface{})
	return !ok || len(items) > 0
}

func abortVirtualCharacterBinding(c *gin.Context, status int, code, message string) {
	if _, exists := c.Get(VirtualCharacterTaskIDKey); exists {
		c.Set(virtualCharacterTaskFailureKey, message)
	}
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message, "type": "invalid_request_error"},
	})
}
