package aipdd

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

// ComfyUI model names the gateway used to ship as built-in defaults. They are
// test fixtures now: AIPDD models come only from the upstream catalog.
const (
	legacyFluxGGUF      = "aipdd-flux-gguf"
	legacyFluxGGUFT2I   = "aipdd-flux-gguf-t2i"
	legacyWan22Wanx     = "aipdd-wan2.2-wanx"
	legacyWan22Animater = "aipdd-wan2.2-animater"
	legacyMimicMotion   = "aipdd-mimic-motion"
	legacyLatentsync15  = "aipdd-latentsync-1.5"
	legacyIndexTTS      = "aipdd-indextts"
)

func legacyMetadata(key string) constant.AIPDDWorkflowValueSource {
	return constant.AIPDDWorkflowValueSource{Type: constant.AIPDDWorkflowSourceMetadata, Key: key}
}

func legacySource(sourceType constant.AIPDDWorkflowSourceType) constant.AIPDDWorkflowValueSource {
	return constant.AIPDDWorkflowValueSource{Type: sourceType}
}

func legacyStringDefault(paramKey string, sources ...constant.AIPDDWorkflowValueSource) constant.AIPDDWorkflowParamDefault {
	return constant.AIPDDWorkflowParamDefault{
		ParamKey:  paramKey,
		ValueType: constant.AIPDDWorkflowValueTypeString,
		Sources:   sources,
	}
}

// legacyComfyUICapabilities reproduces the workflow parameter shapes the
// payload builder is tested against.
func legacyComfyUICapabilities() []constant.AIPDDCapability {
	return []constant.AIPDDCapability{
		{
			ModelName:              legacyFluxGGUF,
			ScriptID:               "c1d4d41c-0d5a-4bf8-bfdb-548d7a710759",
			ScriptCode:             "FLUX-GGUF-V2",
			TaskCost:               1000,
			WorkflowParamKeys:      []string{"image", "positive_prompt"},
			RequiredWorkflowParams: map[string]bool{"image": true, "positive_prompt": true},
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				legacyStringDefault("image", legacyMetadata("image"), legacySource(constant.AIPDDWorkflowSourceImage), legacySource(constant.AIPDDWorkflowSourceFirstImage)),
				legacyStringDefault("positive_prompt", legacyMetadata("positive_prompt"), legacyMetadata("prompt"), legacySource(constant.AIPDDWorkflowSourcePrompt)),
			},
			EndpointType:  constant.EndpointTypeImageGeneration,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
		{
			ModelName:              legacyFluxGGUFT2I,
			ScriptID:               "aa6e64ce-bc73-4295-b78a-a269e5d3c1a9",
			ScriptCode:             "FLUX-GGUF-T2I-V2",
			TaskCost:               1000,
			WorkflowParamKeys:      []string{"text"},
			RequiredWorkflowParams: map[string]bool{"text": true},
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				legacyStringDefault("text", legacyMetadata("text"), legacyMetadata("input"), legacyMetadata("prompt"), legacySource(constant.AIPDDWorkflowSourcePrompt)),
			},
			EndpointType:  constant.EndpointTypeImageGeneration,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
		{
			ModelName:         legacyWan22Wanx,
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
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				legacyStringDefault("image", legacyMetadata("image"), legacySource(constant.AIPDDWorkflowSourceImage), legacySource(constant.AIPDDWorkflowSourceFirstImage)),
				legacyStringDefault("prompt", legacyMetadata("prompt"), legacySource(constant.AIPDDWorkflowSourcePrompt)),
				legacyStringDefault("negative_prompt", legacyMetadata("negative_prompt")),
				legacyStringDefault("fps", legacyMetadata("fps")),
			},
			EndpointType:  constant.EndpointTypeOpenAIVideo,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
		{
			ModelName:  legacyWan22Animater,
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
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				legacyStringDefault("video", legacyMetadata("video"), legacyMetadata("load_video"), legacySource(constant.AIPDDWorkflowSourceInputReference), legacySource(constant.AIPDDWorkflowSourceImage), legacySource(constant.AIPDDWorkflowSourceFirstImage)),
				legacyStringDefault("image", legacyMetadata("image"), legacyMetadata("fullpath"), legacyMetadata("reference_image"), legacySource(constant.AIPDDWorkflowSourceFirstImage), legacySource(constant.AIPDDWorkflowSourceImage)),
				legacyStringDefault("positive_prompt", legacyMetadata("positive_prompt"), legacyMetadata("prompt"), legacySource(constant.AIPDDWorkflowSourcePrompt)),
				legacyStringDefault("filename_prefix", legacyMetadata("filename_prefix")),
			},
			EndpointType:  constant.EndpointTypeOpenAIVideo,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
		{
			ModelName:              legacyMimicMotion,
			ScriptID:               "0628aec4-ab5e-4b3f-a453-760bcb8bfeaf",
			ScriptCode:             "aipdd_mimic_motion",
			TaskCost:               2000,
			WorkflowParamKeys:      []string{"motion_video", "appearance_image"},
			RequiredWorkflowParams: map[string]bool{"motion_video": true, "appearance_image": true},
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				legacyStringDefault("motion_video", legacyMetadata("motion_video"), legacyMetadata("video"), legacyMetadata("load_video"), legacySource(constant.AIPDDWorkflowSourceInputReference), legacySource(constant.AIPDDWorkflowSourceImage), legacySource(constant.AIPDDWorkflowSourceFirstImage)),
				legacyStringDefault("appearance_image", legacyMetadata("appearance_image"), legacyMetadata("image"), legacySource(constant.AIPDDWorkflowSourceFirstImage), legacySource(constant.AIPDDWorkflowSourceImage)),
			},
			EndpointType:  constant.EndpointTypeOpenAIVideo,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
		{
			ModelName:              legacyLatentsync15,
			ScriptID:               "57971711-0255-46b1-893a-10d7216f42a0",
			ScriptCode:             "aipdd_latentsync1.5",
			TaskCost:               1000,
			WorkflowParamKeys:      []string{"video", "LoadAudio"},
			RequiredWorkflowParams: map[string]bool{"video": true, "LoadAudio": true},
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				legacyStringDefault("video", legacyMetadata("video"), legacyMetadata("load_video"), legacySource(constant.AIPDDWorkflowSourceInputReference), legacySource(constant.AIPDDWorkflowSourceImage), legacySource(constant.AIPDDWorkflowSourceFirstImage)),
				legacyStringDefault("LoadAudio", legacyMetadata("LoadAudio"), legacyMetadata("audio")),
			},
			EndpointType:  constant.EndpointTypeOpenAIVideo,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAIVideo},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
		{
			ModelName:              legacyIndexTTS,
			ScriptID:               "eba39b43-6c3b-4930-b0c9-a492706fa464",
			ScriptCode:             "aipdd_IndexTTS",
			TaskCost:               1000,
			WorkflowParamKeys:      []string{"audio", "emotion_audio", "text"},
			RequiredWorkflowParams: map[string]bool{"audio": true, "emotion_audio": false, "text": true},
			WorkflowDefaults: []constant.AIPDDWorkflowParamDefault{
				legacyStringDefault("audio", legacyMetadata("audio"), legacyMetadata("ref_audio"), legacyMetadata("reference_audio"), legacyMetadata("voice"), legacySource(constant.AIPDDWorkflowSourceInputReference), legacySource(constant.AIPDDWorkflowSourceFirstImage), legacySource(constant.AIPDDWorkflowSourceImage)),
				legacyStringDefault("emotion_audio", legacyMetadata("emotion_audio")),
				legacyStringDefault("text", legacyMetadata("text"), legacyMetadata("input"), legacySource(constant.AIPDDWorkflowSourcePrompt)),
			},
			EndpointType:  constant.EndpointTypeAudioSpeech,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeAudioSpeech},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
	}
}

// useLegacyComfyUICapabilities installs the fixture for tests that exercise the
// ComfyUI workflow mapping and restores the previous set afterwards.
func useLegacyComfyUICapabilities(t *testing.T) {
	t.Helper()

	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() { constant.SetAIPDDCapabilities(original) })
	constant.SetAIPDDCapabilities(legacyComfyUICapabilities())
}
