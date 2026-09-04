package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

const (
	virtualCharacterAIPDDKeyEnv     = "AIPDD_API_KEY"
	virtualCharacterAIPDDBaseURLEnv = "AIPDD_BASE_URL"
	virtualCharacterAIPDDDefaultURL = "https://api.aipdd.work"
	virtualCharacterSignedTTL       = 72 * time.Hour
	virtualCharacterResponseMax     = 4 << 20
	aipddMultipartUploadThreshold   = 48 << 20
	aipddDirectMaxPartSize          = 64 << 20
	aipddDirectCleanupTimeout       = 15 * time.Second
)

type AIPDDStoredFile struct {
	FileID    string `json:"file_id"`
	URL       string `json:"url"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	MimeType  string `json:"mime_type"`
	IsPrivate bool   `json:"is_private"`
	ObjectKey string `json:"object_key"`
}

type AIPDDDigitalAsset struct {
	ID int64 `json:"id"`
}

type AIPDDSignedURL struct {
	FileID    string `json:"file_id"`
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at"`
}

type aipddDirectUploadInit struct {
	FileID    string                  `json:"file_id"`
	ObjectKey string                  `json:"object_key"`
	UploadID  string                  `json:"upload_id"`
	PartSize  int64                   `json:"part_size"`
	Parts     []aipddDirectUploadPart `json:"parts"`
}

type aipddDirectUploadPart struct {
	PartNumber int    `json:"part_number"`
	URL        string `json:"url"`
}

type aipddCompletedUploadPart struct {
	PartNumber int    `json:"part_number"`
	ETag       string `json:"etag"`
}

type aipddEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type AIPDDVirtualCharacterStorage struct {
	baseURL   string
	apiKey    string
	channelID int
	client    *http.Client
}

type AIPDDAssetStorage = AIPDDVirtualCharacterStorage

func NewAIPDDVirtualCharacterStorage() (*AIPDDVirtualCharacterStorage, error) {
	apiKey := strings.TrimSpace(os.Getenv(virtualCharacterAIPDDKeyEnv))
	if apiKey != "" {
		baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(virtualCharacterAIPDDBaseURLEnv)), "/")
		if baseURL == "" {
			baseURL = virtualCharacterAIPDDDefaultURL
		}
		return newAIPDDVirtualCharacterStorage(baseURL, apiKey, 0)
	}

	channel, err := findAIPDDVirtualCharacterStorageChannel(0, true)
	if err != nil {
		return nil, err
	}
	return newAIPDDVirtualCharacterStorageFromChannel(channel)
}

// NewAIPDDAssetStorage resolves the deployment-level AIPDD credential first
// and otherwise falls back to the highest-priority enabled AIPDD channel.
// It is the generic entry point for features that store AIPDD digital assets.
func NewAIPDDAssetStorage() (*AIPDDAssetStorage, error) {
	return NewAIPDDVirtualCharacterStorage()
}

// NewAIPDDVirtualCharacterStorageForChannel restores the AIPDD credential used
// for a persisted character. A zero channel ID keeps legacy/env-backed records
// compatible by using the normal env-first resolver.
func NewAIPDDVirtualCharacterStorageForChannel(channelID int) (*AIPDDVirtualCharacterStorage, error) {
	if channelID <= 0 {
		return NewAIPDDVirtualCharacterStorage()
	}
	channel, err := findAIPDDVirtualCharacterStorageChannel(channelID, false)
	if err != nil {
		return nil, err
	}
	return newAIPDDVirtualCharacterStorageFromChannel(channel)
}

func newAIPDDVirtualCharacterStorageFromChannel(channel *model.Channel) (*AIPDDVirtualCharacterStorage, error) {
	if channel == nil {
		return nil, errors.New("AIPDD storage channel is unavailable")
	}
	apiKey := firstEnabledAIPDDChannelKey(channel)
	if apiKey == "" {
		return nil, fmt.Errorf("AIPDD channel %d has no enabled API key", channel.Id)
	}
	baseURL := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/"), "/v1")
	return newAIPDDVirtualCharacterStorage(baseURL, apiKey, channel.Id)
}

func newAIPDDVirtualCharacterStorage(baseURL, apiKey string, channelID int) (*AIPDDVirtualCharacterStorage, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	apiKey = normalizeAIPDDChannelKey(apiKey)
	if apiKey == "" {
		return nil, errors.New("AIPDD API key is not configured")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("AIPDD base URL is invalid")
	}
	return &AIPDDVirtualCharacterStorage{
		baseURL:   baseURL,
		apiKey:    apiKey,
		channelID: channelID,
		client:    &http.Client{Timeout: 5 * time.Minute},
	}, nil
}

func findAIPDDVirtualCharacterStorageChannel(channelID int, enabledOnly bool) (*model.Channel, error) {
	if model.DB == nil {
		return nil, errors.New("AIPDD_API_KEY is not configured and the channel database is unavailable")
	}
	query := model.DB.Where("type = ?", constant.ChannelTypeAIPDD)
	if channelID > 0 {
		query = query.Where("id = ?", channelID)
	}
	if enabledOnly {
		query = query.Where("status = ?", common.ChannelStatusEnabled)
	}
	var channel model.Channel
	if err := query.Order("priority DESC").Order("id ASC").First(&channel).Error; err != nil {
		if channelID > 0 {
			return nil, fmt.Errorf("AIPDD storage channel %d is unavailable: %w", channelID, err)
		}
		return nil, fmt.Errorf("AIPDD_API_KEY is not configured and no enabled AIPDD channel is available: %w", err)
	}
	return &channel, nil
}

func firstEnabledAIPDDChannelKey(channel *model.Channel) string {
	if channel == nil {
		return ""
	}
	keys := channel.GetKeys()
	for index, candidate := range keys {
		if channel.ChannelInfo.IsMultiKey {
			if status, exists := channel.ChannelInfo.MultiKeyStatusList[index]; exists && status != common.ChannelStatusEnabled {
				continue
			}
		}
		if normalized := normalizeAIPDDChannelKey(candidate); normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizeAIPDDChannelKey(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = strings.TrimSpace(unquoted)
		}
	}
	return value
}

func (s *AIPDDVirtualCharacterStorage) ChannelID() int {
	if s == nil {
		return 0
	}
	return s.channelID
}

func (s *AIPDDVirtualCharacterStorage) UploadPrivateFile(ctx context.Context, filename string, reader io.Reader) (*AIPDDStoredFile, error) {
	return s.UploadPrivateFileWithPrefix(ctx, filename, "new-api/virtual-characters", reader)
}

// UploadPrivateAssetFile keeps small uploads on AIPDD's multipart endpoint and
// switches larger playground media to its signed multipart OSS flow. AIPDD's
// default legacy upload limit is 50 MB while playground references may be much
// larger, so the threshold leaves room for the multipart request envelope.
func (s *AIPDDVirtualCharacterStorage) UploadPrivateAssetFile(
	ctx context.Context,
	filename string,
	prefix string,
	mimeType string,
	fileSize int64,
	reader io.Reader,
) (*AIPDDStoredFile, error) {
	if fileSize >= aipddMultipartUploadThreshold {
		return s.uploadPrivateFileDirect(ctx, filename, prefix, mimeType, fileSize, reader)
	}
	return s.UploadPrivateFileWithPrefix(ctx, filename, prefix, reader)
}

func (s *AIPDDVirtualCharacterStorage) UploadPrivateFileWithPrefix(
	ctx context.Context,
	filename string,
	prefix string,
	reader io.Reader,
) (*AIPDDStoredFile, error) {
	if s == nil || reader == nil {
		return nil, errors.New("invalid AIPDD upload request")
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." {
		filename = "upload.bin"
	}

	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	writeDone := make(chan error, 1)
	go func() {
		part, err := writer.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, reader)
		}
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		_ = pipeWriter.CloseWithError(err)
		writeDone <- err
	}()

	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		prefix = "new-api"
	}
	endpoint := s.baseURL + "/oss/upload?prefix=" + url.QueryEscape(prefix) +
		"&is_private=true&valid_time=" + strconv.FormatInt(int64(virtualCharacterSignedTTL/time.Second), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, pipeReader)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	s.setAuth(req)

	var envelope aipddEnvelope[AIPDDStoredFile]
	err = s.do(req, &envelope, false)
	if err != nil {
		_ = pipeReader.CloseWithError(err)
		return nil, err
	}
	if writeErr := <-writeDone; writeErr != nil {
		return nil, fmt.Errorf("stream AIPDD upload: %w", writeErr)
	}
	if strings.TrimSpace(envelope.Data.FileID) == "" {
		return nil, errors.New("AIPDD upload returned an empty file_id")
	}
	return &envelope.Data, nil
}

func (s *AIPDDVirtualCharacterStorage) uploadPrivateFileDirect(
	ctx context.Context,
	filename string,
	prefix string,
	mimeType string,
	fileSize int64,
	reader io.Reader,
) (*AIPDDStoredFile, error) {
	if s == nil || reader == nil || fileSize <= 0 {
		return nil, errors.New("invalid AIPDD direct upload request")
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." {
		filename = "upload.bin"
	}
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		prefix = "new-api"
	}

	initBody := map[string]any{
		"file_name":  filename,
		"size":       fileSize,
		"mime_type":  strings.TrimSpace(mimeType),
		"prefix":     prefix,
		"is_private": true,
		"valid_time": int64(virtualCharacterSignedTTL / time.Second),
	}
	var initEnvelope aipddEnvelope[aipddDirectUploadInit]
	if err := s.doJSON(ctx, http.MethodPost, "/oss/direct-upload/init", initBody, &initEnvelope, false); err != nil {
		return nil, fmt.Errorf("initialize AIPDD direct upload: %w", err)
	}
	init := initEnvelope.Data
	if strings.TrimSpace(init.FileID) == "" || strings.TrimSpace(init.ObjectKey) == "" ||
		strings.TrimSpace(init.UploadID) == "" || init.PartSize <= 0 ||
		init.PartSize > aipddDirectMaxPartSize || len(init.Parts) == 0 {
		return nil, errors.New("AIPDD direct upload initialization returned invalid data")
	}

	abortPending := true
	defer func() {
		if abortPending {
			s.abortDirectUpload(init.ObjectKey, init.UploadID)
		}
	}()

	sort.Slice(init.Parts, func(i, j int) bool {
		return init.Parts[i].PartNumber < init.Parts[j].PartNumber
	})
	expectedPartCount := int(1 + (fileSize-1)/init.PartSize)
	if len(init.Parts) != expectedPartCount {
		return nil, fmt.Errorf(
			"AIPDD direct upload returned %d parts, expected %d",
			len(init.Parts),
			expectedPartCount,
		)
	}
	completedParts := make([]aipddCompletedUploadPart, 0, expectedPartCount)
	remaining := fileSize
	for index, part := range init.Parts {
		if part.PartNumber != index+1 || strings.TrimSpace(part.URL) == "" {
			return nil, errors.New("AIPDD direct upload returned an invalid part")
		}
		partSize := min(remaining, init.PartSize)
		payload := make([]byte, int(partSize))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, fmt.Errorf("read AIPDD upload part %d: %w", part.PartNumber, err)
		}
		etag, err := s.uploadDirectPart(ctx, part, payload)
		if err != nil {
			return nil, err
		}
		completedParts = append(completedParts, aipddCompletedUploadPart{
			PartNumber: part.PartNumber,
			ETag:       etag,
		})
		remaining -= partSize
	}
	if remaining != 0 {
		return nil, fmt.Errorf("AIPDD direct upload returned insufficient parts for %d remaining bytes", remaining)
	}

	completeBody := map[string]any{
		"object_key": init.ObjectKey,
		"upload_id":  init.UploadID,
		"valid_time": int64(virtualCharacterSignedTTL / time.Second),
		"parts":      completedParts,
	}
	var completeEnvelope aipddEnvelope[AIPDDStoredFile]
	if err := s.doJSON(ctx, http.MethodPost, "/oss/direct-upload/complete", completeBody, &completeEnvelope, false); err != nil {
		s.deleteFileBestEffort(init.FileID)
		return nil, fmt.Errorf("complete AIPDD direct upload: %w", err)
	}
	abortPending = false
	if strings.TrimSpace(completeEnvelope.Data.FileID) == "" {
		s.deleteFileBestEffort(init.FileID)
		return nil, errors.New("AIPDD direct upload returned an empty file_id")
	}
	return &completeEnvelope.Data, nil
}

func (s *AIPDDVirtualCharacterStorage) uploadDirectPart(
	ctx context.Context,
	part aipddDirectUploadPart,
	payload []byte,
) (string, error) {
	partURL := strings.TrimSpace(part.URL)
	parsed, err := url.Parse(partURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("AIPDD upload part %d returned an invalid URL", part.PartNumber)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, partURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create AIPDD upload part %d request: %w", part.PartNumber, err)
	}
	req.ContentLength = int64(len(payload))
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload AIPDD part %d: %w", part.PartNumber, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("upload AIPDD part %d returned %d", part.PartNumber, resp.StatusCode)
	}
	etag := strings.TrimSpace(resp.Header.Get("ETag"))
	if etag == "" {
		return "", fmt.Errorf("upload AIPDD part %d returned an empty ETag", part.PartNumber)
	}
	return etag, nil
}

func (s *AIPDDVirtualCharacterStorage) abortDirectUpload(objectKey, uploadID string) {
	ctx, cancel := context.WithTimeout(context.Background(), aipddDirectCleanupTimeout)
	defer cancel()
	body := map[string]any{
		"object_key": strings.TrimSpace(objectKey),
		"upload_id":  strings.TrimSpace(uploadID),
	}
	_ = s.doJSON(ctx, http.MethodPost, "/oss/direct-upload/abort", body, nil, true)
}

func (s *AIPDDVirtualCharacterStorage) deleteFileBestEffort(fileID string) {
	ctx, cancel := context.WithTimeout(context.Background(), aipddDirectCleanupTimeout)
	defer cancel()
	_ = s.DeleteFile(ctx, fileID)
}

func (s *AIPDDVirtualCharacterStorage) CreateDigitalAsset(ctx context.Context, name, assetType, fileID string, fileSize int64) (*AIPDDDigitalAsset, error) {
	return s.CreateDigitalAssetWithLabels(
		ctx,
		name,
		assetType,
		fileID,
		fileSize,
		[]string{"new-api-virtual-character"},
	)
}

func (s *AIPDDVirtualCharacterStorage) CreateDigitalAssetWithLabels(
	ctx context.Context,
	name string,
	assetType string,
	fileID string,
	fileSize int64,
	labels []string,
) (*AIPDDDigitalAsset, error) {
	assetType = strings.ToLower(strings.TrimSpace(assetType))
	if assetType != "video" && assetType != "audio" {
		assetType = "image"
	}
	labels = normalizeAIPDDAssetLabels(labels)
	body := map[string]any{
		"name":     strings.TrimSpace(name),
		"type":     assetType,
		"labels":   labels,
		"url":      strings.TrimSpace(fileID),
		"isOpen":   false,
		"enabled":  true,
		"fileSize": fileSize,
	}
	var envelope aipddEnvelope[AIPDDDigitalAsset]
	if err := s.doJSON(ctx, http.MethodPost, "/digital_asset", body, &envelope, false); err != nil {
		return nil, err
	}
	if envelope.Data.ID <= 0 {
		return nil, errors.New("AIPDD digital asset returned an invalid id")
	}
	return &envelope.Data, nil
}

func normalizeAIPDDAssetLabels(labels []string) []string {
	normalized := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if _, exists := seen[label]; exists {
			continue
		}
		seen[label] = struct{}{}
		normalized = append(normalized, label)
	}
	return normalized
}

func (s *AIPDDVirtualCharacterStorage) SignFile(ctx context.Context, fileID string) (*AIPDDSignedURL, error) {
	path := "/oss/file/" + url.PathEscape(strings.TrimSpace(fileID)) + "/sign?valid_time=" +
		strconv.FormatInt(int64(virtualCharacterSignedTTL/time.Second), 10)
	var envelope aipddEnvelope[AIPDDSignedURL]
	if err := s.doJSON(ctx, http.MethodGet, path, nil, &envelope, false); err != nil {
		return nil, err
	}
	if strings.TrimSpace(envelope.Data.URL) == "" {
		return nil, errors.New("AIPDD file signing returned an empty URL")
	}
	return &envelope.Data, nil
}

func (s *AIPDDVirtualCharacterStorage) DeleteDigitalAsset(ctx context.Context, assetID int64) error {
	if assetID <= 0 {
		return nil
	}
	return s.doJSON(ctx, http.MethodDelete, "/digital_asset/"+strconv.FormatInt(assetID, 10), nil, nil, true)
}

func (s *AIPDDVirtualCharacterStorage) DeleteFile(ctx context.Context, fileID string) error {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil
	}
	return s.doJSON(ctx, http.MethodDelete, "/oss/file/"+url.PathEscape(fileID), nil, nil, true)
}

func (s *AIPDDVirtualCharacterStorage) doJSON(ctx context.Context, method, path string, body any, target any, allowNotFound bool) error {
	var bodyReader io.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	s.setAuth(req)
	return s.do(req, target, allowNotFound)
}

func (s *AIPDDVirtualCharacterStorage) do(req *http.Request, target any, allowNotFound bool) error {
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("AIPDD request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, virtualCharacterResponseMax+1))
	if err != nil {
		return fmt.Errorf("read AIPDD response: %w", err)
	}
	if len(body) > virtualCharacterResponseMax {
		return errors.New("AIPDD response is too large")
	}
	if allowNotFound && resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("AIPDD returned %d: %s", resp.StatusCode, aipddErrorMessage(body))
	}
	if len(body) == 0 {
		return nil
	}
	var status struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(body, &status); err == nil && status.Code != nil && *status.Code != 0 {
		if strings.TrimSpace(status.Message) == "" {
			status.Message = "AIPDD application error"
		}
		return errors.New(status.Message)
	}
	if target == nil {
		return nil
	}
	if err := common.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode AIPDD response: %w", err)
	}
	return nil
}

func (s *AIPDDVirtualCharacterStorage) setAuth(req *http.Request) {
	req.Header.Set("X-API-Key", s.apiKey)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
}

func aipddErrorMessage(body []byte) string {
	var envelope struct {
		Message string `json:"message"`
	}
	if err := common.Unmarshal(body, &envelope); err == nil && strings.TrimSpace(envelope.Message) != "" {
		return envelope.Message
	}
	message := strings.TrimSpace(string(body))
	if len(message) > 500 {
		message = message[:500]
	}
	if message == "" {
		return "empty response"
	}
	return message
}
