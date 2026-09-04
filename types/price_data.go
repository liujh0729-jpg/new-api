package types

import (
	"fmt"
	"strings"
	"unicode"
)

const MembershipMultiplierScale int64 = 1_000_000

type GroupRatioInfo struct {
	GroupRatio        float64
	GroupSpecialRatio float64
	HasSpecialRatio   bool
}

// MembershipRatioInfo is resolved once when a request is accepted and then
// carried through the complete billing lifecycle. ConfiguredMultiplierPPM is
// the user's tier benefit; AppliedMultiplierPPM may become 1.0 for an explicit
// exemption without hiding the tier the user owns.
type MembershipRatioInfo struct {
	GrantId                 int    `json:"grant_id"`
	LevelId                 int    `json:"level_id"`
	Code                    string `json:"code"`
	DisplayName             string `json:"display_name"`
	Rank                    int    `json:"rank"`
	ConfiguredMultiplierPPM int64  `json:"configured_multiplier_ppm"`
	AppliedMultiplierPPM    int64  `json:"applied_multiplier_ppm"`
	StartsAt                int64  `json:"starts_at"`
	EndsAt                  int64  `json:"ends_at"`
	ResolvedAt              int64  `json:"resolved_at"`
	Exempt                  bool   `json:"exempt"`
	ExemptionReason         string `json:"exemption_reason,omitempty"`
}

func (m MembershipRatioInfo) Normalized() MembershipRatioInfo {
	if strings.TrimSpace(m.Code) == "" {
		m.Code = "NORMAL"
	}
	if strings.TrimSpace(m.DisplayName) == "" {
		m.DisplayName = "普通用户"
	}
	if m.ConfiguredMultiplierPPM <= 0 || m.ConfiguredMultiplierPPM > MembershipMultiplierScale {
		m.ConfiguredMultiplierPPM = MembershipMultiplierScale
	}
	if m.AppliedMultiplierPPM <= 0 || m.AppliedMultiplierPPM > MembershipMultiplierScale {
		m.AppliedMultiplierPPM = m.ConfiguredMultiplierPPM
	}
	return m
}

func (m MembershipRatioInfo) ConfiguredMultiplier() float64 {
	m = m.Normalized()
	return float64(m.ConfiguredMultiplierPPM) / float64(MembershipMultiplierScale)
}

func (m MembershipRatioInfo) AppliedMultiplier() float64 {
	m = m.Normalized()
	return float64(m.AppliedMultiplierPPM) / float64(MembershipMultiplierScale)
}

// ApplyMembershipPricingPolicy uses only the public model name and canonical
// resolved request facts. The API model name itself is never rewritten.
func ApplyMembershipPricingPolicy(info MembershipRatioInfo, originModelName string, resolution string) MembershipRatioInfo {
	info = info.Normalized()
	info.AppliedMultiplierPPM = info.ConfiguredMultiplierPPM
	info.Exempt = false
	info.ExemptionReason = ""
	if IsAPSeedance480PMembershipExempt(originModelName, resolution) {
		info.AppliedMultiplierPPM = MembershipMultiplierScale
		info.Exempt = true
		info.ExemptionReason = "ap_seedance_480p"
	}
	return info
}

func IsAPSeedance480PMembershipExempt(originModelName string, resolution string) bool {
	if !strings.EqualFold(strings.TrimSpace(resolution), "480p") {
		return false
	}
	var normalized strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(originModelName)) {
		if unicode.IsSpace(char) || char == '-' || char == '_' {
			continue
		}
		normalized.WriteRune(char)
	}
	return strings.Contains(normalized.String(), "apseedance")
}

type PriceData struct {
	FreeModel            bool
	ModelPrice           float64
	ModelRatio           float64
	CompletionRatio      float64
	CacheRatio           float64
	CacheCreationRatio   float64
	CacheCreation5mRatio float64
	CacheCreation1hRatio float64
	ImageRatio           float64
	AudioRatio           float64
	AudioCompletionRatio float64
	OtherRatios          map[string]float64
	UsePrice             bool
	Quota                int // 按次计费的最终额度（MJ / Task）
	QuotaToPreConsume    int // 按量计费的预消耗额度
	GroupRatioInfo       GroupRatioInfo
	MembershipRatioInfo  MembershipRatioInfo
}

func (p *PriceData) AddOtherRatio(key string, ratio float64) {
	if p.OtherRatios == nil {
		p.OtherRatios = make(map[string]float64)
	}
	if ratio <= 0 {
		return
	}
	p.OtherRatios[key] = ratio
}

func (p *PriceData) ToSetting() string {
	member := p.MembershipRatioInfo.Normalized()
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, MembershipCode: %s, MembershipMultiplier: %f, UsePrice: %t, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, QuotaToPreConsume: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, member.Code, member.AppliedMultiplier(), p.UsePrice, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.QuotaToPreConsume, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio)
}
