package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestApplyAIPDDTransitSettlementResponseStoresOnlySourceCharge(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-transit-response-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}))

	const orderID = "newapi-order-1"
	require.NoError(t, model.EnsureAIPDDTransitOrder(
		"11111111-2222-4333-8444-555555555555", orderID, 1, 2, 3, 0, "model-a"))
	require.NoError(t, ApplyAIPDDTransitSettlementResponse(
		&relaycommon.AIPDDFinanceContext{PlatformOrderID: orderID},
		[]byte(`{"code":0,"data":{"settlement":{"status":"settled","charged_points":1250,"charged_rmb":"2.500000"}}}`)))

	order, err := model.GetAIPDDTransitOrder(orderID)
	require.NoError(t, err)
	require.Equal(t, model.AIPDDTransitSettled, order.Status)
	require.EqualValues(t, 1250, *order.SourceChargeAWCoin)
	require.EqualValues(t, 2_500_000, *order.SourceChargeRMBMic)
}

func TestApplyAIPDDTransitSettlementResponseAcceptsOfficialTopLevelShape(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-transit-official-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}))

	const orderID = "newapi-order-2"
	require.NoError(t, model.EnsureAIPDDTransitOrder(
		"11111111-2222-4333-8444-555555555555", orderID, 1, 2, 3, 0, "model-b"))
	require.NoError(t, ApplyAIPDDTransitSettlementResponse(
		&relaycommon.AIPDDFinanceContext{PlatformOrderID: orderID},
		[]byte(`{"id":"task-1","status":"succeeded","settlement":{"status":"settled","charged_points":800,"charged_rmb":"1.600000"}}`)))

	order, err := model.GetAIPDDTransitOrder(orderID)
	require.NoError(t, err)
	require.EqualValues(t, 800, *order.SourceChargeAWCoin)
	require.EqualValues(t, 1_600_000, *order.SourceChargeRMBMic)
}

func TestSyncAIPDDTaskFinanceFromUpstreamSettlesOfficialBody(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-transit-sync-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}))

	const orderID = "newapi-order-realtime-1"
	require.NoError(t, model.EnsureAIPDDTransitOrder(
		"11111111-2222-4333-8444-555555555555", orderID, 1, 2, 3, 0, "AP Seedance-2.0 轻量版"))
	task := &model.Task{
		Quota: 109589,
		PrivateData: model.TaskPrivateData{
			AIPDDFinance: &relaycommon.AIPDDFinanceContext{PlatformOrderID: orderID, ChannelID: 3},
		},
	}

	SyncAIPDDTaskFinanceFromUpstream(
		task,
		[]byte(`{"id":"cgt-1","status":"succeeded","settlement":{"status":"settled","charged_points":800,"charged_rmb":"8.000000"}}`),
		&relaycommon.TaskInfo{Status: model.TaskStatusSuccess},
	)

	order, err := model.GetAIPDDTransitOrder(orderID)
	require.NoError(t, err)
	require.Equal(t, model.AIPDDTransitSettled, order.Status)
	require.EqualValues(t, 800, *order.SourceChargeAWCoin)
	require.EqualValues(t, 8_000_000, *order.SourceChargeRMBMic)
}

func TestFetchAIPDDTransitSettlementUsesTheSelectedMultiKeyAndMinimalIdentity(t *testing.T) {
	previousDB := model.DB
	previousClient := httpClient
	dsn := fmt.Sprintf("file:aipdd-transit-direct-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = previousDB
		httpClient = previousClient
	})
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.AIPDDTransitOrder{}))

	const (
		orderID   = "newapi-order-chat-1"
		instance  = "11111111-2222-4333-8444-555555555555"
		selected  = "sk-aipdd-selected"
		channelID = 9
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/api/transit/v1/orders/"+orderID+"/settlement", request.URL.Path)
		require.Equal(t, selected, request.Header.Get("X-API-Key"))
		require.Equal(t, "Bearer "+selected, request.Header.Get("Authorization"))
		require.Equal(t, instance, request.Header.Get("X-AIPDD-Instance-ID"))
		require.Empty(t, request.Header.Get("X-AIPDD-Attempt-ID"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":0,"data":{"platform_order_id":"newapi-order-chat-1","settlement":{"status":"settled","charged_points":600,"charged_rmb":"1.200000"}}}`))
	}))
	t.Cleanup(server.Close)
	httpClient = server.Client()
	baseURL := server.URL
	require.NoError(t, db.Create(&model.Channel{
		Id: channelID, Type: 100, Key: "sk-aipdd-other\n" + selected, Name: "AIPDD", BaseURL: &baseURL,
	}).Error)
	require.NoError(t, model.EnsureAIPDDTransitOrder(instance, orderID, 1, 2, channelID, 1, "chat-model"))

	err = fetchAndApplyAIPDDTransitSettlement(&relaycommon.AIPDDFinanceContext{
		InstanceID: instance, PlatformOrderID: orderID, ChannelID: channelID, ChannelKeyIndex: 1,
	})
	require.NoError(t, err)

	order, err := model.GetAIPDDTransitOrder(orderID)
	require.NoError(t, err)
	require.EqualValues(t, 600, *order.SourceChargeAWCoin)
	require.EqualValues(t, 1_200_000, *order.SourceChargeRMBMic)
}
