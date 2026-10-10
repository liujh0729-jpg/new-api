package service

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestWouldDisableChannelIgnoresGlobalAutoDisableSwitch(t *testing.T) {
	previous := common.AutomaticDisableChannelEnabled
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previous })

	// 401 is in the default auto-disable status-code ranges and is not a channel-local
	// error, so it exercises the gate rather than an earlier short-circuit.
	err := types.NewErrorWithStatusCode(errors.New("invalid api key"),
		types.ErrorCodeBadResponseStatusCode, http.StatusUnauthorized)

	common.AutomaticDisableChannelEnabled = false
	require.False(t, ShouldDisableChannel(err))
	require.True(t, WouldDisableChannel(err), "an un-disabled channel must still be recognized as failing")

	common.AutomaticDisableChannelEnabled = true
	require.True(t, ShouldDisableChannel(err))
}

func TestWouldDisableChannelSkipsRetrySafeErrors(t *testing.T) {
	previous := common.AutomaticDisableChannelEnabled
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previous })
	common.AutomaticDisableChannelEnabled = false

	skipRetry := types.NewErrorWithStatusCode(errors.New("temporary upstream hiccup"),
		types.ErrorCodeBadResponseStatusCode, http.StatusUnauthorized,
		types.ErrOptionWithSkipRetry())
	require.False(t, WouldDisableChannel(skipRetry))
}

func TestShouldAlertChannelThrottlesRepeatedFailures(t *testing.T) {
	const channelId = 4_242_001
	channelFailureAlerts.Delete(channelId)
	t.Cleanup(func() { channelFailureAlerts.Delete(channelId) })

	require.True(t, shouldAlertChannel(channelId))
	require.False(t, shouldAlertChannel(channelId), "one alert per window per channel")

	channelFailureAlerts.Store(channelId, time.Now().Add(-channelFailureAlertWindow-time.Minute))
	require.True(t, shouldAlertChannel(channelId), "the window must reopen once it expires")
}
