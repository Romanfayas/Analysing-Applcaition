/**
 * Indian Equity Cash Delivery Transaction Cost Calculator
 * Implements schedule: INDIA_EQ_DELIVERY_FEES_2026Q4_V2
 *
 * Statutory & Regulatory Levies (SEBI / NSE / Ministry of Finance):
 * - STT: 0.1% on delivery BUY, 0.1% on delivery SELL
 * - Stamp Duty: 0.015% on delivery BUY only (Indian Stamp Act, July 2020)
 * - Exchange Turnover: 0.00297% (NSE Cash market)
 * - SEBI Charges: 0.0001% (Rs 10 per crore)
 * - DP Charges: Rs 13.50 flat per scrip on SELL leg (CDSL/NSDL)
 * - GST: 18% on (Exchange Charges + SEBI Charges + Brokerage + DP Charges)
 * - Brokerage: Rs 0.00 (Zero Delivery Brokerage model)
 */

export interface TransactionCostBreakdown {
  turnover: number;
  stt: number;
  stampDuty: number;
  exchangeCharges: number;
  sebiCharges: number;
  brokerage: number;
  dpCharges: number;
  gst: number;
  totalFees: number;
  modelVersion: string;
}

export function calculateDeliveryCosts(tradeType: 'BUY' | 'SELL', turnover: number): TransactionCostBreakdown {
  if (turnover <= 0) {
    return {
      turnover: 0,
      stt: 0,
      stampDuty: 0,
      exchangeCharges: 0,
      sebiCharges: 0,
      brokerage: 0,
      dpCharges: 0,
      gst: 0,
      totalFees: 0,
      modelVersion: 'INDIA_EQ_DELIVERY_FEES_2026Q4_V2'
    };
  }

  const roundPaisa = (val: number) => Math.round(val * 100) / 100;
  const t = roundPaisa(turnover);

  const stt = roundPaisa(t * 0.001);
  const stampDuty = tradeType === 'BUY' ? roundPaisa(t * 0.00015) : 0;
  const exchangeCharges = roundPaisa(t * 0.0000297);
  const sebiCharges = roundPaisa(t * 0.000001);
  const brokerage = 0.0;
  const dpCharges = tradeType === 'SELL' ? 13.50 : 0.0;
  const gstBase = exchangeCharges + sebiCharges + brokerage + dpCharges;
  const gst = roundPaisa(gstBase * 0.18);
  const totalFees = roundPaisa(stt + stampDuty + exchangeCharges + sebiCharges + brokerage + dpCharges + gst);

  return {
    turnover: t,
    stt,
    stampDuty,
    exchangeCharges,
    sebiCharges,
    brokerage,
    dpCharges,
    gst,
    totalFees,
    modelVersion: 'INDIA_EQ_DELIVERY_FEES_2026Q4_V2'
  };
}
