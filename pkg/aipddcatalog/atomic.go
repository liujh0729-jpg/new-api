package aipddcatalog

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

const AtomicCatalogPath = "/v1/new-api/catalog"

// supportedAtomicAdapters lists the upstream adapter families whose task
// capabilities the gateway can execute. token_market_shared submits through
// /shared-tasks/tasks, token_market_media through its own media endpoints.
var supportedAtomicAdapters = map[string]bool{
	"comfyui":             true,
	"seedance":            true,
	"token_market_media":  true,
	"token_market_shared": true,
}

// supportedAtomicProtocols lists the request protocols an AIPDD catalog model
// may declare. embeddings and systemone are synchronous, non-chat surfaces.
var supportedAtomicProtocols = map[string]bool{
	"chat":       true,
	"responses":  true,
	"messages":   true,
	"embeddings": true,
	"systemone":  true,
}

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
	PricingBasis         string                                             `json:"pricingBasis,omitempty"`
	BillingMode          string                                             `json:"billingMode,omitempty"`
	Enabled              bool                                               `json:"enabled"`
	Free                 bool                                               `json:"free,omitempty"`
	ChargeConfig         map[string]any                                     `json:"chargeConfig"`
	PromptPerMillion     float64                                            `json:"promptPerMillion"`
	CompletionPerMillion float64                                            `json:"completionPerMillion"`
	CacheReadPerMillion  *float64                                           `json:"cacheReadPerMillion"`
	CacheWritePerMillion *float64                                           `json:"cacheWritePerMillion"`
	ByResolution         map[string]constant.AIPDDSeedanceResolutionPricing `json:"byResolution"`
	ImagePricing         *AtomicImagePricing                                `json:"imagePricing,omitempty"`
}

// AtomicImagePricing carries the per-image retail tiers the upstream publishes
// beside chargeConfig. chargeConfig.amountAwcoin is only the floor price: a 4K
// request is billed from Tiers, so ignoring it under-charges the site.
type AtomicImagePricing struct {
	DefaultUnitRmb float64                  `json:"defaultUnitRmb"`
	Tiers          []AtomicImagePricingTier `json:"tiers"`
	SupportsEdits  bool                     `json:"supportsEdits"`
}

type AtomicImagePricingTier struct {
	Match   map[string]any `json:"match"`
	UnitRmb float64        `json:"unitRmb"`
}

type AtomicCapability struct {
	ID               string       `json:"id"`
	Code             string       `json:"code"`
	Name             string       `json:"name"`
	Description      string       `json:"description"`
	AdapterCode      string       `json:"adapterCode"`
	EndpointType     string       `json:"endpointType"`
	TaskKind         string       `json:"taskKind"`
	InputModalities  []string     `json:"inputModalities"`
	OutputModalities []string     `json:"outputModalities"`
	Params           ScriptParams `json:"params"`
	// Available is optional. Java ComfyUI entries historically omitted it;
	// a missing or null value must not be treated as false.
	Available *bool           `json:"available"`
	Execution AtomicExecution `json:"execution"`
	Pricing   AtomicPricing   `json:"pricing"`
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
	Protocols        []string        `json:"protocols,omitempty"`
	Features         *AtomicFeatures `json:"features,omitempty"`
}

type AtomicFeatures struct {
	ImageSources  []string                              `json:"imageSources,omitempty"`
	FunctionTools AtomicFunctionTools                   `json:"functionTools"`
	Usage         AtomicUsage                           `json:"usage"`
	ByProtocol    map[string]AtomicProtocolCapabilities `json:"byProtocol,omitempty"`
}

type AtomicProtocolCapabilities struct {
	InputModalities []string            `json:"inputModalities,omitempty"`
	ImageSources    []string            `json:"imageSources,omitempty"`
	FunctionTools   AtomicFunctionTools `json:"functionTools"`
	Usage           AtomicUsage         `json:"usage"`
}

type AtomicFunctionTools struct {
	Basic      bool `json:"basic"`
	Strict     bool `json:"strict"`
	Parallel   bool `json:"parallel"`
	Streaming  bool `json:"streaming"`
	MultiRound bool `json:"multiRound"`
}

type AtomicUsage struct {
	Streaming    bool `json:"streaming"`
	NonStreaming bool `json:"nonStreaming"`
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
	envelope.Data.NormalizePerUnitChargeUnits()
	if err := envelope.Data.Validate(); err != nil {
		return AtomicCatalog{}, err
	}
	envelope.Data.FilterFreeModels()
	return envelope.Data, nil
}

// NormalizePerUnitChargeUnits fills chargeConfig.unit=second when a shared-compute
// snapshot only recorded unitLabel=second. Without this, one LTX row rejects the
// entire catalog and Seedance pricing never loads.
func (catalog *AtomicCatalog) NormalizePerUnitChargeUnits() {
	if catalog == nil {
		return
	}
	for i := range catalog.Capabilities {
		pricing := &catalog.Capabilities[i].Pricing
		if !strings.EqualFold(strings.TrimSpace(pricing.PricingModel), "per_unit") {
			continue
		}
		if pricing.ChargeConfig == nil {
			continue
		}
		if strings.TrimSpace(anyToString(pricing.ChargeConfig["unit"])) != "" {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(anyToString(pricing.ChargeConfig["unitLabel"])), "second") {
			continue
		}
		pricing.ChargeConfig["unit"] = "second"
	}
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
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
	if catalog.SchemaVersion != 1 && catalog.SchemaVersion != 2 {
		return fmt.Errorf("unsupported AIPDD catalog schemaVersion %d", catalog.SchemaVersion)
	}
	if strings.TrimSpace(catalog.Revision) == "" {
		return fmt.Errorf("AIPDD catalog revision is required")
	}
	if catalog.AWCoinRate.RMBPerAWCoin <= 0 || catalog.AWCoinRate.USDPerAWCoin <= 0 {
		return fmt.Errorf("AIPDD catalog AWCoin rate must be positive")
	}
	for _, capability := range catalog.Capabilities {
		if !supportedAtomicAdapters[capability.AdapterCode] {
			return fmt.Errorf("unsupported AIPDD task adapter %q", capability.AdapterCode)
		}
		if strings.TrimSpace(capability.ID) == "" || strings.TrimSpace(capability.Execution.Protocol) == "" || strings.TrimSpace(capability.Execution.Path) == "" {
			return fmt.Errorf("AIPDD task capability has incomplete execution metadata")
		}
		if capability.AdapterCode == "seedance" ||
			(capability.AdapterCode == "token_market_media" &&
				strings.EqualFold(strings.TrimSpace(capability.Pricing.PricingModel), "per_second")) {
			if err := validateSeedancePricing(capability.ID, capability.Pricing); err != nil {
				return err
			}
		} else if strings.EqualFold(strings.TrimSpace(capability.Pricing.PricingModel), "per_unit") {
			if err := validateDurationUnitPricing(capability.ID, capability.Pricing); err != nil {
				return err
			}
		}
	}
	for _, model := range catalog.Models {
		if strings.TrimSpace(model.ID) == "" || model.Pricing.PromptPerMillion < 0 || model.Pricing.CompletionPerMillion < 0 {
			return fmt.Errorf("invalid AIPDD LLM model entry")
		}
		// Cache lanes became mandatory in schema v2. Schema v1 snapshots did
		// not carry them, so they must remain readable during startup/fallback.
		if catalog.SchemaVersion >= 2 && (model.Pricing.CacheReadPerMillion == nil || model.Pricing.CacheWritePerMillion == nil) {
			return fmt.Errorf("AIPDD LLM model %q must provide cache read and cache write prices", model.ID)
		}
		if model.Pricing.CacheReadPerMillion != nil && *model.Pricing.CacheReadPerMillion < 0 {
			return fmt.Errorf("AIPDD LLM model %q has a negative cache read price", model.ID)
		}
		if model.Pricing.CacheWritePerMillion != nil && *model.Pricing.CacheWritePerMillion < 0 {
			return fmt.Errorf("AIPDD LLM model %q has a negative cache write price", model.ID)
		}
		if model.Pricing.Free {
			if !strings.EqualFold(strings.TrimSpace(model.Pricing.PricingModel), "per_token") ||
				!strings.EqualFold(strings.TrimSpace(model.Pricing.Currency), "awcoin") || !model.Pricing.Enabled {
				return fmt.Errorf("AIPDD free LLM model %q has invalid pricing metadata", model.ID)
			}
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(model.ID)), "free-") {
				return fmt.Errorf("AIPDD LLM model %q declares free pricing without a free- model ID", model.ID)
			}
			if model.Pricing.PromptPerMillion != 0 || model.Pricing.CompletionPerMillion != 0 ||
				(model.Pricing.CacheReadPerMillion != nil && *model.Pricing.CacheReadPerMillion != 0) ||
				(model.Pricing.CacheWritePerMillion != nil && *model.Pricing.CacheWritePerMillion != 0) {
				return fmt.Errorf("AIPDD free LLM model %q must have zero prices in every token lane", model.ID)
			}
		} else if model.Pricing.PromptPerMillion == 0 && model.Pricing.CompletionPerMillion == 0 {
			return fmt.Errorf("AIPDD LLM model %q has no effective price", model.ID)
		}
		if catalog.SchemaVersion >= 2 {
			for _, protocol := range model.Protocols {
				value := strings.ToLower(strings.TrimSpace(protocol))
				if !supportedAtomicProtocols[value] {
					return fmt.Errorf("AIPDD LLM model %q has unsupported protocol %q", model.ID, protocol)
				}
			}
			if model.Features != nil {
				for protocol := range model.Features.ByProtocol {
					value := strings.ToLower(strings.TrimSpace(protocol))
					if !supportedAtomicProtocols[value] {
						return fmt.Errorf("AIPDD LLM model %q has unsupported capability protocol %q", model.ID, protocol)
					}
				}
			}
		}
	}
	return nil
}

func validateDurationUnitPricing(modelName string, pricing AtomicPricing) error {
	if !strings.EqualFold(strings.TrimSpace(pricing.Currency), "awcoin") || !pricing.Enabled {
		return fmt.Errorf("AIPDD per-unit model %q has invalid pricing metadata", modelName)
	}
	unit, _ := pricing.ChargeConfig["unit"].(string)
	if !strings.EqualFold(strings.TrimSpace(unit), "second") {
		return fmt.Errorf("AIPDD per-unit model %q has unsupported charge unit %q", modelName, unit)
	}
	amount := TaskAWCoinPrice(pricing)
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return fmt.Errorf("AIPDD per-unit model %q requires a positive AWCoin amount per second", modelName)
	}
	return nil
}

func validateSeedancePricing(modelName string, pricing AtomicPricing) error {
	if !strings.EqualFold(strings.TrimSpace(pricing.PricingModel), "per_second") ||
		!strings.EqualFold(strings.TrimSpace(pricing.Currency), "awcoin") || !pricing.Enabled {
		return fmt.Errorf("AIPDD Seedance model %q has invalid pricing metadata", modelName)
	}
	if basis := strings.TrimSpace(pricing.PricingBasis); !strings.EqualFold(basis, "display") {
		return fmt.Errorf("AIPDD Seedance model %q requires pricingBasis %q", modelName, "display")
	}
	if len(pricing.ByResolution) == 0 {
		return fmt.Errorf("AIPDD Seedance model %q has no resolution pricing", modelName)
	}
	for resolution, item := range pricing.ByResolution {
		if item.DisplayAmountAWCoinPerSecond == nil || item.DisplayVideoInputAWCoinPerSecond == nil {
			return fmt.Errorf(
				"AIPDD Seedance model %q resolution %q requires explicit display pricing fields",
				modelName,
				resolution,
			)
		}
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
			{name: "displayAmountAwcoinPerSecond", value: *item.DisplayAmountAWCoinPerSecond},
			{name: "displayVideoInputAwcoinPerSecond", value: *item.DisplayVideoInputAWCoinPerSecond},
			{name: "defaultFramesPerSecond", value: item.DefaultFramesPerSecond},
		}
		for _, field := range fields {
			if field.value <= 0 || math.IsNaN(field.value) || math.IsInf(field.value, 0) {
				return fmt.Errorf("AIPDD Seedance model %q resolution %q requires positive %s", modelName, resolution, field.name)
			}
		}
		if item.DefaultDurationSeconds != -1 || !isSeedance25Model(modelName) {
			if item.DefaultDurationSeconds <= 0 || math.IsNaN(item.DefaultDurationSeconds) || math.IsInf(item.DefaultDurationSeconds, 0) {
				return fmt.Errorf("AIPDD Seedance model %q resolution %q requires positive defaultDurationSeconds", modelName, resolution)
			}
		}
	}
	return nil
}

func isSeedance25Model(modelName string) bool {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(modelName)), func(r rune) bool {
		return unicode.IsSpace(r) || r == '-' || r == '_' || r == '.'
	})
	for index := 0; index+2 < len(parts); index++ {
		if parts[index] == "seedance" && parts[index+1] == "2" && parts[index+2] == "5" {
			return true
		}
	}
	return false
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
		if protocol, path, ok := item.SynchronousExecution(); ok {
			capability.Synchronous = true
			capability.ExecutionProtocol = protocol
			capability.ExecutionPath = path
		}
		if item.Pricing.ImagePricing != nil {
			applyImagePricing(&capability, item.Pricing.ImagePricing, catalog.AWCoinRate.RMBPerAWCoin)
		}
		if item.AdapterCode == "seedance" ||
			(item.AdapterCode == "token_market_media" &&
				strings.EqualFold(strings.TrimSpace(item.Pricing.PricingModel), "per_second")) {
			capability.BillingType = constant.AIPDDBillingTypeDurationSeconds
			capability.SeedancePricing = &constant.AIPDDSeedancePricing{
				BillingMode:  item.Pricing.BillingMode,
				ByResolution: normalizeSeedanceDisplayPricingMatrix(item.Pricing.ByResolution),
			}
		} else if strings.EqualFold(strings.TrimSpace(item.Pricing.PricingModel), "per_unit") &&
			chargeConfigUnit(item.Pricing.ChargeConfig) == "second" {
			capability.BillingType = constant.AIPDDBillingTypeDurationSeconds
		}
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

// synchronousAIPDDEndpoint lists the catalog task kinds the upstream answers
// directly on a media endpoint even though the catalog dispatches them through
// the shared-task API. Both were verified against the live upstream:
// image-to-image on /v1/images/edits and ASR on /v1/audio/transcriptions.
var synchronousAIPDDEndpoint = map[string]string{
	"image_to_image": "/v1/images/edits",
	"speech_to_text": "/v1/audio/transcriptions",
}

// executionProtocolImage marks the capabilities the upstream already serves
// synchronously on /v1/images/generations, so they must bypass task submission.
const executionProtocolImage = "token_market_image"

// SynchronousExecution reports the protocol and path the gateway calls a
// capability on directly, and whether it is called directly at all.
// RuntimeCapabilities and the pricing importers share this one decision so the
// relay route and the imported price can never disagree about a model.
func (item AtomicCapability) SynchronousExecution() (protocol string, path string, ok bool) {
	if syncPath, ok := synchronousAIPDDEndpoint[strings.TrimSpace(item.TaskKind)]; ok {
		return "openai", syncPath, true
	}
	if strings.EqualFold(strings.TrimSpace(item.Execution.Protocol), executionProtocolImage) {
		return item.Execution.Protocol, item.Execution.Path, true
	}
	return "", "", false
}

// SynchronousPerSecondPrices returns the retail price per second, in AWCoin, of
// every capability the gateway calls synchronously and the upstream charges per
// unit of time (ASR). Their price cannot be imported as a per-call price: one
// request costs as many seconds of audio as it carries.
func (catalog AtomicCatalog) SynchronousPerSecondPrices() map[string]float64 {
	prices := make(map[string]float64)
	for _, item := range catalog.Capabilities {
		if _, _, ok := item.SynchronousExecution(); !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(item.Pricing.PricingModel), "per_unit") ||
			chargeConfigUnit(item.Pricing.ChargeConfig) != "second" {
			continue
		}
		if !item.Pricing.Enabled ||
			(item.Available != nil && !*item.Available) ||
			!strings.EqualFold(strings.TrimSpace(item.Pricing.Currency), "awcoin") {
			continue
		}
		modelName := strings.TrimSpace(item.ID)
		if price := TaskAWCoinPrice(item.Pricing); modelName != "" && price > 0 {
			prices[modelName] = price
		}
	}
	return prices
}

// applyImagePricing copies the retail per-image tiers onto the runtime
// capability. The catalog publishes them in RMB; chargeConfig.amountAwcoin only
// carries the cheapest tier, so the tiers are converted to AWCoin here and used
// by the billing path to price high-resolution requests.
func applyImagePricing(capability *constant.AIPDDCapability, pricing *AtomicImagePricing, rmbPerAWCoin float64) {
	if capability == nil || pricing == nil {
		return
	}
	// A generation capability that accepts reference images also answers the
	// edits endpoint, so it is listed on both.
	if pricing.SupportsEdits && capability.EndpointType == constant.EndpointTypeImageGeneration {
		endpointTypes := append([]constant.EndpointType(nil), capability.EndpointTypes...)
		if !containsAIPDDEndpointType(endpointTypes, constant.EndpointTypeImageEdits) {
			capability.EndpointTypes = append(endpointTypes, constant.EndpointTypeImageEdits)
		}
	}
	if rmbPerAWCoin <= 0 {
		return
	}
	toAWCoin := func(rmb float64) float64 {
		if rmb <= 0 {
			return 0
		}
		return rmb / rmbPerAWCoin
	}

	imagePricing := &constant.AIPDDImagePricing{
		DefaultUnitAWCoin: toAWCoin(pricing.DefaultUnitRmb),
		Tiers:             make([]constant.AIPDDImagePricingTier, 0, len(pricing.Tiers)),
	}
	for _, tier := range pricing.Tiers {
		match := make(map[string]string, len(tier.Match))
		for key, value := range tier.Match {
			match[strings.ToLower(strings.TrimSpace(key))] = anyToString(value)
		}
		imagePricing.Tiers = append(imagePricing.Tiers, constant.AIPDDImagePricingTier{
			Match:      match,
			UnitAWCoin: toAWCoin(tier.UnitRmb),
		})
	}
	capability.ImagePricing = imagePricing
}

func containsAIPDDEndpointType(endpointTypes []constant.EndpointType, endpointType constant.EndpointType) bool {
	for _, item := range endpointTypes {
		if item == endpointType {
			return true
		}
	}
	return false
}

// runtimeEndpointTypes returns the client endpoints each task capability is
// reachable on. It reads the same mapping as RuntimeCapabilities so the stored
// model metadata and the runtime relay decisions can never diverge.
func (catalog AtomicCatalog) runtimeEndpointTypes() map[string][]constant.EndpointType {
	endpoints := make(map[string][]constant.EndpointType)
	for _, capability := range catalog.RuntimeCapabilities() {
		if len(capability.EndpointTypes) > 0 {
			endpoints[capability.ModelName] = capability.EndpointTypes
			continue
		}
		if capability.EndpointType != "" {
			endpoints[capability.ModelName] = []constant.EndpointType{capability.EndpointType}
		}
	}
	return endpoints
}

// ModelMetadata returns the catalog model protocols in the form the runtime
// endpoint lookup consumes.
func (catalog AtomicCatalog) ModelMetadata() []constant.AIPDDModelMeta {
	metas := make([]constant.AIPDDModelMeta, 0, len(catalog.Models))
	for _, model := range catalog.Models {
		metas = append(metas, constant.AIPDDModelMeta{
			ID:        model.ID,
			Protocols: append([]string(nil), model.Protocols...),
		})
	}
	return metas
}

// ModelEndpointTypes returns the client endpoints for every catalog model,
// covering both task capabilities and protocol-based models.
func (catalog AtomicCatalog) ModelEndpointTypes() map[string][]constant.EndpointType {
	endpoints := catalog.runtimeEndpointTypes()
	for _, model := range catalog.Models {
		name := strings.TrimSpace(model.ID)
		if name == "" || len(endpoints[name]) > 0 {
			continue
		}
		if endpointTypes := constant.AIPDDEndpointTypesForProtocols(model.Protocols); len(endpointTypes) > 0 {
			endpoints[name] = endpointTypes
		}
	}
	return endpoints
}

func chargeConfigUnit(chargeConfig map[string]any) string {
	unit, _ := chargeConfig["unit"].(string)
	return strings.ToLower(strings.TrimSpace(unit))
}

func normalizeSeedanceDisplayPricingMatrix(
	byResolution map[string]constant.AIPDDSeedanceResolutionPricing,
) map[string]constant.AIPDDSeedanceResolutionPricing {
	normalized := make(map[string]constant.AIPDDSeedanceResolutionPricing, len(byResolution))
	for resolution, pricing := range byResolution {
		normalized[resolution] = normalizeSeedanceDisplayPricing(pricing)
	}
	return normalized
}

func normalizeSeedanceDisplayPricing(
	pricing constant.AIPDDSeedanceResolutionPricing,
) constant.AIPDDSeedanceResolutionPricing {
	if pricing.DisplayAmountAWCoinPerSecond != nil {
		pricing.AmountAWCoinPerSecond = *pricing.DisplayAmountAWCoinPerSecond
		pricing.TextInputAWCoinPerSecond = *pricing.DisplayAmountAWCoinPerSecond
		pricing.ImageInputAWCoinPerSecond = *pricing.DisplayAmountAWCoinPerSecond
		pricing.AudioInputAWCoinPerSecond = *pricing.DisplayAmountAWCoinPerSecond
	}
	if pricing.DisplayVideoInputAWCoinPerSecond != nil {
		pricing.VideoInputAWCoinPerSecond = *pricing.DisplayVideoInputAWCoinPerSecond
	}
	return pricing
}

func TaskAWCoinPrice(pricing AtomicPricing) float64 {
	for _, key := range []string{"priceAWcoin", "chargeAwcoin", "amountAwcoin", "amount", "awcoin"} {
		if value, ok := pricing.ChargeConfig[key].(float64); ok && value > 0 {
			return value
		}
	}
	best := 0.0
	for _, resolution := range pricing.ByResolution {
		duration := resolution.DefaultDurationSeconds
		if duration == -1 {
			duration = 30
		}
		if resolution.DisplayAmountAWCoinPerSecond == nil || duration <= 0 ||
			*resolution.DisplayAmountAWCoinPerSecond <= 0 {
			continue
		}
		amount := math.Ceil(*resolution.DisplayAmountAWCoinPerSecond * duration)
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
	catalog.NormalizePerUnitChargeUnits()
	if err := catalog.Validate(); err != nil {
		return catalog, err
	}
	catalog.FilterFreeModels()
	return catalog, nil
}
