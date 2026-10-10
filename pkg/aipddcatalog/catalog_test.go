package aipddcatalog

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

// AIPDD has no built-in workflow fallback: every parameter mapping comes from
// the catalog row itself.
func TestBuildCapabilityDerivesWorkflowParamsFromCatalogParams(t *testing.T) {
	capability, _, ok := buildCapability(Script{
		ID:              "ap-ltx-2.5",
		Code:            "LTX-2.5",
		EndpointType:    string(constant.EndpointTypeOpenAIVideo),
		TaskKind:        "video_generation",
		InputModalities: []string{"text", "image"},
		Params: []ScriptParam{
			{ParamKey: "image", DataType: "string", IsRequired: true},
			{ParamKey: "prompt", DataType: "string", IsRequired: true},
		},
	}, nil)
	require.True(t, ok)

	require.Equal(t, "LTX-2.5", capability.ScriptCode)
	require.Equal(t, []string{"image", "prompt"}, capability.WorkflowParamKeys)
	require.True(t, capability.RequiredWorkflowParams["prompt"])
	require.Len(t, capability.WorkflowDefaults, 2)
	require.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAIVideo}, capability.EndpointTypes)
}

func TestBuildCapabilityWithoutCatalogParamsHasNoWorkflowDefaults(t *testing.T) {
	capability, _, ok := buildCapability(Script{
		ID:       "ap-parameterless",
		Code:     "PARAMETERLESS",
		TaskKind: "image_generation",
	}, nil)
	require.True(t, ok)
	require.Empty(t, capability.WorkflowParamKeys)
	require.Empty(t, capability.WorkflowDefaults)
	require.Equal(t, constant.EndpointTypeImageGeneration, capability.EndpointType)
}
