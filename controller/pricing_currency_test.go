package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/require"
)

func TestConvertPricingMonetaryFieldsToCNY(t *testing.T) {
	input := []model.Pricing{
		{
			ModelName:  "fixed",
			QuotaType:  1,
			ModelPrice: 0.25,
		},
		{
			ModelName:  "task",
			QuotaType:  1,
			ModelPrice: 0.04,
			TaskPricing: &billing_setting.TaskPricingConfig{
				Unit:                      billing_setting.TaskPricingUnitSecond,
				NoReferenceVideoUnitPrice: 0.04,
				ReferenceVideoPolicy:      billing_setting.ReferenceVideoPolicyCustom,
				ReferenceVideoUnitPrice:   0.06,
				ByResolution: map[string]billing_setting.TaskPricingTier{
					"720p": {
						NoReferenceVideoUnitPrice: 0.08,
						ReferenceVideoPolicy:      billing_setting.ReferenceVideoPolicyCustom,
						ReferenceVideoUnitPrice:   0.12,
					},
				},
			},
		},
	}

	converted := convertPricingMonetaryFieldsToCNY(input, 7.3)

	require.InDelta(t, 1.825, converted[0].ModelPrice, 1e-12)
	require.InDelta(t, 0.292, converted[1].ModelPrice, 1e-12)
	require.InDelta(t, 0.292, converted[1].TaskPricing.NoReferenceVideoUnitPrice, 1e-12)
	require.InDelta(t, 0.438, converted[1].TaskPricing.ReferenceVideoUnitPrice, 1e-12)
	require.InDelta(t, 0.584, converted[1].TaskPricing.ByResolution["720p"].NoReferenceVideoUnitPrice, 1e-12)
	require.InDelta(t, 0.876, converted[1].TaskPricing.ByResolution["720p"].ReferenceVideoUnitPrice, 1e-12)

	// The response conversion must not mutate the internal USD pricing snapshot.
	require.Equal(t, 0.25, input[0].ModelPrice)
	require.Equal(t, 0.08, input[1].TaskPricing.ByResolution["720p"].NoReferenceVideoUnitPrice)
}

func TestPricingResponseAmountToUSD(t *testing.T) {
	amount, err := pricingResponseAmountToUSD(2.48, "CNY", 7.3)
	require.NoError(t, err)
	require.InDelta(t, 0.3397260273972603, amount, 1e-12)

	amount, err = pricingResponseAmountToUSD(0.25, "", 0)
	require.NoError(t, err)
	require.Equal(t, 0.25, amount)

	_, err = pricingResponseAmountToUSD(2.48, "CNY", 0)
	require.Error(t, err)
}
