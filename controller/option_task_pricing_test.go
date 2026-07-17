package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestBuildTaskPricingRequiredModelsValueUsesDurationCapabilityIdentity(t *testing.T) {
	original := constant.GetAIPDDCapabilities()
	t.Cleanup(func() {
		constant.SetAIPDDCapabilities(original)
		model.InvalidatePricingCache()
	})

	constant.SetAIPDDCapabilities([]constant.AIPDDCapability{
		{ModelName: "seedance-by-adapter", AdapterCode: "seedance"},
		{ModelName: "seedance-by-protocol", ExecutionProtocol: "seedance_official"},
		{ModelName: "duration-workflow", BillingType: constant.AIPDDBillingTypeDurationSeconds},
		{ModelName: "ordinary-aipdd-task", AdapterCode: "workflow", ExecutionProtocol: "shared_task"},
		{ModelName: "seedance-by-adapter", AdapterCode: "SEEDANCE"},
		{ModelName: "  ", AdapterCode: "seedance"},
	})
	model.InvalidatePricingCache()

	var modelNames []string
	require.NoError(t, common.UnmarshalJsonStr(buildTaskPricingRequiredModelsValue(), &modelNames))
	require.Equal(t, []string{"duration-workflow", "seedance-by-adapter", "seedance-by-protocol"}, modelNames)
	require.NotContains(t, modelNames, "ordinary-aipdd-task")
}
