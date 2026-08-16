package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

const (
	VirtualCharacterContextKey     = "virtual_character"
	VirtualCharacterTaskIDKey      = "virtual_character_task_id"
	VirtualCharacterTaskClaimedKey = "virtual_character_task_claimed"
	virtualCharacterTaskFailureKey = "virtual_character_task_failure"
)

func BindVirtualCharacter() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		var req relaycommon.TaskSubmitReq
		if err := common.UnmarshalBodyReusable(c, &req); err != nil {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		raw := make(map[string]interface{})
		if strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "application/json") {
			if err := common.UnmarshalBodyReusable(c, &raw); err != nil {
				abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_request", err.Error())
				return
			}
		}
		assetIDs, err := virtualCharacterAssetReferenceIDs(req, raw)
		if err != nil {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_asset_reference", err.Error())
			return
		}
		if req.CharacterID == nil && len(assetIDs) == 0 {
			c.Next()
			return
		}
		if !model.IsVirtualCharacterSeedanceModel(req.Model) {
			abortVirtualCharacterBinding(c, http.StatusBadRequest, "character_model_not_allowed", "character video requires a model name containing Seedance")
			return
		}
		userID := c.GetInt("id")
		characters := make([]*model.VirtualCharacter, 0, len(assetIDs)+1)
		snapshots := make(map[int64]string, len(assetIDs)+1)
		var item *model.VirtualCharacter
		if req.CharacterID != nil {
			if *req.CharacterID <= 0 {
				abortVirtualCharacterBinding(c, http.StatusBadRequest, "invalid_character_id", "character_id must be a positive integer")
				return
			}
			if virtualCharacterRequestHasExternalReferences(req) || virtualCharacterRawContentHasReference(raw) {
				abortVirtualCharacterBinding(c, http.StatusBadRequest, "character_reference_conflict", "character_id cannot be combined with other reference media")
				return
			}
			item, err = model.GetAccessibleVirtualCharacter(*req.CharacterID, userID)
			if err != nil {
				abortVirtualCharacterBinding(c, http.StatusNotFound, "character_not_found", "character not found")
				return
			}
			snapshot, authErr := service.AuthorizeVirtualCharacterForVideo(c.Request.Context(), item, userID)
			if authErr != nil {
				abortVirtualCharacterBinding(c, authErr.Status, authErr.Code, authErr.Message)
				return
			}
			characters = append(characters, item)
			snapshots[item.ID] = snapshot
			referenceURL := "asset://" + strings.TrimPrefix(strings.TrimSpace(item.ProviderAssetID), "asset://")
			req.Images = []string{referenceURL}
			req.Image = ""
			req.ImageTail = ""
			req.FirstFrame = ""
			req.LastFrame = ""
			req.InputReference = ""
			payload, marshalErr := common.Marshal(req)
			if marshalErr != nil {
				abortVirtualCharacterBinding(c, http.StatusInternalServerError, "character_bind_failed", marshalErr.Error())
				return
			}
			if replaceErr := common.ReplaceRequestBody(c, payload); replaceErr != nil {
				abortVirtualCharacterBinding(c, http.StatusInternalServerError, "character_bind_failed", replaceErr.Error())
				return
			}
			c.Request.Header.Set("Content-Type", "application/json")
		} else {
			for _, assetID := range assetIDs {
				candidate, lookupErr := model.GetRegisteredVirtualCharacterByProviderAssetID(assetID)
				if lookupErr != nil {
					abortVirtualCharacterBinding(c, http.StatusNotFound, "asset_reference_not_registered", "asset reference is not registered in the character library")
					return
				}
				snapshot, authErr := service.AuthorizeVirtualCharacterForVideo(c.Request.Context(), candidate, userID)
				if authErr != nil {
					abortVirtualCharacterBinding(c, authErr.Status, authErr.Code, authErr.Message)
					return
				}
				characters = append(characters, candidate)
				snapshots[candidate.ID] = snapshot
			}
			item = characters[0]
		}

		taskID := model.GenerateTaskID()
		if err := model.CreateVirtualCharacterTaskBinding(&model.VirtualCharacterTask{
			TaskID: taskID, UserID: userID, CharacterID: item.ID,
			CharacterName: item.Name, CharacterScope: item.Scope, ProviderAssetID: item.ProviderAssetID,
			AuthorizationSnapshotJSON: snapshots[item.ID],
		}, characters, snapshots); err != nil {
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
		for index, original := range characters {
			latest, lookupErr := model.GetVirtualCharacterByID(original.ID)
			if lookupErr != nil || latest.ProviderAssetID != original.ProviderAssetID || latest.SourceType != original.SourceType {
				_ = model.RollbackVirtualCharacterTaskBinding(taskID)
				abortVirtualCharacterBinding(c, http.StatusConflict, "character_unavailable", "character is not available for new tasks")
				return
			}
			if _, authErr := service.AuthorizeVirtualCharacterForVideo(c.Request.Context(), latest, userID); authErr != nil {
				_ = model.RollbackVirtualCharacterTaskBinding(taskID)
				abortVirtualCharacterBinding(c, authErr.Status, authErr.Code, authErr.Message)
				return
			}
			characters[index] = latest
			if index == 0 {
				item = latest
			}
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
	switch content.(type) {
	case []interface{}, map[string]interface{}:
	default:
		return true
	}
	return virtualCharacterContentHasReference(content)
}

func virtualCharacterAssetReferenceIDs(req relaycommon.TaskSubmitReq, raw map[string]interface{}) ([]string, error) {
	values := make([]string, 0, len(req.Images)+8)
	values = append(values, req.Images...)
	values = append(values, req.Image, req.ImageTail, req.FirstFrame, req.LastFrame, req.InputReference)
	if req.Metadata != nil {
		collectVirtualCharacterAssetReferenceValues(req.Metadata["content"], &values)
	}
	for _, key := range []string{"image", "image_tail", "first_frame", "last_frame", "input_reference", "images"} {
		collectVirtualCharacterAssetReferenceValues(raw[key], &values)
	}
	collectVirtualCharacterAssetReferenceValues(raw["content"], &values)
	collectVirtualCharacterAssetReferenceValues(raw["metadata"], &values)
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !strings.HasPrefix(strings.ToLower(value), "asset://") {
			continue
		}
		assetID := strings.TrimSpace(value[len("asset://"):])
		if assetID == "" || strings.ContainsAny(assetID, " \t\r\n?#/") {
			return nil, &virtualCharacterAssetReferenceError{value: value}
		}
		if _, exists := seen[assetID]; exists {
			continue
		}
		seen[assetID] = struct{}{}
		result = append(result, assetID)
	}
	return result, nil
}

type virtualCharacterAssetReferenceError struct{ value string }

func (e *virtualCharacterAssetReferenceError) Error() string {
	return "invalid asset reference: " + e.value
}

func collectVirtualCharacterAssetReferenceValues(value interface{}, result *[]string) {
	switch typed := value.(type) {
	case string:
		*result = append(*result, typed)
	case []string:
		*result = append(*result, typed...)
	case []interface{}:
		for _, item := range typed {
			collectVirtualCharacterAssetReferenceValues(item, result)
		}
	case map[string]interface{}:
		for key, item := range typed {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "url", "image_url", "video_url", "input_reference", "image", "images", "source":
				collectVirtualCharacterAssetReferenceValues(item, result)
			case "content":
				collectVirtualCharacterAssetReferenceValues(item, result)
			}
		}
	}
}

func virtualCharacterRawContentHasReference(raw map[string]interface{}) bool {
	if raw == nil {
		return false
	}
	if virtualCharacterContentHasReference(raw["content"]) {
		return true
	}
	if metadata, ok := raw["metadata"].(map[string]interface{}); ok {
		return virtualCharacterContentHasReference(metadata)
	}
	return false
}

func virtualCharacterContentHasReference(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case []interface{}:
		for _, item := range typed {
			if virtualCharacterContentHasReference(item) {
				return true
			}
		}
		return false
	case map[string]interface{}:
		if blockType, ok := typed["type"].(string); ok {
			blockType = strings.ToLower(strings.TrimSpace(blockType))
			if strings.Contains(blockType, "image") || strings.Contains(blockType, "video") || strings.Contains(blockType, "audio") {
				return true
			}
		}
		for key, item := range typed {
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "url", "image_url", "video_url", "input_reference", "image", "images", "source":
				if item != nil {
					return true
				}
			case "content":
				if virtualCharacterContentHasReference(item) {
					return true
				}
			}
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
