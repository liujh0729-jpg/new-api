package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
)

func TestVirtualCharacterRequestHasExternalReferences(t *testing.T) {
	tests := []struct {
		name string
		req  relaycommon.TaskSubmitReq
		want bool
	}{
		{name: "none", req: relaycommon.TaskSubmitReq{}, want: false},
		{name: "empty content", req: relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"content": []interface{}{}}}, want: false},
		{name: "image list", req: relaycommon.TaskSubmitReq{Images: []string{"https://example.com/ref.png"}}, want: true},
		{name: "first frame", req: relaycommon.TaskSubmitReq{FirstFrame: "https://example.com/ref.png"}, want: true},
		{name: "metadata content", req: relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "image_url"}}}}, want: true},
		{name: "malformed content", req: relaycommon.TaskSubmitReq{Metadata: map[string]interface{}{"content": "unexpected"}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, virtualCharacterRequestHasExternalReferences(test.req))
		})
	}
}

func TestChannelSupportsVirtualCharacterModelUsesExactNames(t *testing.T) {
	channel := &model.Channel{Models: "other-model, doubao-seedance-2-0-260128"}
	require.True(t, channelSupportsVirtualCharacterModel(channel, "doubao-seedance-2-0-260128"))
	require.False(t, channelSupportsVirtualCharacterModel(channel, "seedance-2-0"))
}
