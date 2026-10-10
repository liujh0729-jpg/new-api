package model

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
)

// aipddEndpointCategory is the display metadata of one AIPDD endpoint family.
// The gateway exposes eight families, and both the pricing catalogue and the
// synchronised model table read their tags and descriptions from this one table.
type aipddEndpointCategory struct {
	tag         string
	description string
}

var aipddEndpointCategories = map[constant.EndpointType]aipddEndpointCategory{
	constant.EndpointTypeOpenAI: {
		tag:         "文字模型",
		description: "AIPDD 文字模型，通过 /v1/chat/completions 兼容接口生成文本。",
	},
	constant.EndpointTypeEmbeddings: {
		tag:         "向量模型",
		description: "AIPDD 向量模型，通过 /v1/embeddings 将文本转为向量。",
	},
	constant.EndpointTypeSystemOne: {
		tag:         "决策模型",
		description: "AIPDD 决策模型，通过 /v1/systemone 输出结构化决策结果，按 token 计费。",
	},
	constant.EndpointTypeImageGeneration: {
		tag:         "图片生成",
		description: "AIPDD 图片生成模型，通过 /v1/images/generations 按提示词生成图片。",
	},
	constant.EndpointTypeImageEdits: {
		tag:         "图生图",
		description: "AIPDD 图片编辑模型，通过 /v1/images/edits 上传参考图并按指令修改图片。",
	},
	constant.EndpointTypeOpenAIVideo: {
		tag:         "视频生成",
		description: "AIPDD 视频生成模型，通过 AIPDD 任务接口异步创建并轮询结果。",
	},
	constant.EndpointTypeAudioSpeech: {
		tag:         "语音合成",
		description: "AIPDD 语音合成模型，通过 /v1/audio/speech 将文本合成为音频。",
	},
	constant.EndpointTypeAudioTranscriptions: {
		tag:         "语音识别",
		description: "AIPDD 语音识别模型，通过 /v1/audio/transcriptions 转写上传的音频，按处理时长计费。",
	},
}

const aipddDefaultModelDescription = "AIPDD 上游目录同步模型。"

func aipddEndpointTags(endpointTypes []constant.EndpointType) []string {
	tags := make([]string, 0, len(endpointTypes))
	for _, endpointType := range endpointTypes {
		category, ok := aipddEndpointCategories[endpointType]
		if !ok {
			continue
		}
		if !containsStringValue(tags, category.tag) {
			tags = append(tags, category.tag)
		}
	}
	return tags
}

func aipddEndpointDescription(endpointTypes []constant.EndpointType) string {
	for _, endpointType := range endpointTypes {
		if category, ok := aipddEndpointCategories[endpointType]; ok {
			return category.description
		}
	}
	return aipddDefaultModelDescription
}

func capabilityEndpointTypes(capability constant.AIPDDCapability) []constant.EndpointType {
	if len(capability.EndpointTypes) > 0 {
		return capability.EndpointTypes
	}
	if capability.EndpointType != "" {
		return []constant.EndpointType{capability.EndpointType}
	}
	return nil
}

func aipddCapabilityTags(capability constant.AIPDDCapability) []string {
	tags := []string{"AIPDD"}
	tags = append(tags, aipddEndpointTags(capabilityEndpointTypes(capability))...)
	if capability.Synchronous {
		tags = append(tags, "同步返回")
	} else {
		tags = append(tags, "异步任务")
	}
	switch capability.BillingType {
	case constant.AIPDDBillingTypeDurationSeconds:
		tags = append(tags, "按时长")
	case constant.AIPDDBillingTypePerCall:
		tags = append(tags, "按次")
	}
	return tags
}

func aipddCapabilityDescription(capability constant.AIPDDCapability, upstreamDescription string) string {
	if trimmed := strings.TrimSpace(upstreamDescription); trimmed != "" {
		return trimmed
	}
	return aipddEndpointDescription(capabilityEndpointTypes(capability))
}

func aipddLLMModelTags(model aipddcatalog.AtomicModel, endpointTypes []constant.EndpointType) []string {
	tags := []string{"AIPDD"}
	tags = append(tags, aipddEndpointTags(endpointTypes)...)
	tags = append(tags, "LLM", "OpenAI兼容")
	if containsCatalogValue(model.InputModalities, "image") {
		tags = append(tags, "视觉理解")
	}
	if model.Features != nil && model.Features.FunctionTools.Basic {
		tags = append(tags, "Function Tools")
	}
	return tags
}

func containsStringValue(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
