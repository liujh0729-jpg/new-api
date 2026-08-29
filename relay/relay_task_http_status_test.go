package relay

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsSuccessfulTaskSubmitStatusAcceptsEveryHTTP2xx(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{name: "ok", statusCode: http.StatusOK, want: true},
		{name: "created", statusCode: http.StatusCreated, want: true},
		{name: "accepted", statusCode: http.StatusAccepted, want: true},
		{name: "no content", statusCode: http.StatusNoContent, want: true},
		{name: "informational", statusCode: http.StatusContinue, want: false},
		{name: "redirect", statusCode: http.StatusMultipleChoices, want: false},
		{name: "bad request", statusCode: http.StatusBadRequest, want: false},
		{name: "server error", statusCode: http.StatusBadGateway, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, isSuccessfulTaskSubmitStatus(test.statusCode))
		})
	}
}
