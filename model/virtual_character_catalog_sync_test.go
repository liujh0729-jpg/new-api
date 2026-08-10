package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/stretchr/testify/require"
)

func TestMapAIPDDVirtualCharacterCatalogEntriesSkipsDisabled(t *testing.T) {
	ageMin, ageMax := 20, 40
	entries, err := mapAIPDDVirtualCharacterCatalogEntries(aipddcatalog.VirtualCharacterCatalog{
		Version:  "v1",
		Revision: "r1",
		Items: []aipddcatalog.VirtualCharacterCatalogItem{
			{
				ProviderAssetID: "a1", Name: "On", CoverURL: "https://example.com/a.png", Enabled: true,
				Tags:        []string{"中国", "男", "20-40岁", "镖局总镖头", "直率"},
				Nationality: "中国", Gender: "男", AgeMin: &ageMin, AgeMax: &ageMax,
				Occupation: "镖局总镖头", Temperament: "直率",
			},
			{ProviderAssetID: "a2", Name: "Off", CoverURL: "https://example.com/b.png", Enabled: false},
		},
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "a1", entries[0].AssetID)
	require.Equal(t, "中国", entries[0].Nationality)
	require.Equal(t, "男", entries[0].Gender)
	require.NotNil(t, entries[0].AgeMin)
	require.Equal(t, 20, *entries[0].AgeMin)
	require.Equal(t, "镖局总镖头", entries[0].Occupation)
	require.Equal(t, "直率", entries[0].Temperament)
}

func TestMapAIPDDVirtualCharacterCatalogEntriesEnrichesFromTags(t *testing.T) {
	entries, err := mapAIPDDVirtualCharacterCatalogEntries(aipddcatalog.VirtualCharacterCatalog{
		Version:  "v1",
		Revision: "r1",
		Items: []aipddcatalog.VirtualCharacterCatalogItem{
			{
				ProviderAssetID: "a1", Name: "On", CoverURL: "https://example.com/a.png", Enabled: true,
				Tags: []string{"日本", "女", "40-60岁", "教师", "温和"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "日本", entries[0].Nationality)
	require.Equal(t, "女", entries[0].Gender)
	require.NotNil(t, entries[0].AgeMin)
	require.Equal(t, 40, *entries[0].AgeMin)
	require.NotNil(t, entries[0].AgeMax)
	require.Equal(t, 60, *entries[0].AgeMax)
	require.Equal(t, "教师", entries[0].Occupation)
	require.Equal(t, "温和", entries[0].Temperament)
}

func TestSyncVirtualCharacterCatalogFromAIPDDAppliesAndSkipsSameRevision(t *testing.T) {
	cleanupVirtualCharacterABTables(t)
	account := &VirtualCharacterProviderAccount{
		Enabled: true, OfficialEnabled: true, Region: "cn-beijing", ProjectName: "default",
		EncryptedAccessKey: "ak", EncryptedSecretKey: "sk",
	}
	require.NoError(t, DB.Create(account).Error)

	payload := map[string]any{
		"code": 0, "message": "ok",
		"data": map[string]any{
			"schemaVersion": 1,
			"revision":      "rev-sync-1",
			"version":       "catalog-v1",
			"items": []map[string]any{{
				"provider_asset_id": "asset://sync-1",
				"name":              "Sync One",
				"cover_url":         "https://cdn.example.com/1.png",
				"enabled":           true,
				"tags":              []string{"JP"},
			}},
		},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	t.Setenv("AIPDD_API_KEY", "sk-test")
	t.Setenv("AIPDD_BASE_URL", server.URL)

	first, err := SyncVirtualCharacterCatalogFromAIPDD(context.Background(), server.Client(), true, 9)
	require.NoError(t, err)
	require.False(t, first.Skipped)
	require.Equal(t, 1, first.Created)
	require.Equal(t, "rev-sync-1", first.Revision)
	require.Greater(t, GetVirtualCharacterAIPDDCatalogLastSyncAt(), int64(0))

	second, err := SyncVirtualCharacterCatalogFromAIPDD(context.Background(), server.Client(), false, 9)
	require.NoError(t, err)
	require.True(t, second.Skipped)
	require.Equal(t, "same_revision", second.SkipReason)
	require.Equal(t, 2, hits)
}
