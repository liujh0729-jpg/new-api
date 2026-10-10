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
		AIPDDFinance: &relaycommon.AIPDDFinanceContext{
			InstanceID: "instance-id", PlatformOrderID: "order-id", AttemptID: "order-id:2:7",
			NewAPIUserID: 11, NewAPITokenID: 22,
		},
	}
	header := http.Header{}

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	require.Equal(t, "aipdd-key", header.Get("X-API-Key"))
	require.Empty(t, header.Get("Authorization"))
	require.Equal(t, "instance-id", header.Get("X-AIPDD-Instance-ID"))
	require.Equal(t, "order-id", header.Get("X-AIPDD-Order-ID"))
	require.Equal(t, "order-id:2:7", header.Get("X-AIPDD-Attempt-ID"))
	require.Equal(t, "11", header.Get("X-AIPDD-NewAPI-User-ID"))
	require.Equal(t, "22", header.Get("X-AIPDD-NewAPI-Token-ID"))
}

// A deployment without a registered delivery site must not advertise an
// unverified instance identity upstream, otherwise AIPDD refuses the compute.
func TestSetupRequestHeaderOmitsFinanceHeadersWithoutSiteIdentity(t *testing.T) {
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
			InstanceID: "", PlatformOrderID: "order-id", AttemptID: "order-id:0:1",
			NewAPIUserID: 11, NewAPITokenID: 22,
		},
	}
	header := http.Header{}

	require.NoError(t, (&Adaptor{}).SetupRequestHeader(c, &header, info))
	require.Equal(t, "aipdd-key", header.Get("X-API-Key"))
	for _, key := range []string{
		"X-AIPDD-Instance-ID", "X-AIPDD-Order-ID", "X-AIPDD-Attempt-ID",
		"X-AIPDD-NewAPI-User-ID", "X-AIPDD-NewAPI-Token-ID",
	} {
		require.Empty(t, header.Get(key), key+" must not be sent without a site identity")
	}
}
