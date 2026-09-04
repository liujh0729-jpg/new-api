package billing_setting

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	taskPricingCSVFlatResolution = "-"
	taskPricingCSVBillingUnit    = "秒"
	taskPricingCSVMaxBytes       = 8 << 20
)

var taskPricingCSVRequiredColumns = []string{
	"平台模型",
	"输出规格",
	"能力类型",
	"计费单位",
	"对比原生价",
}

var taskPricingCSVUpdateOrder = []string{
	"billing_setting.task_pricing",
	"billing_setting.billing_mode",
}

// TaskPricingCSVError is a user-facing CSV import/export failure.
type TaskPricingCSVError struct {
	Message string
}

func (e *TaskPricingCSVError) Error() string {
	return e.Message
}

func csvFail(format string, args ...any) error {
	return &TaskPricingCSVError{Message: fmt.Sprintf(format, args...)}
}

// TaskPricingCSVSummary summarizes a parsed retail pricing CSV.
type TaskPricingCSVSummary struct {
	Models          []string `json:"models"`
	ResolutionTiers int      `json:"resolution_tiers"`
	SourceRows      int      `json:"source_rows"`
	RMBPerUSD       string   `json:"rmb_per_usd,omitempty"`
}

// TaskPricingCSVOptionUpdate is one option write/rollback entry.
type TaskPricingCSVOptionUpdate struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// TaskPricingCSVPlan is the preview/apply payload for a retail CSV import.
type TaskPricingCSVPlan struct {
	GeneratedAt string                       `json:"generated_at"`
	Summary     TaskPricingCSVSummary        `json:"summary"`
	Updates     []TaskPricingCSVOptionUpdate `json:"updates"`
	Rollback    []TaskPricingCSVOptionUpdate `json:"rollback"`
}

type taskPricingCSVRow = map[string]string

type videoVariantKind int

const (
	videoVariantNoReference videoVariantKind = iota
	videoVariantReference
	videoVariantDisabled
)

type tierRecord struct {
	pricesRMB map[videoVariantKind]decimal.Decimal
}

// EmptyTaskPricingCSVTemplate returns a UTF-8 BOM CSV template with headers and
// two example rows (matrix + flat) that can be replaced by operators.
func EmptyTaskPricingCSVTemplate() []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buf)
	_ = writer.Write(taskPricingCSVRequiredColumns)
	_ = writer.Write([]string{
		"Example Model",
		"480p",
		"不含视频",
		taskPricingCSVBillingUnit,
		"3",
	})
	_ = writer.Write([]string{
		"Example Model",
		"480p",
		"输入含视频",
		taskPricingCSVBillingUnit,
		"4",
	})
	_ = writer.Write([]string{
		"Example Flat Model",
		taskPricingCSVFlatResolution,
		"不含视频",
		taskPricingCSVBillingUnit,
		"2",
	})
	_ = writer.Write([]string{
		"Example Flat Model",
		taskPricingCSVFlatResolution,
		"输入含视频",
		taskPricingCSVBillingUnit,
		"2",
	})
	writer.Flush()
	return buf.Bytes()
}

// ParseRetailPricingCSV decodes UTF-8 (with/without BOM) or GB18030 CSV bytes.
func ParseRetailPricingCSV(data []byte) ([]taskPricingCSVRow, error) {
	if len(data) == 0 {
		return nil, csvFail("CSV 为空")
	}
	if len(data) > taskPricingCSVMaxBytes {
		return nil, csvFail("CSV 超过大小限制（%d 字节）", taskPricingCSVMaxBytes)
	}

	decoded, err := decodeCSVBytes(data)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(strings.NewReader(decoded))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	records, err := reader.ReadAll()
	if err != nil {
		return nil, csvFail("CSV 解析失败：%v", err)
	}
	if len(records) == 0 {
		return nil, csvFail("CSV 缺少表头")
	}

	headers := make([]string, len(records[0]))
	for i, name := range records[0] {
		headers[i] = strings.TrimSpace(name)
	}
	headerSet := make(map[string]struct{}, len(headers))
	for _, name := range headers {
		headerSet[name] = struct{}{}
	}
	var missing []string
	for _, required := range taskPricingCSVRequiredColumns {
		if _, ok := headerSet[required]; !ok {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, csvFail("CSV 缺少列：%s", strings.Join(missing, ", "))
	}

	rows := make([]taskPricingCSVRow, 0, len(records)-1)
	for _, record := range records[1:] {
		row := make(taskPricingCSVRow, len(headers))
		empty := true
		for i, name := range headers {
			value := ""
			if i < len(record) {
				value = strings.TrimSpace(record[i])
			}
			row[name] = value
			if value != "" {
				empty = false
			}
		}
		if empty {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func decodeCSVBytes(data []byte) (string, error) {
	if utf8.Valid(data) {
		text := string(data)
		text = strings.TrimPrefix(text, "\ufeff")
		return text, nil
	}
	decoder := simplifiedchinese.GB18030.NewDecoder()
	decoded, _, err := transform.Bytes(decoder, data)
	if err != nil {
		return "", csvFail("CSV 编码无法识别，仅支持 UTF-8 或 GB18030/GBK")
	}
	if !utf8.Valid(decoded) {
		return "", csvFail("CSV 编码无法识别，仅支持 UTF-8 或 GB18030/GBK")
	}
	return string(decoded), nil
}

// BuildTaskPricingFromRows converts retail CSV rows into task-pricing configs.
func BuildTaskPricingFromRows(rows []taskPricingCSVRow, rmbPerUSD decimal.Decimal) (map[string]TaskPricingConfig, TaskPricingCSVSummary, error) {
	if !rmbPerUSD.IsPositive() {
		return nil, TaskPricingCSVSummary{}, csvFail("人民币/USD 汇率必须大于 0")
	}

	records := make(map[string]*tierRecord)
	for index, row := range rows {
		rowNumber := index + 2
		model := strings.TrimSpace(row["平台模型"])
		if model == "" {
			return nil, TaskPricingCSVSummary{}, csvFail("第 %d 行：平台模型不能为空", rowNumber)
		}

		resolution, flat, err := normalizeCSVResolution(row["输出规格"], rowNumber)
		if err != nil {
			return nil, TaskPricingCSVSummary{}, err
		}
		variant, err := parseVideoVariant(row["能力类型"], rowNumber)
		if err != nil {
			return nil, TaskPricingCSVSummary{}, err
		}
		var nativeRMB decimal.Decimal
		if variant == videoVariantDisabled {
			// Disabled rows only declare the policy; price is taken from 不含视频.
			nativeRMB = decimal.Zero
		} else {
			nativeRMB, err = decimalValue(row["对比原生价"], fmt.Sprintf("第 %d 行对比原生价", rowNumber), true)
			if err != nil {
				return nil, TaskPricingCSVSummary{}, err
			}
		}

		key := model + "\x00" + resolution
		if flat {
			key = model + "\x00" + taskPricingCSVFlatResolution
			resolution = taskPricingCSVFlatResolution
		}
		record := records[key]
		if record == nil {
			record = &tierRecord{pricesRMB: make(map[videoVariantKind]decimal.Decimal)}
			records[key] = record
		}
		if _, exists := record.pricesRMB[variant]; exists {
			return nil, TaskPricingCSVSummary{}, csvFail("%s/%s 存在重复的%s行", model, resolution, videoVariantLabel(variant))
		}
		if variant != videoVariantDisabled {
			record.pricesRMB[variant] = nativeRMB
		} else {
			record.pricesRMB[variant] = decimal.Zero
		}
	}

	type keyedRecord struct {
		model      string
		resolution string
		flat       bool
		record     *tierRecord
	}
	ordered := make([]keyedRecord, 0, len(records))
	for key, record := range records {
		parts := strings.SplitN(key, "\x00", 2)
		model := parts[0]
		resolution := parts[1]
		ordered = append(ordered, keyedRecord{
			model:      model,
			resolution: resolution,
			flat:       resolution == taskPricingCSVFlatResolution,
			record:     record,
		})
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].model != ordered[j].model {
			return ordered[i].model < ordered[j].model
		}
		return TaskPricingResolutionLess(ordered[i].resolution, ordered[j].resolution)
	})

	models := make(map[string]TaskPricingConfig)
	for _, item := range ordered {
		prices := item.record.pricesRMB
		_, hasNoRef := prices[videoVariantNoReference]
		_, hasRef := prices[videoVariantReference]
		_, hasDisabled := prices[videoVariantDisabled]

		if hasDisabled {
			if !hasNoRef {
				return nil, TaskPricingCSVSummary{}, csvFail("%s/%s 缺少不含视频价格行", item.model, item.resolution)
			}
			if hasRef {
				return nil, TaskPricingCSVSummary{}, csvFail("%s/%s 不能同时配置含视频价格与禁用参考视频", item.model, item.resolution)
			}
		} else {
			if !hasNoRef || !hasRef {
				missing := "不含视频"
				if hasNoRef {
					missing = "输入含视频"
				}
				return nil, TaskPricingCSVSummary{}, csvFail("%s/%s 缺少%s价格行", item.model, item.resolution, missing)
			}
		}

		noReference := prices[videoVariantNoReference].Div(rmbPerUSD)
		tier := TaskPricingTier{
			NoReferenceVideoUnitPrice: decimalToFloat(noReference),
		}
		if hasDisabled {
			tier.ReferenceVideoPolicy = ReferenceVideoPolicyDisabled
		} else {
			reference := prices[videoVariantReference].Div(rmbPerUSD)
			if reference.Equal(noReference) {
				tier.ReferenceVideoPolicy = ReferenceVideoPolicySame
			} else {
				tier.ReferenceVideoPolicy = ReferenceVideoPolicyCustom
				tier.ReferenceVideoUnitPrice = decimalToFloat(reference)
			}
		}
		cfg := models[item.model]
		if item.flat {
			if cfg.ByResolution != nil {
				return nil, TaskPricingCSVSummary{}, csvFail("模型 %s 不能同时配置扁平价格与分辨率矩阵", item.model)
			}
			if cfg.Unit != "" {
				return nil, TaskPricingCSVSummary{}, csvFail("模型 %s 存在重复的扁平价格配置", item.model)
			}
			cfg = TaskPricingConfig{
				Unit:                      TaskPricingUnitSecond,
				NoReferenceVideoUnitPrice: tier.NoReferenceVideoUnitPrice,
				ReferenceVideoPolicy:      tier.ReferenceVideoPolicy,
				ReferenceVideoUnitPrice:   tier.ReferenceVideoUnitPrice,
			}
			models[item.model] = cfg
			continue
		}

		if cfg.Unit != "" && cfg.ByResolution == nil {
			return nil, TaskPricingCSVSummary{}, csvFail("模型 %s 不能同时配置扁平价格与分辨率矩阵", item.model)
		}
		if cfg.ByResolution == nil {
			cfg = TaskPricingConfig{
				Unit:         TaskPricingUnitSecond,
				ByResolution: make(map[string]TaskPricingTier),
			}
		}
		cfg.ByResolution[item.resolution] = tier
		models[item.model] = cfg
	}

	if err := ValidateTaskPricingMap(models); err != nil {
		return nil, TaskPricingCSVSummary{}, csvFail("生成的任务价格无效：%v", err)
	}

	modelNames := make([]string, 0, len(models))
	for name := range models {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)

	return models, TaskPricingCSVSummary{
		Models:          modelNames,
		ResolutionTiers: len(records),
		SourceRows:      len(rows),
	}, nil
}

// BuildTaskPricingImportPlan merges imported configs into current option values.
func BuildTaskPricingImportPlan(options map[string]string, imported map[string]TaskPricingConfig, summary TaskPricingCSVSummary) (*TaskPricingCSVPlan, error) {
	taskPricing, err := parseOptionTaskPricing(options["billing_setting.task_pricing"])
	if err != nil {
		return nil, err
	}
	billingMode, err := parseOptionStringMap(options["billing_setting.billing_mode"], "billing_setting.billing_mode")
	if err != nil {
		return nil, err
	}
	for model, cfg := range imported {
		taskPricing[model] = cfg
		billingMode[model] = "task_pricing"
	}
	taskPricingJSON, err := marshalCanonicalJSON(taskPricing)
	if err != nil {
		return nil, csvFail("序列化 task_pricing 失败：%v", err)
	}
	if _, err := ParseTaskPricingMapJSON(taskPricingJSON); err != nil {
		return nil, csvFail("合并后的 task_pricing 无效：%v", err)
	}
	billingModeJSON, err := marshalCanonicalJSON(billingMode)
	if err != nil {
		return nil, csvFail("序列化 billing_mode 失败：%v", err)
	}
	nextValues := map[string]string{
		"billing_setting.task_pricing": taskPricingJSON,
		"billing_setting.billing_mode": billingModeJSON,
	}
	previousValues := make(map[string]string, len(taskPricingCSVUpdateOrder))
	for _, key := range taskPricingCSVUpdateOrder {
		value := options[key]
		if strings.TrimSpace(value) == "" {
			value = "{}"
		}
		previousValues[key] = value
	}

	updates := make([]TaskPricingCSVOptionUpdate, 0, len(taskPricingCSVUpdateOrder))
	for _, key := range taskPricingCSVUpdateOrder {
		updates = append(updates, TaskPricingCSVOptionUpdate{Key: key, Value: nextValues[key]})
	}
	rollback := make([]TaskPricingCSVOptionUpdate, 0, len(taskPricingCSVUpdateOrder))
	for i := len(taskPricingCSVUpdateOrder) - 1; i >= 0; i-- {
		key := taskPricingCSVUpdateOrder[i]
		rollback = append(rollback, TaskPricingCSVOptionUpdate{Key: key, Value: previousValues[key]})
	}

	return &TaskPricingCSVPlan{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Summary:     summary,
		Updates:     updates,
		Rollback:    rollback,
	}, nil
}

// ExportTaskPricingCSV renders the current task-pricing map as a retail CSV.
func ExportTaskPricingCSV(configs map[string]TaskPricingConfig, rmbPerUSD decimal.Decimal) ([]byte, error) {
	if !rmbPerUSD.IsPositive() {
		return nil, csvFail("人民币/USD 汇率必须大于 0")
	}

	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(&buf)
	if err := writer.Write(taskPricingCSVRequiredColumns); err != nil {
		return nil, csvFail("写入 CSV 表头失败：%v", err)
	}

	modelNames := make([]string, 0, len(configs))
	for name := range configs {
		modelNames = append(modelNames, name)
	}
	sort.Strings(modelNames)

	for _, modelName := range modelNames {
		cfg := configs[modelName]
		if cfg.ByResolution == nil {
			if err := writeExportedTierRows(writer, modelName, taskPricingCSVFlatResolution, TaskPricingTier{
				NoReferenceVideoUnitPrice: cfg.NoReferenceVideoUnitPrice,
				ReferenceVideoPolicy:      cfg.ReferenceVideoPolicy,
				ReferenceVideoUnitPrice:   cfg.ReferenceVideoUnitPrice,
				GroupRatioPolicy:          cfg.GroupRatioPolicy,
			}, rmbPerUSD); err != nil {
				return nil, err
			}
			continue
		}
		resolutions := TaskPricingResolutionKeys(cfg)
		for _, resolution := range resolutions {
			if err := writeExportedTierRows(writer, modelName, resolution, cfg.ByResolution[resolution], rmbPerUSD); err != nil {
				return nil, err
			}
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, csvFail("写入 CSV 失败：%v", err)
	}
	return buf.Bytes(), nil
}

func writeExportedTierRows(writer *csv.Writer, model, resolution string, tier TaskPricingTier, rmbPerUSD decimal.Decimal) error {
	noRefRMB := floatToRMBString(tier.NoReferenceVideoUnitPrice, rmbPerUSD)
	if err := writer.Write([]string{
		model,
		resolution,
		"不含视频",
		taskPricingCSVBillingUnit,
		noRefRMB,
	}); err != nil {
		return csvFail("写入 CSV 行失败：%v", err)
	}

	switch tier.ReferenceVideoPolicy {
	case ReferenceVideoPolicyDisabled:
		if err := writer.Write([]string{
			model,
			resolution,
			"禁用参考视频",
			taskPricingCSVBillingUnit,
			"0",
		}); err != nil {
			return csvFail("写入 CSV 行失败：%v", err)
		}
	case ReferenceVideoPolicySame:
		if err := writer.Write([]string{
			model,
			resolution,
			"输入含视频",
			taskPricingCSVBillingUnit,
			noRefRMB,
		}); err != nil {
			return csvFail("写入 CSV 行失败：%v", err)
		}
	case ReferenceVideoPolicyCustom:
		refRMB := floatToRMBString(tier.ReferenceVideoUnitPrice, rmbPerUSD)
		if err := writer.Write([]string{
			model,
			resolution,
			"输入含视频",
			taskPricingCSVBillingUnit,
			refRMB,
		}); err != nil {
			return csvFail("写入 CSV 行失败：%v", err)
		}
	default:
		return csvFail("模型 %s/%s 的 reference_video_policy 无效：%s", model, resolution, tier.ReferenceVideoPolicy)
	}
	return nil
}

func floatToRMBString(usd float64, rmbPerUSD decimal.Decimal) string {
	value := decimal.NewFromFloat(usd).Mul(rmbPerUSD).Round(12)
	if value.Equal(value.Truncate(0)) {
		return value.StringFixed(0)
	}
	trimmed := strings.TrimRight(strings.TrimRight(value.StringFixed(12), "0"), ".")
	if trimmed == "" || trimmed == "-" {
		return "0"
	}
	return trimmed
}

func normalizeCSVResolution(value string, rowNumber int) (resolution string, flat bool, err error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == taskPricingCSVFlatResolution {
		return taskPricingCSVFlatResolution, true, nil
	}
	normalized, normErr := NormalizeTaskPricingResolution(trimmed)
	if normErr != nil {
		return "", false, csvFail("第 %d 行：%v", rowNumber, normErr)
	}
	return normalized, false, nil
}

func parseVideoVariant(value string, rowNumber int) (videoVariantKind, error) {
	text := strings.TrimSpace(value)
	if strings.Contains(text, "禁用参考视频") {
		return videoVariantDisabled, nil
	}
	if strings.Contains(text, "不含视频") {
		return videoVariantNoReference, nil
	}
	if strings.Contains(text, "输入含视频") || strings.Contains(text, "含视频") {
		return videoVariantReference, nil
	}
	return 0, csvFail("第 %d 行：能力类型必须能识别为“输入含视频”、“不含视频”或“禁用参考视频”，实际为 %q", rowNumber, text)
}

func videoVariantLabel(kind videoVariantKind) string {
	switch kind {
	case videoVariantNoReference:
		return "不含视频"
	case videoVariantReference:
		return "输入含视频"
	case videoVariantDisabled:
		return "禁用参考视频"
	default:
		return "未知"
	}
}

func decimalValue(value string, label string, positive bool) (decimal.Decimal, error) {
	text := strings.TrimSpace(strings.ReplaceAll(value, ",", ""))
	if text == "" {
		return decimal.Zero, csvFail("%s 必须是数字", label)
	}
	divisor := decimal.NewFromInt(1)
	if strings.HasSuffix(text, "%") {
		text = strings.TrimSpace(strings.TrimSuffix(text, "%"))
		divisor = decimal.NewFromInt(100)
	}
	parsed, err := decimal.NewFromString(text)
	if err != nil {
		return decimal.Zero, csvFail("%s 必须是数字，实际为 %q", label, value)
	}
	result := parsed.Div(divisor)
	if result.IsNegative() || (positive && !result.IsPositive()) {
		relation := "不小于 0"
		if positive {
			relation = "大于 0"
		}
		return decimal.Zero, csvFail("%s 必须有限且%s", label, relation)
	}
	return result, nil
}

func decimalToFloat(value decimal.Decimal) float64 {
	rounded := value.Round(12)
	f, _ := rounded.Float64()
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return 0
	}
	return f
}

func decimalToJSONNumber(value decimal.Decimal) any {
	rounded := value.Round(12)
	if rounded.Equal(rounded.Truncate(0)) {
		return rounded.IntPart()
	}
	f, _ := rounded.Float64()
	return f
}

func parseOptionTaskPricing(raw string) (map[string]TaskPricingConfig, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]TaskPricingConfig{}, nil
	}
	configs, err := ParseTaskPricingMapJSON(trimmed)
	if err != nil {
		return nil, csvFail("线上选项 billing_setting.task_pricing 无效：%v", err)
	}
	return configs, nil
}

func parseOptionStringMap(raw, key string) (map[string]string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]string{}, nil
	}
	var parsed map[string]any
	if err := common.UnmarshalJsonStr(trimmed, &parsed); err != nil {
		return nil, csvFail("线上选项 %s 不是合法 JSON：%v", key, err)
	}
	result := make(map[string]string, len(parsed))
	for k, v := range parsed {
		result[k] = fmt.Sprint(v)
	}
	return result, nil
}

func parseOptionNumberMap(raw, key string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}, nil
	}
	var parsed map[string]any
	if err := common.UnmarshalJsonStr(trimmed, &parsed); err != nil {
		return nil, csvFail("线上选项 %s 不是合法 JSON：%v", key, err)
	}
	if parsed == nil {
		return nil, csvFail("线上选项 %s 必须是 JSON 对象", key)
	}
	return parsed, nil
}

func marshalCanonicalJSON(value any) (string, error) {
	encoded, err := common.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// ParseUSDExchangeRate parses a positive USD↔RMB rate from an option value.
func ParseUSDExchangeRate(raw string) (decimal.Decimal, error) {
	return decimalValue(raw, "线上 USDExchangeRate", true)
}

// ReadTaskPricingCSVLimited reads at most taskPricingCSVMaxBytes from r.
func ReadTaskPricingCSVLimited(r io.Reader) ([]byte, error) {
	limited := io.LimitReader(r, taskPricingCSVMaxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, csvFail("读取 CSV 失败：%v", err)
	}
	if len(data) > taskPricingCSVMaxBytes {
		return nil, csvFail("CSV 超过大小限制（%d 字节）", taskPricingCSVMaxBytes)
	}
	return data, nil
}
