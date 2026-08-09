package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/volcengine/volcengine-go-sdk/volcengine/universal"
)

type fakeVolcUniversalCaller struct {
	responses []*map[string]interface{}
	errors    []error
	calls     int
}

func (f *fakeVolcUniversalCaller) DoCall(_ universal.RequestUniversal, _ *map[string]interface{}) (*map[string]interface{}, error) {
	index := f.calls
	f.calls++
	var response *map[string]interface{}
	if index < len(f.responses) {
		response = f.responses[index]
	}
	if index < len(f.errors) {
		return response, f.errors[index]
	}
	return response, nil
}

func TestVolcAssetClientParsesNestedValidationAndAssetResponses(t *testing.T) {
	validation := map[string]interface{}{"Result": map[string]interface{}{"BytedToken": "token", "H5Link": "https://example.com/h5"}}
	asset := map[string]interface{}{"Result": map[string]interface{}{"Id": "asset-1", "GroupId": "group-1", "Status": "Active"}}
	fake := &fakeVolcUniversalCaller{responses: []*map[string]interface{}{&validation, &asset}}
	client := &volcAssetClient{client: fake, limiter: &volcAccountRateLimiter{lastRun: make(map[string]time.Time)}}

	session, err := client.CreateVisualValidateSession(context.Background(), "https://example.com/callback", "default", "zh")
	require.NoError(t, err)
	require.Equal(t, "token", session.BytedToken)
	result, err := client.GetAsset(context.Background(), "asset-1", "default")
	require.NoError(t, err)
	require.Equal(t, "asset-1", result.ID)
	require.Equal(t, "Active", result.Status)
}

func TestVolcAssetClientRetriesThrottlingAndHonorsCanceledContext(t *testing.T) {
	success := map[string]interface{}{"Result": map[string]interface{}{"GroupId": "group-1"}}
	fake := &fakeVolcUniversalCaller{responses: []*map[string]interface{}{nil, &success}, errors: []error{errors.New("ThrottlingException"), nil}}
	client := &volcAssetClient{client: fake, limiter: &volcAccountRateLimiter{lastRun: make(map[string]time.Time)}}
	groupID, err := client.GetVisualValidateResult(context.Background(), "token", "default")
	require.NoError(t, err)
	require.Equal(t, "group-1", groupID)
	require.Equal(t, 2, fake.calls)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.GetAsset(canceled, "asset-1", "default")
	require.ErrorIs(t, err, context.Canceled)
}
