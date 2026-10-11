package controller

import (
	"context"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

type Setup struct {
	Status       bool   `json:"status"`
	RootInit     bool   `json:"root_init"`
	DatabaseType string `json:"database_type"`
}

// SetupWarning reports a step that failed after the core install succeeded. The
// wizard shows these instead of a plain success so an operator learns that, for
// example, the AIPDD catalog never synced and models still need pricing.
type SetupWarning struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

const (
	setupWarningCatalogSync  = "aipdd_catalog_sync"
	setupWarningChannelSetup = "aipdd_channel_setup"
)

// setupErrorDetail keeps the operator-facing text short and credential-free: the
// full error stays in the server log.
func setupErrorDetail(err error, secrets ...string) string {
	return common.TruncateForLog(common.RedactSecrets(err.Error(), secrets...), 200)
}

type SetupRequest struct {
	Username           string `json:"username"`
	Password           string `json:"password"`
	ConfirmPassword    string `json:"confirmPassword"`
	SelfUseModeEnabled bool   `json:"SelfUseModeEnabled"`
	DemoSiteEnabled    bool   `json:"DemoSiteEnabled"`
	AIPDDApiKey        string `json:"aipdd_api_key"`
}

func GetSetup(c *gin.Context) {
	setup := Setup{
		Status: constant.Setup,
	}
	if constant.Setup {
		c.JSON(200, gin.H{
			"success": true,
			"data":    setup,
		})
		return
	}
	setup.RootInit = model.RootUserExists()
	if common.UsingMySQL {
		setup.DatabaseType = "mysql"
	}
	if common.UsingPostgreSQL {
		setup.DatabaseType = "postgres"
	}
	if common.UsingSQLite {
		setup.DatabaseType = "sqlite"
	}
	c.JSON(200, gin.H{
		"success": true,
		"data":    setup,
	})
}

func PostSetup(c *gin.Context) {
	// Check if setup is already completed
	if constant.Setup {
		common.ApiErrorI18n(c, i18n.MsgSetupAlreadyCompleted)
		return
	}

	// Check if root user already exists
	rootExists := model.RootUserExists()

	var req SetupRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}

	// If root doesn't exist, validate and create admin account
	if !rootExists {
		// Validate username length: max 12 characters to align with model.User validation
		if len(req.Username) > 12 {
			common.ApiErrorI18n(c, i18n.MsgSetupUsernameTooLong)
			return
		}
		// Validate password
		if req.Password != req.ConfirmPassword {
			common.ApiErrorI18n(c, i18n.MsgSetupPasswordMismatch)
			return
		}

		if len(req.Password) < 8 {
			common.ApiErrorI18n(c, i18n.MsgSetupPasswordTooShort)
			return
		}

		// Create root user
		hashedPassword, err := common.Password2Hash(req.Password)
		if err != nil {
			common.ApiErrorI18n(c, i18n.MsgSetupSystemError, map[string]any{"Error": err.Error()})
			return
		}
		rootUser := model.User{
			Username:    req.Username,
			Password:    hashedPassword,
			Role:        common.RoleRootUser,
			Status:      common.UserStatusEnabled,
			DisplayName: "Root User",
			AccessToken: nil,
			Quota:       100000000,
		}
		err = model.DB.Create(&rootUser).Error
		if err != nil {
			common.ApiErrorI18n(c, i18n.MsgSetupCreateRootFailed, map[string]any{"Error": err.Error()})
			return
		}
	}

	// Set operation modes
	operation_setting.SelfUseModeEnabled = req.SelfUseModeEnabled
	operation_setting.DemoSiteEnabled = req.DemoSiteEnabled

	// Save operation modes to database for persistence
	err = model.UpdateOption("SelfUseModeEnabled", boolToString(req.SelfUseModeEnabled))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgSetupSaveSelfUseFailed, map[string]any{"Error": err.Error()})
		return
	}

	err = model.UpdateOption("DemoSiteEnabled", boolToString(req.DemoSiteEnabled))
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgSetupSaveDemoSiteFailed, map[string]any{"Error": err.Error()})
		return
	}

	// Save AIPDD API key and bootstrap catalog + channel
	aipddKey := strings.TrimSpace(req.AIPDDApiKey)
	var warnings []SetupWarning
	if aipddKey != "" {
		if err = model.UpdateOption("AIPDDApiKey", aipddKey); err != nil {
			common.ApiErrorI18n(c, i18n.MsgSetupSaveAIPDDKeyFailed, map[string]any{"Error": setupErrorDetail(err, aipddKey)})
			return
		}
		baseURL := model.ResolveAIPDDBaseURL()
		syncCtx, syncCancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer syncCancel()
		if _, err = model.SyncAIPDDCatalog(syncCtx, nil, baseURL, aipddKey); err != nil {
			common.SysLog("AIPDD catalog sync during setup failed: " + err.Error())
			warnings = append(warnings, SetupWarning{
				Code:   setupWarningCatalogSync,
				Detail: setupErrorDetail(err, aipddKey),
			})
		}
		if err = model.EnsureAIPDDChannelWithKey(aipddKey, baseURL); err != nil {
			common.SysLog("AIPDD channel creation during setup failed: " + err.Error())
			warnings = append(warnings, SetupWarning{
				Code:   setupWarningChannelSetup,
				Detail: setupErrorDetail(err, aipddKey),
			})
		}
	}

	// Update setup status
	constant.Setup = true

	setup := model.Setup{
		Version:       common.Version,
		InitializedAt: time.Now().Unix(),
	}
	err = model.DB.Create(&setup).Error
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgSetupFailed, map[string]any{"Error": err.Error()})
		return
	}

	response := gin.H{
		"success": true,
		"message": i18n.T(c, i18n.MsgSetupSuccess),
	}
	if len(warnings) > 0 {
		response["warnings"] = warnings
	}
	c.JSON(200, response)
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
