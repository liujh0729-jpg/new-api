package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	virtualCharacterMaintenanceInterval = 15 * time.Second
	virtualCharacterTaskRetention       = 90 * 24 * time.Hour
	virtualCharacterSubmissionTimeout   = time.Hour
	virtualCharacterMaintenanceBatch    = 50
)

func StartVirtualCharacterMaintenanceTask() {
	if !common.IsMasterNode {
		return
	}
	go func() {
		runVirtualCharacterMaintenance()
		ticker := time.NewTicker(virtualCharacterMaintenanceInterval)
		defer ticker.Stop()
		for range ticker.C {
			runVirtualCharacterMaintenance()
		}
	}()
}

func runVirtualCharacterMaintenance() {
	now := time.Now()
	if err := model.FailStaleVirtualCharacterTaskLinks(
		now.Add(-virtualCharacterSubmissionTimeout).Unix(),
		"task submission did not complete before the recovery timeout",
	); err != nil {
		common.SysError("expire stale virtual character task submissions: " + err.Error())
	}
	recoverVirtualCharacterTasks(now)
	checkVirtualCharacterTaskTerminals()
	if err := model.MarkOrphanedVirtualCharactersForCleanup(now.Unix()); err != nil {
		common.SysError("mark orphaned virtual characters for cleanup: " + err.Error())
	}
	cleanupVirtualCharacters(now)
	if err := model.DeleteExpiredVirtualCharacterTaskLinks(now.Add(-virtualCharacterTaskRetention).Unix()); err != nil {
		common.SysError("delete expired virtual character task links: " + err.Error())
	}
}

func recoverVirtualCharacterTasks(now time.Time) {
	items, err := model.ListVirtualCharacterTasksReady(now.Unix(), virtualCharacterMaintenanceBatch)
	if err != nil {
		common.SysError("list recoverable virtual character tasks: " + err.Error())
		return
	}
	for i := range items {
		item := &items[i]
		if err := model.RecoverVirtualCharacterTask(item); err != nil {
			attempts := item.RetryCount + 1
			next := now.Add(virtualCharacterRetryDelay(attempts)).Unix()
			_ = model.RetryVirtualCharacterTask(item.TaskID, attempts, next, err.Error())
		}
	}
}

func checkVirtualCharacterTaskTerminals() {
	items, err := model.ListUncheckedTerminalVirtualCharacterTasks(virtualCharacterMaintenanceBatch)
	if err != nil {
		common.SysError("list unchecked virtual character tasks: " + err.Error())
		return
	}
	for i := range items {
		item := &items[i]
		task, exists, err := model.GetByTaskId(item.UserID, item.TaskID)
		if err != nil || !exists {
			continue
		}
		if task.Status == model.TaskStatusFailure && IsVirtualCharacterRealPersonRejection(task.FailReason) {
			if err := model.MarkVirtualCharacterBlocked(item.CharacterID, task.FailReason); err != nil {
				common.SysError(fmt.Sprintf("block rejected virtual character %d: %v", item.CharacterID, err))
				continue
			}
		}
		if err := model.MarkVirtualCharacterTaskTerminalChecked(item.TaskID); err != nil {
			common.SysError("mark virtual character task checked: " + err.Error())
		}
	}
}

func cleanupVirtualCharacters(now time.Time) {
	items, err := model.ListVirtualCharactersPendingCleanup(now.Unix(), virtualCharacterMaintenanceBatch)
	if err != nil {
		common.SysError("list virtual characters pending cleanup: " + err.Error())
		return
	}
	for i := range items {
		item := &items[i]
		unfinished, err := model.HasUnfinishedVirtualCharacterTasks(item.ID)
		if err != nil {
			retryVirtualCharacterCleanup(item, now, err)
			continue
		}
		if unfinished {
			_ = model.RetryVirtualCharacterCleanup(item.ID, item.CleanupAttempts, now.Add(time.Minute).Unix(), item.LastError)
			continue
		}
		if item.AIPDDAssetID <= 0 && strings.TrimSpace(item.AIPDDFileID) == "" {
			if err := model.CompleteVirtualCharacterCleanup(item.ID); err != nil {
				retryVirtualCharacterCleanup(item, now, err)
			}
			continue
		}
		storage, err := NewAIPDDVirtualCharacterStorage()
		if err != nil {
			retryVirtualCharacterCleanup(item, now, err)
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err = storage.DeleteDigitalAsset(ctx, item.AIPDDAssetID)
		if err == nil {
			err = storage.DeleteFile(ctx, item.AIPDDFileID)
		}
		cancel()
		if err != nil {
			retryVirtualCharacterCleanup(item, now, err)
			continue
		}
		if err := model.CompleteVirtualCharacterCleanup(item.ID); err != nil {
			retryVirtualCharacterCleanup(item, now, err)
		}
	}
}

func retryVirtualCharacterCleanup(item *model.VirtualCharacter, now time.Time, err error) {
	attempts := item.CleanupAttempts + 1
	next := now.Add(virtualCharacterRetryDelay(attempts)).Unix()
	if updateErr := model.RetryVirtualCharacterCleanup(item.ID, attempts, next, err.Error()); updateErr != nil {
		common.SysError(fmt.Sprintf("retry virtual character cleanup %d: %v", item.ID, updateErr))
	}
}

func virtualCharacterRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 8 {
		attempt = 8
	}
	delay := time.Duration(1<<uint(attempt-1)) * 15 * time.Second
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func IsVirtualCharacterRealPersonRejection(reason string) bool {
	normalized := strings.ToLower(strings.TrimSpace(reason))
	if normalized == "" {
		return false
	}
	patterns := []string{
		"real person reference",
		"real-person reference",
		"real human reference",
		"face verification required",
		"portrait verification required",
		"真人参考",
		"真人人脸",
		"真人肖像",
		"需要真人认证",
		"肖像认证",
		"人脸认证",
	}
	for _, pattern := range patterns {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return false
}
