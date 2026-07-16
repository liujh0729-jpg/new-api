package billing_setting

import (
	"errors"
	"fmt"
	"math"
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
)

var (
	ErrInvalidTaskPricing       = errors.New("invalid task pricing")
	ErrTaskPricingNotConfigured = errors.New("task pricing is not configured")
	ErrReferenceVideoDisabled   = errors.New("reference video input is disabled")
)

// TaskPricingConfig defines the local retail price for a duration-based task.
// Prices are stored in New API's USD base unit and are never sourced from the
// upstream provider at quote time.
type TaskPricingConfig struct {
	Unit                      string  `json:"unit"`
	NoReferenceVideoUnitPrice float64 `json:"no_reference_video_unit_price"`
	ReferenceVideoPolicy      string  `json:"reference_video_policy"`
	ReferenceVideoUnitPrice   float64 `json:"reference_video_unit_price,omitempty"`
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
	return cfg, ok
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
		cloned[model] = cfg
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
	if err := ValidateTaskPricingMap(configs); err != nil {
		return nil, err
	}
	return cloneTaskPricingMap(configs), nil
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
	if !isFinitePositive(cfg.NoReferenceVideoUnitPrice) {
		return fmt.Errorf("%w: no_reference_video_unit_price must be finite and greater than 0", ErrInvalidTaskPricing)
	}

	switch cfg.ReferenceVideoPolicy {
	case ReferenceVideoPolicySame, ReferenceVideoPolicyDisabled:
		if !isFiniteNonNegative(cfg.ReferenceVideoUnitPrice) {
			return fmt.Errorf("%w: reference_video_unit_price must be finite and non-negative", ErrInvalidTaskPricing)
		}
	case ReferenceVideoPolicyCustom:
		if !isFinitePositive(cfg.ReferenceVideoUnitPrice) {
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

	variant := TaskPricingVariantNoReferenceVideo
	unitPrice := cfg.NoReferenceVideoUnitPrice
	if hasReferenceVideo {
		variant = TaskPricingVariantReferenceVideo
		switch cfg.ReferenceVideoPolicy {
		case ReferenceVideoPolicySame:
			// The request remains the reference-video variant for logging, but
			// it intentionally reuses the no-reference-video unit price.
		case ReferenceVideoPolicyCustom:
			unitPrice = cfg.ReferenceVideoUnitPrice
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
	}, nil
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func isFiniteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}
