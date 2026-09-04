package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	billingapi "github.com/volcengine/volcengine-go-sdk/service/billing"
	"github.com/volcengine/volcengine-go-sdk/volcengine"
	"github.com/volcengine/volcengine-go-sdk/volcengine/credentials"
	"github.com/volcengine/volcengine-go-sdk/volcengine/session"
)

const (
	seedanceBillSyncIntervalSeconds = int64(3600)
	seedanceBillPageLimit           = int32(100)
)

type seedanceVolcengineBillDetail struct {
	BillDetailID        string
	BillID              string
	BillPeriod          string
	ExpenseDate         string
	ExpenseBeginTime    string
	ExpenseEndTime      string
	Product             string
	ProductZh           string
	InstanceNo          string
	ChargeItemCode      string
	ConfigurationCode   string
	ElementCode         string
	BillingFunction     string
	BillingMethodCode   string
	RegionCode          string
	PayableAmount       string
	Currency            string
	SettlePayableAmount string
	CurrencySettlement  string
}

type seedanceVolcengineBillPage struct {
	Items  []seedanceVolcengineBillDetail
	Total  int32
	Offset int32
	Limit  int32
}

type seedanceVolcengineBillClient interface {
	ListBillDetails(ctx context.Context, billingPeriod string, productCodes []string, offset, limit int32) (*seedanceVolcengineBillPage, error)
}

type seedanceVolcengineSDKBillClient struct {
	client *billingapi.BILLING
}

var seedanceVolcengineBillClientFactory = func(credential *model.SeedanceVolcengineCredential) (seedanceVolcengineBillClient, error) {
	if credential == nil || credential.AccessKeyIDEncrypted == "" || credential.SecretAccessKeyEncrypted == "" {
		return nil, errors.New("active bill read-only AK/SK is not configured")
	}
	accessKey, err := common.DecryptSensitiveValue(credential.AccessKeyIDEncrypted)
	if err != nil {
		return nil, errors.New("decrypt bill access key failed")
	}
	secretKey, err := common.DecryptSensitiveValue(credential.SecretAccessKeyEncrypted)
	if err != nil {
		return nil, errors.New("decrypt bill secret key failed")
	}
	config := volcengine.NewConfig().
		WithCredentials(credentials.NewStaticCredentials(strings.TrimSpace(accessKey), strings.TrimSpace(secretKey), "")).
		WithRegion("cn-beijing").
		WithEndpoint("https://open.volcengineapi.com").
		WithHTTPClient(&http.Client{Timeout: 30 * time.Second}).
		WithMaxRetries(2)
	sess, err := session.NewSession(config)
	if err != nil {
		return nil, fmt.Errorf("create Volcengine billing session: %w", err)
	}
	return &seedanceVolcengineSDKBillClient{client: billingapi.New(sess)}, nil
}

func ValidateSeedanceVolcengineBillCredential(ctx context.Context, credential *model.SeedanceVolcengineCredential) error {
	client, err := seedanceVolcengineBillClientFactory(credential)
	if err != nil {
		return err
	}
	_, err = client.ListBillDetails(ctx, time.Now().Format("2006-01"), nil, 0, 1)
	if err != nil {
		return fmt.Errorf("Volcengine billing credential validation failed: %w", err)
	}
	return nil
}

func (c *seedanceVolcengineSDKBillClient) ListBillDetails(ctx context.Context, billingPeriod string, productCodes []string, offset, limit int32) (*seedanceVolcengineBillPage, error) {
	products := make([]*string, 0, len(productCodes))
	for _, product := range productCodes {
		value := strings.TrimSpace(product)
		if value != "" {
			products = append(products, volcengine.String(value))
		}
	}
	input := &billingapi.ListBillDetailInput{
		BillPeriod:    volcengine.String(billingPeriod),
		Limit:         volcengine.Int32(limit),
		Offset:        volcengine.Int32(offset),
		NeedRecordNum: volcengine.Int32(1),
		Product:       products,
	}
	output, err := c.client.ListBillDetailWithContext(ctx, input)
	if err != nil {
		return nil, err
	}
	page := &seedanceVolcengineBillPage{
		Total: volcengine.Int32Value(output.Total), Offset: volcengine.Int32Value(output.Offset), Limit: volcengine.Int32Value(output.Limit),
		Items: make([]seedanceVolcengineBillDetail, 0, len(output.List)),
	}
	for _, item := range output.List {
		if item == nil {
			continue
		}
		page.Items = append(page.Items, seedanceVolcengineBillDetail{
			BillDetailID: stringValue(item.BillDetailId), BillID: stringValue(item.BillID), BillPeriod: stringValue(item.BillPeriod),
			ExpenseDate: stringValue(item.ExpenseDate), ExpenseBeginTime: stringValue(item.ExpenseBeginTime), ExpenseEndTime: stringValue(item.ExpenseEndTime),
			Product: stringValue(item.Product), ProductZh: stringValue(item.ProductZh), InstanceNo: stringValue(item.InstanceNo),
			ChargeItemCode: stringValue(item.ChargeItemCode), ConfigurationCode: stringValue(item.ConfigurationCode), ElementCode: stringValue(item.ElementCode),
			BillingFunction: stringValue(item.BillingFunction), BillingMethodCode: stringValue(item.BillingMethodCode), RegionCode: stringValue(item.RegionCode),
			PayableAmount: stringValue(item.PayableAmount), Currency: stringValue(item.Currency),
			SettlePayableAmount: stringValue(item.SettlePayableAmount), CurrencySettlement: stringValue(item.CurrencySettlement),
		})
	}
	return page, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// ProcessSeedanceVolcengineBills periodically imports the current and previous
// billing periods. It intentionally stores unallocated rows in the restricted
// reconciliation queue until real-account evidence proves a safe order mapping.
func ProcessSeedanceVolcengineBills(ctx context.Context, maxChannels int) {
	query := model.DB.Where("status = ? AND volcengine_bill_sync_enabled = ?", model.SeedanceConfigActive, true).Order("channel_id asc")
	if maxChannels > 0 {
		query = query.Limit(maxChannels)
	}
	var configs []model.SeedanceChannelConfig
	if err := query.Find(&configs).Error; err != nil {
		common.SysError("Seedance bill sync config query failed: " + err.Error())
		return
	}
	now := time.Now()
	periods := []string{now.Format("2006-01"), now.AddDate(0, -1, 0).Format("2006-01")}
	for i := range configs {
		if err := syncSeedanceVolcengineChannelBills(ctx, &configs[i], periods); err != nil {
			common.SysError(fmt.Sprintf("Seedance bill sync failed for channel %d: %v", configs[i].ChannelID, err))
		}
	}
}

func syncSeedanceVolcengineChannelBills(ctx context.Context, config *model.SeedanceChannelConfig, periods []string) error {
	if config == nil {
		return errors.New("Seedance bill sync config is required")
	}
	products, err := seedanceBillProductCodes(config.VolcengineBillProductCodesJSON)
	if err != nil {
		return err
	}
	configurationCodes, err := seedanceBillConfigurationCodes(config.VolcengineBillConfigurationCodesJSON)
	if err != nil {
		return err
	}
	credential, err := model.GetActiveSeedanceVolcengineCredential(config.ChannelID)
	if err != nil {
		return err
	}
	if credential.BillingValidatedAt <= 0 {
		return errors.New("active bill read-only AK/SK has not passed ListBillDetail validation")
	}
	var client seedanceVolcengineBillClient
	for _, period := range periods {
		owner := "seedance-bill-" + uuid.NewString()
		cursor, claimed, err := model.ClaimSeedanceVolcengineBillCursor(config.ChannelID, period, seedanceBillSyncIntervalSeconds, owner)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		if client == nil {
			client, err = seedanceVolcengineBillClientFactory(credential)
			if err != nil {
				finishErr := model.FinishSeedanceVolcengineBillCursor(cursor.ID, owner, err)
				if finishErr != nil {
					return fmt.Errorf("%v; additionally failed to release cursor: %w", err, finishErr)
				}
				return err
			}
		}
		syncErr := syncSeedanceVolcengineBillPeriod(ctx, config.ChannelID, period, products, configurationCodes, cursor, owner, client)
		finishErr := model.FinishSeedanceVolcengineBillCursor(cursor.ID, owner, syncErr)
		if syncErr != nil {
			if finishErr != nil {
				return fmt.Errorf("%v; additionally failed to release cursor: %w", syncErr, finishErr)
			}
			return syncErr
		}
		if finishErr != nil {
			return finishErr
		}
	}
	return nil
}

func seedanceBillProductCodes(value string) ([]string, error) {
	return seedanceBillExactCodes(value, "product")
}

func seedanceBillConfigurationCodes(value string) ([]string, error) {
	return seedanceBillExactCodes(value, "configuration")
}

func seedanceBillExactCodes(value, kind string) ([]string, error) {
	var products []string
	if err := common.UnmarshalJsonStr(strings.TrimSpace(value), &products); err != nil {
		return nil, fmt.Errorf("volcengine bill %s codes must be a JSON string array", kind)
	}
	seen := make(map[string]struct{}, len(products))
	normalized := make([]string, 0, len(products))
	for _, product := range products {
		product = strings.TrimSpace(product)
		if product == "" {
			continue
		}
		if _, exists := seen[product]; exists {
			continue
		}
		seen[product] = struct{}{}
		normalized = append(normalized, product)
	}
	if len(normalized) == 0 {
		return nil, fmt.Errorf("at least one verified Volcengine bill %s code is required", kind)
	}
	return normalized, nil
}

func seedanceBillMatchesVerifiedSKU(detail seedanceVolcengineBillDetail, products, configurationCodes []string) bool {
	product := strings.TrimSpace(detail.Product)
	configurationCode := strings.TrimSpace(detail.ConfigurationCode)
	productAllowed := false
	for _, allowed := range products {
		if product == allowed {
			productAllowed = true
			break
		}
	}
	if !productAllowed {
		return false
	}
	for _, allowed := range configurationCodes {
		if configurationCode == allowed {
			return true
		}
	}
	return false
}

func syncSeedanceVolcengineBillPeriod(ctx context.Context, channelID int, period string, products, configurationCodes []string, cursor *model.SeedanceVolcengineBillCursor, owner string, client seedanceVolcengineBillClient) error {
	offset64, err := strconv.ParseInt(strings.TrimSpace(cursor.Cursor), 10, 32)
	if err != nil || offset64 < 0 {
		offset64 = 0
	}
	offset := int32(offset64)
	for pageNo := 0; pageNo < 10000; pageNo++ {
		page, err := client.ListBillDetails(ctx, period, products, offset, seedanceBillPageLimit)
		if err != nil {
			return err
		}
		for index, detail := range page.Items {
			if !seedanceBillMatchesVerifiedSKU(detail, products, configurationCodes) {
				continue
			}
			input, err := seedanceBillDetailImport(channelID, period, offset+int32(index), detail)
			if err != nil {
				return err
			}
			item, duplicate, err := model.ImportSeedanceVolcengineBillAuto(input, 0)
			if err != nil {
				return err
			}
			if !duplicate && item.AllocationStatus == model.SeedanceBillAllocated {
				if err := QueueSeedanceCostRevisionEvents(ctx, item.ID); err != nil {
					return err
				}
			}
		}
		next := offset + int32(len(page.Items))
		if err := model.SaveSeedanceVolcengineBillCursorProgress(cursor.ID, owner, next); err != nil {
			return err
		}
		if len(page.Items) == 0 || next >= page.Total {
			return nil
		}
		if next <= offset {
			return errors.New("Volcengine bill pagination did not advance")
		}
		offset = next
	}
	return errors.New("Volcengine bill pagination exceeded the safety limit")
}

func seedanceBillDetailImport(channelID int, requestedPeriod string, absoluteIndex int32, detail seedanceVolcengineBillDetail) (model.SeedanceBillImport, error) {
	period := strings.TrimSpace(detail.BillPeriod)
	if period == "" {
		period = requestedPeriod
	}
	amountText, currency := strings.TrimSpace(detail.PayableAmount), strings.ToUpper(strings.TrimSpace(detail.Currency))
	if strings.EqualFold(strings.TrimSpace(detail.CurrencySettlement), "CNY") && strings.TrimSpace(detail.SettlePayableAmount) != "" {
		amountText, currency = strings.TrimSpace(detail.SettlePayableAmount), "CNY"
	}
	if currency != "CNY" {
		return model.SeedanceBillImport{}, fmt.Errorf("bill detail %q uses unsupported currency %q", detail.BillDetailID, currency)
	}
	amount, err := exactMicroRMB(amountText)
	if err != nil {
		return model.SeedanceBillImport{}, fmt.Errorf("bill detail %q payable amount: %w", detail.BillDetailID, err)
	}
	source := map[string]any{
		"bill_detail_id": detail.BillDetailID, "bill_id": detail.BillID, "bill_period": period,
		"expense_date": detail.ExpenseDate, "expense_begin_time": detail.ExpenseBeginTime, "expense_end_time": detail.ExpenseEndTime,
		"product": detail.Product, "product_zh": detail.ProductZh, "instance_no": detail.InstanceNo,
		"charge_item_code": detail.ChargeItemCode, "configuration_code": detail.ConfigurationCode, "element_code": detail.ElementCode,
		"billing_function": detail.BillingFunction, "billing_method_code": detail.BillingMethodCode, "region_code": detail.RegionCode,
		"payable_amount": detail.PayableAmount, "currency": detail.Currency,
		"settle_payable_amount": detail.SettlePayableAmount, "currency_settlement": detail.CurrencySettlement,
	}
	sourceJSON, err := common.Marshal(source)
	if err != nil {
		return model.SeedanceBillImport{}, err
	}
	detailID := strings.TrimSpace(detail.BillDetailID)
	if detailID == "" || detailID == "-" {
		identity, marshalErr := common.Marshal(map[string]any{
			"period": period, "bill_id": detail.BillID, "expense_date": detail.ExpenseDate, "product": detail.Product,
			"instance_no": detail.InstanceNo, "charge_item_code": detail.ChargeItemCode,
			"configuration_code": detail.ConfigurationCode, "element_code": detail.ElementCode,
			"region_code": detail.RegionCode, "absolute_index": absoluteIndex,
		})
		if marshalErr != nil {
			return model.SeedanceBillImport{}, marshalErr
		}
		detailID = "synthetic:" + strings.TrimPrefix(model.SHA256Evidence(string(identity)), "sha256:")
	}
	return model.SeedanceBillImport{
		ChannelID: channelID, BillDetailID: detailID, BillingPeriod: period, ProductCode: strings.TrimSpace(detail.Product),
		CostCategory: seedanceBillCostCategory(detail),
		InstanceID:   strings.TrimSpace(detail.InstanceNo), UsageStartedAt: parseSeedanceBillTime(detail.ExpenseBeginTime, detail.ExpenseDate),
		UsageEndedAt: parseSeedanceBillTime(detail.ExpenseEndTime, detail.ExpenseDate), AmountMicroRMB: amount,
		SanitizedSourceJSON: string(sourceJSON), Candidates: []model.SeedanceCostCandidate{},
	}, nil
}

func seedanceBillCostCategory(detail seedanceVolcengineBillDetail) string {
	identity := strings.ToLower(strings.Join([]string{
		detail.Product, detail.ProductZh, detail.ChargeItemCode, detail.ConfigurationCode,
		detail.ElementCode, detail.BillingFunction,
	}, " "))
	for _, marker := range []string{"mediakit", "quality_enhance", "quality enhance", "画质增强", "视频增强", "超分"} {
		if strings.Contains(identity, marker) {
			return model.SeedanceBillCostSuperResolution
		}
	}
	return model.SeedanceBillCostArkGeneration
}

func exactMicroRMB(value string) (int64, error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return 0, errors.New("amount is not decimal")
	}
	micro := amount.Mul(decimal.NewFromInt(1_000_000))
	if !micro.Equal(micro.Truncate(0)) {
		return 0, errors.New("amount has precision finer than one micro-RMB")
	}
	if !micro.IsInteger() || micro.GreaterThan(decimal.NewFromInt(math.MaxInt64)) || micro.LessThan(decimal.NewFromInt(math.MinInt64)) {
		return 0, errors.New("amount is outside int64 micro-RMB range")
	}
	return micro.IntPart(), nil
}

func parseSeedanceBillTime(values ...string) int64 {
	formats := []string{time.RFC3339, "2006-01-02 15:04:05", "2006/01/02 15:04:05", "2006-01-02", "2006/01/02"}
	for _, value := range values {
		value = strings.TrimSpace(value)
		for _, format := range formats {
			if parsed, err := time.ParseInLocation(format, value, time.Local); err == nil {
				return parsed.Unix()
			}
		}
	}
	return 0
}
