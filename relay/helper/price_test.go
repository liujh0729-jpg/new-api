package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestHasModelBillingConfigRejectsSeedanceAdapterWithoutCatalogPrice(t *testing.T) {
	configSnapshot := config.GlobalConfig.ExportAllConfigs()
	capabilitiesSnapshot := constant.GetAIPDDCapabilities()
	modelPriceSnapshot := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(configSnapshot))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(modelPriceSnapshot))
		constant.SetAIPDDCapabilities(capabilitiesSnapshot)
	})

	const modelName = "seedance-adapter-without-upstream-price"
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName:    modelName,
		AdapterCode:  "seedance",
		EndpointType: constant.EndpointTypeOpenAIVideo,
	}})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{}`,
		"billing_setting.task_pricing": `{}`,
	}))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"seedance-adapter-without-upstream-price":99}`))

	require.True(t, constant.IsAIPDDSeedanceModel(modelName))
	require.False(t, HasModelBillingConfig(modelName), "legacy ModelPrice must not enable an unpriced Seedance task model")
}

func TestAIPDDDoesNotAcceptUnsetRatioAsLocalPricing(t *testing.T) {
	modelPriceSnapshot := ratio_setting.ModelPrice2JSONString()
	modelRatioSnapshot := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(modelPriceSnapshot))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(modelRatioSnapshot))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("group", "default")
	info := &relaycommon.RelayInfo{
		OriginModelName: "new-aipdd-model-without-local-price",
		UserGroup:       "default",
		UsingGroup:      "default",
		UserSetting:     dto.UserSetting{AcceptUnsetRatioModel: true},
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAIPDD},
	}

	_, err := ModelPriceHelperPerCall(ctx, info)
	require.Error(t, err)
}
