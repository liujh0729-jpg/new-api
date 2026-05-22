package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

type defaultCatalogModel struct {
	ModelName     string
	VendorName    string
	Description   string
	Icon          string
	Tags          string
	ChannelType   int
	EndpointTypes []constant.EndpointType
}

const (
	aipddLogoPath       = constant.AIPDDLogoPath
	legacyAIPDDLogoIcon = "OpenAI"
)

var defaultCatalogModels = []defaultCatalogModel{
	{
		ModelName:     constant.AIPDDModelFluxGGUF,
		VendorName:    "AIPDD",
		Description:   "基于 AIPDD ComfyUI 工作流的 Flux 生图能力，适合根据文本提示词生成图片。通过图片生成接口创建异步任务，任务完成后返回图片结果。",
		Icon:          aipddLogoPath,
		Tags:          "AIPDD,ComfyUI,图片生成,异步任务",
		ChannelType:   constant.ChannelTypeAIPDD,
		EndpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration},
	},
	{
		ModelName:     constant.AIPDDModelWan22Wanx,
		VendorName:    "AIPDD",
		Description:   "基于 Wan2.2 的图生视频能力，输入参考图片和提示词生成短视频。通过视频任务接口异步创建并轮询结果，按生成时长计费：5 秒 0.1 元，10 秒 0.2 元。",
		Icon:          aipddLogoPath,
		Tags:          "AIPDD,ComfyUI,图生视频,按时长,异步任务",
		ChannelType:   constant.ChannelTypeAIPDD,
		EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
	},
	{
		ModelName:     constant.AIPDDModelWan22Animater,
		VendorName:    "AIPDD",
		Description:   "基于 Wan2.2 Animater 的主体替换能力，输入源视频、主体素材和提示词，生成主体替换后的视频结果。通过视频任务接口异步创建，按次计费。",
		Icon:          aipddLogoPath,
		Tags:          "AIPDD,ComfyUI,主体替换,视频生成,按次,异步任务",
		ChannelType:   constant.ChannelTypeAIPDD,
		EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
	},
	{
		ModelName:     constant.AIPDDModelMimicMotion,
		VendorName:    "AIPDD",
		Description:   "基于 MimicMotion 的动作迁移能力，输入动作视频和目标外观图片，将参考动作迁移到目标主体上。通过视频任务接口异步创建，按次计费。",
		Icon:          aipddLogoPath,
		Tags:          "AIPDD,ComfyUI,动作迁移,视频生成,按次,异步任务",
		ChannelType:   constant.ChannelTypeAIPDD,
		EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
	},
	{
		ModelName:     constant.AIPDDModelLatentsync15,
		VendorName:    "AIPDD",
		Description:   "基于 Latentsync 的视频对口型能力，输入视频和音频，让人物口型与音频内容同步生成新视频。通过视频任务接口异步创建，按次计费。",
		Icon:          aipddLogoPath,
		Tags:          "AIPDD,ComfyUI,对口型,视频生成,音频驱动,按次,异步任务",
		ChannelType:   constant.ChannelTypeAIPDD,
		EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
	},
	{
		ModelName:     constant.AIPDDModelIndexTTS,
		VendorName:    "AIPDD",
		Description:   "基于 IndexTTS 的声音复刻能力，输入参考音频和待合成文本，生成接近参考音色的语音结果。通过音频任务接口异步创建，按次计费。",
		Icon:          aipddLogoPath,
		Tags:          "AIPDD,ComfyUI,声音复刻,语音合成,按次,异步任务",
		ChannelType:   constant.ChannelTypeAIPDD,
		EndpointTypes: []constant.EndpointType{constant.EndpointTypeAudioSpeech},
	},
}

// 简化的供应商映射规则
var defaultVendorRules = map[string]string{
	"gpt":      "OpenAI",
	"dall-e":   "OpenAI",
	"whisper":  "OpenAI",
	"o1":       "OpenAI",
	"o3":       "OpenAI",
	"claude":   "Anthropic",
	"gemini":   "Google",
	"moonshot": "Moonshot",
	"kimi":     "Moonshot",
	"chatglm":  "智谱",
	"glm-":     "智谱",
	"qwen":     "阿里巴巴",
	"deepseek": "DeepSeek",
	"abab":     "MiniMax",
	"ernie":    "百度",
	"spark":    "讯飞",
	"hunyuan":  "腾讯",
	"command":  "Cohere",
	"@cf/":     "Cloudflare",
	"360":      "360",
	"yi":       "零一万物",
	"jina":     "Jina",
	"mistral":  "Mistral",
	"grok":     "xAI",
	"llama":    "Meta",
	"doubao":   "字节跳动",
	"kling":    "快手",
	"jimeng":   "即梦",
	"vidu":     "Vidu",
	"aipdd":    "AIPDD",
}

// 供应商默认图标映射
var defaultVendorIcons = map[string]string{
	"OpenAI":     "OpenAI",
	"Anthropic":  "Claude.Color",
	"Google":     "Gemini.Color",
	"Moonshot":   "Moonshot",
	"智谱":         "Zhipu.Color",
	"阿里巴巴":       "Qwen.Color",
	"DeepSeek":   "DeepSeek.Color",
	"MiniMax":    "Minimax.Color",
	"百度":         "Wenxin.Color",
	"讯飞":         "Spark.Color",
	"腾讯":         "Hunyuan.Color",
	"Cohere":     "Cohere.Color",
	"Cloudflare": "Cloudflare.Color",
	"360":        "Ai360.Color",
	"零一万物":       "Yi.Color",
	"Jina":       "Jina",
	"Mistral":    "Mistral.Color",
	"xAI":        "XAI",
	"Meta":       "Ollama",
	"字节跳动":       "Doubao.Color",
	"快手":         "Kling.Color",
	"即梦":         "Jimeng.Color",
	"Vidu":       "Vidu",
	"微软":         "AzureAI",
	"Microsoft":  "AzureAI",
	"Azure":      "AzureAI",
	"AIPDD":      aipddLogoPath,
}

// initDefaultVendorMapping 简化的默认供应商映射
func initDefaultVendorMapping(metaMap map[string]*Model, vendorMap map[int]*Vendor, enableAbilities []AbilityWithChannel) {
	ensureDefaultVendorIcons(vendorMap)

	for _, ability := range enableAbilities {
		modelName := ability.Model
		if meta, exists := metaMap[modelName]; exists {
			if catalog, ok := defaultCatalogModelByName(modelName); ok {
				ensureDefaultCatalogModelIcon(meta, catalog)
			}
			continue
		}

		if catalog, ok := defaultCatalogModelByName(modelName); ok {
			vendorID := getOrCreateVendor(catalog.VendorName, vendorMap)
			metaMap[modelName] = &Model{
				ModelName:   modelName,
				Description: catalog.Description,
				Icon:        catalog.Icon,
				Tags:        catalog.Tags,
				VendorID:    vendorID,
				Endpoints:   marshalEndpointTypes(catalog.EndpointTypes),
				Status:      1,
				NameRule:    NameRuleExact,
			}
			continue
		}

		// 匹配供应商
		vendorID := 0
		modelLower := strings.ToLower(modelName)
		for pattern, vendorName := range defaultVendorRules {
			if strings.Contains(modelLower, pattern) {
				vendorID = getOrCreateVendor(vendorName, vendorMap)
				break
			}
		}

		// 创建模型元数据
		metaMap[modelName] = &Model{
			ModelName: modelName,
			VendorID:  vendorID,
			Status:    1,
			NameRule:  NameRuleExact,
		}
	}
}

func ensureDefaultVendorIcons(vendorMap map[int]*Vendor) {
	for _, vendor := range vendorMap {
		if vendor.Name != "AIPDD" {
			continue
		}

		updates := map[string]interface{}{}
		icon := getDefaultVendorIcon(vendor.Name)
		if shouldReplaceDefaultIcon(vendor.Icon, icon) {
			vendor.Icon = icon
			updates["icon"] = icon
		}
		website := getDefaultVendorWebsite(vendor.Name)
		if strings.TrimSpace(vendor.Website) == "" && website != "" {
			vendor.Website = website
			updates["website"] = website
		}
		if len(updates) > 0 {
			_ = DB.Model(&Vendor{}).Where("id = ?", vendor.Id).Updates(updates).Error
		}
	}
}

func ensureDefaultCatalogModelIcon(meta *Model, catalog defaultCatalogModel) {
	if shouldReplaceDefaultIcon(meta.Icon, catalog.Icon) {
		meta.Icon = catalog.Icon
		if meta.Id > 0 {
			_ = DB.Model(&Model{}).Where("id = ?", meta.Id).Update("icon", catalog.Icon).Error
		}
	}
}

func shouldReplaceDefaultIcon(current string, desired string) bool {
	current = strings.TrimSpace(current)
	desired = strings.TrimSpace(desired)
	if desired == "" || current == desired {
		return false
	}
	return current == "" || current == legacyAIPDDLogoIcon
}

// 查找或创建供应商
func getOrCreateVendor(vendorName string, vendorMap map[int]*Vendor) int {
	// 查找现有供应商
	for id, vendor := range vendorMap {
		if vendor.Name == vendorName {
			return id
		}
	}

	// 创建新供应商
	newVendor := &Vendor{
		Name:    vendorName,
		Status:  1,
		Icon:    getDefaultVendorIcon(vendorName),
		Website: getDefaultVendorWebsite(vendorName),
	}

	if err := newVendor.Insert(); err != nil {
		return 0
	}

	vendorMap[newVendor.Id] = newVendor
	return newVendor.Id
}

// 获取供应商默认图标
func getDefaultVendorIcon(vendorName string) string {
	if icon, exists := defaultVendorIcons[vendorName]; exists {
		return icon
	}
	return ""
}

func getDefaultVendorWebsite(vendorName string) string {
	if vendorName == "AIPDD" {
		return constant.AIPDDWebsiteURL
	}
	return ""
}

func appendDefaultCatalogAbilities(abilities []AbilityWithChannel) []AbilityWithChannel {
	enabledModels := make(map[string]bool, len(abilities))
	for _, ability := range abilities {
		if ability.Enabled {
			enabledModels[ability.Model] = true
		}
	}

	for _, catalog := range defaultCatalogModels {
		if enabledModels[catalog.ModelName] {
			continue
		}
		abilities = append(abilities, AbilityWithChannel{
			Ability: Ability{
				Group:   "default",
				Model:   catalog.ModelName,
				Enabled: true,
			},
			ChannelType: catalog.ChannelType,
		})
	}
	return abilities
}

func defaultCatalogModelByName(modelName string) (defaultCatalogModel, bool) {
	for _, catalog := range defaultCatalogModels {
		if catalog.ModelName == modelName {
			return catalog, true
		}
	}
	return defaultCatalogModel{}, false
}

func getChannelAbilityModels(channel *Channel) []string {
	models := make([]string, 0)
	for _, modelName := range channel.GetModels() {
		modelName = strings.TrimSpace(modelName)
		if modelName != "" {
			models = append(models, modelName)
		}
	}
	if len(models) == 0 && channel.Type == constant.ChannelTypeAIPDD {
		return append([]string(nil), constant.AIPDDTaskModelList...)
	}
	return models
}

func marshalEndpointTypes(endpointTypes []constant.EndpointType) string {
	if len(endpointTypes) == 0 {
		return ""
	}
	data, err := common.Marshal(endpointTypes)
	if err != nil {
		return ""
	}
	return string(data)
}
