package constant

import (
	"strings"
	"sync"
)

const (
	AIPDDLogoPath   = "/aipdd-logo.png"
	AIPDDWebsiteURL = "https://app.aipdd.work"
)

type AIPDDBillingType string

const (
	AIPDDBillingTypePerCall         AIPDDBillingType = "per_call"
	AIPDDBillingTypeDurationSeconds AIPDDBillingType = "duration_seconds"
)

type AIPDDWorkflowValueType string

const (
	AIPDDWorkflowValueTypeString AIPDDWorkflowValueType = "string"
	AIPDDWorkflowValueTypeInt    AIPDDWorkflowValueType = "int"
)

type AIPDDWorkflowSourceType string

const (
	AIPDDWorkflowSourceMetadata       AIPDDWorkflowSourceType = "metadata"
	AIPDDWorkflowSourcePrompt         AIPDDWorkflowSourceType = "prompt"
	AIPDDWorkflowSourceImage          AIPDDWorkflowSourceType = "image"
	AIPDDWorkflowSourceFirstImage     AIPDDWorkflowSourceType = "first_image"
	AIPDDWorkflowSourceLastImage      AIPDDWorkflowSourceType = "last_image"
	AIPDDWorkflowSourceInputReference AIPDDWorkflowSourceType = "input_reference"
	AIPDDWorkflowSourceDuration       AIPDDWorkflowSourceType = "duration"
	AIPDDWorkflowSourceStatic         AIPDDWorkflowSourceType = "static"
)

type AIPDDWorkflowValueSource struct {
	Type AIPDDWorkflowSourceType
	Key  string
}

type AIPDDWorkflowParamDefault struct {
	ParamKey  string
	ValueType AIPDDWorkflowValueType
	Sources   []AIPDDWorkflowValueSource
}

type AIPDDWorkflowParamConstraint struct {
	ParamKey string
	DataType string
	Min      *float64
	Max      *float64
	Allowed  []any
}

type AIPDDCapability struct {
	ModelName              string
	AdapterCode            string
	ScriptID               string
	ScriptCode             string
	TaskKind               string
	InputModalities        []string
	OutputModalities       []string
	TaskCost               float64
	WorkflowParamKeys      []string
	RequiredWorkflowParams map[string]bool
	WorkflowDefaults       []AIPDDWorkflowParamDefault
	WorkflowConstraints    []AIPDDWorkflowParamConstraint
	EndpointType           EndpointType
	// EndpointTypes lists every client endpoint the capability answers on, the
	// primary one first. A generation model that accepts reference images is
	// reachable on both /v1/images/generations and /v1/images/edits.
	EndpointTypes     []EndpointType
	BillingType       AIPDDBillingType
	CatalogRevision   string
	ExecutionProtocol string
	ExecutionPath     string
	AWCoinUSDPerCoin  float64
	SeedancePricing   *AIPDDSeedancePricing
	// Synchronous marks capabilities the gateway calls directly on their media
	// endpoint instead of submitting through the shared-task API. The catalog
	// declares ASR and image-to-image as shared_task, but both are verified to
	// answer synchronously on /v1/audio/transcriptions and /v1/images/edits.
	// Image generation already declares the synchronous protocol itself.
	Synchronous  bool
	ImagePricing *AIPDDImagePricing
}

// AIPDDImagePricing holds the retail per-image tiers published beside
// chargeConfig. chargeConfig.amountAwcoin is only the cheapest tier, so billing
// from it alone under-charges high-resolution requests. Amounts are AWCoin.
type AIPDDImagePricing struct {
	DefaultUnitAWCoin float64
	Tiers             []AIPDDImagePricingTier
}

type AIPDDImagePricingTier struct {
	Match      map[string]string
	UnitAWCoin float64
}

type AIPDDSeedanceResolutionPricing struct {
	TargetResolution                         string   `json:"targetResolution"`
	EnhancementType                          string   `json:"enhancementType"`
	InputTypes                               []string `json:"inputTypes"`
	AmountAWCoinPerSecond                    float64  `json:"amountAwcoinPerSecond"`
	DisplayAmountAWCoinPerSecond             *float64 `json:"displayAmountAwcoinPerSecond,omitempty"`
	BYOKAmountAWCoinPerSecond                *float64 `json:"byokAmountAwcoinPerSecond,omitempty"`
	SuggestedRetailAWCoinPerSecond           *float64 `json:"suggestedRetailAwcoinPerSecond,omitempty"`
	TextInputAWCoinPerSecond                 float64  `json:"textInputAwcoinPerSecond"`
	ImageInputAWCoinPerSecond                float64  `json:"imageInputAwcoinPerSecond"`
	VideoInputAWCoinPerSecond                float64  `json:"videoInputAwcoinPerSecond"`
	DisplayVideoInputAWCoinPerSecond         *float64 `json:"displayVideoInputAwcoinPerSecond,omitempty"`
	BYOKVideoInputAWCoinPerSecond            *float64 `json:"byokVideoInputAwcoinPerSecond,omitempty"`
	SuggestedRetailVideoInputAWCoinPerSecond *float64 `json:"suggestedRetailVideoInputAwcoinPerSecond,omitempty"`
	AudioInputAWCoinPerSecond                float64  `json:"audioInputAwcoinPerSecond"`
	DefaultDurationSeconds                   float64  `json:"defaultDurationSeconds"`
	DefaultFramesPerSecond                   float64  `json:"defaultFramesPerSecond"`
}

type AIPDDSeedancePricing struct {
	BillingMode  string                                    `json:"billingMode,omitempty"`
	ByResolution map[string]AIPDDSeedanceResolutionPricing `json:"byResolution"`
}

var AIPDDCapabilities []AIPDDCapability

var AIPDDTaskModelList = buildAIPDDTaskModelList(AIPDDCapabilities)

var AIPDDOpenAIModelList []string

// AIPDDModelMeta carries the per-model metadata the catalog publishes beside the
// model id: the request protocols it answers, from which the client-facing
// endpoints are derived. AIPDDOpenAIModelList stays the flat name list every
// existing consumer reads. Availability is not repeated here; catalog sync
// already hides unavailable models through V1ModelsListHiddenNames.
type AIPDDModelMeta struct {
	ID        string
	Protocols []string
}

var aipddModelMetaByID = map[string]AIPDDModelMeta{}

// AIPDDModelTokenPrice is the upstream list price of a token-bearing model, in
// AWCoin per million tokens. It is the cost basis for the transit order cost
// estimate; NewAPI's own retail ratios are deliberately not used there.
type AIPDDModelTokenPrice struct {
	ModelName            string
	PromptPerMillion     float64
	CompletionPerMillion float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

var aipddTokenPriceByModel = map[string]AIPDDModelTokenPrice{}

// aipddRMBPerAWCoin is the exchange rate the upstream publishes beside its
// price list. Cost estimates must use it rather than NewAPI's own USD rate,
// because the two are not derived from each other.
var aipddRMBPerAWCoin float64

var aipddCapabilityByAlias = buildAIPDDCapabilityByAlias(AIPDDCapabilities)

var aipddCapabilitiesLock sync.RWMutex

func buildAIPDDTaskModelList(capabilities []AIPDDCapability) []string {
	models := make([]string, 0, len(capabilities))
	seen := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		modelName := strings.TrimSpace(capability.ModelName)
		if modelName == "" || IsAIPDDExcludedModel(modelName) || seen[modelName] {
			continue
		}
		models = append(models, modelName)
		seen[modelName] = true
	}
	return models
}

func buildAIPDDCapabilityByAlias(capabilities []AIPDDCapability) map[string]AIPDDCapability {
	out := make(map[string]AIPDDCapability, len(capabilities)*2)
	for _, capability := range capabilities {
		capability = cloneAIPDDCapability(capability)
		for _, alias := range []string{capability.ModelName, capability.ScriptCode} {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias == "" {
				continue
			}
			out[alias] = capability
		}
	}
	return out
}

func cloneAIPDDCapabilities(capabilities []AIPDDCapability) []AIPDDCapability {
	out := make([]AIPDDCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		out = append(out, cloneAIPDDCapability(capability))
	}
	return out
}

func cloneAIPDDCapability(capability AIPDDCapability) AIPDDCapability {
	capability.InputModalities = append([]string(nil), capability.InputModalities...)
	capability.OutputModalities = append([]string(nil), capability.OutputModalities...)
	capability.WorkflowParamKeys = append([]string(nil), capability.WorkflowParamKeys...)
	capability.EndpointTypes = append([]EndpointType(nil), capability.EndpointTypes...)
	if capability.RequiredWorkflowParams != nil {
		required := make(map[string]bool, len(capability.RequiredWorkflowParams))
		for key, value := range capability.RequiredWorkflowParams {
			required[key] = value
		}
		capability.RequiredWorkflowParams = required
	}
	if capability.WorkflowDefaults != nil {
		defaults := make([]AIPDDWorkflowParamDefault, 0, len(capability.WorkflowDefaults))
		for _, item := range capability.WorkflowDefaults {
			item.Sources = append([]AIPDDWorkflowValueSource(nil), item.Sources...)
			defaults = append(defaults, item)
		}
		capability.WorkflowDefaults = defaults
	}
	if capability.WorkflowConstraints != nil {
		constraints := make([]AIPDDWorkflowParamConstraint, 0, len(capability.WorkflowConstraints))
		for _, item := range capability.WorkflowConstraints {
			item.Allowed = append([]any(nil), item.Allowed...)
			constraints = append(constraints, item)
		}
		capability.WorkflowConstraints = constraints
	}
	if capability.SeedancePricing != nil {
		pricing := &AIPDDSeedancePricing{
			BillingMode:  capability.SeedancePricing.BillingMode,
			ByResolution: make(map[string]AIPDDSeedanceResolutionPricing, len(capability.SeedancePricing.ByResolution)),
		}
		for resolution, item := range capability.SeedancePricing.ByResolution {
			item.InputTypes = append([]string(nil), item.InputTypes...)
			pricing.ByResolution[resolution] = item
		}
		capability.SeedancePricing = pricing
	}
	if capability.ImagePricing != nil {
		imagePricing := &AIPDDImagePricing{
			DefaultUnitAWCoin: capability.ImagePricing.DefaultUnitAWCoin,
			Tiers:             make([]AIPDDImagePricingTier, 0, len(capability.ImagePricing.Tiers)),
		}
		for _, tier := range capability.ImagePricing.Tiers {
			match := make(map[string]string, len(tier.Match))
			for key, value := range tier.Match {
				match[key] = value
			}
			tier.Match = match
			imagePricing.Tiers = append(imagePricing.Tiers, tier)
		}
		capability.ImagePricing = imagePricing
	}
	return capability
}

func SetAIPDDCapabilities(capabilities []AIPDDCapability) {
	filtered := make([]AIPDDCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		if IsAIPDDExcludedModel(capability.ModelName) ||
			IsAIPDDExcludedModel(capability.ScriptCode) ||
			IsAIPDDExcludedModel(capability.TaskKind) {
			continue
		}
		filtered = append(filtered, capability)
	}
	cloned := cloneAIPDDCapabilities(filtered)
	aipddCapabilitiesLock.Lock()
	defer aipddCapabilitiesLock.Unlock()
	AIPDDCapabilities = cloned
	AIPDDTaskModelList = buildAIPDDTaskModelList(cloned)
	aipddCapabilityByAlias = buildAIPDDCapabilityByAlias(cloned)
}

func SetAIPDDOpenAIModels(models []string) {
	setAIPDDOpenAIModels(models, nil)
}

// SetAIPDDOpenAIModelMeta publishes the catalog model list together with the
// protocols each model answers. The flat name list stays in sync so every
// existing consumer keeps working.
func SetAIPDDOpenAIModelMeta(metas []AIPDDModelMeta) {
	models := make([]string, 0, len(metas))
	for _, meta := range metas {
		models = append(models, meta.ID)
	}
	setAIPDDOpenAIModels(models, metas)
}

func setAIPDDOpenAIModels(models []string, metas []AIPDDModelMeta) {
	models = normalizeAIPDDModelList(models)
	kept := make(map[string]bool, len(models))
	for _, modelName := range models {
		kept[modelName] = true
	}

	index := make(map[string]AIPDDModelMeta, len(metas))
	for _, meta := range metas {
		meta.ID = strings.TrimSpace(meta.ID)
		if meta.ID == "" || !kept[meta.ID] {
			continue
		}
		key := strings.ToLower(meta.ID)
		if _, exists := index[key]; exists {
			continue
		}
		meta.Protocols = normalizeAIPDDStringList(meta.Protocols)
		index[key] = meta
	}

	aipddCapabilitiesLock.Lock()
	defer aipddCapabilitiesLock.Unlock()
	AIPDDOpenAIModelList = models
	aipddModelMetaByID = index
}

// ResetAIPDDCapabilities clears the runtime capability table. AIPDD has no
// built-in fallback models: every capability comes from the upstream catalog.
func ResetAIPDDCapabilities() {
	SetAIPDDCapabilities(nil)
}

func ResetAIPDDOpenAIModels() {
	SetAIPDDOpenAIModels(nil)
}

func GetAIPDDCapability(modelName string) (AIPDDCapability, bool) {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	capability, ok := aipddCapabilityByAlias[strings.ToLower(strings.TrimSpace(modelName))]
	return cloneAIPDDCapability(capability), ok
}

func GetAIPDDCapabilities() []AIPDDCapability {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return cloneAIPDDCapabilities(AIPDDCapabilities)
}

func GetAIPDDTaskModelList() []string {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return append([]string(nil), AIPDDTaskModelList...)
}

func GetAIPDDOpenAIModelList() []string {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return append([]string(nil), AIPDDOpenAIModelList...)
}

// GetAIPDDModelMeta returns the catalog metadata for a model id, if known.
func GetAIPDDModelMeta(modelName string) (AIPDDModelMeta, bool) {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	meta, ok := aipddModelMetaByID[strings.ToLower(strings.TrimSpace(modelName))]
	meta.Protocols = append([]string(nil), meta.Protocols...)
	return meta, ok
}

// SetAIPDDTokenPrices publishes the upstream token price table and the AWCoin
// exchange rate of the catalog they came from. An empty table clears the cost
// estimate basis instead of keeping prices of a superseded revision.
func SetAIPDDTokenPrices(prices []AIPDDModelTokenPrice, rmbPerAWCoin float64) {
	index := make(map[string]AIPDDModelTokenPrice, len(prices))
	for _, price := range prices {
		price.ModelName = strings.TrimSpace(price.ModelName)
		if price.ModelName == "" || IsAIPDDExcludedModel(price.ModelName) {
			continue
		}
		key := strings.ToLower(price.ModelName)
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = price
	}
	aipddCapabilitiesLock.Lock()
	defer aipddCapabilitiesLock.Unlock()
	aipddTokenPriceByModel = index
	if rmbPerAWCoin > 0 {
		aipddRMBPerAWCoin = rmbPerAWCoin
	}
}

func GetAIPDDTokenPrice(modelName string) (AIPDDModelTokenPrice, bool) {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	price, ok := aipddTokenPriceByModel[strings.ToLower(strings.TrimSpace(modelName))]
	return price, ok
}

// GetAIPDDRMBPerAWCoin returns the published AWCoin->RMB rate, or zero when no
// catalog has been applied yet.
func GetAIPDDRMBPerAWCoin() float64 {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return aipddRMBPerAWCoin
}

// ResetAIPDDTokenPrices clears the cost estimate basis, used when AIPDD is
// disabled or a catalog without token prices takes over.
func ResetAIPDDTokenPrices() {
	aipddCapabilitiesLock.Lock()
	defer aipddCapabilitiesLock.Unlock()
	aipddTokenPriceByModel = map[string]AIPDDModelTokenPrice{}
	aipddRMBPerAWCoin = 0
}

func GetAIPDDModelList() []string {
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	return mergeAIPDDModelLists(AIPDDTaskModelList, AIPDDOpenAIModelList)
}

func IsAIPDDTaskModel(modelName string) bool {
	_, ok := GetAIPDDCapability(modelName)
	return ok
}

func IsAIPDDOpenAIModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}
	aipddCapabilitiesLock.RLock()
	defer aipddCapabilitiesLock.RUnlock()
	for _, item := range AIPDDOpenAIModelList {
		if item == modelName {
			return true
		}
	}
	return false
}

func IsAIPDDPerCallBillingModel(modelName string) bool {
	capability, ok := GetAIPDDCapability(modelName)
	return ok && capability.BillingType == AIPDDBillingTypePerCall
}

func IsAIPDDExactBillingModel(modelName string) bool {
	return IsAIPDDTaskPricingModel(modelName)
}

// IsAIPDDDurationBillingModel identifies catalog capabilities whose upstream
// charge is proportional to generated seconds.
func IsAIPDDDurationBillingModel(modelName string) bool {
	capability, ok := GetAIPDDCapability(modelName)
	return ok && capability.BillingType == AIPDDBillingTypeDurationSeconds
}

// IsAIPDDSynchronousDurationModel identifies the duration-billed capabilities the
// gateway calls directly (ASR). Unlike task models, their duration is unknown
// before the request, so they settle on the ordinary price pipeline: the relay
// multiplies the per-second base price by the duration the upstream reports.
func IsAIPDDSynchronousDurationModel(modelName string) bool {
	capability, ok := GetAIPDDCapability(modelName)
	return ok && capability.Synchronous && capability.BillingType == AIPDDBillingTypeDurationSeconds
}

// IsAIPDDTaskPricingModel identifies task models that must use the structured
// task-pricing pipeline. This includes resolution-aware Seedance models and
// generic per-second workflows such as LTX.
func IsAIPDDTaskPricingModel(modelName string) bool {
	return IsAIPDDSeedanceModel(modelName) || IsAIPDDDurationBillingModel(modelName)
}

// IsAIPDDSeedanceModel identifies the Seedance execution family without
// relying solely on upstream price data. Catalog validation separately
// requires the current modality-price contract, and this identity check keeps
// Seedance from ever falling back to a legacy per-request retail ModelPrice.
func IsAIPDDSeedanceModel(modelName string) bool {
	capability, ok := GetAIPDDCapability(modelName)
	return ok && (strings.EqualFold(strings.TrimSpace(capability.AdapterCode), "seedance") ||
		strings.EqualFold(strings.TrimSpace(capability.ExecutionProtocol), "seedance_official") ||
		capability.SeedancePricing != nil)
}

func GetAIPDDEndpointTypes(modelName string) []EndpointType {
	if capability, ok := GetAIPDDCapability(modelName); ok {
		if len(capability.EndpointTypes) > 0 {
			return capability.EndpointTypes
		}
		if capability.EndpointType != "" {
			return []EndpointType{capability.EndpointType}
		}
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil
	}
	if meta, ok := GetAIPDDModelMeta(modelName); ok {
		if endpointTypes := AIPDDEndpointTypesForProtocols(meta.Protocols); len(endpointTypes) > 0 {
			return endpointTypes
		}
	}
	return []EndpointType{EndpointTypeOpenAI}
}

// AIPDDEndpointTypesForProtocols maps catalog protocol names onto the client
// endpoints they are reachable on. chat, responses and messages are all served
// by the OpenAI chat endpoint, so they report a single "openai" endpoint.
func AIPDDEndpointTypesForProtocols(protocols []string) []EndpointType {
	endpointTypes := make([]EndpointType, 0, len(protocols))
	seen := make(map[EndpointType]bool, len(protocols))
	for _, protocol := range protocols {
		var endpointType EndpointType
		switch strings.ToLower(strings.TrimSpace(protocol)) {
		case "embeddings":
			endpointType = EndpointTypeEmbeddings
		case "systemone":
			endpointType = EndpointTypeSystemOne
		case "chat", "responses", "messages":
			endpointType = EndpointTypeOpenAI
		default:
			continue
		}
		if seen[endpointType] {
			continue
		}
		seen[endpointType] = true
		endpointTypes = append(endpointTypes, endpointType)
	}
	return endpointTypes
}

func normalizeAIPDDStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeAIPDDModelList(models []string) []string {
	normalized := make([]string, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, modelName := range models {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" || IsAIPDDExcludedModel(modelName) || seen[modelName] {
			continue
		}
		normalized = append(normalized, modelName)
		seen[modelName] = true
	}
	return normalized
}

// IsAIPDDExcludedModel reports whether an AIPDD identifier belongs to a
// capability family intentionally hidden from new-api. FunASR is no longer
// excluded: it is the speech-to-text capability served on
// /v1/audio/transcriptions.
func IsAIPDDExcludedModel(modelName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(modelName))
	normalized = strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(normalized)
	return strings.Contains(normalized, "lightx2v") ||
		strings.Contains(normalized, "seedvr2")
}

// FilterAIPDDModelNames removes capabilities that new-api intentionally does
// not expose and also normalizes duplicate or blank model names.
func FilterAIPDDModelNames(models []string) []string {
	return normalizeAIPDDModelList(models)
}

func mergeAIPDDModelLists(lists ...[]string) []string {
	total := 0
	for _, list := range lists {
		total += len(list)
	}
	merged := make([]string, 0, total)
	seen := make(map[string]bool, total)
	for _, list := range lists {
		for _, modelName := range list {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" || seen[modelName] {
				continue
			}
			merged = append(merged, modelName)
			seen[modelName] = true
		}
	}
	return merged
}
