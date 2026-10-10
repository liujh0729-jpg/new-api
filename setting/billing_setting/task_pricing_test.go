package billing_setting

import (
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"

	settingconfig "github.com/QuantumNous/new-api/setting/config"
)

func validTaskPricing(policy string) TaskPricingConfig {
	cfg := TaskPricingConfig{
		Unit:                      TaskPricingUnitSecond,
		NoReferenceVideoUnitPrice: 0.12,
		ReferenceVideoPolicy:      policy,
	}
	if policy == ReferenceVideoPolicyCustom {
		cfg.ReferenceVideoUnitPrice = 0.18
	}
	return cfg
}

func validMatrixTaskPricing() TaskPricingConfig {
	return TaskPricingConfig{
		Unit: TaskPricingUnitSecond,
		ByResolution: map[string]TaskPricingTier{
			"480p": {
				NoReferenceVideoUnitPrice: 0.04,
				ReferenceVideoPolicy:      ReferenceVideoPolicyCustom,
				ReferenceVideoUnitPrice:   0.06,
			},
			"720p": {
				NoReferenceVideoUnitPrice: 0.08,
				ReferenceVideoPolicy:      ReferenceVideoPolicySame,
			},
			"4k": {
				NoReferenceVideoUnitPrice: 0.5,
				ReferenceVideoPolicy:      ReferenceVideoPolicyDisabled,
			},
		},
	}
}

func installTaskPricingForTest(t *testing.T, configs map[string]TaskPricingConfig) {
	t.Helper()
	original := GetTaskPricingCopy()
	billingSetting.TaskPricing.replace(configs)
	t.Cleanup(func() {
		billingSetting.TaskPricing.replace(original)
	})
}

func TestValidateTaskPricingConfig(t *testing.T) {
	for _, policy := range []string{
		ReferenceVideoPolicySame,
		ReferenceVideoPolicyCustom,
		ReferenceVideoPolicyDisabled,
	} {
		t.Run("valid_"+policy, func(t *testing.T) {
			if err := ValidateTaskPricingConfig(validTaskPricing(policy)); err != nil {
				t.Fatalf("ValidateTaskPricingConfig() error = %v", err)
			}
		})
	}

	tests := []struct {
		name string
		cfg  TaskPricingConfig
	}{
		{name: "missing unit", cfg: TaskPricingConfig{NoReferenceVideoUnitPrice: 0.12, ReferenceVideoPolicy: ReferenceVideoPolicySame}},
		{name: "unsupported unit", cfg: TaskPricingConfig{Unit: "minute", NoReferenceVideoUnitPrice: 0.12, ReferenceVideoPolicy: ReferenceVideoPolicySame}},
		{name: "missing base price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, ReferenceVideoPolicy: ReferenceVideoPolicySame}},
		{name: "zero base price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: 0, ReferenceVideoPolicy: ReferenceVideoPolicySame}},
		{name: "negative base price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: -0.1, ReferenceVideoPolicy: ReferenceVideoPolicySame}},
		{name: "nan base price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: math.NaN(), ReferenceVideoPolicy: ReferenceVideoPolicySame}},
		{name: "infinite base price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: math.Inf(1), ReferenceVideoPolicy: ReferenceVideoPolicySame}},
		{name: "missing policy", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: 0.12}},
		{name: "unsupported policy", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: 0.12, ReferenceVideoPolicy: "upstream"}},
		{name: "custom missing reference price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: 0.12, ReferenceVideoPolicy: ReferenceVideoPolicyCustom}},
		{name: "custom zero reference price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: 0.12, ReferenceVideoPolicy: ReferenceVideoPolicyCustom, ReferenceVideoUnitPrice: 0}},
		{name: "custom negative reference price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: 0.12, ReferenceVideoPolicy: ReferenceVideoPolicyCustom, ReferenceVideoUnitPrice: -0.18}},
		{name: "same negative dormant reference price", cfg: TaskPricingConfig{Unit: TaskPricingUnitSecond, NoReferenceVideoUnitPrice: 0.12, ReferenceVideoPolicy: ReferenceVideoPolicySame, ReferenceVideoUnitPrice: -0.18}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTaskPricingConfig(tt.cfg); !errors.Is(err, ErrInvalidTaskPricing) {
				t.Fatalf("ValidateTaskPricingConfig() error = %v, want ErrInvalidTaskPricing", err)
			}
		})
	}
}

func TestValidateAndParseTaskPricingMap(t *testing.T) {
	if err := ValidateTaskPricingMap(map[string]TaskPricingConfig{
		"":      validTaskPricing(ReferenceVideoPolicySame),
		"model": validTaskPricing(ReferenceVideoPolicyCustom),
	}); !errors.Is(err, ErrInvalidTaskPricing) {
		t.Fatalf("empty model name error = %v, want ErrInvalidTaskPricing", err)
	}
	if err := ValidateTaskPricingMap(map[string]TaskPricingConfig{
		" model ": validTaskPricing(ReferenceVideoPolicySame),
	}); !errors.Is(err, ErrInvalidTaskPricing) {
		t.Fatalf("padded model name error = %v, want ErrInvalidTaskPricing", err)
	}

	parsed, err := ParseTaskPricingMapJSON(`{
		"AP Seedance": {
			"unit": "second",
			"no_reference_video_unit_price": 0.12,
			"reference_video_policy": "custom",
			"reference_video_unit_price": 0.18
		}
	}`)
	if err != nil {
		t.Fatalf("ParseTaskPricingMapJSON() error = %v", err)
	}
	if got := parsed["AP Seedance"]; !reflect.DeepEqual(got, validTaskPricing(ReferenceVideoPolicyCustom)) {
		t.Fatalf("parsed config = %#v", got)
	}

	invalidJSON := []string{
		`null`,
		`[]`,
		`{"AP Seedance":{"unit":"second","no_reference_video_unit_price":"0.12","reference_video_policy":"same"}}`,
		`{"AP Seedance":{"unit":"second","no_reference_video_unit_price":0,"reference_video_policy":"same"}}`,
	}
	for _, raw := range invalidJSON {
		if _, err := ParseTaskPricingMapJSON(raw); !errors.Is(err, ErrInvalidTaskPricing) {
			t.Errorf("ParseTaskPricingMapJSON(%s) error = %v, want ErrInvalidTaskPricing", raw, err)
		}
	}
}

func TestTaskPricingMatrixValidationAndNormalization(t *testing.T) {
	parsed, err := ParseTaskPricingMapJSON(`{
		"matrix": {
			"unit": "second",
			"by_resolution": {
				" 720P ": {"no_reference_video_unit_price": 0.08, "reference_video_policy": "same"},
				"4K": {"no_reference_video_unit_price": 0.5, "reference_video_policy": "disabled"}
			}
		}
	}`)
	if err != nil {
		t.Fatalf("ParseTaskPricingMapJSON(matrix) error = %v", err)
	}
	if got := TaskPricingResolutionKeys(parsed["matrix"]); !reflect.DeepEqual(got, []string{"720p", "4k"}) {
		t.Fatalf("normalized resolution keys = %v", got)
	}
	parsedWithGroupPolicy, err := ParseTaskPricingMapJSON(`{"matrix":{"unit":"second","by_resolution":{"480p":{"no_reference_video_unit_price":0.08,"reference_video_policy":"same","group_ratio_policy":"none"}}}}`)
	if err != nil {
		t.Fatalf("ParseTaskPricingMapJSON(group policy) error = %v", err)
	}
	if got := parsedWithGroupPolicy["matrix"].ByResolution["480p"].GroupRatioPolicy; got != TaskPricingGroupRatioNone {
		t.Fatalf("group ratio policy = %q", got)
	}

	invalid := []TaskPricingConfig{
		{Unit: TaskPricingUnitSecond, ByResolution: map[string]TaskPricingTier{}},
		{
			Unit:                      TaskPricingUnitSecond,
			NoReferenceVideoUnitPrice: 0.12,
			ReferenceVideoPolicy:      ReferenceVideoPolicySame,
			ByResolution:              validMatrixTaskPricing().ByResolution,
		},
	}
	for index, cfg := range invalid {
		if err := ValidateTaskPricingConfig(cfg); !errors.Is(err, ErrInvalidTaskPricing) {
			t.Errorf("invalid matrix %d error = %v, want ErrInvalidTaskPricing", index, err)
		}
	}

	if _, err := ParseTaskPricingMapJSON(`{"matrix":{"unit":"second","by_resolution":{"720P":{"no_reference_video_unit_price":0.08,"reference_video_policy":"same"},"720p":{"no_reference_video_unit_price":0.09,"reference_video_policy":"same"}}}}`); !errors.Is(err, ErrInvalidTaskPricing) {
		t.Fatalf("normalized duplicate error = %v, want ErrInvalidTaskPricing", err)
	}
	if _, err := ParseTaskPricingMapJSON(`{"matrix":{"unit":"second","by_resolution":{"480p":{"no_reference_video_unit_price":0.08,"reference_video_policy":"same","group_ratio_policy":"unexpected"}}}}`); !errors.Is(err, ErrInvalidTaskPricing) {
		t.Fatalf("invalid group ratio policy error = %v, want ErrInvalidTaskPricing", err)
	}
}

func TestTaskPricingMatrixStoreDeepCopiesNestedTiers(t *testing.T) {
	store := NewTaskPricingStore()
	original := validMatrixTaskPricing()
	store.replace(map[string]TaskPricingConfig{"matrix": original})

	original.ByResolution["480p"] = TaskPricingTier{NoReferenceVideoUnitPrice: 99, ReferenceVideoPolicy: ReferenceVideoPolicySame}
	got, ok := store.get("matrix")
	if !ok || got.ByResolution["480p"].NoReferenceVideoUnitPrice != 0.04 {
		t.Fatalf("store retained caller nested mutation: %#v", got)
	}

	got.ByResolution["480p"] = TaskPricingTier{NoReferenceVideoUnitPrice: 88, ReferenceVideoPolicy: ReferenceVideoPolicySame}
	again, _ := store.get("matrix")
	if again.ByResolution["480p"].NoReferenceVideoUnitPrice != 0.04 {
		t.Fatalf("get returned shared nested map: %#v", again)
	}
}

func TestTaskPricingStoreCopiesAndPreservesValidState(t *testing.T) {
	store := NewTaskPricingStore()
	original := map[string]TaskPricingConfig{
		"model-a": validTaskPricing(ReferenceVideoPolicySame),
	}
	store.replace(original)

	original["model-a"] = validTaskPricing(ReferenceVideoPolicyCustom)
	got, ok := store.get("model-a")
	if !ok || got.ReferenceVideoPolicy != ReferenceVideoPolicySame {
		t.Fatalf("store retained caller mutation: %#v, %v", got, ok)
	}

	copyOfStore := store.copy()
	delete(copyOfStore, "model-a")
	if _, ok := store.get("model-a"); !ok {
		t.Fatal("mutating copy removed entry from store")
	}

	if err := store.UnmarshalJSON([]byte(`{"model-b":{"unit":"second","no_reference_video_unit_price":0,"reference_video_policy":"same"}}`)); !errors.Is(err, ErrInvalidTaskPricing) {
		t.Fatalf("UnmarshalJSON() error = %v, want ErrInvalidTaskPricing", err)
	}
	if _, ok := store.get("model-a"); !ok {
		t.Fatal("invalid replacement changed the last valid store state")
	}

	data, err := store.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	parsed, err := parseTaskPricingMap(data)
	if err != nil {
		t.Fatalf("parseTaskPricingMap(MarshalJSON()) error = %v", err)
	}
	if !reflect.DeepEqual(parsed, store.copy()) {
		t.Fatalf("round trip = %#v, want %#v", parsed, store.copy())
	}
}

func TestTaskPricingStoreWorksWithConfigManagerSerialization(t *testing.T) {
	settings := BillingSetting{
		BillingMode: make(map[string]string),
		BillingExpr: make(map[string]string),
		TaskPricing: NewTaskPricingStore(),
	}
	raw := `{"model":{"unit":"second","no_reference_video_unit_price":0.12,"reference_video_policy":"same"}}`
	if err := settingconfig.UpdateConfigFromMap(&settings, map[string]string{TaskPricingField: raw}); err != nil {
		t.Fatalf("UpdateConfigFromMap() error = %v", err)
	}
	if got, ok := settings.TaskPricing.get("model"); !ok || !reflect.DeepEqual(got, validTaskPricing(ReferenceVideoPolicySame)) {
		t.Fatalf("loaded config = %#v, %v", got, ok)
	}

	serialized, err := settingconfig.ConfigToMap(&settings)
	if err != nil {
		t.Fatalf("ConfigToMap() error = %v", err)
	}
	parsed, err := ParseTaskPricingMapJSON(serialized[TaskPricingField])
	if err != nil {
		t.Fatalf("serialized task pricing error = %v", err)
	}
	if got := parsed["model"]; !reflect.DeepEqual(got, validTaskPricing(ReferenceVideoPolicySame)) {
		t.Fatalf("serialized config = %#v", got)
	}
}

func TestTaskPricingStoreConcurrentReadAndReplace(t *testing.T) {
	store := NewTaskPricingStore()
	store.replace(map[string]TaskPricingConfig{"model": validTaskPricing(ReferenceVideoPolicySame)})

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				_, _ = store.get("model")
				_ = store.copy()
			}
		}()
	}
	for i := 0; i < 250; i++ {
		policy := ReferenceVideoPolicySame
		if i%2 == 1 {
			policy = ReferenceVideoPolicyCustom
		}
		store.replace(map[string]TaskPricingConfig{"model": validTaskPricing(policy)})
	}
	wg.Wait()
}

func TestQuoteTaskPricingSelectsVariants(t *testing.T) {
	installTaskPricingForTest(t, map[string]TaskPricingConfig{
		"same":     validTaskPricing(ReferenceVideoPolicySame),
		"custom":   validTaskPricing(ReferenceVideoPolicyCustom),
		"disabled": validTaskPricing(ReferenceVideoPolicyDisabled),
	})

	tests := []struct {
		name              string
		model             string
		hasReferenceVideo bool
		wantVariant       string
		wantUnitPrice     float64
	}{
		{name: "no reference video", model: "custom", wantVariant: TaskPricingVariantNoReferenceVideo, wantUnitPrice: 0.12},
		{name: "same price with reference video", model: "same", hasReferenceVideo: true, wantVariant: TaskPricingVariantReferenceVideo, wantUnitPrice: 0.12},
		{name: "custom price with reference video", model: "custom", hasReferenceVideo: true, wantVariant: TaskPricingVariantReferenceVideo, wantUnitPrice: 0.18},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			quote, err := QuoteTaskPricing(tt.model, 5, "", 1.5, 500_000, tt.hasReferenceVideo)
			if err != nil {
				t.Fatalf("QuoteTaskPricing() error = %v", err)
			}
			if quote.Unit != TaskPricingUnitSecond || quote.Variant != tt.wantVariant || quote.UnitPriceUSD != tt.wantUnitPrice {
				t.Fatalf("quote selection = %#v", quote)
			}
			if quote.Quantity != 5 || quote.GroupRatio != 1.5 || quote.BaseUSD != tt.wantUnitPrice*5 || quote.SaleUSD != tt.wantUnitPrice*5*1.5 {
				t.Fatalf("quote amounts = %#v", quote)
			}
			if quote.Quota != int(math.Round(tt.wantUnitPrice*5*1.5*500_000)) {
				t.Fatalf("quote quota = %d", quote.Quota)
			}
			if quote.HasReferenceVideo != tt.hasReferenceVideo {
				t.Fatalf("HasReferenceVideo = %v", quote.HasReferenceVideo)
			}
		})
	}

	if _, err := QuoteTaskPricing("disabled", 5, "", 1, 500_000, true); !errors.Is(err, ErrReferenceVideoDisabled) {
		t.Fatalf("disabled policy error = %v, want ErrReferenceVideoDisabled", err)
	}
	if _, err := QuoteTaskPricing("missing", 5, "", 1, 500_000, false); !errors.Is(err, ErrTaskPricingNotConfigured) {
		t.Fatalf("missing config error = %v, want ErrTaskPricingNotConfigured", err)
	}
}

func TestQuoteTaskPricingSelectsResolutionTier(t *testing.T) {
	installTaskPricingForTest(t, map[string]TaskPricingConfig{
		"matrix": validMatrixTaskPricing(),
	})

	quote, err := QuoteTaskPricing("matrix", 5, " 480P ", 1.5, 500_000, true)
	if err != nil {
		t.Fatalf("QuoteTaskPricing(matrix) error = %v", err)
	}
	if quote.Resolution != "480p" || quote.UnitPriceUSD != 0.06 || quote.GroupRatio != 1 || quote.Quota != 150_000 {
		t.Fatalf("matrix quote = %#v", quote)
	}

	if _, err := QuoteTaskPricing("matrix", 5, "", 1, 500_000, false); !errors.Is(err, ErrTaskPricingResolutionRequired) {
		t.Fatalf("missing resolution error = %v", err)
	}
	if _, err := QuoteTaskPricing("matrix", 5, "1080p", 1, 500_000, false); !errors.Is(err, ErrTaskPricingResolutionNotConfigured) {
		t.Fatalf("missing tier error = %v", err)
	}
	if _, err := QuoteTaskPricing("matrix", 5, "4k", 1, 500_000, true); !errors.Is(err, ErrReferenceVideoDisabled) {
		t.Fatalf("disabled resolution error = %v", err)
	}
}

func TestQuoteTaskPricingKeeps480pNativeByDefault(t *testing.T) {
	cfg := validMatrixTaskPricing()
	installTaskPricingForTest(t, map[string]TaskPricingConfig{"matrix": cfg})

	nativeQuote, err := QuoteTaskPricing("matrix", 5, "480p", 0.78, 500_000, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing(native resolution) error = %v", err)
	}
	if nativeQuote.GroupRatio != 1 || nativeQuote.BaseUSD != 0.2 || nativeQuote.SaleUSD != 0.2 || nativeQuote.Quota != 100_000 {
		t.Fatalf("native resolution quote = %#v", nativeQuote)
	}

	discountedQuote, err := QuoteTaskPricing("matrix", 5, "720p", 0.78, 500_000, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing(discounted resolution) error = %v", err)
	}
	if discountedQuote.GroupRatio != 0.78 || discountedQuote.SaleUSD != discountedQuote.BaseUSD*0.78 {
		t.Fatalf("discounted resolution quote = %#v", discountedQuote)
	}
}

func TestQuoteTaskPricingExplicitGlobalPolicyDiscounts480p(t *testing.T) {
	cfg := validMatrixTaskPricing()
	tier := cfg.ByResolution["480p"]
	tier.GroupRatioPolicy = TaskPricingGroupRatioGlobal
	cfg.ByResolution["480p"] = tier
	installTaskPricingForTest(t, map[string]TaskPricingConfig{"matrix": cfg})

	quote, err := QuoteTaskPricing("matrix", 5, "480p", 0.78, 500_000, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing(explicit global 480p) error = %v", err)
	}
	if quote.GroupRatio != 0.78 || quote.SaleUSD != quote.BaseUSD*0.78 {
		t.Fatalf("explicit global 480p quote = %#v", quote)
	}
}

func TestQuoteTaskPricingExplicitNoneKeepsAnyResolutionNative(t *testing.T) {
	cfg := validMatrixTaskPricing()
	tier := cfg.ByResolution["720p"]
	tier.GroupRatioPolicy = TaskPricingGroupRatioNone
	cfg.ByResolution["720p"] = tier
	installTaskPricingForTest(t, map[string]TaskPricingConfig{"matrix": cfg})

	quote, err := QuoteTaskPricing("matrix", 5, "720p", 0.78, 500_000, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing(explicit none 720p) error = %v", err)
	}
	if quote.GroupRatio != 1 || quote.SaleUSD != quote.BaseUSD {
		t.Fatalf("explicit none 720p quote = %#v", quote)
	}
}

func TestQuoteLegacyTaskPricingKeeps480pNativeByDefault(t *testing.T) {
	installTaskPricingForTest(t, map[string]TaskPricingConfig{
		"legacy": validTaskPricing(ReferenceVideoPolicySame),
	})

	quote, err := QuoteTaskPricing("legacy", 5, "480p", 0.78, 500_000, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing(legacy 480p) error = %v", err)
	}
	if quote.GroupRatio != 1 || quote.SaleUSD != quote.BaseUSD {
		t.Fatalf("legacy 480p quote = %#v", quote)
	}
}

func TestQuoteTaskPricingAllowsZeroGroupRatio(t *testing.T) {
	installTaskPricingForTest(t, map[string]TaskPricingConfig{
		"model": validTaskPricing(ReferenceVideoPolicySame),
	})

	quote, err := QuoteTaskPricing("model", 5, "", 0, 500_000, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing() error = %v", err)
	}
	if quote.GroupRatio != 0 || quote.BaseUSD != 0.6 || quote.SaleUSD != 0 || quote.Quota != 0 {
		t.Fatalf("zero-ratio quote = %#v", quote)
	}
}

func TestQuoteTaskPricingRoundsOnlyFinalQuota(t *testing.T) {
	installTaskPricingForTest(t, map[string]TaskPricingConfig{
		"model": {
			Unit:                      TaskPricingUnitSecond,
			NoReferenceVideoUnitPrice: 0.49,
			ReferenceVideoPolicy:      ReferenceVideoPolicySame,
		},
	})

	quote, err := QuoteTaskPricing("model", 3, "", 1, 1, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing() error = %v", err)
	}
	if quote.Quota != 1 {
		t.Fatalf("Quota = %d, want one-shot round(0.49*3) = 1", quote.Quota)
	}

	halfQuote, err := QuoteTaskPricing("model", 1, "", 1, 0.5/0.49, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing(half) error = %v", err)
	}
	if halfQuote.Quota != 1 {
		t.Fatalf("Quota = %d, want half-away-from-zero result 1", halfQuote.Quota)
	}
}

func TestQuoteTaskPricingRejectsInvalidInputs(t *testing.T) {
	installTaskPricingForTest(t, map[string]TaskPricingConfig{
		"model": validTaskPricing(ReferenceVideoPolicySame),
	})

	tests := []struct {
		name         string
		quantity     float64
		groupRatio   float64
		quotaPerUnit float64
	}{
		{name: "zero quantity", quantity: 0, groupRatio: 1, quotaPerUnit: 1},
		{name: "negative quantity", quantity: -1, groupRatio: 1, quotaPerUnit: 1},
		{name: "nan quantity", quantity: math.NaN(), groupRatio: 1, quotaPerUnit: 1},
		{name: "negative group ratio", quantity: 1, groupRatio: -1, quotaPerUnit: 1},
		{name: "infinite group ratio", quantity: 1, groupRatio: math.Inf(1), quotaPerUnit: 1},
		{name: "zero quota per unit", quantity: 1, groupRatio: 1, quotaPerUnit: 0},
		{name: "negative quota per unit", quantity: 1, groupRatio: 1, quotaPerUnit: -1},
		{name: "quota integer overflow", quantity: math.MaxFloat64, groupRatio: 1, quotaPerUnit: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := QuoteTaskPricing("model", tt.quantity, "", tt.groupRatio, tt.quotaPerUnit, false); !errors.Is(err, ErrInvalidTaskPricing) {
				t.Fatalf("QuoteTaskPricing() error = %v, want ErrInvalidTaskPricing", err)
			}
		})
	}
}
