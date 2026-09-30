// TypeScript interfaces matching backend models

export interface Symbol {
  id: number;
  symbol: string;
  name: string;
  exchange: string;
  isin?: string;
  sector?: string;
  industry?: string;
  market_cap_category?: string;
  is_active: boolean;
}

export interface OHLCV {
  timestamp: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
  source: string;
  quality_status: DataQualityStatus;
}

export type DataQualityStatus = "VALID" | "MISSING" | "STALE" | "DUPLICATE" | "SUSPECT" | "CORRECTED";
export type ShariahStatus = "PASS" | "FAIL" | "REVIEW_REQUIRED" | "DATA_INSUFFICIENT";
export type SignalType = "STRONG_BUY" | "BUY" | "WATCH" | "AVOID" | "STRONG_AVOID";
export type IPODecisionType = "APPLY" | "WATCH" | "AVOID" | "REVIEW_REQUIRED";

export interface StockSignal {
  symbol_id: number;
  signal: SignalType;
  overall_score: number;
  fundamental_score?: number;
  technical_score?: number;
  candlestick_score?: number;
  momentum_score?: number;
  volume_score?: number;
  valuation_score?: number;
  market_regime_score?: number;
  risk_score?: number;
  quality_score?: number;
  shariah_status: ShariahStatus;
  explanation?: string;
  supporting_factors?: Record<string, unknown>;
  conflicting_factors?: Record<string, unknown>;
  calculated_at: string;
}

export interface ShariahScreening {
  symbol_id: number;
  status: ShariahStatus;
  debt_ratio?: number;
  debt_ratio_pass?: boolean;
  interest_income_ratio?: number;
  interest_income_pass?: boolean;
  cash_deposit_ratio?: number;
  cash_deposit_pass?: boolean;
  receivables_ratio?: number;
  receivables_pass?: boolean;
  business_activity_pass?: boolean;
  purification_per_share?: number;
  screened_at: string;
}

export interface IPO {
  id: number;
  company_name: string;
  symbol?: string;
  sector?: string;
  open_date?: string;
  close_date?: string;
  listing_date?: string;
  price_band_low?: number;
  price_band_high?: number;
  issue_price?: number;
  listing_price?: number;
  lot_size?: number;
  min_investment?: number;
  issue_size?: number;
  status: string;
}

export interface IPOPrediction {
  model_name: string;
  predicted_price: number;
  bull_case?: number;
  base_case?: number;
  bear_case?: number;
  confidence_score?: number;
  predicted_range_low?: number;
  predicted_range_high?: number;
}

export interface RiskCalculation {
  capital: number;
  risk_percentage: number;
  max_risk_amount: number;
  entry_price: number;
  stop_loss: number;
  target_price?: number;
  risk_per_share: number;
  position_size: number;
  position_value: number;
  risk_reward_ratio?: number;
}

export interface QuantMetrics {
  absolute_return?: number;
  cagr?: number;
  volatility_annual?: number;
  max_drawdown?: number;
  sharpe_ratio?: number;
  sortino_ratio?: number;
  calmar_ratio?: number;
  beta?: number;
  alpha?: number;
  var_95?: number;
  cvar_95?: number;
}

export interface APIResponse<T> {
  success: boolean;
  data?: T;
  error?: string;
  meta?: {
    page: number;
    per_page: number;
    total: number;
  };
}
