package controller

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// videoProxyError returns a standardized OpenAI-style error response.
func videoProxyError(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
		},
	})
}

type videoProxyFailure struct {
	status  int
	errType string
	message string
}

type videoContentTarget struct {
	URL     string
	Headers http.Header
	Proxy   string
}

func newVideoProxyFailure(status int, errType string, message string) *videoProxyFailure {
	return &videoProxyFailure{
		status:  status,
		errType: errType,
		message: message,
	}
}

func VideoProxy(c *gin.Context) {
	taskID := c.Param("task_id")
	if taskID == "" {
		videoProxyError(c, http.StatusBadRequest, "invalid_request_error", "task_id is required")
		return
	}

	if failure := serveTaskVideoContent(c, taskID, c.GetInt("id")); failure != nil {
		videoProxyError(c, failure.status, failure.errType, failure.message)
	}
}

func serveTaskVideoContent(c *gin.Context, taskID string, userID int) *videoProxyFailure {
	task, exists, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to query task %s: %s", taskID, err.Error()))
		return newVideoProxyFailure(http.StatusInternalServerError, "server_error", "Failed to query task")
	}
	if !exists || task == nil {
		return newVideoProxyFailure(http.StatusNotFound, "invalid_request_error", "Task not found")
	}

	if task.Status != model.TaskStatusSuccess {
		return newVideoProxyFailure(http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("Task is not completed yet, current status: %s", task.Status))
	}

	target, failure := resolveTaskVideoContentTarget(c.Request.Context(), task)
	if failure != nil {
		return failure
	}
	if target == nil || strings.TrimSpace(target.URL) == "" {
		return newVideoProxyFailure(http.StatusBadGateway, "server_error", "Failed to fetch video content")
	}

	if strings.HasPrefix(target.URL, "data:") {
		if err := writeVideoDataURL(c, taskID, target.URL); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to decode video data URL for task %s: %s", taskID, err.Error()))
			return newVideoProxyFailure(http.StatusBadGateway, "server_error", "Failed to fetch video content")
		}
		return nil
	}

	return streamVideoContentTarget(c, taskID, target)
}

func resolveTaskVideoContentTarget(ctx context.Context, task *model.Task) (*videoContentTarget, *videoProxyFailure) {
	if task == nil {
		return nil, newVideoProxyFailure(http.StatusNotFound, "invalid_request_error", "Task not found")
	}

	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Failed to get channel for task %s: %s", task.TaskID, err.Error()))
		return nil, newVideoProxyFailure(http.StatusInternalServerError, "server_error", "Failed to retrieve channel information")
	}
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	var videoURL string
	headers := http.Header{}

	switch channel.Type {
	case constant.ChannelTypeGemini:
		apiKey := task.PrivateData.Key
		if apiKey == "" {
			logger.LogError(ctx, fmt.Sprintf("Missing stored API key for Gemini task %s", task.TaskID))
			return nil, newVideoProxyFailure(http.StatusInternalServerError, "server_error", "API key not stored for task")
		}
		videoURL, err = getGeminiVideoURL(channel, task, apiKey)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to resolve Gemini video URL for task %s: %s", task.TaskID, err.Error()))
			return nil, newVideoProxyFailure(http.StatusBadGateway, "server_error", "Failed to resolve Gemini video URL")
		}
		headers.Set("x-goog-api-key", apiKey)
	case constant.ChannelTypeVertexAi:
		videoURL, err = getVertexVideoURL(channel, task)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to resolve Vertex video URL for task %s: %s", task.TaskID, err.Error()))
			return nil, newVideoProxyFailure(http.StatusBadGateway, "server_error", "Failed to resolve Vertex video URL")
		}
	case constant.ChannelTypeOpenAI, constant.ChannelTypeSora:
		videoURL = fmt.Sprintf("%s/v1/videos/%s/content", baseURL, task.GetUpstreamTaskID())
		headers.Set("Authorization", "Bearer "+channel.Key)
	default:
		// Video URL is stored in PrivateData.ResultURL (fallback to FailReason for old data)
		videoURL = task.GetResultURL()
	}

	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		logger.LogError(ctx, fmt.Sprintf("Video URL is empty for task %s", task.TaskID))
		return nil, newVideoProxyFailure(http.StatusBadGateway, "server_error", "Failed to fetch video content")
	}

	return &videoContentTarget{
		URL:     videoURL,
		Headers: headers,
		Proxy:   channel.GetSetting().Proxy,
	}, nil
}

func streamVideoContentTarget(c *gin.Context, taskID string, target *videoContentTarget) *videoProxyFailure {
	client, err := service.GetHttpClientWithProxy(target.Proxy)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create proxy client for task %s: %s", taskID, err.Error()))
		return newVideoProxyFailure(http.StatusInternalServerError, "server_error", "Failed to create proxy client")
	}

	fetchSetting := system_setting.GetFetchSetting()
	if err := common.ValidateURLWithFetchSetting(target.URL, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Video URL blocked for task %s: %v", taskID, err))
		return newVideoProxyFailure(http.StatusForbidden, "server_error", fmt.Sprintf("request blocked: %v", err))
	}

	requestCtx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, target.URL, nil)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create request for %s: %s", target.URL, err.Error()))
		return newVideoProxyFailure(http.StatusInternalServerError, "server_error", "Failed to create proxy request")
	}
	copyVideoProxyRequestHeaders(c, req)
	copyHeaderValues(req.Header, target.Headers)

	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to fetch video from %s: %s", target.URL, err.Error()))
		return newVideoProxyFailure(http.StatusBadGateway, "server_error", "Failed to fetch video content")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned status %d for %s", resp.StatusCode, target.URL))
		return newVideoProxyFailure(http.StatusBadGateway, "server_error",
			fmt.Sprintf("Upstream service returned status %d", resp.StatusCode))
	}

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}

	setVideoProxyContentHeaders(c, taskID, target.URL, resp.Header.Get("Content-Type"))
	c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(c.Writer, resp.Body); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream video content: %s", err.Error()))
	}
	return nil
}

func copyHeaderValues(dst http.Header, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyVideoProxyRequestHeaders(c *gin.Context, req *http.Request) {
	for _, header := range []string{"Range", "If-Range"} {
		if value := c.GetHeader(header); value != "" {
			req.Header.Set(header, value)
		}
	}
}

func writeVideoDataURL(c *gin.Context, taskID string, dataURL string) error {
	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid data url")
	}

	header := parts[0]
	payload := parts[1]
	if !strings.HasPrefix(header, "data:") || !strings.Contains(header, ";base64") {
		return fmt.Errorf("unsupported data url")
	}

	mimeType := strings.TrimPrefix(header, "data:")
	mimeType = strings.TrimSuffix(mimeType, ";base64")
	if mimeType == "" {
		mimeType = "video/mp4"
	}

	videoBytes, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		videoBytes, err = base64.RawStdEncoding.DecodeString(payload)
		if err != nil {
			return err
		}
	}

	setVideoProxyContentHeaders(c, taskID, "", mimeType)
	c.Writer.Header().Set("Cache-Control", "public, max-age=86400")
	c.Writer.WriteHeader(http.StatusOK)
	_, err = c.Writer.Write(videoBytes)
	return err
}

func setVideoProxyContentHeaders(c *gin.Context, taskID, videoURL, contentType string) {
	contentType = normalizeVideoContentType(contentType, videoURL)
	c.Writer.Header().Set("Content-Type", contentType)
	c.Writer.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{
		"filename": videoProxyFilename(taskID, videoURL, contentType),
	}))
}

func normalizeVideoContentType(contentType, videoURL string) string {
	contentType = strings.TrimSpace(contentType)
	mediaType := contentType
	if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
		mediaType = parsed
	}
	if mediaType != "" && !strings.EqualFold(mediaType, "application/octet-stream") {
		return contentType
	}
	if inferred := videoContentTypeFromURL(videoURL); inferred != "" {
		return inferred
	}
	return "video/mp4"
}

func videoProxyFilename(taskID, videoURL, contentType string) string {
	filename := sanitizeDownloadFilename(taskID)
	if filename == "" {
		filename = "video"
	}
	if path.Ext(filename) != "" {
		return filename
	}
	ext := videoExtensionFromURL(videoURL)
	if ext == "" {
		ext = videoExtensionFromContentType(contentType)
	}
	if ext == "" {
		ext = ".mp4"
	}
	return filename + ext
}

func sanitizeDownloadFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		`"`, "_",
		"<", "_",
		">", "_",
		"|", "_",
		"\r", "_",
		"\n", "_",
	)
	return strings.Trim(replacer.Replace(value), " .")
}

func videoContentTypeFromURL(rawURL string) string {
	switch videoExtensionFromURL(rawURL) {
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".mpeg", ".mpg":
		return "video/mpeg"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	default:
		return ""
	}
}

func videoExtensionFromURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(path.Ext(parsed.Path))
	if isKnownVideoExtension(ext) {
		return ext
	}
	return ""
}

func videoExtensionFromContentType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		mediaType = strings.TrimSpace(contentType)
	}
	switch strings.ToLower(mediaType) {
	case "video/mp4":
		return ".mp4"
	case "video/quicktime":
		return ".mov"
	case "video/webm":
		return ".webm"
	case "video/mpeg":
		return ".mpg"
	case "video/x-msvideo":
		return ".avi"
	case "video/x-matroska":
		return ".mkv"
	}
	extensions, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(extensions) == 0 {
		return ""
	}
	ext := strings.ToLower(extensions[0])
	if isKnownVideoExtension(ext) {
		return ext
	}
	return ""
}

func isKnownVideoExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".mp4", ".m4v", ".mov", ".webm", ".mpeg", ".mpg", ".avi", ".mkv":
		return true
	default:
		return false
	}
}

func parseSameOriginVideoProxyTaskID(rawURL string) (string, bool) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", false
	}

	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil {
		return "", false
	}
	if parsed.IsAbs() && !isConfiguredServerOrigin(parsed) {
		return "", false
	}
	taskID, ok := parseVideoProxyTaskIDFromPath(parsed.EscapedPath())
	if ok {
		return taskID, true
	}
	return parseVideoProxyTaskIDFromPath(parsed.Path)
}

func isConfiguredServerOrigin(parsed *url.URL) bool {
	serverAddress := strings.TrimSpace(system_setting.ServerAddress)
	if serverAddress == "" {
		return false
	}
	serverURL, err := url.Parse(serverAddress)
	if err != nil || serverURL == nil || serverURL.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Scheme, serverURL.Scheme) &&
		strings.EqualFold(parsed.Host, serverURL.Host)
}

func parseVideoProxyTaskIDFromPath(pathValue string) (string, bool) {
	const prefix = "/v1/videos/"
	const suffix = "/content"
	if !strings.HasPrefix(pathValue, prefix) || !strings.HasSuffix(pathValue, suffix) {
		return "", false
	}
	taskID := strings.TrimSuffix(strings.TrimPrefix(pathValue, prefix), suffix)
	if taskID == "" || strings.Contains(taskID, "/") {
		return "", false
	}
	decoded, err := url.PathUnescape(taskID)
	if err != nil {
		return "", false
	}
	decoded = strings.TrimSpace(decoded)
	if decoded == "" || strings.Contains(decoded, "/") {
		return "", false
	}
	return decoded, decoded != ""
}
