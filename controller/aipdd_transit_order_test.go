package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type aipddTransitAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type aipddTransitPageData struct {
	Page     int                              `json:"page"`
	PageSize int                              `json:"page_size"`
	Total    int                              `json:"total"`
	Items    []*model.AIPDDTransitOrderItem   `json:"items"`
}

func setupAIPDDTransitOrderTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:aipdd-transit-ctrl-%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
		sqlDB, closeErr := db.DB()
		if closeErr == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.AutoMigrate(&model.AIPDDTransitOrder{}, &model.User{}, &model.Token{}, &model.Channel{}))
	return db
}

func seedAIPDDTransitOrder(
	t *testing.T,
	platformOrderID string,
	userID, tokenID, channelID, keyIndex int,
	modelName, status string,
	createdAt int64,
	customerQuota, customerRMBMic int64,
	sourceAWCoin, sourceRMBMic *int64,
	settledAt *int64,
) {
	t.Helper()
	now := time.Now().Unix()
	order := &model.AIPDDTransitOrder{
		ID:                   common.GetUUID(),
		PlatformOrderID:      platformOrderID,
		InstanceID:           "11111111-2222-4333-8444-555555555555",
		UserID:               userID,
		TokenID:              tokenID,
		ChannelID:            channelID,
		ChannelKeyIndex:      keyIndex,
		Model:                modelName,
		Status:               status,
		CustomerChargeQuota:  customerQuota,
		CustomerChargeRMBMic: customerRMBMic,
		SourceChargeAWCoin:   sourceAWCoin,
		SourceChargeRMBMic:   sourceRMBMic,
		CreatedAt:            createdAt,
		SettledAt:            settledAt,
		UpdatedAt:            now,
	}
	require.NoError(t, model.DB.Create(order).Error)
}

func callGetAIPDDTransitOrders(t *testing.T, target string) aipddTransitPageData {
	t.Helper()
	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, target, nil, 1)
	GetAIPDDTransitOrders(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var response aipddTransitAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, response.Message)

	var page aipddTransitPageData
	require.NoError(t, common.Unmarshal(response.Data, &page))
	return page
}

func TestGetAIPDDTransitOrdersPagination(t *testing.T) {
	setupAIPDDTransitOrderTestDB(t)
	base := time.Now().Unix() - 1000
	for i := 0; i < 5; i++ {
		seedAIPDDTransitOrder(
			t, fmt.Sprintf("order-page-%d", i),
			1, 1, 1, 0, "model-a", model.AIPDDTransitSettled,
			base+int64(i), 100, 1_000_000, int64Ptr(50), int64Ptr(500_000), int64Ptr(base+int64(i)),
		)
	}

	page1 := callGetAIPDDTransitOrders(t, "/api/aipdd-transit-orders?p=1&page_size=2")
	require.Equal(t, 5, page1.Total)
	require.Equal(t, 2, page1.PageSize)
	require.Len(t, page1.Items, 2)
	require.Equal(t, "order-page-4", page1.Items[0].PlatformOrderID)

	page2 := callGetAIPDDTransitOrders(t, "/api/aipdd-transit-orders?p=2&page_size=2")
	require.Equal(t, 5, page2.Total)
	require.Len(t, page2.Items, 2)
	require.Equal(t, "order-page-2", page2.Items[0].PlatformOrderID)
}

func TestGetAIPDDTransitOrdersStatusFilter(t *testing.T) {
	setupAIPDDTransitOrderTestDB(t)
	now := time.Now().Unix()
	seedAIPDDTransitOrder(t, "order-settled", 1, 1, 1, 0, "m", model.AIPDDTransitSettled, now, 1, 1, int64Ptr(1), int64Ptr(1), &now)
	seedAIPDDTransitOrder(t, "order-failed", 1, 1, 1, 0, "m", model.AIPDDTransitFailed, now-1, 0, 0, nil, nil, &now)
	seedAIPDDTransitOrder(t, "order-pending", 1, 1, 1, 0, "m", model.AIPDDTransitPending, now-2, 0, 0, nil, nil, nil)

	page := callGetAIPDDTransitOrders(t, "/api/aipdd-transit-orders?status=FAILED&p=1&page_size=20")
	require.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	require.Equal(t, "order-failed", page.Items[0].PlatformOrderID)
	require.Equal(t, model.AIPDDTransitFailed, page.Items[0].Status)
}

func TestGetAIPDDTransitOrdersTimeRangeFilter(t *testing.T) {
	setupAIPDDTransitOrderTestDB(t)
	seedAIPDDTransitOrder(t, "order-old", 1, 1, 1, 0, "m", model.AIPDDTransitPending, 100, 0, 0, nil, nil, nil)
	seedAIPDDTransitOrder(t, "order-mid", 1, 1, 1, 0, "m", model.AIPDDTransitPending, 200, 0, 0, nil, nil, nil)
	seedAIPDDTransitOrder(t, "order-new", 1, 1, 1, 0, "m", model.AIPDDTransitPending, 300, 0, 0, nil, nil, nil)

	page := callGetAIPDDTransitOrders(t, "/api/aipdd-transit-orders?start_timestamp=150&end_timestamp=250&p=1&page_size=20")
	require.Equal(t, 1, page.Total)
	require.Equal(t, "order-mid", page.Items[0].PlatformOrderID)
}

func TestGetAIPDDTransitOrdersMissingAssociationsStillReturnOrder(t *testing.T) {
	setupAIPDDTransitOrderTestDB(t)
	now := time.Now().Unix()
	seedAIPDDTransitOrder(
		t, "order-orphan",
		999, 998, 997, 3, "orphan-model", model.AIPDDTransitSettled,
		now, 2_000, 4_000_000, int64Ptr(1_250), int64Ptr(2_500_000), &now,
	)

	page := callGetAIPDDTransitOrders(t, "/api/aipdd-transit-orders?platform_order_id=order-orphan&p=1&page_size=20")
	require.Equal(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	item := page.Items[0]
	require.Equal(t, "order-orphan", item.PlatformOrderID)
	require.Equal(t, 999, item.UserID)
	require.Equal(t, 998, item.TokenID)
	require.Equal(t, 997, item.ChannelID)
	require.Equal(t, 3, item.ChannelKeyIndex)
	require.Empty(t, item.Username)
	require.Empty(t, item.TokenName)
	require.Empty(t, item.ChannelName)
	require.EqualValues(t, 2_000, item.CustomerChargeQuota)
	require.InDelta(t, 4.0, item.CustomerChargeRMB, 0.000001)
	require.NotNil(t, item.SourceChargeAWCoin)
	require.EqualValues(t, 1_250, *item.SourceChargeAWCoin)
	require.NotNil(t, item.SourceChargeRMB)
	require.InDelta(t, 2.5, *item.SourceChargeRMB, 0.000001)
}

func TestGetAIPDDTransitOrdersNullSourceCharge(t *testing.T) {
	setupAIPDDTransitOrderTestDB(t)
	now := time.Now().Unix()
	seedAIPDDTransitOrder(
		t, "order-pending-source",
		1, 1, 1, 0, "m", model.AIPDDTransitPending,
		now, 100, 200_000, nil, nil, nil,
	)

	page := callGetAIPDDTransitOrders(t, "/api/aipdd-transit-orders?platform_order_id=order-pending-source")
	require.Equal(t, 1, page.Total)
	item := page.Items[0]
	require.Nil(t, item.SourceChargeAWCoin)
	require.Nil(t, item.SourceChargeRMB)
	require.InDelta(t, 0.2, item.CustomerChargeRMB, 0.000001)

	// Ensure raw JSON keeps null instead of 0 for pending source cost.
	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/aipdd-transit-orders?platform_order_id=order-pending-source", nil, 1)
	GetAIPDDTransitOrders(ctx)
	body := recorder.Body.String()
	require.Contains(t, body, `"source_charge_awcoin":null`)
	require.Contains(t, body, `"source_charge_rmb":null`)
}

func TestGetAIPDDTransitOrdersAdminAuthRequired(t *testing.T) {
	setupAIPDDTransitOrderTestDB(t)
	seedAIPDDTransitOrder(t, "order-auth", 1, 1, 1, 0, "m", model.AIPDDTransitPending, time.Now().Unix(), 0, 0, nil, nil, nil)

	store := cookie.NewStore([]byte("aipdd-transit-order-test-secret"))
	router := gin.New()
	router.Use(sessions.Sessions("session", store))
	router.POST("/session", func(c *gin.Context) {
		var body struct {
			Username string `json:"username"`
			Role     int    `json:"role"`
			ID       int    `json:"id"`
		}
		require.NoError(t, c.ShouldBindJSON(&body))
		session := sessions.Default(c)
		session.Set("username", body.Username)
		session.Set("role", body.Role)
		session.Set("id", body.ID)
		session.Set("status", common.UserStatusEnabled)
		require.NoError(t, session.Save())
		c.Status(http.StatusOK)
	})
	router.GET("/api/aipdd-transit-orders", middleware.AdminAuth(), GetAIPDDTransitOrders)

	loginAs := func(role int) *http.Cookie {
		loginBody := fmt.Sprintf(`{"username":"u","role":%d,"id":1}`, role)
		req := httptest.NewRequest(http.MethodPost, "/session", strings.NewReader(loginBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		for _, cookieValue := range rec.Result().Cookies() {
			if cookieValue.Name == "session" {
				return cookieValue
			}
		}
		t.Fatal("session cookie missing")
		return nil
	}

	commonCookie := loginAs(common.RoleCommonUser)
	commonReq := httptest.NewRequest(http.MethodGet, "/api/aipdd-transit-orders", nil)
	commonReq.AddCookie(commonCookie)
	commonReq.Header.Set("New-Api-User", "1")
	commonRec := httptest.NewRecorder()
	router.ServeHTTP(commonRec, commonReq)
	require.Equal(t, http.StatusOK, commonRec.Code)
	var commonResp aipddTransitAPIResponse
	require.NoError(t, common.Unmarshal(commonRec.Body.Bytes(), &commonResp))
	require.False(t, commonResp.Success)

	adminCookie := loginAs(common.RoleAdminUser)
	adminReq := httptest.NewRequest(http.MethodGet, "/api/aipdd-transit-orders", nil)
	adminReq.AddCookie(adminCookie)
	adminReq.Header.Set("New-Api-User", "1")
	adminRec := httptest.NewRecorder()
	router.ServeHTTP(adminRec, adminReq)
	require.Equal(t, http.StatusOK, adminRec.Code)
	var adminResp aipddTransitAPIResponse
	require.NoError(t, common.Unmarshal(adminRec.Body.Bytes(), &adminResp))
	require.True(t, adminResp.Success, adminResp.Message)
	var page aipddTransitPageData
	require.NoError(t, common.Unmarshal(adminResp.Data, &page))
	require.Equal(t, 1, page.Total)
}

func TestGetAIPDDTransitOrdersWithAssociationNames(t *testing.T) {
	db := setupAIPDDTransitOrderTestDB(t)
	now := time.Now().Unix()

	user := &model.User{Id: 10, Username: "alice", Password: "password123", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	token := &model.Token{Id: 20, UserId: 10, Key: "sk-test-token-abcdefghijklmnop", Name: "alice-token", Status: common.TokenStatusEnabled}
	require.NoError(t, db.Create(token).Error)
	channel := &model.Channel{Id: 30, Name: "aipdd-channel", Key: "secret-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(channel).Error)

	seedAIPDDTransitOrder(
		t, "order-named",
		10, 20, 30, 1, "gpt-test", model.AIPDDTransitSettled,
		now, 500, 1_500_000, int64Ptr(250), int64Ptr(750_000), &now,
	)

	page := callGetAIPDDTransitOrders(t, "/api/aipdd-transit-orders?platform_order_id=order-named")
	require.Equal(t, 1, page.Total)
	item := page.Items[0]
	require.Equal(t, "alice", item.Username)
	require.Equal(t, "alice-token", item.TokenName)
	require.Equal(t, "aipdd-channel", item.ChannelName)
	require.Equal(t, 1, item.ChannelKeyIndex)
}

func int64Ptr(v int64) *int64 { return &v }
