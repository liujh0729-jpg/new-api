package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestBuildPublicTokenUsageDataUsesCNY(t *testing.T) {
	token := &model.Token{
		Name:               "public-key",
		RemainQuota:        80_000,
		UsedQuota:          20_000,
		UnlimitedQuota:     false,
		ModelLimitsEnabled: false,
	}

	data := buildPublicTokenUsageData(token, 0, 500_000, 7.3)

	require.Equal(t, "CNY", data.Currency)
	require.Equal(t, 1.46, data.TotalGranted)
	require.Equal(t, 0.292, data.TotalUsed)
	require.Equal(t, 1.168, data.TotalAvailable)
	require.Equal(t, 100_000, data.QuotaTotalGranted)
	require.Equal(t, 20_000, data.QuotaTotalUsed)
	require.Equal(t, 80_000, data.QuotaTotalAvailable)
	require.Equal(t, 500_000.0, data.QuotaPerUnit)
	require.Equal(t, 7.3, data.USDExchangeRate)
}
