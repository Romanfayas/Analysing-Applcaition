// Centralized types for API responses

export type DataStatus = 'AVAILABLE' | 'VALID' | 'PARTIAL' | 'DATA_UNAVAILABLE' | 'DATA_STALE' | 'DATA_INSUFFICIENT' | 'PROVIDER_ERROR';
export type ShariahStatus = 'PASS' | 'FAIL' | 'REVIEW_REQUIRED' | 'DATA_INSUFFICIENT' | 'DATA_UNAVAILABLE' | 'UNKNOWN' | 'PENDING';
export type SignalAction = 'STRONG_BUY' | 'BUY' | 'HOLD' | 'SELL' | 'STRONG_SELL' | 'NEUTRAL' | 'WATCH' | 'AVOID' | 'STRONG_AVOID' | 'NO_SIGNAL';
export type FreshnessStatus = 'FRESH' | 'LAST_AVAILABLE' | 'STALE' | 'UNAVAILABLE' | 'MISSING';
export type MarketSessionStatus = 'OPEN' | 'CLOSED' | 'PRE_MARKET' | 'POST_MARKET';

export interface StockListItem {
  id: number;
  symbol: string;
  name: string;
  exchange: string;
  sector?: string;
  industry?: string;
  market_cap?: string;
  price: number | null;
  change: number | null;
  change_percent: number | null;
  prev_close: number | null;
  marketTimestamp: string | null;
  receivedAt: string | null;
  provider: string | null;
  freshness: FreshnessStatus;
  marketStatus: MarketSessionStatus;
  shariah: ShariahStatus;
  signal: SignalAction;
  score: number | null;
  is_active?: boolean;
  corporate_action?: {
    status: string;
    action_type: string;
    effective_date: string;
    notes?: string;
    successors?: Array<{
      symbol: string;
      name: string;
      ratio: string;
      cost_split_pct: number;
      is_primary: boolean;
    }>;
  };
}

export interface MarketHealthResponse {
  provider: string;
  exchange_verified: boolean;
  guaranteed_realtime: boolean;
  latest_market_timestamp: string | null;
  latest_received_timestamp: string | null;
  latest_ingestion_time?: string | null;
  data_age_seconds: number | null;
  freshness_status: FreshnessStatus;
  market_session: MarketSessionStatus;
  fallback_active: boolean;
  data_interval: number;
  data_type: string;
  total_candles: number;
}


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
  pat?: number;
  eps?: number;
  pe_ratio?: number;
  pb_ratio?: number;
  roe?: number;
  roce?: number;
  debt_to_equity?: number;
  piotroski_f_score?: number;
  total_debt?: number;
  total_assets?: number;
  period_end?: string;
  pit_mode?: boolean;
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
  current_price: number | null;
  change?: number | null;
  change_percent?: number | null;
  prev_close?: number | null;
  timestamp: string | null;
  received_at?: string | null;
  provider?: string | null;
  freshness?: FreshnessStatus;
  market_session?: MarketSessionStatus;
  is_active?: boolean;
  corporate_action?: {
    status: string;
    action_type: string;
    effective_date: string;
    notes?: string;
    successors?: Array<{
      symbol: string;
      name: string;
      ratio: string;
      cost_split_pct: number;
      is_primary: boolean;
    }>;
  };
  
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
