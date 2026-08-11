package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AIPDDTransitPending  = "PENDING"
	AIPDDTransitSettled  = "SETTLED"
	AIPDDTransitFailed   = "FAILED"
	AIPDDTransitRefunded = "REFUNDED"
)

// AIPDDTransitOrder is the small NewAPI-side mirror for one request routed to
// AIPDD. AIPDD's sale is NewAPI's source cost; AIPDD's internal cost/profit are
// intentionally not copied here.
type AIPDDTransitOrder struct {
	ID                   string `json:"id" gorm:"type:varchar(36);primaryKey"`
	PlatformOrderID      string `json:"platform_order_id" gorm:"type:varchar(191);uniqueIndex"`
	InstanceID           string `json:"instance_id" gorm:"type:varchar(36);index"`
	UserID               int    `json:"user_id" gorm:"index"`
	TokenID              int    `json:"token_id" gorm:"index"`
	ChannelID            int    `json:"channel_id" gorm:"index"`
	ChannelKeyIndex      int    `json:"-"`
	Model                string `json:"model" gorm:"type:varchar(191);index"`
	Status               string `json:"status" gorm:"type:varchar(24);index"`
	CustomerChargeQuota  int64  `json:"customer_charge_quota" gorm:"bigint;default:0"`
	CustomerChargeRMBMic int64  `json:"customer_charge_rmb_mic" gorm:"bigint;default:0"`
	SourceChargeAWCoin   *int64 `json:"source_charge_awcoin" gorm:"column:source_charge_awcoin;bigint"`
	SourceChargeRMBMic   *int64 `json:"source_charge_rmb_mic" gorm:"bigint"`
	CreatedAt            int64  `json:"created_at" gorm:"bigint;index"`
	SettledAt            *int64 `json:"settled_at" gorm:"bigint;index"`
	UpdatedAt            int64  `json:"updated_at" gorm:"bigint"`
}

func (AIPDDTransitOrder) TableName() string { return "aipdd_transit_order" }

func EnsureAIPDDTransitOrder(
	instanceID, platformOrderID string,
	userID, tokenID, channelID, channelKeyIndex int,
	modelName string,
) error {
	now := time.Now().Unix()
	order := &AIPDDTransitOrder{
		ID: uuid.NewString(), PlatformOrderID: platformOrderID, InstanceID: instanceID,
		UserID: userID, TokenID: tokenID, ChannelID: channelID, ChannelKeyIndex: channelKeyIndex,
		Model: modelName, Status: AIPDDTransitPending, CreatedAt: now, UpdatedAt: now,
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "platform_order_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"instance_id": instanceID, "channel_id": channelID,
			"channel_key_index": channelKeyIndex, "model": modelName,
			"status": AIPDDTransitPending, "source_charge_awcoin": nil,
			"source_charge_rmb_mic": nil, "settled_at": nil, "updated_at": now,
		}),
	}).Create(order).Error
}

func RecordAIPDDTransitLocalSettlement(
	platformOrderID string,
	quota, rmbMic int64,
	status string,
) error {
	now := time.Now().Unix()
	updates := map[string]any{
		"customer_charge_quota":   quota,
		"customer_charge_rmb_mic": rmbMic,
		"updated_at":              now,
	}
	switch status {
	case "NOT_CHARGED":
		updates["status"] = AIPDDTransitFailed
		updates["settled_at"] = now
	case "REFUNDED":
		updates["status"] = AIPDDTransitRefunded
		updates["settled_at"] = now
	}
	return DB.Model(&AIPDDTransitOrder{}).
		Where("platform_order_id = ?", platformOrderID).
		Updates(updates).Error
}

func ApplyAIPDDTransitSourceSettlement(
	platformOrderID string,
	chargedAWCoin, chargedRMBMic int64,
) error {
	now := time.Now().Unix()
	return DB.Model(&AIPDDTransitOrder{}).
		Where("platform_order_id = ?", platformOrderID).
		Updates(map[string]any{
			"source_charge_awcoin":  chargedAWCoin,
			"source_charge_rmb_mic": chargedRMBMic,
			"status":                AIPDDTransitSettled,
			"settled_at":            now,
			"updated_at":            now,
		}).Error
}

func GetAIPDDTransitOrder(platformOrderID string) (*AIPDDTransitOrder, error) {
	var order AIPDDTransitOrder
	if err := DB.Where("platform_order_id = ?", platformOrderID).First(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

// AIPDDTransitOrderQuery holds admin list filters for aipdd_transit_order.
type AIPDDTransitOrderQuery struct {
	StartTimestamp  int64
	EndTimestamp    int64
	PlatformOrderID string
	UserID          *int
	TokenID         *int
	ChannelID       *int
	Model           string
	Status          string
	StartIdx        int
	PageSize        int
}

// AIPDDTransitOrderItem is the admin-facing list row. Missing user/token/channel
// associations still return the order IDs; source charges stay null until settled.
type AIPDDTransitOrderItem struct {
	PlatformOrderID     string   `json:"platform_order_id"`
	UserID              int      `json:"user_id"`
	Username            string   `json:"username"`
	TokenID             int      `json:"token_id"`
	TokenName           string   `json:"token_name"`
	ChannelID           int      `json:"channel_id"`
	ChannelName         string   `json:"channel_name"`
	ChannelKeyIndex     int      `json:"channel_key_index"`
	Model               string   `json:"model"`
	Status              string   `json:"status"`
	CustomerChargeQuota int64    `json:"customer_charge_quota"`
	CustomerChargeRMB   float64  `json:"customer_charge_rmb"`
	SourceChargeAWCoin  *int64   `json:"source_charge_awcoin"`
	SourceChargeRMB     *float64 `json:"source_charge_rmb"`
	CreatedAt           int64    `json:"created_at"`
	SettledAt           *int64   `json:"settled_at"`
}

func applyAIPDDTransitOrderFilters(tx *gorm.DB, query AIPDDTransitOrderQuery) *gorm.DB {
	if query.StartTimestamp > 0 {
		tx = tx.Where("created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		tx = tx.Where("created_at <= ?", query.EndTimestamp)
	}
	if query.PlatformOrderID != "" {
		tx = tx.Where("platform_order_id = ?", query.PlatformOrderID)
	}
	if query.UserID != nil {
		tx = tx.Where("user_id = ?", *query.UserID)
	}
	if query.TokenID != nil {
		tx = tx.Where("token_id = ?", *query.TokenID)
	}
	if query.ChannelID != nil {
		tx = tx.Where("channel_id = ?", *query.ChannelID)
	}
	if query.Model != "" {
		tx = tx.Where("model = ?", query.Model)
	}
	if query.Status != "" {
		tx = tx.Where("status = ?", query.Status)
	}
	return tx
}

func GetAIPDDTransitOrders(query AIPDDTransitOrderQuery) ([]*AIPDDTransitOrderItem, int64, error) {
	base := applyAIPDDTransitOrderFilters(DB.Model(&AIPDDTransitOrder{}), query)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var orders []AIPDDTransitOrder
	listTx := applyAIPDDTransitOrderFilters(DB.Model(&AIPDDTransitOrder{}), query).
		Order("created_at DESC").
		Offset(query.StartIdx).
		Limit(query.PageSize)
	if err := listTx.Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	userIDs := make([]int, 0, len(orders))
	tokenIDs := make([]int, 0, len(orders))
	channelIDs := make([]int, 0, len(orders))
	seenUsers := map[int]struct{}{}
	seenTokens := map[int]struct{}{}
	seenChannels := map[int]struct{}{}
	for _, order := range orders {
		if order.UserID > 0 {
			if _, ok := seenUsers[order.UserID]; !ok {
				seenUsers[order.UserID] = struct{}{}
				userIDs = append(userIDs, order.UserID)
			}
		}
		if order.TokenID > 0 {
			if _, ok := seenTokens[order.TokenID]; !ok {
				seenTokens[order.TokenID] = struct{}{}
				tokenIDs = append(tokenIDs, order.TokenID)
			}
		}
		if order.ChannelID > 0 {
			if _, ok := seenChannels[order.ChannelID]; !ok {
				seenChannels[order.ChannelID] = struct{}{}
				channelIDs = append(channelIDs, order.ChannelID)
			}
		}
	}

	usernameByID := map[int]string{}
	if len(userIDs) > 0 {
		var users []User
		// Unscoped so soft-deleted users still contribute a display name.
		if err := DB.Unscoped().Select("id, username").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, 0, err
		}
		for _, user := range users {
			usernameByID[user.Id] = user.Username
		}
	}

	tokenNameByID := map[int]string{}
	if len(tokenIDs) > 0 {
		var tokens []Token
		if err := DB.Unscoped().Select("id, name").Where("id IN ?", tokenIDs).Find(&tokens).Error; err != nil {
			return nil, 0, err
		}
		for _, token := range tokens {
			tokenNameByID[token.Id] = token.Name
		}
	}

	channelNameByID := map[int]string{}
	if len(channelIDs) > 0 {
		var channels []Channel
		if err := DB.Select("id, name").Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
			return nil, 0, err
		}
		for _, channel := range channels {
			channelNameByID[channel.Id] = channel.Name
		}
	}

	items := make([]*AIPDDTransitOrderItem, 0, len(orders))
	for i := range orders {
		order := &orders[i]
		item := &AIPDDTransitOrderItem{
			PlatformOrderID:     order.PlatformOrderID,
			UserID:              order.UserID,
			Username:            usernameByID[order.UserID],
			TokenID:             order.TokenID,
			TokenName:           tokenNameByID[order.TokenID],
			ChannelID:           order.ChannelID,
			ChannelName:         channelNameByID[order.ChannelID],
			ChannelKeyIndex:     order.ChannelKeyIndex,
			Model:               order.Model,
			Status:              order.Status,
			CustomerChargeQuota: order.CustomerChargeQuota,
			CustomerChargeRMB:   float64(order.CustomerChargeRMBMic) / 1_000_000,
			CreatedAt:           order.CreatedAt,
			SettledAt:           order.SettledAt,
		}
		if order.SourceChargeAWCoin != nil {
			aw := *order.SourceChargeAWCoin
			item.SourceChargeAWCoin = &aw
		}
		if order.SourceChargeRMBMic != nil {
			rmb := float64(*order.SourceChargeRMBMic) / 1_000_000
			item.SourceChargeRMB = &rmb
		}
		items = append(items, item)
	}
	return items, total, nil
}
