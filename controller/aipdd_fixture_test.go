package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

// Catalog-shaped AIPDD models used by the endpoint/pricing tests. AIPDD ships no
// built-in default models, so every test that expects AIPDD models has to install
// what an upstream catalog sync would have produced.
const (
	aipddFixtureImageToImage = "fixture-aipdd-image"
	aipddFixtureTextToImage  = "fixture-aipdd-t2i"
	aipddFixtureTextToSpeech = "fixture-aipdd-tts"
)

func useAIPDDCatalogCapabilities(t *testing.T) {
	t.Helper()

	original := constant.GetAIPDDCapabilities()
	originalOpenAIModels := constant.GetAIPDDOpenAIModelList()
	t.Cleanup(func() {
		constant.SetAIPDDCapabilities(original)
		constant.SetAIPDDOpenAIModels(originalOpenAIModels)
	})

	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{
			ModelName:     aipddFixtureImageToImage,
			AdapterCode:   "comfyui",
			ScriptCode:    "FixtureImageToImage",
			TaskKind:      "image_to_image",
			TaskCost:      0.02,
			EndpointType:  constant.EndpointTypeImageGeneration,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration, constant.EndpointTypeImageEdits},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
		{
			ModelName:     aipddFixtureTextToImage,
			AdapterCode:   "comfyui",
			ScriptCode:    "FixtureTextToImage",
			TaskKind:      "image_generation",
			TaskCost:      0.02,
			EndpointType:  constant.EndpointTypeImageGeneration,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
		{
			ModelName:     aipddFixtureTextToSpeech,
			AdapterCode:   "comfyui",
			ScriptCode:    "FixtureTextToSpeech",
			TaskKind:      "text_to_speech",
			TaskCost:      0.01,
			EndpointType:  constant.EndpointTypeAudioSpeech,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeAudioSpeech},
			BillingType:   constant.AIPDDBillingTypePerCall,
		},
	})
}
