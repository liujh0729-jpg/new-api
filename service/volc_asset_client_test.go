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
	inputs    []map[string]interface{}
	calls     int
}

func (f *fakeVolcUniversalCaller) DoCall(_ universal.RequestUniversal, input *map[string]interface{}) (*map[string]interface{}, error) {
	index := f.calls
	f.calls++
	if input != nil {
		copied := make(map[string]interface{}, len(*input))
		for key, value := range *input {
			copied[key] = value
		}
		f.inputs = append(f.inputs, copied)
	}
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

func TestVolcAssetClientCreateAssetGroupReturnsID(t *testing.T) {
	response := map[string]interface{}{"Result": map[string]interface{}{"Id": "group-aigc-1"}}
	fake := &fakeVolcUniversalCaller{responses: []*map[string]interface{}{&response}}
	client := &volcAssetClient{client: fake, limiter: &volcAccountRateLimiter{lastRun: make(map[string]time.Time)}, createAssetQPM: 3}
	groupID, err := client.CreateAssetGroup(context.Background(), "Actor One", "desc", "default")
	require.NoError(t, err)
	require.Equal(t, "group-aigc-1", groupID)
}

func TestVolcAssetClientCreateAssetUsesQPMWindow(t *testing.T) {
	response := map[string]interface{}{"Result": map[string]interface{}{"Id": "asset-1"}}
	fake := &fakeVolcUniversalCaller{responses: []*map[string]interface{}{&response, &response}}
	client := &volcAssetClient{client: fake, limiter: &volcAccountRateLimiter{lastRun: make(map[string]time.Time)}, createAssetQPM: 60}
	start := time.Now()
	_, err := client.CreateAsset(context.Background(), "group-1", "https://example.com/a.png", "Image", "a", "default")
	require.NoError(t, err)
	_, err = client.CreateAsset(context.Background(), "group-1", "https://example.com/b.png", "Image", "b", "default")
	require.NoError(t, err)
	// 60 QPM => 1 request/second spacing between consecutive CreateAsset calls.
	require.GreaterOrEqual(t, time.Since(start), 900*time.Millisecond)
}

func TestVolcAssetClientListAssetGroupsIncludesRequiredFilter(t *testing.T) {
	response := map[string]interface{}{"Result": map[string]interface{}{
		"Items": []interface{}{map[string]interface{}{"Id": "group-1", "Name": "demo", "Status": "Active"}},
	}}
	fake := &fakeVolcUniversalCaller{responses: []*map[string]interface{}{&response}}
	client := &volcAssetClient{client: fake, limiter: &volcAccountRateLimiter{lastRun: make(map[string]time.Time)}}

	groups, err := client.ListAssetGroups(context.Background(), "default")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "group-1", groups[0].ID)
	require.Len(t, fake.inputs, 1)
	filter, ok := fake.inputs[0]["Filter"].(map[string]interface{})
	require.True(t, ok, "ListAssetGroups must include Filter")
	require.Equal(t, "AIGC", filter["GroupType"])
	require.Equal(t, "default", fake.inputs[0]["ProjectName"])
}

func TestVolcAssetClientListAssetsUsesFilterGroupIds(t *testing.T) {
	response := map[string]interface{}{"Result": map[string]interface{}{
		"Items": []interface{}{map[string]interface{}{"Id": "asset-1", "GroupId": "group-1", "Status": "Active"}},
	}}
	fake := &fakeVolcUniversalCaller{responses: []*map[string]interface{}{&response}}
	client := &volcAssetClient{client: fake, limiter: &volcAccountRateLimiter{lastRun: make(map[string]time.Time)}}

	assets, err := client.ListAssets(context.Background(), "group-1", "default")
	require.NoError(t, err)
	require.Len(t, assets, 1)
	require.Equal(t, "asset-1", assets[0].ID)
	require.Len(t, fake.inputs, 1)
	filter, ok := fake.inputs[0]["Filter"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, []string{"group-1"}, filter["GroupIds"])
}
