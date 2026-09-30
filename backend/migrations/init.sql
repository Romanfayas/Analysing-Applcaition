-- ==================================================
-- Halal Equity & IPO Research Platform
-- Database Initialization & Schema Migration
-- ==================================================
-- This file runs on first container start via docker-entrypoint-initdb.d
-- All timestamps stored in UTC. Display as IST (UTC+5:30) in frontend.
-- ==================================================

-- Enable extensions
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ==================================================
-- ENUMS
-- ==================================================

CREATE TYPE data_quality_status AS ENUM (
    'VALID', 'MISSING', 'STALE', 'DUPLICATE', 'SUSPECT', 'CORRECTED'
);

CREATE TYPE shariah_status AS ENUM (
    'PASS', 'FAIL', 'REVIEW_REQUIRED', 'DATA_INSUFFICIENT'
);

CREATE TYPE signal_type AS ENUM (
    'STRONG_BUY', 'BUY', 'WATCH', 'AVOID', 'STRONG_AVOID'
);

CREATE TYPE ipo_decision AS ENUM (
    'APPLY', 'WATCH', 'AVOID', 'REVIEW_REQUIRED'
);

CREATE TYPE order_side AS ENUM ('BUY', 'SELL');

CREATE TYPE order_status AS ENUM (
    'PENDING', 'FILLED', 'PARTIAL', 'CANCELLED', 'REJECTED'
);

CREATE TYPE alert_channel AS ENUM ('TELEGRAM', 'EMAIL', 'WEB_PUSH');

CREATE TYPE alert_type AS ENUM (
    'NEW_BUY_SIGNAL', 'SIGNAL_CHANGED', 'STOP_LOSS_REACHED',
    'TARGET_REACHED', 'IPO_OPENS', 'IPO_GMP_CHANGED',
    'IPO_SUBSCRIPTION_CHANGED', 'IPO_DECISION_CHANGED',
    'SHARIAH_STATUS_CHANGED', 'DATA_QUALITY_FAILURE'
);

CREATE TYPE financial_period_type AS ENUM (
    'ANNUAL', 'QUARTERLY', 'HALF_YEARLY', 'TTM'
);

CREATE TYPE corporate_action_type AS ENUM (
    'SPLIT', 'BONUS', 'DIVIDEND', 'RIGHTS', 'MERGER', 'DEMERGER', 'BUYBACK'
);

-- ==================================================
-- CORE: Users & Auth
-- ==================================================

CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_sessions_user ON user_sessions(user_id);
CREATE INDEX idx_user_sessions_expires ON user_sessions(expires_at);

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID REFERENCES users(id),
    action VARCHAR(100) NOT NULL,
    resource VARCHAR(100),
    resource_id VARCHAR(255),
    details JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_user ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_created ON audit_logs(created_at DESC);

-- ==================================================
-- MARKET DATA: Symbols & Exchanges
-- ==================================================

CREATE TABLE symbols (
    id SERIAL PRIMARY KEY,
    symbol VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    exchange VARCHAR(20) NOT NULL DEFAULT 'NSE',  -- NSE, BSE
    isin VARCHAR(12),
    sector VARCHAR(100),
    industry VARCHAR(100),
    market_cap_category VARCHAR(20),  -- LARGE, MID, SMALL, MICRO
    is_active BOOLEAN NOT NULL DEFAULT true,
    listing_date DATE,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(symbol, exchange)
);

CREATE INDEX idx_symbols_symbol ON symbols(symbol);
CREATE INDEX idx_symbols_sector ON symbols(sector);
CREATE INDEX idx_symbols_isin ON symbols(isin);

-- ==================================================
-- MARKET DATA: OHLCV (TimescaleDB Hypertables)
-- ==================================================

CREATE TABLE ohlcv_daily (
    symbol_id INT NOT NULL REFERENCES symbols(id),
    timestamp TIMESTAMPTZ NOT NULL,
    open NUMERIC(18, 4) NOT NULL,
    high NUMERIC(18, 4) NOT NULL,
    low NUMERIC(18, 4) NOT NULL,
    close NUMERIC(18, 4) NOT NULL,
    volume BIGINT NOT NULL DEFAULT 0,
    adjusted_close NUMERIC(18, 4),
    source VARCHAR(50) NOT NULL,
    retrieved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quality_status data_quality_status NOT NULL DEFAULT 'VALID',
    PRIMARY KEY (symbol_id, timestamp)
);

SELECT create_hypertable('ohlcv_daily', 'timestamp',
    chunk_time_interval => INTERVAL '1 month',
    if_not_exists => TRUE
);

CREATE INDEX idx_ohlcv_daily_symbol ON ohlcv_daily(symbol_id, timestamp DESC);

CREATE TABLE ohlcv_intraday (
    symbol_id INT NOT NULL REFERENCES symbols(id),
    timestamp TIMESTAMPTZ NOT NULL,
    interval_minutes INT NOT NULL DEFAULT 5,
    open NUMERIC(18, 4) NOT NULL,
    high NUMERIC(18, 4) NOT NULL,
    low NUMERIC(18, 4) NOT NULL,
    close NUMERIC(18, 4) NOT NULL,
    volume BIGINT NOT NULL DEFAULT 0,
    source VARCHAR(50) NOT NULL,
    retrieved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quality_status data_quality_status NOT NULL DEFAULT 'VALID',
    PRIMARY KEY (symbol_id, timestamp, interval_minutes)
);

SELECT create_hypertable('ohlcv_intraday', 'timestamp',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

-- ==================================================
-- MARKET DATA: Corporate Actions
-- ==================================================

CREATE TABLE corporate_actions (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    action_type corporate_action_type NOT NULL,
    ex_date DATE NOT NULL,
    record_date DATE,
    description TEXT,
    -- For splits: old_ratio/new_ratio (e.g., 1:5 = old=1, new=5)
    old_ratio NUMERIC(10, 4),
    new_ratio NUMERIC(10, 4),
    -- For dividends
    dividend_amount NUMERIC(18, 4),
    dividend_percentage NUMERIC(10, 4),
    source VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_corp_actions_symbol ON corporate_actions(symbol_id, ex_date DESC);

-- ==================================================
-- MARKET DATA: Data Quality Logs
-- Logs data anomalies detected by the Ingestion Worker.
-- ==================================================

CREATE TABLE IF NOT EXISTS data_quality_logs (
    id SERIAL PRIMARY KEY,
    symbol_id INT REFERENCES symbols(id) ON DELETE CASCADE,
    check_type VARCHAR(50) NOT NULL, -- 'missing_days', 'price_spike', 'stale_price'
    severity VARCHAR(20) NOT NULL, -- 'WARNING', 'CRITICAL'
    message TEXT NOT NULL,
    recorded_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_dq_logs_symbol ON data_quality_logs(symbol_id, recorded_at DESC);
CREATE INDEX idx_dq_logs_severity ON data_quality_logs(severity);

-- =========================================================================
-- PHASE 3: BACKTESTING & VALIDATION
-- =========================================================================

-- Represents a versioned configuration of strategy parameters and costs
CREATE TABLE IF NOT EXISTS backtest_configurations (
    id SERIAL PRIMARY KEY,
    strategy_version VARCHAR(50) NOT NULL,
    parameters JSONB NOT NULL,
    cost_model_version VARCHAR(50) NOT NULL,
    slippage_model JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(strategy_version, parameters, cost_model_version, slippage_model)
);

-- Stores the high-level run results
CREATE TABLE IF NOT EXISTS backtest_runs (
    id SERIAL PRIMARY KEY,
    symbol_id INT REFERENCES symbols(id) ON DELETE CASCADE,
    configuration_id INT REFERENCES backtest_configurations(id) ON DELETE CASCADE,
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    initial_capital NUMERIC NOT NULL,
    final_capital NUMERIC NOT NULL,
    cagr NUMERIC,
    max_drawdown NUMERIC,
    sharpe_ratio NUMERIC,
    sortino_ratio NUMERIC,
    win_rate NUMERIC,
    total_trades INT,
    survivorship_bias_warning BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- ==================================================
-- FUNDAMENTAL DATA: Financial Statements
-- ==================================================

CREATE TABLE financial_statements (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    period_type financial_period_type NOT NULL,
    period_end DATE NOT NULL,
    publication_date DATE NOT NULL,
    pit_mode VARCHAR(20) DEFAULT 'PIT_ESTIMATED',
    fiscal_year INT,
    -- Income Statement
    revenue NUMERIC(20, 4),
    revenue_growth NUMERIC(10, 4),
    cost_of_goods_sold NUMERIC(20, 4),
    gross_profit NUMERIC(20, 4),
    operating_expenses NUMERIC(20, 4),
    ebitda NUMERIC(20, 4),
    ebitda_margin NUMERIC(10, 4),
    depreciation NUMERIC(20, 4),
    ebit NUMERIC(20, 4),
    interest_expense NUMERIC(20, 4),
    interest_income NUMERIC(20, 4),
    other_income NUMERIC(20, 4),
    exceptional_items NUMERIC(20, 4),
    profit_before_tax NUMERIC(20, 4),
    tax_expense NUMERIC(20, 4),
    pat NUMERIC(20, 4),  -- Profit After Tax
    pat_growth NUMERIC(10, 4),
    -- Balance Sheet
    total_assets NUMERIC(20, 4),
    total_liabilities NUMERIC(20, 4),
    total_equity NUMERIC(20, 4),
    total_debt NUMERIC(20, 4),
    long_term_debt NUMERIC(20, 4),
    short_term_debt NUMERIC(20, 4),
    cash_and_equivalents NUMERIC(20, 4),
    total_receivables NUMERIC(20, 4),
    inventory NUMERIC(20, 4),
    -- Cash Flow
    operating_cash_flow NUMERIC(20, 4),
    investing_cash_flow NUMERIC(20, 4),
    financing_cash_flow NUMERIC(20, 4),
    capex NUMERIC(20, 4),
    free_cash_flow NUMERIC(20, 4),
    -- Per Share
    eps NUMERIC(18, 4),
    eps_growth NUMERIC(10, 4),
    book_value_per_share NUMERIC(18, 4),
    dividend_per_share NUMERIC(18, 4),
    -- Shares
    shares_outstanding BIGINT,
    -- Meta
    source VARCHAR(50) NOT NULL,
    retrieved_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    quality_status data_quality_status NOT NULL DEFAULT 'VALID',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(symbol_id, period_type, period_end)
);

CREATE INDEX idx_fin_stmt_symbol ON financial_statements(symbol_id, period_end DESC);

-- ==================================================
-- FUNDAMENTAL DATA: Calculated Ratios
-- ==================================================

CREATE TABLE financial_ratios (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    period_end DATE NOT NULL,
    period_type financial_period_type NOT NULL,
    -- Profitability
    roe NUMERIC(10, 4),
    roce NUMERIC(10, 4),
    gross_margin NUMERIC(10, 4),
    operating_margin NUMERIC(10, 4),
    net_margin NUMERIC(10, 4),
    -- Leverage
    debt_to_equity NUMERIC(10, 4),
    interest_coverage NUMERIC(10, 4),
    current_ratio NUMERIC(10, 4),
    -- Valuation
    pe_ratio NUMERIC(10, 4),
    pb_ratio NUMERIC(10, 4),
    peg_ratio NUMERIC(10, 4),
    ev_to_ebitda NUMERIC(10, 4),
    market_cap_to_sales NUMERIC(10, 4),
    dividend_yield NUMERIC(10, 4),
    -- Size
    market_cap NUMERIC(20, 4),
    enterprise_value NUMERIC(20, 4),
    -- Quality flags
    has_exceptional_income BOOLEAN DEFAULT false,
    has_one_time_gains BOOLEAN DEFAULT false,
    deteriorating_cash_flow BOOLEAN DEFAULT false,
    rising_debt BOOLEAN DEFAULT false,
    margin_compression BOOLEAN DEFAULT false,
    -- Meta
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(symbol_id, period_type, period_end)
);

CREATE INDEX idx_fin_ratios_symbol ON financial_ratios(symbol_id, period_end DESC);

-- ==================================================
-- FUNDAMENTAL DATA: Shareholding Patterns
-- ==================================================

CREATE TABLE shareholding_patterns (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    period_end DATE NOT NULL,
    promoter_holding NUMERIC(8, 4),
    promoter_pledge NUMERIC(8, 4),
    fii_holding NUMERIC(8, 4),
    dii_holding NUMERIC(8, 4),
    public_holding NUMERIC(8, 4),
    promoter_pledge_change NUMERIC(8, 4),  -- vs previous period
    source VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(symbol_id, period_end)
);

-- ==================================================
-- FUNDAMENTAL DATA: Anomaly Detection
-- ==================================================

CREATE TABLE fundamental_anomalies (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    period_end DATE NOT NULL,
    anomaly_type VARCHAR(100) NOT NULL,
    severity VARCHAR(20) NOT NULL,  -- LOW, MEDIUM, HIGH, CRITICAL
    description TEXT NOT NULL,
    metric_name VARCHAR(100),
    metric_value NUMERIC(20, 4),
    expected_range_low NUMERIC(20, 4),
    expected_range_high NUMERIC(20, 4),
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_anomalies_symbol ON fundamental_anomalies(symbol_id, detected_at DESC);

-- ==================================================
-- TECHNICAL ANALYSIS: Indicators
-- ==================================================

CREATE TABLE technical_indicators (
    symbol_id INT NOT NULL REFERENCES symbols(id),
    timestamp TIMESTAMPTZ NOT NULL,
    indicator_name VARCHAR(50) NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}',  -- e.g., {"period": 14}
    value NUMERIC(18, 6) NOT NULL,
    signal VARCHAR(20),  -- BULLISH, BEARISH, NEUTRAL
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (symbol_id, timestamp, indicator_name, parameters)
);

SELECT create_hypertable('technical_indicators', 'timestamp',
    chunk_time_interval => INTERVAL '1 month',
    if_not_exists => TRUE
);

-- ==================================================
-- TECHNICAL ANALYSIS: Candlestick Patterns
-- ==================================================

CREATE TABLE candlestick_patterns (
    id BIGSERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    timestamp TIMESTAMPTZ NOT NULL,
    pattern_name VARCHAR(50) NOT NULL,
    pattern_type VARCHAR(20) NOT NULL,  -- BULLISH, BEARISH, NEUTRAL
    confidence NUMERIC(5, 2) NOT NULL,  -- 0-100
    -- Context evaluation
    trend_context VARCHAR(20),       -- UPTREND, DOWNTREND, SIDEWAYS
    support_resistance VARCHAR(20),  -- AT_SUPPORT, AT_RESISTANCE, NONE
    volume_context VARCHAR(20),      -- HIGH, NORMAL, LOW
    atr_context NUMERIC(18, 4),
    rsi_context NUMERIC(8, 4),
    macd_context VARCHAR(20),        -- BULLISH, BEARISH, NEUTRAL
    market_regime VARCHAR(20),       -- TRENDING, RANGING, VOLATILE
    -- Result
    signal_contribution NUMERIC(5, 2),  -- contribution to overall signal
    context_json JSONB,  -- full context details
    detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_candle_patterns_symbol ON candlestick_patterns(symbol_id, timestamp DESC);

-- ==================================================
-- TECHNICAL ANALYSIS: Support & Resistance
-- ==================================================

CREATE TABLE support_resistance_levels (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    level_type VARCHAR(20) NOT NULL,  -- SUPPORT, RESISTANCE
    price NUMERIC(18, 4) NOT NULL,
    strength INT NOT NULL DEFAULT 1,  -- number of touches
    timeframe VARCHAR(20) NOT NULL,   -- DAILY, WEEKLY, MONTHLY
    is_active BOOLEAN NOT NULL DEFAULT true,
    first_touch TIMESTAMPTZ,
    last_touch TIMESTAMPTZ,
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_sr_levels_symbol ON support_resistance_levels(symbol_id);

-- ==================================================
-- SHARIAH: Rule Sets & Screenings
-- ==================================================

CREATE TABLE shariah_rule_sets (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE shariah_rule_versions (
    id SERIAL PRIMARY KEY,
    rule_set_id INT NOT NULL REFERENCES shariah_rule_sets(id),
    version INT NOT NULL,
    rules JSONB NOT NULL,
    -- Example rules JSON:
    -- {
    --   "debt_to_market_cap_max": 0.33,
    --   "interest_income_to_revenue_max": 0.05,
    --   "cash_and_deposits_to_market_cap_max": 0.33,
    --   "receivables_to_market_cap_max": 0.49,
    --   "prohibited_business_activities": ["alcohol", "tobacco", "gambling", "conventional_finance", "pork"],
    --   "prohibited_revenue_threshold": 0.05
    -- }
    effective_from DATE NOT NULL,
    effective_to DATE,
    notes TEXT,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(rule_set_id, version)
);

CREATE TABLE shariah_screenings (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    rule_set_id INT NOT NULL REFERENCES shariah_rule_sets(id),
    rule_version_id INT NOT NULL REFERENCES shariah_rule_versions(id),
    screening_date DATE NOT NULL,
    status shariah_status NOT NULL,
    -- Individual ratio results
    debt_ratio NUMERIC(10, 4),
    debt_ratio_pass BOOLEAN,
    interest_income_ratio NUMERIC(10, 4),
    interest_income_pass BOOLEAN,
    cash_deposit_ratio NUMERIC(10, 4),
    cash_deposit_pass BOOLEAN,
    receivables_ratio NUMERIC(10, 4),
    receivables_pass BOOLEAN,
    business_activity_pass BOOLEAN,
    business_activity_notes TEXT,
    -- Purification
    purification_per_share NUMERIC(18, 6),
    purification_method VARCHAR(50),
    -- Full results
    details JSONB,
    source VARCHAR(50),
    screened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(symbol_id, rule_set_id, rule_version_id, screening_date)
);

CREATE INDEX idx_shariah_screenings_symbol ON shariah_screenings(symbol_id, screening_date DESC);
CREATE INDEX idx_shariah_screenings_status ON shariah_screenings(status);

-- Insert default Shariah rule set
INSERT INTO shariah_rule_sets (name, description) VALUES
    ('AAOIFI Standard', 'Accounting and Auditing Organization for Islamic Financial Institutions standard screening criteria');

INSERT INTO shariah_rule_versions (rule_set_id, version, rules, effective_from, notes) VALUES
    (1, 1, '{
        "debt_to_market_cap_max": 0.33,
        "interest_income_to_revenue_max": 0.05,
        "cash_and_interest_bearing_deposits_to_market_cap_max": 0.33,
        "receivables_to_market_cap_max": 0.49,
        "prohibited_business_activities": [
            "alcohol", "tobacco", "gambling", "conventional_finance",
            "conventional_insurance", "pork", "weapons",
            "adult_entertainment", "interest_based_lending"
        ],
        "prohibited_revenue_threshold": 0.05,
        "purification_method": "dividend_based"
    }', '2024-01-01', 'Initial AAOIFI-based screening criteria v1');

-- ==================================================
-- SIGNALS: Stock Signals
-- ==================================================

CREATE TABLE signal_weight_configs (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT true,
    weights JSONB NOT NULL,
    -- Example:
    -- {
    --   "fundamental": 0.30,
    --   "technical": 0.20,
    --   "candlestick": 0.10,
    --   "momentum": 0.10,
    --   "volume": 0.05,
    --   "valuation": 0.10,
    --   "market_regime": 0.05,
    --   "risk": 0.05,
    --   "quality": 0.05
    -- }
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert default weight config
INSERT INTO signal_weight_configs (name, weights) VALUES
    ('Default', '{
        "fundamental": 0.30,
        "technical": 0.20,
        "candlestick": 0.10,
        "momentum": 0.10,
        "volume": 0.05,
        "valuation": 0.10,
        "market_regime": 0.05,
        "risk": 0.05,
        "quality": 0.05
    }');

CREATE TABLE stock_signals (
    id BIGSERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    signal signal_type NOT NULL,
    overall_score NUMERIC(5, 2) NOT NULL,
    -- Component scores
    fundamental_score NUMERIC(5, 2),
    technical_score NUMERIC(5, 2),
    candlestick_score NUMERIC(5, 2),
    momentum_score NUMERIC(5, 2),
    volume_score NUMERIC(5, 2),
    valuation_score NUMERIC(5, 2),
    market_regime_score NUMERIC(5, 2),
    risk_score NUMERIC(5, 2),
    quality_score NUMERIC(5, 2),
    -- Shariah
    shariah_status shariah_status NOT NULL,
    shariah_override BOOLEAN NOT NULL DEFAULT false,
    -- Weight config used
    weight_config_id INT REFERENCES signal_weight_configs(id),
    -- Explanation
    explanation TEXT,
    supporting_factors JSONB,
    conflicting_factors JSONB,
    invalidation_factors JSONB,
    -- Meta
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_stock_signals_symbol ON stock_signals(symbol_id, calculated_at DESC);
CREATE INDEX idx_stock_signals_signal ON stock_signals(signal);

-- ==================================================
-- RISK: Calculations
-- ==================================================

CREATE TABLE risk_calculations (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    -- Position sizing
    capital NUMERIC(20, 4) NOT NULL,
    risk_percentage NUMERIC(5, 4) NOT NULL,
    max_risk_amount NUMERIC(20, 4) NOT NULL,
    entry_price NUMERIC(18, 4) NOT NULL,
    stop_loss NUMERIC(18, 4) NOT NULL,
    target_price NUMERIC(18, 4),
    risk_per_share NUMERIC(18, 4) NOT NULL,
    position_size INT NOT NULL,
    position_value NUMERIC(20, 4) NOT NULL,
    risk_reward_ratio NUMERIC(8, 4),
    -- Portfolio context
    portfolio_exposure NUMERIC(5, 4),
    sector_exposure NUMERIC(5, 4),
    -- Risk metrics
    volatility_adjusted_size INT,
    max_drawdown NUMERIC(10, 4),
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_risk_calc_symbol ON risk_calculations(symbol_id, calculated_at DESC);

-- ==================================================
-- IPO: Master Data
-- ==================================================

CREATE TABLE ipos (
    id SERIAL PRIMARY KEY,
    company_name VARCHAR(255) NOT NULL,
    symbol VARCHAR(50),
    exchange VARCHAR(20) DEFAULT 'NSE',
    sector VARCHAR(100),
    industry VARCHAR(100),
    -- Dates
    open_date DATE,
    close_date DATE,
    listing_date DATE,
    -- Pricing
    price_band_low NUMERIC(18, 4),
    price_band_high NUMERIC(18, 4),
    issue_price NUMERIC(18, 4),
    listing_price NUMERIC(18, 4),
    listing_day_close NUMERIC(18, 4),
    -- Size
    lot_size INT,
    min_investment NUMERIC(18, 4),
    issue_size NUMERIC(20, 4),
    fresh_issue NUMERIC(20, 4),
    ofs NUMERIC(20, 4),           -- Offer For Sale
    -- Shares
    pre_ipo_shares BIGINT,
    post_ipo_shares BIGINT,
    -- Holdings
    promoter_holding_pre NUMERIC(8, 4),
    promoter_holding_post NUMERIC(8, 4),
    -- Valuation
    market_cap_at_issue NUMERIC(20, 4),
    -- Status
    status VARCHAR(20) DEFAULT 'UPCOMING',  -- UPCOMING, OPEN, CLOSED, LISTED
    -- Meta
    source VARCHAR(50),
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ipos_status ON ipos(status);
CREATE INDEX idx_ipos_dates ON ipos(open_date, close_date);

-- ==================================================
-- IPO: Financials
-- ==================================================

CREATE TABLE ipo_financials (
    id SERIAL PRIMARY KEY,
    ipo_id INT NOT NULL REFERENCES ipos(id),
    period_end DATE NOT NULL,
    period_type financial_period_type NOT NULL,
    revenue NUMERIC(20, 4),
    revenue_growth NUMERIC(10, 4),
    ebitda NUMERIC(20, 4),
    ebitda_margin NUMERIC(10, 4),
    pat NUMERIC(20, 4),
    pat_growth NUMERIC(10, 4),
    eps NUMERIC(18, 4),
    roe NUMERIC(10, 4),
    roce NUMERIC(10, 4),
    total_debt NUMERIC(20, 4),
    debt_to_equity NUMERIC(10, 4),
    operating_cash_flow NUMERIC(20, 4),
    free_cash_flow NUMERIC(20, 4),
    source VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(ipo_id, period_end, period_type)
);

-- ==================================================
-- IPO: GMP Tracking
-- ==================================================

CREATE TABLE ipo_gmp (
    id BIGSERIAL PRIMARY KEY,
    ipo_id INT NOT NULL REFERENCES ipos(id),
    gmp_value NUMERIC(18, 4) NOT NULL,
    gmp_percentage NUMERIC(10, 4),
    source VARCHAR(100) NOT NULL,
    source_url TEXT,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ipo_gmp ON ipo_gmp(ipo_id, recorded_at DESC);

-- ==================================================
-- IPO: Subscription Data
-- ==================================================

CREATE TABLE ipo_subscriptions (
    id SERIAL PRIMARY KEY,
    ipo_id INT NOT NULL REFERENCES ipos(id),
    day_number INT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,
    qib_subscription NUMERIC(10, 4),
    nii_subscription NUMERIC(10, 4),
    retail_subscription NUMERIC(10, 4),
    employee_subscription NUMERIC(10, 4),
    total_subscription NUMERIC(10, 4),
    source VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ipo_subs ON ipo_subscriptions(ipo_id, recorded_at DESC);

-- ==================================================
-- IPO: Peer Comparisons
-- ==================================================

CREATE TABLE ipo_peer_comparisons (
    id SERIAL PRIMARY KEY,
    ipo_id INT NOT NULL REFERENCES ipos(id),
    peer_symbol_id INT REFERENCES symbols(id),
    peer_name VARCHAR(255),
    pe_ratio NUMERIC(10, 4),
    pb_ratio NUMERIC(10, 4),
    ev_to_ebitda NUMERIC(10, 4),
    market_cap_to_sales NUMERIC(10, 4),
    roe NUMERIC(10, 4),
    pat_growth NUMERIC(10, 4),
    revenue_growth NUMERIC(10, 4),
    source VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ==================================================
-- IPO: Listing Predictions
-- ==================================================

CREATE TABLE ipo_predictions (
    id SERIAL PRIMARY KEY,
    ipo_id INT NOT NULL REFERENCES ipos(id),
    model_name VARCHAR(100) NOT NULL,
    -- GMP, COMPARABLE, SUBSCRIPTION, SENTIMENT, FUNDAMENTAL
    predicted_price NUMERIC(18, 4) NOT NULL,
    predicted_return NUMERIC(10, 4),
    bull_case NUMERIC(18, 4),
    base_case NUMERIC(18, 4),
    bear_case NUMERIC(18, 4),
    confidence_score NUMERIC(5, 2),
    -- Combined prediction
    predicted_range_low NUMERIC(18, 4),
    predicted_range_high NUMERIC(18, 4),
    predicted_midpoint NUMERIC(18, 4),
    factors JSONB,
    predicted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ipo_predictions ON ipo_predictions(ipo_id, predicted_at DESC);

-- ==================================================
-- IPO: Decision Engine
-- ==================================================

CREATE TABLE ipo_decisions (
    id SERIAL PRIMARY KEY,
    ipo_id INT NOT NULL REFERENCES ipos(id),
    decision ipo_decision NOT NULL,
    overall_score NUMERIC(5, 2) NOT NULL,
    fundamental_score NUMERIC(5, 2),
    growth_score NUMERIC(5, 2),
    profitability_score NUMERIC(5, 2),
    debt_score NUMERIC(5, 2),
    valuation_score NUMERIC(5, 2),
    gmp_score NUMERIC(5, 2),
    subscription_score NUMERIC(5, 2),
    market_score NUMERIC(5, 2),
    shariah_status shariah_status NOT NULL,
    explanation TEXT,
    factors JSONB,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_ipo_decisions ON ipo_decisions(ipo_id, decided_at DESC);

-- ==================================================
-- BACKTESTING
-- ==================================================

CREATE TABLE backtest_strategies (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    version INT NOT NULL DEFAULT 1,
    description TEXT,
    config JSONB NOT NULL,
    -- Config includes: entry_rules, exit_rules, stop_loss, take_profit,
    -- position_sizing, filters, etc.
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(name, version)
);

CREATE TABLE backtest_runs (
    id SERIAL PRIMARY KEY,
    strategy_id INT NOT NULL REFERENCES backtest_strategies(id),
    -- Parameters
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    initial_capital NUMERIC(20, 4) NOT NULL,
    -- Costs
    brokerage_pct NUMERIC(8, 6) NOT NULL DEFAULT 0.0003,
    stt_pct NUMERIC(8, 6) NOT NULL DEFAULT 0.001,
    exchange_charges_pct NUMERIC(8, 6) NOT NULL DEFAULT 0.0000345,
    gst_pct NUMERIC(8, 6) NOT NULL DEFAULT 0.18,
    stamp_duty_pct NUMERIC(8, 6) NOT NULL DEFAULT 0.00015,
    slippage_pct NUMERIC(8, 6) NOT NULL DEFAULT 0.001,
    -- Bias prevention
    survivorship_bias_handled BOOLEAN NOT NULL DEFAULT true,
    look_ahead_bias_checked BOOLEAN NOT NULL DEFAULT true,
    -- Results
    total_return NUMERIC(10, 4),
    cagr NUMERIC(10, 4),
    win_rate NUMERIC(5, 4),
    loss_rate NUMERIC(5, 4),
    profit_factor NUMERIC(10, 4),
    avg_win NUMERIC(10, 4),
    avg_loss NUMERIC(10, 4),
    sharpe_ratio NUMERIC(10, 4),
    sortino_ratio NUMERIC(10, 4),
    max_drawdown NUMERIC(10, 4),
    total_trades INT,
    avg_holding_days NUMERIC(10, 2),
    best_trade_return NUMERIC(10, 4),
    worst_trade_return NUMERIC(10, 4),
    -- Benchmark comparison
    benchmark_symbol VARCHAR(50) DEFAULT 'NIFTY 50',
    benchmark_return NUMERIC(10, 4),
    benchmark_sharpe NUMERIC(10, 4),
    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'RUNNING',
    error_message TEXT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE backtest_trades (
    id BIGSERIAL PRIMARY KEY,
    run_id INT NOT NULL REFERENCES backtest_runs(id) ON DELETE CASCADE,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    -- Entry
    entry_date TIMESTAMPTZ NOT NULL,
    entry_price NUMERIC(18, 4) NOT NULL,
    entry_signal signal_type,
    quantity INT NOT NULL,
    -- Exit
    exit_date TIMESTAMPTZ,
    exit_price NUMERIC(18, 4),
    exit_reason VARCHAR(50),  -- STOP_LOSS, TAKE_PROFIT, SIGNAL, HOLDING_PERIOD
    -- P&L
    gross_pnl NUMERIC(18, 4),
    transaction_costs NUMERIC(18, 4),
    net_pnl NUMERIC(18, 4),
    return_pct NUMERIC(10, 4),
    holding_days INT
);

CREATE INDEX idx_bt_trades_run ON backtest_trades(run_id);

-- ==================================================
-- PAPER TRADING
-- ==================================================

CREATE TABLE paper_portfolios (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    name VARCHAR(100) NOT NULL,
    initial_capital NUMERIC(20, 4) NOT NULL,
    current_capital NUMERIC(20, 4) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE paper_orders (
    id SERIAL PRIMARY KEY,
    portfolio_id INT NOT NULL REFERENCES paper_portfolios(id),
    symbol_id INT NOT NULL REFERENCES symbols(id),
    side order_side NOT NULL,
    quantity INT NOT NULL,
    order_price NUMERIC(18, 4) NOT NULL,
    fill_price NUMERIC(18, 4),
    simulated_slippage NUMERIC(18, 4),
    status order_status NOT NULL DEFAULT 'PENDING',
    signal_id BIGINT REFERENCES stock_signals(id),
    -- Tracking
    predicted_return NUMERIC(10, 4),
    actual_return NUMERIC(10, 4),
    prediction_error NUMERIC(10, 4),
    ordered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    filled_at TIMESTAMPTZ
);

CREATE INDEX idx_paper_orders_portfolio ON paper_orders(portfolio_id, ordered_at DESC);

CREATE TABLE paper_positions (
    id SERIAL PRIMARY KEY,
    portfolio_id INT NOT NULL REFERENCES paper_portfolios(id),
    symbol_id INT NOT NULL REFERENCES symbols(id),
    quantity INT NOT NULL,
    avg_entry_price NUMERIC(18, 4) NOT NULL,
    current_price NUMERIC(18, 4),
    unrealized_pnl NUMERIC(18, 4),
    realized_pnl NUMERIC(18, 4) DEFAULT 0,
    stop_loss NUMERIC(18, 4),
    target_price NUMERIC(18, 4),
    opened_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE paper_sessions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    start_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    end_time TIMESTAMPTZ,
    market_date DATE NOT NULL,
    strategy_version VARCHAR(50) NOT NULL,
    strategy_hash VARCHAR(255) NOT NULL,
    universe_version VARCHAR(50),
    market_data_provider VARCHAR(50),
    cost_model VARCHAR(50),
    slippage_model VARCHAR(50),
    initial_capital NUMERIC(20, 4) NOT NULL
);

CREATE TABLE paper_events (
    id SERIAL PRIMARY KEY,
    session_id UUID REFERENCES paper_sessions(id),
    symbol_id INT REFERENCES symbols(id),
    event_type VARCHAR(50) NOT NULL, -- DATA_GAP, WEBSOCKET_DISCONNECT, FALLBACK, STALE, RESTART
    severity VARCHAR(20) NOT NULL,
    message TEXT,
    metadata JSONB,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE paper_portfolio_snapshots (
    id SERIAL PRIMARY KEY,
    portfolio_id INT NOT NULL REFERENCES paper_portfolios(id),
    session_id UUID REFERENCES paper_sessions(id),
    snapshot_date DATE NOT NULL,
    opening_cash NUMERIC(20, 4) NOT NULL,
    closing_cash NUMERIC(20, 4) NOT NULL,
    opening_positions INT NOT NULL,
    closing_positions INT NOT NULL,
    gross_pnl NUMERIC(18, 4) NOT NULL,
    net_pnl NUMERIC(18, 4) NOT NULL,
    fees NUMERIC(18, 4) NOT NULL,
    slippage NUMERIC(18, 4) NOT NULL,
    exposure NUMERIC(5, 4) NOT NULL,
    drawdown NUMERIC(5, 4) NOT NULL,
    portfolio_value NUMERIC(20, 4) NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(portfolio_id, snapshot_date)
);

-- ==================================================
-- QUANTITATIVE ANALYTICS
-- ==================================================

CREATE TABLE quantitative_metrics (
    id SERIAL PRIMARY KEY,
    symbol_id INT NOT NULL REFERENCES symbols(id),
    benchmark_symbol VARCHAR(50) NOT NULL DEFAULT 'NIFTY 50',
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    -- Returns
    absolute_return NUMERIC(10, 4),
    cagr NUMERIC(10, 4),
    -- Risk
    volatility_annual NUMERIC(10, 4),
    downside_deviation NUMERIC(10, 4),
    max_drawdown NUMERIC(10, 4),
    var_95 NUMERIC(10, 4),
    cvar_95 NUMERIC(10, 4),
    -- Ratios
    sharpe_ratio NUMERIC(10, 4),
    sortino_ratio NUMERIC(10, 4),
    calmar_ratio NUMERIC(10, 4),
    information_ratio NUMERIC(10, 4),
    -- Market relationship
    beta NUMERIC(10, 4),
    alpha NUMERIC(10, 4),
    correlation NUMERIC(10, 4),
    covariance NUMERIC(18, 8),
    tracking_error NUMERIC(10, 4),
    -- Meta
    calculated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(symbol_id, benchmark_symbol, period_start, period_end)
);

CREATE INDEX idx_quant_metrics_symbol ON quantitative_metrics(symbol_id);

-- ==================================================
-- ALERTS
-- ==================================================

CREATE TABLE alert_rules (
    id SERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    alert_type alert_type NOT NULL,
    channel alert_channel NOT NULL,
    symbol_id INT REFERENCES symbols(id),
    ipo_id INT REFERENCES ipos(id),
    config JSONB DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE alert_history (
    id BIGSERIAL PRIMARY KEY,
    rule_id INT REFERENCES alert_rules(id),
    alert_type alert_type NOT NULL,
    channel alert_channel NOT NULL,
    recipient VARCHAR(255) NOT NULL,
    subject VARCHAR(500),
    message TEXT NOT NULL,
    reason TEXT NOT NULL,
    metadata JSONB,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered BOOLEAN DEFAULT false,
    error TEXT
);

CREATE INDEX idx_alert_history_sent ON alert_history(sent_at DESC);

-- ==================================================
-- JOBS & MONITORING
-- ==================================================

CREATE TABLE job_status (
    id SERIAL PRIMARY KEY,
    job_name VARCHAR(100) NOT NULL,
    job_type VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_job_status_name ON job_status(job_name, created_at DESC);

-- ==================================================
-- API KEYS (encrypted)
-- ==================================================

CREATE TABLE api_keys (
    id SERIAL PRIMARY KEY,
    provider VARCHAR(50) NOT NULL UNIQUE,
    encrypted_key BYTEA NOT NULL,
    key_metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ==================================================
-- APPLICATION SETTINGS
-- ==================================================

CREATE TABLE app_settings (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert default settings
INSERT INTO app_settings (key, value, description) VALUES
    ('default_benchmark', '"NIFTY 50"', 'Default benchmark index for comparisons'),
    ('default_risk_percentage', '0.01', 'Default risk per trade (1%)'),
    ('data_retention_days', '3650', 'Number of days to retain historical data'),
    ('max_position_pct', '0.10', 'Maximum single position as % of capital'),
    ('timezone_display', '"Asia/Kolkata"', 'Display timezone for UI');
