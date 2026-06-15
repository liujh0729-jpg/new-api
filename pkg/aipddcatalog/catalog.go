package aipddcatalog

import (
	"context"
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
	ID               string        `json:"id"`
	Code             string        `json:"code"`
	Name             string        `json:"name"`
	Description      string        `json:"description"`
	PriceAWCoin      float64       `json:"priceAWcoin"`
	EndpointType     string        `json:"endpointType"`
	TaskKind         string        `json:"taskKind"`
	InputModalities  []string      `json:"inputModalities"`
	OutputModalities []string      `json:"outputModalities"`
	Params           []ScriptParam `json:"params"`
}

type ScriptParam struct {
	ParamKey          string   `json:"paramKey"`
	ParamName         string   `json:"paramName"`
	ParamDesc         string   `json:"paramDesc"`
	DefaultValue      string   `json:"defaultValue"`
	DataType          string   `json:"dataType"`
	IsRequired        bool     `json:"isRequired"`
	OrderNo           int      `json:"orderNo"`
	MaxDuration       int      `json:"maxDuration"`
	MaxFileSize       int      `json:"maxFileSize"`
	UIType            string   `json:"uiType"`
	AcceptedMimeTypes []string `json:"acceptedMimeTypes"`
	Aliases           []string `json:"aliases"`
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

	scripts, err := fetchScripts(ctx, client, baseURL, apiKey)
	if err != nil {
		return Catalog{}, err
	}
	feeRules, err := fetchFeeRules(ctx, client, baseURL, apiKey)
	if err != nil {
		feeRules = nil
	}
	awcoinUSDRate, _ := fetchAWCoinUSDRate(ctx, client, baseURL, apiKey)
	return convertScriptsToCatalog(scripts, feeRules, awcoinUSDRate), nil
}

func (c Catalog) ModelNames() []string {
	models := make([]string, 0, len(c.Capabilities))
	seen := make(map[string]bool, len(c.Capabilities))
	for _, capability := range c.Capabilities {
		modelName := strings.TrimSpace(capability.ModelName)
		if modelName == "" || seen[modelName] {
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
		UploadTargets:          buildUploadTargets(params, base),
		EndpointType:           inferEndpointType(script, params, base, hasBase),
		BillingType:            inferBillingType(script, params, feeRule, hasFeeRule, base, hasBase),
	}
	return capability, rawPrice, true
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
	case strings.Contains(normalized, "duration") || strings.Contains(normalized, "seconds") || strings.Contains(normalized, "时长"):
		sources = append(sources, source(constant.AIPDDWorkflowSourceDuration), metadataSource("seconds"))
	case strings.Contains(normalized, "prompt") || strings.Contains(normalized, "text") || strings.Contains(normalized, "提示词"):
		sources = append(sources, metadataSource("prompt"), source(constant.AIPDDWorkflowSourcePrompt))
	case strings.Contains(normalized, "video") || strings.Contains(normalized, "load_video") || strings.Contains(normalized, "motion"):
		sources = append(sources, metadataSource("video"), metadataSource("input_reference"), source(constant.AIPDDWorkflowSourceInputReference), source(constant.AIPDDWorkflowSourceFirstImage), source(constant.AIPDDWorkflowSourceImage))
	case strings.Contains(normalized, "audio") || strings.Contains(normalized, "voice") || strings.Contains(normalized, "sound"):
		sources = append(sources, metadataSource("audio"), metadataSource("voice"), metadataSource("input_reference"))
	case strings.Contains(normalized, "image") || strings.Contains(normalized, "img") || strings.Contains(normalized, "图片"):
		sources = append(sources, metadataSource("image"), source(constant.AIPDDWorkflowSourceImage), source(constant.AIPDDWorkflowSourceFirstImage), source(constant.AIPDDWorkflowSourceInputReference))
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

func buildUploadTargets(params []ScriptParam, base constant.AIPDDCapability) []constant.AIPDDUploadTarget {
	targets := make([]constant.AIPDDUploadTarget, 0, len(base.UploadTargets)+len(params))
	seen := make(map[string]bool)
	for _, target := range base.UploadTargets {
		target.Aliases = append([]string(nil), target.Aliases...)
		targets = append(targets, target)
		seen[target.ParamKey] = true
	}

	for _, param := range params {
		if seen[param.ParamKey] {
			continue
		}
		target, ok := inferUploadTarget(param)
		if !ok {
			continue
		}
		targets = append(targets, target)
		seen[target.ParamKey] = true
	}
	return targets
}

func inferUploadTarget(param ScriptParam) (constant.AIPDDUploadTarget, bool) {
	switch strings.TrimSpace(param.UIType) {
	case "image_url":
		return constant.AIPDDUploadTarget{
			ParamKey: param.ParamKey,
			Aliases:  mergeAliases(param.Aliases, "file", "input_reference", "reference", "images", "image"),
		}, true
	case "video_url":
		return constant.AIPDDUploadTarget{
			ParamKey: param.ParamKey,
			Aliases:  mergeAliases(param.Aliases, "file", "input_reference", "reference", "video", "load_video"),
		}, true
	case "audio_url":
		return constant.AIPDDUploadTarget{
			ParamKey: param.ParamKey,
			Aliases:  mergeAliases(param.Aliases, "file", "audio", "input_audio", "voice", "ref_audio", "reference_audio"),
		}, true
	case "file_url":
		return constant.AIPDDUploadTarget{
			ParamKey: param.ParamKey,
			Aliases:  mergeAliases(param.Aliases, "file", "input_reference", "reference"),
		}, true
	}

	normalized := normalizedParamText(param)
	switch {
	case strings.Contains(normalized, "video") || strings.Contains(normalized, "load_video") || strings.Contains(normalized, "motion"):
		return constant.AIPDDUploadTarget{
			ParamKey: param.ParamKey,
			Aliases:  []string{"file", "input_reference", "reference", "video", "load_video"},
		}, true
	case strings.Contains(normalized, "audio") || strings.Contains(normalized, "voice") || strings.Contains(normalized, "sound"):
		return constant.AIPDDUploadTarget{
			ParamKey: param.ParamKey,
			Aliases:  []string{"file", "audio", "input_audio", "voice", "ref_audio", "reference_audio"},
		}, true
	case strings.Contains(normalized, "image") || strings.Contains(normalized, "img") || strings.Contains(normalized, "file") || strings.Contains(normalized, "图片"):
		return constant.AIPDDUploadTarget{
			ParamKey: param.ParamKey,
			Aliases:  []string{"file", "input_reference", "reference", "images", "image"},
		}, true
	default:
		return constant.AIPDDUploadTarget{}, false
	}
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

func mergeAliases(values []string, defaults ...string) []string {
	return normalizeStringList(append(append([]string(nil), values...), defaults...))
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
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func normalizedParamText(param ScriptParam) string {
	return strings.ToLower(strings.Join([]string{param.ParamKey, param.ParamName, param.ParamDesc, param.DataType}, " "))
}

func isIntegerParam(param ScriptParam) bool {
	dataType := strings.ToLower(strings.TrimSpace(param.DataType))
	return dataType == "int" || dataType == "integer"
}

func metadataSource(key string) constant.AIPDDWorkflowValueSource {
	return constant.AIPDDWorkflowValueSource{Type: constant.AIPDDWorkflowSourceMetadata, Key: key}
}

func source(sourceType constant.AIPDDWorkflowSourceType) constant.AIPDDWorkflowValueSource {
	return constant.AIPDDWorkflowValueSource{Type: sourceType}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
