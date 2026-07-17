package model

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

type Pricing struct {
	ModelName              string                             `json:"model_name"`
	Description            string                             `json:"description,omitempty"`
	Icon                   string                             `json:"icon,omitempty"`
	Tags                   string                             `json:"tags,omitempty"`
	VendorID               int                                `json:"vendor_id,omitempty"`
	QuotaType              int                                `json:"quota_type"`
	ModelRatio             float64                            `json:"model_ratio"`
	ModelPrice             float64                            `json:"model_price"`
	OwnerBy                string                             `json:"owner_by"`
	CompletionRatio        float64                            `json:"completion_ratio"`
	CacheRatio             *float64                           `json:"cache_ratio,omitempty"`
	CreateCacheRatio       *float64                           `json:"create_cache_ratio,omitempty"`
	ImageRatio             *float64                           `json:"image_ratio,omitempty"`
	AudioRatio             *float64                           `json:"audio_ratio,omitempty"`
	AudioCompletionRatio   *float64                           `json:"audio_completion_ratio,omitempty"`
	EnableGroup            []string                           `json:"enable_groups"`
	SupportedEndpointTypes []constant.EndpointType            `json:"supported_endpoint_types"`
	BillingMode            string                             `json:"billing_mode,omitempty"`
	BillingExpr            string                             `json:"billing_expr,omitempty"`
	TaskPricing            *billing_setting.TaskPricingConfig `json:"task_pricing,omitempty"`
	TaskPricingResolutions []string                           `json:"task_pricing_resolutions,omitempty"`
	PricingVersion         string                             `json:"pricing_version,omitempty"`
}

type PricingVendor struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Website     string `json:"website,omitempty"`
}

var (
	pricingMap                        []Pricing
	vendorsList                       []PricingVendor
	supportedEndpointMap              map[string]common.EndpointInfo
	lastGetPricingTime                time.Time
	updatePricingLock                 sync.Mutex
	aipddSeedancePricingRequiredSet   map[string]struct{}
	aipddTaskPricingResolutionOptions map[string][]string

	// 缓存映射：模型名 -> 启用分组 / 计费类型
	modelEnableGroups     = make(map[string][]string)
	modelQuotaTypeMap     = make(map[string]int)
	modelEnableGroupsLock = sync.RWMutex{}
)

var (
	modelSupportEndpointTypes = make(map[string][]constant.EndpointType)
	modelSupportEndpointsLock = sync.RWMutex{}
)

func GetPricing() []Pricing {
	updatePricingLock.Lock()
	defer updatePricingLock.Unlock()
	refreshPricingLocked()
	cloned := make([]Pricing, len(pricingMap))
	for index, pricing := range pricingMap {
		cloned[index] = pricing
		cloned[index].EnableGroup = append([]string(nil), pricing.EnableGroup...)
		cloned[index].SupportedEndpointTypes = append([]constant.EndpointType(nil), pricing.SupportedEndpointTypes...)
		cloned[index].TaskPricingResolutions = append([]string(nil), pricing.TaskPricingResolutions...)
		if pricing.TaskPricing != nil {
			taskPricing := *pricing.TaskPricing
			if pricing.TaskPricing.ByResolution != nil {
				taskPricing.ByResolution = make(map[string]billing_setting.TaskPricingTier, len(pricing.TaskPricing.ByResolution))
				for resolution, tier := range pricing.TaskPricing.ByResolution {
					taskPricing.ByResolution[resolution] = tier
				}
			}
			cloned[index].TaskPricing = &taskPricing
		}
	}
	return cloned
}

func GetTaskPricingResolutions(modelName string) []string {
	for _, pricing := range GetPricing() {
		if pricing.ModelName == modelName && pricing.BillingMode == billing_setting.BillingModeTaskPricing {
			return append([]string(nil), pricing.TaskPricingResolutions...)
		}
	}
	return nil
}

func refreshPricingLocked() {
	if time.Since(lastGetPricingTime) > time.Minute*1 || len(pricingMap) == 0 {
		modelSupportEndpointsLock.Lock()
		updatePricing()
		modelSupportEndpointsLock.Unlock()
	}
}

// GetAIPDDSeedancePricingRequiredModels returns local/origin model names that
// route to Seedance. It includes channel model mappings so an alias cannot
// expose a legacy fixed ModelPrice while relay correctly rejects that price.
func GetAIPDDSeedancePricingRequiredModels() []string {
	updatePricingLock.Lock()
	defer updatePricingLock.Unlock()
	required := getAIPDDSeedancePricingRequiredSetLocked()
	return sortedPricingModelNames(required)
}

// GetTaskPricingResolutionOptions returns the current upstream-supported
// resolution identifiers for each local/origin Seedance model. The returned
// map and slices are isolated copies safe for callers to mutate.
func GetTaskPricingResolutionOptions() map[string][]string {
	updatePricingLock.Lock()
	defer updatePricingLock.Unlock()
	options := getTaskPricingResolutionOptionsLocked()
	cloned := make(map[string][]string, len(options))
	for modelName, resolutions := range options {
		cloned[modelName] = append([]string(nil), resolutions...)
	}
	return cloned
}

func getTaskPricingResolutionOptionsLocked() map[string][]string {
	if aipddTaskPricingResolutionOptions != nil {
		return aipddTaskPricingResolutionOptions
	}
	aipddTaskPricingResolutionOptions = computeTaskPricingResolutionOptions()
	return aipddTaskPricingResolutionOptions
}

func capabilityResolutionSet(capability constant.AIPDDCapability) map[string]struct{} {
	if capability.SeedancePricing == nil {
		return nil
	}
	resolutions := make(map[string]struct{}, len(capability.SeedancePricing.ByResolution))
	for rawResolution := range capability.SeedancePricing.ByResolution {
		resolution, err := billing_setting.NormalizeTaskPricingResolution(rawResolution)
		if err == nil {
			resolutions[resolution] = struct{}{}
		}
	}
	return resolutions
}

func intersectResolutionSets(current, next map[string]struct{}) map[string]struct{} {
	if current == nil {
		cloned := make(map[string]struct{}, len(next))
		for resolution := range next {
			cloned[resolution] = struct{}{}
		}
		return cloned
	}
	for resolution := range current {
		if _, ok := next[resolution]; !ok {
			delete(current, resolution)
		}
	}
	return current
}

func computeTaskPricingResolutionOptions() map[string][]string {
	sets := make(map[string]map[string]struct{})
	for _, capability := range constant.GetAIPDDCapabilities() {
		modelName := strings.TrimSpace(capability.ModelName)
		if modelName == "" || !constant.IsAIPDDSeedanceModel(modelName) {
			continue
		}
		sets[modelName] = intersectResolutionSets(sets[modelName], capabilityResolutionSet(capability))
	}

	if DB != nil {
		var rows []struct {
			Model        string
			ModelMapping *string
		}
		err := DB.Table("abilities").
			Select("abilities.model, channels.model_mapping").
			Joins("JOIN channels ON channels.id = abilities.channel_id").
			Where("abilities.enabled = ? AND channels.status = ? AND channels.type = ?", true, common.ChannelStatusEnabled, constant.ChannelTypeAIPDD).
			Scan(&rows).Error
		if err != nil {
			common.SysLog("failed to resolve AIPDD Seedance pricing resolutions: " + err.Error())
		} else {
			for _, row := range rows {
				origin := strings.TrimSpace(row.Model)
				if origin == "" {
					continue
				}
				mapped := origin
				if row.ModelMapping != nil && strings.TrimSpace(*row.ModelMapping) != "" {
					var mapping map[string]string
					if err := common.UnmarshalJsonStr(*row.ModelMapping, &mapping); err == nil {
						mapped = finalPricingMappedModel(origin, mapping)
					}
				}
				capability, ok := constant.GetAIPDDCapability(mapped)
				if !ok || !constant.IsAIPDDSeedanceModel(capability.ModelName) {
					continue
				}
				sets[origin] = intersectResolutionSets(sets[origin], capabilityResolutionSet(capability))
			}
		}
	}

	options := make(map[string][]string, len(sets))
	for modelName, set := range sets {
		resolutions := make([]string, 0, len(set))
		for resolution := range set {
			resolutions = append(resolutions, resolution)
		}
		sort.SliceStable(resolutions, func(left, right int) bool {
			return billing_setting.TaskPricingResolutionLess(resolutions[left], resolutions[right])
		})
		options[modelName] = resolutions
	}
	return options
}

func effectiveTaskPricingResolutions(cfg billing_setting.TaskPricingConfig, supported []string) []string {
	if len(supported) == 0 {
		return nil
	}
	if len(cfg.ByResolution) == 0 {
		return append([]string(nil), supported...)
	}
	configured := make(map[string]struct{}, len(cfg.ByResolution))
	for _, resolution := range billing_setting.TaskPricingResolutionKeys(cfg) {
		configured[resolution] = struct{}{}
	}
	effective := make([]string, 0, len(supported))
	for _, resolution := range supported {
		if _, ok := configured[resolution]; ok {
			effective = append(effective, resolution)
		}
	}
	return effective
}

func taskPricingMinimumUnitPrice(cfg billing_setting.TaskPricingConfig, activeResolutions []string) (float64, bool) {
	prices := billing_setting.TaskPricingUnitPrices(cfg)
	if len(cfg.ByResolution) > 0 {
		prices = prices[:0]
		for _, resolution := range activeResolutions {
			tier, ok := cfg.ByResolution[resolution]
			if !ok {
				continue
			}
			prices = append(prices, tier.NoReferenceVideoUnitPrice)
			if tier.ReferenceVideoPolicy == billing_setting.ReferenceVideoPolicySame {
				prices = append(prices, tier.NoReferenceVideoUnitPrice)
			} else if tier.ReferenceVideoPolicy == billing_setting.ReferenceVideoPolicyCustom {
				prices = append(prices, tier.ReferenceVideoUnitPrice)
			}
		}
	}
	minimum := 0.0
	for _, price := range prices {
		if price > 0 && (minimum == 0 || price < minimum) {
			minimum = price
		}
	}
	return minimum, minimum > 0
}

// IsAIPDDSeedancePricingRequiredModel applies the same origin-level rule used
// by the public pricing list. If any enabled AIPDD route maps an origin model
// to Seedance, every route for that origin must use local task pricing; a
// different selected channel must not revive a legacy fixed ModelPrice.
func IsAIPDDSeedancePricingRequiredModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}
	updatePricingLock.Lock()
	defer updatePricingLock.Unlock()
	_, ok := getAIPDDSeedancePricingRequiredSetLocked()[modelName]
	return ok
}

func getAIPDDSeedancePricingRequiredSetLocked() map[string]struct{} {
	if aipddSeedancePricingRequiredSet != nil {
		return aipddSeedancePricingRequiredSet
	}
	aipddSeedancePricingRequiredSet = computeAIPDDSeedancePricingRequiredSet()
	return aipddSeedancePricingRequiredSet
}

func computeAIPDDSeedancePricingRequiredSet() map[string]struct{} {
	required := make(map[string]struct{})
	for _, capability := range constant.GetAIPDDCapabilities() {
		name := strings.TrimSpace(capability.ModelName)
		if name != "" && constant.IsAIPDDSeedanceModel(name) {
			required[name] = struct{}{}
		}
	}

	if DB != nil {
		var rows []struct {
			Model        string
			ModelMapping *string
		}
		err := DB.Table("abilities").
			Select("abilities.model, channels.model_mapping").
			Joins("JOIN channels ON channels.id = abilities.channel_id").
			Where("abilities.enabled = ? AND channels.status = ? AND channels.type = ?", true, common.ChannelStatusEnabled, constant.ChannelTypeAIPDD).
			Scan(&rows).Error
		if err != nil {
			common.SysLog("failed to resolve AIPDD Seedance pricing aliases: " + err.Error())
		} else {
			for _, row := range rows {
				origin := strings.TrimSpace(row.Model)
				if origin == "" {
					continue
				}
				mapped := origin
				if row.ModelMapping != nil && strings.TrimSpace(*row.ModelMapping) != "" {
					var mapping map[string]string
					if err := common.UnmarshalJsonStr(*row.ModelMapping, &mapping); err == nil {
						mapped = finalPricingMappedModel(origin, mapping)
					}
				}
				if constant.IsAIPDDSeedanceModel(mapped) {
					required[origin] = struct{}{}
				}
			}
		}
	}

	return required
}

func sortedPricingModelNames(models map[string]struct{}) []string {
	names := make([]string, 0, len(models))
	for modelName := range models {
		names = append(names, modelName)
	}
	sort.Strings(names)
	return names
}

func finalPricingMappedModel(origin string, mapping map[string]string) string {
	current := origin
	visited := map[string]struct{}{current: {}}
	for {
		next := strings.TrimSpace(mapping[current])
		if next == "" {
			return current
		}
		if _, exists := visited[next]; exists {
			return origin
		}
		visited[next] = struct{}{}
		current = next
	}
}

func InvalidatePricingCache() {
	updatePricingLock.Lock()
	defer updatePricingLock.Unlock()

	pricingMap = nil
	vendorsList = nil
	lastGetPricingTime = time.Time{}
	aipddSeedancePricingRequiredSet = nil
	aipddTaskPricingResolutionOptions = nil
}

// GetVendors 返回当前定价接口使用到的供应商信息
func GetVendors() []PricingVendor {
	updatePricingLock.Lock()
	defer updatePricingLock.Unlock()
	refreshPricingLocked()
	return append([]PricingVendor(nil), vendorsList...)
}

func GetModelSupportEndpointTypes(model string) []constant.EndpointType {
	if model == "" {
		return make([]constant.EndpointType, 0)
	}
	modelSupportEndpointsLock.RLock()
	defer modelSupportEndpointsLock.RUnlock()
	if endpoints, ok := modelSupportEndpointTypes[model]; ok {
		return endpoints
	}
	return make([]constant.EndpointType, 0)
}

func updatePricing() {
	//modelRatios := common.GetModelRatios()
	enableAbilities, err := GetAllEnableAbilityWithChannels()
	if err != nil {
		common.SysLog(fmt.Sprintf("GetAllEnableAbilityWithChannels error: %v", err))
		return
	}
	enableAbilities = appendDefaultCatalogAbilities(enableAbilities)
	// 预加载模型元数据与供应商一次，避免循环查询
	var allMeta []Model
	_ = DB.Find(&allMeta).Error
	metaMap := make(map[string]*Model)
	prefixList := make([]*Model, 0)
	suffixList := make([]*Model, 0)
	containsList := make([]*Model, 0)
	for i := range allMeta {
		m := &allMeta[i]
		if m.NameRule == NameRuleExact {
			metaMap[m.ModelName] = m
		} else {
			switch m.NameRule {
			case NameRulePrefix:
				prefixList = append(prefixList, m)
			case NameRuleSuffix:
				suffixList = append(suffixList, m)
			case NameRuleContains:
				containsList = append(containsList, m)
			}
		}
	}

	// 将非精确规则模型匹配到 metaMap
	for _, m := range prefixList {
		for _, pricingModel := range enableAbilities {
			if strings.HasPrefix(pricingModel.Model, m.ModelName) {
				if _, exists := metaMap[pricingModel.Model]; !exists {
					metaMap[pricingModel.Model] = m
				}
			}
		}
	}
	for _, m := range suffixList {
		for _, pricingModel := range enableAbilities {
			if strings.HasSuffix(pricingModel.Model, m.ModelName) {
				if _, exists := metaMap[pricingModel.Model]; !exists {
					metaMap[pricingModel.Model] = m
				}
			}
		}
	}
	for _, m := range containsList {
		for _, pricingModel := range enableAbilities {
			if strings.Contains(pricingModel.Model, m.ModelName) {
				if _, exists := metaMap[pricingModel.Model]; !exists {
					metaMap[pricingModel.Model] = m
				}
			}
		}
	}

	// 预加载供应商
	var vendors []Vendor
	_ = DB.Find(&vendors).Error
	vendorMap := make(map[int]*Vendor)
	for i := range vendors {
		vendorMap[vendors[i].Id] = &vendors[i]
	}

	// 初始化默认供应商映射
	initDefaultVendorMapping(metaMap, vendorMap, enableAbilities)

	// 构建对前端友好的供应商列表
	vendorsList = make([]PricingVendor, 0, len(vendorMap))
	for _, v := range vendorMap {
		vendorsList = append(vendorsList, PricingVendor{
			ID:          v.Id,
			Name:        v.Name,
			Description: v.Description,
			Icon:        v.Icon,
			Website:     v.Website,
		})
	}

	modelGroupsMap := make(map[string]*types.Set[string])

	for _, ability := range enableAbilities {
		groups, ok := modelGroupsMap[ability.Model]
		if !ok {
			groups = types.NewSet[string]()
			modelGroupsMap[ability.Model] = groups
		}
		groups.Add(ability.Group)
	}

	//这里使用切片而不是Set，因为一个模型可能支持多个端点类型，并且第一个端点是优先使用端点
	modelSupportEndpointsStr := make(map[string][]string)

	// 先根据已有能力填充原生端点
	for _, ability := range enableAbilities {
		endpoints := modelSupportEndpointsStr[ability.Model]
		channelTypes := common.GetEndpointTypesByChannelType(ability.ChannelType, ability.Model)
		for _, channelType := range channelTypes {
			if !common.StringsContains(endpoints, string(channelType)) {
				endpoints = append(endpoints, string(channelType))
			}
		}
		modelSupportEndpointsStr[ability.Model] = endpoints
	}

	// 再补充模型自定义端点：若配置有效则替换默认端点，不做合并
	for modelName, meta := range metaMap {
		if strings.TrimSpace(meta.Endpoints) == "" {
			continue
		}
		var raw map[string]interface{}
		if err := common.Unmarshal([]byte(meta.Endpoints), &raw); err == nil {
			endpoints := make([]string, 0, len(raw))
			for k, v := range raw {
				switch v.(type) {
				case string, map[string]interface{}:
					if !common.StringsContains(endpoints, k) {
						endpoints = append(endpoints, k)
					}
				}
			}
			if len(endpoints) > 0 {
				modelSupportEndpointsStr[modelName] = endpoints
			}
		}
	}

	modelSupportEndpointTypes = make(map[string][]constant.EndpointType)
	for model, endpoints := range modelSupportEndpointsStr {
		supportedEndpoints := make([]constant.EndpointType, 0)
		for _, endpointStr := range endpoints {
			endpointType := constant.EndpointType(endpointStr)
			supportedEndpoints = append(supportedEndpoints, endpointType)
		}
		modelSupportEndpointTypes[model] = supportedEndpoints
	}

	// 构建全局 supportedEndpointMap（默认 + 自定义覆盖）
	supportedEndpointMap = make(map[string]common.EndpointInfo)
	// 1. 默认端点
	for _, endpoints := range modelSupportEndpointTypes {
		for _, et := range endpoints {
			if info, ok := common.GetDefaultEndpointInfo(et); ok {
				if _, exists := supportedEndpointMap[string(et)]; !exists {
					supportedEndpointMap[string(et)] = info
				}
			}
		}
	}
	// 2. 自定义端点（models 表）覆盖默认
	for _, meta := range metaMap {
		if strings.TrimSpace(meta.Endpoints) == "" {
			continue
		}
		var raw map[string]interface{}
		if err := common.Unmarshal([]byte(meta.Endpoints), &raw); err == nil {
			for k, v := range raw {
				switch val := v.(type) {
				case string:
					supportedEndpointMap[k] = common.EndpointInfo{Path: val, Method: "POST"}
				case map[string]interface{}:
					ep := common.EndpointInfo{Method: "POST"}
					if p, ok := val["path"].(string); ok {
						ep.Path = p
					}
					if m, ok := val["method"].(string); ok {
						ep.Method = strings.ToUpper(m)
					}
					supportedEndpointMap[k] = ep
				default:
					// ignore unsupported types
				}
			}
		}
	}

	pricingMap = make([]Pricing, 0)
	taskPricingRequiredModels := getAIPDDSeedancePricingRequiredSetLocked()
	taskPricingResolutionOptions := getTaskPricingResolutionOptionsLocked()
	for model, groups := range modelGroupsMap {
		pricing := Pricing{
			ModelName:              model,
			EnableGroup:            groups.Items(),
			SupportedEndpointTypes: modelSupportEndpointTypes[model],
		}

		// 补充模型元数据（描述、标签、供应商、状态）
		if meta, ok := metaMap[model]; ok {
			// 若模型被禁用(status!=1)，则直接跳过，不返回给前端
			if meta.Status != 1 {
				continue
			}
			pricing.Description = meta.Description
			pricing.Icon = meta.Icon
			pricing.Tags = meta.Tags
			pricing.VendorID = meta.VendorID
		}
		billingMode := billing_setting.GetBillingMode(model)
		if billingMode == billing_setting.BillingModeTaskPricing {
			taskPricing, ok := billing_setting.GetTaskPricing(model)
			if !ok || billing_setting.ValidateTaskPricingConfig(taskPricing) != nil {
				continue
			}
			activeResolutions := effectiveTaskPricingResolutions(taskPricing, taskPricingResolutionOptions[model])
			if len(activeResolutions) == 0 {
				continue
			}
			minimumPrice, ok := taskPricingMinimumUnitPrice(taskPricing, activeResolutions)
			if !ok {
				continue
			}
			pricing.BillingMode = billingMode
			pricing.TaskPricing = &taskPricing
			pricing.TaskPricingResolutions = activeResolutions
			pricing.ModelPrice = minimumPrice
			pricing.QuotaType = 1
		} else {
			if _, requiresTaskPricing := taskPricingRequiredModels[model]; requiresTaskPricing {
				continue
			}
			modelPrice, findPrice := ratio_setting.GetModelPrice(model, false)
			if findPrice {
				pricing.ModelPrice = modelPrice
				pricing.QuotaType = 1
			} else {
				modelRatio, _, _ := ratio_setting.GetModelRatio(model)
				pricing.ModelRatio = modelRatio
				pricing.CompletionRatio = ratio_setting.GetCompletionRatio(model)
				pricing.QuotaType = 0
			}
		}
		if cacheRatio, ok := ratio_setting.GetCacheRatio(model); ok {
			pricing.CacheRatio = &cacheRatio
		}
		if createCacheRatio, ok := ratio_setting.GetCreateCacheRatio(model); ok {
			pricing.CreateCacheRatio = &createCacheRatio
		}
		if imageRatio, ok := ratio_setting.GetImageRatio(model); ok {
			pricing.ImageRatio = &imageRatio
		}
		if ratio_setting.ContainsAudioRatio(model) {
			audioRatio := ratio_setting.GetAudioRatio(model)
			pricing.AudioRatio = &audioRatio
		}
		if ratio_setting.ContainsAudioCompletionRatio(model) {
			audioCompletionRatio := ratio_setting.GetAudioCompletionRatio(model)
			pricing.AudioCompletionRatio = &audioCompletionRatio
		}
		if billingMode == billing_setting.BillingModeTieredExpr {
			if expr, ok := billing_setting.GetBillingExpr(model); ok && strings.TrimSpace(expr) != "" {
				pricing.BillingMode = billingMode
				pricing.BillingExpr = expr
			}
		}
		pricingMap = append(pricingMap, pricing)
	}

	// 防止大更新后数据不通用
	if len(pricingMap) > 0 {
		pricingMap[0].PricingVersion = "5a90f2b86c08bd983a9a2e6d66c255f4eaef9c4bc934386d2b6ae84ef0ff1f1f"
	}

	// 刷新缓存映射，供高并发快速查询
	modelEnableGroupsLock.Lock()
	modelEnableGroups = make(map[string][]string)
	modelQuotaTypeMap = make(map[string]int)
	for _, p := range pricingMap {
		modelEnableGroups[p.ModelName] = p.EnableGroup
		modelQuotaTypeMap[p.ModelName] = p.QuotaType
	}
	modelEnableGroupsLock.Unlock()

	lastGetPricingTime = time.Now()
}

// GetSupportedEndpointMap 返回全局端点到路径的映射
func GetSupportedEndpointMap() map[string]common.EndpointInfo {
	return supportedEndpointMap
}
