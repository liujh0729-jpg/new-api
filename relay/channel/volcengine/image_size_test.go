package volcengine

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
)

func TestValidateSeedream50LiteImageSizeAllowsSupportedSizes(t *testing.T) {
	models := []string{"Doubao-Seedream-5.0-lite"}
	validSizes := []string{
		"",
		"2K",
		"3k",
		"4K",
		"1920x1920",
		"2048x2048",
		"2560x1440",
		"1440x2560",
		"3750x1250",
		"4096x4096",
	}

	for _, size := range validSizes {
		t.Run(size, func(t *testing.T) {
			if err := validateSeedream50LiteImageSize(models, size); err != nil {
				t.Fatalf("validateSeedream50LiteImageSize() error = %v", err)
			}
		})
	}
}

func TestValidateSeedream50LiteImageSizeRejectsUnsupportedSizes(t *testing.T) {
	models := []string{"doubao-seedream-5-0-260128"}
	invalidSizes := []string{
		"1024x1024",
		"1024x1536",
		"512x512",
		"16:9",
		"5000x5000",
		"5000x100",
		"abc",
	}

	for _, size := range invalidSizes {
		t.Run(size, func(t *testing.T) {
			if err := validateSeedream50LiteImageSize(models, size); err == nil {
				t.Fatal("validateSeedream50LiteImageSize() error = nil, want error")
			}
		})
	}
}

func TestValidateSeedream50LiteImageSizeIgnoresOtherModels(t *testing.T) {
	if err := validateSeedream50LiteImageSize([]string{"doubao-seedream-4-5"}, "1024x1024"); err != nil {
		t.Fatalf("validateSeedream50LiteImageSize() error = %v", err)
	}
}

func TestConvertImageRequestDefaultsWatermarkFalse(t *testing.T) {
	adaptor := &Adaptor{}
	cases := []struct {
		name string
		mode int
	}{
		{name: "generations", mode: relayconstant.RelayModeImagesGenerations},
		{name: "edits", mode: relayconstant.RelayModeImagesEdits},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := adaptor.ConvertImageRequest(nil, newImageRelayInfo(tc.mode), dto.ImageRequest{
				Model:  "doubao-seedream-4-0",
				Prompt: "cat",
			})
			if err != nil {
				t.Fatalf("ConvertImageRequest() error = %v", err)
			}

			request, ok := got.(dto.ImageRequest)
			if !ok {
				t.Fatalf("ConvertImageRequest() returned %T, want dto.ImageRequest", got)
			}
			if request.Watermark == nil {
				t.Fatal("Watermark = nil, want false pointer")
			}
			if *request.Watermark {
				t.Fatal("Watermark = true, want false")
			}
		})
	}
}

func TestConvertImageRequestPreservesExplicitWatermark(t *testing.T) {
	adaptor := &Adaptor{}
	watermark := true

	got, err := adaptor.ConvertImageRequest(nil, newImageRelayInfo(relayconstant.RelayModeImagesGenerations), dto.ImageRequest{
		Model:     "doubao-seedream-4-0",
		Prompt:    "cat",
		Watermark: &watermark,
	})
	if err != nil {
		t.Fatalf("ConvertImageRequest() error = %v", err)
	}

	request, ok := got.(dto.ImageRequest)
	if !ok {
		t.Fatalf("ConvertImageRequest() returned %T, want dto.ImageRequest", got)
	}
	if request.Watermark == nil {
		t.Fatal("Watermark = nil, want true pointer")
	}
	if !*request.Watermark {
		t.Fatal("Watermark = false, want true")
	}
}

func newImageRelayInfo(mode int) *relaycommon.RelayInfo {
	const model = "doubao-seedream-4-0"
	return &relaycommon.RelayInfo{
		RelayMode:       mode,
		OriginModelName: model,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model,
		},
	}
}
