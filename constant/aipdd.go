package constant

import "strings"

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
		TaskCost:          200,
		WorkflowParamKeys: []string{"image", "positive_prompt", "negative_prompt"},
		RequiredWorkflowParams: map[string]bool{
			"image":           false,
			"positive_prompt": false,
			"negative_prompt": false,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("image", aipddMetadata("image"), aipddSource(AIPDDWorkflowSourceImage), aipddSource(AIPDDWorkflowSourceFirstImage)),
			aipddStringDefault("positive_prompt", aipddMetadata("positive_prompt"), aipddMetadata("prompt"), aipddSource(AIPDDWorkflowSourcePrompt)),
			aipddStringDefault("negative_prompt", aipddMetadata("negative_prompt")),
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
		TaskCost:          200,
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
		WorkflowParamKeys: []string{"image", "prompt", "positive_prompt", "duration"},
		RequiredWorkflowParams: map[string]bool{
			"image":           true,
			"prompt":          true,
			"positive_prompt": true,
			"duration":        false,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("image", aipddMetadata("image"), aipddSource(AIPDDWorkflowSourceImage), aipddSource(AIPDDWorkflowSourceFirstImage)),
			aipddStringDefault("prompt", aipddMetadata("prompt"), aipddSource(AIPDDWorkflowSourcePrompt)),
			aipddStringDefault("positive_prompt", aipddMetadata("positive_prompt"), aipddMetadata("prompt"), aipddSource(AIPDDWorkflowSourcePrompt)),
			aipddIntDefault("duration", aipddSource(AIPDDWorkflowSourceDuration)),
		},
		UploadTargets: []AIPDDUploadTarget{
			{ParamKey: "image", Aliases: []string{"file", "input_reference", "reference", "images"}},
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypeDurationSeconds,
	},
	{
		ModelName:  AIPDDModelWan22Animater,
		ScriptID:   "4f1401c1-9791-406e-8ce2-4808f9b95d9c",
		ScriptCode: "aipdd_Wan2.2-Animater",
		TaskCost:   2000,
		WorkflowParamKeys: []string{
			"load_video",
			"fullpath",
			"positive_prompt",
			"negative_prompt",
		},
		RequiredWorkflowParams: map[string]bool{
			"load_video":      true,
			"fullpath":        false,
			"positive_prompt": true,
			"negative_prompt": true,
		},
		WorkflowDefaults: []AIPDDWorkflowParamDefault{
			aipddStringDefault("load_video", aipddMetadata("load_video"), aipddMetadata("video"), aipddSource(AIPDDWorkflowSourceInputReference), aipddSource(AIPDDWorkflowSourceImage), aipddSource(AIPDDWorkflowSourceFirstImage)),
			aipddStringDefault("positive_prompt", aipddMetadata("positive_prompt"), aipddMetadata("prompt"), aipddSource(AIPDDWorkflowSourcePrompt)),
			aipddStringDefault("negative_prompt", aipddMetadata("negative_prompt")),
		},
		UploadTargets: []AIPDDUploadTarget{
			{ParamKey: "load_video", Aliases: []string{"file", "input_reference", "reference", "video"}},
			{ParamKey: "fullpath"},
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

var AIPDDTaskModelList = func() []string {
	models := make([]string, 0, len(AIPDDCapabilities))
	for _, capability := range AIPDDCapabilities {
		models = append(models, capability.ModelName)
	}
	return models
}()

var aipddCapabilityByAlias = func() map[string]AIPDDCapability {
	out := make(map[string]AIPDDCapability, len(AIPDDCapabilities)*2)
	for _, capability := range AIPDDCapabilities {
		out[strings.ToLower(capability.ModelName)] = capability
		out[strings.ToLower(capability.ScriptCode)] = capability
	}
	return out
}()

func GetAIPDDCapability(modelName string) (AIPDDCapability, bool) {
	capability, ok := aipddCapabilityByAlias[strings.ToLower(strings.TrimSpace(modelName))]
	return capability, ok
}

func GetAIPDDCapabilities() []AIPDDCapability {
	capabilities := make([]AIPDDCapability, len(AIPDDCapabilities))
	copy(capabilities, AIPDDCapabilities)
	return capabilities
}

func IsAIPDDTaskModel(modelName string) bool {
	_, ok := GetAIPDDCapability(modelName)
	return ok
}

func IsAIPDDPerCallBillingModel(modelName string) bool {
	capability, ok := GetAIPDDCapability(modelName)
	return ok && capability.BillingType == AIPDDBillingTypePerCall
}

func GetAIPDDEndpointTypes(modelName string) []EndpointType {
	capability, ok := GetAIPDDCapability(modelName)
	if !ok {
		return nil
	}
	return []EndpointType{capability.EndpointType}
}
