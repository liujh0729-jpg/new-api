package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

const seedanceMaintenanceInterval = 15 * time.Second

// StartSeedanceMaintenanceTasks keeps every network-bound Seedance side effect
// outside TaskPollingLoop. A slow billing system, customer callback, or cloud
// cost API must never delay polling the generation task itself.
func StartSeedanceMaintenanceTasks() {
	if !common.IsMasterNode || !constant.UpdateTask {
		return
	}
	startSeedanceMaintenanceLoop(func(ctx context.Context) {
		if _, err := model.RetireUnusedSeedanceCredentials(20); err != nil {
			logger.LogError(ctx, "retire unused Seedance credentials: "+err.Error())
		}
		ProcessSeedanceCustomerRefunds(ctx, 20)
		ProcessSeedanceCostRevisionQueue(ctx, 20)
		ProcessSeedanceBillingOutbox(ctx, 20)
	})
	startSeedanceMaintenanceLoop(func(ctx context.Context) {
		ProcessSeedanceCallbacks(ctx, 20)
	})
	startSeedanceMaintenanceLoop(func(ctx context.Context) {
		ProcessSeedanceVolcengineBills(ctx, 10)
	})
}

func startSeedanceMaintenanceLoop(run func(context.Context)) {
	go func() {
		for {
			run(context.Background())
			time.Sleep(seedanceMaintenanceInterval)
		}
	}()
}
