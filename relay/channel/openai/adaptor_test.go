package openai

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupRequestHeaderUsesAIPDDAPIKeyHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeAIPDD,
			ApiKey:      "aipdd-key",
		},
	}
	header := http.Header{}

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	require.Equal(t, "aipdd-key", header.Get("X-API-Key"))
	require.Empty(t, header.Get("Authorization"))
}
