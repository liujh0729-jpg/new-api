package model

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
)

const virtualCharacterAIPDDCatalogLastSyncAtKey = "VirtualCharacterAIPDDCatalogLastSyncAt"

type VirtualCharacterAIPDDCatalogSyncResult struct {
	Version      string `json:"version"`
	Revision     string `json:"revision"`
	Total        int    `json:"total"`
	Created      int    `json:"created"`
	Updated      int    `json:"updated"`
	Offlined     int    `json:"offlined"`
	Skipped      bool   `json:"skipped"`
	SkipReason   string `json:"skip_reason,omitempty"`
	LastSyncedAt int64  `json:"last_synced_at"`
}

func GetVirtualCharacterAIPDDCatalogLastSyncAt() int64 {
	common.OptionMapRWMutex.RLock()
	raw := strings.TrimSpace(common.OptionMap[virtualCharacterAIPDDCatalogLastSyncAtKey])
	common.OptionMapRWMutex.RUnlock()
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func markVirtualCharacterAIPDDCatalogSynced(at time.Time) error {
	return UpdateOption(virtualCharacterAIPDDCatalogLastSyncAtKey, strconv.FormatInt(at.Unix(), 10))
}

// SyncVirtualCharacterCatalogFromAIPDD pulls the published official catalog and
// upserts local public volc_preset characters. When force is false and the
// upstream revision matches the latest local import, the apply step is skipped.
func SyncVirtualCharacterCatalogFromAIPDD(ctx context.Context, client *http.Client, force bool, operatorUserID int) (VirtualCharacterAIPDDCatalogSyncResult, error) {
	apiKey := getAIPDDKeyFromEnv()
	if apiKey == "" {
		return VirtualCharacterAIPDDCatalogSyncResult{}, errors.New("AIPDD_API_KEY is not configured")
	}
	account, err := GetEnabledVirtualCharacterProviderAccount()
	if err != nil {
		return VirtualCharacterAIPDDCatalogSyncResult{}, errors.New("virtual character provider account is not enabled")
	}
	if !account.Enabled {
		return VirtualCharacterAIPDDCatalogSyncResult{}, errors.New("virtual character library is disabled")
	}

	ifNoneMatch := ""
	if !force {
		if latest, latestErr := GetLatestVirtualCharacterCatalogImport(); latestErr == nil {
			ifNoneMatch = strings.TrimSpace(latest.ContentHash)
		}
	}

	catalog, err := aipddcatalog.FetchVirtualCharacterCatalog(ctx, client, getAIPDDBaseURLFromEnv(), apiKey, ifNoneMatch)
	if errors.Is(err, aipddcatalog.ErrVirtualCharacterCatalogNotModified) {
		now := time.Now()
		_ = markVirtualCharacterAIPDDCatalogSynced(now)
		latest, _ := GetLatestVirtualCharacterCatalogImport()
		result := VirtualCharacterAIPDDCatalogSyncResult{
			Skipped:      true,
			SkipReason:   "not_modified",
			LastSyncedAt: now.Unix(),
		}
		if latest != nil {
			result.Version = latest.Version
			result.Revision = latest.ContentHash
			result.Total = latest.Total
			result.Created = latest.Created
			result.Updated = latest.Updated
			result.Offlined = latest.Offlined
		}
		return result, nil
	}
	if err != nil {
		return VirtualCharacterAIPDDCatalogSyncResult{}, err
	}

	if !force {
		if latest, latestErr := GetLatestVirtualCharacterCatalogImport(); latestErr == nil &&
			strings.TrimSpace(latest.ContentHash) == strings.TrimSpace(catalog.Revision) {
			now := time.Now()
			_ = markVirtualCharacterAIPDDCatalogSynced(now)
			return VirtualCharacterAIPDDCatalogSyncResult{
				Version: catalog.Version, Revision: catalog.Revision, Total: latest.Total,
				Created: latest.Created, Updated: latest.Updated, Offlined: latest.Offlined,
				Skipped: true, SkipReason: "same_revision", LastSyncedAt: now.Unix(),
			}, nil
		}
	}

	entries, err := mapAIPDDVirtualCharacterCatalogEntries(catalog)
	if err != nil {
		return VirtualCharacterAIPDDCatalogSyncResult{}, err
	}
	stats, err := ApplyVirtualCharacterCatalog(catalog.Version, catalog.Revision, entries, operatorUserID, account.ID)
	if err != nil {
		return VirtualCharacterAIPDDCatalogSyncResult{}, err
	}
	now := time.Now()
	if err := markVirtualCharacterAIPDDCatalogSynced(now); err != nil {
		common.SysError("record virtual character AIPDD catalog sync time: " + err.Error())
	}
	return VirtualCharacterAIPDDCatalogSyncResult{
		Version: catalog.Version, Revision: catalog.Revision, Total: stats.Total,
		Created: stats.Created, Updated: stats.Updated, Offlined: stats.Offlined,
		LastSyncedAt: now.Unix(),
	}, nil
}

func mapAIPDDVirtualCharacterCatalogEntries(catalog aipddcatalog.VirtualCharacterCatalog) ([]VirtualCharacterCatalogEntry, error) {
	entries := make([]VirtualCharacterCatalogEntry, 0, len(catalog.Items))
	for i := range catalog.Items {
		item := catalog.Items[i]
		if !item.Enabled {
			continue
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return nil, fmt.Errorf("item %s has an empty name", item.ProviderAssetID)
		}
		if utf8.RuneCountInString(name) > 191 {
			name = string([]rune(name)[:191])
		}
		description := strings.TrimSpace(item.Description)
		if utf8.RuneCountInString(description) > 2000 {
			description = string([]rune(description)[:2000])
		}
		tagsJSON, err := marshalVirtualCharacterCatalogTags(item.Tags)
		if err != nil {
			return nil, fmt.Errorf("item %s tags: %w", item.ProviderAssetID, err)
		}
		entry := VirtualCharacterCatalogEntry{
			AssetID:     item.ProviderAssetID,
			Name:        name,
			Description: description,
			TagsJSON:    tagsJSON,
			CoverURL:    item.CoverURL,
			Enabled:     true,
			Nationality: strings.TrimSpace(item.Nationality),
			Gender:      strings.TrimSpace(item.Gender),
			AgeMin:      item.AgeMin,
			AgeMax:      item.AgeMax,
			Occupation:  truncateVirtualCharacterFacet(item.Occupation, 128),
			Temperament: truncateVirtualCharacterFacet(item.Temperament, 128),
		}
		EnrichVirtualCharacterCatalogEntryFacets(&entry, item.Tags)
		if entry.AgeMin != nil && entry.AgeMax != nil && *entry.AgeMin > *entry.AgeMax {
			return nil, fmt.Errorf("item %s has invalid age range", item.ProviderAssetID)
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, errors.New("AIPDD virtual character catalog has no enabled items")
	}
	return entries, nil
}

func truncateVirtualCharacterFacet(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	return string([]rune(value)[:maxRunes])
}

// EnrichVirtualCharacterCatalogEntryFacets fills missing structured facets from tags.
func EnrichVirtualCharacterCatalogEntryFacets(entry *VirtualCharacterCatalogEntry, tags []string) {
	if entry == nil {
		return
	}
	if strings.TrimSpace(entry.Nationality) == "" {
		entry.Nationality = GuessVirtualCharacterNationalityFromTags(tags)
	}
	if strings.TrimSpace(entry.Gender) == "" {
		entry.Gender = GuessVirtualCharacterGenderFromTags(tags)
	}
	if entry.AgeMin == nil || entry.AgeMax == nil {
		if ageMin, ageMax, ok := ParseVirtualCharacterAgeBandFromTags(tags); ok {
			if entry.AgeMin == nil {
				entry.AgeMin = &ageMin
			}
			if entry.AgeMax == nil {
				entry.AgeMax = &ageMax
			}
		}
	}
	if strings.TrimSpace(entry.Occupation) == "" || strings.TrimSpace(entry.Temperament) == "" {
		for _, tag := range tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, knownNationality := virtualCharacterNationalitySet[tag]; knownNationality {
				continue
			}
			if _, knownGender := virtualCharacterGenderSet[tag]; knownGender {
				continue
			}
			if _, _, isAge := ParseVirtualCharacterAgeBandKey(tag); isAge {
				continue
			}
			if strings.TrimSpace(entry.Occupation) == "" {
				entry.Occupation = truncateVirtualCharacterFacet(tag, 128)
				continue
			}
			if strings.TrimSpace(entry.Temperament) == "" {
				entry.Temperament = truncateVirtualCharacterFacet(tag, 128)
			}
		}
	}
}

func marshalVirtualCharacterCatalogTags(tags []string) (string, error) {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > 32 {
			tag = string([]rune(tag)[:32])
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
		if len(normalized) >= 20 {
			break
		}
	}
	payload, err := common.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
