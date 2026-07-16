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
	if got := parsed["AP Seedance"]; got != validTaskPricing(ReferenceVideoPolicyCustom) {
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
	if got, ok := settings.TaskPricing.get("model"); !ok || got != validTaskPricing(ReferenceVideoPolicySame) {
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
	if got := parsed["model"]; got != validTaskPricing(ReferenceVideoPolicySame) {
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
			quote, err := QuoteTaskPricing(tt.model, 5, 1.5, 500_000, tt.hasReferenceVideo)
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

	if _, err := QuoteTaskPricing("disabled", 5, 1, 500_000, true); !errors.Is(err, ErrReferenceVideoDisabled) {
		t.Fatalf("disabled policy error = %v, want ErrReferenceVideoDisabled", err)
	}
	if _, err := QuoteTaskPricing("missing", 5, 1, 500_000, false); !errors.Is(err, ErrTaskPricingNotConfigured) {
		t.Fatalf("missing config error = %v, want ErrTaskPricingNotConfigured", err)
	}
}

func TestQuoteTaskPricingAllowsZeroGroupRatio(t *testing.T) {
	installTaskPricingForTest(t, map[string]TaskPricingConfig{
		"model": validTaskPricing(ReferenceVideoPolicySame),
	})

	quote, err := QuoteTaskPricing("model", 5, 0, 500_000, false)
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

	quote, err := QuoteTaskPricing("model", 3, 1, 1, false)
	if err != nil {
		t.Fatalf("QuoteTaskPricing() error = %v", err)
	}
	if quote.Quota != 1 {
		t.Fatalf("Quota = %d, want one-shot round(0.49*3) = 1", quote.Quota)
	}

	halfQuote, err := QuoteTaskPricing("model", 1, 1, 0.5/0.49, false)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := QuoteTaskPricing("model", tt.quantity, tt.groupRatio, tt.quotaPerUnit, false); !errors.Is(err, ErrInvalidTaskPricing) {
				t.Fatalf("QuoteTaskPricing() error = %v, want ErrInvalidTaskPricing", err)
			}
		})
	}
}
