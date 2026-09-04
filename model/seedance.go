package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrSeedanceChannelUnavailable = errors.New("Seedance channel is not active")
	ErrSeedanceInvalidPricing     = errors.New("Seedance pricing is not available")
	ErrSeedanceDeadLetterRevision = errors.New("Seedance dead-letter events require a higher revision")
)

const (
	seedancePublicVendorName       = "字节跳动（火山引擎）"
	seedancePublicVendorIcon       = "Doubao.Color"
	seedancePublicVendorWebsite    = "https://www.volcengine.com/product/ark"
	seedancePublicModelDescription = "Seedance 视频生成模型，支持通过异步任务生成并获取视频结果。"
	seedancePublicModelTags        = "Seedance,视频生成,异步任务"

	SeedanceConfigActive   = "ACTIVE"
	SeedanceConfigDisabled = "DISABLED"

	SeedanceCredentialActive   = "ACTIVE"
	SeedanceCredentialPending  = "PENDING"
	SeedanceCredentialRetiring = "RETIRING"
	SeedanceCredentialRetired  = "RETIRED"

	SeedanceProviderDirect = "DIRECT_EXTERNAL"
	SeedanceProviderAIPDD  = "AIPDD_INTERNAL"

	// adapter_type carries the wire protocol only. provider_type keeps meaning
	// execution and cost attribution, which the AIPDD finance API constrains to
	// its own enum, so the two dimensions must not be merged into one field.
	SeedanceAdapterGenericHTTP          = "GENERIC_HTTP"
	SeedanceAdapterVolcengineMediaKit   = "VOLCENGINE_MEDIAKIT"
	SeedanceAdapterAIPDDSuperResolution = "AIPDD_SUPER_RESOLUTION"

	// The official AI MediaKit host and service code are owned by the server so
	// a dirty database row or a crafted admin request cannot redirect a
	// credentialed request to an attacker-controlled endpoint.
	SeedanceMediaKitBaseURL     = "https://mediakit.cn-beijing.volces.com"
	SeedanceMediaKitServiceCode = "volcengine_ai_mediakit_quality_enhance"

	SeedanceOrderReceived             = "RECEIVED"
	SeedanceOrderGenerationSubmitting = "GENERATION_SUBMITTING"
	SeedanceOrderGenerationProcessing = "GENERATION_PROCESSING"
	SeedanceOrderEnhancing            = "ENHANCING"
	SeedanceOrderSucceeded            = "SUCCEEDED"
	SeedanceOrderFailed               = "FAILED"
	SeedanceOrderCancelled            = "CANCELLED"

	SeedanceUsagePending   = "PENDING"
	SeedanceUsageRunning   = "RUNNING"
	SeedanceUsageSucceeded = "SUCCEEDED"
	SeedanceUsageFailed    = "FAILED"

	SeedanceSubmissionOutcomeUnknown = "SUBMISSION_OUTCOME_UNKNOWN"

	SeedanceSyncPending    = "PENDING"
	SeedanceSyncReady      = "READY"
	SeedanceSyncSending    = "SENDING"
	SeedanceSyncSynced     = "SYNCED"
	SeedanceSyncRetryWait  = "RETRY_WAIT"
	SeedanceSyncDeadLetter = "DEAD_LETTER"
	SeedanceSyncAuthPaused = "AUTH_PAUSED"

	SeedanceCallbackNone       = "NONE"
	SeedanceCallbackWaiting    = "WAITING"
	SeedanceCallbackReady      = "READY"
	SeedanceCallbackSending    = "SENDING"
	SeedanceCallbackDelivered  = "DELIVERED"
	SeedanceCallbackRetryWait  = "RETRY_WAIT"
	SeedanceCallbackDeadLetter = "DEAD_LETTER"

	SeedanceProtocolOfficial = "SEEDANCE_OFFICIAL"
	SeedanceProtocolOpenAI   = "OPENAI_VIDEO"

	SeedanceCostEstimated              = "ESTIMATED"
	SeedanceCostPartial                = "PARTIAL"
	SeedanceCostConfirmed              = "CONFIRMED"
	SeedanceCostReconciliationRequired = "RECONCILIATION_REQUIRED"

	SeedanceServiceTypeVideoSuperResolution = "VIDEO_SUPER_RESOLUTION"

	// A request resolves the active credential shortly before its order
	// transaction. Keeping the retired version for a short grace period closes
	// that hand-off window; non-terminal orders remain the authoritative gate
	// after the grace period.
	seedanceCredentialRetirementGrace = time.Hour
)

// SeedanceChannelConfig stores the independent channel's non-public workflow
// configuration. Secrets are encrypted separately and are never serialized.
type SeedanceChannelConfig struct {
	ChannelID                            int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	Revision                             int    `json:"revision" gorm:"not null;default:1"`
	InstanceID                           string `json:"instance_id" gorm:"type:varchar(64);not null;index"`
	AIPDDBillingBaseURL                  string `json:"aipdd_billing_base_url,omitempty" gorm:"type:varchar(512)"`
	AIPDDBillingCredentialEncrypted      string `json:"-" gorm:"type:text"`
	BillingAuthPausedAt                  int64  `json:"billing_auth_paused_at,omitempty" gorm:"index"`
	BillingAuthLastHTTPStatus            int    `json:"billing_auth_last_http_status,omitempty"`
	VolcengineBillSyncEnabled            bool   `json:"volcengine_bill_sync_enabled" gorm:"not null;default:false"`
	VolcengineBillProductCodesJSON       string `json:"volcengine_bill_product_codes" gorm:"column:volcengine_bill_product_codes;type:text"`
	VolcengineBillConfigurationCodesJSON string `json:"volcengine_bill_configuration_codes" gorm:"column:volcengine_bill_configuration_codes;type:text"`
	DefaultEnhancementProviderID         *int64 `json:"default_enhancement_provider_id,omitempty" gorm:"index"`
	Status                               string `json:"status" gorm:"type:varchar(32);not null;index"`
	LastVerifiedAt                       int64  `json:"last_verified_at,omitempty"`
	CreatedAt                            int64  `json:"created_at"`
	UpdatedAt                            int64  `json:"updated_at"`
}

type SeedanceVolcengineCredential struct {
	ID                       int64  `json:"id" gorm:"primaryKey"`
	ChannelID                int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_seedance_credential_version;index"`
	Version                  int    `json:"version" gorm:"not null;uniqueIndex:idx_seedance_credential_version"`
	ArkAPIKeyEncrypted       string `json:"-" gorm:"type:text;not null"`
	AccessKeyIDEncrypted     string `json:"-" gorm:"type:text"`
	SecretAccessKeyEncrypted string `json:"-" gorm:"type:text"`
	Fingerprint              string `json:"fingerprint" gorm:"type:varchar(128);not null"`
	MaskedSuffix             string `json:"masked_suffix" gorm:"type:varchar(16);not null"`
	Status                   string `json:"status" gorm:"type:varchar(32);not null;index"`
	ValidatedAt              int64  `json:"validated_at,omitempty"`
	BillingValidatedAt       int64  `json:"billing_validated_at,omitempty"`
	RetireAfter              int64  `json:"retire_after,omitempty"`
	CreatedBy                int    `json:"created_by"`
	CreatedAt                int64  `json:"created_at"`
}

type MediaEnhancementProvider struct {
	ID                  int64  `json:"id" gorm:"primaryKey"`
	Version             int    `json:"version" gorm:"not null;default:1"`
	ProviderType        string `json:"provider_type" gorm:"type:varchar(32);not null;index"`
	AdapterType         string `json:"adapter_type" gorm:"type:varchar(32);not null;default:'GENERIC_HTTP';index"`
	DisplayName         string `json:"display_name" gorm:"type:varchar(191);not null"`
	ServiceEndpoint     string `json:"service_endpoint" gorm:"type:varchar(1024);not null"`
	CredentialEncrypted string `json:"-" gorm:"type:text"`
	ServiceCode         string `json:"service_code" gorm:"type:varchar(128);not null"`
	CapabilitiesJSON    string `json:"capabilities" gorm:"column:capabilities;type:text"`
	Status              string `json:"status" gorm:"type:varchar(32);not null;index"`
	TimeoutPolicyJSON   string `json:"timeout_policy" gorm:"column:timeout_policy;type:text"`
	RetryPolicyJSON     string `json:"retry_policy" gorm:"column:retry_policy;type:text"`
	FallbackPolicyJSON  string `json:"fallback_policy" gorm:"column:fallback_policy;type:text"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`

	// ClearCredential distinguishes "keep the stored secret" from "erase it".
	// An empty credential keeps the previous value, so removal needs its own
	// explicit signal rather than an ambiguous blank form field.
	ClearCredential bool `json:"-" gorm:"-"`
}

// NormalizeSeedanceAdapterType resolves and validates the adapter for a provider
// type. Only the combinations enabled in this phase are accepted; everything
// else is rejected instead of silently falling back to a wire protocol the
// operator did not choose.
func NormalizeSeedanceAdapterType(providerType string, adapterType string) (string, error) {
	provider := strings.ToUpper(strings.TrimSpace(providerType))
	adapter := strings.ToUpper(strings.TrimSpace(adapterType))
	if adapter == "" {
		adapter = LegacySeedanceAdapterType(provider)
	}
	if provider == SeedanceProviderDirect &&
		(adapter == SeedanceAdapterGenericHTTP || adapter == SeedanceAdapterVolcengineMediaKit) {
		return adapter, nil
	}
	return "", fmt.Errorf("unsupported provider_type/adapter_type combination %s/%s", provider, adapter)
}

// LegacySeedanceAdapterType reads provider rows and order snapshots written
// before adapter_type existed. Every historical provider spoke the generic
// protocol, so the mapping is unambiguous.
func LegacySeedanceAdapterType(providerType string) string {
	if strings.ToUpper(strings.TrimSpace(providerType)) == SeedanceProviderAIPDD {
		return SeedanceAdapterAIPDDSuperResolution
	}
	return SeedanceAdapterGenericHTTP
}

// BackfillSeedanceProviderAdapterType gives rows created before the column
// existed the generic adapter they were actually configured against.
func BackfillSeedanceProviderAdapterType() error {
	if !DB.Migrator().HasColumn(&MediaEnhancementProvider{}, "adapter_type") {
		return nil
	}
	return DB.Model(&MediaEnhancementProvider{}).
		Where("adapter_type IS NULL OR adapter_type = ?", "").
		Update("adapter_type", SeedanceAdapterGenericHTTP).Error
}

// BackfillSeedanceOrderFinanceRevision gives pre-upgrade terminal orders a
// source-order revision independent from each service line's revision. The
// maximum existing line revision preserves compatibility with finance events
// that may already have reached AIPDD before this column existed.
func BackfillSeedanceOrderFinanceRevision() error {
	if !DB.Migrator().HasColumn(&SeedanceOrder{}, "finance_revision") {
		return nil
	}
	var orders []SeedanceOrder
	if err := DB.Select("id", "platform_order_id").
		Where("finance_revision = 0 AND order_status IN ?", []string{
			SeedanceOrderSucceeded, SeedanceOrderFailed, SeedanceOrderCancelled,
		}).Find(&orders).Error; err != nil {
		return err
	}
	for i := range orders {
		var maxRevision int
		if err := DB.Model(&MediaServiceUsage{}).
			Where("platform_order_id = ?", orders[i].PlatformOrderID).
			Select("COALESCE(MAX(revision), 1)").Scan(&maxRevision).Error; err != nil {
			return err
		}
		if maxRevision < 1 {
			maxRevision = 1
		}
		if err := DB.Model(&SeedanceOrder{}).
			Where("id = ? AND finance_revision = 0", orders[i].ID).
			Update("finance_revision", maxRevision).Error; err != nil {
			return err
		}
	}
	return nil
}

type SeedanceModelOffering struct {
	ID                              int64  `json:"id" gorm:"primaryKey"`
	ChannelID                       int    `json:"channel_id" gorm:"not null;index;uniqueIndex:idx_seedance_offering_model"`
	DisplayName                     string `json:"display_name" gorm:"type:varchar(191);not null;uniqueIndex:idx_seedance_offering_model"`
	BaseModelID                     int64  `json:"base_model_id" gorm:"index"`
	EnhancementModelID              *int64 `json:"enhancement_model_id,omitempty" gorm:"index"`
	SourceResolution                string `json:"source_resolution" gorm:"type:varchar(32);index"`
	TargetResolution                string `json:"target_resolution" gorm:"type:varchar(32);index"`
	OutputFPS                       int    `json:"output_fps" gorm:"not null;default:24"`
	NoReferenceUnitPriceMicroRMB    int64  `json:"no_reference_unit_price_micro_rmb" gorm:"not null;default:0"`
	ReferenceUnitPriceMicroRMB      int64  `json:"reference_unit_price_micro_rmb" gorm:"not null;default:0"`
	MigrationNeedsReview            bool   `json:"migration_needs_review" gorm:"not null;default:false;index"`
	ArchivedAt                      int64  `json:"archived_at,omitempty" gorm:"index"`
	ProviderModelID                 string `json:"provider_model_id" gorm:"type:varchar(191);not null"`
	ResolutionRulesJSON             string `json:"resolution_rules" gorm:"column:resolution_rules;type:text"`
	DurationRulesJSON               string `json:"duration_rules" gorm:"column:duration_rules;type:text"`
	EnhancementProviderID           int64  `json:"enhancement_provider_id" gorm:"not null;index"`
	EnhancementServiceCode          string `json:"enhancement_service_code" gorm:"type:varchar(128);not null"`
	EnhancementSpecificationJSON    string `json:"enhancement_specification" gorm:"column:enhancement_specification;type:text"`
	EnhancementSpecificationVersion string `json:"enhancement_specification_version" gorm:"type:varchar(64);not null"`
	ModelSaleMicroRMB               int64  `json:"model_sale_micro_rmb" gorm:"not null"`
	ServiceChargeMicroRMB           int64  `json:"service_charge_micro_rmb" gorm:"not null"`
	ProviderCostMicroRMB            *int64 `json:"provider_cost_micro_rmb,omitempty"`
	VolcengineUnitCostMicroRMB      int64  `json:"volcengine_unit_cost_micro_rmb" gorm:"not null"`
	PricingVersion                  string `json:"pricing_version" gorm:"type:varchar(64);not null"`
	Enabled                         bool   `json:"enabled" gorm:"not null;index"`
	PublishedAt                     int64  `json:"published_at,omitempty"`
	CreatedAt                       int64  `json:"created_at"`
	UpdatedAt                       int64  `json:"updated_at"`
}

type SeedanceOrder struct {
	ID                                      int64  `json:"id" gorm:"primaryKey"`
	PlatformOrderID                         string `json:"platform_order_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	NewAPITaskID                            string `json:"newapi_task_id" gorm:"type:varchar(191);not null;uniqueIndex"`
	NewAPIUserID                            int    `json:"newapi_user_id" gorm:"not null;index"`
	TokenID                                 int    `json:"token_id" gorm:"index"`
	ChannelID                               int    `json:"channel_id" gorm:"not null;index"`
	InstanceID                              string `json:"instance_id" gorm:"type:varchar(64);not null;index"`
	AIPDDBillingConfigRevision              int    `json:"-" gorm:"column:aipdd_billing_config_revision;not null;default:0;index"`
	AIPDDBillingBaseURLSnapshot             string `json:"-" gorm:"column:aipdd_billing_base_url_snapshot;type:varchar(512)"`
	AIPDDBillingCredentialSnapshotEncrypted string `json:"-" gorm:"column:aipdd_billing_credential_snapshot;type:text"`
	VolcengineCredentialID                  int64  `json:"volcengine_credential_id" gorm:"not null"`
	CredentialVersion                       int    `json:"credential_version" gorm:"not null"`
	Model                                   string `json:"model" gorm:"type:varchar(191);not null;index"`
	OfferingID                              int64  `json:"offering_id" gorm:"index"`
	BaseModelID                             int64  `json:"base_model_id" gorm:"index"`
	EnhancementModelID                      *int64 `json:"enhancement_model_id,omitempty" gorm:"index"`
	SourceResolution                        string `json:"source_resolution" gorm:"type:varchar(32);index"`
	TargetResolution                        string `json:"target_resolution" gorm:"type:varchar(32);index"`
	OutputFPS                               int    `json:"output_fps" gorm:"not null;default:24"`
	HasReferenceVideo                       bool   `json:"has_reference_video" gorm:"not null;default:false;index"`
	RequestedDurationMillis                 int64  `json:"requested_duration_millis" gorm:"not null;default:0"`
	ActualDurationMillis                    int64  `json:"actual_duration_millis" gorm:"not null;default:0"`
	SaleUnitPriceMicroRMB                   int64  `json:"sale_unit_price_micro_rmb" gorm:"not null;default:0"`
	SuperResolutionUnitCostMicroRMB         int64  `json:"super_resolution_unit_cost_micro_rmb" gorm:"not null;default:0"`
	SuperResolutionCostMicroRMB             int64  `json:"super_resolution_cost_micro_rmb" gorm:"not null;default:0"`
	RequestFactsJSON                        string `json:"request_facts" gorm:"column:request_facts;type:text"`
	GenerationPublicUsageJSON               string `json:"-" gorm:"column:generation_public_usage;type:text"`
	OrderStatus                             string `json:"order_status" gorm:"type:varchar(32);not null;index"`
	VolcengineCostStatus                    string `json:"volcengine_cost_status" gorm:"type:varchar(32);not null;index"`
	SyncStatus                              string `json:"sync_status" gorm:"type:varchar(32);not null;index"`
	FinanceRevision                         int    `json:"-" gorm:"column:finance_revision;not null;default:0"`
	ModelSaleMicroRMB                       int64  `json:"model_sale_micro_rmb" gorm:"not null"`
	ServiceChargeTotalMicroRMB              int64  `json:"service_charge_total_micro_rmb" gorm:"not null"`
	VolcengineEstimatedMicroRMB             int64  `json:"volcengine_estimated_micro_rmb" gorm:"not null"`
	VolcengineActualMicroRMB                *int64 `json:"volcengine_actual_micro_rmb,omitempty"`
	NewAPIEstimatedProfitMicroRMB           int64  `json:"newapi_estimated_profit_micro_rmb" gorm:"not null"`
	NewAPIActualProfitMicroRMB              *int64 `json:"newapi_actual_profit_micro_rmb,omitempty"`
	PricingSnapshotJSON                     string `json:"pricing_snapshot" gorm:"column:pricing_snapshot;type:text;not null"`
	PricingSnapshotHash                     string `json:"pricing_snapshot_hash" gorm:"type:varchar(80);not null"`
	EnhancementProviderSnapshotEncrypted    string `json:"-" gorm:"column:enhancement_provider_snapshot;type:text"`
	PublicProtocol                          string `json:"public_protocol" gorm:"type:varchar(32);not null"`
	CallbackURLEncrypted                    string `json:"-" gorm:"type:text"`
	CallbackStatus                          string `json:"callback_status" gorm:"type:varchar(32);not null;index"`
	CallbackAttemptCount                    int    `json:"callback_attempt_count" gorm:"not null"`
	CallbackNextAttemptAt                   int64  `json:"callback_next_attempt_at,omitempty" gorm:"index"`
	CallbackLeaseOwner                      string `json:"-" gorm:"type:varchar(128);index"`
	CallbackLeaseUntil                      int64  `json:"-" gorm:"index"`
	CallbackLastHTTPStatus                  int    `json:"callback_last_http_status,omitempty"`
	CallbackLastError                       string `json:"callback_last_error,omitempty" gorm:"type:text"`
	GenerationStartedAt                     int64  `json:"generation_started_at,omitempty"`
	GenerationCompletedAt                   int64  `json:"generation_completed_at,omitempty"`
	CreatedAt                               int64  `json:"created_at"`
	CompletedAt                             int64  `json:"completed_at,omitempty"`
	UpdatedAt                               int64  `json:"updated_at"`
	DeletedAt                               int64  `json:"-" gorm:"index"`
}

type MediaServiceUsage struct {
	ID                     int64  `json:"id" gorm:"primaryKey"`
	ServiceLineItemID      string `json:"service_line_item_id" gorm:"type:varchar(191);not null;uniqueIndex"`
	PlatformOrderID        string `json:"platform_order_id" gorm:"type:varchar(64);not null;index"`
	ServiceType            string `json:"service_type" gorm:"type:varchar(64);not null"`
	ProviderType           string `json:"provider_type" gorm:"type:varchar(32);not null"`
	ProviderID             int64  `json:"provider_id" gorm:"not null"`
	ServiceCode            string `json:"service_code" gorm:"type:varchar(128);not null"`
	SpecificationJSON      string `json:"specification" gorm:"column:specification;type:text"`
	SpecificationVersion   string `json:"specification_version" gorm:"type:varchar(64);not null"`
	AttemptID              string `json:"attempt_id" gorm:"type:varchar(191);not null;index"`
	ExecutionTaskID        string `json:"execution_task_id,omitempty" gorm:"type:varchar(191);index"`
	Status                 string `json:"status" gorm:"type:varchar(32);not null;index"`
	FailureReason          string `json:"failure_reason,omitempty" gorm:"type:varchar(64);index"`
	ChargeMicroRMB         int64  `json:"charge_micro_rmb" gorm:"not null"`
	PriceVersion           string `json:"price_version" gorm:"type:varchar(64);not null"`
	ProviderCostMicroRMB   *int64 `json:"provider_cost_micro_rmb,omitempty"`
	UsageFactsJSON         string `json:"usage_facts" gorm:"column:usage_facts;type:text"`
	UsageEvidenceHash      string `json:"usage_evidence_hash,omitempty" gorm:"type:varchar(80)"`
	Revision               int    `json:"revision" gorm:"not null"`
	UnknownSubmissionCount int64  `json:"unknown_submission_count" gorm:"not null"`
	StartedAt              int64  `json:"started_at,omitempty"`
	CompletedAt            int64  `json:"completed_at,omitempty"`
	CreatedAt              int64  `json:"created_at"`
	UpdatedAt              int64  `json:"updated_at"`
}

type SeedanceAttempt struct {
	ID                   int64  `json:"id" gorm:"primaryKey"`
	PlatformOrderID      string `json:"platform_order_id" gorm:"type:varchar(64);not null;index"`
	AttemptID            string `json:"attempt_id" gorm:"type:varchar(191);not null;uniqueIndex"`
	Stage                string `json:"stage" gorm:"type:varchar(32);not null"`
	AttemptNo            int    `json:"attempt_no" gorm:"not null"`
	ProviderType         string `json:"provider_type,omitempty" gorm:"type:varchar(32)"`
	ProviderID           int64  `json:"provider_id,omitempty"`
	ServiceCode          string `json:"service_code,omitempty" gorm:"type:varchar(128)"`
	SpecificationVersion string `json:"specification_version,omitempty" gorm:"type:varchar(64)"`
	ExternalTaskID       string `json:"external_task_id,omitempty" gorm:"type:varchar(191);index"`
	Status               string `json:"status" gorm:"type:varchar(32);not null;index"`
	RequestHash          string `json:"request_hash,omitempty" gorm:"type:varchar(80)"`
	ResponseEvidenceHash string `json:"response_evidence_hash,omitempty" gorm:"type:varchar(80)"`
	StartedAt            int64  `json:"started_at,omitempty"`
	CompletedAt          int64  `json:"completed_at,omitempty"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`
}

type ServiceBillingEvent struct {
	ID                int64   `json:"id" gorm:"primaryKey"`
	EventID           string  `json:"event_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	SourceRevisionKey *string `json:"source_revision_key,omitempty" gorm:"type:varchar(191);uniqueIndex"`
	PlatformOrderID   string  `json:"platform_order_id" gorm:"type:varchar(64);not null;index"`
	ServiceLineItemID string  `json:"service_line_item_id" gorm:"type:varchar(191);not null;index"`
	Revision          int     `json:"revision" gorm:"not null"`
	EventType         string  `json:"event_type" gorm:"type:varchar(64);not null"`
	PayloadJSON       string  `json:"payload" gorm:"column:payload;type:text;not null"`
	PayloadHash       string  `json:"payload_hash" gorm:"type:varchar(80);not null"`
	CreatedAt         int64   `json:"created_at"`
}

type ServiceBillingOutbox struct {
	ID             int64  `json:"id" gorm:"primaryKey"`
	EventID        string `json:"event_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	Status         string `json:"status" gorm:"type:varchar(32);not null;index"`
	AttemptCount   int    `json:"attempt_count" gorm:"not null"`
	NextAttemptAt  int64  `json:"next_attempt_at" gorm:"index"`
	LeaseOwner     string `json:"lease_owner,omitempty" gorm:"type:varchar(128);index"`
	LeaseUntil     int64  `json:"lease_until,omitempty" gorm:"index"`
	LastHTTPStatus int    `json:"last_http_status,omitempty"`
	LastError      string `json:"last_error,omitempty" gorm:"type:text"`
	ResponseJSON   string `json:"response,omitempty" gorm:"column:response;type:text"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

type ServiceBillingFailureAttempt struct {
	ID         int64  `json:"id" gorm:"primaryKey"`
	EventID    string `json:"event_id" gorm:"type:varchar(64);not null;index;uniqueIndex:idx_seedance_billing_failure_attempt"`
	AttemptNo  int    `json:"attempt_no" gorm:"not null;uniqueIndex:idx_seedance_billing_failure_attempt"`
	HTTPStatus int    `json:"http_status" gorm:"not null;index"`
	CreatedAt  int64  `json:"created_at" gorm:"index"`
}

type SeedanceAdminAudit struct {
	ID            int64  `json:"id" gorm:"primaryKey"`
	ActorUserID   int    `json:"actor_user_id" gorm:"not null;index"`
	ResourceType  string `json:"resource_type" gorm:"type:varchar(64);not null;index"`
	ResourceID    string `json:"resource_id" gorm:"type:varchar(191);not null"`
	Action        string `json:"action" gorm:"type:varchar(64);not null"`
	BeforeVersion string `json:"before_version,omitempty" gorm:"type:varchar(128)"`
	AfterVersion  string `json:"after_version,omitempty" gorm:"type:varchar(128)"`
	ChangeSummary string `json:"change_summary" gorm:"type:text;not null"`
	CreatedAt     int64  `json:"created_at" gorm:"index"`
}

type SeedanceOrderCreate struct {
	Task                            *Task
	Config                          *SeedanceChannelConfig
	Credential                      *SeedanceVolcengineCredential
	Offering                        *SeedanceModelOffering
	Provider                        *MediaEnhancementProvider
	HasReferenceVideo               bool
	RequestedDurationMillis         int64
	SaleUnitPriceMicroRMB           int64
	SuperResolutionUnitCostMicroRMB int64
	RequestFactsJSON                string
	PricingSnapshot                 string
	GenerationTaskID                string
	PublicProtocol                  string
	CallbackURL                     string
}

func GenerateSeedanceOrderID() string {
	id, err := uuid.NewV7()
	if err == nil {
		return id.String()
	}
	return uuid.NewString()
}

func SHA256Evidence(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func GetSeedanceChannelConfig(channelID int) (*SeedanceChannelConfig, error) {
	var item SeedanceChannelConfig
	if err := DB.Where("channel_id = ?", channelID).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func GetActiveSeedanceVolcengineCredential(channelID int) (*SeedanceVolcengineCredential, error) {
	var item SeedanceVolcengineCredential
	err := DB.Where("channel_id = ? AND status = ?", channelID, SeedanceCredentialActive).
		Order("version desc").First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func ResolveSeedanceArkAPIKey(channelID int) (string, *SeedanceVolcengineCredential, error) {
	credential, err := GetActiveSeedanceVolcengineCredential(channelID)
	if err != nil {
		return "", nil, err
	}
	key, err := common.DecryptSensitiveValue(credential.ArkAPIKeyEncrypted)
	if err != nil {
		return "", nil, err
	}
	if strings.TrimSpace(key) == "" {
		return "", nil, errors.New("Seedance Ark API key is empty")
	}
	return strings.TrimSpace(key), credential, nil
}

func ResolveSeedanceArkAPIKeyForTask(taskID string, fallbackChannelID int) (string, *SeedanceVolcengineCredential, error) {
	var order SeedanceOrder
	err := DB.Where("new_api_task_id = ?", taskID).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ResolveSeedanceArkAPIKey(fallbackChannelID)
	}
	if err != nil {
		return "", nil, err
	}
	var credential SeedanceVolcengineCredential
	if err := DB.Where("id = ? AND channel_id = ?", order.VolcengineCredentialID, order.ChannelID).First(&credential).Error; err != nil {
		return "", nil, err
	}
	key, err := common.DecryptSensitiveValue(credential.ArkAPIKeyEncrypted)
	if err != nil {
		return "", nil, err
	}
	return strings.TrimSpace(key), &credential, nil
}

func GetPublishedSeedanceOffering(channelID int, modelName string) (*SeedanceModelOffering, error) {
	var item SeedanceModelOffering
	err := DB.Where("channel_id = ? AND display_name = ? AND enabled = ? AND published_at > ? AND archived_at = ?",
		channelID, strings.TrimSpace(modelName), true, 0, 0).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func GetMediaEnhancementProvider(id int64) (*MediaEnhancementProvider, error) {
	var item MediaEnhancementProvider
	if err := DB.Where("id = ? AND status = ?", id, SeedanceConfigActive).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// GetMediaEnhancementProviderForExecution resolves a provider pinned by an
// existing order. Disabling a provider blocks new offerings, but must not make
// already accepted orders unrecoverable.
func GetMediaEnhancementProviderForExecution(id int64) (*MediaEnhancementProvider, error) {
	var item MediaEnhancementProvider
	if err := DB.Where("id = ?", id).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// InsertTaskWithSeedanceOrder persists the generic public task and the private
// order/attempt facts atomically in the NewAPI database.
func InsertTaskWithSeedanceOrder(input SeedanceOrderCreate) (*SeedanceOrder, error) {
	if input.Task == nil || input.Config == nil || input.Credential == nil || input.Offering == nil {
		return nil, errors.New("incomplete Seedance order input")
	}
	if input.RequestedDurationMillis <= 0 {
		// Rows and internal callers created before duration pricing used the
		// Ark default five-second request. Keep that migration path explicit.
		input.RequestedDurationMillis = 5000
	}
	estimatedProfit, err := CalculateSeedanceProfit(
		input.Offering.ModelSaleMicroRMB,
		input.Offering.ServiceChargeMicroRMB,
		input.Offering.VolcengineUnitCostMicroRMB,
	)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	platformOrderID := GenerateSeedanceOrderID()
	pricingSnapshot := strings.TrimSpace(input.PricingSnapshot)
	if pricingSnapshot == "" {
		pricingSnapshot = "{}"
	}
	callbackEncrypted := ""
	callbackStatus := SeedanceCallbackNone
	if strings.TrimSpace(input.CallbackURL) != "" {
		var err error
		callbackEncrypted, err = common.EncryptSensitiveValue(strings.TrimSpace(input.CallbackURL))
		if err != nil {
			return nil, err
		}
		callbackStatus = SeedanceCallbackWaiting
	}
	publicProtocol := strings.TrimSpace(input.PublicProtocol)
	if publicProtocol == "" {
		publicProtocol = SeedanceProtocolOfficial
	}
	providerSnapshotEncrypted := ""
	if input.Provider != nil {
		// adapter_type belongs to the frozen snapshot: the provider row stays
		// editable, so a later switch of wire protocol must not change how an
		// already accepted remote task is queried or cancelled.
		adapterType := strings.TrimSpace(input.Provider.AdapterType)
		if adapterType == "" {
			adapterType = LegacySeedanceAdapterType(input.Provider.ProviderType)
		}
		providerSnapshot, err := common.Marshal(map[string]any{
			"id": input.Provider.ID, "provider_type": input.Provider.ProviderType,
			"adapter_type": adapterType,
			"display_name": input.Provider.DisplayName, "service_endpoint": input.Provider.ServiceEndpoint,
			"credential_encrypted": input.Provider.CredentialEncrypted, "service_code": input.Provider.ServiceCode,
			"capabilities": input.Provider.CapabilitiesJSON, "timeout_policy": input.Provider.TimeoutPolicyJSON,
			"retry_policy": input.Provider.RetryPolicyJSON, "fallback_policy": input.Provider.FallbackPolicyJSON,
		})
		if err != nil {
			return nil, err
		}
		providerSnapshotEncrypted, err = common.EncryptSensitiveValue(string(providerSnapshot))
		if err != nil {
			return nil, err
		}
	}
	generationTaskID := strings.TrimSpace(input.GenerationTaskID)
	orderStatus := SeedanceOrderGenerationSubmitting
	attemptStatus := "SUBMITTING"
	if generationTaskID != "" {
		orderStatus = SeedanceOrderGenerationProcessing
		attemptStatus = SeedanceUsageRunning
	}
	order := &SeedanceOrder{
		PlatformOrderID:                         platformOrderID,
		NewAPITaskID:                            input.Task.TaskID,
		NewAPIUserID:                            input.Task.UserId,
		TokenID:                                 input.Task.PrivateData.TokenId,
		ChannelID:                               input.Task.ChannelId,
		InstanceID:                              input.Config.InstanceID,
		AIPDDBillingConfigRevision:              input.Config.Revision,
		AIPDDBillingBaseURLSnapshot:             input.Config.AIPDDBillingBaseURL,
		AIPDDBillingCredentialSnapshotEncrypted: input.Config.AIPDDBillingCredentialEncrypted,
		VolcengineCredentialID:                  input.Credential.ID,
		CredentialVersion:                       input.Credential.Version,
		Model:                                   input.Task.Properties.OriginModelName,
		OfferingID:                              input.Offering.ID,
		BaseModelID:                             input.Offering.BaseModelID,
		EnhancementModelID:                      input.Offering.EnhancementModelID,
		SourceResolution:                        input.Offering.SourceResolution,
		TargetResolution:                        input.Offering.TargetResolution,
		OutputFPS:                               input.Offering.OutputFPS,
		HasReferenceVideo:                       input.HasReferenceVideo,
		RequestedDurationMillis:                 input.RequestedDurationMillis,
		SaleUnitPriceMicroRMB:                   input.SaleUnitPriceMicroRMB,
		SuperResolutionUnitCostMicroRMB:         input.SuperResolutionUnitCostMicroRMB,
		SuperResolutionCostMicroRMB:             input.Offering.ServiceChargeMicroRMB,
		RequestFactsJSON:                        input.RequestFactsJSON,
		OrderStatus:                             orderStatus,
		VolcengineCostStatus:                    SeedanceCostEstimated,
		SyncStatus:                              SeedanceSyncPending,
		ModelSaleMicroRMB:                       input.Offering.ModelSaleMicroRMB,
		ServiceChargeTotalMicroRMB:              input.Offering.ServiceChargeMicroRMB,
		VolcengineEstimatedMicroRMB:             input.Offering.VolcengineUnitCostMicroRMB,
		NewAPIEstimatedProfitMicroRMB:           estimatedProfit,
		PricingSnapshotJSON:                     pricingSnapshot,
		PricingSnapshotHash:                     SHA256Evidence(pricingSnapshot),
		EnhancementProviderSnapshotEncrypted:    providerSnapshotEncrypted,
		PublicProtocol:                          publicProtocol,
		CallbackURLEncrypted:                    callbackEncrypted,
		CallbackStatus:                          callbackStatus,
		GenerationStartedAt:                     now,
		CreatedAt:                               now,
		UpdatedAt:                               now,
	}
	attemptID := platformOrderID + ":generation:1"
	attempt := &SeedanceAttempt{
		PlatformOrderID: order.PlatformOrderID,
		AttemptID:       attemptID,
		Stage:           "GENERATION",
		AttemptNo:       1,
		ExternalTaskID:  generationTaskID,
		Status:          attemptStatus,
		RequestHash:     SHA256Evidence(input.RequestFactsJSON),
		StartedAt:       now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var storedCredential SeedanceVolcengineCredential
		if input.Credential.ID <= 0 {
			return errors.New("Seedance credential is not persisted")
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND channel_id = ? AND version = ?", input.Credential.ID, input.Credential.ChannelID, input.Credential.Version).
			First(&storedCredential).Error; err != nil {
			return err
		}
		if storedCredential.Status != SeedanceCredentialActive && storedCredential.Status != SeedanceCredentialRetiring {
			return errors.New("Seedance credential is not available for this order")
		}
		if strings.TrimSpace(storedCredential.ArkAPIKeyEncrypted) == "" {
			return errors.New("Seedance credential key material has been retired")
		}
		if err := tx.Create(input.Task).Error; err != nil {
			return err
		}
		if err := tx.Create(order).Error; err != nil {
			return err
		}
		return tx.Create(attempt).Error
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

func ResolveSeedanceProviderForOrder(order *SeedanceOrder) (*MediaEnhancementProvider, error) {
	if order == nil {
		return nil, errors.New("Seedance order is required")
	}
	if strings.TrimSpace(order.EnhancementProviderSnapshotEncrypted) == "" {
		var snapshot struct {
			ProviderID int64 `json:"provider_id"`
		}
		if err := common.UnmarshalJsonStr(order.PricingSnapshotJSON, &snapshot); err != nil {
			return nil, err
		}
		provider, err := GetMediaEnhancementProviderForExecution(snapshot.ProviderID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(provider.AdapterType) == "" {
			provider.AdapterType = LegacySeedanceAdapterType(provider.ProviderType)
		}
		return provider, nil
	}
	value, err := common.DecryptSensitiveValue(order.EnhancementProviderSnapshotEncrypted)
	if err != nil {
		return nil, err
	}
	var snapshot struct {
		ID                  int64  `json:"id"`
		ProviderType        string `json:"provider_type"`
		AdapterType         string `json:"adapter_type"`
		DisplayName         string `json:"display_name"`
		ServiceEndpoint     string `json:"service_endpoint"`
		CredentialEncrypted string `json:"credential_encrypted"`
		ServiceCode         string `json:"service_code"`
		CapabilitiesJSON    string `json:"capabilities"`
		TimeoutPolicyJSON   string `json:"timeout_policy"`
		RetryPolicyJSON     string `json:"retry_policy"`
		FallbackPolicyJSON  string `json:"fallback_policy"`
	}
	if err := common.UnmarshalJsonStr(value, &snapshot); err != nil {
		return nil, err
	}
	if snapshot.ID <= 0 || strings.TrimSpace(snapshot.ServiceEndpoint) == "" {
		return nil, errors.New("Seedance provider snapshot is incomplete")
	}
	adapterType := strings.TrimSpace(snapshot.AdapterType)
	if adapterType == "" {
		adapterType = LegacySeedanceAdapterType(snapshot.ProviderType)
	}
	return &MediaEnhancementProvider{
		ID: snapshot.ID, ProviderType: snapshot.ProviderType, AdapterType: adapterType,
		DisplayName:     snapshot.DisplayName,
		ServiceEndpoint: snapshot.ServiceEndpoint, CredentialEncrypted: snapshot.CredentialEncrypted,
		ServiceCode: snapshot.ServiceCode, CapabilitiesJSON: snapshot.CapabilitiesJSON,
		TimeoutPolicyJSON: snapshot.TimeoutPolicyJSON, RetryPolicyJSON: snapshot.RetryPolicyJSON,
		FallbackPolicyJSON: snapshot.FallbackPolicyJSON, Status: SeedanceConfigActive,
	}, nil
}

func GetSeedanceOrderByTaskID(taskID string) (*SeedanceOrder, error) {
	var item SeedanceOrder
	if err := DB.Where("new_api_task_id = ?", taskID).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func GetVisibleSeedanceOrderByTaskID(taskID string) (*SeedanceOrder, error) {
	var item SeedanceOrder
	if err := DB.Where("new_api_task_id = ? AND deleted_at = ?", taskID, 0).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func SoftDeleteSeedanceOrder(taskID string) error {
	return DB.Model(&SeedanceOrder{}).Where("new_api_task_id = ? AND deleted_at = ?", taskID, 0).
		Updates(map[string]any{"deleted_at": time.Now().Unix(), "updated_at": time.Now().Unix()}).Error
}

func IsRecordNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

func SaveSeedanceChannelConfig(item *SeedanceChannelConfig, actorUserID int, summary string) error {
	if item == nil {
		return errors.New("Seedance channel config is required")
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		beforeVersion := ""
		var existing SeedanceChannelConfig
		err := tx.Where("channel_id = ?", item.ChannelID).First(&existing).Error
		if err == nil {
			beforeVersion = fmtInt(existing.Revision)
			item.Revision = existing.Revision + 1
			item.CreatedAt = existing.CreatedAt
			item.BillingAuthPausedAt = existing.BillingAuthPausedAt
			item.BillingAuthLastHTTPStatus = existing.BillingAuthLastHTTPStatus
			if item.LastVerifiedAt == 0 {
				item.LastVerifiedAt = existing.LastVerifiedAt
			}
			if item.AIPDDBillingCredentialEncrypted == "" {
				item.AIPDDBillingCredentialEncrypted = existing.AIPDDBillingCredentialEncrypted
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		} else {
			item.Revision = 1
			item.CreatedAt = now
		}
		item.UpdatedAt = now
		if err := tx.Save(item).Error; err != nil {
			return err
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "CHANNEL_CONFIG", ResourceID: fmtInt(item.ChannelID),
			Action: "UPSERT", BeforeVersion: beforeVersion, AfterVersion: fmtInt(item.Revision),
			ChangeSummary: summary, CreatedAt: now,
		}).Error
	})
}

func CreateSeedanceCredential(item *SeedanceVolcengineCredential, actorUserID int) error {
	if item == nil {
		return errors.New("Seedance credential is required")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var maxVersion int
		if err := tx.Model(&SeedanceVolcengineCredential{}).Where("channel_id = ?", item.ChannelID).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		item.Version = maxVersion + 1
		item.Status = SeedanceCredentialPending
		item.CreatedAt = now
		if err := tx.Create(item).Error; err != nil {
			return err
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "VOLCENGINE_CREDENTIAL", ResourceID: fmtInt64(item.ID),
			Action: "CREATE_VERSION", BeforeVersion: fmtInt(maxVersion), AfterVersion: fmtInt(item.Version),
			ChangeSummary: "created encrypted credential version", CreatedAt: now,
		}).Error
	})
}

func ActivateSeedanceCredential(id int64, actorUserID int, billingValidated bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var item SeedanceVolcengineCredential
		if err := tx.Where("id = ?", id).First(&item).Error; err != nil {
			return err
		}
		if item.Status == SeedanceCredentialRetired || strings.TrimSpace(item.ArkAPIKeyEncrypted) == "" {
			return errors.New("retired Seedance credentials cannot be activated")
		}
		var config SeedanceChannelConfig
		configErr := tx.Where("channel_id = ?", item.ChannelID).First(&config).Error
		if configErr != nil && !errors.Is(configErr, gorm.ErrRecordNotFound) {
			return configErr
		}
		if configErr == nil && config.VolcengineBillSyncEnabled && !billingValidated {
			return errors.New("bill sync requires a credential that passed ListBillDetail validation")
		}
		beforeVersion := ""
		var previousActive SeedanceVolcengineCredential
		activeErr := tx.Where("channel_id = ? AND status = ?", item.ChannelID, SeedanceCredentialActive).First(&previousActive).Error
		if activeErr == nil {
			beforeVersion = fmtInt(previousActive.Version)
		} else if !errors.Is(activeErr, gorm.ErrRecordNotFound) {
			return activeErr
		}
		now := time.Now().Unix()
		if err := tx.Model(&SeedanceVolcengineCredential{}).
			Where("channel_id = ? AND id <> ? AND status = ?", item.ChannelID, item.ID, SeedanceCredentialActive).
			Updates(map[string]any{
				"status":       SeedanceCredentialRetiring,
				"retire_after": now + int64(seedanceCredentialRetirementGrace/time.Second),
			}).Error; err != nil {
			return err
		}
		credentialUpdates := map[string]any{"status": SeedanceCredentialActive, "validated_at": now}
		if billingValidated {
			credentialUpdates["billing_validated_at"] = now
		}
		if err := tx.Model(&SeedanceVolcengineCredential{}).Where("id = ?", item.ID).
			Updates(credentialUpdates).Error; err != nil {
			return err
		}
		if err := tx.Model(&SeedanceChannelConfig{}).Where("channel_id = ?", item.ChannelID).
			Updates(map[string]any{"last_verified_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "VOLCENGINE_CREDENTIAL", ResourceID: fmtInt64(item.ID),
			Action: "VALIDATE_AND_ACTIVATE", BeforeVersion: beforeVersion, AfterVersion: fmtInt(item.Version),
			ChangeSummary: "credential validated against Ark", CreatedAt: now,
		}).Error
	})
}

// RetireUnusedSeedanceCredentials destroys superseded Ark and billing key
// material only after the activation hand-off grace period and after every
// order pinned to that exact credential version is terminal. Fingerprint,
// mask, version and audit history remain available without retaining secrets.
func RetireUnusedSeedanceCredentials(limit int) (int, error) {
	if limit <= 0 {
		limit = 20
	}
	now := time.Now().Unix()
	var candidateIDs []int64
	if err := DB.Model(&SeedanceVolcengineCredential{}).
		Where("status = ? AND retire_after > 0 AND retire_after <= ?", SeedanceCredentialRetiring, now).
		Order("id asc").Limit(limit).Pluck("id", &candidateIDs).Error; err != nil {
		return 0, err
	}
	retired := 0
	terminalStatuses := []string{SeedanceOrderSucceeded, SeedanceOrderFailed, SeedanceOrderCancelled}
	for _, credentialID := range candidateIDs {
		retiredThisCredential := false
		err := DB.Transaction(func(tx *gorm.DB) error {
			var credential SeedanceVolcengineCredential
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ? AND status = ? AND retire_after > 0 AND retire_after <= ?", credentialID, SeedanceCredentialRetiring, now).
				First(&credential).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil
				}
				return err
			}
			var openOrders int64
			if err := tx.Model(&SeedanceOrder{}).
				Where("volcengine_credential_id = ? AND credential_version = ? AND order_status NOT IN ?",
					credential.ID, credential.Version, terminalStatuses).
				Count(&openOrders).Error; err != nil {
				return err
			}
			if openOrders > 0 {
				return nil
			}
			result := tx.Model(&SeedanceVolcengineCredential{}).
				Where("id = ? AND status = ?", credential.ID, SeedanceCredentialRetiring).
				Updates(map[string]any{
					"status": SeedanceCredentialRetired, "ark_api_key_encrypted": "",
					"access_key_id_encrypted": "", "secret_access_key_encrypted": "",
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return nil
			}
			if err := tx.Create(&SeedanceAdminAudit{
				ActorUserID: 0, ResourceType: "VOLCENGINE_CREDENTIAL", ResourceID: fmtInt64(credential.ID),
				Action: "AUTO_RETIRE", BeforeVersion: fmtInt(credential.Version), AfterVersion: fmtInt(credential.Version),
				ChangeSummary: "destroyed superseded credential key material after all pinned orders became terminal",
				CreatedAt:     now,
			}).Error; err != nil {
				return err
			}
			retiredThisCredential = true
			return nil
		})
		if err != nil {
			return retired, err
		}
		if retiredThisCredential {
			retired++
		}
	}
	return retired, nil
}

func ListSeedanceCredentials(channelID int) ([]SeedanceVolcengineCredential, error) {
	var items []SeedanceVolcengineCredential
	err := DB.Where("channel_id = ?", channelID).Order("version desc").Find(&items).Error
	return items, err
}

func SaveMediaEnhancementProvider(item *MediaEnhancementProvider, actorUserID int, summary string) error {
	if item == nil {
		return errors.New("enhancement provider is required")
	}
	if strings.TrimSpace(item.AdapterType) == "" {
		item.AdapterType = LegacySeedanceAdapterType(item.ProviderType)
	}
	if item.ClearCredential {
		// A provider without a credential cannot authenticate, so it must stop
		// accepting new work in the same transaction that erases the secret.
		item.CredentialEncrypted = ""
		item.Status = SeedanceConfigDisabled
		summary = strings.TrimSpace(summary + "; cleared the stored credential and disabled the provider")
	}
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		beforeVersion := ""
		if item.ID != 0 {
			var existing MediaEnhancementProvider
			if err := tx.Where("id = ?", item.ID).First(&existing).Error; err != nil {
				return err
			}
			beforeVersion = fmtInt(existing.Version)
			item.Version = existing.Version + 1
			item.CreatedAt = existing.CreatedAt
			if item.CredentialEncrypted == "" && !item.ClearCredential {
				item.CredentialEncrypted = existing.CredentialEncrypted
			}
		} else {
			item.Version = 1
			item.CreatedAt = now
		}
		item.UpdatedAt = now
		if err := tx.Save(item).Error; err != nil {
			return err
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "ENHANCEMENT_PROVIDER", ResourceID: fmtInt64(item.ID),
			Action: "UPSERT", BeforeVersion: beforeVersion, AfterVersion: fmtInt(item.Version),
			ChangeSummary: summary, CreatedAt: now,
		}).Error
	})
}

func ListMediaEnhancementProviders() ([]MediaEnhancementProvider, error) {
	var items []MediaEnhancementProvider
	err := DB.Order("id asc").Find(&items).Error
	return items, err
}

func SaveSeedanceModelOffering(item *SeedanceModelOffering, actorUserID int, summary string) error {
	if item == nil {
		return errors.New("Seedance model offering is required")
	}
	if err := ValidateSeedanceMoneySnapshot(
		item.ModelSaleMicroRMB,
		item.ServiceChargeMicroRMB,
		item.VolcengineUnitCostMicroRMB,
		item.ProviderCostMicroRMB,
	); err != nil {
		return err
	}
	now := time.Now().Unix()
	var updatedChannel Channel
	err := DB.Transaction(func(tx *gorm.DB) error {
		var channel Channel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND type = ?", item.ChannelID, constant.ChannelTypeSeedance).First(&channel).Error; err != nil {
			return err
		}
		beforeVersion := ""
		if item.ID != 0 {
			var existing SeedanceModelOffering
			if err := tx.Where("id = ?", item.ID).First(&existing).Error; err != nil {
				return err
			}
			if err := validateSeedanceOfferingVersionChange(&existing, item); err != nil {
				return err
			}
			beforeVersion = existing.PricingVersion
			item.CreatedAt = existing.CreatedAt
		} else {
			item.CreatedAt = now
		}
		if item.Enabled {
			item.PublishedAt = now
		}
		item.UpdatedAt = now
		if err := tx.Save(item).Error; err != nil {
			return err
		}
		var modelNames []string
		if err := tx.Model(&SeedanceModelOffering{}).Where("channel_id = ? AND enabled = ? AND published_at > 0", item.ChannelID, true).
			Order("id asc").Pluck("display_name", &modelNames).Error; err != nil {
			return err
		}
		channel.Models = strings.Join(modelNames, ",")
		var mappings []SeedanceModelOffering
		if err := tx.Where("channel_id = ? AND enabled = ? AND published_at > 0", item.ChannelID, true).Find(&mappings).Error; err != nil {
			return err
		}
		modelMapping := make(map[string]string, len(mappings))
		for _, offering := range mappings {
			modelMapping[offering.DisplayName] = offering.ProviderModelID
		}
		mappingJSON, err := common.Marshal(modelMapping)
		if err != nil {
			return err
		}
		mappingText := string(mappingJSON)
		channel.ModelMapping = &mappingText
		if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{"models": channel.Models, "model_mapping": mappingText}).Error; err != nil {
			return err
		}
		if err := channel.UpdateAbilities(tx); err != nil {
			return err
		}
		for _, modelName := range modelNames {
			if err := ensureSeedancePublicModelMetadataTx(tx, modelName); err != nil {
				return err
			}
		}
		updatedChannel = channel
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "MODEL_OFFERING", ResourceID: fmtInt64(item.ID),
			Action: "UPSERT", BeforeVersion: beforeVersion, AfterVersion: item.PricingVersion,
			ChangeSummary: summary, CreatedAt: now,
		}).Error
	})
	if err == nil {
		CacheUpdateChannel(&updatedChannel)
	}
	return err
}

func ArchiveSeedanceModelOffering(id int64, actorUserID int) error {
	if id <= 0 {
		return errors.New("valid Seedance offering id is required")
	}
	var item SeedanceModelOffering
	if err := DB.Where("id = ?", id).First(&item).Error; err != nil {
		return err
	}
	if item.ArchivedAt > 0 {
		return nil
	}
	item.Enabled = false
	item.ArchivedAt = time.Now().Unix()
	return SaveSeedanceModelOffering(&item, actorUserID, "archived sale model")
}

// ensureSeedancePublicModelMetadataTx makes the independent Seedance channel
// the owner of its public model metadata. In particular, an identically named
// legacy AIPDD catalog row must never leak AIPDD branding after this offering
// is published.
func ensureSeedancePublicModelMetadataTx(tx *gorm.DB, modelName string) error {
	modelName = strings.TrimSpace(modelName)
	if tx == nil || modelName == "" {
		return errors.New("Seedance public model name is required")
	}
	now := common.GetTimestamp()
	var vendor Vendor
	err := tx.Where("name = ?", seedancePublicVendorName).First(&vendor).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		vendor = Vendor{
			Name: seedancePublicVendorName, Icon: seedancePublicVendorIcon,
			Website: seedancePublicVendorWebsite, Status: 1,
			CreatedTime: now, UpdatedTime: now,
		}
		if err := tx.Create(&vendor).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		updates := map[string]any{"status": 1, "updated_time": now}
		if strings.TrimSpace(vendor.Icon) == "" {
			updates["icon"] = seedancePublicVendorIcon
		}
		if strings.TrimSpace(vendor.Website) == "" {
			updates["website"] = seedancePublicVendorWebsite
		}
		if err := tx.Model(&Vendor{}).Where("id = ?", vendor.Id).Updates(updates).Error; err != nil {
			return err
		}
	}

	updates := map[string]any{
		"description":   seedancePublicModelDescription,
		"icon":          seedancePublicVendorIcon,
		"tags":          seedancePublicModelTags,
		"vendor_id":     vendor.Id,
		"endpoints":     marshalEndpointTypes([]constant.EndpointType{constant.EndpointTypeOpenAIVideo}),
		"status":        1,
		"sync_official": 1,
		"name_rule":     NameRuleExact,
		"updated_time":  now,
	}
	var metadata Model
	err = tx.Where("model_name = ?", modelName).First(&metadata).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		metadata = Model{
			ModelName: modelName, Description: seedancePublicModelDescription,
			Icon: seedancePublicVendorIcon, Tags: seedancePublicModelTags,
			VendorID:  vendor.Id,
			Endpoints: marshalEndpointTypes([]constant.EndpointType{constant.EndpointTypeOpenAIVideo}),
			Status:    1, SyncOfficial: 1, NameRule: NameRuleExact,
			CreatedTime: now, UpdatedTime: now,
		}
		return tx.Create(&metadata).Error
	}
	if err != nil {
		return err
	}
	return tx.Model(&Model{}).Where("id = ?", metadata.Id).Updates(updates).Error
}

func validateSeedanceOfferingVersionChange(before, after *SeedanceModelOffering) error {
	if before == nil || after == nil {
		return errors.New("offering versions are required")
	}
	if seedanceOfferingSnapshotChanged(before, after) &&
		strings.TrimSpace(before.PricingVersion) == strings.TrimSpace(after.PricingVersion) {
		return fmt.Errorf("%w: pricing_version must change when offering snapshot fields change", ErrSeedanceInvalidPricing)
	}
	return nil
}

func seedanceOfferingSnapshotChanged(before, after *SeedanceModelOffering) bool {
	if before == nil || after == nil {
		return true
	}
	return before.DisplayName != after.DisplayName ||
		before.BaseModelID != after.BaseModelID ||
		!equalOptionalInt64(before.EnhancementModelID, after.EnhancementModelID) ||
		before.SourceResolution != after.SourceResolution ||
		before.TargetResolution != after.TargetResolution ||
		before.OutputFPS != after.OutputFPS ||
		before.NoReferenceUnitPriceMicroRMB != after.NoReferenceUnitPriceMicroRMB ||
		before.ReferenceUnitPriceMicroRMB != after.ReferenceUnitPriceMicroRMB ||
		before.ProviderModelID != after.ProviderModelID ||
		before.ResolutionRulesJSON != after.ResolutionRulesJSON ||
		before.DurationRulesJSON != after.DurationRulesJSON ||
		before.EnhancementProviderID != after.EnhancementProviderID ||
		before.EnhancementServiceCode != after.EnhancementServiceCode ||
		before.EnhancementSpecificationJSON != after.EnhancementSpecificationJSON ||
		before.EnhancementSpecificationVersion != after.EnhancementSpecificationVersion ||
		before.ModelSaleMicroRMB != after.ModelSaleMicroRMB ||
		before.ServiceChargeMicroRMB != after.ServiceChargeMicroRMB ||
		!equalOptionalMicroRMB(before.ProviderCostMicroRMB, after.ProviderCostMicroRMB) ||
		before.VolcengineUnitCostMicroRMB != after.VolcengineUnitCostMicroRMB
}

func equalOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalOptionalMicroRMB(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func ListSeedanceModelOfferings(channelID int) ([]SeedanceModelOffering, error) {
	var items []SeedanceModelOffering
	query := DB.Order("id asc")
	if channelID > 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	err := query.Find(&items).Error
	return items, err
}

type SeedanceOrderQuery struct {
	ChannelID       int
	PlatformOrderID string
	Status          string
	Offset          int
	Limit           int
}

func ListSeedanceOrders(query SeedanceOrderQuery) ([]SeedanceOrder, int64, error) {
	db := DB.Model(&SeedanceOrder{})
	if query.ChannelID > 0 {
		db = db.Where("channel_id = ?", query.ChannelID)
	}
	if query.PlatformOrderID != "" {
		db = db.Where("platform_order_id = ?", query.PlatformOrderID)
	}
	if query.Status != "" {
		db = db.Where("order_status = ?", query.Status)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []SeedanceOrder
	if query.Limit <= 0 {
		query.Limit = 20
	}
	err := db.Order("id desc").Offset(query.Offset).Limit(query.Limit).Find(&items).Error
	return items, total, err
}

type ServiceBillingOutboxQuery struct {
	ChannelID       int
	InstanceID      string
	PlatformOrderID string
	Status          string
	Offset          int
	Limit           int
}

type ServiceBillingOutboxAdminRow struct {
	ServiceBillingOutbox
	PlatformOrderID   string `json:"platform_order_id"`
	ServiceLineItemID string `json:"service_line_item_id"`
	ChannelID         int    `json:"channel_id"`
	InstanceID        string `json:"instance_id"`
}

func ListServiceBillingOutbox(query ServiceBillingOutboxQuery) ([]ServiceBillingOutboxAdminRow, int64, error) {
	db := DB.Table("service_billing_outboxes").
		Joins("JOIN service_billing_events ON service_billing_events.event_id = service_billing_outboxes.event_id").
		Joins("JOIN seedance_orders ON seedance_orders.platform_order_id = service_billing_events.platform_order_id")
	if strings.TrimSpace(query.Status) != "" {
		db = db.Where("service_billing_outboxes.status = ?", strings.TrimSpace(query.Status))
	}
	if query.ChannelID > 0 {
		db = db.Where("seedance_orders.channel_id = ?", query.ChannelID)
	}
	if strings.TrimSpace(query.InstanceID) != "" {
		db = db.Where("seedance_orders.instance_id = ?", strings.TrimSpace(query.InstanceID))
	}
	if strings.TrimSpace(query.PlatformOrderID) != "" {
		db = db.Where("service_billing_events.platform_order_id = ?", strings.TrimSpace(query.PlatformOrderID))
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if query.Limit <= 0 {
		query.Limit = 20
	}
	var items []ServiceBillingOutboxAdminRow
	err := db.Select(
		"service_billing_outboxes.*, service_billing_events.platform_order_id, service_billing_events.service_line_item_id, seedance_orders.channel_id, seedance_orders.instance_id",
	).Order("service_billing_outboxes.id desc").Offset(query.Offset).Limit(query.Limit).Scan(&items).Error
	return items, total, err
}

func ReplayServiceBillingOutbox(eventID string, actorUserID int) error {
	now := time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		var outbox ServiceBillingOutbox
		if err := tx.Where("event_id = ?", eventID).First(&outbox).Error; err != nil {
			return err
		}
		if outbox.Status == SeedanceSyncDeadLetter {
			return ErrSeedanceDeadLetterRevision
		}
		if outbox.Status == SeedanceSyncSynced || outbox.Status == SeedanceSyncSending {
			return errors.New("billing event is not eligible for replay")
		}
		var event ServiceBillingEvent
		if err := tx.Where("event_id = ?", eventID).First(&event).Error; err != nil {
			return err
		}
		var order SeedanceOrder
		if err := tx.Where("platform_order_id = ?", event.PlatformOrderID).First(&order).Error; err != nil {
			return err
		}
		if outbox.Status == SeedanceSyncAuthPaused {
			var eventIDs []string
			scopeQuery := tx.Table("service_billing_events").
				Joins("JOIN seedance_orders ON seedance_orders.platform_order_id = service_billing_events.platform_order_id").
				Where("seedance_orders.channel_id = ?", order.ChannelID)
			if order.AIPDDBillingConfigRevision > 0 {
				scopeQuery = scopeQuery.Where("seedance_orders.aipdd_billing_config_revision = ?", order.AIPDDBillingConfigRevision)
			} else {
				scopeQuery = scopeQuery.Where("seedance_orders.aipdd_billing_config_revision = 0")
			}
			if err := scopeQuery.Pluck("service_billing_events.event_id", &eventIDs).Error; err != nil {
				return err
			}
			if len(eventIDs) > 0 {
				if err := tx.Model(&ServiceBillingOutbox{}).
					Where("event_id IN ? AND status = ?", eventIDs, SeedanceSyncAuthPaused).
					Updates(map[string]any{
						"status": SeedanceSyncReady, "next_attempt_at": now,
						"lease_owner": "", "lease_until": 0, "last_error": "", "updated_at": now,
					}).Error; err != nil {
					return err
				}
			}
			var remainingPaused int64
			if err := tx.Table("service_billing_outboxes").
				Joins("JOIN service_billing_events ON service_billing_events.event_id = service_billing_outboxes.event_id").
				Joins("JOIN seedance_orders ON seedance_orders.platform_order_id = service_billing_events.platform_order_id").
				Where("seedance_orders.channel_id = ? AND service_billing_outboxes.status = ?", order.ChannelID, SeedanceSyncAuthPaused).
				Count(&remainingPaused).Error; err != nil {
				return err
			}
			if remainingPaused == 0 {
				if err := tx.Model(&SeedanceChannelConfig{}).Where("channel_id = ?", order.ChannelID).
					Updates(map[string]any{"billing_auth_paused_at": 0, "billing_auth_last_http_status": 0, "updated_at": now}).Error; err != nil {
					return err
				}
			}
		}
		result := tx.Model(&ServiceBillingOutbox{}).
			Where("id = ? AND status IN ?", outbox.ID, []string{SeedanceSyncReady, SeedanceSyncRetryWait, SeedanceSyncAuthPaused}).
			Updates(map[string]any{
				"status": SeedanceSyncReady, "next_attempt_at": now, "lease_owner": "", "lease_until": 0,
				"last_error": "", "updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("billing event is not eligible for replay")
		}
		return tx.Create(&SeedanceAdminAudit{
			ActorUserID: actorUserID, ResourceType: "BILLING_OUTBOX", ResourceID: eventID,
			Action: "REPLAY", ChangeSummary: "manually queued billing event replay", CreatedAt: now,
		}).Error
	})
}

func fmtInt(value int) string {
	return strconv.Itoa(value)
}

func fmtInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
