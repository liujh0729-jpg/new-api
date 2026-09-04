package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestImageEndpointDefaultsMatchOpenAITransport(t *testing.T) {
	generation, ok := GetDefaultEndpointInfo(constant.EndpointTypeImageGeneration)
	require.True(t, ok)
	require.Equal(t, "/v1/images/generations", generation.Path)

	imageToImage, ok := GetDefaultEndpointInfo(constant.EndpointTypeImageToImage)
	require.True(t, ok)
	require.Equal(t, "/v1/images/edits", imageToImage.Path)

	imageEdit, ok := GetDefaultEndpointInfo(constant.EndpointTypeImageEdit)
	require.True(t, ok)
	require.Equal(t, "/v1/images/edits", imageEdit.Path)
}

func TestIndependentSeedanceChannelAdvertisesVideoEndpointRegardlessOfAlias(t *testing.T) {
	require.Equal(t,
		[]constant.EndpointType{constant.EndpointTypeOpenAIVideo},
		GetEndpointTypesByChannelType(constant.ChannelTypeSeedance, "Public cinematic video"),
	)
}
