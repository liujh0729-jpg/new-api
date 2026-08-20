package dto

type VideoRequest struct {
	Model                 string                    `json:"model,omitempty" example:"AP Seedance-2.0 标准版"`                 // Model/style ID
	Prompt                string                    `json:"prompt,omitempty" example:"电影感城市夜景"`                            // Text prompt
	Image                 string                    `json:"image,omitempty" example:"https://example.com/reference.jpg"`   // Image input (URL/Base64)
	Duration              *float64                  `json:"duration,omitempty" example:"5.0"`                              // Video duration (seconds)
	Width                 *int                      `json:"width,omitempty" example:"1280"`                                // Video width
	Height                *int                      `json:"height,omitempty" example:"720"`                                // Video height
	Fps                   *int                      `json:"fps,omitempty" example:"24"`                                    // Video frame rate
	Seed                  *int                      `json:"seed,omitempty" example:"0"`                                    // Random seed
	N                     *int                      `json:"n,omitempty" example:"1"`                                       // Number of videos to generate
	Resolution            *string                   `json:"resolution,omitempty" example:"720p"`                           // Seedance resolution tier
	Ratio                 *string                   `json:"ratio,omitempty" example:"16:9"`                                // Seedance aspect ratio
	Content               []VideoRequestContentItem `json:"content,omitempty"`                                             // Seedance official multimodal content
	GenerateAudio         *bool                     `json:"generate_audio,omitempty" example:"false"`                      // Generate synchronized audio
	Watermark             *bool                     `json:"watermark,omitempty" example:"false"`                           // Add a visible watermark
	OutputFormat          *string                   `json:"output_format,omitempty" example:"mp4"`                         // Seedance output container: mp4 or mov
	ReturnLastFrame       *bool                     `json:"return_last_frame,omitempty" example:"false"`                   // Return the generated last frame
	OmniReferenceTaskType *string                   `json:"omni_reference_task_type,omitempty" example:"auto"`             // auto, reference, edit, or extend
	ServiceTier           *string                   `json:"service_tier,omitempty"`                                        // Seedance service tier
	Priority              *int                      `json:"priority,omitempty" example:"0"`                                // Seedance task priority
	ExecutionExpiresAfter *int                      `json:"execution_expires_after,omitempty" example:"172800"`            // Task execution expiry in seconds
	SafetyIdentifier      *string                   `json:"safety_identifier,omitempty" example:"tenant-user-hash"`        // Stable end-user safety identifier
	Tools                 []VideoRequestTool        `json:"tools,omitempty"`                                               // Optional Seedance tools
	CallbackURL           *string                   `json:"callback_url,omitempty" example:"https://example.com/callback"` // Completion callback URL
	ResponseFormat        string                    `json:"response_format,omitempty" example:"url"`                       // Response format
	User                  string                    `json:"user,omitempty" example:"user-1234"`                            // User identifier
	Metadata              map[string]any            `json:"metadata,omitempty"`                                            // Vendor-specific/custom params
}

type VideoRequestTool struct {
	Type string `json:"type" example:"web_search"`
}

type VideoRequestContentItem struct {
	Type     string                `json:"type" example:"text"`
	Role     *string               `json:"role,omitempty" example:"reference_image"`
	Text     *string               `json:"text,omitempty" example:"电影感城市夜景"`
	ImageURL *VideoRequestMediaURL `json:"image_url,omitempty"`
	VideoURL *VideoRequestMediaURL `json:"video_url,omitempty"`
	AudioURL *VideoRequestMediaURL `json:"audio_url,omitempty"`
}

type VideoRequestMediaURL struct {
	URL string `json:"url" example:"https://example.com/reference.jpg"`
}

// VideoResponse 视频生成提交任务后的响应
type VideoResponse struct {
	TaskId string `json:"task_id"`
	Status string `json:"status"`
}

// VideoTaskResponse 查询视频生成任务状态的响应
type VideoTaskResponse struct {
	TaskId   string             `json:"task_id" example:"abcd1234efgh"` // 任务ID
	Status   string             `json:"status" example:"succeeded"`     // 任务状态
	Url      string             `json:"url,omitempty"`                  // 视频资源URL（成功时）
	Format   string             `json:"format,omitempty" example:"mp4"` // 视频格式
	Metadata *VideoTaskMetadata `json:"metadata,omitempty"`             // 结果元数据
	Error    *VideoTaskError    `json:"error,omitempty"`                // 错误信息（失败时）
}

// VideoTaskMetadata 视频任务元数据
type VideoTaskMetadata struct {
	Duration float64 `json:"duration" example:"5.0"`  // 实际生成的视频时长
	Fps      int     `json:"fps" example:"30"`        // 实际帧率
	Width    int     `json:"width" example:"512"`     // 实际宽度
	Height   int     `json:"height" example:"512"`    // 实际高度
	Seed     int     `json:"seed" example:"20231234"` // 使用的随机种子
}

// VideoTaskError 视频任务错误信息
type VideoTaskError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
