package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

const aipddFinanceExportOrderLimit = 100000

func StartAIPDDFinanceExport(jobID string) {
	gopool.Go(func() {
		if err := runAIPDDFinanceExport(jobID); err != nil {
			_ = model.FailAIPDDFinanceExport(jobID, err)
			common.SysLog("AIPDD finance XLSX export failed: " + err.Error())
		}
	})
}

func runAIPDDFinanceExport(jobID string) error {
	job, err := model.GetAIPDDFinanceExportJob(jobID)
	if err != nil {
		return err
	}
	if err = model.MarkAIPDDFinanceExportRunning(jobID); err != nil {
		return err
	}
	var filter model.AIPDDFinanceOrderFilter
	if err = common.UnmarshalJsonStr(job.FilterJSON, &filter); err != nil {
		return err
	}
	orders, err := model.ListAllAIPDDFinanceOrders(filter, aipddFinanceExportOrderLimit)
	if err != nil {
		return err
	}
	orderIDs := make([]string, 0, len(orders))
	for _, order := range orders {
		orderIDs = append(orderIDs, order.ID)
	}
	movements, err := model.ListAIPDDFinanceMovements(orderIDs)
	if err != nil {
		return err
	}
	issues, err := model.ListAIPDDFinanceIssues(orderIDs)
	if err != nil {
		return err
	}
	summary, err := model.GetAIPDDFinanceSummary(filter)
	if err != nil {
		return err
	}
	data, err := buildAIPDDFinanceWorkbook(summary, orders, movements, issues)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	fileName := "aipdd-profit-report-" + time.Now().In(shanghaiLocation()).Format("20060102-150405") + ".xlsx"
	return model.CompleteAIPDDFinanceExport(jobID, fileName, hex.EncodeToString(hash[:]), data, int64(len(orders)))
}

func buildAIPDDFinanceWorkbook(summary model.AIPDDFinanceSummary, orders []model.AIPDDFinanceOrderListItem, movements []model.AIPDDFinanceMovement, issues []model.AIPDDFinanceOutbox) ([]byte, error) {
	book := excelize.NewFile()
	defer func() { _ = book.Close() }()
	if err := book.SetSheetName("Sheet1", "Summary"); err != nil {
		return nil, err
	}
	sheets := []string{"Order Details", "Cost Components", "Charge Refunds", "Invoice Adjustments", "Reconciliation Issues"}
	for _, sheet := range sheets {
		if _, err := book.NewSheet(sheet); err != nil {
			return nil, err
		}
	}
	headerStyle, err := book.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"2563EB"}, Pattern: 1},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	if err = writeSummarySheet(book, headerStyle, summary); err != nil {
		return nil, err
	}
	if err = writeOrderSheet(book, headerStyle, orders); err != nil {
		return nil, err
	}
	if err = writeCostSheets(book, headerStyle, orders, movements); err != nil {
		return nil, err
	}
	if err = writeIssueSheet(book, headerStyle, orders, issues); err != nil {
		return nil, err
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return bytes.Clone(buffer.Bytes()), nil
}

func writeSummarySheet(book *excelize.File, headerStyle int, summary model.AIPDDFinanceSummary) error {
	rows := [][]any{
		{"Metric", "Value"},
		{"Order count", summary.OrderCount},
		{"Customer net consumption (RMB)", moneyString(summary.CustomerNetConsumptionRMBMic)},
		{"Confirmed AIPDD source cost (RMB)", moneyString(summary.ConfirmedSourceCostRMBMic)},
		{"Confirmed profit (RMB)", moneyString(summary.ConfirmedProfitRMBMic)},
		{"Estimated AIPDD source cost (RMB)", moneyString(summary.EstimatedSourceCostRMBMic)},
		{"Estimated profit (RMB)", moneyString(summary.EstimatedProfitRMBMic)},
		{"Loss orders", summary.LossOrderCount},
		{"Pending confirmation", summary.PendingConfirmationCount},
		{"Manual review", summary.ManualReviewCount},
		{"Generated at (Asia/Shanghai)", time.Now().In(shanghaiLocation()).Format("2006-01-02 15:04:05")},
	}
	for index, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := book.SetSheetRow("Summary", cell, &row); err != nil {
			return err
		}
	}
	if err := book.SetCellStyle("Summary", "A1", "B1", headerStyle); err != nil {
		return err
	}
	_ = book.SetColWidth("Summary", "A", "A", 42)
	_ = book.SetColWidth("Summary", "B", "B", 24)
	return nil
}

func writeOrderSheet(book *excelize.File, headerStyle int, orders []model.AIPDDFinanceOrderListItem) error {
	headers := []any{"Occurred at", "Platform order", "Attempt", "NewAPI user ID", "Username", "Token ID", "Token name", "Masked token", "Channel ID", "Channel", "Instance ID", "Model", "Order status", "Billing status", "Cost status", "Customer charge (quota)", "Customer charge (RMB)", "AIPDD charge (AWCoin)", "AIPDD source charge (RMB)", "Base model cost (RMB)", "AIPDD model cost (RMB)", "Actual spend (AWCoin)", "Actual spend (RMB)", "Confirmed profit (RMB)", "Estimated profit (RMB)", "Settlement revision", "Trace completeness", "Upstream reference"}
	if err := book.SetSheetRow("Order Details", "A1", &headers); err != nil {
		return err
	}
	for index, order := range orders {
		row := []any{formatShanghaiTime(order.OccurredAt), order.PlatformOrderID, order.LatestAttemptID, order.UserID, order.Username,
			order.TokenID, order.TokenName, order.TokenMaskedKey, order.ChannelID, order.ChannelName, order.InstanceID, order.Model,
			order.OrderStatus, order.LocalBillingStatus, order.CostStatus, order.CustomerChargeQuota, moneyString(order.CustomerChargeRMBMic),
			order.AIPDDChargeAWCoin, optionalMoneyString(order.AIPDDChargeRMBMic), optionalMoneyString(order.BaseModelCostRMBMic),
			optionalMoneyString(order.AIPDDModelCostRMBMic), optionalIntString(order.ActualSpendAWCoin), optionalMoneyString(order.ActualSpendRMBMic),
			optionalMoneyString(order.ConfirmedProfitRMBMic), optionalMoneyString(order.EstimatedProfitRMBMic), order.SettlementRevision,
			order.FinancialTraceCompleteness, order.UpstreamReference}
		cell, _ := excelize.CoordinatesToCellName(1, index+2)
		if err := book.SetSheetRow("Order Details", cell, &row); err != nil {
			return err
		}
	}
	return finishTableSheet(book, "Order Details", headerStyle, len(headers), len(orders)+1)
}

func writeCostSheets(book *excelize.File, headerStyle int, orders []model.AIPDDFinanceOrderListItem, movements []model.AIPDDFinanceMovement) error {
	orderMap := make(map[string]model.AIPDDFinanceOrderListItem, len(orders))
	for _, order := range orders {
		orderMap[order.ID] = order
	}
	costHeaders := []any{"Platform order", "Component", "AWCoin/quota delta", "RMB delta", "Occurred at", "Evidence"}
	chargeHeaders := append([]any(nil), costHeaders...)
	adjustmentHeaders := append([]any(nil), costHeaders...)
	if err := book.SetSheetRow("Cost Components", "A1", &costHeaders); err != nil {
		return err
	}
	if err := book.SetSheetRow("Charge Refunds", "A1", &chargeHeaders); err != nil {
		return err
	}
	if err := book.SetSheetRow("Invoice Adjustments", "A1", &adjustmentHeaders); err != nil {
		return err
	}
	costRow, chargeRow, adjustmentRow := 2, 2, 2
	for _, movement := range movements {
		order := orderMap[movement.FinanceOrderID]
		row := []any{order.PlatformOrderID, movement.Component, movement.QuotaDelta, moneyString(movement.RMBDeltaMic), formatShanghaiTime(movement.OccurredAt), movement.Evidence}
		target := "Cost Components"
		rowNumber := &costRow
		if movement.Component == "CUSTOMER_CHARGE" || movement.Component == "ORDER_STATUS" {
			target, rowNumber = "Charge Refunds", &chargeRow
		} else if movement.Component != "AIPDD_SOURCE_CHARGE" && movement.Component != "BASE_MODEL_COST" && movement.Component != "AIPDD_MODEL_COST" && movement.Component != "ACTUAL_SPEND" && movement.Component != "ACTUAL_SPEND_AWCOIN" {
			target, rowNumber = "Invoice Adjustments", &adjustmentRow
		}
		cell, _ := excelize.CoordinatesToCellName(1, *rowNumber)
		if err := book.SetSheetRow(target, cell, &row); err != nil {
			return err
		}
		*rowNumber++
	}
	if err := finishTableSheet(book, "Cost Components", headerStyle, len(costHeaders), costRow-1); err != nil {
		return err
	}
	if err := finishTableSheet(book, "Charge Refunds", headerStyle, len(chargeHeaders), chargeRow-1); err != nil {
		return err
	}
	return finishTableSheet(book, "Invoice Adjustments", headerStyle, len(adjustmentHeaders), adjustmentRow-1)
}

func writeIssueSheet(book *excelize.File, headerStyle int, orders []model.AIPDDFinanceOrderListItem, issues []model.AIPDDFinanceOutbox) error {
	headers := []any{"Platform order", "Channel ID", "Instance ID", "Order status", "Billing status", "Cost status", "Issue type", "Sync state", "Attempts", "Last error", "Updated at"}
	if err := book.SetSheetRow("Reconciliation Issues", "A1", &headers); err != nil {
		return err
	}
	rowNumber := 2
	seen := make(map[string]bool)
	for _, issue := range issues {
		row := []any{issue.PlatformOrderID, issue.ChannelID, issue.InstanceID, "", "", "", "SYNC", issue.State, issue.Attempts, issue.LastError, formatShanghaiTime(issue.UpdatedAt)}
		cell, _ := excelize.CoordinatesToCellName(1, rowNumber)
		if err := book.SetSheetRow("Reconciliation Issues", cell, &row); err != nil {
			return err
		}
		rowNumber++
		seen[issue.PlatformOrderID] = true
	}
	for _, order := range orders {
		if !order.RequiresManualReview && order.CostStatus == "CONFIRMED" {
			continue
		}
		if seen[order.PlatformOrderID] {
			continue
		}
		issueType := "COST_PENDING"
		if order.RequiresManualReview {
			issueType = "MANUAL_REVIEW"
		}
		if order.CostStatus == "UNVERIFIABLE" {
			issueType = "UNVERIFIABLE"
		}
		row := []any{order.PlatformOrderID, order.ChannelID, order.InstanceID, order.OrderStatus, order.LocalBillingStatus, order.CostStatus, issueType, "", 0, "", formatShanghaiTime(order.UpdatedAt)}
		cell, _ := excelize.CoordinatesToCellName(1, rowNumber)
		if err := book.SetSheetRow("Reconciliation Issues", cell, &row); err != nil {
			return err
		}
		rowNumber++
	}
	return finishTableSheet(book, "Reconciliation Issues", headerStyle, len(headers), rowNumber-1)
}

func finishTableSheet(book *excelize.File, sheet string, headerStyle, columnCount, rowCount int) error {
	lastColumn, _ := excelize.ColumnNumberToName(columnCount)
	if err := book.SetCellStyle(sheet, "A1", lastColumn+"1", headerStyle); err != nil {
		return err
	}
	if err := book.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return err
	}
	if rowCount > 0 {
		if err := book.AutoFilter(sheet, "A1:"+lastColumn+fmt.Sprint(max(1, rowCount)), nil); err != nil {
			return err
		}
	}
	_ = book.SetColWidth(sheet, "A", lastColumn, 20)
	return nil
}

func moneyString(mic int64) string { return decimal.NewFromInt(mic).Shift(-6).StringFixed(6) }
func optionalMoneyString(mic *int64) string {
	if mic == nil {
		return "Pending confirmation"
	}
	return moneyString(*mic)
}
func optionalIntString(value *int64) any {
	if value == nil {
		return "Pending confirmation"
	}
	return *value
}
func formatShanghaiTime(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}
	return time.Unix(timestamp, 0).In(shanghaiLocation()).Format("2006-01-02 15:04:05")
}
func shanghaiLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("Asia/Shanghai", 8*3600)
	}
	return location
}
