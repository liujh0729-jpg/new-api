package aipddcatalog

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

const AtomicCatalogPath = "/v1/new-api/catalog"

type AtomicCatalog struct {
	SchemaVersion int                `json:"schemaVersion"`
	Revision      string             `json:"revision"`
	GeneratedAt   string             `json:"generatedAt"`
	AWCoinRate    AtomicAWCoinRate   `json:"awcoinRate"`
	Capabilities  []AtomicCapability `json:"capabilities"`
	Models        []AtomicModel      `json:"models"`
}

type AtomicAWCoinRate struct {
	RMBPerAWCoin float64 `json:"rmbPerAwcoin"`
	USDPerAWCoin float64 `json:"usdPerAwcoin"`
	UpdatedAt    string  `json:"updatedAt"`
}

type AtomicExecution struct {
	Protocol string `json:"protocol"`
	Path     string `json:"path"`
}

type AtomicPricing struct {
	PricingModel         string                                             `json:"pricingModel"`
	Currency             string                                             `json:"currency"`
	Enabled              bool                                               `json:"enabled"`
	ChargeConfig         map[string]any                                     `json:"chargeConfig"`
	PromptPerMillion     float64                                            `json:"promptPerMillion"`
	CompletionPerMillion float64                                            `json:"completionPerMillion"`
	ByResolution         map[string]constant.AIPDDSeedanceResolutionPricing `json:"byResolution"`
}

type AtomicCapability struct {
	ID               string          `json:"id"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	AdapterCode      string          `json:"adapterCode"`
	EndpointType     string          `json:"endpointType"`
	TaskKind         string          `json:"taskKind"`
	InputModalities  []string        `json:"inputModalities"`
	OutputModalities []string        `json:"outputModalities"`
	Params           ScriptParams    `json:"params"`
	Available        bool            `json:"available"`
	Execution        AtomicExecution `json:"execution"`
	Pricing          AtomicPricing   `json:"pricing"`
}

type AtomicModel struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	InputModalities  []string        `json:"inputModalities"`
	OutputModalities []string        `json:"outputModalities"`
	Available        bool            `json:"available"`
	Execution        AtomicExecution `json:"execution"`
	Pricing          AtomicPricing   `json:"pricing"`
}

type atomicCatalogResponse struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    AtomicCatalog `json:"data"`
}

func FetchAtomic(ctx context.Context, client *http.Client, baseURL, apiKey string) (AtomicCatalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizeBaseURL(baseURL)+AtomicCatalogPath, nil)
	if err != nil {
		return AtomicCatalog{}, err
	}
	if key := strings.TrimSpace(apiKey); key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
		request.Header.Set("X-API-Key", key)
	}
	response, err := client.Do(request)
	if err != nil {
		return AtomicCatalog{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return AtomicCatalog{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return AtomicCatalog{}, fmt.Errorf("AIPDD atomic catalog returned HTTP %d", response.StatusCode)
	}
	var envelope atomicCatalogResponse
	if err := common.Unmarshal(body, &envelope); err != nil {
		return AtomicCatalog{}, fmt.Errorf("decode AIPDD atomic catalog: %w", err)
	}
	if err := validateAIPDDResponse(envelope.Code, envelope.Message, "fetch AIPDD atomic catalog"); err != nil {
		return AtomicCatalog{}, err
	}
	envelope.Data.FilterExcluded()
	if err := envelope.Data.Validate(); err != nil {
		return AtomicCatalog{}, err
	}
	return envelope.Data, nil
}

func (catalog *AtomicCatalog) FilterExcluded() {
	if catalog == nil {
		return
	}
	capabilities := catalog.Capabilities[:0]
	for _, capability := range catalog.Capabilities {
		if excludedAIPDDCatalogText(capability.AdapterCode, capability.Code, capability.ID, capability.Name) {
			continue
		}
		capabilities = append(capabilities, capability)
	}
	catalog.Capabilities = capabilities
	models := catalog.Models[:0]
	for _, model := range catalog.Models {
		if excludedAIPDDCatalogText(model.ID, model.Name) {
			continue
		}
		models = append(models, model)
	}
	catalog.Models = models
}

func excludedAIPDDCatalogText(values ...string) bool {
	return constant.IsAIPDDExcludedModel(strings.Join(values, " "))
}

func (catalog AtomicCatalog) Validate() error {
	if catalog.SchemaVersion != 1 {
		return fmt.Errorf("unsupported AIPDD catalog schemaVersion %d", catalog.SchemaVersion)
	}
	if strings.TrimSpace(catalog.Revision) == "" {
		return fmt.Errorf("AIPDD catalog revision is required")
	}
	if catalog.AWCoinRate.RMBPerAWCoin <= 0 || catalog.AWCoinRate.USDPerAWCoin <= 0 {
		return fmt.Errorf("AIPDD catalog AWCoin rate must be positive")
	}
	for _, capability := range catalog.Capabilities {
		if capability.AdapterCode != "comfyui" && capability.AdapterCode != "seedance" {
			return fmt.Errorf("unsupported AIPDD task adapter %q", capability.AdapterCode)
		}
		if strings.TrimSpace(capability.ID) == "" || strings.TrimSpace(capability.Execution.Protocol) == "" || strings.TrimSpace(capability.Execution.Path) == "" {
			return fmt.Errorf("AIPDD task capability has incomplete execution metadata")
		}
		if capability.AdapterCode == "seedance" {
			if err := validateSeedancePricing(capability.ID, capability.Pricing); err != nil {
				return err
			}
		}
	}
	for _, model := range catalog.Models {
		if strings.TrimSpace(model.ID) == "" || model.Pricing.PromptPerMillion < 0 || model.Pricing.CompletionPerMillion < 0 {
			return fmt.Errorf("invalid AIPDD LLM model entry")
		}
		if model.Pricing.PromptPerMillion == 0 && model.Pricing.CompletionPerMillion == 0 {
			return fmt.Errorf("AIPDD LLM model %q has no effective price", model.ID)
		}
	}
	return nil
}

func validateSeedancePricing(modelName string, pricing AtomicPricing) error {
	if !strings.EqualFold(strings.TrimSpace(pricing.PricingModel), "per_second") ||
		!strings.EqualFold(strings.TrimSpace(pricing.Currency), "awcoin") || !pricing.Enabled {
		return fmt.Errorf("AIPDD Seedance model %q has invalid pricing metadata", modelName)
	}
	if len(pricing.ByResolution) == 0 {
		return fmt.Errorf("AIPDD Seedance model %q has no resolution pricing", modelName)
	}
	for resolution, item := range pricing.ByResolution {
		resolution = strings.TrimSpace(resolution)
		if resolution == "" || !strings.EqualFold(resolution, strings.TrimSpace(item.TargetResolution)) {
			return fmt.Errorf(
				"AIPDD Seedance model %q has invalid targetResolution for %q: got %q",
				modelName,
				resolution,
				item.TargetResolution,
			)
		}
		fields := []struct {
			name  string
			value float64
		}{
			{name: "amountAwcoinPerSecond", value: item.AmountAWCoinPerSecond},
			{name: "textInputAwcoinPerSecond", value: item.TextInputAWCoinPerSecond},
			{name: "imageInputAwcoinPerSecond", value: item.ImageInputAWCoinPerSecond},
			{name: "videoInputAwcoinPerSecond", value: item.VideoInputAWCoinPerSecond},
			{name: "audioInputAwcoinPerSecond", value: item.AudioInputAWCoinPerSecond},
			{name: "defaultDurationSeconds", value: item.DefaultDurationSeconds},
			{name: "defaultFramesPerSecond", value: item.DefaultFramesPerSecond},
		}
		for _, field := range fields {
			if field.value <= 0 || math.IsNaN(field.value) || math.IsInf(field.value, 0) {
				return fmt.Errorf("AIPDD Seedance model %q resolution %q requires positive %s", modelName, resolution, field.name)
			}
		}
	}
	return nil
}

func (catalog AtomicCatalog) ModelNames() []string {
	seen := make(map[string]bool)
	models := make([]string, 0, len(catalog.Capabilities)+len(catalog.Models))
	for _, capability := range catalog.Capabilities {
		name := strings.TrimSpace(capability.ID)
		if name != "" && !excludedAIPDDCatalogText(capability.AdapterCode, capability.Code, capability.ID, capability.Name) && !seen[name] {
			seen[name] = true
			models = append(models, name)
		}
	}
	for _, model := range catalog.Models {
		name := strings.TrimSpace(model.ID)
		if name != "" && !excludedAIPDDCatalogText(model.ID, model.Name) && !seen[name] {
			seen[name] = true
			models = append(models, name)
		}
	}
	sort.Strings(models)
	return models
}

func (catalog AtomicCatalog) RuntimeCapabilities() []constant.AIPDDCapability {
	capabilities := make([]constant.AIPDDCapability, 0, len(catalog.Capabilities))
	for _, item := range catalog.Capabilities {
		if excludedAIPDDCatalogText(item.AdapterCode, item.Code, item.ID, item.Name) {
			continue
		}
		script := Script{
			ID: item.ID, Code: item.Code, Name: item.Name, Description: item.Description,
			AdapterCode: item.AdapterCode, EndpointType: item.EndpointType, TaskKind: item.TaskKind,
			InputModalities: item.InputModalities, OutputModalities: item.OutputModalities, Params: item.Params,
			PriceAWCoin: TaskAWCoinPrice(item.Pricing),
		}
		capability, _, ok := buildCapability(script, nil)
		if !ok {
			continue
		}
		capability.ModelName = strings.TrimSpace(item.ID)
		capability.CatalogRevision = catalog.Revision
		capability.ExecutionProtocol = item.Execution.Protocol
		capability.ExecutionPath = item.Execution.Path
		capability.AWCoinUSDPerCoin = catalog.AWCoinRate.USDPerAWCoin
		if item.AdapterCode == "seedance" {
			capability.BillingType = constant.AIPDDBillingTypeDurationSeconds
			capability.SeedancePricing = &constant.AIPDDSeedancePricing{ByResolution: item.Pricing.ByResolution}
		}
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

func TaskAWCoinPrice(pricing AtomicPricing) float64 {
	for _, key := range []string{"priceAWcoin", "chargeAwcoin", "amountAwcoin", "amount", "awcoin"} {
		if value, ok := pricing.ChargeConfig[key].(float64); ok && value > 0 {
			return value
		}
	}
	best := 0.0
	for _, resolution := range pricing.ByResolution {
		if resolution.DefaultDurationSeconds <= 0 || resolution.AmountAWCoinPerSecond <= 0 {
			continue
		}
		amount := math.Ceil(resolution.AmountAWCoinPerSecond * resolution.DefaultDurationSeconds)
		if amount > 0 && (best == 0 || amount < best) {
			best = amount
		}
	}
	return best
}

func MarshalAtomic(catalog AtomicCatalog) ([]byte, error) {
	return common.Marshal(catalog)
}

func UnmarshalAtomic(data []byte) (AtomicCatalog, error) {
	var catalog AtomicCatalog
	if err := common.Unmarshal(data, &catalog); err != nil {
		return catalog, err
	}
	catalog.FilterExcluded()
	return catalog, catalog.Validate()
}
