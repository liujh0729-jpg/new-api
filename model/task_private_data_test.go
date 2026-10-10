package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskPrivateDataScanAcceptsLegacyAIPDDFinanceStringIDs(t *testing.T) {
	var privateData TaskPrivateData
	require.NoError(t, privateData.Scan([]byte(`{
		"aipdd_finance":{
			"instance_id":"instance-1",
			"platform_order_id":"order-1",
			"newapi_user_id":"12",
			"newapi_token_id":"34",
			"channel_id":56
		}
	}`)))

	require.NotNil(t, privateData.AIPDDFinance)
	require.Equal(t, 12, privateData.AIPDDFinance.NewAPIUserID)
	require.Equal(t, 34, privateData.AIPDDFinance.NewAPITokenID)
}
