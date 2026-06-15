package constant

import (
	"strings"
	"sync"
)

const (
	AIPDDModelFluxGGUF      = "aipdd-flux-gguf"
	AIPDDModelFluxGGUFT2I   = "aipdd-flux-gguf-t2i"
	AIPDDModelWan22Wanx     = "aipdd-wan2.2-wanx"
	AIPDDModelWan22Animater = "aipdd-wan2.2-animater"
	AIPDDModelMimicMotion   = "aipdd-mimic-motion"
	AIPDDModelLatentsync15  = "aipdd-latentsync-1.5"
	AIPDDModelIndexTTS      = "aipdd-indextts"
	AIPDDLogoPath           = "/aipdd-logo.png"
	AIPDDWebsiteURL         = "https://app.aipdd.work"
)

type AIPDDBillingType string

const (
	AIPDDBillingTypePerCall         AIPDDBillingType = "per_call"
	AIPDDBillingTypeDurationSeconds AIPDDBillingType = "duration_seconds"
)

type AIPDDWorkflowValueType string

const (
	AIPDDWorkflowValueTypeString AIPDDWorkflowValueType = "string"
	AIPDDWorkflowValueTypeInt    AIPDDWorkflowValueType = "int"
)

type AIPDDWorkflowSourceType string

const (
	AIPDDWorkflowSourceMetadata       AIPDDWorkflowSourceType = "metadata"
	AIPDDWorkflowSourcePrompt         AIPDDWorkflowSourceType = "prompt"
	AIPDDWorkflowSourceImage          AIPDDWorkflowSourceType = "image"
	AIPDDWorkflowSourceFirstImage     AIPDDWorkflowSourceType = "first_image"
	AIPDDWorkflowSourceInputReference AIPDDWorkflowSourceType = "input_reference"
	AIPDDWorkflowSourceDuration       AIPDDWorkflowSourceType = "duration"
)

type AIPDDWorkflowValueSource struct {
	Type AIPDDWorkflowSourceType
	Key  string
}

type AIPDDWorkflowParamDefault struct {
	ParamKey  string
	ValueType AIPDDWorkflowValueType
	Sources   []AIPDDWorkflowValueSource
}

type AIPDDUploadTarget struct {
	ParamKey string
	Aliases  []string
}

type AIPDDCapability struct {
	ModelName              string
	ScriptID               string
	ScriptCode             string
	TaskKind               string
	InputModalities        []string
	OutputModalities       []string
	TaskCost               float64
	WorkflowParamKeys      []string
	RequiredWorkflowParams map[string]bool
	WorkflowDefaults       []AIPDDWorkflowParamDefault
	UploadTargets          []AIPDDUploadTarget
	EndpointType           EndpointType
	BillingType            AIPDDBillingType
}

const AIPDDWan22WanxRMBPerSecond = 0.02

func aipddMetadata(key string) AIPDDWorkflowValueSource {
	return AIPDDWorkflowValueSource{Type: AIPDDWorkflowSourceMetadata, Key: key}
}

func aipddSource(sourceType AIPDDWorkflowSourceType) AIPDDWorkflowValueSource {
	return AIPDDWorkflowValueSource{Type: sourceType}
}

func aipddStringDefault(paramKey string, sources ...AIPDDWorkflowValueSource) AIPDDWorkflowParamDefault {
	return AIPDDWorkflowParamDefault{
		ParamKey:  paramKey,
		ValueType: AIPDDWorkflowValueTypeString,
		Sources:   sources,
	}
}

func aipddIntDefault(paramKey string, sources ...AIPDDWorkflowValueSource) AIPDDWorkflowParamDefault {
	return AIPDDWorkflowParamDefault{
		ParamKey:  paramKey,
		ValueType: AIPDDWorkflowValueTypeInt,
		Sources:   sources,
	}
}

var AIPDDCapabilities = []AIPDDCapability{
	{
		ModelName:         AIPDDModelFluxGGUF,
		ScriptID:          "c1d4d41c-0d5a-4bf8-bfdb-548d7a710759",
		ScriptCode:        "FLUX-GGUF-V2",
		TaskCost:          1000,
		WorkflowParamKeys: []string{"image", "positive_prompt"},
		RequiredWorkflowParams: map[string]bool{
			"image":           true,
			"positive_prompt": true,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("image", aipddMetadata("image"), aipddSource(AIPDDWorkflowSourceImage), aipddSource(AIPDDWorkflowSourceFirstImage)),
			aipddStringDefault("positive_prompt", aipddMetadata("positive_prompt"), aipddMetadata("prompt"), aipddSource(AIPDDWorkflowSourcePrompt)),
		},
		UploadTargets: []AIPDDUploadTarget{
			{ParamKey: "image", Aliases: []string{"file", "input_reference", "reference", "images"}},
		},
		EndpointType: EndpointTypeImageGeneration,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelFluxGGUFT2I,
		ScriptID:          "aa6e64ce-bc73-4295-b78a-a269e5d3c1a9",
		ScriptCode:        "FLUX-GGUF-T2I-V2",
		TaskCost:          1000,
		WorkflowParamKeys: []string{"text"},
		RequiredWorkflowParams: map[string]bool{
			"text": true,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("text", aipddMetadata("text"), aipddMetadata("input"), aipddMetadata("prompt"), aipddSource(AIPDDWorkflowSourcePrompt)),
		},
		EndpointType: EndpointTypeImageGeneration,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelWan22Wanx,
		ScriptID:          "3eae5a25-98cf-4658-aa9f-c48bb41043a6",
		ScriptCode:        "aipdd_wan2.2_wanx",
		TaskCost:          2000,
		WorkflowParamKeys: []string{"image", "prompt", "negative_prompt", "fps"},
		RequiredWorkflowParams: map[string]bool{
			"image":           true,
			"prompt":          true,
			"negative_prompt": false,
			"fps":             false,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("image", aipddMetadata("image"), aipddSource(AIPDDWorkflowSourceImage), aipddSource(AIPDDWorkflowSourceFirstImage)),
			aipddStringDefault("prompt", aipddMetadata("prompt"), aipddSource(AIPDDWorkflowSourcePrompt)),
			aipddStringDefault("negative_prompt", aipddMetadata("negative_prompt")),
			aipddStringDefault("fps", aipddMetadata("fps")),
		},
		UploadTargets: []AIPDDUploadTarget{
			{ParamKey: "image", Aliases: []string{"file", "input_reference", "reference", "images"}},
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:  AIPDDModelWan22Animater,
		ScriptID:   "4f1401c1-9791-406e-8ce2-4808f9b95d9c",
		ScriptCode: "aipdd_Wan2.2-Animater",
		TaskCost:   2000,
		WorkflowParamKeys: []string{
			"video",
			"image",
			"positive_prompt",
			"filename_prefix",
		},
		RequiredWorkflowParams: map[string]bool{
			"video":           true,
			"image":           false,
			"positive_prompt": true,
			"filename_prefix": false,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("video", aipddMetadata("video"), aipddMetadata("load_video"), aipddSource(AIPDDWorkflowSourceInputReference), aipddSource(AIPDDWorkflowSourceImage), aipddSource(AIPDDWorkflowSourceFirstImage)),
			aipddStringDefault("image", aipddMetadata("image"), aipddMetadata("fullpath"), aipddMetadata("reference_image"), aipddSource(AIPDDWorkflowSourceFirstImage), aipddSource(AIPDDWorkflowSourceImage)),
			aipddStringDefault("positive_prompt", aipddMetadata("positive_prompt"), aipddMetadata("prompt"), aipddSource(AIPDDWorkflowSourcePrompt)),
			aipddStringDefault("filename_prefix", aipddMetadata("filename_prefix")),
		},
		UploadTargets: []AIPDDUploadTarget{
			{ParamKey: "video", Aliases: []string{"file", "input_reference", "reference", "load_video"}},
			{ParamKey: "image", Aliases: []string{"fullpath", "reference_image", "appearance_image"}},
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelMimicMotion,
		ScriptID:          "0628aec4-ab5e-4b3f-a453-760bcb8bfeaf",
		ScriptCode:        "aipdd_mimic_motion",
		TaskCost:          2000,
		WorkflowParamKeys: []string{"motion_video", "appearance_image"},
		RequiredWorkflowParams: map[string]bool{
			"motion_video":     true,
			"appearance_image": true,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("motion_video", aipddMetadata("motion_video"), aipddMetadata("video"), aipddMetadata("load_video"), aipddSource(AIPDDWorkflowSourceInputReference), aipddSource(AIPDDWorkflowSourceImage), aipddSource(AIPDDWorkflowSourceFirstImage)),
			aipddStringDefault("appearance_image", aipddMetadata("appearance_image"), aipddMetadata("image"), aipddSource(AIPDDWorkflowSourceFirstImage), aipddSource(AIPDDWorkflowSourceImage)),
		},
		UploadTargets: []AIPDDUploadTarget{
			{ParamKey: "motion_video", Aliases: []string{"video", "load_video", "input_reference", "motion"}},
			{ParamKey: "appearance_image", Aliases: []string{"image", "reference_image", "appearance", "person"}},
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelLatentsync15,
		ScriptID:          "57971711-0255-46b1-893a-10d7216f42a0",
		ScriptCode:        "aipdd_latentsync1.5",
		TaskCost:          1000,
		WorkflowParamKeys: []string{"video", "LoadAudio"},
		RequiredWorkflowParams: map[string]bool{
			"video":     true,
			"LoadAudio": true,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("video", aipddMetadata("video"), aipddMetadata("load_video"), aipddSource(AIPDDWorkflowSourceInputReference), aipddSource(AIPDDWorkflowSourceImage), aipddSource(AIPDDWorkflowSourceFirstImage)),
			aipddStringDefault("LoadAudio", aipddMetadata("LoadAudio"), aipddMetadata("audio")),
		},
		UploadTargets: []AIPDDUploadTarget{
			{ParamKey: "video", Aliases: []string{"file", "input_reference", "reference", "load_video"}},
			{ParamKey: "LoadAudio", Aliases: []string{"audio", "input_audio", "voice"}},
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelIndexTTS,
		ScriptID:          "eba39b43-6c3b-4930-b0c9-a492706fa464",
		ScriptCode:        "aipdd_IndexTTS",
		TaskCost:          1000,
		WorkflowParamKeys: []string{"audio", "emotion_audio", "text"},
		RequiredWorkflowParams: map[string]bool{
			"audio":         true,
			"emotion_audio": false,
			"text":          true,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("audio", aipddMetadata("audio"), aipddMetadata("ref_audio"), aipddMetadata("reference_audio"), aipddMetadata("voice"), aipddSource(AIPDDWorkflowSourceInputReference), aipddSource(AIPDDWorkflowSourceFirstImage), aipddSource(AIPDDWorkflowSourceImage)),
			aipddStringDefault("emotion_audio", aipddMetadata("emotion_audio")),
			aipddStringDefault("text", aipddMetadata("text"), aipddMetadata("input"), aipddSource(AIPDDWorkflowSourcePrompt)),
		},
		UploadTargets: []AIPDDUploadTarget{
			{ParamKey: "audio", Aliases: []string{"file", "input_reference", "ref_audio", "reference_audio", "voice"}},
			{ParamKey: "emotion_audio"},
		},
		EndpointType: EndpointTypeAudioSpeech,
		BillingType:  AIPDDBillingTypePerCall,
	},
}

var defaultAIPDDCapabilities = cloneAIPDDCapabilities(AIPDDCapabilities)

var AIPDDTaskModelList = buildAIPDDTaskModelList(AIPDDCapabilities)

var AIPDDOpenAIModelList []string

var aipddCapabilityByAlias = buildAIPDDCapabilityByAlias(AIPDDCapabilities)

var defaultAIPDDCapabilityByAlias = buildAIPDDCapabilityByAlias(defaultAIPDDCapabilities)

var aipddCapabilitiesLock sync.RWMutex

func buildAIPDDTaskModelList(capabilities []AIPDDCapability) []string {
	models := make([]string, 0, len(capabilities))
	seen := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		modelName := strings.TrimSpace(capability.ModelName)
		if modelName == "" || seen[modelName] {
			continue
		}
		models = append(models, modelName)
		seen[modelName] = true
	}
	return models
}

func buildAIPDDCapabilityByAlias(capabilities []AIPDDCapability) map[string]AIPDDCapability {
	out := make(map[string]AIPDDCapability, len(capabilities)*2)
	for _, capability := range capabilities {
		capability = cloneAIPDDCapability(capability)
		for _, alias := range []string{capability.ModelName, capability.ScriptCode} {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				continue
			}
			out[alias] = capability
		}
	}
	return out
}

func cloneAIPDDCapabilities(capabilities []AIPDDCapability) []AIPDDCapability {
	out := make([]AIPDDCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, cloneAIPDDCapability(capability))
	}
	return out
}

func cloneAIPDDCapability(capability AIPDDCapability) AIPDDCapability {
	capability.InputModalities = append([]string(nil), capability.InputModalities...)
	capability.OutputModalities = append([]string(nil), capability.OutputModalities...)
	capability.WorkflowParamKeys = append([]string(nil), capability.WorkflowParamKeys...)
	if capability.RequiredWorkflowParams != nil {
		required := make(map[string]bool, len(capability.RequiredWorkflowParams))
		for key, value := range capability.RequiredWorkflowParams {
			required[key] = value
		}
		capability.RequiredWorkflowParams = required
	}
	if capability.WorkflowDefaults != nil {
		defaults := make([]AIPDDWorkflowParamDefault, 0, len(capability.WorkflowDefaults))
		for _, item := range capability.WorkflowDefaults {
			item.Sources = append([]AIPDDWorkflowValueSource(nil), item.Sources...)
			defaults = append(defaults, item)
		}
		capability.WorkflowDefaults = defaults
	}
	if capability.UploadTargets != nil {
		targets := make([]AIPDDUploadTarget, 0, len(capability.UploadTargets))
		for _, target := range capability.UploadTargets {
			target.Aliases = append([]string(nil), target.Aliases...)
			targets = append(targets, target)
		}
		capability.UploadTargets = targets
	}
	return capability
}

func SetAIPDDCapabilities(capabilities []AIPDDCapability) {
	if len(capabilities) == 0 {
		return
	}
	cloned := cloneAIPDDCapabilities(capabilities)
	aipddCapabilitiesLock.Lock()
	defer aipddCapabilitiesLock.Unlock()
	AIPDDCapabilities = cloned
	AIPDDTaskModelList = buildAIPDDTaskModelList(cloned)
	aipddCapabilityByAlias = buildAIPDDCapabilityByAlias(cloned)
}

func SetAIPDDOpenAIModels(models []string) {
	models = normalizeAIPDDModelList(models)
	aipddCapabilitiesLock.Lock()
	defer aipddCapabilitiesLock.Unlock()
	AIPDDOpenAIModelList = models
}

func ResetAIPDDCapabilities() {
	SetAIPDDCapabilities(defaultAIPDDCapabilities)
}

func ResetAIPDDOpenAIModels() {
	SetAIPDDOpenAIModels(nil)
}

func GetAIPDDCapability(modelName string) (AIPDDCapability, bool) {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	capability, ok := aipddCapabilityByAlias[strings.ToLower(strings.TrimSpace(modelName))]
	return cloneAIPDDCapability(capability), ok
}

func GetDefaultAIPDDCapability(modelName string) (AIPDDCapability, bool) {
	capability, ok := defaultAIPDDCapabilityByAlias[strings.ToLower(strings.TrimSpace(modelName))]
	return cloneAIPDDCapability(capability), ok
}

func GetAIPDDCapabilities() []AIPDDCapability {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return cloneAIPDDCapabilities(AIPDDCapabilities)
}

func GetAIPDDTaskModelList() []string {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return append([]string(nil), AIPDDTaskModelList...)
}

func GetAIPDDOpenAIModelList() []string {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return append([]string(nil), AIPDDOpenAIModelList...)
}

func GetAIPDDModelList() []string {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return mergeAIPDDModelLists(AIPDDTaskModelList, AIPDDOpenAIModelList)
}

func IsAIPDDTaskModel(modelName string) bool {
	_, ok := GetAIPDDCapability(modelName)
	return ok
}

func IsAIPDDOpenAIModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	for _, item := range AIPDDOpenAIModelList {
		if item == modelName {
			return true
		}
	}
	return false
}

func IsAIPDDPerCallBillingModel(modelName string) bool {
	capability, ok := GetAIPDDCapability(modelName)
	return ok && capability.BillingType == AIPDDBillingTypePerCall
}

func GetAIPDDEndpointTypes(modelName string) []EndpointType {
	capability, ok := GetAIPDDCapability(modelName)
	if !ok {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			return nil
		}
		if IsAIPDDOpenAIModel(modelName) {
			return []EndpointType{EndpointTypeOpenAI}
		}
		return []EndpointType{EndpointTypeOpenAI}
	}
	return []EndpointType{capability.EndpointType}
}

func normalizeAIPDDModelList(models []string) []string {
	normalized := make([]string, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || seen[modelName] {
			continue
		}
		normalized = append(normalized, modelName)
		seen[modelName] = true
	}
	return normalized
}

func mergeAIPDDModelLists(lists ...[]string) []string {
	total := 0
	for _, list := range lists {
		total += len(list)
	}
	merged := make([]string, 0, total)
	seen := make(map[string]bool, total)
	for _, list := range lists {
		for _, modelName := range list {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" || seen[modelName] {
				continue
			}
			merged = append(merged, modelName)
			seen[modelName] = true
		}
	}
	return merged
}
