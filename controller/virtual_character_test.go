package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeleteVirtualCharacterDoesNotRecordUserActionAsError(t *testing.T) {
	setupVirtualCharacterControllerTestDB(t)

	slot := 1
	item := &model.VirtualCharacter{
		UserID: 101,
		Slot:   &slot,
		Scope:  model.VirtualCharacterScopePrivate,
		Name:   "test character",
		Status: model.VirtualCharacterStatusActive,
	}
	require.NoError(t, model.DB.Create(item).Error)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.DELETE("/api/virtual-characters/:id", func(c *gin.Context) {
		c.Set("id", item.UserID)
		DeleteVirtualCharacter(c)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/virtual-characters/%d", item.ID), nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var stored model.VirtualCharacter
	require.NoError(t, model.DB.First(&stored, item.ID).Error)
	require.Equal(t, model.VirtualCharacterStatusDeleting, stored.Status)
	require.Nil(t, stored.Slot)
	require.Empty(t, stored.LastError)
}

func setupVirtualCharacterControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.VirtualCharacter{}))
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
	})

	return db
}
