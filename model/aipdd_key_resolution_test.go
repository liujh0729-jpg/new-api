package model

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/aipddcatalog"
	"github.com/stretchr/testify/require"
)

func TestResolveAIPDDCatalogKeyPrefersChannel(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "aipdd-env-key")
	require.NoError(t, UpdateOption(aipddAPIKeyOptionKey, "aipdd-wizard-key"))
	require.NoError(t, DB.Create(&Channel{
		Type:   constant.ChannelTypeAIPDD,
		Key:    "aipdd-channel-key",
		Name:   aipddEnvChannelName,
		Status: common.ChannelStatusEnabled,
	}).Error)

	require.Equal(t, "aipdd-channel-key", ResolveAIPDDCatalogKey())
}

func TestResolveAIPDDCatalogKeyFallsBackToWizardOption(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "aipdd-env-key")
	require.NoError(t, UpdateOption(aipddAPIKeyOptionKey, "aipdd-wizard-key"))

	require.Equal(t, "aipdd-wizard-key", ResolveAIPDDCatalogKey())
}

func TestResolveAIPDDCatalogKeyFallsBackToEnv(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "  aipdd-env-key  ")

	require.Equal(t, "aipdd-env-key", ResolveAIPDDCatalogKey())
}

func TestResolveAIPDDCatalogKeyEmptyWhenNothingConfigured(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "")

	require.Equal(t, "", ResolveAIPDDCatalogKey())
	require.False(t, IsAIPDDCatalogKeyConfigured())
}

func TestResolveAIPDDCatalogKeyUsesFirstNonBlankPoolLine(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_API_KEY", "aipdd-env-key")
	require.NoError(t, DB.Create(&Channel{
		Type: constant.ChannelTypeAIPDD,
		Key:  "\naipdd-pool-second",
		Name: aipddEnvChannelName,
	}).Error)

	require.Equal(t, "aipdd-pool-second", ResolveAIPDDCatalogKey())
}

// TestResolveAIPDDBaseURLPrefersChannel documents the database-first order for
// the AIPDD origin: what the channel says, then AIPDD_BASE_URL, then the default.
func TestResolveAIPDDBaseURLPrefersChannel(t *testing.T) {
	truncateTables(t)
	t.Setenv("AIPDD_BASE_URL", "https://env.aipdd.example")
	channelBaseURL := "https://channel.aipdd.example/"
	require.NoError(t, DB.Create(&Channel{
		Type:    constant.ChannelTypeAIPDD,
		Name:    aipddEnvChannelName,
		Key:     "aipdd-channel-key",
		BaseURL: &channelBaseURL,
	}).Error)

	require.Equal(t, "https://channel.aipdd.example", ResolveAIPDDBaseURL())
}

func TestResolveAIPDDBaseURLFallsBackToEnvThenDefault(t *testing.T) {
	truncateTables(t)
	blank := "  "
	require.NoError(t, DB.Create(&Channel{
		Type:    constant.ChannelTypeAIPDD,
		Name:    aipddEnvChannelName,
		BaseURL: &blank,
	}).Error)

	t.Setenv("AIPDD_BASE_URL", "https://env.aipdd.example/")
	require.Equal(t, "https://env.aipdd.example", ResolveAIPDDBaseURL())

	// With nothing configured anywhere the built-in default still answers, so an
	// installation that never touched the channel behaves exactly as before.
	t.Setenv("AIPDD_BASE_URL", "")
	require.Equal(t, constant.ChannelBaseURLs[constant.ChannelTypeAIPDD], ResolveAIPDDBaseURL())
}

// TestEnsureAIPDDDefaultsSyncsCatalogWithStoredChannelKey covers the production
// shape: the credential only exists on the AIPDD channel, the container has no
// AIPDD_API_KEY, and pricing must still sync on boot.
func TestEnsureAIPDDDefaultsSyncsCatalogWithStoredChannelKey(t *testing.T) {
	truncateTables(t)
	preserveAIPDDPricingRuntime(t)
	constant.ResetAIPDDCapabilities()
	constant.ResetAIPDDOpenAIModels()
	t.Cleanup(func() {
		constant.ResetAIPDDCapabilities()
		constant.ResetAIPDDOpenAIModels()
		aipddcatalog.ResetV1ModelsListHidden()
		aipddcatalog.ResetExplicitFreeModels()
	})

	const storedKey = "sk-stored-channel-key"
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/new-api/catalog" {
			http.NotFound(w, r)
			return
		}
		gotKey = r.Header.Get("X-API-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{
			"code": 0,
			"message": "fetched",
			"data": {
				"schemaVersion": 2,
				"revision": "stored-key-revision",
				"generatedAt": "2026-07-12T10:00:00",
				"awcoinRate": {"rmbPerAwcoin": 0.01, "usdPerAwcoin": 0.0015, "updatedAt": "2026-07-12T09:00:00"},
				"capabilities": [{
					"id": "stored-key-task",
					"code": "stored-key-task",
					"name": "Stored Key Task",
					"adapterCode": "comfyui",
					"endpointType": "openai-video",
					"taskKind": "image_to_video",
					"available": true,
					"execution": {"protocol": "shared_task", "path": "/shared-tasks/tasks"},
					"pricing": {"pricingModel": "per_call", "currency": "awcoin", "enabled": true, "chargeConfig": {"amountAwcoin": 500}}
				}],
				"models": []
			}
		}`)
	}))
	defer server.Close()

	t.Setenv("AIPDD_API_KEY", "")
	t.Setenv("AIPDD_BASE_URL", server.URL)
	t.Setenv("AIPDD_CATALOG_SYNC_ON_BOOT", "true")

	require.NoError(t, DB.Create(&Channel{
		Type:    constant.ChannelTypeAIPDD,
		Key:     storedKey,
		Name:    aipddEnvChannelName,
		Status:  common.ChannelStatusEnabled,
		Group:   "default",
		BaseURL: &server.URL,
	}).Error)

	require.NoError(t, EnsureAIPDDDefaults())

	require.Equal(t, storedKey, gotKey)
	require.Contains(t, constant.GetAIPDDModelList(), "stored-key-task")
}

func TestIsAIPDDCatalogSyncOnBootEnabledAcceptsAnyKeyFormat(t *testing.T) {
	t.Setenv("AIPDD_CATALOG_SYNC_ON_BOOT", "")

	// The key format belongs to AIPDD, not to this gateway: a configured key of
	// any shape must boot-sync, otherwise the instance runs on a stale snapshot.
	require.True(t, isAIPDDCatalogSyncOnBootEnabled("custom-format-key"))
	require.True(t, isAIPDDCatalogSyncOnBootEnabled(" sk-prefixed-key "))
	require.False(t, isAIPDDCatalogSyncOnBootEnabled("   "))

	// The environment flag still overrides in both directions.
	t.Setenv("AIPDD_CATALOG_SYNC_ON_BOOT", "false")
	require.False(t, isAIPDDCatalogSyncOnBootEnabled("sk-prefixed-key"))
	t.Setenv("AIPDD_CATALOG_SYNC_ON_BOOT", "true")
	require.True(t, isAIPDDCatalogSyncOnBootEnabled("custom-format-key"))
}
