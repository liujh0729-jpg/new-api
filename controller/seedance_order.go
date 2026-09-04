package controller

import (
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func prepareTaskAndWorkflowOrder(info *relaycommon.RelayInfo, taskAction string) (*model.Task, error) {
	if info == nil {
		return nil, fmt.Errorf("task submission context is required")
	}
	seedancePlatform := constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeSeedance))
	task := model.InitTask(seedancePlatform, info)
	task.PrivateData.LogRequestID = strings.TrimSpace(info.RequestId)
	task.PrivateData.BillingSource = info.BillingSource
	task.PrivateData.SubscriptionId = info.SubscriptionId
	task.PrivateData.SubscriptionPreConsumed = info.SubscriptionPreConsumed
	task.PrivateData.TokenId = info.TokenId
	task.PrivateData.BillingContext = &model.TaskBillingContext{
		ModelPrice:      info.PriceData.ModelPrice,
		GroupRatio:      info.PriceData.GroupRatioInfo.GroupRatio,
		ModelRatio:      info.PriceData.ModelRatio,
		OtherRatios:     info.PriceData.OtherRatios,
		OriginModelName: info.OriginModelName,
		PerCallBilling:  isTaskPerCallBilling(info),
		QuotaPerUnit:    common.QuotaPerUnit,
		USDExchangeRate: operation_setting.USDExchangeRate,
	}
	if quote := info.TaskPricingQuote; quote != nil {
		task.PrivateData.BillingContext.PerCallBilling = false
		task.PrivateData.BillingContext.GroupRatio = quote.GroupRatio
		task.PrivateData.BillingContext.BillingMode = billing_setting.BillingModeTaskPricing
		task.PrivateData.BillingContext.BillingUnit = quote.Unit
		task.PrivateData.BillingContext.PricingVariant = quote.Variant
		task.PrivateData.BillingContext.UnitPriceUSD = quote.UnitPriceUSD
		task.PrivateData.BillingContext.Quantity = quote.Quantity
		task.PrivateData.BillingContext.SaleUSD = quote.SaleUSD
		task.PrivateData.BillingContext.HasReferenceVideo = quote.HasReferenceVideo
		task.PrivateData.BillingContext.Resolution = quote.Resolution
	}
	task.PrivateData.AIPDDFinance = info.AIPDDFinance
	task.Quota = info.PriceData.Quota
	task.Action = taskAction
	if strings.TrimSpace(task.Action) == "" {
		task.Action = info.Action
	}

	config, err := model.GetSeedanceChannelConfig(task.ChannelId)
	if err != nil {
		return nil, fmt.Errorf("load Seedance channel config: %w", err)
	}
	credential, err := model.GetActiveSeedanceVolcengineCredential(task.ChannelId)
	if err != nil {
		return nil, fmt.Errorf("load Seedance credential: %w", err)
	}
	offering, err := model.GetPublishedSeedanceOffering(task.ChannelId, task.Properties.OriginModelName)
	if err != nil {
		return nil, fmt.Errorf("load Seedance offering: %w", err)
	}
	var baseModel *model.SeedanceBaseModel
	if offering.BaseModelID > 0 {
		baseModel, err = model.GetSeedanceBaseModelForExecution(offering.BaseModelID)
		if err != nil {
			return nil, fmt.Errorf("load Seedance base model: %w", err)
		}
	}
	var provider *model.MediaEnhancementProvider
	var enhancementModel *model.SeedanceEnhancementModel
	if offering.EnhancementModelID != nil {
		enhancementModel, err = model.GetSeedanceEnhancementModelForExecution(*offering.EnhancementModelID)
		if err != nil {
			return nil, fmt.Errorf("load Seedance enhancement model: %w", err)
		}
		provider, err = model.GetMediaEnhancementProvider(enhancementModel.ProviderID)
		if err != nil {
			return nil, fmt.Errorf("load Seedance provider: %w", err)
		}
	} else if offering.EnhancementProviderID > 0 {
		// Compatibility for an offering accepted before the three-layer catalog
		// migration. New and migrated rows always use enhancement_model_id.
		provider, err = model.GetMediaEnhancementProvider(offering.EnhancementProviderID)
		if err != nil {
			return nil, fmt.Errorf("load legacy Seedance provider: %w", err)
		}
	}
	effectiveOffering := *offering
	groupRatio := 1.0
	hasReferenceVideo := false
	durationSeconds := 5.0
	if info.TaskPricingQuote != nil {
		groupRatio = info.TaskPricingQuote.GroupRatio
		hasReferenceVideo = info.TaskPricingQuote.HasReferenceVideo
		durationSeconds = info.TaskPricingQuote.Quantity
	}
	if durationSeconds <= 0 || math.IsNaN(durationSeconds) || math.IsInf(durationSeconds, 0) || durationSeconds > float64(math.MaxInt64)/1000 {
		return nil, fmt.Errorf("invalid Seedance requested duration")
	}
	requestedDurationMillis := int64(math.Round(durationSeconds * 1000))
	saleUnitPrice := offering.NoReferenceUnitPriceMicroRMB
	if hasReferenceVideo {
		saleUnitPrice = offering.ReferenceUnitPriceMicroRMB
	}
	if saleUnitPrice == 0 && offering.ModelSaleMicroRMB > 0 {
		saleUnitPrice = offering.ModelSaleMicroRMB
	}
	baseUnitCost := offering.VolcengineUnitCostMicroRMB
	if baseModel != nil {
		baseUnitCost, err = model.ResolveSeedanceBaseUnitCost(baseModel, offering.SourceResolution, hasReferenceVideo)
		if err != nil {
			return nil, err
		}
	}
	superResolutionUnitCost := offering.ServiceChargeMicroRMB
	if enhancementModel != nil {
		superResolutionUnitCost, err = model.ResolveSeedanceEnhancementUnitCost(enhancementModel, offering.TargetResolution, offering.OutputFPS)
		if err != nil {
			return nil, err
		}
	}
	baseSaleTotal, err := model.CalculateSeedanceTimedAmount(saleUnitPrice, requestedDurationMillis)
	if err != nil {
		return nil, err
	}
	effectiveOffering.ModelSaleMicroRMB, err = discountedSeedanceSaleMicroRMB(baseSaleTotal, groupRatio)
	if err != nil {
		return nil, err
	}
	effectiveOffering.VolcengineUnitCostMicroRMB, err = model.CalculateSeedanceTimedAmount(baseUnitCost, requestedDurationMillis)
	if err != nil {
		return nil, err
	}
	effectiveOffering.ServiceChargeMicroRMB, err = model.CalculateSeedanceTimedAmount(superResolutionUnitCost, requestedDurationMillis)
	if err != nil {
		return nil, err
	}
	if effectiveOffering.ProviderCostMicroRMB != nil {
		providerCostTotal, costErr := model.CalculateSeedanceTimedAmount(*effectiveOffering.ProviderCostMicroRMB, requestedDurationMillis)
		if costErr != nil {
			return nil, costErr
		}
		effectiveOffering.ProviderCostMicroRMB = &providerCostTotal
	}

	requestFacts, err := common.Marshal(map[string]any{
		"model":              task.Properties.OriginModelName,
		"upstream_model":     task.Properties.UpstreamModelName,
		"action":             task.Action,
		"task_pricing_facts": info.TaskPricingFacts,
	})
	if err != nil {
		return nil, err
	}
	// Task records are exposed by the ordinary user task log. Keep the actual
	// Ark model only in the private request evidence and store the public model
	// name in the generic task projection.
	task.Properties.UpstreamModelName = task.Properties.OriginModelName
	task.SetData(map[string]any{
		"id":     task.TaskID,
		"model":  task.Properties.OriginModelName,
		"status": "queued",
	})
	pricingSnapshot, err := common.Marshal(map[string]any{
		"pricing_version":                      offering.PricingVersion,
		"model_sale_micro_rmb":                 effectiveOffering.ModelSaleMicroRMB,
		"sale_unit_price_micro_rmb":            saleUnitPrice,
		"group_ratio":                          groupRatio,
		"service_charge_micro_rmb":             effectiveOffering.ServiceChargeMicroRMB,
		"super_resolution_unit_cost_micro_rmb": superResolutionUnitCost,
		"volcengine_unit_cost_micro_rmb":       effectiveOffering.VolcengineUnitCostMicroRMB,
		"base_unit_cost_micro_rmb":             baseUnitCost,
		"requested_duration_millis":            requestedDurationMillis,
		"has_reference_video":                  hasReferenceVideo,
		"base_model_id":                        offering.BaseModelID,
		"enhancement_model_id":                 offering.EnhancementModelID,
		"source_resolution":                    offering.SourceResolution,
		"target_resolution":                    offering.TargetResolution,
		"output_fps":                           offering.OutputFPS,
		"enhancement_required":                 provider != nil,
		"service_code":                         offering.EnhancementServiceCode,
		"specification":                        offering.EnhancementSpecificationJSON,
		"specification_version":                offering.EnhancementSpecificationVersion,
		"provider_cost_micro_rmb":              effectiveOffering.ProviderCostMicroRMB,
		"quota_per_unit":                       common.QuotaPerUnit,
		"usd_exchange_rate":                    task.PrivateData.BillingContext.USDExchangeRate,
	})
	if err != nil {
		return nil, err
	}
	if provider != nil {
		var snapshot map[string]any
		if err := common.Unmarshal(pricingSnapshot, &snapshot); err != nil {
			return nil, err
		}
		snapshot["provider_type"] = provider.ProviderType
		snapshot["adapter_type"] = providerAdapterType(provider)
		snapshot["provider_id"] = provider.ID
		pricingSnapshot, err = common.Marshal(snapshot)
		if err != nil {
			return nil, err
		}
	}

	protocol := model.SeedanceProtocolOfficial
	if strings.HasPrefix(strings.TrimSpace(info.RequestURLPath), "/v1/videos") {
		protocol = model.SeedanceProtocolOpenAI
	}
	_, err = model.InsertTaskWithSeedanceOrder(model.SeedanceOrderCreate{
		Task:                            task,
		Config:                          config,
		Credential:                      credential,
		Offering:                        &effectiveOffering,
		Provider:                        provider,
		HasReferenceVideo:               hasReferenceVideo,
		RequestedDurationMillis:         requestedDurationMillis,
		SaleUnitPriceMicroRMB:           saleUnitPrice,
		SuperResolutionUnitCostMicroRMB: superResolutionUnitCost,
		RequestFactsJSON:                string(requestFacts),
		PricingSnapshot:                 string(pricingSnapshot),
		GenerationTaskID:                "",
		PublicProtocol:                  protocol,
		CallbackURL:                     info.SeedanceCallbackURL,
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

func discountedSeedanceSaleMicroRMB(baseMicroRMB int64, groupRatio float64) (int64, error) {
	discountedSale := float64(baseMicroRMB) * groupRatio
	if math.IsNaN(discountedSale) || math.IsInf(discountedSale, 0) || discountedSale < 0 || discountedSale > math.MaxInt64 {
		return 0, fmt.Errorf("invalid Seedance quoted model sale")
	}
	return int64(math.Round(discountedSale)), nil
}
