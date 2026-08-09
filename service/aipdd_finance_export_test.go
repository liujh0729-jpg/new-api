package service

import (
	"bytes"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestAIPDDFinanceWorkbookContainsCompleteFinancialSheets(t *testing.T) {
	cost := int64(1_000_000)
	profit := int64(2_000_000)
	data, err := buildAIPDDFinanceWorkbook(
		model.AIPDDFinanceSummary{OrderCount: 1, CustomerNetConsumptionRMBMic: 3_000_000, ConfirmedSourceCostRMBMic: cost, ConfirmedProfitRMBMic: profit},
		[]model.AIPDDFinanceOrderListItem{{ID: "ledger-1", PlatformOrderID: "order-1", TokenMaskedKey: "sk-a**********1234", CustomerChargeRMBMic: 3_000_000, AIPDDChargeRMBMic: &cost, ConfirmedProfitRMBMic: &profit, CostStatus: "CONFIRMED"}},
		[]model.AIPDDFinanceMovement{{FinanceOrderID: "ledger-1", Component: "CUSTOMER_CHARGE", RMBDeltaMic: 3_000_000}},
		[]model.AIPDDFinanceOutbox{},
	)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	book, err := excelize.OpenReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer func() { _ = book.Close() }()
	require.Equal(t, []string{"Summary", "Order Details", "Cost Components", "Charge Refunds", "Invoice Adjustments", "Reconciliation Issues"}, book.GetSheetList())
	masked, err := book.GetCellValue("Order Details", "H2")
	require.NoError(t, err)
	require.Equal(t, "sk-a**********1234", masked)
}
