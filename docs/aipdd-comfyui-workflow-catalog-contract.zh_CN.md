# AIPDD ComfyUI Workflow Catalog 返回规范

本文档用于说明 AIPDD 上游接口 `/scripts/admin/comfyui_workflow` 应如何返回模型能力信息，方便 new-api 在不改变现有规则的前提下识别模型适用场景，并在游乐场中按文本、图片、视频、音频等模式筛选和调用模型。

## 目标

当前 AIPDD 已能返回 workflow 的 `code`、`name`、`description`、`priceAWcoin` 和 `params`。这些字段可以让 new-api 得到精确 workflow 参数名，但还不足以稳定判断模型属于文生图、图生图、图生视频、对口型、主体替换、语音合成等哪一种任务。

建议在每个 workflow 条目上新增结构化能力字段：

```json
{
  "endpointType": "image-generation",
  "taskKind": "image_to_image",
  "inputModalities": ["image", "text"],
  "outputModalities": ["image"]
}
```

## 必须遵守的 new-api 规则

`endpointType` 必须使用 new-api 已有值，不新增命名规则：

| endpointType | 含义 | 常用接口 |
|---|---|---|
| `image-generation` | 图片生成任务 | `/v1/images/generations` |
| `openai-video` | 视频生成/视频处理任务 | `/v1/videos` 或 `/v1/video/generations` |
| `audio-speech` | 语音合成任务 | `/v1/audio/speech` |

不要返回 `image_generation`、`video_generation`、`audio_speech` 这类下划线值，除非 new-api 额外做映射。

## 推荐返回结构

接口仍然返回现有成功结构：

```json
{
  "code": 200,
  "message": "获取成功",
  "data": []
}
```

`data` 中每个 workflow 条目建议如下：

```json
{
  "id": "script-id",
  "code": "aipdd_latentsync1.5",
  "name": "aipdd_latentsync1.5",
  "description": "视频对口型工作流",
  "priceAWcoin": 1000,
  "endpointType": "openai-video",
  "taskKind": "lip_sync",
  "inputModalities": ["video", "audio"],
  "outputModalities": ["video"],
  "params": [
    {
      "id": "param-id",
      "paramKey": "video",
      "paramName": "输入视频",
      "paramDesc": "需要对口型的视频 URL",
      "defaultValue": null,
      "dataType": "string",
      "isRequired": true,
      "orderNo": 1,
      "maxDuration": 0,
      "maxFileSize": 0,
      "uiType": "video_url"
    },
    {
      "id": "param-id",
      "paramKey": "LoadAudio",
      "paramName": "输入音频",
      "paramDesc": "驱动口型的音频 URL",
      "defaultValue": null,
      "dataType": "string",
      "isRequired": true,
      "orderNo": 2,
      "maxDuration": 0,
      "maxFileSize": 0,
      "uiType": "audio_url"
    }
  ]
}
```

## 字段说明

### endpointType

必填。表示该 workflow 应该归属到 new-api 的哪类接口。

允许值：

```text
image-generation
openai-video
audio-speech
```

### taskKind

必填。表示更细的任务类型，用于游乐场生成合适的表单和提示。

推荐值：

| taskKind | 含义 |
|---|---|
| `text_to_image` | 文生图 |
| `image_to_image` | 图生图 |
| `image_to_video` | 图生视频 |
| `video_to_video` | 视频生成/视频编辑 |
| `lip_sync` | 对口型 |
| `motion_transfer` | 动作迁移 |
| `subject_replace` | 更换主体 |
| `voice_clone` | 声音复刻 |
| `text_to_speech` | 文本转语音 |

### inputModalities

必填。表示模型输入需要什么类型的内容。

允许值：

```text
text
image
video
audio
file
```

### outputModalities

必填。表示模型输出类型。

允许值：

```text
image
video
audio
text
file
```

### params

必填。表示 workflow 的实际参数定义。`paramKey` 必须是最终调用 `/comfyui/task/create` 时 `task_content` 里的精确字段名，大小写需要保持一致。

已有字段继续保留：

| 字段 | 说明 |
|---|---|
| `paramKey` | 精确 workflow 参数名 |
| `dataType` | 参数类型，例如 `string`、`int`、`float`、`bool`、`json` |
| `isRequired` | 是否必填 |
| `defaultValue` | 默认值 |
| `maxDuration` | 最大时长，单位秒，`0` 表示不限制 |
| `maxFileSize` | 最大文件大小，单位字节，`0` 表示不限制 |

建议新增：

| 字段 | 说明 |
|---|---|
| `uiType` | 游乐场表单控件类型 |
| `acceptedMimeTypes` | 文件上传可接受 MIME 类型 |
| `aliases` | 可选别名，方便兼容前端通用字段 |

`uiType` 推荐值：

```text
text
textarea
image_url
video_url
audio_url
file_url
number
select
switch
```

示例：

```json
{
  "paramKey": "appearance_image",
  "dataType": "string",
  "isRequired": true,
  "uiType": "image_url",
  "acceptedMimeTypes": ["image/*"],
  "aliases": ["image", "reference_image", "subject_image"]
}
```

## 当前 7 个 AIPDD workflow 建议返回

### FLUX-GGUF-T2I-V2

```json
{
  "code": "FLUX-GGUF-T2I-V2",
  "endpointType": "image-generation",
  "taskKind": "text_to_image",
  "inputModalities": ["text"],
  "outputModalities": ["image"],
  "params": [
    {
      "paramKey": "text",
      "dataType": "string",
      "isRequired": true,
      "uiType": "textarea"
    }
  ]
}
```

### FLUX-GGUF-V2

```json
{
  "code": "FLUX-GGUF-V2",
  "endpointType": "image-generation",
  "taskKind": "image_to_image",
  "inputModalities": ["image", "text"],
  "outputModalities": ["image"],
  "params": [
    {
      "paramKey": "positive_prompt",
      "dataType": "string",
      "isRequired": true,
      "uiType": "textarea"
    },
    {
      "paramKey": "image",
      "dataType": "string",
      "isRequired": true,
      "uiType": "image_url",
      "acceptedMimeTypes": ["image/*"]
    }
  ]
}
```

### aipdd_wan2.2_wanx

```json
{
  "code": "aipdd_wan2.2_wanx",
  "endpointType": "openai-video",
  "taskKind": "image_to_video",
  "inputModalities": ["image", "text"],
  "outputModalities": ["video"],
  "params": [
    {
      "paramKey": "image",
      "dataType": "string",
      "isRequired": true,
      "uiType": "image_url",
      "acceptedMimeTypes": ["image/*"]
    },
    {
      "paramKey": "prompt",
      "dataType": "string",
      "isRequired": true,
      "uiType": "textarea"
    },
    {
      "paramKey": "negative_prompt",
      "dataType": "string",
      "isRequired": false,
      "uiType": "textarea"
    },
    {
      "paramKey": "fps",
      "dataType": "string",
      "isRequired": false,
      "uiType": "number"
    }
  ]
}
```

### aipdd_Wan2.2-Animater

```json
{
  "code": "aipdd_Wan2.2-Animater",
  "endpointType": "openai-video",
  "taskKind": "subject_replace",
  "inputModalities": ["video", "image", "text"],
  "outputModalities": ["video"],
  "params": [
    {
      "paramKey": "video",
      "dataType": "string",
      "isRequired": true,
      "uiType": "video_url",
      "acceptedMimeTypes": ["video/*"]
    },
    {
      "paramKey": "image",
      "dataType": "string",
      "isRequired": false,
      "uiType": "image_url",
      "acceptedMimeTypes": ["image/*"]
    },
    {
      "paramKey": "positive_prompt",
      "dataType": "string",
      "isRequired": true,
      "uiType": "textarea"
    },
    {
      "paramKey": "filename_prefix",
      "dataType": "string",
      "isRequired": false,
      "uiType": "text"
    }
  ]
}
```

### aipdd_mimic_motion

```json
{
  "code": "aipdd_mimic_motion",
  "endpointType": "openai-video",
  "taskKind": "motion_transfer",
  "inputModalities": ["video", "image"],
  "outputModalities": ["video"],
  "params": [
    {
      "paramKey": "motion_video",
      "dataType": "string",
      "isRequired": true,
      "uiType": "video_url",
      "acceptedMimeTypes": ["video/*"]
    },
    {
      "paramKey": "appearance_image",
      "dataType": "string",
      "isRequired": true,
      "uiType": "image_url",
      "acceptedMimeTypes": ["image/*"]
    }
  ]
}
```

### aipdd_latentsync1.5

```json
{
  "code": "aipdd_latentsync1.5",
  "endpointType": "openai-video",
  "taskKind": "lip_sync",
  "inputModalities": ["video", "audio"],
  "outputModalities": ["video"],
  "params": [
    {
      "paramKey": "video",
      "dataType": "string",
      "isRequired": true,
      "uiType": "video_url",
      "acceptedMimeTypes": ["video/*"]
    },
    {
      "paramKey": "LoadAudio",
      "dataType": "string",
      "isRequired": true,
      "uiType": "audio_url",
      "acceptedMimeTypes": ["audio/*"]
    }
  ]
}
```

### aipdd_IndexTTS

```json
{
  "code": "aipdd_IndexTTS",
  "endpointType": "audio-speech",
  "taskKind": "voice_clone",
  "inputModalities": ["audio", "text"],
  "outputModalities": ["audio"],
  "params": [
    {
      "paramKey": "audio",
      "dataType": "string",
      "isRequired": true,
      "uiType": "audio_url",
      "acceptedMimeTypes": ["audio/*"]
    },
    {
      "paramKey": "emotion_audio",
      "dataType": "string",
      "isRequired": false,
      "uiType": "audio_url",
      "acceptedMimeTypes": ["audio/*"]
    },
    {
      "paramKey": "text",
      "dataType": "string",
      "isRequired": true,
      "uiType": "textarea"
    }
  ]
}
```

## new-api 识别规则建议

在不修改 new-api 现有 endpoint 规则的情况下，AIPDD 上游应保证：

1. 每个可在游乐场调用的 workflow 必须返回 `endpointType`。
2. `endpointType` 必须是 `image-generation`、`openai-video`、`audio-speech` 之一。
3. 每个可自动生成表单的 workflow 必须返回 `taskKind`。
4. 每个 workflow 必须返回完整 `params`，其中 `paramKey` 是实际 task_content 字段。
5. `inputModalities` 和 `outputModalities` 必须能解释该模型的输入输出类型。
6. 如果某个 workflow 缺少这些结构化字段，new-api 可以仍然把它作为渠道模型同步，但不应展示到游乐场的对应模式里。

## 价格接口保持不变

`/fee-rules` 继续作为实际扣费规则来源。

`/scripts/admin/comfyui_workflow` 中的 `priceAWcoin` 可以保留，但 new-api 更推荐以 `/fee-rules` 中和 `script_code` 匹配的 `price` 为准。

`/system/awcoin-rate` 应继续返回：

```json
{
  "code": 200,
  "message": "获取成功",
  "data": {
    "rmb": 0.01,
    "usd": 0.001475,
    "updatedAt": "2026-06-03T21:38:28"
  }
}
```

new-api 会用 `usd` 将 AIPDD 积分价格换算成自身的 `model_price`。

