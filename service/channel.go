package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldDisableChannel(err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	return WouldDisableChannel(err)
}

// WouldDisableChannel reports whether an error is severe enough that a channel would be
// auto-disabled if disabling were on. The global switch is deliberately not consulted, so
// deployments that keep auto-disabling off still learn that a channel is broken.
func WouldDisableChannel(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

var channelFailureAlerts sync.Map

const channelFailureAlertWindow = 10 * time.Minute

// shouldAlertChannel keeps one broken channel from turning the alert into per-request
// spam; the underlying notification limiter rejects rather than defers.
func shouldAlertChannel(channelId int) bool {
	last, ok := channelFailureAlerts.Load(channelId)
	if ok && time.Since(last.(time.Time)) < channelFailureAlertWindow {
		return false
	}
	channelFailureAlerts.Store(channelId, time.Now())
	return true
}

// AlertChannelPersistentFailure tells the root user that a channel keeps failing badly
// enough to be disabled while nothing is disabling it. Without this, turning auto-disable
// off removes the only alert path, and a broken upstream fails silently.
func AlertChannelPersistentFailure(channelError types.ChannelError, reason string) {
	if !shouldAlertChannel(channelError.ChannelId) {
		return
	}
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）错误已达禁用标准，但未自动禁用，原因：%s",
		channelError.ChannelName, channelError.ChannelId, reason))

	subject := fmt.Sprintf("通道「%s」（#%d）持续报错（未自动禁用）", channelError.ChannelName, channelError.ChannelId)
	content := fmt.Sprintf("通道「%s」（#%d）的错误已达到自动禁用标准，但本部署未自动禁用它，请人工确认上游是否可用。原因：%s",
		channelError.ChannelName, channelError.ChannelId, reason)
	NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}
