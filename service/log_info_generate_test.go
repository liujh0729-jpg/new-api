package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testBillingSettler struct {
	preConsumed int
}

func (s *testBillingSettler) Settle(int) error         { return nil }
func (s *testBillingSettler) Refund(*gin.Context)      {}
func (s *testBillingSettler) NeedsRefund() bool        { return false }
func (s *testBillingSettler) GetPreConsumedQuota() int { return s.preConsumed }
func (s *testBillingSettler) Reserve(int) error        { return nil }

func TestResolvePreConsumedQuota_PrefersBillingSession(t *testing.T) {
	info := &relaycommon.RelayInfo{
		FinalPreConsumedQuota: 1000,
		Billing:               &testBillingSettler{preConsumed: 2500},
	}
	assert.Equal(t, 2500, resolvePreConsumedQuota(info))
}

func TestResolvePreConsumedQuota_FallsBackToFinal(t *testing.T) {
	info := &relaycommon.RelayInfo{FinalPreConsumedQuota: 1800}
	assert.Equal(t, 1800, resolvePreConsumedQuota(info))
}

func TestResolvePreConsumedQuota_NilRelayInfo(t *testing.T) {
	assert.Equal(t, 0, resolvePreConsumedQuota(nil))
}

func TestAppendBillingInfo_WritesPreConsumedQuotaIncludingZero(t *testing.T) {
	other := make(map[string]interface{})
	appendBillingInfo(&relaycommon.RelayInfo{FinalPreConsumedQuota: 0}, other)
	require.Contains(t, other, "pre_consumed_quota")
	assert.Equal(t, 0, other["pre_consumed_quota"])
}

func TestGenerateTextOtherInfo_IncludesPreConsumedQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	now := time.Now()
	info := &relaycommon.RelayInfo{
		FinalPreConsumedQuota: 4200,
		Billing:               &testBillingSettler{preConsumed: 5000},
		StartTime:             now,
		FirstResponseTime:     now,
		ChannelMeta:           &relaycommon.ChannelMeta{},
	}
	other := GenerateTextOtherInfo(c, info, 1, 1, 1, 0, 1, 0, 0)
	require.Contains(t, other, "pre_consumed_quota")
	assert.Equal(t, 5000, other["pre_consumed_quota"])
}

func TestGenerateMjOtherInfo_IncludesPreConsumedQuota(t *testing.T) {
	info := &relaycommon.RelayInfo{FinalPreConsumedQuota: 300}
	other := GenerateMjOtherInfo(info, types.PriceData{
		ModelPrice: 0.04,
		GroupRatioInfo: types.GroupRatioInfo{
			GroupRatio: 1,
		},
	})
	require.Contains(t, other, "pre_consumed_quota")
	assert.Equal(t, 300, other["pre_consumed_quota"])
}
