package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// setupTestEnv makes the already-initialized branch reachable without a database
// and restores the process-wide flag afterwards, because every other handler in
// the package assumes setup is pending.
func setupTestEnv(t *testing.T) {
	t.Helper()
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)

	previous := constant.Setup
	constant.Setup = true
	t.Cleanup(func() { constant.Setup = previous })
}

func newSetupRequest(acceptLanguage string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/setup", strings.NewReader("{}"))
	c.Request.Header.Set("Content-Type", "application/json")
	if acceptLanguage != "" {
		c.Request.Header.Set("Accept-Language", acceptLanguage)
	}
	return c, recorder
}

// The wizard is the first screen a fresh deployment sees, so its failures must be
// in the operator's language and keep the {success, message} shape it parses.
func TestPostSetupReportsAlreadyInitializedInOperatorLanguage(t *testing.T) {
	setupTestEnv(t)

	cases := []struct {
		acceptLanguage string
		expected       string
	}{
		{"", "System initialization has already been completed"},
		{"zh-CN", "系统已经初始化完成"},
		{"zh-TW", "系統已經初始化完成"},
	}
	for _, testCase := range cases {
		c, recorder := newSetupRequest(testCase.acceptLanguage)
		PostSetup(c)

		require.Equal(t, http.StatusOK, recorder.Code)
		var body struct {
			Success bool   `json:"success"`
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		require.False(t, body.Success)
		require.Equal(t, testCase.expected, body.Message)
	}
}

// A short detail is meant for a toast, so the key that failed the sync can never
// ride along with it.
func TestSetupErrorDetailRedactsAndTruncates(t *testing.T) {
	secret := "sk-testsecrettestsecret"
	detail := setupErrorDetail(
		errors.New("upstream rejected the request: "+secret+" and again "+secret),
		secret,
	)
	require.NotContains(t, detail, secret)
	require.Contains(t, detail, "[redacted]")

	long := setupErrorDetail(errors.New(strings.Repeat("x", 400)))
	require.LessOrEqual(t, len(long), 215)
	require.True(t, strings.HasSuffix(long, "...(truncated)"))
}
