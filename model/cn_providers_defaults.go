package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"gorm.io/gorm"
)

type cnProviderInfo struct {
	ChannelType int
	Name        string
	Description string
	Icon        string
	Website     string
	BaseURL     string
	Models      []cnModelInfo
}

type cnModelInfo struct {
	ModelName     string
	Description   string
	Tags          string
	EndpointTypes []constant.EndpointType
}

var cnProviders = []cnProviderInfo{
	{
		ChannelType: constant.ChannelTypeAli,
		Name:        "阿里云百炼 (DashScope)",
		Description: "阿里云通义千问大模型系列，覆盖文本生成、全模态理解、图像视频生成、语音合成等能力。",
		Icon:        "Qwen.Color",
		Website:     "https://bailian.console.aliyun.com",
		Models: []cnModelInfo{
			{
				ModelName:   "qwen3.7-plus",
				Description: "通义千问3.7 Plus，能力与成本均衡，1M上下文，支持思考模式、Function Calling、结构化输出。",
				Tags:        "通义千问,文本生成,多模态,1M上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "qwen3.6-flash",
				Description: "通义千问3.6 Flash，轻量低成本旗舰模型，1M上下文，效果接近旗舰，性价比极高。",
				Tags:        "通义千问,文本生成,轻量,1M上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "qwen3.7-max",
				Description: "通义千问3.7 Max，最强推理模型，1M上下文，支持思考模式、Function Calling。",
				Tags:        "通义千问,文本生成,深度推理,1M上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "qwen3.5-omni-plus",
				Description: "通义千问3.5 Omni Plus，全模态模型，支持文本、图像、音频、视频的理解与生成。",
				Tags:        "通义千问,全模态,语音,视频,图片理解",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "qwen-image-2.0-pro",
				Description: "通义千问图像生成2.0 Pro，支持高精度文生图和参考图生成。",
				Tags:        "通义千问,图片生成,文生图",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeImageGeneration,
				},
			},
			{
				ModelName:   "wan2.7-image-pro",
				Description: "万相2.7图片生成Pro，新一代图片生成模型，支持文生图、参考图生成。",
				Tags:        "万相,图片生成,视频生成",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeImageGeneration,
				},
			},
			{
				ModelName:   "text-embedding-v4",
				Description: "阿里云文本向量化模型V4，用于文本语义检索和向量化存储。",
				Tags:        "Embedding,向量化",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeEmbeddings,
				},
			},
		},
	},
	{
		ChannelType: constant.ChannelTypeZhipu_v4,
		Name:        "智谱 AI (Zhipu/GLM)",
		Description: "智谱GLM大模型系列，覆盖文本生成、视觉推理、图像视频生成、音视频处理等能力。",
		Icon:        "Zhipu.Color",
		Website:     "https://open.bigmodel.cn",
		Models: []cnModelInfo{
			{
				ModelName:   "glm-5.2",
				Description: "智谱GLM-5.2旗舰模型，1M上下文，面向长周期Agent任务、工程项目级编码与复杂多步执行。",
				Tags:        "智谱,GLM,旗舰,文本生成,编程,1M上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "glm-5.1",
				Description: "智谱GLM-5.1高智能模型，200K上下文，面向通用Agent、工程交付和复杂推理任务。",
				Tags:        "智谱,GLM,文本生成,Agent,编程,200K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "glm-5-turbo",
				Description: "智谱GLM-5-Turbo，高性价比主流模型，适合日常对话、代码辅助、工具调用和长文本场景。",
				Tags:        "智谱,GLM,文本生成,轻量,200K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "glm-5v-turbo",
				Description: "智谱GLM-5V多模态Coding基座，200K上下文，兼顾视觉理解与Coding能力，复杂视觉推理。",
				Tags:        "智谱,GLM,视觉,多模态,编程,200K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "glm-image",
				Description: "智谱GLM-Image旗舰图像生成模型，文字渲染开源SOTA，支持多分辨率，汉字渲染尤其出色。",
				Tags:        "智谱,图片生成,文生图",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeImageGeneration,
				},
			},
		},
	},
	{
		ChannelType: constant.ChannelTypeDeepSeek,
		Name:        "DeepSeek (深度求索)",
		Description: "DeepSeek大模型系列，最新V4系列支持思考模式，1M上下文，编码与推理能力顶尖。",
		Icon:        "DeepSeek.Color",
		Website:     "https://platform.deepseek.com",
		Models: []cnModelInfo{
			{
				ModelName:   "deepseek-v4-pro",
				Description: "DeepSeek V4 Pro，顶级推理与编码模型，1M上下文，支持思考与非思考两种模式。缓存命中¥0.026/1M tokens。",
				Tags:        "DeepSeek,深度推理,编程,1M上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "deepseek-v4-flash",
				Description: "DeepSeek V4 Flash，轻量高速模型，1M上下文，支持思考与非思考两种模式，适合高吞吐Agent与通用任务。",
				Tags:        "DeepSeek,文本生成,轻量,1M上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
		},
	},
	{
		ChannelType: constant.ChannelTypeMoonshot,
		Name:        "Moonshot AI (月之暗面/Kimi)",
		Description: "Kimi大模型系列，最新K2.6多模态模型，支持文本、图片与视频输入，256K上下文。",
		Icon:        "Moonshot",
		Website:     "https://platform.moonshot.cn",
		Models: []cnModelInfo{
			{
				ModelName:   "kimi-k2.7-code",
				Description: "Kimi K2.7 Code，Kimi最新主力Coding模型，支持文本、图片、视频输入，256K上下文，适合Agent与代码任务。",
				Tags:        "Kimi,Coding,Agent,多模态,256K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "kimi-k2.7-code-highspeed",
				Description: "Kimi K2.7 Code高速版，与K2.7 Code同能力但输出速度更快，适合低时延编码和Agent场景。",
				Tags:        "Kimi,Coding,Agent,高速,多模态,256K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "kimi-k2.6",
				Description: "Kimi K2.6，多模态主力模型，支持文本、图片与视频输入，256K上下文，支持思考与非思考模式。",
				Tags:        "Kimi,多模态,视觉,视频,256K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
		},
	},
	{
		ChannelType: constant.ChannelTypeMiniMax,
		Name:        "MiniMax (稀宇科技)",
		Description: "MiniMax大模型系列，覆盖文本生成、语音合成、图片生成，新一代M2.7旗舰模型。",
		Icon:        "Minimax.Color",
		Website:     "https://platform.minimaxi.com",
		Models: []cnModelInfo{
			{
				ModelName:   "MiniMax-M3",
				Description: "MiniMax M3，最新M系列主力模型，1M上下文，面向Agent推理、工具调用、编码和长上下文任务。",
				Tags:        "MiniMax,旗舰,文本生成,Agent,Coding,1M上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "MiniMax-M2.7",
				Description: "MiniMax M2.7，主流高性能文本模型，适合工程任务、办公交付、角色互动和复杂推理。",
				Tags:        "MiniMax,文本生成,Agent,Coding",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "MiniMax-M2.7-highspeed",
				Description: "MiniMax M2.7 Highspeed，高速版主流文本模型，适合低时延Agent和实时交互。",
				Tags:        "MiniMax,文本生成,Agent,Coding,高速",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "speech-2.8-hd",
				Description: "MiniMax Speech 2.8 HD，最新高质量语音合成模型，支持自然语音和声音标签。",
				Tags:        "MiniMax,TTS,语音合成,高质量",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeAudioSpeech,
				},
			},
			{
				ModelName:   "speech-2.8-turbo",
				Description: "MiniMax Speech 2.8 Turbo，最新低时延语音合成模型，适合实时语音和高并发场景。",
				Tags:        "MiniMax,TTS,语音合成",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeAudioSpeech,
				},
			},
		},
	},
	{
		ChannelType: constant.ChannelTypeOpenAI,
		Name:        "腾讯混元 (Hunyuan)",
		Description: "腾讯混元TokenHub主流模型，覆盖文本生成、深度思考、Function Calling和长上下文能力。",
		Icon:        "Hunyuan.Color",
		Website:     "https://cloud.tencent.com/product/tokenhub",
		BaseURL:     "https://tokenhub.tencentmaas.com/v1",
		Models: []cnModelInfo{
			{
				ModelName:   "hy3-preview",
				Description: "腾讯混元Hy3 Preview，TokenHub主流模型，支持深度思考、结构化输出、Function Calling和Cache缓存。",
				Tags:        "腾讯,混元,深度思考,Function Calling,256K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
		},
	},
	{
		ChannelType: constant.ChannelTypeVolcEngine,
		Name:        "字节跳动豆包 (Doubao/火山引擎)",
		Description: "字节跳动豆包大模型系列，覆盖文本生成、图片生成、视频生成、向量化等能力。",
		Icon:        "Doubao.Color",
		Website:     "https://console.volcengine.com/ark",
		Models: []cnModelInfo{
			{
				ModelName:   "doubao-seed-evolving",
				Description: "Doubao Seed Evolving，持续进化的深度思考模型，通过统一Model ID获取最新版本，适合Coding、Agent与复杂任务编排。",
				Tags:        "豆包,Seed,持续进化,深度思考,Coding,Agent",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "doubao-seed-2-1-pro-260628",
				Description: "豆包Seed 2.1 Pro，最新旗舰深度思考模型，256K上下文，面向复杂Coding、长链路Agent和多模态理解。",
				Tags:        "豆包,Seed 2.1,旗舰,深度思考,Coding,Agent,256K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "doubao-seed-2-1-turbo-260628",
				Description: "豆包Seed 2.1 Turbo，低成本低时延深度思考模型，适合规模化生产、高吞吐调用和通用Agent场景。",
				Tags:        "豆包,Seed 2.1,轻量,深度思考,Coding,Agent,低时延",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "doubao-seed-character-260628",
				Description: "豆包Seed Character，面向泛娱乐场景的角色模型，支持自然对话、剧情推理、情感递进和多模态理解。",
				Tags:        "豆包,角色模型,深度思考,多模态,Function Calling",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "doubao-seedream-5-0-260128",
				Description: "豆包Seedream 5.0 Lite，最新图像生成模型，支持文生图、图生图、参考图生成和高分辨率输出。",
				Tags:        "豆包,Seedream 5.0,图片生成,文生图,图生图",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeImageGeneration,
				},
			},
			{
				ModelName:   "doubao-seedream-5-0-lite-260128",
				Description: "豆包Seedream 5.0 Lite兼容模型ID，支持文生图、图生图、参考图生成和高分辨率输出。",
				Tags:        "豆包,Seedream 5.0,图片生成,文生图,图生图",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeImageGeneration,
				},
			},
			{
				ModelName:   "doubao-seedance-2-0-260128",
				Description: "豆包Seedance 2.0，最新视频生成模型，支持文生视频、图生视频、多参考媒体与声画生成。",
				Tags:        "豆包,Seedance 2.0,视频生成,文生视频,图生视频",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAIVideo,
				},
			},
			{
				ModelName:   "doubao-seedance-2-0-fast-260128",
				Description: "豆包Seedance 2.0 Fast，低时延视频生成模型，适合更快的视频任务提交与生产场景。",
				Tags:        "豆包,Seedance 2.0,视频生成,低时延",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAIVideo,
				},
			},
			{
				ModelName:   "doubao-embedding-vision-251215",
				Description: "豆包Embedding Vision，多模态向量化模型，支持视频、文本、图片输入和语义检索。",
				Tags:        "豆包,Embedding,多模态,向量化,检索",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeEmbeddings,
				},
			},
		},
	},
}

func EnsureCNProviderDefaults() error {
	changed := false
	for _, provider := range cnProviders {
		vendorID, vendorChanged, err := ensureCNVendor(provider)
		if err != nil {
			return err
		}
		changed = changed || vendorChanged

		for _, m := range provider.Models {
			modelChanged, err := ensureCNModel(vendorID, provider, m)
			if err != nil {
				return err
			}
			changed = changed || modelChanged
		}

		channelChanged, err := ensureCNChannel(provider)
		if err != nil {
			return err
		}
		changed = changed || channelChanged
	}

	if changed {
		InvalidatePricingCache()
	}
	return nil
}

func ensureCNVendor(info cnProviderInfo) (int, bool, error) {
	var vendor Vendor
	err := DB.Where("name = ?", info.Name).First(&vendor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		vendor = Vendor{
			Name:        info.Name,
			Description: info.Description,
			Icon:        info.Icon,
			Website:     info.Website,
			Status:      1,
		}
		if err := vendor.Insert(); err != nil {
			return 0, false, err
		}
		return vendor.Id, true, nil
	}
	if err != nil {
		return 0, false, err
	}

	updates := map[string]interface{}{}
	if shouldReplaceDefaultIcon(vendor.Icon, info.Icon) {
		updates["icon"] = info.Icon
	}
	if strings.TrimSpace(vendor.Website) == "" && info.Website != "" {
		updates["website"] = info.Website
	}
	if strings.TrimSpace(vendor.Description) == "" && info.Description != "" {
		updates["description"] = info.Description
	}
	if len(updates) > 0 {
		if err := DB.Model(&Vendor{}).Where("id = ?", vendor.Id).Updates(updates).Error; err != nil {
			return 0, false, err
		}
		return vendor.Id, true, nil
	}
	return vendor.Id, false, nil
}

func ensureCNModel(vendorID int, info cnProviderInfo, m cnModelInfo) (bool, error) {
	endpoints := marshalEndpointTypes(m.EndpointTypes)

	var item Model
	err := DB.Where("model_name = ?", m.ModelName).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item = Model{
			ModelName:    m.ModelName,
			Description:  m.Description,
			Icon:         info.Icon,
			Tags:         m.Tags,
			VendorID:     vendorID,
			Endpoints:    endpoints,
			Status:       1,
			SyncOfficial: 1,
			NameRule:     NameRuleExact,
		}
		if err := item.Insert(); err != nil {
			return false, err
		}
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if item.SyncOfficial == 0 {
		return false, nil
	}

	updates := map[string]interface{}{}
	if item.VendorID == 0 {
		updates["vendor_id"] = vendorID
	}
	if strings.TrimSpace(item.Description) == "" {
		updates["description"] = m.Description
	}
	if shouldReplaceDefaultIcon(item.Icon, info.Icon) {
		updates["icon"] = info.Icon
	}
	if strings.TrimSpace(item.Tags) == "" {
		updates["tags"] = m.Tags
	}
	if strings.TrimSpace(item.Endpoints) == "" {
		updates["endpoints"] = endpoints
	}
	if len(updates) == 0 {
		return false, nil
	}
	return true, DB.Model(&Model{}).Where("id = ?", item.Id).Updates(updates).Error
}

func ensureCNChannel(info cnProviderInfo) (bool, error) {
	baseURL := constant.ChannelBaseURLs[info.ChannelType]
	if strings.TrimSpace(info.BaseURL) != "" {
		baseURL = strings.TrimSpace(info.BaseURL)
	}
	modelNames := make([]string, 0, len(info.Models))
	for _, m := range info.Models {
		modelNames = append(modelNames, m.ModelName)
	}
	modelsStr := strings.Join(modelNames, ",")

	var channel Channel
	err := DB.Where("type = ? AND name = ?", info.ChannelType, info.Name).First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		channel = Channel{
			Type:        info.ChannelType,
			Name:        info.Name,
			Key:         "",
			Status:      common.ChannelStatusEnabled,
			Group:       "default",
			BaseURL:     &baseURL,
			Models:      modelsStr,
			CreatedTime: common.GetTimestamp(),
		}
		if err := channel.Insert(); err != nil {
			return false, err
		}
		common.SysLog("CN provider channel created: " + info.Name)
		return true, nil
	}
	if err != nil {
		return false, err
	}

	updates := map[string]interface{}{}
	if strings.TrimSpace(channel.Models) == "" {
		updates["models"] = modelsStr
	}
	if strings.TrimSpace(channel.Group) == "" {
		updates["group"] = "default"
	}
	if channel.Status != common.ChannelStatusEnabled {
		updates["status"] = common.ChannelStatusEnabled
	}
	if channel.BaseURL == nil || strings.TrimSpace(*channel.BaseURL) == "" {
		if baseURL != "" {
			updates["base_url"] = baseURL
			channel.BaseURL = &baseURL
		}
	}
	if len(updates) == 0 {
		return false, nil
	}
	if err := DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
		return false, err
	}
	if _, hasModels := updates["models"]; hasModels {
		if err := channel.UpdateAbilities(nil); err != nil {
			return false, err
		}
	}
	common.SysLog("CN provider channel updated: " + info.Name)
	return true, nil
}
