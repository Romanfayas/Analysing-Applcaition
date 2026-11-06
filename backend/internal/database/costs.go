package database

import (
	"math"
)

// FeeModelVersion identifies the Indian delivery equity fee schedule in force.
const FeeModelVersion = "INDIA_EQ_DELIVERY_FEES_2026Q4_V2"

// TransactionCostBreakdown details each statutory and exchange levy for Indian cash delivery.
// Rates follow official SEBI, NSE, and Ministry of Finance schedules (effective post-Oct 2024):
// - STT: 0.1% on delivery BUY, 0.1% on delivery SELL (Securities Transaction Tax Act)
// - Stamp Duty: 0.015% on delivery BUY only (Indian Stamp Act, effective July 1, 2020)
// - Exchange Turnover Charges: 0.00297% (NSE Cash market headline rate)
// - SEBI Turnover Charges: 0.0001% (Rs 10 per crore)
// - Depository Participant (DP) Charges: Rs 13.50 flat per scrip per sell day (CDSL/NSDL)
// - GST: 18% on (Exchange Charges + SEBI Charges + Brokerage + DP Charges). NOT levied on STT or Stamp Duty.
// - Brokerage: Rs 0.00 (Zero Delivery Brokerage discount-broker schedule: Zerodha/Groww/Angel One).
type TransactionCostBreakdown struct {
	Turnover        float64 `json:"turnover"`
	STT             float64 `json:"stt"`
	StampDuty       float64 `json:"stamp_duty"`
	ExchangeCharges float64 `json:"exchange_charges"`
	SEBICharges     float64 `json:"sebi_charges"`
	Brokerage       float64 `json:"brokerage"`
	DPCharges       float64 `json:"dp_charges"`
	GST             float64 `json:"gst"`
	TotalFees       float64 `json:"total_fees"`
	FeeModelVersion string  `json:"fee_model_version"`
}

// roundPaisa rounds a float to the nearest paisa (2 decimal places).
func roundPaisa(val float64) float64 {
	return math.Round(val*100.0) / 100.0
}

// CalculateDeliveryTransactionCosts computes exact itemized Indian cash delivery fees.
func CalculateDeliveryTransactionCosts(tradeType string, turnover float64) TransactionCostBreakdown {
	if turnover <= 0 {
		return TransactionCostBreakdown{FeeModelVersion: FeeModelVersion}
	}

	b := TransactionCostBreakdown{
		Turnover:        roundPaisa(turnover),
		FeeModelVersion: FeeModelVersion,
	}

	// 1. Securities Transaction Tax (STT) - 0.1% on both BUY and SELL
	b.STT = roundPaisa(turnover * 0.001)

	// 2. Stamp Duty - 0.015% on BUY only; 0% on SELL
	if tradeType == "BUY" {
		b.StampDuty = roundPaisa(turnover * 0.00015)
	} else {
		b.StampDuty = 0.0
	}

	// 3. Exchange Transaction Charges - 0.00297% (NSE Cash)
	b.ExchangeCharges = roundPaisa(turnover * 0.0000297)

	// 4. SEBI Turnover Charges - 0.0001% (Rs 10 per crore)
	b.SEBICharges = roundPaisa(turnover * 0.000001)

	// 5. Brokerage - Rs 0.00 for delivery equity
	b.Brokerage = 0.0

	// 6. DP Charges - Rs 13.50 flat on SELL leg only
	if tradeType == "SELL" {
		b.DPCharges = 13.50
	} else {
		b.DPCharges = 0.0
	}

	// 7. GST - 18% on (Exchange Charges + SEBI Charges + Brokerage + DP Charges)
	gstBase := b.ExchangeCharges + b.SEBICharges + b.Brokerage + b.DPCharges
	b.GST = roundPaisa(gstBase * 0.18)

	// Total Fees is the exact sum of line items
	b.TotalFees = roundPaisa(b.STT + b.StampDuty + b.ExchangeCharges + b.SEBICharges + b.Brokerage + b.DPCharges + b.GST)

	return b
}

// IntradayFeeModelVersion identifies the same-day (MIS) equity schedule; it mirrors the quant engine's schedule.
const IntradayFeeModelVersion = "INDIA_EQ_INTRADAY_FEES_2026Q4_V1"

// CalculateIntradayTransactionCosts is the same-day equity schedule: STT 0.025% on the sell leg only, stamp duty
// 0.003% on the buy leg, exchange and SEBI charges as for delivery, a discount-broker brokerage of min(Rs 20, 0.03%)
// per executed order, GST on brokerage + exchange + SEBI, and no depository charge. Re-verify the brokerage against
// your own contract note.
func CalculateIntradayTransactionCosts(tradeType string, turnover float64) TransactionCostBreakdown {
	if turnover <= 0 {
		return TransactionCostBreakdown{FeeModelVersion: IntradayFeeModelVersion}
	}
	b := TransactionCostBreakdown{Turnover: roundPaisa(turnover), FeeModelVersion: IntradayFeeModelVersion}
	if tradeType == "SELL" {
		b.STT = roundPaisa(turnover * 0.00025)
	} else {
		b.StampDuty = roundPaisa(turnover * 0.00003)
	}
	b.ExchangeCharges = roundPaisa(turnover * 0.0000297)
	b.SEBICharges = roundPaisa(turnover * 0.000001)
	b.Brokerage = roundPaisa(math.Min(20, turnover*0.0003))
	b.GST = roundPaisa((b.ExchangeCharges + b.SEBICharges + b.Brokerage) * 0.18)
	b.TotalFees = roundPaisa(b.STT + b.StampDuty + b.ExchangeCharges + b.SEBICharges + b.Brokerage + b.GST)
	return b
}
