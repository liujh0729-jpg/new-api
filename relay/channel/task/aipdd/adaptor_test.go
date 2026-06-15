package aipdd

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestConvertToRequestPayloadBuildsIndexTTSContent(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Prompt: "hello from new-api",
		Model:  ModelIndexTTS,
		Metadata: map[string]interface{}{
			"audio":       "https://cdn.example.com/reference.wav",
			"task_name":   "voice clone",
			"extra_key":   "ignored",
			"script_id":   "ignored",
			"script_code": "ignored",
		},
	}

	payload, err := adaptor.convertToRequestPayload(req, relayInfoWithModel(ModelIndexTTS))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if payload.TaskTypeCode != "aipdd_IndexTTS" {
		t.Fatalf("unexpected task type code: %s", payload.TaskTypeCode)
	}
	if payload.TaskName != "voice clone" {
		t.Fatalf("unexpected task name: %s", payload.TaskName)
	}

	content := payload.Input
	if content["audio"] != "https://cdn.example.com/reference.wav" {
		t.Fatalf("audio was not forwarded: %#v", content)
	}
	if content["text"] != "hello from new-api" {
		t.Fatalf("text did not fall back to prompt: %#v", content)
	}
	if _, ok := content["extra_key"]; ok {
		t.Fatalf("unexpected extra workflow key forwarded: %#v", content)
	}
}

func TestConvertToRequestPayloadDoesNotForwardFilenameForVideoModels(t *testing.T) {
	adaptor := &TaskAdaptor{}
	tests := []relaycommon.TaskSubmitReq{
		{
			Model:  ModelWan22Animater,
			Prompt: "replace subject",
			Metadata: map[string]interface{}{
				"video":           "https://cdn.example.com/uploads/input-video.mp4?x=1",
				"negative_prompt": "low quality",
				"filename":        "input-video.mp4",
			},
		},
		{
			Model: ModelLatentsync15,
			Metadata: map[string]interface{}{
				"video":     "https://cdn.example.com/uploads/input-video.mp4?x=1",
				"LoadAudio": "https://cdn.example.com/uploads/input-audio.wav",
				"filename":  "input-video.mp4",
			},
		},
	}

	for _, req := range tests {
		t.Run(req.Model, func(t *testing.T) {
			payload, err := adaptor.convertToRequestPayload(req, relayInfoWithModel(req.Model))
			if err != nil {
				t.Fatalf("convertToRequestPayload returned error: %v", err)
			}

			content := payload.Input
			if _, ok := content["filename"]; ok {
				t.Fatalf("filename should not be forwarded: %#v", content)
			}
		})
	}
}

func TestConvertToRequestPayloadForAllAIPDDModels(t *testing.T) {
	adaptor := &TaskAdaptor{}
	tests := []struct {
		name       string
		req        relaycommon.TaskSubmitReq
		wantCode   string
		wantFields map[string]string
	}{
		{
			name:     ModelFluxGGUF,
			wantCode: "FLUX-GGUF-V2",
			req: relaycommon.TaskSubmitReq{
				Model:  ModelFluxGGUF,
				Prompt: "a cinematic robot",
				Image:  "https://cdn.example.com/input.png",
			},
			wantFields: map[string]string{"image": "https://cdn.example.com/input.png", "positive_prompt": "a cinematic robot"},
		},
		{
			name:     ModelFluxGGUFT2I,
			wantCode: "FLUX-GGUF-T2I-V2",
			req: relaycommon.TaskSubmitReq{
				Model:  ModelFluxGGUFT2I,
				Prompt: "a cinematic robot",
			},
			wantFields: map[string]string{"text": "a cinematic robot"},
		},
		{
			name:     ModelWan22Wanx,
			wantCode: "aipdd_wan2.2_wanx",
			req: relaycommon.TaskSubmitReq{
				Model:    ModelWan22Wanx,
				Prompt:   "camera push in",
				Image:    "https://cdn.example.com/input.png",
				Duration: 10,
			},
			wantFields: map[string]string{"image": "https://cdn.example.com/input.png", "prompt": "camera push in"},
		},
		{
			name:     ModelWan22Animater,
			wantCode: "aipdd_Wan2.2-Animater",
			req: relaycommon.TaskSubmitReq{
				Model:  ModelWan22Animater,
				Prompt: "replace subject",
				Metadata: map[string]interface{}{
					"video":           "https://cdn.example.com/subject.mp4",
					"negative_prompt": "low quality",
				},
			},
			wantFields: map[string]string{"video": "https://cdn.example.com/subject.mp4", "positive_prompt": "replace subject"},
		},
		{
			name:     ModelMimicMotion,
			wantCode: "aipdd_mimic_motion",
			req: relaycommon.TaskSubmitReq{
				Model: ModelMimicMotion,
				Metadata: map[string]interface{}{
					"motion_video":     "https://cdn.example.com/motion.mp4",
					"appearance_image": "https://cdn.example.com/person.png",
				},
			},
			wantFields: map[string]string{"motion_video": "https://cdn.example.com/motion.mp4", "appearance_image": "https://cdn.example.com/person.png"},
		},
		{
			name:     ModelLatentsync15,
			wantCode: "aipdd_latentsync1.5",
			req: relaycommon.TaskSubmitReq{
				Model: ModelLatentsync15,
				Metadata: map[string]interface{}{
					"video":     "https://cdn.example.com/lips.mp4",
					"LoadAudio": "https://cdn.example.com/speech.wav",
				},
			},
			wantFields: map[string]string{"video": "https://cdn.example.com/lips.mp4", "LoadAudio": "https://cdn.example.com/speech.wav"},
		},
		{
			name:     ModelIndexTTS,
			wantCode: "aipdd_IndexTTS",
			req: relaycommon.TaskSubmitReq{
				Model: ModelIndexTTS,
				Metadata: map[string]interface{}{
					"input":     "hello",
					"ref_audio": "https://cdn.example.com/ref.wav",
				},
			},
			wantFields: map[string]string{"audio": "https://cdn.example.com/ref.wav", "text": "hello"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := adaptor.convertToRequestPayload(tt.req, relayInfoWithModel(tt.req.Model))
			if err != nil {
				t.Fatalf("convertToRequestPayload returned error: %v", err)
			}
			if payload.TaskTypeCode != tt.wantCode {
				t.Fatalf("unexpected task type code: %s", payload.TaskTypeCode)
			}
			content := payload.Input
			for key, want := range tt.wantFields {
				if got := anyToString(content[key]); got != want {
					t.Fatalf("unexpected %s: got %q want %q in %#v", key, got, want, content)
				}
			}
		})
	}
}

func TestWan22WanxDoesNotSendDurationToJavaBackend(t *testing.T) {
	adaptor := &TaskAdaptor{}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("task_request", relaycommon.TaskSubmitReq{
		Model:    ModelWan22Wanx,
		Duration: 5,
	})
	ratios := adaptor.EstimateBilling(ctx, relayInfoWithModel(ModelWan22Wanx))
	if ratios != nil {
		t.Fatalf("wanx should not provide duration billing ratios for Java backend: %#v", ratios)
	}

	payload, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model:    ModelWan22Wanx,
		Prompt:   "camera push in",
		Image:    "https://cdn.example.com/input.png",
		Duration: 10,
	}, relayInfoWithModel(ModelWan22Wanx))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if _, ok := payload.Input["duration"]; ok {
		t.Fatalf("duration should not be sent to Java backend input: %#v", payload.Input)
	}
}

func TestBuildRequestBodyUploadsMultipartFileToAIPDDOSS(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotAPIKey string
	var gotParamKey string
	var gotScriptID string
	var gotPrefix string
	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oss/upload" {
			t.Fatalf("unexpected upload path: %s", r.URL.Path)
		}
		gotAPIKey = r.Header.Get("X-API-Key")
		gotParamKey = r.URL.Query().Get("param_key")
		gotScriptID = r.URL.Query().Get("script_id")
		gotPrefix = r.URL.Query().Get("prefix")
		file, fileHeader, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected uploaded file: %v", err)
		}
		defer file.Close()
		fileBytes, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read uploaded file: %v", err)
		}
		if fileHeader.Filename != "input.png" || string(fileBytes) != "fake image bytes" {
			t.Fatalf("unexpected uploaded file: filename=%s body=%q", fileHeader.Filename, string(fileBytes))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"上传成功","data":{"file_id":"file-id","url":"https://oss.example.com/files/input.png"}}`))
	}))
	defer uploadServer.Close()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	_ = writer.WriteField("model", ModelWan22Wanx)
	_ = writer.WriteField("prompt", "camera push in")
	_ = writer.WriteField("duration", "5")
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="image"; filename="input.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("create multipart part: %v", err)
	}
	_, _ = part.Write([]byte("fake image bytes"))
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &requestBody)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	info := relayInfoWithModel(ModelWan22Wanx)
	info.ChannelBaseUrl = uploadServer.URL
	info.ApiKey = "aipdd-key"
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)

	if taskErr := adaptor.ValidateRequestAndSetAction(ctx, info); taskErr != nil {
		t.Fatalf("ValidateRequestAndSetAction returned task error: %v", taskErr)
	}
	bodyReader, err := adaptor.BuildRequestBody(ctx, info)
	if err != nil {
		t.Fatalf("BuildRequestBody returned error: %v", err)
	}
	bodyBytes, err := io.ReadAll(bodyReader)
	if err != nil {
		t.Fatalf("read built request body: %v", err)
	}

	var payload createTaskPayload
	if err := common.Unmarshal(bodyBytes, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TaskTypeCode != "aipdd_wan2.2_wanx" {
		t.Fatalf("unexpected task type code: %s", payload.TaskTypeCode)
	}
	content := payload.Input
	if got := anyToString(content["image"]); got != "https://oss.example.com/files/input.png" {
		t.Fatalf("uploaded image URL was not injected: got %q content=%#v", got, content)
	}
	if got := anyToString(content["prompt"]); got != "camera push in" {
		t.Fatalf("prompt was not preserved: got %q content=%#v", got, content)
	}
	if gotAPIKey != "aipdd-key" {
		t.Fatalf("upload did not use AIPDD API key: %q", gotAPIKey)
	}
	if gotPrefix != "files" {
		t.Fatalf("unexpected upload prefix: %q", gotPrefix)
	}
	if gotParamKey != "" || gotScriptID != "" {
		t.Fatalf("upload should not trigger aipdd-api script validation, got script_id=%q param_key=%q", gotScriptID, gotParamKey)
	}
}

func TestBuildRequestBodyDoesNotInjectFilenameForVideoUploads(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uploadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, fileHeader, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected uploaded file: %v", err)
		}
		defer file.Close()

		uploadedURL := "https://oss.aipdd.work/files/uploaded.bin"
		switch fileHeader.Filename {
		case "fullpath.mp4":
			uploadedURL = "https://oss.aipdd.work/files/ef5220cd-18e7-47be-b99c-e4390e03168f.mp4"
		case "source.mp4":
			uploadedURL = "https://oss.aipdd.work/files/source.mp4"
		case "speech.wav":
			uploadedURL = "https://oss.aipdd.work/files/speech.wav"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"上传成功","data":{"url":"` + uploadedURL + `"}}`))
	}))
	defer uploadServer.Close()

	tests := []struct {
		name       string
		model      string
		endpoint   string
		fields     map[string]string
		files      map[string]string
		wantFields map[string]string
	}{
		{
			name:     "animater fullpath upload",
			model:    ModelWan22Animater,
			endpoint: "/v1/videos",
			fields: map[string]string{
				"load_video":      "https://oss.aipdd.work/distributed_compute/443/15e6278f-a3ca-4682-89ef-802af9913273/ComfyUI_00006_.mp4",
				"prompt":          "natural motion, stable subject",
				"negative_prompt": "low quality, distorted, flicker",
			},
			files: map[string]string{
				"fullpath": "fullpath.mp4",
			},
			wantFields: map[string]string{
				"video":           "https://oss.aipdd.work/distributed_compute/443/15e6278f-a3ca-4682-89ef-802af9913273/ComfyUI_00006_.mp4",
				"image":           "https://oss.aipdd.work/files/ef5220cd-18e7-47be-b99c-e4390e03168f.mp4",
				"positive_prompt": "natural motion, stable subject",
			},
		},
		{
			name:     "latentsync video and audio upload",
			model:    ModelLatentsync15,
			endpoint: "/v1/videos",
			fields:   map[string]string{},
			files: map[string]string{
				"video":     "source.mp4",
				"LoadAudio": "speech.wav",
			},
			wantFields: map[string]string{
				"video":     "https://oss.aipdd.work/files/source.mp4",
				"LoadAudio": "https://oss.aipdd.work/files/speech.wav",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestBody bytes.Buffer
			writer := multipart.NewWriter(&requestBody)
			_ = writer.WriteField("model", tt.model)
			for key, value := range tt.fields {
				_ = writer.WriteField(key, value)
			}
			for field, filename := range tt.files {
				header := make(textproto.MIMEHeader)
				header.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
				header.Set("Content-Type", "application/octet-stream")
				part, err := writer.CreatePart(header)
				if err != nil {
					t.Fatalf("create multipart part: %v", err)
				}
				_, _ = part.Write([]byte("fake file bytes"))
			}
			if err := writer.Close(); err != nil {
				t.Fatalf("close multipart writer: %v", err)
			}

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, tt.endpoint, &requestBody)
			ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

			info := relayInfoWithModel(tt.model)
			info.ChannelBaseUrl = uploadServer.URL
			info.ApiKey = "aipdd-key"
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)

			if taskErr := adaptor.ValidateRequestAndSetAction(ctx, info); taskErr != nil {
				t.Fatalf("ValidateRequestAndSetAction returned task error: %v", taskErr)
			}
			bodyReader, err := adaptor.BuildRequestBody(ctx, info)
			if err != nil {
				t.Fatalf("BuildRequestBody returned error: %v", err)
			}
			bodyBytes, err := io.ReadAll(bodyReader)
			if err != nil {
				t.Fatalf("read built request body: %v", err)
			}

			var payload createTaskPayload
			if err := common.Unmarshal(bodyBytes, &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			content := payload.Input
			t.Logf("input=%#v", payload.Input)
			for key, want := range tt.wantFields {
				if got := anyToString(content[key]); got != want {
					t.Fatalf("unexpected %s: got %q want %q in %#v", key, got, want, content)
				}
			}
			if _, ok := content["filename"]; ok {
				t.Fatalf("filename should not be injected: %#v", content)
			}
		})
	}
}

func TestResolveAIPDDUploadTargetAliases(t *testing.T) {
	flux, _ := resolveModelConfig(ModelFluxGGUF)
	target, direct, ok := resolveAIPDDUploadTarget(flux, "file")
	if !ok || direct || target != "image" {
		t.Fatalf("file alias should resolve to image, got target=%q direct=%v ok=%v", target, direct, ok)
	}

	latentsync, _ := resolveModelConfig(ModelLatentsync15)
	target, direct, ok = resolveAIPDDUploadTarget(latentsync, "audio")
	if !ok || direct || target != "LoadAudio" {
		t.Fatalf("audio alias should resolve to LoadAudio, got target=%q direct=%v ok=%v", target, direct, ok)
	}

	target, direct, ok = resolveAIPDDUploadTarget(latentsync, "LoadAudio")
	if !ok || !direct || target != "LoadAudio" {
		t.Fatalf("LoadAudio should resolve directly, got target=%q direct=%v ok=%v", target, direct, ok)
	}

	indexTTS, _ := resolveModelConfig(ModelIndexTTS)
	target, direct, ok = resolveAIPDDUploadTarget(indexTTS, "ref_audio")
	if !ok || direct || target != "audio" {
		t.Fatalf("ref_audio should resolve to audio, got target=%q direct=%v ok=%v", target, direct, ok)
	}

	mimicMotion, _ := resolveModelConfig(ModelMimicMotion)
	target, direct, ok = resolveAIPDDUploadTarget(mimicMotion, "image")
	if !ok || direct || target != "appearance_image" {
		t.Fatalf("image alias should resolve to appearance_image, got target=%q direct=%v ok=%v", target, direct, ok)
	}
}

func TestWan22WanxIgnoresUnsupportedDurationForJavaBackend(t *testing.T) {
	adaptor := &TaskAdaptor{}
	payload, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model:    ModelWan22Wanx,
		Prompt:   "camera push in",
		Image:    "https://cdn.example.com/input.png",
		Duration: 7,
	}, relayInfoWithModel(ModelWan22Wanx))
	if err != nil {
		t.Fatalf("convertToRequestPayload returned error: %v", err)
	}
	if _, ok := payload.Input["duration"]; ok {
		t.Fatalf("duration should not be forwarded: %#v", payload.Input)
	}
}

func TestFluxGGUFRequiresImageForJavaBackend(t *testing.T) {
	adaptor := &TaskAdaptor{}
	_, err := adaptor.convertToRequestPayload(relaycommon.TaskSubmitReq{
		Model:  ModelFluxGGUF,
		Prompt: "a cinematic robot",
	}, relayInfoWithModel(ModelFluxGGUF))
	if err == nil {
		t.Fatal("expected missing image validation error")
	}
}

func TestPerCallBillingCapabilities(t *testing.T) {
	if !constant.IsAIPDDPerCallBillingModel(ModelWan22Animater) {
		t.Fatal("subject replacement should be per-call billed")
	}
	if !constant.IsAIPDDPerCallBillingModel(ModelWan22Wanx) {
		t.Fatal("wanx image-to-video should be per-call billed for Java backend")
	}
}

func TestDoResponseReturnsAsyncTaskForIndexTTS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":200,"message":"ok","data":{"id":"upstream-task","task_status":0}}`)),
	}
	info := relayInfoWithModel(ModelIndexTTS)
	info.OriginModelName = ModelIndexTTS
	info.PublicTaskID = "task_public"

	taskID, _, taskErr := adaptor.DoResponse(ctx, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse returned task error: %v", taskErr)
	}
	if taskID != "upstream-task" {
		t.Fatalf("unexpected upstream task id: %s", taskID)
	}
	var body map[string]any
	if err := common.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if body["task_id"] != "task_public" || body["object"] != "audio.speech.task" {
		t.Fatalf("unexpected async task response: %#v", body)
	}
}

func TestDoResponseParsesJavaCreateTaskResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	adaptor := &TaskAdaptor{}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"code":0,"message":"created","data":{"id":"java-task","taskTypeCode":"aipdd_IndexTTS","status":"QUEUED"}}`)),
	}
	info := relayInfoWithModel(ModelIndexTTS)
	info.OriginModelName = ModelIndexTTS
	info.PublicTaskID = "task_public"

	taskID, _, taskErr := adaptor.DoResponse(ctx, resp, info)
	if taskErr != nil {
		t.Fatalf("DoResponse returned task error: %v", taskErr)
	}
	if taskID != "java-task" {
		t.Fatalf("unexpected upstream task id: %s", taskID)
	}
}

func TestFetchTaskFollowsJavaResultEndpoint(t *testing.T) {
	var sawDetail bool
	var sawResult bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "aipdd-key" {
			t.Fatalf("unexpected api key header: %q", r.Header.Get("X-API-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/shared-tasks/tasks/java-task/detail":
			sawDetail = true
			_, _ = w.Write([]byte(`{"code":0,"message":"fetched","data":{"id":"java-task","taskTypeCode":"aipdd_wan2.2_wanx","status":"SUCCESS","progress":100,"resultReady":true}}`))
		case "/shared-tasks/tasks/java-task/result":
			sawResult = true
			_, _ = w.Write([]byte(`{"code":0,"message":"fetched","data":{"taskId":"java-task","status":"PENDING_CONFIRMATION","output":{"url":"https://oss.example.com/result.mp4"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adaptor := &TaskAdaptor{}
	resp, err := adaptor.FetchTask(server.URL, "aipdd-key", map[string]any{"task_id": "java-task"}, "")
	if err != nil {
		t.Fatalf("FetchTask returned error: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if !sawDetail || !sawResult {
		t.Fatalf("expected detail and result endpoints to be called, detail=%v result=%v", sawDetail, sawResult)
	}
	if info.Status != model.TaskStatusSuccess || info.Url != "https://oss.example.com/result.mp4" {
		t.Fatalf("unexpected task info: %+v body=%s", info, string(body))
	}
}

func relayInfoWithModel(modelName string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: modelName,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestParseTaskResultExtractsNonJSONURLArray(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":2,"task_result":"[https://cdn.example.com/a.mp4,https://cdn.example.com/b.mp4]"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Url != "https://cdn.example.com/a.mp4" {
		t.Fatalf("unexpected result URL: %s", info.Url)
	}
}

func TestParseTaskResultTreatsSuccessFalseAsFailure(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":3,"task_result":"{\"success\":false,\"message\":\"render failed\"}"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusFailure {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Reason != "render failed" {
		t.Fatalf("unexpected reason: %s", info.Reason)
	}
}

func TestParseTaskResultTreatsStatusTwoURLResultAsSuccess(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":2,"task_result":"https://oss.aipdd.work/distributed_compute/task-1/result.wav"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Url != "https://oss.aipdd.work/distributed_compute/task-1/result.wav" {
		t.Fatalf("unexpected result URL: %s", info.Url)
	}
}

func TestParseTaskResultTreatsStatusFourURLResultAsSuccess(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":4,"task_result":"https://oss.aipdd.work/distributed_compute/task-1/result.mp4"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusSuccess {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Url != "https://oss.aipdd.work/distributed_compute/task-1/result.mp4" {
		t.Fatalf("unexpected result URL: %s", info.Url)
	}
}

func TestParseTaskResultTreatsCompletedErrorTextAsFailure(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body := []byte(`{"code":200,"message":"获取成功","data":{"id":"task-1","task_status":3,"task_result":"ComfyUI ??: prompt_outputs_failed_validation - Prompt outputs failed validation"}}`)

	info, err := adaptor.ParseTaskResult(body)
	if err != nil {
		t.Fatalf("ParseTaskResult returned error: %v", err)
	}
	if info.Status != model.TaskStatusFailure {
		t.Fatalf("unexpected status: %s", info.Status)
	}
	if info.Url != "" {
		t.Fatalf("error text should not be treated as URL: %s", info.Url)
	}
	if info.Reason != "ComfyUI ??: prompt_outputs_failed_validation - Prompt outputs failed validation" {
		t.Fatalf("unexpected reason: %s", info.Reason)
	}
}
