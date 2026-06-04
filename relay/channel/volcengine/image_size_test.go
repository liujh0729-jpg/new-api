package volcengine

import "testing"

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
