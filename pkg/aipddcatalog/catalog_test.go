package aipddcatalog

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestBuildCapabilityUsesLTXDefaultsWhenCatalogParamsAreEmpty(t *testing.T) {
	capability, _, ok := buildCapability(Script{
		ID:              constant.AIPDDModelLTX23,
		Code:            constant.AIPDDModelLTX23,
		EndpointType:    string(constant.EndpointTypeOpenAIVideo),
		TaskKind:        "image_to_video",
		InputModalities: []string{"text", "image"},
	}, nil)

	require.True(t, ok)
	require.Contains(t, capability.WorkflowParamKeys, "prompt")
	require.Contains(t, capability.WorkflowParamKeys, "image")
	require.True(t, capability.RequiredWorkflowParams["prompt"])
	require.NotEmpty(t, capability.WorkflowDefaults)
}
