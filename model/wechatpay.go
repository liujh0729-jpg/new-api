package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const WechatPayConfigScopeDefault = "default"

type WechatPayConfig struct {
	Id                             int    `json:"id"`
	Scope                          string `json:"-" gorm:"type:varchar(32);uniqueIndex;not null"`
	Enabled                        bool   `json:"enabled"`
	ShowEpayWechat                 bool   `json:"show_epay_wechat"`
	AppId                          string `json:"appid" gorm:"type:varchar(32);not null"`
	MchId                          string `json:"mchid" gorm:"type:varchar(32);not null"`
	MerchantCertificateSerial      string `json:"merchant_certificate_serial" gorm:"type:varchar(128);not null"`
	MerchantCertificateFingerprint string `json:"merchant_certificate_fingerprint" gorm:"type:varchar(128);not null"`
	WechatPayPublicKeyId           string `json:"wechatpay_public_key_id" gorm:"type:varchar(128);not null"`
	WechatPayPublicKeyFingerprint  string `json:"wechatpay_public_key_fingerprint" gorm:"type:varchar(128);not null"`
	MerchantCertificateEncrypted   string `json:"-" gorm:"type:text;not null"`
	MerchantPrivateKeyEncrypted    string `json:"-" gorm:"type:text;not null"`
	ApiV3KeyEncrypted              string `json:"-" gorm:"type:text;not null"`
	WechatPayPublicKeyEncrypted    string `json:"-" gorm:"type:text;not null"`
	ValidatedAt                    int64  `json:"validated_at"`
	VerifiedAt                     int64  `json:"verified_at"`
	CreateTime                     int64  `json:"create_time"`
	UpdateTime                     int64  `json:"update_time"`
}

type WechatPayTestOrder struct {
	Id                    int     `json:"id"`
	TradeNo               string  `json:"trade_no" gorm:"type:varchar(32);uniqueIndex;not null"`
	PaymentAmountCents    int64   `json:"payment_amount_cents"`
	Currency              string  `json:"currency" gorm:"type:varchar(8);not null"`
	Status                string  `json:"status" gorm:"type:varchar(20);index;not null"`
	CodeUrl               string  `json:"-" gorm:"type:text"`
	ProviderTransactionId *string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	NotifyId              *string `json:"-" gorm:"type:varchar(64);uniqueIndex"`
	ProviderQueryTime     int64   `json:"-"`
	ExpireTime            int64   `json:"expire_time"`
	CreateTime            int64   `json:"create_time"`
	CompleteTime          int64   `json:"complete_time"`
}

type WechatPaySettlementResult struct {
	Settled       bool
	UserId        int
	QuotaToAdd    int
	Money         float64
	PaymentMethod string
}

func GetWechatPayConfig() (*WechatPayConfig, error) {
	config := &WechatPayConfig{}
	err := DB.Where("scope = ?", WechatPayConfigScopeDefault).First(config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return config, nil
}

func SaveWechatPayConfig(config *WechatPayConfig) error {
	if config == nil {
		return errors.New("wechat pay config is required")
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		existing := &WechatPayConfig{}
		err := tx.Where("scope = ?", WechatPayConfigScopeDefault).First(existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			config.Id = 0
			config.Scope = WechatPayConfigScopeDefault
			config.CreateTime = now
			config.UpdateTime = now
			return tx.Create(config).Error
		}
		if err != nil {
			return err
		}
		config.Id = existing.Id
		config.Scope = WechatPayConfigScopeDefault
		config.CreateTime = existing.CreateTime
		config.UpdateTime = now
		return tx.Save(config).Error
	})
}

func HasActivePendingWechatPayOrders(now int64) (bool, error) {
	var count int64
	if err := DB.Model(&TopUp{}).
		Where("payment_provider = ? AND status = ? AND expire_time > ?", PaymentProviderWechatPay, common.TopUpStatusPending, now).
		Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := DB.Model(&WechatPayTestOrder{}).
		Where("status = ? AND expire_time > ?", common.TopUpStatusPending, now).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func FindReusableWechatPayTopUp(userId int, amount int64, paymentAmountCents int64, quotaToAdd int64, now int64) (*TopUp, error) {
	order := &TopUp{}
	err := DB.Where(
		"user_id = ? AND payment_provider = ? AND amount = ? AND payment_amount_cents = ? AND quota_to_add = ? AND status = ? AND expire_time > ?",
		userId, PaymentProviderWechatPay, amount, paymentAmountCents, quotaToAdd, common.TopUpStatusPending, now,
	).Order("id desc").First(order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return order, err
}

func GetWechatPayTopUpByTradeNo(tradeNo string) (*TopUp, error) {
	order := &TopUp{}
	err := DB.Where("trade_no = ? AND payment_provider = ?", tradeNo, PaymentProviderWechatPay).First(order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTopUpNotFound
	}
	return order, err
}

func GetWechatPayTopUpForUser(userId int, tradeNo string) (*TopUp, error) {
	order := &TopUp{}
	err := DB.Where("user_id = ? AND trade_no = ? AND payment_provider = ?", userId, tradeNo, PaymentProviderWechatPay).First(order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTopUpNotFound
	}
	return order, err
}

func SetWechatPayTopUpCodeUrl(tradeNo string, codeUrl string) error {
	result := DB.Model(&TopUp{}).Where(
		"trade_no = ? AND payment_provider = ? AND status = ?",
		tradeNo, PaymentProviderWechatPay, common.TopUpStatusPending,
	).Update("code_url", codeUrl)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTopUpStatusInvalid
	}
	return nil
}

func TryReserveWechatPayTopUpQuery(tradeNo string, now int64, intervalSeconds int64) (bool, error) {
	result := DB.Model(&TopUp{}).Where(
		"trade_no = ? AND payment_provider = ? AND status = ? AND provider_query_time <= ?",
		tradeNo, PaymentProviderWechatPay, common.TopUpStatusPending, now-intervalSeconds,
	).Update("provider_query_time", now)
	return result.RowsAffected == 1, result.Error
}

func UpdateWechatPayTopUpPendingStatus(tradeNo string, status string) error {
	result := DB.Model(&TopUp{}).Where(
		"trade_no = ? AND payment_provider = ? AND status = ?",
		tradeNo, PaymentProviderWechatPay, common.TopUpStatusPending,
	).Updates(map[string]interface{}{"status": status, "complete_time": common.GetTimestamp()})
	return result.Error
}

func SettleWechatPayTopUp(tradeNo string, transactionId string, paidCents int64, currency string) (WechatPaySettlementResult, error) {
	result := WechatPaySettlementResult{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		order := &TopUp{}
		if err := tx.Where("trade_no = ? AND payment_provider = ?", tradeNo, PaymentProviderWechatPay).First(order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if order.PaymentAmountCents != paidCents || order.Currency != currency {
			return fmt.Errorf("wechat pay amount mismatch")
		}
		if order.Status == common.TopUpStatusSuccess {
			if order.ProviderTransactionId != nil && *order.ProviderTransactionId != transactionId {
				return errors.New("wechat pay transaction mismatch")
			}
			result = WechatPaySettlementResult{UserId: order.UserId, Money: order.Money, PaymentMethod: order.PaymentMethod}
			return nil
		}
		if order.Status != common.TopUpStatusPending && order.Status != common.TopUpStatusExpired {
			return ErrTopUpStatusInvalid
		}

		quota64 := order.QuotaToAdd
		maxInt := int64(^uint(0) >> 1)
		if quota64 <= 0 || quota64 > maxInt {
			return errors.New("invalid topup quota")
		}
		now := common.GetTimestamp()
		update := tx.Model(&TopUp{}).Where(
			"id = ? AND status IN ? AND payment_provider = ?",
			order.Id, []string{common.TopUpStatusPending, common.TopUpStatusExpired}, PaymentProviderWechatPay,
		).Updates(map[string]interface{}{
			"status":                  common.TopUpStatusSuccess,
			"complete_time":           now,
			"provider_transaction_id": transactionId,
		})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			current := &TopUp{}
			if err := tx.Select("status", "provider_transaction_id").First(current, order.Id).Error; err != nil {
				return err
			}
			if current.Status == common.TopUpStatusSuccess && current.ProviderTransactionId != nil && *current.ProviderTransactionId == transactionId {
				result = WechatPaySettlementResult{UserId: order.UserId, Money: order.Money, PaymentMethod: order.PaymentMethod}
				return nil
			}
			return ErrTopUpStatusInvalid
		}
		userUpdate := tx.Model(&User{}).Where("id = ?", order.UserId).
			Update("quota", gorm.Expr("quota + ?", int(quota64)))
		if userUpdate.Error != nil {
			return userUpdate.Error
		}
		if userUpdate.RowsAffected != 1 {
			return errors.New("topup user not found")
		}
		result = WechatPaySettlementResult{
			Settled: true, UserId: order.UserId, QuotaToAdd: int(quota64),
			Money: order.Money, PaymentMethod: order.PaymentMethod,
		}
		return nil
	})
	return result, err
}

func FindActiveWechatPayTestOrder(now int64) (*WechatPayTestOrder, error) {
	order := &WechatPayTestOrder{}
	err := DB.Where("status = ? AND expire_time > ?", common.TopUpStatusPending, now).Order("id desc").First(order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return order, err
}

func GetWechatPayTestOrder(tradeNo string) (*WechatPayTestOrder, error) {
	order := &WechatPayTestOrder{}
	err := DB.Where("trade_no = ?", tradeNo).First(order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return order, err
}

func SetWechatPayTestOrderCodeUrl(tradeNo string, codeUrl string) error {
	result := DB.Model(&WechatPayTestOrder{}).Where("trade_no = ? AND status = ?", tradeNo, common.TopUpStatusPending).
		Update("code_url", codeUrl)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTopUpStatusInvalid
	}
	return nil
}

func InsertWechatPayTestOrder(order *WechatPayTestOrder) error {
	return DB.Create(order).Error
}

func TryReserveWechatPayTestQuery(tradeNo string, now int64, intervalSeconds int64) (bool, error) {
	result := DB.Model(&WechatPayTestOrder{}).Where(
		"trade_no = ? AND status = ? AND provider_query_time <= ?",
		tradeNo, common.TopUpStatusPending, now-intervalSeconds,
	).Update("provider_query_time", now)
	return result.RowsAffected == 1, result.Error
}

func UpdateWechatPayTestPendingStatus(tradeNo string, status string) error {
	return DB.Model(&WechatPayTestOrder{}).Where("trade_no = ? AND status = ?", tradeNo, common.TopUpStatusPending).
		Updates(map[string]interface{}{"status": status, "complete_time": common.GetTimestamp()}).Error
}

func CompleteWechatPayTestOrder(tradeNo string, transactionId string, notifyId string, paidCents int64, currency string) (bool, error) {
	completed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		order := &WechatPayTestOrder{}
		if err := tx.Where("trade_no = ?", tradeNo).First(order).Error; err != nil {
			return err
		}
		if order.PaymentAmountCents != paidCents || order.Currency != currency {
			return errors.New("wechat pay test amount mismatch")
		}
		if order.Status == common.TopUpStatusSuccess {
			if order.ProviderTransactionId != nil && *order.ProviderTransactionId != transactionId {
				return errors.New("wechat pay test transaction mismatch")
			}
			return nil
		}
		if order.Status != common.TopUpStatusPending && order.Status != common.TopUpStatusExpired {
			return ErrTopUpStatusInvalid
		}
		updates := map[string]interface{}{
			"status":                  common.TopUpStatusSuccess,
			"complete_time":           common.GetTimestamp(),
			"provider_transaction_id": transactionId,
		}
		if notifyId != "" {
			updates["notify_id"] = notifyId
		}
		update := tx.Model(&WechatPayTestOrder{}).
			Where("id = ? AND status IN ?", order.Id, []string{common.TopUpStatusPending, common.TopUpStatusExpired}).
			Updates(updates)
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			current := &WechatPayTestOrder{}
			if err := tx.Select("status", "provider_transaction_id").First(current, order.Id).Error; err != nil {
				return err
			}
			if current.Status == common.TopUpStatusSuccess && current.ProviderTransactionId != nil && *current.ProviderTransactionId == transactionId {
				return nil
			}
			return ErrTopUpStatusInvalid
		}
		if err := tx.Model(&WechatPayConfig{}).Where("scope = ?", WechatPayConfigScopeDefault).
			Update("verified_at", common.GetTimestamp()).Error; err != nil {
			return err
		}
		completed = true
		return nil
	})
	return completed, err
}

func ExpireStaleWechatPayOrders(now int64) error {
	if err := DB.Model(&TopUp{}).Where(
		"payment_provider = ? AND status = ? AND expire_time > 0 AND expire_time <= ?",
		PaymentProviderWechatPay, common.TopUpStatusPending, now,
	).Updates(map[string]interface{}{"status": common.TopUpStatusExpired, "complete_time": now}).Error; err != nil {
		return err
	}
	return DB.Model(&WechatPayTestOrder{}).Where(
		"status = ? AND expire_time > 0 AND expire_time <= ?", common.TopUpStatusPending, now,
	).Updates(map[string]interface{}{"status": common.TopUpStatusExpired, "complete_time": now}).Error
}

func NewWechatPayTestOrder(tradeNo string, expireTime int64) *WechatPayTestOrder {
	return &WechatPayTestOrder{
		TradeNo: tradeNo, PaymentAmountCents: 1, Currency: "CNY",
		Status: common.TopUpStatusPending, CreateTime: time.Now().Unix(), ExpireTime: expireTime,
	}
}
