package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAIPDDTransitOrderKeepsOneGlobalOrderAndStoresSourceSale(t *testing.T) {
	previousDB := DB
	dsn := fmt.Sprintf("file:aipdd-transit-order-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = previousDB })
	require.NoError(t, db.AutoMigrate(&AIPDDTransitOrder{}))

	const orderID = "global-order-1"
	require.NoError(t, EnsureAIPDDTransitOrder(
		"11111111-2222-4333-8444-555555555555", orderID, orderID+":0:30", 10, 20, 30, 0, "model-a"))
	require.NoError(t, EnsureAIPDDTransitOrder(
		"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", orderID, orderID+":1:31", 10, 20, 31, 2, "model-b"))

	var count int64
	require.NoError(t, db.Model(&AIPDDTransitOrder{}).Where("platform_order_id = ?", orderID).Count(&count).Error)
	require.EqualValues(t, 1, count)

	estimatedAWCoin := int64(41)
	estimatedRMBMic := int64(4_100)
	require.NoError(t, RecordAIPDDTransitLocalSettlement(
		orderID, 2_000, 4_000_000, "CHARGED", &estimatedAWCoin, &estimatedRMBMic))
	require.NoError(t, ApplyAIPDDTransitSourceSettlement(orderID, 1_250, 2_500_000))

	order, err := GetAIPDDTransitOrder(orderID)
	require.NoError(t, err)
	require.Equal(t, AIPDDTransitSettled, order.Status)
	require.Equal(t, 31, order.ChannelID)
	require.Equal(t, 2, order.ChannelKeyIndex)
	require.Equal(t, orderID+":1:31", order.LatestAttemptID)
	require.Equal(t, "model-b", order.Model)
	require.EqualValues(t, 2_000, order.CustomerChargeQuota)
	require.EqualValues(t, 4_000_000, order.CustomerChargeRMBMic)
	require.EqualValues(t, 1_250, *order.SourceChargeAWCoin)
	require.EqualValues(t, 2_500_000, *order.SourceChargeRMBMic)
	// A real settlement must never overwrite the estimate it was compared against.
	require.EqualValues(t, 41, *order.EstimatedSourceAWCoin)
	require.EqualValues(t, 4_100, *order.EstimatedSourceRMBMic)

	// Replaying the same transport attempt must not reopen or erase settlement.
	require.NoError(t, EnsureAIPDDTransitOrder(
		"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", orderID, orderID+":1:31", 10, 20, 31, 2, "model-b"))
	order, err = GetAIPDDTransitOrder(orderID)
	require.NoError(t, err)
	require.Equal(t, AIPDDTransitSettled, order.Status)
	require.EqualValues(t, 1_250, *order.SourceChargeAWCoin)

	// A genuine retry attempt reopens the mirror, and the stale estimate goes
	// with the stale settlement.
	require.NoError(t, EnsureAIPDDTransitOrder(
		"aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee", orderID, orderID+":2:31", 10, 20, 31, 2, "model-b"))
	order, err = GetAIPDDTransitOrder(orderID)
	require.NoError(t, err)
	require.Equal(t, AIPDDTransitPending, order.Status)
	require.Nil(t, order.EstimatedSourceAWCoin)
	require.Nil(t, order.EstimatedSourceRMBMic)
	require.Nil(t, order.SourceChargeAWCoin)
}
