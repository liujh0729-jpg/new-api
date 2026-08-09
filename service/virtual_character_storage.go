package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	virtualCharacterAIPDDKeyEnv     = "AIPDD_API_KEY"
	virtualCharacterAIPDDBaseURLEnv = "AIPDD_BASE_URL"
	virtualCharacterAIPDDDefaultURL = "https://api.aipdd.work"
	virtualCharacterSignedTTL       = 72 * time.Hour
	virtualCharacterResponseMax     = 4 << 20
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

type aipddEnvelope[T any] struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type AIPDDVirtualCharacterStorage struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewAIPDDVirtualCharacterStorage() (*AIPDDVirtualCharacterStorage, error) {
	apiKey := strings.TrimSpace(os.Getenv(virtualCharacterAIPDDKeyEnv))
	if apiKey == "" {
		return nil, errors.New("AIPDD_API_KEY is not configured")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(virtualCharacterAIPDDBaseURLEnv)), "/")
	if baseURL == "" {
		baseURL = virtualCharacterAIPDDDefaultURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("AIPDD_BASE_URL is invalid")
	}
	return &AIPDDVirtualCharacterStorage{
		baseURL: baseURL,
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 90 * time.Second},
	}, nil
}

func (s *AIPDDVirtualCharacterStorage) UploadPrivateFile(ctx context.Context, filename string, reader io.Reader) (*AIPDDStoredFile, error) {
	if s == nil || reader == nil {
		return nil, errors.New("invalid AIPDD upload request")
	}
	filename = filepath.Base(strings.TrimSpace(filename))
	if filename == "" || filename == "." {
		filename = "character-image"
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

	endpoint := s.baseURL + "/oss/upload?prefix=" + url.QueryEscape("new-api/virtual-characters") +
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

func (s *AIPDDVirtualCharacterStorage) CreateDigitalAsset(ctx context.Context, name, fileID string, fileSize int64) (*AIPDDDigitalAsset, error) {
	body := map[string]any{
		"name":     strings.TrimSpace(name),
		"type":     "image",
		"labels":   []string{"new-api-virtual-character"},
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
	if target == nil || len(body) == 0 {
		return nil
	}
	if err := common.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode AIPDD response: %w", err)
	}
	var status struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(body, &status); err == nil && status.Code != 0 {
		if strings.TrimSpace(status.Message) == "" {
			status.Message = "AIPDD application error"
		}
		return errors.New(status.Message)
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
