package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAIPDDFinanceReportSeparatesConfirmedAndEstimatedProfit(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:aipdd-report-%d?mode=memory&cache=shared", time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = previousDB })
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &Channel{}, &AIPDDFinanceOrder{}, &AIPDDFinanceMovement{}, &AIPDDFinanceInbox{}, &AIPDDFinanceOutbox{}, &AIPDDFinanceCursor{}, &AIPDDFinanceExportJob{}))
	require.NoError(t, db.Create(&User{Id: 71, Username: "finance-user", DisplayName: "Finance User", Password: "unused-password"}).Error)
	require.NoError(t, db.Create(&Token{Id: 81, UserId: 71, Name: "production-key", Key: "sk-sensitive-raw-token-value"}).Error)
	require.NoError(t, db.Create(&Channel{Id: 91, Name: "AIPDD Production", Type: 58, Key: "upstream-secret"}).Error)

	confirmedCost := int64(3_000_000)
	confirmedProfit := int64(2_000_000)
	pendingCost := int64(2_000_000)
	now := time.Now().Unix()
	orders := []AIPDDFinanceOrder{
		{ID: uuid.NewString(), PlatformOrderID: "confirmed-order", InstanceID: uuid.NewString(), UserID: 71, TokenID: 81, ChannelID: 91,
			Model: "model-a", OrderStatus: "SUCCEEDED", LocalBillingStatus: "CHARGED", CostStatus: "CONFIRMED",
			CustomerChargeRMBMic: 5_000_000, AIPDDChargeRMBMic: &confirmedCost, ProfitRMBMic: &confirmedProfit, OccurredAt: now, CreatedAt: now, UpdatedAt: now},
		{ID: uuid.NewString(), PlatformOrderID: "pending-order", InstanceID: uuid.NewString(), UserID: 71, TokenID: 81, ChannelID: 91,
			Model: "model-b", OrderStatus: "SUCCEEDED", LocalBillingStatus: "CHARGED", CostStatus: "PARTIAL",
			CustomerChargeRMBMic: 4_000_000, AIPDDChargeRMBMic: &pendingCost, OccurredAt: now, CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, db.Create(&orders).Error)

	summary, err := GetAIPDDFinanceSummary(AIPDDFinanceOrderFilter{})
	require.NoError(t, err)
	require.EqualValues(t, 9_000_000, summary.CustomerNetConsumptionRMBMic)
	require.EqualValues(t, 3_000_000, summary.ConfirmedSourceCostRMBMic)
	require.EqualValues(t, 2_000_000, summary.ConfirmedProfitRMBMic)
	require.EqualValues(t, 5_000_000, summary.EstimatedSourceCostRMBMic)
	require.EqualValues(t, 4_000_000, summary.EstimatedProfitRMBMic)
	require.EqualValues(t, 1, summary.PendingConfirmationCount)

	items, total, err := ListAIPDDFinanceOrders(0, 50, AIPDDFinanceOrderFilter{UserQuery: "finance-user", TokenQuery: "production-key"})
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, items, 2)
	require.Equal(t, "production-key", items[0].TokenName)
	require.NotEqual(t, "sk-sensitive-raw-token-value", items[0].TokenMaskedKey)
	encoded, err := common.Marshal(items)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(encoded), "sk-sensitive-raw-token-value"))
	for _, item := range items {
		if item.PlatformOrderID == "pending-order" {
			require.Nil(t, item.ConfirmedProfitRMBMic, "pending cost must not appear as confirmed profit")
		}
	}
}
