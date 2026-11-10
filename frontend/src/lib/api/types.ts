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
  /** Breakout-engine stage behind `score`. */
  setup?: string | null;
  score_as_of?: string | null;
  score_basis?: string | null;
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

// Go serialises missing values as null (never 0): every numeric field must be null-checked before use.
export interface FundamentalMetrics {
  status: DataStatus;
  revenue?: number | null;
  net_profit?: number | null;
  pat?: number | null;
  eps?: number | null;
  pe_ratio?: number | null;
  pb_ratio?: number | null;
  roe?: number | null;
  roce?: number | null;
  debt_to_equity?: number | null;
  piotroski_f_score?: number | null;
  total_debt?: number | null;
  total_assets?: number | null;
  period_end?: string;
  pit_mode?: string;
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
    overall_score: number | null;
    action: SignalAction;
    explanation: string;
    is_actionable?: boolean;
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

// ==========================================
// API envelope & shared view models
// ==========================================

export interface ApiEnvelope<T> {
  success: boolean;
  data: T;
  error?: string;
  meta?: { page?: number; per_page?: number; total?: number };
}

// Mirrors the Go BreakoutResult: numeric fields are always present (never null).
export interface BreakoutCandidate {
  symbol: string;
  sector?: string;
  stage: string;
  /** Zero for any instrument that is not a certified Shariah PASS (the engine's hard gate). Do not display as the setup score. */
  overall_score: number;
  /** Price-structure score 0-100 before the Shariah gate; null when there is too little history. */
  structure_score: number | null;
  setup_type?: string;
  market_regime?: string;
  liquidity_adt_cr?: number;
  shariah_status: ShariahStatus | string;
  current_price: number;
  resistance_level: number;
  support_level: number;
  distance_to_resistance_pct: number;
  rvol: number;
  atr: number;
  bollinger_band_width: number;
  consolidation_bars: number;
  volume_demand_proxy: number;
  order_book_depth_status: string;
  entry_zone: [number, number];
  invalidation_level: number;
  target_1: number;
  target_2: number;
  reasons: string[];
  invalidation_conditions: string[];
  score_breakdown?: Record<string, number>;
  /** Where the latest price stands against these levels, which come from the last completed session. */
  live?: LivePosition;
}

export interface SectorHeat {
  sector: string;
  count: number;
  average_score: number;
  strength: string;
}

export interface BreakoutScan {
  /** Date of the newest daily bar scored. */
  data_as_of?: string | null;
  computed_at?: string;
  stale_sessions?: number;
  data_quality?: 'FRESH' | 'STALE';
  basis?: string;
  excluded?: Array<{ symbol: string; reason: string }>;
  candidates?: BreakoutCandidate[];
  stage_counts?: Record<string, number>;
  bullish_breadth_percent?: number;
  sector_heatmap?: SectorHeat[];
  total_screened?: number;
}

export interface ShariahScreeningRow {
  id: number;
  symbol: string;
  sector?: string;
  status: ShariahStatus | string;
  screening_date?: string;
  debt_ratio?: number | null;
  cash_deposit_ratio?: number | null;
  receivables_ratio?: number | null;
  interest_income_ratio?: number | null;
}

export interface LedgerPosition {
  id?: number;
  symbol: string;
  shares: number;
  avg_cost_basis: number;
  last_price?: number;
  unrealized_pnl?: number;
}

export interface PortfolioSummary {
  id: number;
  cash_balance: number;
  initial_capital: number;
  invested_value: number;
  total_equity: number;
  unrealized_pnl: number;
  unrealized_pnl_pct: number;
}

export interface PaperTrade {
  id: number;
  symbol: string;
  trade_type: 'BUY' | 'SELL';
  shares: number;
  price: number;
  fees_paid: number;
  realized_pnl: number | null;
  trade_date: string;
  shariah_status_at_trade: string | null;
  /** Where the fill price came from (INTRADAY_1M, EXCHANGE_DAILY_CLOSE, LIVE_LATEST, ...). */
  price_source?: string | null;
  price_bar_time?: string | null;
  /** True when the order was run "as of" a past time rather than at the latest price. */
  is_backdated?: boolean;
}

export interface PricePoint {
  price: number;
  source: string;
  bar_time: string | null;
  resolved_at: string;
  note?: string;
}

export interface PriceAtResult {
  symbol: string;
  requested_at: string;
  price: PricePoint;
}

export interface CostBreakdown {
  turnover: number;
  total_fees: number;
  fee_model_version: string;
}

export interface StopOutcome {
  triggered: boolean;
  reason?: 'STOP_LOSS' | 'TRAILING_STOP' | 'TARGET';
  price?: number;
  bar_start?: string;
  basis?: string;
  gap?: boolean;
  ambiguous?: boolean;
  stop_level?: number;
  high_water: number;
}

export interface LiquidityInfo {
  adv_shares: number;
  daily_volatility_pct: number;
  sessions: number;
  participation_pct: number;
  impact_bps: number;
}

export interface SimulationResult {
  symbol: string;
  shares: number;
  product: 'DELIVERY' | 'INTRADAY';
  from: string;
  to: string;
  entry: PricePoint;
  exit: PricePoint;
  exit_reason: 'END_OF_WINDOW' | 'STOP_LOSS' | 'TRAILING_STOP' | 'TARGET';
  protective_orders: {
    config: { StopLossPct: number; TargetPct: number; TrailingStopPct: number };
    outcome: StopOutcome | null;
    without_protection: { exit: PricePoint; net_pnl: number; net_return_pct: number } | null;
  };
  slippage: { base_bps: number; total_bps_each_side: number; liquidity: LiquidityInfo | null; assumption: string };
  buy_costs: CostBreakdown;
  sell_costs: CostBreakdown;
  invested: number;
  proceeds: number;
  net_pnl: number;
  net_return_pct: number;
  benchmark: { index: string; from_date?: string; to_date?: string; return_pct?: number; excess_return_pct?: number; note?: string };
  corporate_actions: { type: string; ex_date: string; subject: string }[];
  warnings: string[];
  disclaimer: string;
}

export interface IpoPrediction {
  shariah_status?: string;
  recommendation?: string;
  explanation?: string;
  predicted_price?: number;
  listing_gains_pct?: number;
  data_cutoff_at?: string;
  actual_listing_price?: number;
  actual_listing_return?: number;
  prediction_error_pct?: number;
  inputs?: Record<string, unknown>;
}

export interface IpoView {
  id: string;
  name: string;
  symbol: string;
  sector?: string;
  status: string;
  shariah_status?: string;
  source?: string;
  price_band?: string;
  issue_size?: string;
  issue_size_raw?: number | null;
  open_date?: string | null;
  close_date?: string | null;
  listing_date?: string | null;
  /** MAINBOARD | SME, from the exchange's series. */
  board?: 'MAINBOARD' | 'SME' | null;
  listing_at?: string | null;
  details?: IpoDetails | null;
  listing_price?: number | null;
  listing_day_close?: number | null;
  issue_price?: number | null;
  expected_listing_date?: string | null;
  listing_date_basis?: 'ANNOUNCED' | 'SCHEDULED_IN_PROSPECTUS' | 'EXPECTED_SEBI_T_PLUS_3' | null;
  lot_size?: number | null;
  subscription?: { total?: number; qib?: number | null; nii?: number | null; retail?: number | null; recorded_at?: string } | null;
  gmp?: { value: number; percentage?: number | null; source?: string; updated_at?: string; disclaimer?: string } | null;
  prediction?: IpoPrediction | null;
}

export interface IpoRun {
  id: number;
  provider: string;
  started_at: string;
  completed_at?: string;
  status: string;
  records_fetched: number;
  records_inserted: number;
  records_updated: number;
  records_rejected?: number;
  error_details?: string;
  source_url?: string;
}

export interface IpoHealth {
  feed_status?: string;
  active_issues_count?: number;
  historical_count?: number;
  latest_run?: { completed_at?: string } | null;
}

export interface DataQualityDataset {
  dataset: string;
  provider?: string;
  status: string;
  latest_data?: string | null;
  valid_records?: number;
  invalid_records?: number;
}

export interface DataQualityIssue {
  id: number;
  symbol?: string;
  severity: string;
  check_type?: string;
  message?: string;
  recorded_at?: string;
}

export interface ResearchGate {
  status: 'RESEARCH_BLOCKED' | 'RESEARCH_ELIGIBLE';
  reason: string;
  quarantined_bars_count: number;
  equities_with_quarantined_bars: number;
  total_instruments: number;
  instruments_with_bars: number;
  unauthenticated_demoted_events: number;
  primary_verified_events: number;
  canonical_strategy_hash: string | null;
  is_strategy_hash_valid: boolean | null;
  strategy_problems?: string[];
}

export interface BacktestResult {
  status?: 'RESEARCH_BLOCKED';
  reason?: string;
  quarantined_bars?: number;
  gate?: ResearchGate;
  total_return_percent?: number;
  max_drawdown_percent?: number;
  win_rate_percent?: number;
  total_trades?: number;
}

export interface UserAlert {
  id: number;
  symbol: string;
  alert_type: 'PRICE_ABOVE' | 'PRICE_BELOW';
  threshold: number;
  channel: 'IN_APP' | 'TELEGRAM';
  is_active: boolean;
  created_at: string;
  triggered_at: string | null;
}

export interface AlertEvent {
  id: number;
  alert_id: number;
  symbol: string;
  alert_type: string;
  threshold: number;
  observed_value: number;
  observed_at: string;
  message: string;
  triggered_at: string;
}

export interface UserPreferences {
  default_benchmark: string;
  default_risk_pct: number;
  display_timezone: string;
}

export interface SettingsResponse {
  preferences: UserPreferences;
  notifications: { telegram_configured: boolean; note: string };
  execution_safety: { paper_trading_only: boolean; live_order_execution: boolean; broker_trading_enabled: boolean };
}

export interface PortfolioRiskMetrics {
  portfolio_id?: number;
  total_equity?: number;
  cash_balance?: number;
  invested_value?: number;
  open_positions_count?: number;
  max_risk_per_trade_pct?: number;
  max_portfolio_heat_pct?: number;
  execution_mode?: string;
  short_selling_permitted?: boolean;
  margin_permitted?: boolean;
}

export interface MarketIndex {
  symbol: string;
  name: string;
  value: number | null;
  change: number | null;
  change_percent: number | null;
  status: string;
  as_of?: string;
  pe?: number | null;
  pb?: number | null;
  div_yield?: number | null;
}

export interface MarketOverview {
  market_status: string;
  indices: MarketIndex[];
  indices_note?: string;
  total_active_equities: number;
  sectors: Array<{ sector: string; count: number }>;
}

// ==========================================
// Intraday prediction engine
// ==========================================

export type PredictionVerdict = 'ACTIONABLE' | 'NO_DEMONSTRATED_EDGE' | 'DATA_INSUFFICIENT';
export type PredictionAction = 'BUY_CANDIDATE' | 'EXIT_OR_AVOID' | 'NO_TRADE';

export interface ConfidenceInterval {
  estimate: number | null;
  low: number | null;
  high: number | null;
}

export interface WalkForwardMetrics {
  reason?: string;
  days_available: number;
  round_trip_cost: number;
  test_days?: string[];
  oos_rows?: number;
  log_loss?: { model: number; climatology: number };
  log_loss_gain_vs_climatology?: ConfidenceInterval;
  accuracy?: { model: number; climatology: number };
  expected_calibration_error_p_up?: number | null;
  trading?: {
    model_signals: number;
    model_net_return_per_trade: ConfidenceInterval;
    model_hit_rate_net: number | null;
    baseline_momentum_net_return_per_trade: ConfidenceInterval;
    baseline_always_long_net_return_per_trade: ConfidenceInterval;
  };
  edge_checks?: Record<string, boolean>;
  positive_net_days?: number;
  per_day?: Array<{ date: string; regime: string; signals: number; mean_net_return: number | null }>;
  regimes?: Record<string, { days: number; signals: number; mean_net_return: number | null; log_loss_gain: number }>;
}

export interface PredictionModel {
  run_id: number;
  model_version: string;
  horizon_minutes: number;
  trained_at: string;
  data_from: string | null;
  data_to: string | null;
  n_symbols: number;
  verdict: PredictionVerdict;
  metrics: WalkForwardMetrics;
  cost_model: { round_trip_cost_fraction: number; fee_schedule: string };
}

export interface PredictionReason {
  code: string;
  detail: string;
}

export interface Prediction {
  id: number;
  symbol: string;
  horizon_minutes: number;
  as_of: string;
  target_time: string;
  issued_at: string;
  data_latency_seconds: number;
  reference_price: number;
  p_down: number;
  p_neutral: number;
  p_up: number;
  expected_return: number;
  return_q10: number;
  return_q50: number;
  return_q90: number;
  action: PredictionAction;
  action_reasons: PredictionReason[];
  entry_price: number | null;
  target_price: number | null;
  stop_price: number | null;
  shariah_status: string;
  model_verdict: PredictionVerdict;
  realized_price: number | null;
  realized_return: number | null;
  realized_class: 'UP' | 'NEUTRAL' | 'DOWN' | null;
  resolved_at: string | null;
}

export interface PredictionTracking {
  horizon_minutes: number;
  issued: number;
  resolved: number;
  pending: number;
  unresolvable: number;
  recommended_buys: { n: number; mean_net_return: number | null; hit_rate_net: number | null };
  model_view_buys: { n: number; mean_net_return: number | null };
  baseline_always_long_mean_net_return: number | null;
  brier_p_up: number | null;
  mean_p_up: number | null;
  realized_up_rate: number | null;
}

// ==========================================
// P1.9.24 historical readiness & feed health
// ==========================================

export interface SymbolReadiness {
  symbol: string;
  is_active: boolean;
  vendor_bars: number;
  quarantined: number;
  research_eligible: number;
  exchange_bars: number;
  exchange_first: string | null;
  exchange_last: string | null;
  nominal_match: number;
  vendor_adjusted: number;
  mismatch: number;
  missing_vendor_bars: number;
  missing_exchange_bars: number;
  missing_price_changing_actions: number;
  missing_other_actions: number;
  events_confirmed_pending_review: number;
  parser_fragments: number;
  feed_unavailable: boolean;
  canonical_events: number;
  verified_events: number;
  status: 'READY' | 'BLOCKED';
  blockers: string[];
}

export interface HistoricalReadiness {
  symbols: SymbolReadiness[];
  latest_reconciliation_run: { id: number; run_at: string } | null;
  research_series?: { status: string; built_at: string; series: { bars: number; symbols: number; from: string; to: string; adjustments_total: number; adjustments_confirmed: number; unexplained_breaks: unknown[] } } | null;
  next_day_prediction_research: {
    status: 'DATA_INSUFFICIENT' | 'READY_FOR_RESEARCH';
    required_eligible_sessions: number;
    required_symbols: number;
    symbols_meeting_requirement: number;
    note: string;
  };
  quarantine_release_floor: string;
  policy_note: string;
}

export interface IntradayFeedHealth {
  provider: string;
  poll_interval_minutes: string;
  sessions: Array<{ date: string; symbols: number; valid_bars: number; suspect_bars: number; coverage_pct_of_375_minutes: number }>;
  structural_gaps: Array<{ minute_ist: string; missing_pct_of_symbol_days: number }>;
  latency: {
    definition: string;
    bar_receipt: LatencyStats;
    poll_freshness: LatencyStats;
  };
  recovery: string;
  truedata: { configured: boolean; status: string };
}

export interface LatencyStats {
  status: 'MEASURED' | 'MEASURED_LOW_SAMPLE' | 'NOT_MEASURED' | 'ERROR';
  observations: number;
  median_seconds: number | null;
  p95_seconds: number | null;
  p99_seconds: number | null;
  max_seconds: number | null;
}

export interface OnboardStatus {
  state: 'RUNNING' | 'DONE' | 'FAILED';
  steps: { name: string; ok: boolean; detail: string }[];
  updated_at?: string;
}

export interface AddStockResponse {
  symbol: string;
  name?: string;
  already_tracked?: boolean;
  message?: string;
  status?: OnboardStatus;
}

export interface OnboardStatusResponse {
  symbol: string;
  user_added: boolean;
  status: OnboardStatus | null;
}

export interface BcaInterval {
  estimate: number;
  low: number;
  high: number;
  confidence: number;
  n_boot: number;
}

export interface PerformanceReport {
  n_trades: number;
  reliability: 'INSUFFICIENT' | 'LOW' | 'ADEQUATE';
  message?: string;
  verdict?: string;
  total_net_pnl?: number;
  total_fees?: number;
  win_rate_pct?: number;
  avg_win_pct?: number | null;
  avg_loss_pct?: number | null;
  payoff_ratio?: number | null;
  profit_factor?: number | null;
  profit_factor_note?: string | null;
  cost_drag_pct_of_gross_profit?: number | null;
  avg_holding_days?: number | null;
  expectancy_pct?: BcaInterval | null;
  profit_factor_ci?: BcaInterval | null;
  max_drawdown?: { actual: number; median: number; p95: number; p99: number } | null;
  ulcer_index?: number;
  excess_vs_benchmark_pct?: BcaInterval | null;
  benchmark_beaten_pct_of_trades?: number;
  assumptions: { trading_days_per_year: number; risk_free_rate_pct: number; risk_free_note: string };
}

export interface LivePosition {
  price: number;
  as_of: string;
  status: 'IN_ENTRY_ZONE' | 'ABOVE_ENTRY_ZONE' | 'BELOW_ENTRY_ZONE' | 'INVALIDATED' | 'TARGET_1_REACHED' | 'TARGET_2_REACHED';
  message: string;
  from_zone_pct: number;
  reward_risk_now: number | null;
  planned_reward_risk: number;
  change_since_scan_pct: number;
}

export interface TrackedBreakout {
  symbol: string;
  signal_date: string;
  origin: 'LIVE' | 'HISTORY_REPLAY';
  signal_close: number;
  entry_low: number;
  entry_high: number;
  stop_level: number;
  target_1: number;
  target_2: number;
  structure_score: number | null;
  rvol: number | null;
  shariah_status: string | null;
  status: string;
  verdict: 'PENDING' | 'WORKED' | 'FAILED' | 'NO_FOLLOW_THROUGH' | 'NOT_TRADEABLE';
  entry_at: string | null;
  entry_price: number | null;
  exit_at: string | null;
  exit_price: number | null;
  return_pct: number | null;
  best_pct: number | null;
  worst_pct: number | null;
  sessions: number;
  closed_below_at: string | null;
  last_price: number | null;
  last_price_at: string | null;
  nifty_return_pct: number | null;
}

export interface EvidenceStats {
  trades: number;
  win_rate_pct?: number;
  target_hit_pct?: number;
  stopped_pct?: number;
  time_exit_pct?: number;
  avg_net_return_pct?: number;
  median_net_return_pct?: number;
  avg_gross_return_pct?: number;
  profit_factor?: number | null;
  avg_best_move_pct?: number;
  avg_worst_move_pct?: number;
  avg_bars_held?: number;
  avg_days?: number;
  net_return_ci?: BcaInterval | null;
  avg_excess_vs_nifty_pct?: number | null;
  excess_vs_nifty_ci?: BcaInterval | null;
  excess_ci?: BcaInterval | null;
}

export interface BreakoutMonitor {
  signals: TrackedBreakout[];
  scorecard: {
    tracked: number; open: number; worked: number; failed: number; no_follow_through: number; not_tradeable: number;
    avg_return_pct_closed: number | null; avg_excess_vs_nifty_pct_closed: number | null; worked_rate_pct: number | null;
    repeat_signals_excluded?: number; live?: ScoreGroup; history_replay?: ScoreGroup; all?: ScoreGroup; note?: string;
  };
  history: { overall: EvidenceStats; by_period: { DISCOVERY: EvidenceStats; HOLDOUT: EvidenceStats }; shariah_pass_only: { overall: EvidenceStats; holdout: EvidenceStats } } | null;
  history_run_at: string | null;
}

export interface ResearchPlanRow {
  plan: string;
  entry: string;
  market: string;
  text: string;
  discovery: EvidenceStats;
  holdout: EvidenceStats;
}

export interface ResearchRun {
  available: boolean;
  run_at?: string;
  result?: {
    version: string;
    symbols_tested: number;
    excluded_short_history: string[];
    series: {
      series_version: string; symbols: number; bars: number; from: string; to: string;
      adjustments_total: number; adjustments_confirmed: number; adjustments_flagged: unknown[]; unexplained_breaks: unknown[];
    };
    adjustments: { symbol: string; ex_date: string; factor: number; observed_step: number | null; subjects: string; basis: string; status: string }[];
    rules_of_the_test: { entry: string; stop: string; target: string; time_limit_sessions: number; costs: string; discovery_period_ends: string; holdout_starts: string };
    overall: EvidenceStats;
    by_period: { DISCOVERY: EvidenceStats; HOLDOUT: EvidenceStats };
    shariah_pass_only: { overall: EvidenceStats; holdout: EvidenceStats };
    rules: { rule: string; description: string; verdict: 'ADOPTED' | 'REJECTED'; why: string; discovery: EvidenceStats; holdout: EvidenceStats }[];
    plans: { tested: number; holdout_verdict: string; selected_on_discovery: ResearchPlanRow | null; table: ResearchPlanRow[]; multiple_testing_note: string };
    plans_shariah_pass: { tested: number; holdout_verdict: string; selected_on_discovery: ResearchPlanRow | null; table: ResearchPlanRow[]; multiple_testing_note: string };
    recent_trades: { symbol: string; signal_date: string; entry_date: string; exit_date: string; entry_price: number; exit_price: number; exit_reason: string; net_return_pct: number; excess_vs_nifty_pct: number | null; shariah: string }[];
    limitations: string[];
  };
}

export interface IpoCategoryBook {
  category: 'QIB' | 'NII' | 'BHNI' | 'SHNI' | 'RETAIL' | 'TOTAL';
  label: string;
  shares_offered: number | null;
  shares_bid: number | null;
  times: number | null;
  basis: 'NSE_CONSOLIDATED' | 'NSE_ONLY' | 'DERIVED';
}

export interface IpoDetails {
  official: {
    categories: IpoCategoryBook[];
    structure: { fresh_shares: number | null; ofs_shares: number | null; fresh_amount_cr: number | null; ofs_amount_cr: number | null; anchor_shares: number | null; market_maker_shares: number | null };
    issue_size_text?: string; lot_size: number | null; face_value?: string; issue_type?: string; lead_managers?: string; registrar?: string;
    sponsor_bank?: string; prospectus_url?: string; anchor_report_url?: string; industry?: string; book_updated_at?: string;
  } | null;
  unofficial: {
    url: string; listing_at?: string; lot_tiers?: { application: string; lots: string; shares: string; amount: string }[];
    reservations?: { category: string; shares: number | null; percent: number | null }[];
    anchor?: Record<string, string>; timeline?: Record<string, string>; shareholding?: Record<string, string>[];
    objects?: Record<string, string>[]; financials?: Record<string, string>[]; kpis?: Record<string, string>;
  } | null;
  categories: IpoCategoryBook[] | null;
  fresh_issue_cr: number | null;
  ofs_cr: number | null;
  issue_size_cr: number | null;
  lot_size: number | null;
  min_investment: number | null;
  listing_at?: string;
  gmp_source?: string;
  gmp_source_url?: string;
}

// ---- Learning loop (P1.9.28) ----
export interface StrategyVersion {
  id: number; name: string; origin: 'FROZEN_CONTROL' | 'LEARNER_SPACE'; params: { plan: string; entry: string; market: string };
  status: string; status_reason: string; actor: string; status_at: string | null;
}
export interface LearningGate { pass: boolean; detail: string; interval?: { estimate?: number; low: number; high: number } | null; recent_choices?: string[] }
export interface LearningBlock {
  origin: string; completed: boolean; chosen: string; basis: string;
  policy_trades: number; control_trades: number; policy_mean_net_pct: number | null; control_mean_net_pct: number | null;
}
export interface PnlAudit {
  label: string; trades: number; capital_inr?: number; notional_per_trade_inr?: number; gross_pnl_inr?: number; total_costs_inr?: number; net_pnl_inr?: number;
  ending_equity_inr?: number; reconciled_to_ledger?: boolean; reconciliation_difference_inr?: number; peak_concurrent_positions?: number;
  max_drawdown?: { max_drawdown_pct: number; peak_date: string; trough_date: string } | null; capital_warning?: string | null; cost_model?: string;
}
export interface LearningRunResult {
  spec: string; space: string; data_through: string; candidates_tried: number; current_selection: string; selection_basis: string;
  walk_forward_blocks: LearningBlock[]; gates: Record<string, LearningGate>; gates_passed: boolean;
  policy_oos: { trades: number; mean_net_pct: number | null; ci: { low: number; high: number } | null };
  control_oos: { trades: number; mean_net_pct: number | null };
  difference_ci: { estimate: number; low: number; high: number } | null;
  probation: { forward_trades: number; judged: boolean; failed: boolean; active_mean_net_pct: number | null; control_mean_net_pct: number | null; rule: string } | null;
  pnl_audit: PnlAudit[]; multiple_testing_note: string; limitations: string[];
}
export interface LessonTag { tag: string; n: number; worked: number; failed: number }
export interface Lesson { symbol: string; signal_date: string; origin: string; verdict: string; return_pct: number | null; lesson: string; tags: string[] }
export interface LearningSummary {
  versions: StrategyVersion[]; active: StrategyVersion | null; control: StrategyVersion | null;
  last_run: LearningRunResult | null; last_run_id: number | null; last_run_at: string | null; last_decision: string | null;
  runs: { id: number; data_through: string; decision: string; created_at: string }[];
  lesson_tags: LessonTag[]; recent_lessons: Lesson[]; lessons_total: number;
  recent_status_events: { id: number; version: string; status: string; reason: string; actor: string; at: string }[];
}

// ---- Breakouts, news, events, automatic paper trading (P1.9.29) ----
export interface BreakoutReading {
  symbol: string; stage: string; current_price: number; resistance_level: number; support_level: number; distance_to_resistance_pct: number;
  rvol: number; atr: number; consolidation_bars: number; entry_zone: number[]; invalidation_level: number; target_1: number; target_2: number;
  risk_reward_ratio: number; structure_score: number | null; score_breakdown: Record<string, number>; shariah_status: string;
  reasons: string[]; invalidation_conditions: string[]; market_regime?: string; liquidity_adt_cr?: number; sector?: string;
  live?: { price: number; status: string; message: string; as_of: string } | null;
}
export interface WatchFlag {
  symbol: string; flagged_date: string; origin: 'LIVE' | 'HISTORY_REPLAY'; flagged_close: number; resistance_level: number; stop_level: number;
  entry_low: number | null; entry_high: number | null; target_1: number | null; target_2: number | null; structure_score: number | null; rvol: number | null;
  reasons: string[]; score_breakdown: Record<string, number>; shariah_status: string | null;
  status: 'WATCHING' | 'BROKE_OUT' | 'INVALIDATED' | 'EXPIRED'; sessions_elapsed: number; last_close: number | null; broke_out_session: string | null;
  sessions_to_breakout: number | null; ret_5d_pct: number | null; ret_10d_pct: number | null; max_gain_pct: number | null; max_drawdown_pct: number | null;
  counts_in_stats: boolean; distance_to_resistance_pct: number | null;
}
export interface TrackedSignal extends TrackedBreakout {
  reasons: string[]; resistance_level: number | null; atr: number | null; score_breakdown: Record<string, number> | null;
  counts_in_stats: boolean; excluded_reason: string | null; net_return_pct: number | null;
}
export interface ScoreGroup { closed: number; wins: number; avg_gross_pct: number | null; avg_net_pct: number | null; net_win_rate_pct: number | null; total_net_inr_on_1l_each: number }
export interface WatchScorecard { watching: number; broke_out: number; invalidated: number; expired: number; hit_rate_pct: number | null; avg_10d_move_pct: number | null; repeat_flags_excluded: number; note: string }
export interface BreakoutsOverview {
  confirmed: { signals: TrackedSignal[]; scorecard: BreakoutMonitor['scorecard'] & { repeat_signals_excluded: number; live: ScoreGroup; history_replay: ScoreGroup; all: ScoreGroup; note: string } };
  upcoming: { flags: WatchFlag[]; scorecard_all?: WatchScorecard; scorecard_live?: WatchScorecard; scorecard_history_replay?: WatchScorecard };
  candidate_lift?: { result: { base_rate_next5_pct: number | null; candidate_flags: number; candidate_hit_next5_pct: number | null; candidate_hit_next10_pct: number | null; candidate_mean_10d_move_pct: number | null; note: string; data_through: string }; run_at: string };
  today: { available: boolean; error?: string; data_as_of?: string | null; data_quality?: string; computed_at?: string; basis?: string; total_screened?: number; stage_counts?: Record<string, number>;
    confirmed?: BreakoutReading[]; upcoming?: BreakoutReading[]; excluded?: { symbol: string; reason: string }[] };
  auto_paper?: { portfolio_id: number; label: string; closed_count: number; wins: number; net_pnl_closed: number; return_on_capital_pct: number; open_positions: number; reconciled: boolean; processed_through: string }[];
}
export interface StockTest {
  verdict: 'INSUFFICIENT_DATA' | 'POSITIVE_AFTER_COSTS' | 'NEGATIVE_AFTER_COSTS' | 'NO_EDGE_DEMONSTRATED'; verdict_text: string; plan: string; series_through: string;
  confirmed_breakouts: { trades: number; wins: number; win_rate_pct: number | null; mean_net_pct: number | null; net_ci95: { low: number; high: number } | null; total_net_inr_on_1L_each?: number;
    by_period: Record<string, { trades: number; mean_net_pct: number | null }>; last_trades: { entry: string; exit: string; why: string; net_pct: number }[] };
  upcoming_flags: { resolved_flags: number; broke_out: number; hit_rate_pct: number | null; mean_10d_move_pct: number | null; note: string };
}
export interface StockBreakouts {
  symbol: string; name: string; today?: { available: boolean; data_as_of?: string | null; reading?: BreakoutReading; reason?: string };
  signals: TrackedSignal[]; flags: WatchFlag[]; test: { result: StockTest; as_of: string } | null;
}
export interface FeedStatus { last_success: string | null; last_error: string | null }
export interface StockNews {
  symbol: string; name: string; headlines: { title: string; publisher: string | null; url: string; source: string; published_at: string | null }[];
  filings: { announced_at: string; category: string | null; subject: string; attachment_url: string | null }[];
  status: Record<string, FeedStatus>; sources: { headlines: string; filings: string };
}
export interface StockEvents {
  symbol: string; name: string; upcoming: StockEvent[]; recent: StockEvent[]; status: Record<string, FeedStatus>; source: string;
}
export interface StockEvent { kind: 'BOARD_MEETING' | 'CORPORATE_ACTION'; event_date: string; title: string; detail: string | null; source: string }
export interface AutoPaperPortfolio {
  portfolio_id: number; label: string; plan: Record<string, string>; initial_capital: number; cash: number; processed_through: string;
  open_positions: { symbol: string; shares: number; avg_cost: number; entry_date: string; signal_date: string }[];
  closed_trades: { symbol: string; origin: string; signal_date: string; entry_date: string; exit_date: string; shares: number; entry_price: number; exit_price: number; fees: number; net_pnl: number; net_pct: number; exit_reason: string }[];
  skipped: { symbol: string; signal_date: string; leg: string; reason: string }[];
  closed_count: number; wins: number; net_pnl_closed: number; total_fees: number; return_on_capital_pct: number; max_drawdown_realized_pct: number;
  expected_cash_from_trades: number; cash_reconciles_to_ledger: boolean; note: string;
}
