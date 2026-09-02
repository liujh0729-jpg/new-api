package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyQueryUserIDHeaderSetsMissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/pg/material/file/1?user_id=42", nil)

	applyQueryUserIDHeader(ctx)

	require.Equal(t, "42", ctx.Request.Header.Get("New-Api-User"))
}

func TestApplyQueryUserIDHeaderPreservesExistingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/pg/material/file/1?user_id=42", nil)
	ctx.Request.Header.Set("New-Api-User", "7")

	applyQueryUserIDHeader(ctx)

	require.Equal(t, "7", ctx.Request.Header.Get("New-Api-User"))
}

func newSessionAuthTestRouter(t *testing.T, auth gin.HandlerFunc) *gin.Engine {
	t.Helper()
	store := cookie.NewStore([]byte("auth-test-secret"))
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
		c.Status(http.StatusNoContent)
	})
	router.GET("/protected", auth, func(c *gin.Context) {
		require.Equal(t, 7, c.GetInt("id"))
		c.Status(http.StatusNoContent)
	})
	return router
}

func loginSession(t *testing.T, router http.Handler, role int) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/session", strings.NewReader(fmt.Sprintf(
		`{"username":"admin","role":%d,"id":7}`,
		role,
	)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	for _, sessionCookie := range recorder.Result().Cookies() {
		if sessionCookie.Name == "session" {
			return sessionCookie
		}
	}
	t.Fatal("session cookie missing")
	return nil
}

func TestAdminAuthWithSessionUserIDAllowsPrivilegedSessionWithoutUserHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, role := range map[string]int{
		"admin": common.RoleAdminUser,
		"root":  common.RoleRootUser,
	} {
		t.Run(name, func(t *testing.T) {
			router := newSessionAuthTestRouter(t, AdminAuthWithSessionUserID())
			cookie := loginSession(t, router, role)
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.AddCookie(cookie)
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, req)

			require.Equal(t, http.StatusNoContent, recorder.Code)
			require.NotEmpty(t, recorder.Header().Get("Auth-Version"))
		})
	}
}

func TestAdminAuthWithSessionUserIDRejectsNonAdminSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newSessionAuthTestRouter(t, AdminAuthWithSessionUserID())
	cookie := loginSession(t, router, common.RoleCommonUser)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":false`)
}

func TestAdminAuthWithSessionUserIDRejectsMissingSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newSessionAuthTestRouter(t, AdminAuthWithSessionUserID())
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/protected", nil))

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
}

func TestAdminAuthWithSessionUserIDRejectsExplicitMismatchedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newSessionAuthTestRouter(t, AdminAuthWithSessionUserID())
	cookie := loginSession(t, router, common.RoleRootUser)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(cookie)
	req.Header.Set("New-Api-User", "8")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Contains(t, recorder.Body.String(), "auth.user_id_mismatch")
}

func TestAdminAuthStillRequiresUserHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newSessionAuthTestRouter(t, AdminAuth())
	cookie := loginSession(t, router, common.RoleAdminUser)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Contains(t, recorder.Body.String(), "auth.user_id_not_provided")
}
