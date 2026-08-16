package service

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/volcengine/volcengine-go-sdk/volcengine/volcengineerr"
)

var (
	volcErrorCodePattern      = regexp.MustCompile(`(?i)(?:Volc\s+\w+\s+failed:\s*)?([^\s:]+):\s*`)
	volcErrorStatusPattern    = regexp.MustCompile(`(?i)status\s*code\s*:\s*(\d{3})`)
	volcErrorRequestIDPattern = regexp.MustCompile(`(?i)request\s*id\s*:\s*([a-z0-9_-]+)`)
	volcAspectRatioPattern    = regexp.MustCompile(`(?i)aspect\s*ratio.*?between\s*([0-9]+(?:\.[0-9]+)?)\s*and\s*([0-9]+(?:\.[0-9]+)?)`)
	volcMetadataPattern       = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)
)

// LocalizeVolcAssetError converts known Volc Assets API failures into an
// actionable Chinese message while retaining the upstream code and request ID
// needed for support diagnostics.
func LocalizeVolcAssetError(err error) string {
	if err == nil {
		return ""
	}
	code, message, requestID, statusCode := volcErrorDetails(err)
	return LocalizeVolcAssetErrorDetails(code, message, requestID, statusCode)
}

// LocalizeVolcAssetErrorDetails handles asynchronous asset failures returned
// by GetAsset as well as synchronous SDK request failures.
func LocalizeVolcAssetErrorDetails(code, message, requestID string, statusCode int) string {
	code = strings.TrimSpace(code)
	translationCode := code
	message = common.MaskSensitiveInfo(strings.TrimSpace(message))
	requestID = strings.TrimSpace(requestID)
	if !volcMetadataPattern.MatchString(code) {
		code = ""
	}
	if !volcMetadataPattern.MatchString(requestID) {
		requestID = ""
	}

	localized := localizedVolcAssetReason(translationCode, message, statusCode)
	if localized == "" {
		localized = "火山素材服务请求失败"
		if message != "" {
			localized += "：" + message
		}
	}

	metadata := make([]string, 0, 3)
	if code != "" {
		metadata = append(metadata, "错误码："+code)
	}
	if statusCode > 0 {
		metadata = append(metadata, "HTTP 状态码："+strconv.Itoa(statusCode))
	}
	if requestID != "" {
		metadata = append(metadata, "请求 ID："+requestID)
	}
	if len(metadata) > 0 {
		localized += "（" + strings.Join(metadata, "，") + "）"
	}
	return localized
}

func volcErrorDetails(err error) (code, message, requestID string, statusCode int) {
	var requestFailure volcengineerr.RequestFailure
	if errors.As(err, &requestFailure) {
		return requestFailure.Code(), requestFailure.Message(), requestFailure.RequestID(), requestFailure.StatusCode()
	}
	var volcErr volcengineerr.Error
	if errors.As(err, &volcErr) {
		return volcErr.Code(), volcErr.Message(), "", 0
	}

	raw := strings.TrimSpace(err.Error())
	if match := volcErrorCodePattern.FindStringSubmatch(raw); len(match) == 2 {
		code = strings.TrimSpace(match[1])
		message = strings.TrimSpace(raw[volcErrorCodePattern.FindStringIndex(raw)[1]:])
	} else {
		message = raw
	}
	if match := volcErrorStatusPattern.FindStringSubmatch(raw); len(match) == 2 {
		statusCode, _ = strconv.Atoi(match[1])
	}
	if match := volcErrorRequestIDPattern.FindStringSubmatch(raw); len(match) == 2 {
		requestID = strings.TrimSpace(match[1])
	}
	message = strings.TrimSpace(volcErrorStatusPattern.Split(message, 2)[0])
	message = strings.TrimSuffix(message, ",")
	message = strings.TrimSpace(message)
	return code, message, requestID, statusCode
}

func localizedVolcAssetReason(code, message string, statusCode int) string {
	compactCode := strings.NewReplacer(".", "", "_", "", "-", "", " ", "").Replace(strings.ToLower(code))
	lowerMessage := strings.ToLower(message)
	if match := volcAspectRatioPattern.FindStringSubmatch(message); len(match) == 3 {
		return fmt.Sprintf("素材宽高比不符合要求，必须在 %s 到 %s 之间", match[1], match[2])
	}

	switch {
	case strings.Contains(compactCode, "aspectratiotoosmall"),
		strings.Contains(compactCode, "aspectratiotoolarge"),
		strings.Contains(compactCode, "invalidaspectratio"):
		return "素材宽高比不符合要求，请调整到 0.4 至 2.5 之间"
	case strings.Contains(compactCode, "widthtoosmall"), strings.Contains(compactCode, "heighttoosmall"),
		strings.Contains(compactCode, "imagesizetoosmall"), strings.Contains(compactCode, "imageresolutiontoosmall"),
		strings.Contains(compactCode, "resolutiontoosmall"), strings.Contains(compactCode, "pixelcounttoosmall"):
		return "图片尺寸过小，宽和高均需大于 300 像素"
	case strings.Contains(compactCode, "widthtoolarge"), strings.Contains(compactCode, "heighttoolarge"),
		strings.Contains(compactCode, "imagesizetoolarge"), strings.Contains(compactCode, "imageresolutiontoolarge"),
		strings.Contains(compactCode, "resolutiontoolarge"), strings.Contains(compactCode, "pixelcounttoolarge"):
		return "图片尺寸过大，宽和高均需小于 6000 像素"
	case strings.Contains(compactCode, "filesizetoolarge"), strings.Contains(compactCode, "assetsizetoolarge"),
		strings.Contains(compactCode, "filetoolarge"),
		strings.Contains(lowerMessage, "file size") && strings.Contains(lowerMessage, "too large"):
		return "素材文件过大；图片需小于 30 MB"
	case strings.Contains(compactCode, "durationtooshort"):
		return "素材时长过短，请使用时长符合要求的文件"
	case strings.Contains(compactCode, "durationtoolong"):
		return "素材时长过长，请使用时长符合要求的文件"
	case strings.Contains(compactCode, "frameratetoosmall"), strings.Contains(compactCode, "fpstoosmall"):
		return "视频帧率过低，请使用 24 至 60 FPS 的视频"
	case strings.Contains(compactCode, "frameratetoolarge"), strings.Contains(compactCode, "fpstoolarge"):
		return "视频帧率过高，请使用 24 至 60 FPS 的视频"
	case strings.Contains(compactCode, "unsupportedformat"), strings.Contains(compactCode, "formatnotsupported"),
		strings.Contains(compactCode, "unsupportedfileformat"), strings.Contains(compactCode, "fileformatnotsupported"),
		strings.Contains(compactCode, "invalidfileformat"), strings.Contains(compactCode, "invalidmediatype"),
		strings.Contains(compactCode, "mediatypenotsupported"):
		return "素材格式不受支持，请更换为火山素材库支持的文件格式"
	case strings.Contains(compactCode, "corruptedfile"), strings.Contains(compactCode, "decodefailed"),
		strings.Contains(compactCode, "invalidmediafile"):
		return "素材文件损坏或无法解析，请重新导出文件后上传"
	case strings.Contains(compactCode, "downloadfailed"), strings.Contains(compactCode, "urlnotaccessible"),
		strings.Contains(compactCode, "invalidurl"), strings.Contains(compactCode, "fetchfailed"):
		return "火山无法下载暂存素材，请确认素材链接可通过公网访问后重试"
	case strings.Contains(compactCode, "multiplefaces"), strings.Contains(compactCode, "toomanyfaces"):
		return "素材中检测到多张人脸，请仅保留一个清晰主体后重试"
	case strings.Contains(compactCode, "noface"), strings.Contains(compactCode, "facenotfound"):
		return "素材中未检测到清晰人脸，请上传正面、无遮挡的角色图片"
	case strings.Contains(compactCode, "facemismatch"), strings.Contains(compactCode, "notsameperson"),
		strings.Contains(compactCode, "facesimilaritytoolow"):
		return "素材人物与已认证角色不一致，请上传同一人物的清晰图片"
	case strings.Contains(compactCode, "sensitivecontent"), strings.Contains(compactCode, "contentpolicy"),
		strings.Contains(compactCode, "contentrisk"), strings.Contains(compactCode, "moderationfailed"):
		return "素材未通过火山内容安全审核，请更换图片后重试"
	case strings.Contains(compactCode, "assetgroupnotfound"), strings.Contains(compactCode, "groupnotfound"),
		strings.Contains(compactCode, "resourcenotfound"):
		return "目标素材组不存在或已失效，请重新创建角色后上传"
	case strings.Contains(compactCode, "projectnotfound"), strings.Contains(compactCode, "projectmismatch"):
		return "火山项目配置不匹配，请联系管理员检查 ProjectName"
	case strings.Contains(compactCode, "quotaexceeded"), strings.Contains(compactCode, "assetquota"),
		strings.Contains(compactCode, "groupquota"):
		return "火山素材库配额已用尽，请清理已有素材或提升配额"
	case strings.Contains(compactCode, "ratelimit"), strings.Contains(compactCode, "requestlimit"),
		strings.Contains(compactCode, "flowlimit"), strings.Contains(compactCode, "throttl"), statusCode == 429:
		return "火山素材服务请求过于频繁，请稍后重试"
	case strings.Contains(compactCode, "invalidcredential"), strings.Contains(compactCode, "signature"),
		strings.Contains(compactCode, "unauthorized"), statusCode == 401:
		return "火山素材服务鉴权失败，请联系管理员检查 AK/SK 配置"
	case strings.Contains(compactCode, "accessdenied"), strings.Contains(compactCode, "forbidden"),
		strings.Contains(compactCode, "aigcnotavailable"), statusCode == 403:
		return "当前火山账号无权使用该素材能力，请联系管理员检查服务权限和项目配置"
	case strings.Contains(compactCode, "missingparameter"):
		return "火山素材请求缺少必要参数，请联系管理员检查配置"
	case strings.Contains(compactCode, "invalidparameter"), statusCode == 400:
		return "素材参数不符合火山要求，请检查文件尺寸、比例、格式和内容后重试"
	case strings.Contains(compactCode, "timeout"), statusCode == 408, statusCode == 504:
		return "火山素材服务处理超时，请稍后重试"
	case strings.Contains(compactCode, "internalerror"), strings.Contains(compactCode, "serviceunavailable"),
		strings.Contains(compactCode, "serveroverloaded"), statusCode >= 500:
		return "火山素材服务暂时不可用，请稍后重试"
	}
	return ""
}
