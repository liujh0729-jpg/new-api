package model

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/seedancepublic"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

func IsSeedanceTask(task *Task) bool {
	return task != nil && (seedancepublic.IsModel(task.Properties.OriginModelName) || seedancepublic.IsModel(task.Properties.UpstreamModelName) || (task.PrivateData.AIPDDExecution != nil && task.PrivateData.AIPDDExecution.Protocol == "seedance_official"))
}
func SeedanceVideoURL(task *Task) string {
	if task == nil {
		return ""
	}
	if value := strings.TrimSpace(task.GetResultURL()); value != "" {
		return value
	}
	var data struct {
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		ContentURL string `json:"content_url"`
	}
	if common.Unmarshal(task.Data, &data) != nil {
		return ""
	}
	if data.Content.VideoURL != "" {
		return data.Content.VideoURL
	}
	return data.ContentURL
}
func PublicTaskVideoURL(task *Task) string {
	if task == nil || task.Status != TaskStatusSuccess || SeedanceVideoURL(task) == "" {
		return ""
	}
	return strings.TrimRight(system_setting.ServerAddress, "/") + "/v1/videos/" + url.PathEscape(task.TaskID) + "/content"
}
func PublicSeedanceResponse(task *Task, data []byte, openAI bool) ([]byte, error) {
	data, err := seedancepublic.Media(data, openAI, PublicTaskVideoURL(task), "", task.PrivateData.Key)
	if err != nil {
		return nil, err
	}
	var payload map[string]json.RawMessage
	if err := common.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	payload["id"], _ = common.Marshal(task.TaskID)
	if openAI {
		payload["task_id"], _ = common.Marshal(task.TaskID)
	}
	if task.Properties.OriginModelName != "" {
		payload["model"], _ = common.Marshal(seedancepublic.Text(task.Properties.OriginModelName, "video"))
	} else {
		delete(payload, "model")
	}
	return common.Marshal(payload)
}
