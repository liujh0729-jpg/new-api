package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestSeedanceModelsNeverSerializeEncryptedSecrets(t *testing.T) {
	fixtures := []any{
		&SeedanceChannelConfig{AIPDDBillingCredentialEncrypted: "billing-secret-ciphertext"},
		&SeedanceVolcengineCredential{
			ArkAPIKeyEncrypted: "ark-secret-ciphertext", AccessKeyIDEncrypted: "ak-secret-ciphertext",
			SecretAccessKeyEncrypted: "sk-secret-ciphertext",
		},
		&MediaEnhancementProvider{CredentialEncrypted: "provider-secret-ciphertext"},
		&SeedanceOrder{
			AIPDDBillingBaseURLSnapshot:             "https://private-billing.example",
			AIPDDBillingCredentialSnapshotEncrypted: "billing-snapshot-secret-ciphertext",
			EnhancementProviderSnapshotEncrypted:    "snapshot-secret-ciphertext",
			CallbackURLEncrypted:                    "callback-secret-ciphertext",
			CallbackLeaseOwner:                      "private-lease-owner",
		},
	}

	for _, fixture := range fixtures {
		body, err := json.Marshal(fixture)
		require.NoError(t, err)
		serialized := strings.ToLower(string(body))
		for _, forbidden := range []string{
			"secret-ciphertext", "private-lease-owner", "encrypted", "credential_encrypted",
			"ark_api_key", "access_key_id", "secret_access_key", "callback_url", "private-billing",
		} {
			require.NotContains(t, serialized, forbidden)
		}
	}
}

func TestActivateSeedanceCredentialCannotBreakEnabledBillSync(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open("file:seedance-credential-bill-sync?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(
		&SeedanceChannelConfig{}, &SeedanceVolcengineCredential{}, &SeedanceAdminAudit{}))
	require.NoError(t, db.Create(&SeedanceChannelConfig{
		ChannelID: 901, VolcengineBillSyncEnabled: true, VolcengineBillProductCodesJSON: `["verified-product"]`,
	}).Error)
	oldCredential := &SeedanceVolcengineCredential{
		ChannelID: 901, Version: 1, ArkAPIKeyEncrypted: "old-bill-sync-ciphertext",
		Status: SeedanceCredentialActive, BillingValidatedAt: 100,
	}
	newCredential := &SeedanceVolcengineCredential{
		ChannelID: 901, Version: 2, ArkAPIKeyEncrypted: "new-bill-sync-ciphertext",
		Status: SeedanceCredentialPending,
	}
	require.NoError(t, db.Create(oldCredential).Error)
	require.NoError(t, db.Create(newCredential).Error)

	err = ActivateSeedanceCredential(newCredential.ID, 1, false)

	require.ErrorContains(t, err, "ListBillDetail validation")
	require.NoError(t, db.First(oldCredential, oldCredential.ID).Error)
	require.NoError(t, db.First(newCredential, newCredential.ID).Error)
	require.Equal(t, SeedanceCredentialActive, oldCredential.Status)
	require.Equal(t, SeedanceCredentialPending, newCredential.Status)
}

func TestSeedanceAuditRecordsNonSecretBeforeAndAfterVersions(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open("file:seedance-audit-versions?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(
		&SeedanceChannelConfig{}, &SeedanceVolcengineCredential{}, &SeedanceAdminAudit{}))

	config := &SeedanceChannelConfig{ChannelID: 902, InstanceID: "instance-1", Status: SeedanceConfigActive}
	require.NoError(t, SaveSeedanceChannelConfig(config, 7, "created configuration"))
	config.Status = SeedanceConfigDisabled
	require.NoError(t, SaveSeedanceChannelConfig(config, 7, "disabled configuration"))
	require.Equal(t, 2, config.Revision)

	var configAudits []SeedanceAdminAudit
	require.NoError(t, db.Where("resource_type = ?", "CHANNEL_CONFIG").Order("id asc").Find(&configAudits).Error)
	require.Len(t, configAudits, 2)
	require.Equal(t, "", configAudits[0].BeforeVersion)
	require.Equal(t, "1", configAudits[0].AfterVersion)
	require.Equal(t, "1", configAudits[1].BeforeVersion)
	require.Equal(t, "2", configAudits[1].AfterVersion)

	oldCredential := &SeedanceVolcengineCredential{
		ChannelID: 902, Version: 1, ArkAPIKeyEncrypted: "old-audit-ciphertext",
		Status: SeedanceCredentialActive, ValidatedAt: 100,
	}
	require.NoError(t, db.Create(oldCredential).Error)
	newCredential := &SeedanceVolcengineCredential{ChannelID: 902, ArkAPIKeyEncrypted: "new-audit-ciphertext"}
	require.NoError(t, CreateSeedanceCredential(newCredential, 7))
	require.Equal(t, 2, newCredential.Version)
	require.NoError(t, ActivateSeedanceCredential(newCredential.ID, 7, false))

	var activation SeedanceAdminAudit
	require.NoError(t, db.Where("resource_type = ? AND action = ?", "VOLCENGINE_CREDENTIAL", "VALIDATE_AND_ACTIVATE").First(&activation).Error)
	require.Equal(t, "1", activation.BeforeVersion)
	require.Equal(t, "2", activation.AfterVersion)
	require.NotContains(t, activation.ChangeSummary, "secret")
}

func TestRetiringSeedanceCredentialKeepsSecretsUntilPinnedOrdersTerminate(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open("file:seedance-credential-retirement?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(
		&SeedanceChannelConfig{}, &SeedanceVolcengineCredential{}, &SeedanceOrder{}, &SeedanceAdminAudit{}))

	now := time.Now().Unix()
	oldCredential := &SeedanceVolcengineCredential{
		ChannelID: 903, Version: 1, ArkAPIKeyEncrypted: "old-ark-ciphertext",
		AccessKeyIDEncrypted: "old-ak-ciphertext", SecretAccessKeyEncrypted: "old-sk-ciphertext",
		Fingerprint: "sha256:old", MaskedSuffix: "old1", Status: SeedanceCredentialActive, CreatedAt: now,
	}
	newCredential := &SeedanceVolcengineCredential{
		ChannelID: 903, Version: 2, ArkAPIKeyEncrypted: "new-ark-ciphertext",
		AccessKeyIDEncrypted: "new-ak-ciphertext", SecretAccessKeyEncrypted: "new-sk-ciphertext",
		Fingerprint: "sha256:new", MaskedSuffix: "new2", Status: SeedanceCredentialPending, CreatedAt: now,
	}
	require.NoError(t, db.Create(oldCredential).Error)
	require.NoError(t, db.Create(newCredential).Error)
	require.NoError(t, ActivateSeedanceCredential(newCredential.ID, 9, false))

	require.NoError(t, db.First(oldCredential, oldCredential.ID).Error)
	require.Equal(t, SeedanceCredentialRetiring, oldCredential.Status)
	require.GreaterOrEqual(t, oldCredential.RetireAfter, now+int64(seedanceCredentialRetirementGrace/time.Second)-1)
	require.Equal(t, "old-ark-ciphertext", oldCredential.ArkAPIKeyEncrypted)

	// Force the grace window to have elapsed while a task is still using the
	// old credential. The key material must remain available for polling and
	// cancellation of that historical task.
	require.NoError(t, db.Model(&SeedanceVolcengineCredential{}).Where("id = ?", oldCredential.ID).
		Update("retire_after", now-1).Error)
	order := &SeedanceOrder{
		PlatformOrderID: "order-retiring-credential", NewAPITaskID: "task-retiring-credential",
		ChannelID: 903, InstanceID: "instance-retiring-credential",
		VolcengineCredentialID: oldCredential.ID, CredentialVersion: oldCredential.Version,
		Model: "Public video", OrderStatus: SeedanceOrderGenerationProcessing,
		VolcengineCostStatus: SeedanceCostEstimated, SyncStatus: SeedanceSyncPending,
		PricingSnapshotJSON: `{}`, PricingSnapshotHash: SHA256Evidence(`{}`),
		PublicProtocol: SeedanceProtocolOfficial, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, db.Create(order).Error)

	retired, err := RetireUnusedSeedanceCredentials(20)
	require.NoError(t, err)
	require.Zero(t, retired)
	require.NoError(t, db.First(oldCredential, oldCredential.ID).Error)
	require.Equal(t, SeedanceCredentialRetiring, oldCredential.Status)
	require.Equal(t, "old-ark-ciphertext", oldCredential.ArkAPIKeyEncrypted)

	require.NoError(t, db.Model(&SeedanceOrder{}).Where("id = ?", order.ID).
		Update("order_status", SeedanceOrderSucceeded).Error)
	retired, err = RetireUnusedSeedanceCredentials(20)
	require.NoError(t, err)
	require.Equal(t, 1, retired)
	require.NoError(t, db.First(oldCredential, oldCredential.ID).Error)
	require.Equal(t, SeedanceCredentialRetired, oldCredential.Status)
	require.Empty(t, oldCredential.ArkAPIKeyEncrypted)
	require.Empty(t, oldCredential.AccessKeyIDEncrypted)
	require.Empty(t, oldCredential.SecretAccessKeyEncrypted)
	require.Equal(t, "sha256:old", oldCredential.Fingerprint)
	require.Equal(t, "old1", oldCredential.MaskedSuffix)

	retired, err = RetireUnusedSeedanceCredentials(20)
	require.NoError(t, err)
	require.Zero(t, retired)
	var audits int64
	require.NoError(t, db.Model(&SeedanceAdminAudit{}).
		Where("resource_type = ? AND resource_id = ? AND action = ?", "VOLCENGINE_CREDENTIAL", fmtInt64(oldCredential.ID), "AUTO_RETIRE").
		Count(&audits).Error)
	require.EqualValues(t, 1, audits)

	err = ActivateSeedanceCredential(oldCredential.ID, 9, false)
	require.ErrorContains(t, err, "retired Seedance credentials")
	require.NoError(t, db.First(newCredential, newCredential.ID).Error)
	require.Equal(t, SeedanceCredentialActive, newCredential.Status)
}

func TestInsertSeedanceOrderAcceptsRetiringCredentialButRejectsDestroyedCredential(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open("file:seedance-order-credential-state?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
	})
	require.NoError(t, db.AutoMigrate(&Task{}, &SeedanceVolcengineCredential{}, &SeedanceOrder{}, &SeedanceAttempt{}))

	previousSecret, previousConfigured := common.CryptoSecret, common.CryptoSecretConfigured
	common.CryptoSecret = strings.Repeat("o", 32)
	common.CryptoSecretConfigured = true
	t.Cleanup(func() {
		common.CryptoSecret = previousSecret
		common.CryptoSecretConfigured = previousConfigured
	})

	retiring := &SeedanceVolcengineCredential{
		ChannelID: 904, Version: 1, ArkAPIKeyEncrypted: "retiring-key-ciphertext",
		Fingerprint: "sha256:retiring", MaskedSuffix: "ring", Status: SeedanceCredentialRetiring,
	}
	retired := &SeedanceVolcengineCredential{
		ChannelID: 904, Version: 2, ArkAPIKeyEncrypted: "",
		Fingerprint: "sha256:retired", MaskedSuffix: "ired", Status: SeedanceCredentialRetired,
	}
	require.NoError(t, db.Create(retiring).Error)
	require.NoError(t, db.Create(retired).Error)
	config := &SeedanceChannelConfig{ChannelID: 904, InstanceID: "instance-order-credential-state", Revision: 1}
	provider := &MediaEnhancementProvider{
		ID: 1, ProviderType: SeedanceProviderDirect, AdapterType: SeedanceAdapterGenericHTTP,
		DisplayName: "private", ServiceEndpoint: "https://provider.example.test", ServiceCode: "private-service",
	}
	offering := &SeedanceModelOffering{
		ChannelID: 904, DisplayName: "Public video", ProviderModelID: "private-model",
		ModelSaleMicroRMB: 5_000_000, ServiceChargeMicroRMB: 1_000_000,
		VolcengineUnitCostMicroRMB: 2_000_000, PricingVersion: "price-v1",
	}
	newTask := func(id string) *Task {
		return &Task{TaskID: id, UserId: 1, ChannelId: 904, Properties: Properties{OriginModelName: "Public video"}}
	}

	created, err := InsertTaskWithSeedanceOrder(SeedanceOrderCreate{
		Task: newTask("task-retiring-credential-order"), Config: config, Credential: retiring,
		Offering: offering, Provider: provider, RequestFactsJSON: `{}`, PricingSnapshot: `{}`,
	})
	require.NoError(t, err)
	require.Equal(t, retiring.ID, created.VolcengineCredentialID)
	require.Equal(t, retiring.Version, created.CredentialVersion)

	_, err = InsertTaskWithSeedanceOrder(SeedanceOrderCreate{
		Task: newTask("task-retired-credential-order"), Config: config, Credential: retired,
		Offering: offering, Provider: provider, RequestFactsJSON: `{}`, PricingSnapshot: `{}`,
	})
	require.ErrorContains(t, err, "not available")
	var rejectedTasks int64
	require.NoError(t, db.Model(&Task{}).Where("task_id = ?", "task-retired-credential-order").Count(&rejectedTasks).Error)
	require.Zero(t, rejectedTasks, "a task must not be committed after its credential key material was destroyed")
}
