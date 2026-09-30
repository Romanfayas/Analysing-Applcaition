// Centralized types for API responses

export type DataStatus = 'AVAILABLE' | 'PARTIAL' | 'DATA_UNAVAILABLE' | 'DATA_STALE' | 'DATA_INSUFFICIENT' | 'PROVIDER_ERROR';
export type ShariahStatus = 'PASS' | 'FAIL' | 'REVIEW_REQUIRED' | 'DATA_INSUFFICIENT' | 'DATA_UNAVAILABLE';
export type SignalAction = 'STRONG_BUY' | 'BUY' | 'HOLD' | 'SELL' | 'STRONG_SELL' | 'NEUTRAL';

export interface OHLCV {
  timestamp: string;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface TechnicalIndicators {
  sma_20?: number;
  ema_20?: number;
  rsi_14?: number;
  macd?: number;
  macd_signal?: number;
  macd_hist?: number;
  adx?: number;
}

export interface FundamentalMetrics {
  status: DataStatus;
  revenue?: number;
  net_profit?: number;
  eps?: number;
  pe_ratio?: number;
  pb_ratio?: number;
  roe?: number;
  roce?: number;
  debt_to_equity?: number;
  piotroski_f_score?: number;
}

export interface ShariahScreening {
  status: ShariahStatus;
  methodology: string;
  screening_date: string;
  debt_to_assets?: number;
  illiquid_to_assets?: number;
  non_compliant_revenue?: number;
}

export interface QuantMetrics {
  volatility_daily?: number;
  volatility_annual?: number;
  max_drawdown?: number;
  sharpe_ratio?: number;
  sortino_ratio?: number;
  var_95?: number;
}

export interface StockDetail {
  symbol: string;
  name: string;
  exchange: string;
  sector?: string;
  current_price: number;
  timestamp: string;
  
  ohlcv: OHLCV[];
  technicals: TechnicalIndicators;
  fundamentals: FundamentalMetrics;
  shariah: ShariahScreening;
  quant: QuantMetrics;
  
  signal?: {
    overall_score: number;
    action: SignalAction;
    confidence: number;
    explanation: string;
  };
}

export interface IPO {
  id: string;
  name: string;
  symbol: string;
  open_date: string;
  close_date: string;
  listing_date: string;
  price_band: string;
  issue_size: string;
  status: DataStatus;
  
  subscription?: {
    qib: number;
    nii: number;
    retail: number;
    total: number;
  };
  
  gmp?: {
    value: number;
    updated_at: string;
  };
  
  prediction?: {
    shariah_status: ShariahStatus;
    listing_gains_pct: number;
    recommendation: 'APPLY' | 'AVOID' | 'REVIEW_REQUIRED';
    confidence: number;
    explanation: string;
  };
}

export interface SignalOverview {
  symbol: string;
  name: string;
  price: number;
  action: SignalAction;
  score: number;
  shariah_status: ShariahStatus;
  timestamp: string;
}

export interface PortfolioPosition {
  symbol: string;
  name: string;
  shares: number;
  avg_price: number;
  current_price: number;
  pnl: number;
  pnl_pct: number;
}
