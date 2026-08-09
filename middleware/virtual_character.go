package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	VirtualCharacterContextKey       = "virtual_character"
	VirtualCharacterLockedChannelKey = "virtual_character_locked_channel"
	VirtualCharacterTaskIDKey        = "virtual_character_task_id"
	VirtualCharacterTaskClaimedKey   = "virtual_character_task_claimed"
	virtualCharacterTaskFailureKey   = "virtual_character_task_failure"
)

func BindVirtualCharacter() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req relaycommon.TaskSubmitReq
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		if req.CharacterID == nil {
			c.Next()
			return
		}
		if *req.CharacterID <= 0 {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_character_id", "character_id must be a positive integer")
			return
		}
		if !model.IsVirtualCharacterModelAllowed(strings.TrimSpace(req.Model)) {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "character_model_not_allowed", "the selected model is not enabled for virtual characters")
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
		taskID := model.GenerateTaskID()
		if err := model.CreateVirtualCharacterTaskLink(&model.VirtualCharacterTask{
			TaskID: taskID, UserID: c.GetInt("id"), CharacterID: item.ID,
			CharacterName: item.Name, CharacterScope: item.Scope,
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
		latest, err := model.GetVirtualCharacterByID(item.ID)
		if err != nil || latest.Status != model.VirtualCharacterStatusActive ||
			latest.Scope != item.Scope || (latest.Scope == model.VirtualCharacterScopePrivate && latest.UserID != c.GetInt("id")) {
			abortVirtualCharacterBinding(c, http.StatusConflict, "character_unavailable", "character is not available for new tasks")
			return
		}
		item = latest

		var referenceURL string
		if item.Scope == model.VirtualCharacterScopePublic {
			channel, err := validateVirtualCharacterPublicChannel(item, req.Model)
			if err != nil {
				abortVirtualCharacterBinding(c, http.StatusServiceUnavailable, "public_character_channel_unavailable", err.Error())
				return
			}
			if setupErr := SetupContextForSelectedChannel(c, channel, req.Model); setupErr != nil {
				abortVirtualCharacterBinding(c, http.StatusServiceUnavailable, "public_character_channel_unavailable", setupErr.Error())
				return
			}
			c.Set(VirtualCharacterLockedChannelKey, channel)
			referenceURL = "asset://" + strings.TrimPrefix(strings.TrimSpace(item.VolcAssetID), "asset://")
		} else {
			if strings.TrimSpace(item.AIPDDFileID) == "" {
				abortVirtualCharacterBinding(c, http.StatusConflict, "character_source_unavailable", "character source is unavailable")
				return
			}
			storage, err := service.NewAIPDDVirtualCharacterStorage()
			if err != nil {
				abortVirtualCharacterBinding(c, http.StatusServiceUnavailable, "character_storage_unavailable", err.Error())
				return
			}
			signed, err := storage.SignFile(c.Request.Context(), item.AIPDDFileID)
			if err != nil {
				abortVirtualCharacterBinding(c, http.StatusBadGateway, "character_sign_failed", err.Error())
				return
			}
			referenceURL = signed.URL
		}

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
		c.Next()
	}
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

func validateVirtualCharacterPublicChannel(item *model.VirtualCharacter, modelName string) (*model.Channel, error) {
	channel, err := model.GetChannelById(item.PublicChannelID, true)
	if err != nil {
		return nil, err
	}
	if channel.Status != common.ChannelStatusEnabled {
		return nil, fmt.Errorf("configured public channel is disabled")
	}
	if channel.ChannelInfo.IsMultiKey {
		return nil, fmt.Errorf("configured public channel must use one stable upstream key")
	}
	if channel.Type != constant.ChannelTypeVolcEngine && channel.Type != constant.ChannelTypeDoubaoVideo {
		return nil, fmt.Errorf("configured public channel is not a Volc video channel")
	}
	if !channelSupportsVirtualCharacterModel(channel, modelName) {
		return nil, fmt.Errorf("configured public channel does not support model %s", modelName)
	}
	return channel, nil
}

func channelSupportsVirtualCharacterModel(channel *model.Channel, modelName string) bool {
	for _, supported := range strings.Split(channel.Models, ",") {
		if strings.TrimSpace(supported) == strings.TrimSpace(modelName) {
			return true
		}
	}
	return false
}

func abortVirtualCharacterBinding(c *gin.Context, status int, code, message string) {
	if _, exists := c.Get(VirtualCharacterTaskIDKey); exists {
		c.Set(virtualCharacterTaskFailureKey, message)
	}
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{"code": code, "message": message, "type": "invalid_request_error"},
	})
}
