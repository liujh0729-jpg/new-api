package constant

import "strings"

const (
	AIPDDModelFluxGGUF      = "aipdd-flux-gguf"
	AIPDDModelWan22Wanx     = "aipdd-wan2.2-wanx"
	AIPDDModelWan22Animater = "aipdd-wan2.2-animater"
	AIPDDModelMimicMotion   = "aipdd-mimic-motion"
	AIPDDModelLatentsync15  = "aipdd-latentsync-1.5"
	AIPDDModelIndexTTS      = "aipdd-indextts"
)

type AIPDDBillingType string

const (
	AIPDDBillingTypePerCall         AIPDDBillingType = "per_call"
	AIPDDBillingTypeDurationSeconds AIPDDBillingType = "duration_seconds"
)

type AIPDDCapability struct {
	ModelName              string
	ScriptID               string
	ScriptCode             string
	TaskCost               float64
	WorkflowParamKeys      []string
	RequiredWorkflowParams map[string]bool
	EndpointType           EndpointType
	BillingType            AIPDDBillingType
}

const AIPDDWan22WanxRMBPerSecond = 0.02

var AIPDDCapabilities = []AIPDDCapability{
	{
		ModelName:         AIPDDModelFluxGGUF,
		ScriptID:          "c1d4d41c-0d5a-4bf8-bfdb-548d7a710759",
		ScriptCode:        "FLUX_GGUF",
		TaskCost:          0,
		WorkflowParamKeys: []string{"positive_prompt", "negative_prompt"},
		RequiredWorkflowParams: map[string]bool{
			"positive_prompt": false,
			"negative_prompt": false,
		},
		EndpointType: EndpointTypeImageGeneration,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelWan22Wanx,
		ScriptID:          "3eae5a25-98cf-4658-aa9f-c48bb41043a6",
		ScriptCode:        "aipdd_wan2.2_wanx",
		TaskCost:          5000,
		WorkflowParamKeys: []string{"image", "prompt", "positive_prompt", "duration"},
		RequiredWorkflowParams: map[string]bool{
			"image":           true,
			"prompt":          true,
			"positive_prompt": true,
			"duration":        false,
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypeDurationSeconds,
	},
	{
		ModelName:  AIPDDModelWan22Animater,
		ScriptID:   "4f1401c1-9791-406e-8ce2-4808f9b95d9c",
		ScriptCode: "aipdd_Wan2.2-Animater",
		TaskCost:   10000,
		WorkflowParamKeys: []string{
			"load_video",
			"filename",
			"fullpath",
			"WanVideoTextEncodeCached_positive_prompt",
			"WanVideoTextEncodeCached_negative_prompt",
		},
		RequiredWorkflowParams: map[string]bool{
			"load_video": true,
			"filename":   true,
			"fullpath":   false,
			"WanVideoTextEncodeCached_positive_prompt": true,
			"WanVideoTextEncodeCached_negative_prompt": true,
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelMimicMotion,
		ScriptID:          "0628aec4-ab5e-4b3f-a453-760bcb8bfeaf",
		ScriptCode:        "aipdd_mimic_motion",
		TaskCost:          0,
		WorkflowParamKeys: []string{"motion_video", "appearance_image"},
		RequiredWorkflowParams: map[string]bool{
			"motion_video":     true,
			"appearance_image": true,
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelLatentsync15,
		ScriptID:          "57971711-0255-46b1-893a-10d7216f42a0",
		ScriptCode:        "aipdd_latentsync1.5",
		TaskCost:          0,
		WorkflowParamKeys: []string{"video", "filename", "LoadAudio"},
		RequiredWorkflowParams: map[string]bool{
			"video":     true,
			"filename":  true,
			"LoadAudio": true,
		},
		EndpointType: EndpointTypeOpenAIVideo,
		BillingType:  AIPDDBillingTypePerCall,
	},
	{
		ModelName:         AIPDDModelIndexTTS,
		ScriptID:          "eba39b43-6c3b-4930-b0c9-a492706fa464",
		ScriptCode:        "aipdd_IndexTTS",
		TaskCost:          5000,
		WorkflowParamKeys: []string{"audio", "emotion_audio", "text"},
		RequiredWorkflowParams: map[string]bool{
			"audio":         true,
			"emotion_audio": false,
			"text":          true,
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
