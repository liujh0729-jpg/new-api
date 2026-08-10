package aipddcatalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestVirtualCharacterCatalogValidateRejectsSignedCover(t *testing.T) {
	catalog := VirtualCharacterCatalog{
		SchemaVersion: 1,
		Revision:      "rev-1",
		Version:       "v1",
		Items: []VirtualCharacterCatalogItem{{
			ProviderAssetID: "asset://abc",
			Name:            "A",
			CoverURL:        "https://example.com/a.png?X-Tos-Signature=deadbeef",
			Enabled:         true,
		}},
	}
	require.Error(t, catalog.Validate())
}

func TestFetchVirtualCharacterCatalogSupports304(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, VirtualCharacterCatalogPath, r.URL.Path)
		require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
		require.NotEmpty(t, r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	_, err := FetchVirtualCharacterCatalog(context.Background(), server.Client(), server.URL, "sk-test", "rev-1")
	require.ErrorIs(t, err, ErrVirtualCharacterCatalogNotModified)
}

func TestFetchVirtualCharacterCatalogDecodesPublishedPayload(t *testing.T) {
	payload := map[string]any{
		"code":    0,
		"message": "ok",
		"data": map[string]any{
			"schemaVersion": 1,
			"revision":      "rev-2",
			"version":       "2026.03.01",
			"publishedAt":   "2026-03-01T00:00:00Z",
			"items": []map[string]any{{
				"provider":          "volc",
				"provider_asset_id": "asset://preset-1",
				"name":              "Preset One",
				"description":       "desc",
				"tags":              []string{"CN"},
				"cover_url":         "https://cdn.example.com/covers/1.png",
				"enabled":           true,
				"sort_order":        1,
			}},
		},
	}
	body, err := common.Marshal(payload)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	catalog, err := FetchVirtualCharacterCatalog(context.Background(), server.Client(), server.URL, "sk-test", "")
	require.NoError(t, err)
	require.Equal(t, "rev-2", catalog.Revision)
	require.Equal(t, "preset-1", catalog.Items[0].ProviderAssetID)
	require.Equal(t, "https://cdn.example.com/covers/1.png", catalog.Items[0].CoverURL)
}
