package openai

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const sttTestModel = "ap-funasr"

// installSTTPricingForTest registers the catalog capability and the imported
// per-second price the same way the catalog sync does at runtime. A unitPrice of
// zero leaves the model unpriced, as it is before the first catalog sync.
func installSTTPricingForTest(t *testing.T, synchronous bool, unitPrice float64) {
	t.Helper()

	previousCapabilities := constant.GetAIPDDCapabilities()
	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{{
		ModelName:     sttTestModel,
		Synchronous:   synchronous,
		BillingType:   constant.AIPDDBillingTypeDurationSeconds,
		EndpointType:  constant.EndpointTypeAudioTranscriptions,
		ExecutionPath: "/v1/audio/transcriptions",
	}})
	t.Cleanup(func() { constant.SetAIPDDCapabilities(previousCapabilities) })

	billing := config.GlobalConfig.Get("billing_setting")
	require.NotNil(t, billing, "billing_setting is not registered")
	snapshot, err := config.ConfigToMap(billing)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, config.UpdateConfigFromMap(billing, snapshot))
	})

	modeJSON := "{}"
	pricingJSON := "{}"
	if unitPrice > 0 {
		modeJSON = fmt.Sprintf(`{%q:%q}`, sttTestModel, billing_setting.BillingModeTaskPricing)
		pricingJSON = fmt.Sprintf(`{%q:{"unit":"second","no_reference_video_unit_price":%v,"reference_video_policy":"same"}}`, sttTestModel, unitPrice)
	}
	require.NoError(t, config.UpdateConfigFromMap(billing, map[string]string{
		"billing_mode": modeJSON,
		"task_pricing": pricingJSON,
	}))
}

func newSTTTestContext(t *testing.T, body []byte, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	if body == nil {
		body = []byte{}
	}
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", contentType)
	return c, recorder
}

func upstreamSTTResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

const verboseSTTBody = `{"text":"我。","task":"transcribe","language":"en","duration":10.0,"segments":[{"id":0,"start":0,"end":9.98,"text":"我。"}]}`

func TestOpenaiSTTHandlerBillsReportedDuration(t *testing.T) {
	installSTTPricingForTest(t, true, 0.001)

	tests := []struct {
		name            string
		clientFormat    string
		wantBody        string
		wantContentType string
	}{
		{
			name:            "json clients only see the transcript",
			clientFormat:    "json",
			wantBody:        `{"text":"我。"}`,
			wantContentType: "application/json",
		},
		{
			name:            "text clients get plain text",
			clientFormat:    "text",
			wantBody:        "我。",
			wantContentType: "text/plain; charset=utf-8",
		},
		{
			name:            "verbose_json clients get the upstream answer",
			clientFormat:    "verbose_json",
			wantBody:        verboseSTTBody,
			wantContentType: "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newSTTTestContext(t, nil, "application/json")
			info := &relaycommon.RelayInfo{OriginModelName: sttTestModel}
			apiErr, _ := OpenaiSTTHandler(c, upstreamSTTResponse(verboseSTTBody), info, tt.clientFormat)

			require.Nil(t, apiErr)
			require.Equal(t, tt.wantBody, recorder.Body.String())
			require.Equal(t, tt.wantContentType, recorder.Header().Get("Content-Type"))
			require.Equal(t, 10.0, info.PriceData.OtherRatios["seconds"])
		})
	}
}

func TestOpenaiSTTHandlerLeavesOtherFormatsUntouched(t *testing.T) {
	installSTTPricingForTest(t, true, 0.001)

	// The gateway cannot rebuild an srt answer from verbose_json, so it must not
	// ask for one; the upstream response reaches the client verbatim. With no
	// duration from either source, one second is billed rather than nothing.
	srtBody := "1\n00:00:00,000 --> 00:00:09,980\n我。\n"
	c, recorder := newSTTTestContext(t, nil, "application/json")
	info := &relaycommon.RelayInfo{OriginModelName: sttTestModel}
	apiErr, _ := OpenaiSTTHandler(c, upstreamSTTResponse(srtBody), info, "srt")

	require.Nil(t, apiErr)
	require.Equal(t, srtBody, recorder.Body.String())
	require.Equal(t, 1.0, info.PriceData.OtherRatios["seconds"])
}

func TestOpenaiSTTHandlerMeasuresUploadedAudio(t *testing.T) {
	installSTTPricingForTest(t, true, 0.001)

	// A two-second 8 kHz mono 16-bit PCM recording.
	const samples = 16000
	var audio bytes.Buffer
	audio.WriteString("RIFF")
	audio.Write(make([]byte, 4))
	audio.WriteString("WAVEfmt ")
	binary.Write(&audio, binary.LittleEndian, uint32(16))
	binary.Write(&audio, binary.LittleEndian, uint16(1))
	binary.Write(&audio, binary.LittleEndian, uint16(1))
	binary.Write(&audio, binary.LittleEndian, uint32(8000))
	binary.Write(&audio, binary.LittleEndian, uint32(16000))
	binary.Write(&audio, binary.LittleEndian, uint16(2))
	binary.Write(&audio, binary.LittleEndian, uint16(16))
	audio.WriteString("data")
	binary.Write(&audio, binary.LittleEndian, uint32(samples*2))
	audio.Write(make([]byte, samples*2))
	binary.LittleEndian.PutUint32(audio.Bytes()[4:], uint32(audio.Len()-8))

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	part, err := writer.CreateFormFile("file", "recording.wav")
	require.NoError(t, err)
	_, err = part.Write(audio.Bytes())
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("model", sttTestModel))
	require.NoError(t, writer.Close())

	c, _ := newSTTTestContext(t, form.Bytes(), writer.FormDataContentType())
	info := &relaycommon.RelayInfo{OriginModelName: sttTestModel}
	// The upstream answered without a duration, so the uploaded file is measured.
	apiErr, _ := OpenaiSTTHandler(c, upstreamSTTResponse(`{"text":"我。"}`), info, "json")

	require.Nil(t, apiErr)
	require.InDelta(t, 2.0, info.PriceData.OtherRatios["seconds"], 0.01)
}

func TestOpenaiSTTHandlerSkipsUnpricedModels(t *testing.T) {
	installSTTPricingForTest(t, true, 0)

	body := `{"text":"hello"}`
	c, recorder := newSTTTestContext(t, nil, "application/json")
	info := &relaycommon.RelayInfo{OriginModelName: sttTestModel}
	apiErr, _ := OpenaiSTTHandler(c, upstreamSTTResponse(body), info, "json")

	require.Nil(t, apiErr)
	require.Equal(t, body, recorder.Body.String())
	require.Nil(t, info.PriceData.OtherRatios)
}

func TestOpenaiSTTHandlerLeavesTaskPricedModelsToTheTaskPipeline(t *testing.T) {
	// A video model priced per second settles through the task pipeline, which
	// resolves the duration from the request. The synchronous relay must not
	// multiply its price by a duration this endpoint never reports.
	installSTTPricingForTest(t, false, 0.001)

	body := `{"text":"hello"}`
	c, recorder := newSTTTestContext(t, nil, "application/json")
	info := &relaycommon.RelayInfo{OriginModelName: sttTestModel}
	apiErr, _ := OpenaiSTTHandler(c, upstreamSTTResponse(body), info, "json")

	require.Nil(t, apiErr)
	require.Equal(t, body, recorder.Body.String())
	require.Nil(t, info.PriceData.OtherRatios)
}

func TestSTTFormatIsRebuildable(t *testing.T) {
	require.True(t, sttFormatIsRebuildable(""))
	require.True(t, sttFormatIsRebuildable("json"))
	require.True(t, sttFormatIsRebuildable("Text"))
	require.True(t, sttFormatIsRebuildable("verbose_json"))
	require.False(t, sttFormatIsRebuildable("srt"))
	require.False(t, sttFormatIsRebuildable("vtt"))
	require.False(t, sttFormatIsRebuildable("audio"))
}

func TestConvertAudioRequestForcesVerboseJSON(t *testing.T) {
	tests := []struct {
		name          string
		priced        bool
		clientFormat  string
		wantSentField string
	}{
		{name: "priced json", priced: true, clientFormat: "json", wantSentField: sttVerboseJSONFormat},
		{name: "priced text", priced: true, clientFormat: "text", wantSentField: sttVerboseJSONFormat},
		{name: "priced srt keeps its own format", priced: true, clientFormat: "srt", wantSentField: "srt"},
		{name: "unpriced json keeps its own format", priced: false, clientFormat: "json", wantSentField: "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unitPrice := 0.0
			if tt.priced {
				unitPrice = 0.001
			}
			installSTTPricingForTest(t, true, unitPrice)

			var form bytes.Buffer
			writer := multipart.NewWriter(&form)
			part, err := writer.CreateFormFile("file", "recording.wav")
			require.NoError(t, err)
			_, err = part.Write([]byte("RIFFxxxxWAVE"))
			require.NoError(t, err)
			require.NoError(t, writer.WriteField("model", sttTestModel))
			require.NoError(t, writer.WriteField("response_format", tt.clientFormat))
			require.NoError(t, writer.WriteField("language", "zh"))
			require.NoError(t, writer.Close())

			c, _ := newSTTTestContext(t, form.Bytes(), writer.FormDataContentType())
			info := &relaycommon.RelayInfo{
				OriginModelName: sttTestModel,
				RelayMode:       relayconstant.RelayModeAudioTranscription,
			}
			reader, err := (&Adaptor{}).ConvertAudioRequest(c, info, dto.AudioRequest{
				Model:          sttTestModel,
				ResponseFormat: tt.clientFormat,
			})
			require.NoError(t, err)

			_, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
			require.NoError(t, err)
			sent, err := multipart.NewReader(reader, params["boundary"]).ReadForm(1 << 20)
			require.NoError(t, err)
			defer sent.RemoveAll()

			require.Equal(t, []string{tt.wantSentField}, sent.Value["response_format"])
			require.Equal(t, []string{"zh"}, sent.Value["language"])
			require.Len(t, sent.File["file"], 1)
		})
	}
}
