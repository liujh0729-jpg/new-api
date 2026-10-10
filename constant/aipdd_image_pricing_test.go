package constant

import (
	"math"
	"testing"
)

// The tables below are transcribed from the live /v1/new-api/catalog imagePricing
// blocks (rmbPerAwcoin = 0.0001, so 0.2 RMB == 2000 awcoin).

func pixelsPricing() *AIPDDImagePricing {
	return &AIPDDImagePricing{
		DefaultUnitAWCoin: 2000,
		Tiers: []AIPDDImagePricingTier{
			{Match: map[string]string{"quality": "4K"}, UnitAWCoin: 6000},
			{Match: map[string]string{"quality": "2K"}, UnitAWCoin: 4000},
			{Match: map[string]string{"quality": "1K"}, UnitAWCoin: 3000},
			{Match: map[string]string{"quality": "auto"}, UnitAWCoin: 2000},
			{Match: map[string]string{"size": "4096x4096"}, UnitAWCoin: 6000},
			{Match: map[string]string{"size": "2048x2048"}, UnitAWCoin: 4000},
			{Match: map[string]string{"size": "1024x1024"}, UnitAWCoin: 3000},
			{Match: map[string]string{"size": "1024x1536"}, UnitAWCoin: 3000},
			{Match: map[string]string{"size": "1536x1024"}, UnitAWCoin: 3000},
			{Match: map[string]string{"size": "auto"}, UnitAWCoin: 2000},
		},
	}
}

func qualityPricing(defaultUnit float64) *AIPDDImagePricing {
	return &AIPDDImagePricing{
		DefaultUnitAWCoin: defaultUnit,
		Tiers: []AIPDDImagePricingTier{
			{Match: map[string]string{"quality": "auto"}, UnitAWCoin: defaultUnit},
			{Match: map[string]string{"quality": "1K"}, UnitAWCoin: defaultUnit * 1.5},
			{Match: map[string]string{"quality": "2K"}, UnitAWCoin: defaultUnit * 2},
			{Match: map[string]string{"quality": "4K"}, UnitAWCoin: defaultUnit * 3},
		},
	}
}

func sizeBucketPricing() *AIPDDImagePricing {
	return &AIPDDImagePricing{
		DefaultUnitAWCoin: 2400,
		Tiers: []AIPDDImagePricingTier{
			{Match: map[string]string{"size": "auto"}, UnitAWCoin: 2400},
			{Match: map[string]string{"size": "1K"}, UnitAWCoin: 2400},
			{Match: map[string]string{"size": "2K"}, UnitAWCoin: 3000},
			{Match: map[string]string{"size": "4K"}, UnitAWCoin: 3600},
		},
	}
}

func nanoPricing() *AIPDDImagePricing {
	return &AIPDDImagePricing{
		DefaultUnitAWCoin: 5000,
		Tiers: []AIPDDImagePricingTier{
			{Match: map[string]string{"size": "1K"}, UnitAWCoin: 4000},
			{Match: map[string]string{"size": "2K"}, UnitAWCoin: 5000},
			{Match: map[string]string{"size": "4K"}, UnitAWCoin: 6000},
			{Match: map[string]string{"size": "1024x1024"}, UnitAWCoin: 4000},
			{Match: map[string]string{"size": "2048x2048"}, UnitAWCoin: 5000},
			{Match: map[string]string{"size": "4096x4096"}, UnitAWCoin: 6000},
		},
	}
}

func TestAIPDDImagePricingRatioForRealCatalogTables(t *testing.T) {
	cases := []struct {
		name    string
		pricing *AIPDDImagePricing
		quality string
		size    string
		want    float64
	}{
		{name: "ap-image-2.0 auto+1024x1024", pricing: pixelsPricing(), quality: "auto", size: "1024x1024", want: 1.5},
		{name: "ap-image-2.0 1K", pricing: pixelsPricing(), quality: "1K", want: 1.5},
		{name: "ap-image-2.0 4096x4096", pricing: pixelsPricing(), size: "4096x4096", want: 3},
		{name: "ap-image-2.0 empty request", pricing: pixelsPricing(), want: 1},
		{name: "ap-image-2.0 listed size is not rounded up", pricing: pixelsPricing(), size: "1024x1536", want: 1.5},
		{name: "ap-image-2.0 unlisted size rounds up", pricing: pixelsPricing(), size: "3840x2160", want: 3},
		{name: "flare-2.5 2K", pricing: qualityPricing(2000), quality: "2K", want: 2},
		{name: "flare-2.5 pixel size derives class", pricing: qualityPricing(2000), size: "3840x2160", want: 3},
		{name: "flare-2.5 1024x1024 derives class", pricing: qualityPricing(2000), size: "1024x1024", want: 1.5},
		{name: "sunburst-2.5 auto", pricing: qualityPricing(3000), quality: "auto", want: 1},
		{name: "lite-2.0 1024x1024", pricing: sizeBucketPricing(), size: "1024x1024", want: 1},
		{name: "lite-2.0 4K", pricing: sizeBucketPricing(), size: "4K", want: 1.5},
		{name: "lite-2.0 2048x2048", pricing: sizeBucketPricing(), size: "2048x2048", want: 1.25},
		{name: "nano-2.1 1K is cheaper than base", pricing: nanoPricing(), size: "1K", want: 0.8},
		{name: "nano-2.1 4096x4096", pricing: nanoPricing(), size: "4096x4096", want: 1.2},
		{name: "nano-2.1 2048x2160 rounds up", pricing: nanoPricing(), size: "2048x2160", want: 1.2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.pricing.RatioFor(tc.quality, tc.size)
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("RatioFor(%q, %q) = %v, want %v", tc.quality, tc.size, got, tc.want)
			}
		})
	}
}

func TestAIPDDImagePricingRatioForFallsBack(t *testing.T) {
	for _, pricing := range []*AIPDDImagePricing{nil, {}, {DefaultUnitAWCoin: 1000}} {
		if got := pricing.RatioFor("4K", "4096x4096"); got != 1 {
			t.Fatalf("RatioFor with no usable tiers = %v, want 1", got)
		}
	}
	invalid := &AIPDDImagePricing{
		DefaultUnitAWCoin: 0,
		Tiers:             []AIPDDImagePricingTier{{Match: map[string]string{"size": "4K"}, UnitAWCoin: 1000}},
	}
	if got := invalid.RatioFor("", "4K"); got != 1 {
		t.Fatalf("RatioFor with zero default unit = %v, want 1", got)
	}
}

func TestAIPDDImagePricingTiePicksMostExpensiveTier(t *testing.T) {
	unit, ok := pixelsPricing().UnitAWCoinFor("4K", "2048x2048")
	if !ok || unit != 6000 {
		t.Fatalf("UnitAWCoinFor = %v, %v; want 6000, true", unit, ok)
	}
}

func TestAIPDDImagePricingIgnoresUnknownMatchKeys(t *testing.T) {
	pricing := &AIPDDImagePricing{
		DefaultUnitAWCoin: 1000,
		Tiers: []AIPDDImagePricingTier{
			{Match: map[string]string{"format": "webp"}, UnitAWCoin: 9000},
			{Match: map[string]string{}, UnitAWCoin: 9000},
			{Match: map[string]string{"quality": "4K", "size": "1024x1024"}, UnitAWCoin: 9000},
		},
	}
	if got := pricing.RatioFor("", "4K"); got != 1 {
		t.Fatalf("RatioFor = %v, want 1 (no tier is satisfiable)", got)
	}
}

func TestAIPDDImagePricingRequiresEveryMatchKey(t *testing.T) {
	pricing := pixelsPricing()
	pricing.Tiers = append(pricing.Tiers, AIPDDImagePricingTier{
		Match:      map[string]string{"quality": "1K", "size": "4096x4096"},
		UnitAWCoin: 12000,
	})
	if got := pricing.RatioFor("1K", ""); got != 1.5 {
		t.Fatalf("RatioFor = %v, want 1.5 (partial composite tier must not match)", got)
	}
	if got := pricing.RatioFor("1K", "4096x4096"); got != 6 {
		t.Fatalf("RatioFor = %v, want 6 (composite tier must match)", got)
	}
}
