package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	aipddCatalogSyncIntervalMinutesEnv = "AIPDD_CATALOG_SYNC_INTERVAL_MINUTES"
	aipddCatalogSyncTimeoutSecondsEnv  = "AIPDD_CATALOG_SYNC_TIMEOUT_SECONDS"
	defaultAIPDDCatalogSyncMinutes     = 5
	defaultAIPDDCatalogSyncTimeout     = 10
)

// StartAIPDDCatalogSyncTask periodically reconciles the managed NewAPI channel
// with AIPDD's revisioned atomic catalog. A non-positive interval disables it.
func StartAIPDDCatalogSyncTask() {
	if !common.IsMasterNode {
		return
	}
	minutes := common.GetEnvOrDefault(aipddCatalogSyncIntervalMinutesEnv, defaultAIPDDCatalogSyncMinutes)
	if minutes <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Duration(minutes) * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			// Checked every tick rather than once here: the key is usually saved
			// after boot through the setup wizard or the channel editor, and a
			// start-time check would leave those installs never syncing.
			if !model.IsAIPDDCatalogKeyConfigured() {
				continue
			}
			runAIPDDCatalogSync()
		}
	}()
}

func runAIPDDCatalogSync() {
	timeoutSeconds := common.GetEnvOrDefault(aipddCatalogSyncTimeoutSecondsEnv, defaultAIPDDCatalogSyncTimeout)
	if timeoutSeconds <= 0 {
		timeoutSeconds = defaultAIPDDCatalogSyncTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	result, err := model.SyncAIPDDCatalogFromStoredKey(ctx)
	if err != nil {
		common.SysError("periodic AIPDD catalog sync failed: " + err.Error())
		return
	}
	if result.AddedModels > 0 || result.RemovedModels > 0 || result.UpdatedPrices > 0 {
		common.SysLog("periodic AIPDD catalog sync applied revision=" + result.Revision)
	}
}
