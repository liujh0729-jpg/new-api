package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/volcengine/volcengine-go-sdk/volcengine/volcengineerr"
)

func TestLocalizeVolcAssetErrorAspectRatio(t *testing.T) {
	upstream := volcengineerr.NewRequestFailure(
		volcengineerr.New("InvalidParameter.AspectRatioTooSmall", "Aspect ratio must bebetween 0.4 and 2.5.", nil),
		400,
		"202608140002222F71AD72947FB5C3D631",
	)
	message := LocalizeVolcAssetError(fmt.Errorf("Volc CreateAsset failed: %w", upstream))

	require.Contains(t, message, "素材宽高比不符合要求")
	require.Contains(t, message, "0.4 到 2.5")
	require.Contains(t, message, "错误码：InvalidParameter.AspectRatioTooSmall")
	require.Contains(t, message, "HTTP 状态码：400")
	require.Contains(t, message, "请求 ID：202608140002222F71AD72947FB5C3D631")
	require.NotContains(t, message, "Volc CreateAsset failed")
}

func TestLocalizeVolcAssetErrorParsesPlainSDKMessage(t *testing.T) {
	message := LocalizeVolcAssetError(errors.New(
		"Volc CreateAsset failed: InvalidParameter.AspectRatioTooSmall: Aspect ratio must bebetween 0.4 and 2.5. status code: 400, requestid: 202608140002222F71AD72947FB5C3D631",
	))

	require.Contains(t, message, "素材宽高比不符合要求")
	require.Contains(t, message, "错误码：InvalidParameter.AspectRatioTooSmall")
	require.Contains(t, message, "HTTP 状态码：400")
	require.Contains(t, message, "请求 ID：202608140002222F71AD72947FB5C3D631")
}

func TestLocalizeVolcAssetErrorCommonCodes(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{code: "InvalidParameter.FileSizeTooLarge", want: "素材文件过大"},
		{code: "InvalidParameter.UnsupportedFormat", want: "素材格式不受支持"},
		{code: "AssetGroupNotFound", want: "目标素材组不存在"},
		{code: "QuotaExceeded", want: "配额已用尽"},
		{code: "RequestLimitExceeded", want: "请求过于频繁"},
		{code: "InvalidCredential", want: "鉴权失败"},
		{code: "AIGCNotAvailable", want: "无权使用该素材能力"},
		{code: "InternalError", want: "暂时不可用"},
	}

	for _, test := range tests {
		t.Run(test.code, func(t *testing.T) {
			message := LocalizeVolcAssetErrorDetails(test.code, "upstream message", "", 0)
			require.Contains(t, message, test.want)
			require.Contains(t, message, "错误码："+test.code)
		})
	}
}

func TestLocalizeVolcAssetErrorUnknownKeepsSafeDetails(t *testing.T) {
	message := LocalizeVolcAssetError(errors.New("opaque upstream failure"))
	require.Equal(t, "火山素材服务请求失败：opaque upstream failure", message)
}
