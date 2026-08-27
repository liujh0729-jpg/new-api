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
	t.Setenv(aipddTokenMarketPrivacyAcceptedEnv, "2026-08-v1")
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeAIPDD,
			ApiKey:      "aipdd-key",
		},
		AIPDDFinance: &relaycommon.AIPDDFinanceContext{
			InstanceID: "instance-id", PlatformOrderID: "order-id", AttemptID: "order-id:2:7",
			NewAPIUserID: 11, NewAPITokenID: 22,
		},
	}
	header := http.Header{}

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	require.Equal(t, "aipdd-key", header.Get("X-API-Key"))
	require.Equal(t, "2026-08-v1", header.Get("X-AIPDD-Token-Market-Privacy-Accepted"))
	require.Empty(t, header.Get("Authorization"))
	require.Equal(t, "instance-id", header.Get("X-AIPDD-Instance-ID"))
	require.Equal(t, "order-id", header.Get("X-AIPDD-Order-ID"))
	require.Equal(t, "order-id:2:7", header.Get("X-AIPDD-Attempt-ID"))
	require.Equal(t, "11", header.Get("X-AIPDD-NewAPI-User-ID"))
	require.Equal(t, "22", header.Get("X-AIPDD-NewAPI-Token-ID"))
}

func TestSetupRequestHeaderDoesNotImplicitlyAcceptTokenMarketPrivacy(t *testing.T) {
	t.Setenv(aipddTokenMarketPrivacyAcceptedEnv, "")
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeAIPDD,
		ApiKey:      "aipdd-key",
	}}
	header := http.Header{}

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	require.Empty(t, header.Get("X-AIPDD-Token-Market-Privacy-Accepted"))
}
