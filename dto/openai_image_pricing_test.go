package dto

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func withAIPDDImageCapability(t *testing.T, pricing *constant.AIPDDImagePricing) {
	previous := constant.GetAIPDDCapabilities()
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{
			ModelName:     "ap-image-tiered",
			EndpointType:  constant.EndpointTypeImageGeneration,
			EndpointTypes: []constant.EndpointType{constant.EndpointTypeImageGeneration},
			Synchronous:   true,
			ImagePricing:  pricing,
		},
	})
	t.Cleanup(func() { constant.SetAIPDDCapabilities(previous) })
}

func tieredImagePricing() *constant.AIPDDImagePricing {
	return &constant.AIPDDImagePricing{
		DefaultUnitAWCoin: 2000,
		Tiers: []constant.AIPDDImagePricingTier{
			{Match: map[string]string{"quality": "1K"}, UnitAWCoin: 3000},
			{Match: map[string]string{"quality": "4K"}, UnitAWCoin: 6000},
		},
	}
}

func TestImageRequestPricingAppliesAIPDDTier(t *testing.T) {
	withAIPDDImageCapability(t, tieredImagePricing())

	cases := []struct {
		quality string
		size    string
		want    float64
	}{
		{want: 1},
		{quality: "1K", want: 1.5},
		{quality: "4K", want: 3},
		{size: "3840x2160", want: 3},
	}
	for _, tc := range cases {
		request := &ImageRequest{Model: "ap-image-tiered", Prompt: "a red cube", Quality: tc.quality, Size: tc.size}
		got := request.GetTokenCountMeta().ImagePriceRatio
		if math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("quality=%q size=%q ImagePriceRatio = %v, want %v", tc.quality, tc.size, got, tc.want)
		}
	}
}

func TestImageRequestPricingKeepsDallEBehaviour(t *testing.T) {
	withAIPDDImageCapability(t, tieredImagePricing())

	request := &ImageRequest{Model: "dall-e-3", Prompt: "a red cube", Quality: "hd", Size: "1024x1024"}
	if got := request.GetTokenCountMeta().ImagePriceRatio; math.Abs(got-2) > 1e-9 {
		t.Fatalf("dall-e-3 hd 1024x1024 ImagePriceRatio = %v, want 2", got)
	}
}
