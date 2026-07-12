package aipddcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

const (
	defaultTimeoutSeconds = 10
	feeRulePageSize       = 100

	envUSDPerCoin  = "AIPDD_UPSTREAM_PRICE_USD_PER_COIN"
	envCoinsPerRMB = "AIPDD_UPSTREAM_PRICE_COINS_PER_RMB"
	envUSD2RMB     = "AIPDD_UPSTREAM_PRICE_USD2RMB"
)

type Catalog struct {
	Capabilities  []constant.AIPDDCapability
	ModelPrices   map[string]float64
	AWCoinUSDRate float64
}

type Script struct {
	ID               string       `json:"id"`
	Code             string       `json:"code"`
	Name             string       `json:"name"`
	Description      string       `json:"description"`
	PriceAWCoin      float64      `json:"priceAWcoin"`
	AdapterCode      string       `json:"adapterCode"`
	EndpointType     string       `json:"endpointType"`
	TaskKind         string       `json:"taskKind"`
	InputModalities  []string     `json:"inputModalities"`
	OutputModalities []string     `json:"outputModalities"`
	Params           ScriptParams `json:"params"`
}

// ScriptParams accepts both the current array representation and the object
// representation returned by older or parameterless AIPDD capabilities.
type ScriptParams []ScriptParam

func (p *ScriptParams) UnmarshalJSON(data []byte) error {
	switch common.GetJsonType(json.RawMessage(data)) {
	case "null":
		*p = nil
		return nil
	case "array":
		var params []ScriptParam
		if err := common.Unmarshal(data, &params); err != nil {
			return err
		}
		*p = params
		return nil
	case "object":
		var paramsByKey map[string]ScriptParam
		if err := common.Unmarshal(data, &paramsByKey); err != nil {
			return err
		}

		keys := make([]string, 0, len(paramsByKey))
		for key := range paramsByKey {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		params := make([]ScriptParam, 0, len(keys))
		for _, key := range keys {
			param := paramsByKey[key]
			if strings.TrimSpace(param.ParamKey) == "" {
				param.ParamKey = key
			}
			params = append(params, param)
		}
		*p = params
		return nil
	default:
		return fmt.Errorf("AIPDD script params must be an array, object, or null")
	}
}

type ScriptParam struct {
	ParamKey          string          `json:"paramKey"`
	ParamName         string          `json:"paramName"`
	ParamDesc         string          `json:"paramDesc"`
	DefaultValue      json.RawMessage `json:"defaultValue"`
	DataType          string          `json:"dataType"`
	IsRequired        bool            `json:"isRequired"`
	OrderNo           int             `json:"orderNo"`
	MaxDuration       int             `json:"maxDuration"`
	MaxFileSize       int             `json:"maxFileSize"`
	UIType            string          `json:"uiType"`
	AcceptedMimeTypes []string        `json:"acceptedMimeTypes"`
	Aliases           []string        `json:"aliases"`
	Min               *float64        `json:"min"`
	Max               *float64        `json:"max"`
	Allowed           []any           `json:"allowed"`
}

type FeeRule struct {
	Key   string  `json:"key"`
	Name  string  `json:"name"`
	Type  string  `json:"type"`
	Price float64 `json:"price"`
	Unit  string  `json:"unit"`
}

type scriptsResponse struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    []Script `json:"data"`
}

type capabilitiesResponse struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    []Script `json:"data"`
}

type feeRulesResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Total    int       `json:"total"`
		List     []FeeRule `json:"list"`
		Page     int       `json:"page"`
		PageSize int       `json:"pageSize"`
	} `json:"data"`
}

type awcoinRateResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		RMB float64  `json:"rmb"`
		USD *float64 `json:"usd"`
	} `json:"data"`
}

type openAIModelsResponse struct {
	Data []openAIModel `json:"data"`
}

type openAIModel struct {
	ID string `json:"id"`
}

func FetchWithTimeout(baseURL, apiKey string, timeout time.Duration) (Catalog, error) {
	if timeout <= 0 {
		timeout = time.Duration(defaultTimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return Fetch(ctx, http.DefaultClient, baseURL, apiKey)
}

func Fetch(ctx context.Context, client *http.Client, baseURL, apiKey string) (Catalog, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = http.DefaultClient
	}
	baseURL = normalizeBaseURL(baseURL)

	var capabilitiesErr error
	if catalog, err := fetchCapabilitiesCatalog(ctx, client, baseURL, apiKey); err == nil {
		if catalogNeedsWorkflowEnrichment(catalog) {
			if scripts, legacyErr := fetchScripts(ctx, client, baseURL, apiKey); legacyErr == nil {
				catalog = enrichCatalogWorkflowParams(catalog, scripts)
			}
		}
		return catalog, nil
	} else {
		capabilitiesErr = err
	}

	scripts, err := fetchScripts(ctx, client, baseURL, apiKey)
	if err != nil {
		return Catalog{}, fmt.Errorf("fetch AIPDD capabilities failed: %v; fetch legacy AIPDD scripts failed: %w", capabilitiesErr, err)
	}
	feeRules, err := fetchFeeRules(ctx, client, baseURL, apiKey)
	if err != nil {
		feeRules = nil
	}
	awcoinUSDRate, _ := fetchAWCoinUSDRate(ctx, client, baseURL, apiKey)
	return convertScriptsToCatalog(scripts, feeRules, awcoinUSDRate), nil
}

// catalogNeedsWorkflowEnrichment reports whether the unified capabilities
// response omitted or incompletely described workflow parameters for at least
// one task capability. LLM capabilities are already filtered from Catalog.
func catalogNeedsWorkflowEnrichment(catalog Catalog) bool {
	for _, capability := range catalog.Capabilities {
		if capabilityNeedsWorkflowEnrichment(capability) {
			return true
		}
	}
	return false
}

func capabilityNeedsWorkflowEnrichment(capability constant.AIPDDCapability) bool {
	return len(capability.WorkflowParamKeys) == 0 || ltxWorkflowParamsIncomplete(capability)
}

func ltxWorkflowParamsIncomplete(capability constant.AIPDDCapability) bool {
	values := []string{capability.ModelName, capability.ScriptCode}
	for _, value := range values {
		normalized := strings.NewReplacer("_", "-", ".", "-", " ", "-").Replace(strings.ToLower(strings.TrimSpace(value)))
		if strings.Contains(value, "首尾帧") ||
			(strings.Contains(normalized, "ltx") && strings.Contains(normalized, "first") && strings.Contains(normalized, "last")) {
			return !workflowParamsContainAll(capability.WorkflowParamKeys,
				"first_frame_image", "last_frame_image", "audio", "local_prompts", "timeline_data", "length", "global_prompt")
		}
		if normalized == "aipdd-ltx-2-3" {
			return !workflowParamsContainAll(capability.WorkflowParamKeys,
				"prompt", "image", "negativePrompt", "width", "height", "numFrames", "frameRate", "seed")
		}
	}
	return false
}

func workflowParamsContainAll(actual []string, expected ...string) bool {
	keys := make(map[string]bool, len(actual))
	for _, key := range actual {
		keys[key] = true
	}
	for _, key := range expected {
		if !keys[key] {
			return false
		}
	}
	return true
}

// enrichCatalogWorkflowParams supplements the unified catalog with workflow
// details from the legacy catalog. Unified metadata remains authoritative;
// legacy data only fills missing parameter definitions and constraints.
func enrichCatalogWorkflowParams(catalog Catalog, scripts []Script) Catalog {
	legacyByCode := make(map[string]constant.AIPDDCapability, len(scripts))
	for _, script := range scripts {
		if isFunASRScript(script) {
			continue
		}
		capability, _, ok := buildCapability(script, nil)
		if !ok {
			continue
		}
		legacyByCode[strings.ToLower(strings.TrimSpace(script.Code))] = capability
	}

	for i := range catalog.Capabilities {
		capability := &catalog.Capabilities[i]
		if !capabilityNeedsWorkflowEnrichment(*capability) {
			continue
		}
		legacy, ok := legacyCapabilityFor(*capability, legacyByCode)
		if !ok {
			continue
		}
		mergeWorkflowDetails(capability, legacy)
	}
	return catalog
}

func legacyCapabilityFor(capability constant.AIPDDCapability, legacyByCode map[string]constant.AIPDDCapability) (constant.AIPDDCapability, bool) {
	for _, value := range []string{capability.ScriptCode, capability.ModelName} {
		if legacy, ok := legacyByCode[strings.ToLower(strings.TrimSpace(value))]; ok {
			return legacy, true
		}
	}
	return constant.AIPDDCapability{}, false
}

func mergeWorkflowDetails(target *constant.AIPDDCapability, legacy constant.AIPDDCapability) {
	if target == nil {
		return
	}

	keys := append([]string(nil), target.WorkflowParamKeys...)
	seenKeys := make(map[string]bool, len(keys))
	for _, key := range keys {
		seenKeys[key] = true
	}
	for _, key := range legacy.WorkflowParamKeys {
		if !seenKeys[key] {
			keys = append(keys, key)
			seenKeys[key] = true
		}
	}
	if len(keys) > 0 {
		target.WorkflowParamKeys = keys
	}

	if target.RequiredWorkflowParams == nil {
		target.RequiredWorkflowParams = make(map[string]bool)
	}
	for key, required := range legacy.RequiredWorkflowParams {
		if _, exists := target.RequiredWorkflowParams[key]; !exists {
			target.RequiredWorkflowParams[key] = required
		}
	}

	target.WorkflowDefaults = mergeWorkflowDefaults(target.WorkflowDefaults, legacy.WorkflowDefaults)
	target.WorkflowConstraints = mergeWorkflowConstraints(target.WorkflowConstraints, legacy.WorkflowConstraints)
}

func mergeWorkflowDefaults(primary, supplement []constant.AIPDDWorkflowParamDefault) []constant.AIPDDWorkflowParamDefault {
	merged := append([]constant.AIPDDWorkflowParamDefault(nil), primary...)
	seen := make(map[string]bool, len(merged))
	for _, item := range merged {
		seen[item.ParamKey] = true
	}
	for _, item := range supplement {
		if !seen[item.ParamKey] {
			merged = append(merged, item)
			seen[item.ParamKey] = true
		}
	}
	return merged
}

func mergeWorkflowConstraints(primary, supplement []constant.AIPDDWorkflowParamConstraint) []constant.AIPDDWorkflowParamConstraint {
	merged := append([]constant.AIPDDWorkflowParamConstraint(nil), primary...)
	seen := make(map[string]bool, len(merged))
	for _, item := range merged {
		seen[item.ParamKey] = true
	}
	for _, item := range supplement {
		if !seen[item.ParamKey] {
			merged = append(merged, item)
			seen[item.ParamKey] = true
		}
	}
	return merged
}

func FetchOpenAIModels(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if client == nil {
		client = http.DefaultClient
	}
	baseURL = normalizeBaseURL(baseURL)

	var response openAIModelsResponse
	if err := getJSON(ctx, client, baseURL, "/v1/models", nil, apiKey, &response); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(response.Data))
	for _, item := range response.Data {
		models = append(models, item.ID)
	}
	return constant.FilterAIPDDModelNames(models), nil
}

func (c Catalog) ModelNames() []string {
	models := make([]string, 0, len(c.Capabilities))
	seen := make(map[string]bool, len(c.Capabilities))
	for _, capability := range c.Capabilities {
		modelName := strings.TrimSpace(capability.ModelName)
		if modelName == "" || constant.IsAIPDDFunASRModel(modelName) || seen[modelName] {
			continue
		}
		models = append(models, modelName)
		seen[modelName] = true
	}
	return models
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeAIPDD]
	}
	return baseURL
}

func fetchScripts(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]Script, error) {
	var response scriptsResponse
	if err := getJSON(ctx, client, baseURL, "/scripts/admin/comfyui_workflow", nil, apiKey, &response); err != nil {
		return nil, err
	}
	if err := validateAIPDDResponse(response.Code, response.Message, "fetch AIPDD scripts"); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func fetchCapabilities(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]Script, error) {
	var response capabilitiesResponse
	if err := getJSON(ctx, client, baseURL, "/v1/capabilities", nil, apiKey, &response); err != nil {
		return nil, err
	}
	if err := validateAIPDDResponse(response.Code, response.Message, "fetch AIPDD capabilities"); err != nil {
		return nil, err
	}
	return response.Data, nil
}

func fetchCapabilitiesCatalog(ctx context.Context, client *http.Client, baseURL, apiKey string) (Catalog, error) {
	capabilities, err := fetchCapabilities(ctx, client, baseURL, apiKey)
	if err != nil {
		return Catalog{}, err
	}
	awcoinUSDRate, _ := fetchAWCoinUSDRate(ctx, client, baseURL, apiKey)
	return convertCapabilitiesToCatalog(capabilities, awcoinUSDRate), nil
}

func fetchFeeRules(ctx context.Context, client *http.Client, baseURL, apiKey string) ([]FeeRule, error) {
	var all []FeeRule
	for page := 1; ; page++ {
		query := url.Values{}
		query.Set("page", strconv.Itoa(page))
		query.Set("pageSize", strconv.Itoa(feeRulePageSize))

		var response feeRulesResponse
		if err := getJSON(ctx, client, baseURL, "/fee-rules", query, apiKey, &response); err != nil {
			return nil, err
		}
		if err := validateAIPDDResponse(response.Code, response.Message, "fetch AIPDD fee rules"); err != nil {
			return nil, err
		}

		all = append(all, response.Data.List...)
		if response.Data.Total <= len(all) || len(response.Data.List) < feeRulePageSize {
			break
		}
	}
	return all, nil
}

func fetchAWCoinUSDRate(ctx context.Context, client *http.Client, baseURL, apiKey string) (float64, bool) {
	var response awcoinRateResponse
	if err := getJSON(ctx, client, baseURL, "/system/awcoin-rate", nil, apiKey, &response); err != nil {
		return 0, false
	}
	if err := validateAIPDDResponse(response.Code, response.Message, "fetch AIPDD AWcoin rate"); err != nil {
		return 0, false
	}
	if response.Data.USD != nil && *response.Data.USD > 0 {
		return *response.Data.USD, true
	}
	if response.Data.RMB > 0 {
		return response.Data.RMB / ratio_setting.USD2RMB, true
	}
	return 0, false
}

func getJSON(ctx context.Context, client *http.Client, baseURL, path string, query url.Values, apiKey string, out any) error {
	uri, err := buildURL(baseURL, path, query)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if apiKey = strings.TrimSpace(apiKey); apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("AIPDD catalog request failed: %s %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return common.DecodeJson(resp.Body, out)
}

func buildURL(baseURL, path string, query url.Values) (string, error) {
	parsed, err := url.Parse(normalizeBaseURL(baseURL))
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	endpointPath := "/" + strings.TrimLeft(path, "/")
	if basePath == "" || basePath == "/" {
		parsed.Path = endpointPath
	} else {
		parsed.Path = basePath + endpointPath
	}
	if len(query) > 0 {
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), nil
}

func validateAIPDDResponse(code int, message string, operation string) error {
	if code == 0 || code == http.StatusOK {
		return nil
	}
	if strings.TrimSpace(message) == "" {
		message = "unknown error"
	}
	return fmt.Errorf("%s failed: %s", operation, message)
}

func convertScriptsToCatalog(scripts []Script, feeRules []FeeRule, awcoinUSDRate float64) Catalog {
	feeRuleByKey := buildFeeRuleMap(feeRules)
	capabilities := make([]constant.AIPDDCapability, 0, len(scripts))
	modelPrices := make(map[string]float64)

	for _, script := range scripts {
		if isFunASRScript(script) {
			continue
		}
		capability, rawPrice, ok := buildCapability(script, feeRuleByKey)
		if !ok {
			continue
		}
		capabilities = append(capabilities, capability)
		modelPrices[capability.ModelName] = ConvertUpstreamPriceToModelPriceWithRate(rawPrice, awcoinUSDRate)
	}

	sortCapabilities(capabilities)
	return Catalog{Capabilities: capabilities, ModelPrices: modelPrices, AWCoinUSDRate: awcoinUSDRate}
}

func isFunASRScript(script Script) bool {
	for _, value := range []string{
		script.ID,
		script.Code,
		script.Name,
		script.Description,
		script.AdapterCode,
		script.EndpointType,
		script.TaskKind,
	} {
		if constant.IsAIPDDFunASRModel(value) {
			return true
		}
	}
	for _, param := range script.Params {
		for _, value := range []string{param.ParamKey, param.ParamName, param.ParamDesc} {
			if constant.IsAIPDDFunASRModel(value) {
				return true
			}
		}
	}
	return false
}

func convertCapabilitiesToCatalog(scripts []Script, awcoinUSDRate float64) Catalog {
	taskScripts := make([]Script, 0, len(scripts))
	for _, script := range scripts {
		if isLLMCapability(script) {
			continue
		}
		taskScripts = append(taskScripts, script)
	}
	return convertScriptsToCatalog(taskScripts, nil, awcoinUSDRate)
}

func isLLMCapability(script Script) bool {
	adapterCode := strings.ToLower(strings.TrimSpace(script.AdapterCode))
	endpointType := strings.ToLower(strings.TrimSpace(script.EndpointType))
	return adapterCode == "llm" || endpointType == "llm-chat"
}

func buildFeeRuleMap(feeRules []FeeRule) map[string]FeeRule {
	out := make(map[string]FeeRule, len(feeRules)*2)
	for _, feeRule := range feeRules {
		key := strings.TrimSpace(feeRule.Key)
		if key == "" {
			continue
		}
		out[key] = feeRule
		out[strings.ToLower(key)] = feeRule
	}
	return out
}

func buildCapability(script Script, feeRuleByKey map[string]FeeRule) (constant.AIPDDCapability, float64, bool) {
	base, hasBase := defaultCapabilityForScript(script)
	modelName := strings.TrimSpace(base.ModelName)
	if modelName == "" {
		modelName = firstNonEmpty(script.Code, script.Name, script.ID)
	}
	if modelName == "" {
		return constant.AIPDDCapability{}, 0, false
	}

	feeRule, hasFeeRule := lookupFeeRule(script.Code, feeRuleByKey)
	rawPrice := script.PriceAWCoin
	if hasFeeRule {
		rawPrice = feeRule.Price
	}

	params := normalizeParams(script.Params)
	paramKeys, requiredParams := buildParamMaps(params, base)
	capability := constant.AIPDDCapability{
		ModelName:              modelName,
		ScriptID:               firstNonEmpty(script.ID, base.ScriptID),
		ScriptCode:             firstNonEmpty(script.Code, base.ScriptCode, modelName),
		TaskKind:               strings.TrimSpace(script.TaskKind),
		InputModalities:        normalizeStringList(script.InputModalities),
		OutputModalities:       normalizeStringList(script.OutputModalities),
		TaskCost:               rawPrice,
		WorkflowParamKeys:      paramKeys,
		RequiredWorkflowParams: requiredParams,
		WorkflowDefaults:       buildWorkflowDefaults(params, base),
		WorkflowConstraints:    buildWorkflowConstraints(params, base),
		EndpointType:           inferEndpointType(script, params, base, hasBase),
		BillingType:            inferBillingType(script, params, feeRule, hasFeeRule, base, hasBase),
	}
	return capability, rawPrice, true
}

func buildWorkflowConstraints(params []ScriptParam, base constant.AIPDDCapability) []constant.AIPDDWorkflowParamConstraint {
	if len(params) == 0 {
		return append([]constant.AIPDDWorkflowParamConstraint(nil), base.WorkflowConstraints...)
	}
	constraints := make([]constant.AIPDDWorkflowParamConstraint, 0, len(params))
	for _, param := range params {
		if param.Min == nil && param.Max == nil && len(param.Allowed) == 0 {
			continue
		}
		constraints = append(constraints, constant.AIPDDWorkflowParamConstraint{
			ParamKey: param.ParamKey,
			DataType: param.DataType,
			Min:      param.Min,
			Max:      param.Max,
			Allowed:  append([]any(nil), param.Allowed...),
		})
	}
	return constraints
}

func defaultCapabilityForScript(script Script) (constant.AIPDDCapability, bool) {
	for _, alias := range []string{script.Code, script.Name, script.ID} {
		if capability, ok := constant.GetDefaultAIPDDCapability(alias); ok {
			return capability, true
		}
	}
	return constant.AIPDDCapability{}, false
}

func lookupFeeRule(scriptCode string, feeRuleByKey map[string]FeeRule) (FeeRule, bool) {
	scriptCode = strings.TrimSpace(scriptCode)
	if scriptCode == "" {
		return FeeRule{}, false
	}
	if feeRule, ok := feeRuleByKey[scriptCode]; ok {
		return feeRule, true
	}
	feeRule, ok := feeRuleByKey[strings.ToLower(scriptCode)]
	return feeRule, ok
}

func normalizeParams(params []ScriptParam) []ScriptParam {
	out := make([]ScriptParam, 0, len(params))
	seen := make(map[string]bool, len(params))
	for _, param := range params {
		param.ParamKey = strings.TrimSpace(param.ParamKey)
		if param.ParamKey == "" || seen[param.ParamKey] {
			continue
		}
		seen[param.ParamKey] = true
		out = append(out, param)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].OrderNo < out[j].OrderNo
	})
	return out
}

func buildParamMaps(params []ScriptParam, base constant.AIPDDCapability) ([]string, map[string]bool) {
	if len(params) == 0 {
		return append([]string(nil), base.WorkflowParamKeys...), cloneBoolMap(base.RequiredWorkflowParams)
	}

	keys := make([]string, 0, len(params))
	required := make(map[string]bool, len(params))
	for _, param := range params {
		keys = append(keys, param.ParamKey)
		required[param.ParamKey] = param.IsRequired
	}
	return keys, required
}

func cloneBoolMap(values map[string]bool) map[string]bool {
	out := make(map[string]bool, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func buildWorkflowDefaults(params []ScriptParam, base constant.AIPDDCapability) []constant.AIPDDWorkflowParamDefault {
	if len(params) == 0 {
		return append([]constant.AIPDDWorkflowParamDefault(nil), base.WorkflowDefaults...)
	}

	paramSet := make(map[string]bool, len(params))
	for _, param := range params {
		paramSet[param.ParamKey] = true
	}

	defaults := make([]constant.AIPDDWorkflowParamDefault, 0, len(params))
	covered := make(map[string]bool, len(params))
	for _, item := range base.WorkflowDefaults {
		if !paramSet[item.ParamKey] {
			continue
		}
		item.Sources = append([]constant.AIPDDWorkflowValueSource(nil), item.Sources...)
		defaults = append(defaults, item)
		covered[item.ParamKey] = true
	}

	for _, param := range params {
		if covered[param.ParamKey] {
			continue
		}
		defaults = append(defaults, inferWorkflowDefault(param))
	}
	return defaults
}

func inferWorkflowDefault(param ScriptParam) constant.AIPDDWorkflowParamDefault {
	sources := []constant.AIPDDWorkflowValueSource{metadataSource(param.ParamKey)}
	normalized := normalizedParamText(param)
	switch {
	case isNegativePromptParam(normalized):
		sources = append(sources, metadataSource("negativePrompt"), metadataSource("negative_prompt"), metadataSource("negative"))
	case isLastFrameParam(normalized):
		sources = append(sources,
			metadataSource("last_frame_image"),
			metadataSource("last_frame"),
			metadataSource("image_tail"),
			source(constant.AIPDDWorkflowSourceLastImage))
	case isFirstFrameParam(normalized):
		sources = append(sources,
			metadataSource("first_frame_image"),
			metadataSource("first_frame"),
			source(constant.AIPDDWorkflowSourceFirstImage),
			metadataSource("image"),
			source(constant.AIPDDWorkflowSourceImage))
	case strings.Contains(normalized, "duration") || strings.Contains(normalized, "seconds") || strings.Contains(normalized, "时长"):
		sources = append(sources, metadataSource("durationSeconds"), metadataSource("duration_seconds"), source(constant.AIPDDWorkflowSourceDuration), metadataSource("seconds"))
	case isFrameRateParam(normalized):
		sources = append(sources, metadataSource("frameRate"), metadataSource("fps"))
	case isFrameCountParam(normalized):
		sources = append(sources,
			metadataSource("length"),
			metadataSource("numFrames"),
			metadataSource("frames"),
			metadataSource("frame_count"),
			source(constant.AIPDDWorkflowSourceDuration),
			metadataSource("duration"),
			metadataSource("durationSeconds"),
		)
	case isImageCountParam(normalized):
		sources = append(sources, metadataSource("n"), metadataSource("image_count"), metadataSource("count"), metadataSource("batch_size"), metadataSource("num_outputs"))
	case strings.Contains(normalized, "prompt") || strings.Contains(normalized, "text") || strings.Contains(normalized, "提示词"):
		sources = append(sources, metadataSource("prompt"), source(constant.AIPDDWorkflowSourcePrompt))
	case strings.Contains(normalized, "video") || strings.Contains(normalized, "load_video") || strings.Contains(normalized, "motion"):
		sources = append(sources, metadataSource("video"), metadataSource("input_reference"), source(constant.AIPDDWorkflowSourceInputReference), source(constant.AIPDDWorkflowSourceFirstImage), source(constant.AIPDDWorkflowSourceImage))
	case strings.Contains(normalized, "audio") || strings.Contains(normalized, "voice") || strings.Contains(normalized, "sound"):
		sources = append(sources,
			metadataSource("audio"),
			metadataSource("audio_url"),
			metadataSource("ref_audio"),
			metadataSource("reference_audio"),
			metadataSource("voice"),
			metadataSource("input_reference"),
		)
	case strings.Contains(normalized, "image") || strings.Contains(normalized, "img") || strings.Contains(normalized, "图片"):
		sources = append(sources, metadataSource("image"), source(constant.AIPDDWorkflowSourceImage), source(constant.AIPDDWorkflowSourceFirstImage), source(constant.AIPDDWorkflowSourceInputReference))
	}
	if value := defaultValueString(param); value != "" {
		sources = append(sources, staticSource(value))
	}

	valueType := constant.AIPDDWorkflowValueTypeString
	if isIntegerParam(param) {
		valueType = constant.AIPDDWorkflowValueTypeInt
	}
	return constant.AIPDDWorkflowParamDefault{
		ParamKey:  param.ParamKey,
		ValueType: valueType,
		Sources:   sources,
	}
}

func isNegativePromptParam(value string) bool {
	return strings.Contains(value, "negativeprompt") ||
		strings.Contains(value, "negative_prompt") ||
		strings.Contains(value, "negative prompt") ||
		strings.Contains(value, "negative") ||
		strings.Contains(value, "负向") ||
		strings.Contains(value, "反向")
}

func isFirstFrameParam(value string) bool {
	return (strings.Contains(value, "first") && strings.Contains(value, "frame")) ||
		strings.Contains(value, "首帧") || strings.Contains(value, "起始帧")
}

func isLastFrameParam(value string) bool {
	return (strings.Contains(value, "last") && strings.Contains(value, "frame")) ||
		strings.Contains(value, "image_tail") || strings.Contains(value, "尾帧") || strings.Contains(value, "结束帧")
}

func isFrameRateParam(value string) bool {
	return strings.Contains(value, "framerate") ||
		strings.Contains(value, "frame_rate") ||
		strings.Contains(value, "frame rate") ||
		strings.Contains(value, "fps") ||
		strings.Contains(value, "帧率")
}

func isFrameCountParam(value string) bool {
	return strings.Contains(value, "numframes") ||
		strings.Contains(value, "num_frames") ||
		strings.Contains(value, "framecount") ||
		strings.Contains(value, "frame_count") ||
		strings.Contains(value, "frames") ||
		strings.Contains(value, "length") ||
		strings.Contains(value, "帧数")
}

func isImageCountParam(value string) bool {
	return strings.Contains(value, "numoutputs") ||
		strings.Contains(value, "num_outputs") ||
		strings.Contains(value, "num outputs") ||
		strings.Contains(value, "outputcount") ||
		strings.Contains(value, "output_count") ||
		strings.Contains(value, "output count") ||
		strings.Contains(value, "batchsize") ||
		strings.Contains(value, "batch_size") ||
		strings.Contains(value, "batch size") ||
		strings.Contains(value, "imagecount") ||
		strings.Contains(value, "image_count") ||
		strings.Contains(value, "image count") ||
		strings.Contains(value, "numberofimages") ||
		strings.Contains(value, "number of images") ||
		strings.Contains(value, "生成数量") ||
		strings.Contains(value, "图片数量") ||
		strings.Contains(value, "出图数量")
}

func inferEndpointType(script Script, params []ScriptParam, base constant.AIPDDCapability, hasBase bool) constant.EndpointType {
	if endpointType, ok := parseEndpointType(script.EndpointType); ok {
		return endpointType
	}
	if hasBase && base.EndpointType != "" {
		return base.EndpointType
	}
	text := normalizedScriptText(script, params)
	if strings.Contains(text, "video") || strings.Contains(text, "motion") || strings.Contains(text, "wan") || strings.Contains(text, "latent") || strings.Contains(text, "animater") || strings.Contains(text, "load_video") {
		return constant.EndpointTypeOpenAIVideo
	}
	if strings.Contains(text, "tts") || strings.Contains(text, "audio") || strings.Contains(text, "voice") || strings.Contains(text, "speech") || strings.Contains(text, "语音") {
		return constant.EndpointTypeAudioSpeech
	}
	return constant.EndpointTypeImageGeneration
}

func parseEndpointType(value string) (constant.EndpointType, bool) {
	switch constant.EndpointType(strings.TrimSpace(value)) {
	case constant.EndpointTypeImageGeneration:
		return constant.EndpointTypeImageGeneration, true
	case constant.EndpointTypeOpenAIVideo:
		return constant.EndpointTypeOpenAIVideo, true
	case constant.EndpointTypeAudioSpeech:
		return constant.EndpointTypeAudioSpeech, true
	default:
		return "", false
	}
}

func inferBillingType(script Script, params []ScriptParam, feeRule FeeRule, hasFeeRule bool, base constant.AIPDDCapability, hasBase bool) constant.AIPDDBillingType {
	if hasBase && base.BillingType != "" {
		return base.BillingType
	}
	if hasFeeRule {
		text := strings.ToLower(feeRule.Type + " " + feeRule.Unit)
		if strings.Contains(text, "second") || strings.Contains(text, "seconds") || strings.Contains(text, "sec") || strings.Contains(text, "秒") {
			return constant.AIPDDBillingTypeDurationSeconds
		}
	}
	return constant.AIPDDBillingTypePerCall
}

func sortCapabilities(capabilities []constant.AIPDDCapability) {
	staticModels := []string{
		constant.AIPDDModelFluxGGUF,
		constant.AIPDDModelFluxGGUFT2I,
		constant.AIPDDModelWan22Wanx,
		constant.AIPDDModelWan22Animater,
		constant.AIPDDModelMimicMotion,
		constant.AIPDDModelLatentsync15,
		constant.AIPDDModelIndexTTS,
	}
	order := make(map[string]int, len(staticModels))
	for idx, modelName := range staticModels {
		order[modelName] = idx
	}
	sort.SliceStable(capabilities, func(i, j int) bool {
		leftOrder, leftKnown := order[capabilities[i].ModelName]
		rightOrder, rightKnown := order[capabilities[j].ModelName]
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown {
			return leftOrder < rightOrder
		}
		return capabilities[i].ModelName < capabilities[j].ModelName
	})
}

func ConvertUpstreamPriceToModelPrice(rawPrice float64) float64 {
	return ConvertUpstreamPriceToModelPriceWithRate(rawPrice, 0)
}

func ConvertUpstreamPriceToModelPriceWithRate(rawPrice float64, awcoinUSDRate float64) float64 {
	if rawPrice <= 0 {
		return 0
	}
	if awcoinUSDRate > 0 {
		return roundPrice(rawPrice * awcoinUSDRate)
	}
	if directUSDPerCoin, ok := envFloat(envUSDPerCoin); ok && directUSDPerCoin >= 0 {
		return roundPrice(rawPrice * directUSDPerCoin)
	}

	coinsPerRMB := 100.0
	if configured, ok := envFloat(envCoinsPerRMB); ok && configured > 0 {
		coinsPerRMB = configured
	}
	usd2RMB := ratio_setting.USD2RMB
	if configured, ok := envFloat(envUSD2RMB); ok && configured > 0 {
		usd2RMB = configured
	}
	return roundPrice(rawPrice / coinsPerRMB / usd2RMB)
}

func envFloat(name string) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil
}

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		out = append(out, value)
		seen[value] = true
	}
	return out
}

func roundPrice(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*1e6) / 1e6
}

func normalizedScriptText(script Script, params []ScriptParam) string {
	parts := []string{script.Code, script.Name, script.Description}
	for _, param := range params {
		parts = append(parts, param.ParamKey, param.ParamName, param.ParamDesc, param.DataType)
		parts = append(parts, param.Aliases...)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func normalizedParamText(param ScriptParam) string {
	parts := []string{param.ParamKey, param.ParamName, param.ParamDesc, param.DataType}
	parts = append(parts, param.Aliases...)
	return strings.ToLower(strings.Join(parts, " "))
}

func isIntegerParam(param ScriptParam) bool {
	dataType := strings.ToLower(strings.TrimSpace(param.DataType))
	return dataType == "int" || dataType == "integer" ||
		(dataType == "number" && isFrameCountParam(normalizedParamText(param)))
}

func metadataSource(key string) constant.AIPDDWorkflowValueSource {
	return constant.AIPDDWorkflowValueSource{Type: constant.AIPDDWorkflowSourceMetadata, Key: key}
}

func staticSource(value string) constant.AIPDDWorkflowValueSource {
	return constant.AIPDDWorkflowValueSource{Type: constant.AIPDDWorkflowSourceStatic, Key: value}
}

func source(sourceType constant.AIPDDWorkflowSourceType) constant.AIPDDWorkflowValueSource {
	return constant.AIPDDWorkflowValueSource{Type: sourceType}
}

func defaultValueString(param ScriptParam) string {
	value := strings.TrimSpace(common.JsonRawMessageToString(param.DefaultValue))
	if value == "" || strings.EqualFold(value, "null") {
		return ""
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
