package billing_setting

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

const (
	TaskPricingUnitSecond = "second"

	ReferenceVideoPolicySame     = "same"
	ReferenceVideoPolicyCustom   = "custom"
	ReferenceVideoPolicyDisabled = "disabled"

	TaskPricingVariantNoReferenceVideo = "no_reference_video"
	TaskPricingVariantReferenceVideo   = "reference_video"
	maxTaskPricingResolutionLength     = 128
)

var (
	ErrInvalidTaskPricing                 = errors.New("invalid task pricing")
	ErrTaskPricingNotConfigured           = errors.New("task pricing is not configured")
	ErrReferenceVideoDisabled             = errors.New("reference video input is disabled")
	ErrTaskPricingResolutionRequired      = errors.New("task pricing resolution is required")
	ErrTaskPricingResolutionNotConfigured = errors.New("task pricing resolution is not configured")
)

// TaskPricingTier defines the local retail price for one output resolution.
// It intentionally mirrors the legacy model-level variant fields so each
// resolution can independently allow, disable, or customize video input.
type TaskPricingTier struct {
	NoReferenceVideoUnitPrice float64 `json:"no_reference_video_unit_price"`
	ReferenceVideoPolicy      string  `json:"reference_video_policy"`
	ReferenceVideoUnitPrice   float64 `json:"reference_video_unit_price,omitempty"`
}

// TaskPricingConfig defines the local retail price for a duration-based task.
// Prices are stored in New API's USD base unit and are never sourced from the
// upstream provider at quote time.
type TaskPricingConfig struct {
	Unit                      string                     `json:"unit"`
	NoReferenceVideoUnitPrice float64                    `json:"no_reference_video_unit_price,omitempty"`
	ReferenceVideoPolicy      string                     `json:"reference_video_policy,omitempty"`
	ReferenceVideoUnitPrice   float64                    `json:"reference_video_unit_price,omitempty"`
	ByResolution              map[string]TaskPricingTier `json:"by_resolution,omitempty"`
}

// TaskPricingQuote is an immutable result of selecting a local task-pricing
// variant. BaseUSD is the price before applying the group ratio; SaleUSD is
// the final local sale price after the group ratio. Quota applies quotaPerUnit
// to SaleUSD and is rounded exactly once.
type TaskPricingQuote struct {
	Unit              string  `json:"unit"`
	Variant           string  `json:"variant"`
	UnitPriceUSD      float64 `json:"unit_price_usd"`
	Quantity          float64 `json:"quantity"`
	GroupRatio        float64 `json:"group_ratio"`
	BaseUSD           float64 `json:"base_usd"`
	SaleUSD           float64 `json:"sale_usd"`
	Quota             int     `json:"quota"`
	HasReferenceVideo bool    `json:"has_reference_video"`
	Resolution        string  `json:"resolution,omitempty"`
}

type taskPricingState struct {
	mu     sync.RWMutex
	values map[string]TaskPricingConfig
}

// TaskPricingStore serializes as a regular model-to-config JSON map while
// keeping runtime reads and config replacement safe. It is intentionally a
// value field in BillingSetting so the existing config manager can persist it.
type TaskPricingStore struct {
	state *taskPricingState
}

func NewTaskPricingStore() TaskPricingStore {
	return TaskPricingStore{
		state: &taskPricingState{values: make(map[string]TaskPricingConfig)},
	}
}

func (s TaskPricingStore) MarshalJSON() ([]byte, error) {
	return common.Marshal(s.copy())
}

func (s *TaskPricingStore) UnmarshalJSON(data []byte) error {
	configs, err := parseTaskPricingMap(data)
	if err != nil {
		return err
	}
	s.replace(configs)
	return nil
}

func (s TaskPricingStore) get(model string) (TaskPricingConfig, bool) {
	if s.state == nil {
		return TaskPricingConfig{}, false
	}
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	cfg, ok := s.state.values[model]
	return cloneTaskPricingConfig(cfg), ok
}

func (s TaskPricingStore) copy() map[string]TaskPricingConfig {
	if s.state == nil {
		return make(map[string]TaskPricingConfig)
	}
	s.state.mu.RLock()
	defer s.state.mu.RUnlock()
	return cloneTaskPricingMap(s.state.values)
}

func (s *TaskPricingStore) replace(configs map[string]TaskPricingConfig) {
	if s.state == nil {
		s.state = &taskPricingState{}
	}
	s.state.mu.Lock()
	s.state.values = cloneTaskPricingMap(configs)
	s.state.mu.Unlock()
}

func cloneTaskPricingMap(configs map[string]TaskPricingConfig) map[string]TaskPricingConfig {
	cloned := make(map[string]TaskPricingConfig, len(configs))
	for model, cfg := range configs {
		cloned[model] = cloneTaskPricingConfig(cfg)
	}
	return cloned
}

func cloneTaskPricingConfig(cfg TaskPricingConfig) TaskPricingConfig {
	if cfg.ByResolution == nil {
		return cfg
	}
	cloned := cfg
	cloned.ByResolution = make(map[string]TaskPricingTier, len(cfg.ByResolution))
	for resolution, tier := range cfg.ByResolution {
		cloned.ByResolution[resolution] = tier
	}
	return cloned
}

func parseTaskPricingMap(data []byte) (map[string]TaskPricingConfig, error) {
	var configs map[string]TaskPricingConfig
	if err := common.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("%w: decode task pricing: %v", ErrInvalidTaskPricing, err)
	}
	if configs == nil {
		return nil, fmt.Errorf("%w: task pricing must be a JSON object", ErrInvalidTaskPricing)
	}
	normalized, err := normalizeTaskPricingMap(configs)
	if err != nil {
		return nil, err
	}
	if err := ValidateTaskPricingMap(normalized); err != nil {
		return nil, err
	}
	return cloneTaskPricingMap(normalized), nil
}

func normalizeTaskPricingMap(configs map[string]TaskPricingConfig) (map[string]TaskPricingConfig, error) {
	normalized := make(map[string]TaskPricingConfig, len(configs))
	for model, cfg := range configs {
		cfg = cloneTaskPricingConfig(cfg)
		if cfg.ByResolution != nil {
			byResolution := make(map[string]TaskPricingTier, len(cfg.ByResolution))
			for rawResolution, tier := range cfg.ByResolution {
				resolution, err := NormalizeTaskPricingResolution(rawResolution)
				if err != nil {
					return nil, fmt.Errorf("model %q: %w", model, err)
				}
				if _, exists := byResolution[resolution]; exists {
					return nil, fmt.Errorf("model %q: %w: duplicate resolution %q after normalization", model, ErrInvalidTaskPricing, resolution)
				}
				byResolution[resolution] = tier
			}
			cfg.ByResolution = byResolution
		}
		normalized[model] = cfg
	}
	return normalized, nil
}

// NormalizeTaskPricingResolution returns the provider-facing canonical
// resolution identifier used for local price lookup. It deliberately does not
// invent semantic aliases such as 2k=1440p or 4k=2160p.
func NormalizeTaskPricingResolution(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", fmt.Errorf("%w: resolution must be non-empty", ErrInvalidTaskPricing)
	}
	if len(normalized) > maxTaskPricingResolutionLength {
		return "", fmt.Errorf("%w: resolution %q exceeds %d characters", ErrInvalidTaskPricing, normalized, maxTaskPricingResolutionLength)
	}
	return normalized, nil
}

// TaskPricingResolutionLess orders common p/k resolution identifiers by
// effective scale without treating distinct provider identifiers as aliases.
func TaskPricingResolutionLess(left, right string) bool {
	leftValue, leftOK := taskPricingResolutionSortValue(left)
	rightValue, rightOK := taskPricingResolutionSortValue(right)
	if leftOK && rightOK && leftValue != rightValue {
		return leftValue < rightValue
	}
	if leftOK != rightOK {
		return leftOK
	}
	return strings.ToLower(left) < strings.ToLower(right)
}

func taskPricingResolutionSortValue(value string) (float64, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(normalized) < 2 {
		return 0, false
	}
	factor := 1.0
	switch normalized[len(normalized)-1] {
	case 'p':
	case 'k':
		factor = 1000
	default:
		return 0, false
	}
	number, err := strconv.ParseFloat(normalized[:len(normalized)-1], 64)
	if err != nil || number <= 0 || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, false
	}
	return number * factor, true
}

// ParseTaskPricingMapJSON decodes and validates the complete task-pricing
// option before it is persisted.
func ParseTaskPricingMapJSON(jsonStr string) (map[string]TaskPricingConfig, error) {
	return parseTaskPricingMap(common.StringToByteSlice(jsonStr))
}

// GetTaskPricing returns a value copy of a model's local task-pricing config.
func GetTaskPricing(model string) (TaskPricingConfig, bool) {
	return billingSetting.TaskPricing.get(model)
}

// GetTaskPricingCopy returns an isolated copy safe for callers to mutate.
func GetTaskPricingCopy() map[string]TaskPricingConfig {
	return billingSetting.TaskPricing.copy()
}

// ValidateTaskPricingConfig enforces the persisted contract for one model.
func ValidateTaskPricingConfig(cfg TaskPricingConfig) error {
	if cfg.Unit != TaskPricingUnitSecond {
		return fmt.Errorf("%w: unit must be %q", ErrInvalidTaskPricing, TaskPricingUnitSecond)
	}
	if cfg.ByResolution != nil {
		if len(cfg.ByResolution) == 0 {
			return fmt.Errorf("%w: by_resolution must contain at least one tier", ErrInvalidTaskPricing)
		}
		if cfg.NoReferenceVideoUnitPrice != 0 || cfg.ReferenceVideoPolicy != "" || cfg.ReferenceVideoUnitPrice != 0 {
			return fmt.Errorf("%w: legacy price fields and by_resolution are mutually exclusive", ErrInvalidTaskPricing)
		}
		seen := make(map[string]struct{}, len(cfg.ByResolution))
		for rawResolution, tier := range cfg.ByResolution {
			resolution, err := NormalizeTaskPricingResolution(rawResolution)
			if err != nil {
				return err
			}
			if _, exists := seen[resolution]; exists {
				return fmt.Errorf("%w: duplicate resolution %q after normalization", ErrInvalidTaskPricing, resolution)
			}
			seen[resolution] = struct{}{}
			if err := validateTaskPricingTier(tier); err != nil {
				return fmt.Errorf("resolution %q: %w", resolution, err)
			}
		}
		return nil
	}

	return validateTaskPricingTier(TaskPricingTier{
		NoReferenceVideoUnitPrice: cfg.NoReferenceVideoUnitPrice,
		ReferenceVideoPolicy:      cfg.ReferenceVideoPolicy,
		ReferenceVideoUnitPrice:   cfg.ReferenceVideoUnitPrice,
	})
}

func validateTaskPricingTier(tier TaskPricingTier) error {
	if !isFinitePositive(tier.NoReferenceVideoUnitPrice) {
		return fmt.Errorf("%w: no_reference_video_unit_price must be finite and greater than 0", ErrInvalidTaskPricing)
	}

	switch tier.ReferenceVideoPolicy {
	case ReferenceVideoPolicySame, ReferenceVideoPolicyDisabled:
		if !isFiniteNonNegative(tier.ReferenceVideoUnitPrice) {
			return fmt.Errorf("%w: reference_video_unit_price must be finite and non-negative", ErrInvalidTaskPricing)
		}
	case ReferenceVideoPolicyCustom:
		if !isFinitePositive(tier.ReferenceVideoUnitPrice) {
			return fmt.Errorf("%w: reference_video_unit_price must be finite and greater than 0 for custom policy", ErrInvalidTaskPricing)
		}
	default:
		return fmt.Errorf(
			"%w: reference_video_policy must be one of %q, %q, or %q",
			ErrInvalidTaskPricing,
			ReferenceVideoPolicySame,
			ReferenceVideoPolicyCustom,
			ReferenceVideoPolicyDisabled,
		)
	}
	return nil
}

// TaskPricingResolutionKeys returns sorted canonical keys for a matrix config.
// Legacy configs intentionally return nil because they apply to every
// resolution supported by the selected upstream capability.
func TaskPricingResolutionKeys(cfg TaskPricingConfig) []string {
	if cfg.ByResolution == nil {
		return nil
	}
	keys := make([]string, 0, len(cfg.ByResolution))
	for rawResolution := range cfg.ByResolution {
		resolution, err := NormalizeTaskPricingResolution(rawResolution)
		if err == nil {
			keys = append(keys, resolution)
		}
	}
	sort.SliceStable(keys, func(left, right int) bool {
		return TaskPricingResolutionLess(keys[left], keys[right])
	})
	return keys
}

// TaskPricingUnitPrices returns every billable per-second unit price in cfg.
func TaskPricingUnitPrices(cfg TaskPricingConfig) []float64 {
	prices := make([]float64, 0, max(2, len(cfg.ByResolution)*2))
	appendTier := func(tier TaskPricingTier) {
		prices = append(prices, tier.NoReferenceVideoUnitPrice)
		switch tier.ReferenceVideoPolicy {
		case ReferenceVideoPolicySame:
			prices = append(prices, tier.NoReferenceVideoUnitPrice)
		case ReferenceVideoPolicyCustom:
			prices = append(prices, tier.ReferenceVideoUnitPrice)
		}
	}
	if cfg.ByResolution == nil {
		appendTier(TaskPricingTier{
			NoReferenceVideoUnitPrice: cfg.NoReferenceVideoUnitPrice,
			ReferenceVideoPolicy:      cfg.ReferenceVideoPolicy,
			ReferenceVideoUnitPrice:   cfg.ReferenceVideoUnitPrice,
		})
		return prices
	}
	for _, tier := range cfg.ByResolution {
		appendTier(tier)
	}
	return prices
}

// ValidateTaskPricingMap validates every model entry without mutating it.
func ValidateTaskPricingMap(configs map[string]TaskPricingConfig) error {
	for model, cfg := range configs {
		if strings.TrimSpace(model) == "" || model != strings.TrimSpace(model) {
			return fmt.Errorf("%w: model name must be non-empty and have no surrounding whitespace", ErrInvalidTaskPricing)
		}
		if err := ValidateTaskPricingConfig(cfg); err != nil {
			return fmt.Errorf("model %q: %w", model, err)
		}
	}
	return nil
}

// QuoteTaskPricing selects the configured input-video variant and returns the
// local retail quote. quantity is expressed in cfg.Unit (currently seconds).
func QuoteTaskPricing(
	model string,
	quantity float64,
	resolution string,
	groupRatio float64,
	quotaPerUnit float64,
	hasReferenceVideo bool,
) (TaskPricingQuote, error) {
	cfg, ok := GetTaskPricing(model)
	if !ok {
		return TaskPricingQuote{}, fmt.Errorf("%w: model %q", ErrTaskPricingNotConfigured, model)
	}
	if err := ValidateTaskPricingConfig(cfg); err != nil {
		return TaskPricingQuote{}, fmt.Errorf("model %q: %w", model, err)
	}
	if !isFinitePositive(quantity) {
		return TaskPricingQuote{}, fmt.Errorf("%w: quantity must be finite and greater than 0", ErrInvalidTaskPricing)
	}
	if !isFiniteNonNegative(groupRatio) {
		return TaskPricingQuote{}, fmt.Errorf("%w: group ratio must be finite and non-negative", ErrInvalidTaskPricing)
	}
	if !isFinitePositive(quotaPerUnit) {
		return TaskPricingQuote{}, fmt.Errorf("%w: quota per unit must be finite and greater than 0", ErrInvalidTaskPricing)
	}

	canonicalResolution := ""
	if strings.TrimSpace(resolution) != "" {
		var err error
		canonicalResolution, err = NormalizeTaskPricingResolution(resolution)
		if err != nil {
			return TaskPricingQuote{}, fmt.Errorf("%w: %v", ErrTaskPricingResolutionRequired, err)
		}
	}

	tier := TaskPricingTier{
		NoReferenceVideoUnitPrice: cfg.NoReferenceVideoUnitPrice,
		ReferenceVideoPolicy:      cfg.ReferenceVideoPolicy,
		ReferenceVideoUnitPrice:   cfg.ReferenceVideoUnitPrice,
	}
	if cfg.ByResolution != nil {
		if canonicalResolution == "" {
			return TaskPricingQuote{}, fmt.Errorf("%w: model %q", ErrTaskPricingResolutionRequired, model)
		}
		var found bool
		tier, found = cfg.ByResolution[canonicalResolution]
		if !found {
			return TaskPricingQuote{}, fmt.Errorf("%w: model %q resolution %q", ErrTaskPricingResolutionNotConfigured, model, canonicalResolution)
		}
	}

	variant := TaskPricingVariantNoReferenceVideo
	unitPrice := tier.NoReferenceVideoUnitPrice
	if hasReferenceVideo {
		variant = TaskPricingVariantReferenceVideo
		switch tier.ReferenceVideoPolicy {
		case ReferenceVideoPolicySame:
			// The request remains the reference-video variant for logging, but
			// it intentionally reuses the no-reference-video unit price.
		case ReferenceVideoPolicyCustom:
			unitPrice = tier.ReferenceVideoUnitPrice
		case ReferenceVideoPolicyDisabled:
			return TaskPricingQuote{}, fmt.Errorf("%w: model %q", ErrReferenceVideoDisabled, model)
		}
	}

	baseUSD := unitPrice * quantity
	if !isFinitePositive(baseUSD) {
		return TaskPricingQuote{}, fmt.Errorf("%w: calculated sale USD is not finite and positive", ErrInvalidTaskPricing)
	}
	saleUSD := baseUSD * groupRatio
	if !isFiniteNonNegative(saleUSD) {
		return TaskPricingQuote{}, fmt.Errorf("%w: calculated sale USD is invalid", ErrInvalidTaskPricing)
	}
	quotaValue := saleUSD * quotaPerUnit
	if math.IsNaN(quotaValue) || math.IsInf(quotaValue, 0) || quotaValue < 0 {
		return TaskPricingQuote{}, fmt.Errorf("%w: calculated quota is invalid", ErrInvalidTaskPricing)
	}
	if quotaValue > float64(int(^uint(0)>>1)) {
		return TaskPricingQuote{}, fmt.Errorf("%w: calculated quota exceeds integer range", ErrInvalidTaskPricing)
	}

	return TaskPricingQuote{
		Unit:              cfg.Unit,
		Variant:           variant,
		UnitPriceUSD:      unitPrice,
		Quantity:          quantity,
		GroupRatio:        groupRatio,
		BaseUSD:           baseUSD,
		SaleUSD:           saleUSD,
		Quota:             billingexpr.QuotaRound(quotaValue),
		HasReferenceVideo: hasReferenceVideo,
		Resolution:        canonicalResolution,
	}, nil
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func isFiniteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
