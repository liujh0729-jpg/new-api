package constant

type EndpointType string

const (
	EndpointTypeOpenAI                EndpointType = "openai"
	EndpointTypeOpenAIResponse        EndpointType = "openai-response"
	EndpointTypeOpenAIResponseCompact EndpointType = "openai-response-compact"
	EndpointTypeAnthropic             EndpointType = "anthropic"
	EndpointTypeGemini                EndpointType = "gemini"
	EndpointTypeJinaRerank            EndpointType = "jina-rerank"
	EndpointTypeImageGeneration       EndpointType = "image-generation"
	EndpointTypeImageEdits            EndpointType = "image-edits"
	EndpointTypeEmbeddings            EndpointType = "embeddings"
	EndpointTypeOpenAIVideo           EndpointType = "openai-video"
	EndpointTypeAudioSpeech           EndpointType = "audio-speech"
	EndpointTypeAudioTranscriptions   EndpointType = "audio-transcriptions"
	EndpointTypeSystemOne             EndpointType = "systemone"
	//EndpointTypeMidjourney     EndpointType = "midjourney-proxy"
	//EndpointTypeSuno           EndpointType = "suno-proxy"
	//EndpointTypeKling          EndpointType = "kling"
	//EndpointTypeJimeng         EndpointType = "jimeng"
)
