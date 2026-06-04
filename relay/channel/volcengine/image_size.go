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
	return fmt.Errorf("Doubao-Seedream-5.0-lite 不支持当前 size 参数 %q。请使用 2K、3K、4K，或满足总像素 [%d, %d] 且宽高比 [1/16, 16] 的 WIDTHxHEIGHT 尺寸",
		size,
		seedream50LiteMinPixels,
		seedream50LiteMaxPixels,
	)
}
