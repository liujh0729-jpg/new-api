package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func withSeedanceProviderDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = "0123456789abcdef0123456789abcdef"
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&model.MediaEnhancementProvider{}, &model.SeedanceAdminAudit{}))
}

func saveSeedanceProviderRequest(t *testing.T, payload string) (*httptest.ResponseRecorder, seedanceProviderView) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 7)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/seedance-admin/providers", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	SaveSeedanceProvider(c)
	var response struct {
		Success bool                 `json:"success"`
		Message string               `json:"message"`
		Data    seedanceProviderView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	return recorder, response.Data
}

// The MediaKit host and service code are owned by the server so a crafted admin
// request cannot redirect a credentialed call or split cost aggregation.
func TestSaveMediaKitProviderPinsOfficialEndpointAndServiceCode(t *testing.T) {
	withSeedanceProviderDB(t)

	recorder, view := saveSeedanceProviderRequest(t, `{
		"adapter_type":"VOLCENGINE_MEDIAKIT",
		"display_name":"火山 AI MediaKit",
		"service_endpoint":"https://attacker.example/collect",
		"service_code":"attacker_code",
		"capabilities":"{\"cancel_supported\":true,\"submit_retry_safe\":true}",
		"mediakit_api_key":"mediakit-secret",
		"timeout_policy":"{\"timeout_seconds\":600}",
		"retry_policy":"{\"mode\":\"SAME_ATTEMPT_UNKNOWN_ONLY\"}",
		"fallback_policy":"{\"mode\":\"NONE\"}"
	}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, model.SeedanceProviderDirect, view.ProviderType)
	require.Equal(t, model.SeedanceAdapterVolcengineMediaKit, view.AdapterType)
	require.Equal(t, model.SeedanceMediaKitBaseURL, view.ServiceEndpoint)
	require.Equal(t, model.SeedanceMediaKitServiceCode, view.ServiceCode)
	// Capabilities come from the built-in adapter, never from administrator input.
	require.Equal(t, "{}", view.CapabilitiesJSON)
	require.True(t, view.CredentialConfigured)
	// A new MediaKit provider stays disabled until the administrator explicitly
	// enables it after validating the account entitlement and cost setup.
	require.Equal(t, model.SeedanceConfigDisabled, view.Status)

	// The response must expose only whether a credential exists.
	body := recorder.Body.String()
	require.NotContains(t, body, "mediakit-secret")
	require.NotContains(t, body, "enc:v")
	require.NotContains(t, body, "credential_encrypted")
}

func TestSaveMediaKitProviderRequiresAnAPIKeyOnCreate(t *testing.T) {
	withSeedanceProviderDB(t)

	recorder, _ := saveSeedanceProviderRequest(t, `{
		"adapter_type":"VOLCENGINE_MEDIAKIT","display_name":"火山 AI MediaKit"
	}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "mediakit_api_key is required")

	recorder, _ = saveSeedanceProviderRequest(t, `{
		"adapter_type":"VOLCENGINE_MEDIAKIT","display_name":"火山 AI MediaKit",
		"credential":"wrong-field"
	}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "use mediakit_api_key")
}

// Editing leaves the key field blank, which must mean "keep", while removal needs
// an explicit action that also stops the provider from taking new work.
func TestBlankCredentialKeepsTheKeyAndClearCredentialDisablesTheProvider(t *testing.T) {
	withSeedanceProviderDB(t)

	_, created := saveSeedanceProviderRequest(t, `{
		"adapter_type":"VOLCENGINE_MEDIAKIT","display_name":"火山 AI MediaKit",
		"mediakit_api_key":"mediakit-secret","status":"ACTIVE"
	}`)
	require.True(t, created.CredentialConfigured)
	require.Equal(t, model.SeedanceConfigActive, created.Status)
	var stored model.MediaEnhancementProvider
	require.NoError(t, model.DB.Where("id = ?", created.ID).First(&stored).Error)
	originalCiphertext := stored.CredentialEncrypted
	require.NotEmpty(t, originalCiphertext)

	_, kept := saveSeedanceProviderRequest(t, fmt.Sprintf(`{
		"id":%d,"adapter_type":"VOLCENGINE_MEDIAKIT","display_name":"火山 AI MediaKit","status":"ACTIVE"
	}`, created.ID))
	require.True(t, kept.CredentialConfigured)
	require.NoError(t, model.DB.Where("id = ?", created.ID).First(&stored).Error)
	require.Equal(t, originalCiphertext, stored.CredentialEncrypted)

	_, cleared := saveSeedanceProviderRequest(t, fmt.Sprintf(`{
		"id":%d,"adapter_type":"VOLCENGINE_MEDIAKIT","display_name":"火山 AI MediaKit",
		"status":"ACTIVE","clear_credential":true
	}`, created.ID))
	require.False(t, cleared.CredentialConfigured)
	require.Equal(t, model.SeedanceConfigDisabled, cleared.Status)
	require.NoError(t, model.DB.Where("id = ?", created.ID).First(&stored).Error)
	require.Empty(t, stored.CredentialEncrypted)
	require.Equal(t, model.SeedanceConfigDisabled, stored.Status)

	recorder, _ := saveSeedanceProviderRequest(t, fmt.Sprintf(`{
		"id":%d,"adapter_type":"VOLCENGINE_MEDIAKIT","display_name":"火山 AI MediaKit",
		"status":"ACTIVE"
	}`, created.ID))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "mediakit_api_key is required before enabling")

	var audits int64
	require.NoError(t, model.DB.Model(&model.SeedanceAdminAudit{}).
		Where("resource_type = ?", "ENHANCEMENT_PROVIDER").Count(&audits).Error)
	require.EqualValues(t, 3, audits)
}

func TestExistingProviderAdapterCannotBeChanged(t *testing.T) {
	withSeedanceProviderDB(t)

	_, created := saveSeedanceProviderRequest(t, `{
		"adapter_type":"GENERIC_HTTP","display_name":"private supplier",
		"service_endpoint":"https://supplier.example/tasks","service_code":"video_sr_v1",
		"credential":"supplier-secret"
	}`)
	recorder, _ := saveSeedanceProviderRequest(t, fmt.Sprintf(`{
		"id":%d,"adapter_type":"VOLCENGINE_MEDIAKIT","display_name":"火山 AI MediaKit",
		"mediakit_api_key":"mediakit-secret"
	}`, created.ID))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "adapter_type cannot be changed")
}

func TestSaveGenericProviderKeepsTheExistingContract(t *testing.T) {
	withSeedanceProviderDB(t)

	recorder, view := saveSeedanceProviderRequest(t, `{
		"adapter_type":"GENERIC_HTTP","display_name":"private supplier",
		"service_endpoint":"https://supplier.example/tasks","service_code":"video_sr_v1",
		"credential":"supplier-secret","capabilities":"{\"cancel\":true}",
		"timeout_policy":"{\"timeout_seconds\":600}",
		"retry_policy":"{\"mode\":\"SAME_ATTEMPT_UNKNOWN_ONLY\"}",
		"fallback_policy":"{\"mode\":\"NONE\"}"
	}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, model.SeedanceAdapterGenericHTTP, view.AdapterType)
	require.Equal(t, "https://supplier.example/tasks", view.ServiceEndpoint)
	require.Equal(t, "video_sr_v1", view.ServiceCode)
	require.Equal(t, `{"cancel":true}`, view.CapabilitiesJSON)
	require.True(t, view.CredentialConfigured)
	require.NotContains(t, recorder.Body.String(), "supplier-secret")

	// A request with no adapter_type is a pre-existing client and must keep the
	// generic protocol rather than silently changing wire format.
	recorder, legacy := saveSeedanceProviderRequest(t, `{
		"provider_type":"DIRECT_EXTERNAL","display_name":"legacy supplier",
		"service_endpoint":"https://legacy.example/tasks","service_code":"video_sr_v0"
	}`)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, model.SeedanceAdapterGenericHTTP, legacy.AdapterType)
}

func TestSaveGenericProviderStillRequiresHTTPSAndAServiceCode(t *testing.T) {
	withSeedanceProviderDB(t)

	recorder, _ := saveSeedanceProviderRequest(t, `{
		"adapter_type":"GENERIC_HTTP","display_name":"private supplier",
		"service_endpoint":"http://supplier.example/tasks","service_code":"video_sr_v1"
	}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "must use HTTPS")

	recorder, _ = saveSeedanceProviderRequest(t, `{
		"adapter_type":"GENERIC_HTTP","display_name":"private supplier",
		"service_endpoint":"https://supplier.example/tasks"
	}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "service_code is required")

	// The MediaKit key must not be usable as a second credential channel for a
	// custom remote service.
	recorder, _ = saveSeedanceProviderRequest(t, `{
		"adapter_type":"GENERIC_HTTP","display_name":"private supplier",
		"service_endpoint":"https://supplier.example/tasks","service_code":"video_sr_v1",
		"mediakit_api_key":"mediakit-secret"
	}`)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "only applies to the VOLCENGINE_MEDIAKIT adapter")
}

func TestSaveProviderRejectsUnsupportedTypeCombinations(t *testing.T) {
	withSeedanceProviderDB(t)

	for _, payload := range []string{
		`{"provider_type":"AIPDD_INTERNAL","adapter_type":"AIPDD_SUPER_RESOLUTION","display_name":"future"}`,
		`{"provider_type":"DIRECT_EXTERNAL","adapter_type":"AIPDD_SUPER_RESOLUTION","display_name":"future"}`,
		`{"provider_type":"DIRECT_EXTERNAL","adapter_type":"SOMETHING_ELSE","display_name":"future"}`,
	} {
		recorder, _ := saveSeedanceProviderRequest(t, payload)
		require.Equal(t, http.StatusBadRequest, recorder.Code, payload)
	}
}

func TestListSeedanceProvidersNeverSerializesTheStoredSecret(t *testing.T) {
	withSeedanceProviderDB(t)
	ciphertext, err := common.EncryptSensitiveValue("mediakit-secret")
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, AdapterType: model.SeedanceAdapterVolcengineMediaKit,
		DisplayName: "火山 AI MediaKit", ServiceEndpoint: model.SeedanceMediaKitBaseURL,
		ServiceCode: model.SeedanceMediaKitServiceCode, CredentialEncrypted: ciphertext,
		Status: model.SeedanceConfigDisabled, Version: 1,
	}).Error)
	// A row written before adapter_type existed must read back as generic.
	require.NoError(t, model.DB.Create(&model.MediaEnhancementProvider{
		ProviderType: model.SeedanceProviderDirect, DisplayName: "legacy supplier",
		ServiceEndpoint: "https://legacy.example/tasks", ServiceCode: "video_sr_v0",
		Status: model.SeedanceConfigActive, Version: 1,
	}).Error)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/seedance-admin/providers", nil)
	ListSeedanceProviders(c)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Data []seedanceProviderView `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
	require.Equal(t, model.SeedanceAdapterVolcengineMediaKit, response.Data[0].AdapterType)
	require.True(t, response.Data[0].CredentialConfigured)
	require.Equal(t, model.SeedanceAdapterGenericHTTP, response.Data[1].AdapterType)
	require.False(t, response.Data[1].CredentialConfigured)
	body := recorder.Body.String()
	require.NotContains(t, body, "mediakit-secret")
	require.NotContains(t, body, ciphertext)
}
