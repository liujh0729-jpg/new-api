package constant

import (
	"math"
	"strconv"
	"strings"
)

// aipddImageQualityBuckets maps a resolution class onto the pixel area of its
// square form (1K is 1024x1024, 2K is 2048x2048, 4K is 4096x4096).
var aipddImageQualityBuckets = []struct {
	name string
	area float64
}{
	{name: "1k", area: 1 << 20},
	{name: "2k", area: 4 << 20},
	{name: "4k", area: 16 << 20},
}

// RatioFor returns the retail tier the request selects relative to the base
// per-call price. chargeConfig.amountAwcoin — and therefore the synced model
// price — only carries the cheapest published tier, so billing every request
// from it under-charges high resolutions.
//
// It returns 1 when the request matches no tier, which keeps the base price.
func (pricing *AIPDDImagePricing) RatioFor(quality, size string) float64 {
	if pricing == nil || pricing.DefaultUnitAWCoin <= 0 {
		return 1
	}
	unit, ok := pricing.UnitAWCoinFor(quality, size)
	if !ok {
		return 1
	}
	ratio := unit / pricing.DefaultUnitAWCoin
	if math.IsNaN(ratio) || math.IsInf(ratio, 0) || ratio <= 0 {
		return 1
	}
	return ratio
}

// UnitAWCoinFor returns the price of the tier selected by the requested quality
// and size. The catalog keys tiers either by quality class or by pixel size, and
// the two describe the same thing, so a request token may satisfy either key.
// When several tiers match, the most expensive one wins so a request is never
// billed below a tier it satisfies.
//
// Tier values are matched literally, or by pixel area ("2K" == "2048x2048").
// Only when nothing matches is a pixel size rounded up to its resolution class,
// so a listed size keeps its listed price while an unlisted one (for example
// 3840x2160 on a quality-tiered model) never falls back to the cheapest tier.
func (pricing *AIPDDImagePricing) UnitAWCoinFor(quality, size string) (float64, bool) {
	candidates := aipddImageRequestTokens(quality, size)
	if pricing == nil || len(candidates) == 0 {
		return 0, false
	}
	for _, roundUp := range []bool{false, true} {
		var (
			best  float64
			found bool
		)
		for _, tier := range pricing.Tiers {
			if tier.UnitAWCoin <= 0 {
				continue
			}
			if !tierSatisfiedBy(tier.Match, candidates, roundUp) {
				continue
			}
			if !found || tier.UnitAWCoin > best {
				best = tier.UnitAWCoin
				found = true
			}
		}
		if found {
			return best, true
		}
	}
	return 0, false
}

func tierSatisfiedBy(match map[string]string, candidates []aipddImageToken, roundUp bool) bool {
	if len(match) == 0 {
		return false
	}
	for rawKey, rawValue := range match {
		switch normalizeAIPDDImageToken(rawKey) {
		case "quality", "size":
		default:
			// An unrecognized match key cannot be satisfied by the request.
			return false
		}
		value := newAIPDDImageToken(rawValue)
		if value.text == "" {
			return false
		}
		matched := false
		for _, candidate := range candidates {
			if candidate.satisfies(value, roundUp) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// aipddImageToken is one quality or size token, resolved as far as the published
// vocabularies allow: "2048x2048" and "2K" describe the same image.
type aipddImageToken struct {
	text  string
	area  float64
	class string
}

func newAIPDDImageToken(value string) aipddImageToken {
	token := aipddImageToken{text: normalizeAIPDDImageToken(value)}
	if token.text == "" {
		return token
	}
	if area, ok := aipddImagePixelArea(token.text); ok {
		token.area = area
		token.class = aipddImageQualityBucket(area)
		return token
	}
	if area, ok := aipddImageClassArea(token.text); ok {
		token.area = area
		token.class = token.text
	}
	return token
}

func aipddImageRequestTokens(quality, size string) []aipddImageToken {
	tokens := make([]aipddImageToken, 0, 2)
	for _, value := range []string{quality, size} {
		if token := newAIPDDImageToken(value); token.text != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func (t aipddImageToken) satisfies(value aipddImageToken, roundUp bool) bool {
	if t.text == value.text {
		return true
	}
	if t.area > 0 && t.area == value.area {
		return true
	}
	return roundUp && t.class != "" && t.class == value.class
}

func normalizeAIPDDImageToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// aipddImagePixelArea parses "1024x1536" into its pixel area.
func aipddImagePixelArea(token string) (float64, bool) {
	width, height, ok := strings.Cut(token, "x")
	if !ok {
		return 0, false
	}
	w, err := strconv.ParseFloat(strings.TrimSpace(width), 64)
	if err != nil || w <= 0 {
		return 0, false
	}
	h, err := strconv.ParseFloat(strings.TrimSpace(height), 64)
	if err != nil || h <= 0 {
		return 0, false
	}
	area := w * h
	if math.IsNaN(area) || math.IsInf(area, 0) {
		return 0, false
	}
	return area, true
}

// aipddImageClassArea returns the square area a resolution class such as "4K"
// stands for.
func aipddImageClassArea(token string) (float64, bool) {
	if !strings.HasSuffix(token, "k") {
		return 0, false
	}
	multiplier, err := strconv.ParseFloat(strings.TrimSuffix(token, "k"), 64)
	if err != nil || multiplier <= 0 {
		return 0, false
	}
	side := multiplier * 1024
	area := side * side
	if math.IsNaN(area) || math.IsInf(area, 0) {
		return 0, false
	}
	return area, true
}

// aipddImageQualityBucket returns the smallest class whose square still fits the
// pixel area, so an in-between size is never billed at the cheaper class.
func aipddImageQualityBucket(area float64) string {
	for _, bucket := range aipddImageQualityBuckets {
		if area <= bucket.area {
			return bucket.name
		}
	}
	return aipddImageQualityBuckets[len(aipddImageQualityBuckets)-1].name
}
