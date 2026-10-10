package helper

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

func newSystemOneContext(t *testing.T, body string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/systemone", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = io.NopCloser(strings.NewReader(body))
	return c
}

func TestGetAndValidateSystemOneRequestPreservesPayload(t *testing.T) {
	c := newSystemOneContext(t, `{"model":"jev","state":{"turn":1},"questions":{"q1":{"type":"choice","criteria":{"yes":"a","no":"b"}}}}`)

	request, err := GetAndValidateSystemOneRequest(c)
	require.NoError(t, err)
	require.Equal(t, "jev", request.Model)

	// The decision payload is forwarded verbatim, so re-marshal must keep both objects.
	normalizedState, err := common.Marshal(request.State)
	require.NoError(t, err)
	require.JSONEq(t, `{"turn":1}`, string(normalizedState))
	normalizedQuestions, err := common.Marshal(request.Questions)
	require.NoError(t, err)
	require.JSONEq(t, `{"q1":{"type":"choice","criteria":{"yes":"a","no":"b"}}}`, string(normalizedQuestions))

	require.False(t, request.IsStream(c))
	require.Contains(t, request.GetTokenCountMeta().CombineText, `"turn":1`)
}

func TestGetAndValidateSystemOneRequestRequiresDecisionFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "missing model", body: `{"state":{},"questions":{}}`, want: "model is required"},
		{name: "missing state", body: `{"model":"jev","questions":{}}`, want: "field state is required"},
		{name: "missing questions", body: `{"model":"jev","state":{}}`, want: "field questions is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := GetAndValidateSystemOneRequest(newSystemOneContext(t, test.body))
			require.EqualError(t, err, test.want)
		})
	}
}
