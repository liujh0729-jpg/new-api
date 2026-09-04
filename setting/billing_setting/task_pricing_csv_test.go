package billing_setting

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func csvRow(model, resolution, kind, native string, ratios []string) taskPricingCSVRow {
	return taskPricingCSVRow{
		"平台模型":  model,
		"输出规格":  resolution,
		"能力类型":  kind,
		"计费单位":  "条/5秒",
		"对比原生价": native,
	}
}

func TestBuildTaskPricingFromRowsTreatsNativeAsRMBPerSecond(t *testing.T) {
	rows := []taskPricingCSVRow{
		csvRow("AP Seedance", "480P", "输入含视频+平超", "4", []string{"1", "1", "1", "1", "1", "1"}),
		csvRow("AP Seedance", "480P", "不含视频+平超", "3", []string{"1", "1", "1", "1", "1", "1"}),
		csvRow("AP Seedance", "720p", "输入含视频 超一档", "6.7", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
		csvRow("AP Seedance", "720p", "不含视频 超一档", "4.97", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
	}

	pricing, summary, err := BuildTaskPricingFromRows(rows, decimal.RequireFromString("7.3"))
	if err != nil {
		t.Fatal(err)
	}
	tiers := pricing["AP Seedance"].ByResolution
	if tiers["480p"].GroupRatioPolicy != "" {
		t.Fatalf("480p policy = %q", tiers["480p"].GroupRatioPolicy)
	}
	if tiers["720p"].GroupRatioPolicy != "" {
		t.Fatalf("720p should omit group_ratio_policy, got %q", tiers["720p"].GroupRatioPolicy)
	}
	wantNoRef := 3 / 7.3
	wantRef := 4 / 7.3
	if mathAbs(tiers["480p"].NoReferenceVideoUnitPrice-wantNoRef) > 1e-9 {
		t.Fatalf("480p no-ref = %v, want %v", tiers["480p"].NoReferenceVideoUnitPrice, wantNoRef)
	}
	if mathAbs(tiers["480p"].ReferenceVideoUnitPrice-wantRef) > 1e-9 {
		t.Fatalf("480p ref = %v, want %v", tiers["480p"].ReferenceVideoUnitPrice, wantRef)
	}
	if summary.ResolutionTiers != 2 {
		t.Fatalf("resolution tiers = %d", summary.ResolutionTiers)
	}
}

func TestBuildTaskPricingBillingUnitDoesNotScale(t *testing.T) {
	oneSecond := []taskPricingCSVRow{
		csvRow("AP Seedance", "720p", "输入含视频", "6", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
		csvRow("AP Seedance", "720p", "不含视频", "4", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
	}
	fiveSecond := []taskPricingCSVRow{
		csvRow("AP Seedance", "720p", "输入含视频", "6", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
		csvRow("AP Seedance", "720p", "不含视频", "4", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
	}
	oneSecond[0]["计费单位"] = "条/1秒"
	oneSecond[1]["计费单位"] = "条/1秒"
	fiveSecond[0]["计费单位"] = "条/5秒"
	fiveSecond[1]["计费单位"] = "条/5秒"

	left, _, err := BuildTaskPricingFromRows(oneSecond, decimal.NewFromInt(2))
	if err != nil {
		t.Fatal(err)
	}
	right, _, err := BuildTaskPricingFromRows(fiveSecond, decimal.NewFromInt(2))
	if err != nil {
		t.Fatal(err)
	}
	leftTier := left["AP Seedance"].ByResolution["720p"]
	rightTier := right["AP Seedance"].ByResolution["720p"]
	if leftTier != rightTier {
		t.Fatalf("billing unit scaled prices: %#v vs %#v", leftTier, rightTier)
	}
	if leftTier.NoReferenceVideoUnitPrice != 2 || leftTier.ReferenceVideoUnitPrice != 3 {
		t.Fatalf("unexpected prices: %#v", leftTier)
	}
}

func TestBuildTaskPricingIgnoresRemovedLegacyGroupRatioColumns(t *testing.T) {
	rows := []taskPricingCSVRow{
		csvRow("AP Seedance", "1080p", "输入含视频", "10.5", []string{".70", ".75", ".75", ".8", ".8", ".85"}),
		csvRow("AP Seedance", "1080p", "不含视频", "8", []string{".70", ".75", ".75", ".8", ".8", ".85"}),
	}
	_, _, err := BuildTaskPricingFromRows(rows, decimal.RequireFromString("7.3"))
	if err != nil {
		t.Fatalf("legacy columns should not affect base pricing: %v", err)
	}
}

func TestBuildTaskPricingImportPlanPreservesUnrelatedEntries(t *testing.T) {
	pricing := map[string]TaskPricingConfig{
		"AP Seedance": {
			Unit: TaskPricingUnitSecond,
			ByResolution: map[string]TaskPricingTier{
				"480p": {
					NoReferenceVideoUnitPrice: 1,
					ReferenceVideoPolicy:      ReferenceVideoPolicySame,
					GroupRatioPolicy:          TaskPricingGroupRatioNone,
				},
			},
		},
	}
	options := map[string]string{
		"billing_setting.task_pricing": `{"other":{"unit":"second","no_reference_video_unit_price":9,"reference_video_policy":"same"}}`,
		"billing_setting.billing_mode": `{"other":"ratio"}`,
		"GroupRatio":                   `{"default":1}`,
		"UserUsableGroups":             `{"default":"默认分组"}`,
	}
	plan, err := BuildTaskPricingImportPlan(options, pricing, TaskPricingCSVSummary{
		Models:          []string{"AP Seedance"},
		ResolutionTiers: 1,
		SourceRows:      2,
	})
	if err != nil {
		t.Fatal(err)
	}
	updates := map[string]string{}
	for _, item := range plan.Updates {
		updates[item.Key] = item.Value
	}
	merged, err := ParseTaskPricingMapJSON(updates["billing_setting.task_pricing"])
	if err != nil {
		t.Fatal(err)
	}
	if merged["other"].NoReferenceVideoUnitPrice != 9 {
		t.Fatalf("lost unrelated task pricing: %#v", merged["other"])
	}
	if !strings.Contains(updates["billing_setting.billing_mode"], `"AP Seedance":"task_pricing"`) &&
		!strings.Contains(updates["billing_setting.billing_mode"], `"AP Seedance": "task_pricing"`) {
		t.Fatalf("billing mode missing imported model: %s", updates["billing_setting.billing_mode"])
	}
	if _, exists := updates["GroupRatio"]; exists {
		t.Fatalf("base-price import must not mutate group ratios: %#v", updates)
	}
	if _, exists := updates["UserUsableGroups"]; exists {
		t.Fatalf("base-price import must not mutate usable groups: %#v", updates)
	}
}

func TestBuildTaskPricingDoesNotInferExemptionFromLegacyColumns(t *testing.T) {
	rows := []taskPricingCSVRow{
		csvRow("AP Seedance", "480p", "输入含视频", "4", []string{"1", "1", "1", "1", "1", "1"}),
		csvRow("AP Seedance", "480p", "不含视频", "3", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
	}
	pricing, _, err := BuildTaskPricingFromRows(rows, decimal.RequireFromString("7.3"))
	if err != nil {
		t.Fatal(err)
	}
	if pricing["AP Seedance"].ByResolution["480p"].GroupRatioPolicy != "" {
		t.Fatalf("legacy VIP columns must not create pricing exemptions")
	}
}

func TestParseRetailPricingCSVUTF8BOMAndGBK(t *testing.T) {
	header := strings.Join(taskPricingCSVRequiredColumns, ",")
	body := header + "\n" + strings.Join([]string{
		"AP Seedance", "480p", "不含视频", "条/5秒", "3",
	}, ",")

	rows, err := ParseRetailPricingCSV([]byte("\ufeff" + body))
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["平台模型"] != "AP Seedance" {
		t.Fatalf("utf8 bom row = %#v", rows[0])
	}

	gbk, err := simplifiedChineseBytes(body)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = ParseRetailPricingCSV(gbk)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["平台模型"] != "AP Seedance" {
		t.Fatalf("gbk row = %#v", rows[0])
	}
}

func TestParseRetailPricingCSVAcceptsLegacyExtraTierHeaders(t *testing.T) {
	legacy := []string{"平台模型", "输出规格", "能力类型", "计费单位", "对比原生价", "VIP1", "VIP2", "VIP3", "VIP4", "VIP5"}
	content := strings.Join(legacy, ",") + "\n" + strings.Join([]string{
		"AP Seedance", "480p", "不含视频", "条/5秒", "3", "1", "1", "1", "1", "1",
	}, ",")
	rows, err := ParseRetailPricingCSV([]byte(content))
	if err != nil || len(rows) != 1 {
		t.Fatalf("legacy extra columns should be ignored, got rows=%d err=%v", len(rows), err)
	}
}

func TestBuildTaskPricingFlatAndDisabled(t *testing.T) {
	rows := []taskPricingCSVRow{
		csvRow("Flat Model", "-", "不含视频", "4", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
		csvRow("Flat Model", "-", "禁用参考视频", "0", []string{".73", ".78", ".8", ".85", ".9", ".95"}),
	}
	pricing, _, err := BuildTaskPricingFromRows(rows, decimal.NewFromInt(2))
	if err != nil {
		t.Fatal(err)
	}
	cfg := pricing["Flat Model"]
	if cfg.ByResolution != nil {
		t.Fatalf("expected flat config, got %#v", cfg)
	}
	if cfg.NoReferenceVideoUnitPrice != 2 {
		t.Fatalf("price = %v", cfg.NoReferenceVideoUnitPrice)
	}
	if cfg.ReferenceVideoPolicy != ReferenceVideoPolicyDisabled {
		t.Fatalf("policy = %q", cfg.ReferenceVideoPolicy)
	}
}

func TestExportTaskPricingCSVRoundTrip(t *testing.T) {
	original := map[string]TaskPricingConfig{
		"AP Seedance": {
			Unit: TaskPricingUnitSecond,
			ByResolution: map[string]TaskPricingTier{
				"480p": {
					NoReferenceVideoUnitPrice: 3 / 7.3,
					ReferenceVideoPolicy:      ReferenceVideoPolicyCustom,
					ReferenceVideoUnitPrice:   4 / 7.3,
					GroupRatioPolicy:          TaskPricingGroupRatioNone,
				},
			},
		},
	}
	data, err := ExportTaskPricingCSV(original, decimal.RequireFromString("7.3"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := ParseRetailPricingCSV(data)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, _, err := BuildTaskPricingFromRows(rows, decimal.RequireFromString("7.3"))
	if err != nil {
		t.Fatal(err)
	}
	got := rebuilt["AP Seedance"].ByResolution["480p"]
	want := original["AP Seedance"].ByResolution["480p"]
	if mathAbs(got.NoReferenceVideoUnitPrice-want.NoReferenceVideoUnitPrice) > 1e-9 {
		t.Fatalf("no-ref roundtrip %v vs %v", got.NoReferenceVideoUnitPrice, want.NoReferenceVideoUnitPrice)
	}
	if mathAbs(got.ReferenceVideoUnitPrice-want.ReferenceVideoUnitPrice) > 1e-9 {
		t.Fatalf("ref roundtrip %v vs %v", got.ReferenceVideoUnitPrice, want.ReferenceVideoUnitPrice)
	}
	if got.GroupRatioPolicy != "" {
		t.Fatalf("policy = %q", got.GroupRatioPolicy)
	}
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func simplifiedChineseBytes(text string) ([]byte, error) {
	encoder := simplifiedchinese.GB18030.NewEncoder()
	encoded, _, err := transform.Bytes(encoder, []byte(text))
	return encoded, err
}
