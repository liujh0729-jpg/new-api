package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
	"github.com/volcengine/volcengine-go-sdk/volcengine/credentials"
	"github.com/volcengine/volcengine-go-sdk/volcengine/session"
	"github.com/volcengine/volcengine-go-sdk/volcengine/universal"
)

const (
	volcAssetServiceName = "ark"
	volcAssetAPIVersion  = "2024-01-01"
)

type VolcValidationSessionResult struct {
	BytedToken string
	H5Link     string
}

type VolcAssetResult struct {
	ID        string
	GroupID   string
	Name      string
	AssetType string
	Status    string
	Error     string
}

type VolcAssetGroupResult struct {
	ID     string
	Name   string
	Status string
}

type VolcAssetClient interface {
	CreateVisualValidateSession(ctx context.Context, callbackURL, projectName, language string) (*VolcValidationSessionResult, error)
	GetVisualValidateResult(ctx context.Context, bytedToken, projectName string) (string, error)
	CreateAssetGroup(ctx context.Context, name, description, projectName string) (string, error)
	UpdateAssetGroup(ctx context.Context, groupID, name, description, projectName string) error
	CreateAsset(ctx context.Context, groupID, sourceURL, assetType, name, projectName string) (string, error)
	UpdateAsset(ctx context.Context, assetID, name, projectName string) error
	GetAsset(ctx context.Context, assetID, projectName string) (*VolcAssetResult, error)
	ListAssets(ctx context.Context, groupID, projectName string) ([]VolcAssetResult, error)
	DeleteAsset(ctx context.Context, assetID, projectName string) error
	DeleteAssetGroup(ctx context.Context, groupID, projectName string) error
	ListAssetGroups(ctx context.Context, projectName string) ([]VolcAssetGroupResult, error)
	GetAssetGroup(ctx context.Context, groupID, projectName string) (*VolcAssetGroupResult, error)
}

type volcUniversalCaller interface {
	DoCall(info universal.RequestUniversal, input *map[string]interface{}) (*map[string]interface{}, error)
}

type volcAssetClient struct {
	client         volcUniversalCaller
	limiter        *volcAccountRateLimiter
	createAssetQPM int
}

type volcAccountRateLimiter struct {
	mu      sync.Mutex
	lastRun map[string]time.Time
}

var volcAccountRateLimiters sync.Map

func NewVolcAssetClient(account *model.VirtualCharacterProviderAccount) (VolcAssetClient, error) {
	if account == nil || !account.Enabled {
		return nil, errors.New("virtual character provider account is not enabled")
	}
	if !common.HasStableCryptoSecret() {
		return nil, common.ErrCryptoSecretNotConfigured
	}
	accessKey, err := common.DecryptSensitiveValue(account.EncryptedAccessKey)
	if err != nil {
		return nil, errors.New("decrypt provider access key failed")
	}
	secretKey, err := common.DecryptSensitiveValue(account.EncryptedSecretKey)
	if err != nil {
		return nil, errors.New("decrypt provider secret key failed")
	}
	region := strings.TrimSpace(account.Region)
	if region == "" {
		region = "cn-beijing"
	}
	endpoint := fmt.Sprintf("https://ark.%s.volcengineapi.com", region)
	config := volcengine.NewConfig().
		WithCredentials(credentials.NewStaticCredentials(strings.TrimSpace(accessKey), strings.TrimSpace(secretKey), "")).
		WithRegion(region).
		WithEndpoint(endpoint).
		WithHTTPClient(&http.Client{Timeout: 30 * time.Second}).
		WithMaxRetries(0)
	sess, err := session.NewSession(config)
	if err != nil {
		return nil, fmt.Errorf("create Volc session: %w", err)
	}
	limiterValue, _ := volcAccountRateLimiters.LoadOrStore(account.ID, &volcAccountRateLimiter{lastRun: make(map[string]time.Time)})
	return &volcAssetClient{
		client:         universal.New(sess),
		limiter:        limiterValue.(*volcAccountRateLimiter),
		createAssetQPM: account.EffectiveCreateAssetQPM(),
	}, nil
}

func (c *volcAssetClient) CreateVisualValidateSession(ctx context.Context, callbackURL, projectName, language string) (*VolcValidationSessionResult, error) {
	body := map[string]interface{}{"CallbackURL": callbackURL, "ProjectName": projectOrDefault(projectName)}
	if strings.TrimSpace(language) != "" {
		body["Language"] = strings.TrimSpace(language)
	}
	result, err := c.call(ctx, "CreateVisualValidateSession", body, 5)
	if err != nil {
		return nil, err
	}
	item := &VolcValidationSessionResult{BytedToken: findString(result, "BytedToken"), H5Link: findString(result, "H5Link")}
	if item.BytedToken == "" || item.H5Link == "" {
		return nil, errors.New("Volc validation session response is incomplete")
	}
	return item, nil
}

func (c *volcAssetClient) GetVisualValidateResult(ctx context.Context, bytedToken, projectName string) (string, error) {
	result, err := c.call(ctx, "GetVisualValidateResult", map[string]interface{}{"BytedToken": bytedToken, "ProjectName": projectOrDefault(projectName)}, 10)
	if err != nil {
		return "", err
	}
	groupID := findString(result, "GroupId", "GroupID")
	if groupID == "" {
		return "", errors.New("Volc validation result did not contain a group id")
	}
	return groupID, nil
}

func (c *volcAssetClient) CreateAssetGroup(ctx context.Context, name, description, projectName string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("asset group name is required")
	}
	body := map[string]interface{}{
		"Name":        name,
		"GroupType":   "AIGC",
		"ProjectName": projectOrDefault(projectName),
	}
	if description = strings.TrimSpace(description); description != "" {
		body["Description"] = description
	}
	result, err := c.call(ctx, "CreateAssetGroup", body, 10)
	if err != nil {
		return "", err
	}
	id := findString(result, "Id", "ID", "GroupId", "GroupID")
	if id == "" {
		return "", errors.New("Volc CreateAssetGroup response did not contain an id")
	}
	return id, nil
}

func (c *volcAssetClient) UpdateAssetGroup(ctx context.Context, groupID, name, description, projectName string) error {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return errors.New("asset group id is required")
	}
	body := map[string]interface{}{"Id": groupID, "ProjectName": projectOrDefault(projectName)}
	if name = strings.TrimSpace(name); name != "" {
		body["Name"] = name
	}
	if description = strings.TrimSpace(description); description != "" {
		body["Description"] = description
	}
	_, err := c.call(ctx, "UpdateAssetGroup", body, 10)
	return err
}

func (c *volcAssetClient) CreateAsset(ctx context.Context, groupID, sourceURL, assetType, name, projectName string) (string, error) {
	body := map[string]interface{}{"GroupId": groupID, "URL": sourceURL, "AssetType": assetType, "ProjectName": projectOrDefault(projectName)}
	if strings.TrimSpace(name) != "" {
		body["Name"] = strings.TrimSpace(name)
	}
	result, err := c.callQPM(ctx, "CreateAsset", body, c.createAssetQPM)
	if err != nil {
		return "", err
	}
	id := findString(result, "Id", "ID", "AssetId", "AssetID")
	if id == "" {
		return "", errors.New("Volc CreateAsset response did not contain an id")
	}
	return id, nil
}

func (c *volcAssetClient) UpdateAsset(ctx context.Context, assetID, name, projectName string) error {
	assetID = strings.TrimSpace(assetID)
	name = strings.TrimSpace(name)
	if assetID == "" || name == "" {
		return errors.New("asset id and name are required")
	}
	_, err := c.call(ctx, "UpdateAsset", map[string]interface{}{
		"Id": assetID, "Name": name, "ProjectName": projectOrDefault(projectName),
	}, 10)
	return err
}

func (c *volcAssetClient) GetAsset(ctx context.Context, assetID, projectName string) (*VolcAssetResult, error) {
	result, err := c.call(ctx, "GetAsset", map[string]interface{}{"Id": assetID, "ProjectName": projectOrDefault(projectName)}, 10)
	if err != nil {
		return nil, err
	}
	return parseVolcAsset(result), nil
}

func (c *volcAssetClient) ListAssets(ctx context.Context, groupID, projectName string) ([]VolcAssetResult, error) {
	filter := map[string]interface{}{
		"GroupType": "AIGC",
	}
	if groupID = strings.TrimSpace(groupID); groupID != "" {
		filter["GroupIds"] = []string{groupID}
	}
	body := map[string]interface{}{
		"Filter":      filter,
		"PageNumber":  1,
		"PageSize":    100,
		"ProjectName": projectOrDefault(projectName),
	}
	result, err := c.call(ctx, "ListAssets", body, 100)
	if err != nil {
		return nil, err
	}
	return parseVolcAssets(result), nil
}

func (c *volcAssetClient) DeleteAsset(ctx context.Context, assetID, projectName string) error {
	_, err := c.call(ctx, "DeleteAsset", map[string]interface{}{"Id": assetID, "ProjectName": projectOrDefault(projectName)}, 10)
	return err
}

func (c *volcAssetClient) DeleteAssetGroup(ctx context.Context, groupID, projectName string) error {
	_, err := c.call(ctx, "DeleteAssetGroup", map[string]interface{}{"Id": groupID, "ProjectName": projectOrDefault(projectName)}, 10)
	return err
}

func (c *volcAssetClient) ListAssetGroups(ctx context.Context, projectName string) ([]VolcAssetGroupResult, error) {
	// Volc requires Filter.GroupType for ListAssetGroups.
	body := map[string]interface{}{
		"Filter": map[string]interface{}{
			"GroupType": "AIGC",
		},
		"PageNumber":  1,
		"PageSize":    10,
		"ProjectName": projectOrDefault(projectName),
	}
	result, err := c.call(ctx, "ListAssetGroups", body, 10)
	if err != nil {
		return nil, err
	}
	return parseVolcAssetGroups(result), nil
}

func (c *volcAssetClient) GetAssetGroup(ctx context.Context, groupID, projectName string) (*VolcAssetGroupResult, error) {
	result, err := c.call(ctx, "GetAssetGroup", map[string]interface{}{"Id": groupID, "ProjectName": projectOrDefault(projectName)}, 10)
	if err != nil {
		return nil, err
	}
	return &VolcAssetGroupResult{ID: findString(result, "GroupId", "GroupID", "Id", "ID"), Name: findString(result, "Name"), Status: findString(result, "Status")}, nil
}

func (c *volcAssetClient) call(ctx context.Context, action string, body map[string]interface{}, qps int) (map[string]interface{}, error) {
	return c.doCall(ctx, action, body, time.Second, qps)
}

func (c *volcAssetClient) callQPM(ctx context.Context, action string, body map[string]interface{}, qpm int) (map[string]interface{}, error) {
	return c.doCall(ctx, action, body, time.Minute, qpm)
}

func (c *volcAssetClient) doCall(ctx context.Context, action string, body map[string]interface{}, window time.Duration, rate int) (map[string]interface{}, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("Volc asset client is not configured")
	}
	info := universal.RequestUniversal{ServiceName: volcAssetServiceName, Action: action, Version: volcAssetAPIVersion, HttpMethod: universal.POST, ContentType: universal.ApplicationJSON}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Claim rate-limit budget per attempt: retries are mostly triggered by upstream
		// throttling, so reusing one slot for all four attempts would burst against it.
		if err := c.wait(ctx, action, window, rate); err != nil {
			return nil, err
		}
		output, err := c.client.DoCall(info, &body)
		if err == nil && output != nil {
			return *output, nil
		}
		lastErr = err
		if !isRetryableVolcError(err) || attempt == 3 {
			break
		}
		delay := time.Duration(1<<uint(attempt)) * 250 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr == nil {
		lastErr = errors.New("empty Volc response")
	}
	return nil, fmt.Errorf("Volc %s failed: %w", action, lastErr)
}

func (c *volcAssetClient) wait(ctx context.Context, action string, window time.Duration, rate int) error {
	if rate <= 0 {
		rate = 1
	}
	if window <= 0 {
		window = time.Second
	}
	interval := window / time.Duration(rate)
	if interval <= 0 {
		interval = time.Millisecond
	}
	if c.limiter == nil {
		c.limiter = &volcAccountRateLimiter{lastRun: make(map[string]time.Time)}
	}
	c.limiter.mu.Lock()
	previous := c.limiter.lastRun[action]
	next := previous.Add(interval)
	now := time.Now()
	if next.Before(now) {
		next = now
	}
	c.limiter.lastRun[action] = next
	c.limiter.mu.Unlock()
	if delay := time.Until(next); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			c.releaseReservation(action, previous, next)
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// releaseReservation hands an unused slot back to the limiter so a cancelled caller
// does not make later callers wait out a request that was never sent.
func (c *volcAssetClient) releaseReservation(action string, previous, reserved time.Time) {
	c.limiter.mu.Lock()
	if c.limiter.lastRun[action].Equal(reserved) {
		c.limiter.lastRun[action] = previous
	}
	c.limiter.mu.Unlock()
}

func projectOrDefault(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "default"
}

func isRetryableVolcError(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "throttl") || strings.Contains(value, "limit") || strings.Contains(value, "timeout") || strings.Contains(value, "temporar") || strings.Contains(value, "connection reset") || strings.Contains(value, "eof") || strings.Contains(value, "status code: 5")
}

func parseVolcAsset(result map[string]interface{}) *VolcAssetResult {
	return &VolcAssetResult{ID: findString(result, "Id", "ID", "AssetId", "AssetID"), GroupID: findString(result, "GroupId", "GroupID"), Name: findString(result, "Name"), AssetType: findString(result, "AssetType", "Type"), Status: findString(result, "Status"), Error: findString(result, "Error", "Message", "FailReason")}
}

func parseVolcAssets(result map[string]interface{}) []VolcAssetResult {
	items := findObjectSlice(result, "Assets", "Items", "Data")
	assets := make([]VolcAssetResult, 0, len(items))
	for _, item := range items {
		assets = append(assets, *parseVolcAsset(item))
	}
	return assets
}

func parseVolcAssetGroups(result map[string]interface{}) []VolcAssetGroupResult {
	items := findObjectSlice(result, "AssetGroups", "Groups", "Items", "Data")
	groups := make([]VolcAssetGroupResult, 0, len(items))
	for _, item := range items {
		groups = append(groups, VolcAssetGroupResult{ID: findString(item, "GroupId", "GroupID", "Id", "ID"), Name: findString(item, "Name"), Status: findString(item, "Status")})
	}
	return groups
}

func findString(value map[string]interface{}, keys ...string) string {
	for key, item := range value {
		for _, wanted := range keys {
			if strings.EqualFold(key, wanted) {
				switch typed := item.(type) {
				case string:
					return strings.TrimSpace(typed)
				case fmt.Stringer:
					return strings.TrimSpace(typed.String())
				case float64:
					return fmt.Sprintf("%.0f", typed)
				}
			}
		}
		if nested, ok := item.(map[string]interface{}); ok {
			if found := findString(nested, keys...); found != "" {
				return found
			}
		}
	}
	return ""
}

func findObjectSlice(value map[string]interface{}, keys ...string) []map[string]interface{} {
	for key, item := range value {
		for _, wanted := range keys {
			if !strings.EqualFold(key, wanted) {
				continue
			}
			switch typed := item.(type) {
			case []map[string]interface{}:
				return typed
			case []interface{}:
				result := make([]map[string]interface{}, 0, len(typed))
				for _, entry := range typed {
					if object, ok := entry.(map[string]interface{}); ok {
						result = append(result, object)
					}
				}
				return result
			}
		}
		if nested, ok := item.(map[string]interface{}); ok {
			if found := findObjectSlice(nested, keys...); len(found) > 0 {
				return found
			}
		}
	}
	return nil
}
