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
			ModelName:   "glm-5.1",
			Description: "智谱最新旗舰模型，开源SOTA能力，200K上下文，Coding对齐Claude Opus 4.6，擅长长程任务。",
			Tags:        "智谱,GLM,旗舰,文本生成,编程,200K上下文",
			EndpointTypes: []constant.EndpointType{
				constant.EndpointTypeOpenAI,
			},
		},
		{
			ModelName:   "glm-5",
			Description: "智谱GLM-5高智能基座，200K上下文，编程能力对齐Claude Opus 4.5，擅长Agentic长程规划。",
			Tags:        "智谱,GLM,文本生成,Agent,编程,200K上下文",
			EndpointTypes: []constant.EndpointType{
				constant.EndpointTypeOpenAI,
			},
		},
		{
			ModelName:   "glm-4.7",
			Description: "智谱GLM-4.7高智能模型，200K上下文，通用对话、推理与智能体能力全面升级。",
			Tags:        "智谱,GLM,文本生成,编程,200K上下文",
			EndpointTypes: []constant.EndpointType{
				constant.EndpointTypeOpenAI,
			},
		},
		{
			ModelName:   "glm-4.7-flashx",
			Description: "智谱GLM-4.7 FlashX轻量高速模型，小尺寸强能力，200K上下文，适合中文写作、翻译、长文本场景。",
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
				Description: "DeepSeek V4 Flash，轻量高速模型，1M上下文，支持思考与非思考两种模式。deepseek-chat/reasoner将于2026.7.24弃用。",
				Tags:        "DeepSeek,文本生成,轻量,1M上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "deepseek-chat",
				Description: "DeepSeek V4 Flash非思考模式兼容名，将于2026.7.24弃用，请迁移至 deepseek-v4-flash。",
				Tags:        "DeepSeek,文本生成,兼容,即将弃用",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "deepseek-reasoner",
				Description: "DeepSeek V4 Flash思考模式兼容名，将于2026.7.24弃用，请迁移至 deepseek-v4-flash。",
				Tags:        "DeepSeek,深度推理,兼容,即将弃用",
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
				ModelName:   "kimi-k2.6",
				Description: "Kimi K2.6，最新最智能多模态模型，支持文本、图片与视频输入，256K上下文，支持思考与非思考模式。",
				Tags:        "Kimi,多模态,视觉,视频,256K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "kimi-k2-thinking",
				Description: "Kimi K2 Thinking，深度推理模型，256K上下文，擅长复杂推理和深度分析。",
				Tags:        "Kimi,深度推理,编程,256K上下文",
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
				ModelName:   "MiniMax-M2.7",
				Description: "MiniMax M2.7 最新旗舰文本模型，综合能力全面提升，支持思考模式。",
				Tags:        "MiniMax,旗舰,文本生成",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "MiniMax-M2.5",
				Description: "MiniMax M2.5 高性能文本模型，平衡能力与成本，支持思考模式。",
				Tags:        "MiniMax,文本生成",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "speech-2.5-hd-preview",
				Description: "MiniMax语音合成2.5 HD预览版，基于先进TTS技术，支持高质量语音合成。",
				Tags:        "MiniMax,TTS,语音合成",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeAudioSpeech,
				},
			},
		},
	},
	{
		ChannelType: constant.ChannelTypeLingYiWanWu,
		Name:        "零一万物 (01.AI/Yi)",
		Description: "零一万物Yi大模型系列，覆盖文本生成、视觉理解等多模态能力。",
		Icon:        "Yi.Color",
		Website:     "https://platform.lingyiwanwu.com",
		Models: []cnModelInfo{
			{
				ModelName:   "yi-lightning",
				Description: "Yi Lightning，极速推理模型，低延迟高吞吐，适合高并发实时场景。",
				Tags:        "零一万物,Yi,文本生成,极速",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "yi-large",
				Description: "Yi Large，旗舰推理模型，强大推理与知识能力，适合复杂任务。",
				Tags:        "零一万物,Yi,旗舰,文本生成,深度推理",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "yi-vision",
				Description: "Yi Vision，多模态视觉模型，支持图片理解和视觉推理。",
				Tags:        "零一万物,Yi,视觉,多模态",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "yi-medium",
				Description: "Yi Medium，中等尺寸模型，能力与成本平衡，适合通用场景。",
				Tags:        "零一万物,Yi,文本生成",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
		},
	},
	{
		ChannelType: constant.ChannelTypeTencent,
		Name:        "腾讯混元 (Hunyuan)",
		Description: "腾讯混元大模型系列，覆盖文本生成、视觉理解、图像生成、翻译等能力。",
		Icon:        "Hunyuan.Color",
		Website:     "https://cloud.tencent.com/product/hunyuan",
		Models: []cnModelInfo{
			{
				ModelName:   "hunyuan-2.0-thinking-20251109",
				Description: "混元2.0 Think，基于混元2.0的深度推理模型，128K输入，64K输出，擅长复杂指令遵循和代码推理。",
				Tags:        "腾讯,混元,深度推理,编程,128K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "hunyuan-2.0-instruct-20251111",
				Description: "混元2.0 Instruct，基于混元2.0的指令遵循模型，128K输入，16K输出，能力全面提升。",
				Tags:        "腾讯,混元,文本生成,128K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "hunyuan-t1-latest",
				Description: "混元T1最新推理模型，业内首个Hybrid-Transformer-Mamba架构，扩展推理能力，超强解码速度。",
				Tags:        "腾讯,混元,深度推理,32K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "hunyuan-vision-1.5-instruct",
				Description: "混元Vision 1.5，图生文理解模型，24K输入，16K输出，图片识别、分析推理能力显著提升。",
				Tags:        "腾讯,混元,视觉,多模态,图片理解",
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
				ModelName:   "Doubao-pro-32k",
				Description: "豆包Pro 32K，旗舰文本生成模型，32K上下文，综合能力强劲。",
				Tags:        "豆包,文本生成,32K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "Doubao-lite-32k",
				Description: "豆包Lite 32K，轻量文本生成模型，32K上下文，成本低廉，适合高并发场景。",
				Tags:        "豆包,文本生成,轻量,32K上下文",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "doubao-seed-1-6-thinking-250715",
				Description: "豆包Seed 1.6 Thinking，深度推理模型，支持思考模式，擅长复杂数理推理和代码。",
				Tags:        "豆包,深度推理,编程",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAI,
				},
			},
			{
				ModelName:   "doubao-seedream-5.0-lite",
				Description: "豆包Seedream 5.0 Lite，图像生成模型，支持多种高分辨率输出，文生图和参考图生成。",
				Tags:        "豆包,图片生成,文生图",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeImageGeneration,
				},
			},
			{
				ModelName:   "doubao-seedance-1-0-pro-250528",
				Description: "豆包Seedance 1.0 Pro，视频生成模型，支持文生视频和参考图生成视频。",
				Tags:        "豆包,视频生成,文生视频",
				EndpointTypes: []constant.EndpointType{
					constant.EndpointTypeOpenAIVideo,
				},
			},
			{
				ModelName:   "Doubao-embedding",
				Description: "豆包向量化模型，用于文本语义检索和向量化存储。",
				Tags:        "豆包,Embedding,向量化",
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
