package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
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
	pollVirtualCharacterImageActivation(now)
	refreshRealPersonProviderStates(now)
	expireVirtualCharacterValidationSessions(now)
	cleanupVirtualCharacterJobs(now)
	if err := model.DeleteExpiredVirtualCharacterTaskLinks(now.Add(-virtualCharacterTaskRetention).Unix()); err != nil {
		common.SysError("delete expired virtual character task links: " + err.Error())
	}
	if err := model.DeleteExpiredVirtualCharacterTaskReferences(now.Add(-virtualCharacterTaskRetention).Unix()); err != nil {
		common.SysError("delete expired virtual character task references: " + err.Error())
	}
}

func pollVirtualCharacterImageActivation(now time.Time) {
	characters, err := model.ListVirtualCharactersToPoll(now.Unix(), virtualCharacterMaintenanceBatch)
	if err != nil {
		common.SysError("list virtual characters to poll: " + err.Error())
		return
	}
	account, err := model.GetEnabledVirtualCharacterProviderAccount()
	if err != nil {
		for i := range characters {
			retryVirtualCharacterImagePoll(&characters[i], now, errors.New("provider account is unavailable"))
		}
		return
	}
	client, err := NewVolcAssetClient(account)
	if err != nil {
		for i := range characters {
			retryVirtualCharacterImagePoll(&characters[i], now, err)
		}
		return
	}
	for i := range characters {
		character := &characters[i]
		if character.SourceType == model.VirtualCharacterSourceVolcRealPerson {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			_, callErr := SyncRealPersonVirtualCharacter(ctx, character.ID)
			cancel()
			if callErr != nil {
				retryVirtualCharacterImagePoll(character, now, callErr)
			}
			continue
		}
		if character.ProviderAccountID != account.ID {
			retryVirtualCharacterImagePoll(character, now, errors.New("asset provider account is not enabled"))
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		result, callErr := client.GetAsset(ctx, character.ProviderAssetID, account.ProjectName)
		cancel()
		if callErr != nil {
			retryVirtualCharacterImagePoll(character, now, callErr)
			continue
		}
		switch strings.ToLower(strings.TrimSpace(result.Status)) {
		case "processing", "creating", "pending":
			attempts := character.AssetPollAttempts + 1
			_ = model.RetryVirtualCharacterImagePoll(character.ID, attempts, now.Add(virtualCharacterRetryDelay(attempts)).Unix(), "")
		case "active":
			finishVirtualCharacterImagePoll(character, true, "")
		case "failed", "error":
			reason := strings.TrimSpace(result.Error)
			if reason == "" {
				reason = "Volc asset processing failed"
			}
			finishVirtualCharacterImagePoll(character, false, LocalizeVolcAssetErrorDetails(result.ErrorCode, reason, "", 0))
		default:
			retryVirtualCharacterImagePoll(character, now, fmt.Errorf("unexpected Volc asset status %q", result.Status))
		}
	}
}

func refreshRealPersonProviderStates(now time.Time) {
	items, err := model.ListRealPersonVirtualCharactersDueForProviderCheck(now.Unix(), now.Add(-realPersonProviderStateTTL).Unix(), virtualCharacterMaintenanceBatch)
	if err != nil {
		common.SysError("list real-person characters for provider check: " + err.Error())
		return
	}
	account, accountErr := model.GetEnabledVirtualCharacterProviderAccount()
	for i := range items {
		character := &items[i]
		authorization, err := model.GetVirtualCharacterAuthorization(character.ID)
		if err != nil {
			continue
		}
		if authorization.ValidUntil > 0 && authorization.ValidUntil <= now.Unix() {
			if err := model.ExpireRealPersonAuthorization(character.ID, "authorization expired"); err != nil {
				common.SysError(fmt.Sprintf("expire real-person authorization %d: %v", character.ID, err))
			}
			continue
		}
		if accountErr != nil || account.ID != character.ProviderAccountID {
			_ = model.BlockRealPersonVirtualCharacter(character.ID, model.VirtualCharacterAuthorizationProviderUnavailable, "provider account is unavailable")
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		if strings.TrimSpace(character.ProviderAssetID) == "" {
			_, err = SyncRealPersonVirtualCharacter(ctx, character.ID)
		} else {
			_, err = RefreshRealPersonVirtualCharacterState(ctx, character, account)
		}
		cancel()
		if err != nil {
			common.SysError(fmt.Sprintf("refresh real-person provider state %d: %v", character.ID, err))
		}
	}
}

func expireVirtualCharacterValidationSessions(now time.Time) {
	items, err := model.ListExpiredPendingVirtualCharacterValidationSessions(now.Unix(), virtualCharacterMaintenanceBatch)
	if err != nil {
		common.SysError("list expired real-person validation sessions: " + err.Error())
		return
	}
	for i := range items {
		if err := model.FailReservedVirtualCharacterValidation(items[i].ID, model.VirtualCharacterValidationExpired, "expired", "validation session expired"); err != nil {
			common.SysError("expire real-person validation session: " + err.Error())
		}
	}
}

func finishVirtualCharacterImagePoll(character *model.VirtualCharacter, active bool, reason string) {
	if !active {
		if err := model.DiscardFailedAIGCVirtualCharacterUpload(character.ID, character.ProviderGroupID, reason); err != nil {
			common.SysError(fmt.Sprintf("discard failed virtual character upload %d: %v", character.ID, err))
		}
		return
	}
	if err := model.MarkVirtualCharacterImageTerminal(character.ID, true, ""); err != nil {
		common.SysError(fmt.Sprintf("mark virtual character %d terminal: %v", character.ID, err))
	}
}

func retryVirtualCharacterImagePoll(character *model.VirtualCharacter, now time.Time, err error) {
	attempts := character.AssetPollAttempts + 1
	if updateErr := model.RetryVirtualCharacterImagePoll(character.ID, attempts, now.Add(virtualCharacterRetryDelay(attempts)).Unix(), common.MaskSensitiveInfo(err.Error())); updateErr != nil {
		common.SysError(fmt.Sprintf("retry virtual character asset poll %d: %v", character.ID, updateErr))
	}
}

func cleanupVirtualCharacterJobs(now time.Time) {
	jobs, err := model.ListVirtualCharacterCleanupJobs(now.Unix(), virtualCharacterMaintenanceBatch)
	if err != nil {
		common.SysError("list virtual character cleanup jobs: " + err.Error())
		return
	}
	for i := range jobs {
		job := &jobs[i]
		if err := executeVirtualCharacterCleanupJob(job); err != nil {
			attempts := job.Attempts + 1
			_ = model.RetryVirtualCharacterCleanupJob(job.ID, attempts, now.Add(virtualCharacterRetryDelay(attempts)).Unix(), common.MaskSensitiveInfo(err.Error()))
			continue
		}
		if err := model.CompleteVirtualCharacterCleanupJob(job.ID); err != nil {
			common.SysError(fmt.Sprintf("complete virtual character cleanup job %d: %v", job.ID, err))
			continue
		}
		if err := finalizeDeletedVirtualCharacterIfReady(job.CharacterID); err != nil {
			common.SysError(fmt.Sprintf("finalize virtual character cleanup %d: %v", job.CharacterID, err))
		}
	}
}

func executeVirtualCharacterCleanupJob(job *model.VirtualCharacterCleanupJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	switch job.TargetType {
	case "aipdd_asset":
		storage, err := NewAIPDDVirtualCharacterStorage()
		if err != nil {
			return err
		}
		id, err := strconv.ParseInt(job.TargetID, 10, 64)
		if err != nil {
			return errors.New("invalid AIPDD asset cleanup id")
		}
		return storage.DeleteDigitalAsset(ctx, id)
	case "aipdd_file":
		storage, err := NewAIPDDVirtualCharacterStorage()
		if err != nil {
			return err
		}
		if err := storage.DeleteFile(ctx, job.TargetID); err != nil {
			return err
		}
		if job.CharacterID > 0 {
			return model.DB.Model(&model.VirtualCharacter{}).
				Where("id = ? AND staging_file_id = ?", job.CharacterID, job.TargetID).
				Updates(map[string]any{"staging_file_id": "", "cover_url": ""}).Error
		}
		return nil
	case "volc_asset", "volc_group":
		account, err := model.GetEnabledVirtualCharacterProviderAccount()
		if err != nil || account.ID != job.ProviderAccountID {
			return errors.New("cleanup provider account is unavailable")
		}
		client, err := NewVolcAssetClient(account)
		if err != nil {
			return err
		}
		if job.TargetType == "volc_asset" {
			err = client.DeleteAsset(ctx, job.TargetID, account.ProjectName)
			if isVolcNotFoundError(err) {
				err = nil
			}
			if err == nil && job.CharacterID > 0 {
				err = model.DB.Model(&model.VirtualCharacter{}).
					Where("id = ? AND provider_asset_id = ?", job.CharacterID, job.TargetID).
					Update("provider_asset_id", "").Error
			}
			return err
		}
		pending, err := model.HasIncompleteVirtualCharacterCleanupJobs(job.CharacterID, job.ID)
		if err != nil {
			return err
		}
		if pending {
			return errors.New("character cleanup dependencies are still pending")
		}
		err = client.DeleteAssetGroup(ctx, job.TargetID, account.ProjectName)
		if isVolcNotFoundError(err) {
			err = nil
		}
		if err == nil && job.CharacterID > 0 {
			err = model.DB.Model(&model.VirtualCharacter{}).
				Where("id = ? AND provider_group_id = ?", job.CharacterID, job.TargetID).
				Update("provider_group_id", "").Error
		}
		return err
	default:
		return fmt.Errorf("unsupported cleanup target type %s", job.TargetType)
	}
}

func finalizeDeletedVirtualCharacterIfReady(characterID int64) error {
	if characterID <= 0 {
		return nil
	}
	pending, err := model.HasIncompleteVirtualCharacterCleanupJobs(characterID, 0)
	if err != nil || pending {
		return err
	}
	return model.DB.Unscoped().Where("id = ? AND status = ?", characterID, model.VirtualCharacterStatusDeleting).
		Delete(&model.VirtualCharacter{}).Error
}

func isVolcNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(err.Error())
	return strings.Contains(value, "notfound") || strings.Contains(value, "not found") || strings.Contains(value, "does not exist")
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
		character, characterErr := model.GetVirtualCharacterByID(item.CharacterID)
		if task.Status == model.TaskStatusFailure && characterErr == nil &&
			character.SourceType != model.VirtualCharacterSourceVolcRealPerson &&
			character.SourceType != model.VirtualCharacterSourceVolcAIGC &&
			IsVirtualCharacterRealPersonRejection(task.FailReason) {
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
		if err := enqueueVirtualCharacterProviderCleanup(item, now); err != nil {
			retryVirtualCharacterCleanup(item, now, err)
			continue
		}
		if err := finalizeDeletedVirtualCharacterIfReady(item.ID); err != nil {
			retryVirtualCharacterCleanup(item, now, err)
			continue
		}
		_ = model.RetryVirtualCharacterCleanup(item.ID, item.CleanupAttempts, now.Add(time.Hour).Unix(), item.LastError)
	}
}

func enqueueVirtualCharacterProviderCleanup(item *model.VirtualCharacter, now time.Time) error {
	if item == nil {
		return nil
	}
	if strings.TrimSpace(item.ProviderAssetID) != "" {
		if err := model.CreateVirtualCharacterCleanupJob(&model.VirtualCharacterCleanupJob{
			CharacterID: item.ID, ProviderAccountID: item.ProviderAccountID,
			TargetType: "volc_asset", TargetID: item.ProviderAssetID, Status: model.VirtualCharacterCleanupPending, NextAttemptAt: now.Unix(),
		}); err != nil {
			return err
		}
	}
	if strings.TrimSpace(item.StagingFileID) != "" {
		if err := model.CreateVirtualCharacterCleanupJob(&model.VirtualCharacterCleanupJob{
			CharacterID: item.ID, ProviderAccountID: item.ProviderAccountID,
			TargetType: "aipdd_file", TargetID: item.StagingFileID, Status: model.VirtualCharacterCleanupPending, NextAttemptAt: now.Unix(),
		}); err != nil {
			return err
		}
	}
	if strings.TrimSpace(item.ProviderGroupID) != "" {
		if err := model.CreateVirtualCharacterCleanupJob(&model.VirtualCharacterCleanupJob{
			CharacterID: item.ID, ProviderAccountID: item.ProviderAccountID,
			TargetType: "volc_group", TargetID: item.ProviderGroupID, Status: model.VirtualCharacterCleanupPending, NextAttemptAt: now.Unix(),
		}); err != nil {
			return err
		}
	}
	return nil
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
