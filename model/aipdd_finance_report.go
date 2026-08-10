package model

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AIPDDFinanceExportPending = "PENDING"
	AIPDDFinanceExportRunning = "RUNNING"
	AIPDDFinanceExportReady   = "READY"
	AIPDDFinanceExportFailed  = "FAILED"
)

type AIPDDFinanceOrderFilter struct {
	UserID             int    `json:"user_id"`
	TokenID            int    `json:"token_id"`
	ChannelID          int    `json:"channel_id"`
	UserQuery          string `json:"user_query"`
	TokenQuery         string `json:"token_query"`
	InstanceID         string `json:"instance_id"`
	Model              string `json:"model"`
	PlatformOrderID    string `json:"platform_order_id"`
	OrderStatus        string `json:"order_status"`
	LocalBillingStatus string `json:"local_billing_status"`
	CostStatus         string `json:"cost_status"`
	IssueView          string `json:"issue_view"`
	StartTime          int64  `json:"start_time"`
	EndTime            int64  `json:"end_time"`
}

// AIPDDFinanceOrderListItem deliberately excludes upstream snapshots and token keys.
type AIPDDFinanceOrderListItem struct {
	ID                         string `json:"id"`
	PlatformOrderID            string `json:"platform_order_id"`
	LatestAttemptID            string `json:"latest_attempt_id"`
	InstanceID                 string `json:"instance_id"`
	RequestID                  string `json:"request_id"`
	UserID                     int    `json:"user_id"`
	Username                   string `json:"username"`
	UserDisplayName            string `json:"user_display_name"`
	TokenID                    int    `json:"token_id"`
	TokenName                  string `json:"token_name"`
	TokenMaskedKey             string `json:"token_masked_key"`
	ChannelID                  int    `json:"channel_id"`
	ChannelName                string `json:"channel_name"`
	Model                      string `json:"model"`
	OrderStatus                string `json:"order_status"`
	LocalBillingStatus         string `json:"local_billing_status"`
	CostStatus                 string `json:"cost_status"`
	SettlementRevision         int64  `json:"settlement_revision"`
	CustomerChargeQuota        int64  `json:"customer_charge_quota"`
	CustomerChargeRMBMic       int64  `json:"customer_charge_rmb_mic"`
	PendingChargeQuota         int64  `json:"pending_charge_quota"`
	PendingChargeRMBMic        int64  `json:"pending_charge_rmb_mic"`
	AIPDDChargeAWCoin          int64  `json:"aipdd_charge_awcoin"`
	AIPDDChargeRMBMic          *int64 `json:"aipdd_charge_rmb_mic"`
	ActualSpendAWCoin          *int64 `json:"actual_spend_awcoin"`
	BaseModelCostRMBMic        *int64 `json:"base_model_cost_rmb_mic"`
	AIPDDModelCostRMBMic       *int64 `json:"aipdd_model_cost_rmb_mic"`
	ActualSpendRMBMic          *int64 `json:"actual_spend_rmb_mic"`
	ConfirmedProfitRMBMic      *int64 `json:"confirmed_profit_rmb_mic"`
	EstimatedProfitRMBMic      *int64 `json:"estimated_profit_rmb_mic"`
	UpstreamReference          string `json:"upstream_reference"`
	SourceType                 string `json:"source_type"`
	SourceID                   string `json:"source_id"`
	OccurredAt                 int64  `json:"occurred_at"`
	SettledAt                  *int64 `json:"settled_at"`
	CreatedAt                  int64  `json:"created_at"`
	UpdatedAt                  int64  `json:"updated_at"`
	RequiresManualReview       bool   `json:"requires_manual_review"`
	SourceCostConfirmed        bool   `json:"source_cost_confirmed"`
	FinancialTraceCompleteness string `json:"financial_trace_completeness"`
}

type AIPDDFinanceOrderDetail struct {
	Order                   AIPDDFinanceOrderListItem `json:"order"`
	CustomerRateSnapshot    string                    `json:"customer_rate_snapshot"`
	UpstreamSnapshot        string                    `json:"upstream_snapshot"`
	Movements               []AIPDDFinanceMovement    `json:"movements"`
	SettlementEvents        []AIPDDFinanceInbox       `json:"settlement_events"`
	PendingOrFailedSyncJobs []AIPDDFinanceOutbox      `json:"pending_or_failed_sync_jobs"`
}

type AIPDDFinanceSummary struct {
	OrderCount                   int64 `json:"order_count"`
	CustomerNetConsumptionRMBMic int64 `json:"customer_net_consumption_rmb_mic"`
	ConfirmedSourceCostRMBMic    int64 `json:"confirmed_source_cost_rmb_mic"`
	ConfirmedProfitRMBMic        int64 `json:"confirmed_profit_rmb_mic"`
	EstimatedSourceCostRMBMic    int64 `json:"estimated_source_cost_rmb_mic"`
	EstimatedProfitRMBMic        int64 `json:"estimated_profit_rmb_mic"`
	LossOrderCount               int64 `json:"loss_order_count"`
	PendingConfirmationCount     int64 `json:"pending_confirmation_count"`
	ManualReviewCount            int64 `json:"manual_review_count"`
}

type AIPDDFinanceSyncStatus struct {
	ChannelID             int    `json:"channel_id"`
	ChannelName           string `json:"channel_name"`
	InstanceID            string `json:"instance_id"`
	LastSequence          int64  `json:"last_sequence"`
	LastSuccessAt         int64  `json:"last_success_at"`
	BacklogCount          int64  `json:"backlog_count"`
	DeadCount             int64  `json:"dead_count"`
	IgnoredCount          int64  `json:"ignored_count"`
	NextPullAt            int64  `json:"next_pull_at"`
	ConsecutiveFailures   int    `json:"consecutive_failures"`
	PoisonSequence        int64  `json:"poison_sequence"`
	PoisonError           string `json:"poison_error"`
	LastError             string `json:"last_error"`
	LastErrorAt           int64  `json:"last_error_at"`
	SingleKeyValid        bool   `json:"single_key_valid"`
	MultiKeyEnabled       bool   `json:"multi_key_enabled"`
}

type AIPDDFinanceExportJob struct {
	ID           string `gorm:"type:varchar(36);primaryKey" json:"id"`
	Status       string `gorm:"type:varchar(20);index" json:"status"`
	FilterJSON   string `gorm:"type:text" json:"-"`
	FileName     string `gorm:"type:varchar(255)" json:"file_name"`
	ContentType  string `gorm:"type:varchar(100)" json:"content_type"`
	FileData     []byte `json:"-"`
	SHA256       string `gorm:"type:varchar(64)" json:"sha256"`
	RowCount     int64  `json:"row_count"`
	FailureCause string `gorm:"type:text" json:"failure_cause"`
	CreatedBy    int    `json:"created_by"`
	CreatedAt    int64  `gorm:"index" json:"created_at"`
	StartedAt    int64  `json:"started_at"`
	CompletedAt  int64  `json:"completed_at"`
	ExpiresAt    int64  `gorm:"index" json:"expires_at"`
}

func (AIPDDFinanceExportJob) TableName() string { return "aipdd_finance_export_job" }

func aipddFinanceOrderQuery(filter AIPDDFinanceOrderFilter) *gorm.DB {
	query := DB.Model(&AIPDDFinanceOrder{})
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.TokenID > 0 {
		query = query.Where("token_id = ?", filter.TokenID)
	}
	if filter.ChannelID > 0 {
		query = query.Where("channel_id = ?", filter.ChannelID)
	}
	if value := strings.TrimSpace(filter.UserQuery); value != "" {
		var ids []int
		userQuery := DB.Model(&User{}).Select("id")
		if id, err := strconv.Atoi(value); err == nil && id > 0 {
			userQuery = userQuery.Where("id = ? OR username = ? OR display_name = ?", id, value, value)
		} else {
			userQuery = userQuery.Where("username = ? OR display_name = ?", value, value)
		}
		if userQuery.Find(&ids).Error != nil || len(ids) == 0 {
			return query.Where("1 = 0")
		}
		query = query.Where("user_id IN ?", ids)
	}
	if value := strings.TrimSpace(filter.TokenQuery); value != "" {
		var ids []int
		tokenQuery := DB.Model(&Token{}).Select("id")
		if id, err := strconv.Atoi(value); err == nil && id > 0 {
			tokenQuery = tokenQuery.Where("id = ? OR name = ?", id, value)
		} else {
			tokenQuery = tokenQuery.Where("name = ?", value)
		}
		if tokenQuery.Find(&ids).Error != nil || len(ids) == 0 {
			return query.Where("1 = 0")
		}
		query = query.Where("token_id IN ?", ids)
	}
	if value := strings.TrimSpace(filter.InstanceID); value != "" {
		query = query.Where("instance_id = ?", value)
	}
	if value := strings.TrimSpace(filter.Model); value != "" {
		query = query.Where("model = ?", value)
	}
	if value := strings.TrimSpace(filter.PlatformOrderID); value != "" {
		query = query.Where("platform_order_id = ?", value)
	}
	if value := strings.TrimSpace(filter.OrderStatus); value != "" {
		query = query.Where("order_status = ?", value)
	}
	if value := strings.TrimSpace(filter.LocalBillingStatus); value != "" {
		query = query.Where("local_billing_status = ?", value)
	}
	if value := strings.TrimSpace(filter.CostStatus); value != "" {
		query = query.Where("cost_status = ?", value)
	}
	switch strings.ToUpper(strings.TrimSpace(filter.IssueView)) {
	case "LOSS":
		query = query.Where("cost_status = ? AND profit_rmb_mic < 0", "CONFIRMED")
	case "PENDING":
		query = query.Where("cost_status <> ? OR cost_status = ''", "CONFIRMED")
	case "REVIEW":
		query = query.Where("local_billing_status LIKE ?", "%REVIEW_REQUIRED")
	case "UNVERIFIABLE":
		query = query.Where("cost_status = ?", "UNVERIFIABLE")
	}
	if filter.StartTime > 0 {
		query = query.Where("occurred_at >= ?", filter.StartTime)
	}
	if filter.EndTime > 0 {
		query = query.Where("occurred_at < ?", filter.EndTime)
	}
	return query
}

func ListAIPDDFinanceOrders(offset, limit int, filter AIPDDFinanceOrderFilter) ([]AIPDDFinanceOrderListItem, int64, error) {
	query := aipddFinanceOrderQuery(filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var orders []AIPDDFinanceOrder
	if err := query.Order("occurred_at desc").Offset(max(0, offset)).Limit(max(1, min(500, limit))).Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	items, err := buildAIPDDFinanceOrderItems(orders)
	return items, total, err
}

func ListAllAIPDDFinanceOrders(filter AIPDDFinanceOrderFilter, limit int) ([]AIPDDFinanceOrderListItem, error) {
	var orders []AIPDDFinanceOrder
	if err := aipddFinanceOrderQuery(filter).Order("occurred_at asc").Limit(max(1, min(100000, limit))).Find(&orders).Error; err != nil {
		return nil, err
	}
	return buildAIPDDFinanceOrderItems(orders)
}

func buildAIPDDFinanceOrderItems(orders []AIPDDFinanceOrder) ([]AIPDDFinanceOrderListItem, error) {
	userIDs := make([]int, 0, len(orders))
	tokenIDs := make([]int, 0, len(orders))
	channelIDs := make([]int, 0, len(orders))
	for _, order := range orders {
		userIDs = append(userIDs, order.UserID)
		tokenIDs = append(tokenIDs, order.TokenID)
		channelIDs = append(channelIDs, order.ChannelID)
	}
	users := make([]User, 0)
	tokens := make([]Token, 0)
	channels := make([]Channel, 0)
	if len(userIDs) > 0 {
		if err := DB.Select("id", "username", "display_name").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, err
		}
		if err := DB.Select("id", "name", "key").Where("id IN ?", tokenIDs).Find(&tokens).Error; err != nil {
			return nil, err
		}
		if err := DB.Select("id", "name").Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
			return nil, err
		}
	}
	userMap := make(map[int]User, len(users))
	tokenMap := make(map[int]Token, len(tokens))
	channelMap := make(map[int]Channel, len(channels))
	for _, item := range users {
		userMap[item.Id] = item
	}
	for _, item := range tokens {
		tokenMap[item.Id] = item
	}
	for _, item := range channels {
		channelMap[item.Id] = item
	}
	items := make([]AIPDDFinanceOrderListItem, 0, len(orders))
	for _, order := range orders {
		user := userMap[order.UserID]
		token := tokenMap[order.TokenID]
		channel := channelMap[order.ChannelID]
		var estimatedProfit *int64
		if order.AIPDDChargeRMBMic != nil {
			value := order.CustomerChargeRMBMic - *order.AIPDDChargeRMBMic
			estimatedProfit = &value
		}
		review := strings.HasSuffix(order.LocalBillingStatus, "REVIEW_REQUIRED")
		completeness := "PRECISE"
		if order.CostStatus == "UNVERIFIABLE" {
			completeness = "UNVERIFIABLE"
		} else if order.CostStatus != "CONFIRMED" {
			completeness = "RULE_DERIVED"
		}
		items = append(items, AIPDDFinanceOrderListItem{
			ID: order.ID, PlatformOrderID: order.PlatformOrderID, LatestAttemptID: order.LatestAttemptID,
			InstanceID: order.InstanceID, RequestID: order.RequestID, UserID: order.UserID,
			Username: user.Username, UserDisplayName: user.DisplayName, TokenID: order.TokenID,
			TokenName: token.Name, TokenMaskedKey: MaskTokenKey(token.Key), ChannelID: order.ChannelID,
			ChannelName: channel.Name, Model: order.Model, OrderStatus: order.OrderStatus,
			LocalBillingStatus: order.LocalBillingStatus, CostStatus: order.CostStatus,
			SettlementRevision: order.SettlementRevision, CustomerChargeQuota: order.CustomerChargeQuota,
			CustomerChargeRMBMic: order.CustomerChargeRMBMic, PendingChargeQuota: order.PendingChargeQuota,
			PendingChargeRMBMic: order.PendingChargeRMBMic, AIPDDChargeAWCoin: order.AIPDDChargeAWCoin,
			AIPDDChargeRMBMic: order.AIPDDChargeRMBMic, ActualSpendAWCoin: order.ActualSpendAWCoin,
			BaseModelCostRMBMic: order.BaseModelCostRMBMic, AIPDDModelCostRMBMic: order.AIPDDModelCostRMBMic,
			ActualSpendRMBMic: order.ActualSpendRMBMic, ConfirmedProfitRMBMic: order.ProfitRMBMic,
			EstimatedProfitRMBMic: estimatedProfit, UpstreamReference: order.UpstreamReference,
			SourceType: order.SourceType, SourceID: order.SourceID, OccurredAt: order.OccurredAt,
			SettledAt: order.SettledAt, CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt,
			RequiresManualReview: review, SourceCostConfirmed: order.CostStatus == "CONFIRMED",
			FinancialTraceCompleteness: completeness,
		})
	}
	return items, nil
}

func GetAIPDDFinanceOrderDetail(id string) (*AIPDDFinanceOrderDetail, error) {
	var order AIPDDFinanceOrder
	query := DB.Where("id = ?", id)
	if err := query.First(&order).Error; err != nil {
		return nil, err
	}
	items, err := buildAIPDDFinanceOrderItems([]AIPDDFinanceOrder{order})
	if err != nil {
		return nil, err
	}
	detail := &AIPDDFinanceOrderDetail{Order: items[0], CustomerRateSnapshot: order.CustomerRateSnapshot, UpstreamSnapshot: order.UpstreamSnapshot}
	if err = DB.Where("finance_order_id = ?", order.ID).Order("occurred_at asc").Find(&detail.Movements).Error; err != nil {
		return nil, err
	}
	if err = DB.Where("platform_order_id = ?", order.PlatformOrderID).Order("source_sequence asc").Find(&detail.SettlementEvents).Error; err != nil {
		return nil, err
	}
	if err = DB.Where("channel_id = ? AND instance_id = ? AND platform_order_id = ? AND state IN ?",
		order.ChannelID, order.InstanceID, order.PlatformOrderID,
		[]string{AIPDDFinanceOutboxPending, AIPDDFinanceOutboxDead, AIPDDFinanceOutboxIgnored}).
		Order("created_at desc").Find(&detail.PendingOrFailedSyncJobs).Error; err != nil {
		return nil, err
	}
	return detail, nil
}

func ListAIPDDFinanceMovements(financeOrderIDs []string) ([]AIPDDFinanceMovement, error) {
	if len(financeOrderIDs) == 0 {
		return []AIPDDFinanceMovement{}, nil
	}
	var movements []AIPDDFinanceMovement
	err := DB.Where("finance_order_id IN ?", financeOrderIDs).Order("occurred_at asc").Find(&movements).Error
	return movements, err
}

func ListAIPDDFinanceIssues(financeOrderIDs []string) ([]AIPDDFinanceOutbox, error) {
	if len(financeOrderIDs) == 0 {
		return []AIPDDFinanceOutbox{}, nil
	}
	var orders []AIPDDFinanceOrder
	if err := DB.Select("platform_order_id", "channel_id", "instance_id").Where("id IN ?", financeOrderIDs).Find(&orders).Error; err != nil {
		return nil, err
	}
	platformOrderIDs := make([]string, 0, len(orders))
	for _, order := range orders {
		platformOrderIDs = append(platformOrderIDs, order.PlatformOrderID)
	}
	var issues []AIPDDFinanceOutbox
	err := DB.Where("platform_order_id IN ? AND state IN ?", platformOrderIDs,
		[]string{AIPDDFinanceOutboxPending, AIPDDFinanceOutboxDead}).
		Order("created_at asc").Find(&issues).Error
	return issues, err
}

func GetAIPDDFinanceSummary(filter AIPDDFinanceOrderFilter) (AIPDDFinanceSummary, error) {
	var orders []AIPDDFinanceOrder
	if err := aipddFinanceOrderQuery(filter).Find(&orders).Error; err != nil {
		return AIPDDFinanceSummary{}, err
	}
	var result AIPDDFinanceSummary
	for _, order := range orders {
		result.OrderCount++
		result.CustomerNetConsumptionRMBMic += order.CustomerChargeRMBMic
		if order.AIPDDChargeRMBMic != nil {
			result.EstimatedSourceCostRMBMic += *order.AIPDDChargeRMBMic
			result.EstimatedProfitRMBMic += order.CustomerChargeRMBMic - *order.AIPDDChargeRMBMic
		}
		if order.CostStatus == "CONFIRMED" && order.AIPDDChargeRMBMic != nil {
			result.ConfirmedSourceCostRMBMic += *order.AIPDDChargeRMBMic
			profit := order.CustomerChargeRMBMic - *order.AIPDDChargeRMBMic
			result.ConfirmedProfitRMBMic += profit
			if profit < 0 {
				result.LossOrderCount++
			}
		} else {
			result.PendingConfirmationCount++
		}
		if strings.HasSuffix(order.LocalBillingStatus, "REVIEW_REQUIRED") {
			result.ManualReviewCount++
		}
	}
	return result, nil
}

func ListAIPDDFinanceSyncStatus(instanceID string) ([]AIPDDFinanceSyncStatus, error) {
	channels, err := GetAIPDDChannelsForFinance()
	if err != nil {
		return nil, err
	}
	statuses := make([]AIPDDFinanceSyncStatus, 0, len(channels))
	for _, channel := range channels {
		instanceSet := make(map[string]struct{})
		instances := make([]string, 0)
		query := DB.Model(&AIPDDFinanceOrder{}).Distinct("instance_id").Where("channel_id = ?", channel.Id)
		cursorQuery := DB.Model(&AIPDDFinanceCursor{}).Distinct("instance_id").Where("channel_id = ?", channel.Id)
		if trimmed := strings.TrimSpace(instanceID); trimmed != "" {
			query = query.Where("instance_id = ?", trimmed)
			cursorQuery = cursorQuery.Where("instance_id = ?", trimmed)
		}
		var orderInstances []string
		var cursorInstances []string
		if err = query.Pluck("instance_id", &orderInstances).Error; err != nil {
			return nil, err
		}
		if err = cursorQuery.Pluck("instance_id", &cursorInstances).Error; err != nil {
			return nil, err
		}
		for _, current := range append(orderInstances, cursorInstances...) {
			if current == "" {
				continue
			}
			if _, exists := instanceSet[current]; exists {
				continue
			}
			instanceSet[current] = struct{}{}
			instances = append(instances, current)
		}
		if len(instances) == 0 && strings.TrimSpace(instanceID) != "" {
			instances = append(instances, strings.TrimSpace(instanceID))
		}
		for _, currentInstance := range instances {
			status := AIPDDFinanceSyncStatus{ChannelID: channel.Id, ChannelName: channel.Name, InstanceID: currentInstance,
				SingleKeyValid: !channel.ChannelInfo.IsMultiKey, MultiKeyEnabled: channel.ChannelInfo.IsMultiKey}
			var cursor AIPDDFinanceCursor
			if cursorErr := DB.Where("channel_id = ? AND instance_id = ?", channel.Id, currentInstance).First(&cursor).Error; cursorErr == nil {
				status.LastSequence = cursor.LastSequence
				status.NextPullAt = cursor.NextPullAt
				status.ConsecutiveFailures = cursor.ConsecutiveFailures
				status.PoisonSequence = cursor.PoisonSequence
				status.PoisonError = cursor.PoisonError
				if cursor.LastPullError != "" {
					status.LastError, status.LastErrorAt = cursor.LastPullError, cursor.LastPullErrorAt
				}
				if cursor.ConsecutiveFailures == 0 && cursor.PoisonSequence == 0 {
					status.LastSuccessAt = cursor.UpdatedAt
				}
			} else if !errors.Is(cursorErr, gorm.ErrRecordNotFound) {
				return nil, cursorErr
			}
			if err = DB.Model(&AIPDDFinanceOutbox{}).Where("channel_id = ? AND instance_id = ? AND state = ?", channel.Id, currentInstance, AIPDDFinanceOutboxPending).Count(&status.BacklogCount).Error; err != nil {
				return nil, err
			}
			if err = DB.Model(&AIPDDFinanceOutbox{}).Where("channel_id = ? AND instance_id = ? AND state = ?", channel.Id, currentInstance, AIPDDFinanceOutboxDead).Count(&status.DeadCount).Error; err != nil {
				return nil, err
			}
			if err = DB.Model(&AIPDDFinanceOutbox{}).Where("channel_id = ? AND instance_id = ? AND state = ?", channel.Id, currentInstance, AIPDDFinanceOutboxIgnored).Count(&status.IgnoredCount).Error; err != nil {
				return nil, err
			}
			if status.LastError == "" {
				var failed AIPDDFinanceOutbox
				failedErr := DB.Where("channel_id = ? AND instance_id = ? AND state IN ? AND last_error <> ''",
					channel.Id, currentInstance, []string{AIPDDFinanceOutboxPending, AIPDDFinanceOutboxDead}).
					Order("updated_at desc").First(&failed).Error
				if failedErr == nil {
					status.LastError, status.LastErrorAt = failed.LastError, failed.UpdatedAt
				} else if !errors.Is(failedErr, gorm.ErrRecordNotFound) {
					return nil, failedErr
				}
			}
			statuses = append(statuses, status)
		}
	}
	return statuses, nil
}

func QueueAIPDDFinanceManualRetry(filter AIPDDFinanceOrderFilter) (int64, error) {
	var orders []AIPDDFinanceOrder
	if err := aipddFinanceOrderQuery(filter).Limit(500).Find(&orders).Error; err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	var queued int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		for _, order := range orders {
			key := "manual-refresh:" + uuid.NewString()
			if err := enqueueAIPDDFinanceOutbox(tx, key, order.PlatformOrderID, order.ChannelID, order.InstanceID, "REFRESH", map[string]any{"requested_at": now}, now); err != nil {
				return err
			}
			queued++
		}
		return nil
	})
	return queued, err
}

func CreateAIPDDFinanceExportJob(filter AIPDDFinanceOrderFilter, createdBy int) (*AIPDDFinanceExportJob, error) {
	body, err := common.Marshal(filter)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	job := &AIPDDFinanceExportJob{ID: uuid.NewString(), Status: AIPDDFinanceExportPending, FilterJSON: string(body),
		ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", CreatedBy: createdBy,
		CreatedAt: now, ExpiresAt: now + 7*24*3600}
	return job, DB.Create(job).Error
}

func GetAIPDDFinanceExportJob(id string) (*AIPDDFinanceExportJob, error) {
	var job AIPDDFinanceExportJob
	if err := DB.Where("id = ?", id).First(&job).Error; err != nil {
		return nil, err
	}
	return &job, nil
}

func ListAIPDDFinanceExportJobs(limit int) ([]AIPDDFinanceExportJob, error) {
	var jobs []AIPDDFinanceExportJob
	err := DB.Select("id", "status", "file_name", "content_type", "sha256", "row_count", "failure_cause", "created_by", "created_at", "started_at", "completed_at", "expires_at").
		Order("created_at desc").Limit(max(1, min(100, limit))).Find(&jobs).Error
	return jobs, err
}

func MarkAIPDDFinanceExportRunning(id string) error {
	return DB.Model(&AIPDDFinanceExportJob{}).Where("id = ? AND status = ?", id, AIPDDFinanceExportPending).
		Updates(map[string]any{"status": AIPDDFinanceExportRunning, "started_at": time.Now().Unix()}).Error
}

func CompleteAIPDDFinanceExport(id, fileName, sha256 string, data []byte, rows int64) error {
	now := time.Now().Unix()
	return DB.Model(&AIPDDFinanceExportJob{}).Where("id = ?", id).Updates(map[string]any{
		"status": AIPDDFinanceExportReady, "file_name": fileName, "sha256": sha256,
		"file_data": data, "row_count": rows, "failure_cause": "", "completed_at": now,
	}).Error
}

func FailAIPDDFinanceExport(id string, cause error) error {
	message := "unknown export error"
	if cause != nil {
		message = cause.Error()
	}
	return DB.Model(&AIPDDFinanceExportJob{}).Where("id = ?", id).Updates(map[string]any{
		"status": AIPDDFinanceExportFailed, "failure_cause": message, "completed_at": time.Now().Unix(),
	}).Error
}
