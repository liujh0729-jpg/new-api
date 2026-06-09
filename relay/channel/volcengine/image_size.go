package volcengine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	seedream50LiteMinPixels = 2560 * 1440
	seedream50LiteMaxPixels = 4096 * 4096
)

var seedream50LitePixelSizePattern = regexp.MustCompile(`^(\d+)x(\d+)$`)

var seedream50LiteSupportedSizes = map[string]bool{
	"2560x1440": true,
	"2496x1664": true,
	"3072x1536": true,
	"3248x1824": true,
	"3456x1944": true,
	"3648x1536": true,
	"3840x2160": true,
	"4032x2688": true,
	"4160x2340": true,
	"4096x1728": true,
	"4096x2304": true,
	"4096x3072": true,
	"2048x2048": true,
	"3072x3072": true,
	"4096x4096": true,
	"1920x1920": true,
	"1440x2560": true,
}

func isSeedream50LiteModel(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	normalized = strings.ReplaceAll(normalized, ".", "-")

	return strings.Contains(normalized, "seedream-5-0-lite") ||
		strings.Contains(normalized, "seedream-5-0-260128")
}

func matchesSeedream50LiteModel(models ...string) bool {
	for _, model := range models {
		if isSeedream50LiteModel(model) {
			return true
		}
	}
	return false
}

func validateSeedream50LiteImageSize(models []string, size string) error {
	if !matchesSeedream50LiteModel(models...) {
		return nil
	}

	normalizedSize := strings.TrimSpace(size)
	if normalizedSize == "" {
		return nil
	}

	switch strings.ToUpper(normalizedSize) {
	case "2K", "3K", "4K":
		return nil
	}

	width, height, ok := parseImagePixelSize(normalizedSize)
	if !ok {
		return seedream50LiteSizeError(normalizedSize)
	}

	if seedream50LiteSupportedSizes[normalizedSize] {
		return nil
	}

	pixels := width * height
	if pixels < seedream50LiteMinPixels || pixels > seedream50LiteMaxPixels {
		return seedream50LiteSizeError(normalizedSize)
	}

	ratio := float64(width) / float64(height)
	if ratio < 1.0/16.0 || ratio > 16.0 {
		return seedream50LiteSizeError(normalizedSize)
	}

	return nil
}

func parseImagePixelSize(size string) (int, int, bool) {
	match := seedream50LitePixelSizePattern.FindStringSubmatch(strings.ToLower(size))
	if match == nil {
		return 0, 0, false
	}

	width, err := strconv.Atoi(match[1])
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err := strconv.Atoi(match[2])
	if err != nil || height <= 0 {
		return 0, 0, false
	}

	return width, height, true
}

func seedream50LiteSizeError(size string) error {
	return fmt.Errorf("Doubao-Seedream-5.0-lite does not support size %q. Please use 2K, 3K, 4K, or a WIDTHxHEIGHT dimension within total pixels [%d, %d] and aspect ratio [1/16, 16]. Recommended sizes: 2K (2560x1440), 3K (3456x1944), 4K (3840x2160)",
		size,
		seedream50LiteMinPixels,
		seedream50LiteMaxPixels,
	)
}
