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
	"seedream": "字节跳动",
	"seedance": "字节跳动",
	"seed-":    "字节跳动",
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
				ensureDefaultModelVendor(meta, getOrCreateVendor(catalog.VendorName, vendorMap))
			} else {
				ensureDefaultModelVendor(meta, inferDefaultVendorID(modelName, vendorMap))
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

		// 创建模型元数据
		metaMap[modelName] = &Model{
			ModelName: modelName,
			VendorID:  inferDefaultVendorID(modelName, vendorMap),
			Status:    1,
			NameRule:  NameRuleExact,
		}
	}
}

func ensureDefaultVendorIcons(vendorMap map[int]*Vendor) {
	for _, vendor := range vendorMap {
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

func inferDefaultVendorID(modelName string, vendorMap map[int]*Vendor) int {
	modelLower := strings.ToLower(modelName)
	for pattern, vendorName := range defaultVendorRules {
		if strings.Contains(modelLower, pattern) {
			return getOrCreateVendor(vendorName, vendorMap)
		}
	}
	return 0
}

func ensureDefaultModelVendor(meta *Model, vendorID int) {
	if meta == nil || meta.VendorID != 0 || vendorID == 0 {
		return
	}
	meta.VendorID = vendorID
	if meta.Id > 0 {
		_ = DB.Model(&Model{}).Where("id = ?", meta.Id).Update("vendor_id", vendorID).Error
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

	for _, catalog := range aipddCurrentCatalogModels() {
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
		enabledModels[catalog.ModelName] = true
	}
	return abilities
}

func defaultCatalogModelByName(modelName string) (defaultCatalogModel, bool) {
	return aipddCatalogModelByName(modelName)
}

func aipddCatalogModelByName(modelName string) (defaultCatalogModel, bool) {
	capability, ok := constant.GetAIPDDCapability(modelName)
	if !ok {
		return defaultCatalogModel{}, false
	}
	return aipddCatalogModelFromCapability(capability), true
}

func aipddCurrentCatalogModels() []defaultCatalogModel {
	capabilities := constant.GetAIPDDCapabilities()
	catalogs := make([]defaultCatalogModel, 0, len(capabilities))
	seen := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		if strings.TrimSpace(capability.ModelName) == "" || seen[capability.ModelName] {
			continue
		}
		catalogs = append(catalogs, aipddCatalogModelFromCapability(capability))
		seen[capability.ModelName] = true
	}
	return catalogs
}

func aipddCatalogModelFromCapability(capability constant.AIPDDCapability) defaultCatalogModel {
	endpointTypes := append([]constant.EndpointType(nil), capability.EndpointTypes...)
	if len(endpointTypes) == 0 && capability.EndpointType != "" {
		endpointTypes = []constant.EndpointType{capability.EndpointType}
	}

	return defaultCatalogModel{
		ModelName:     capability.ModelName,
		VendorName:    "AIPDD",
		Description:   aipddCapabilityDescription(capability, ""),
		Icon:          aipddLogoPath,
		Tags:          strings.Join(aipddCapabilityTags(capability), ","),
		ChannelType:   constant.ChannelTypeAIPDD,
		EndpointTypes: endpointTypes,
	}
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
		return constant.GetAIPDDModelList()
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
