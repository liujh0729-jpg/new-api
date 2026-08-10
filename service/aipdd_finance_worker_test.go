package service

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHandleAIPDDFinanceOutboxFailureIgnoresTerminal404(t *testing.T) {
	previousDB := model.DB
	dsn := fmt.Sprintf("file:aipdd-finance-worker-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })
	require.NoError(t, db.AutoMigrate(&model.AIPDDFinanceOrder{}, &model.AIPDDFinanceOutbox{}, &model.AIPDDFinanceMovement{}))

	instanceID := uuid.NewString()
	orderID := "worker-orphan-" + uuid.NewString()
	require.NoError(t, model.EnsureAIPDDFinanceOrder(instanceID, orderID, orderID+":0:7", 1, 2, 7, "test-model"))
	require.NoError(t, model.RecordLocalAIPDDFinanceSettlement(instanceID, orderID, 7, 0, 0, `{"quota_per_unit":"500000"}`, "REFUNDED"))

	var event model.AIPDDFinanceOutbox
	require.NoError(t, db.Where("platform_order_id = ? AND state = ?", orderID, model.AIPDDFinanceOutboxPending).
		First(&event).Error)

	handled := handleAIPDDFinanceOutboxFailure(&event, &aipddFinanceHTTPError{
		StatusCode: http.StatusNotFound, Body: `{"detail":"not found"}`,
	})
	require.True(t, handled)
	require.NoError(t, db.Where("id = ?", event.ID).First(&event).Error)
	require.Equal(t, model.AIPDDFinanceOutboxIgnored, event.State)
}

func TestIsAIPDDFinanceNotFound(t *testing.T) {
	require.True(t, isAIPDDFinanceNotFound(&aipddFinanceHTTPError{StatusCode: 404, Body: "missing"}))
	require.False(t, isAIPDDFinanceNotFound(&aipddFinanceHTTPError{StatusCode: 500, Body: "boom"}))
	require.True(t, isAIPDDFinanceNotFound(errors.New("AIPDD finance endpoint returned 404: gone")))
}
