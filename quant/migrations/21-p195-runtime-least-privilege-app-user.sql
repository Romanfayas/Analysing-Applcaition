-- HalalEquity Migration 21: P1.9.5 Runtime Least-Privilege Application Role Configuration
--
-- Objective:
-- 1. Grant least-privilege operational DML permissions to non-superuser application role (halal_app_user).
-- 2. Configure default search_path for halal_app_user to resolve public and ca schemas.
-- 3. Grant SELECT on views and tables in ca schema to halal_app_user.
-- 4. Grant DML on corporate action evidence tables (corporate_action_source_artifacts,
--    corporate_action_events, corporate_action_adjustment_factors) to allow immutability triggers
--    and verification guards to execute and enforce integrity rules.
-- 5. Revoke all CREATE permissions on public and ca schemas from halal_app_user and PUBLIC.
-- 6. Enable complete transition of backend API, background workers, and quant engine to halal_app_user.

BEGIN;

-- 1. Configure default search path
ALTER ROLE halal_app_user SET search_path = public, ca;

-- 2. Ensure schemas USAGE
GRANT USAGE ON SCHEMA public, ca TO halal_app_user;

-- 3. Revoke CREATE on schemas
REVOKE CREATE ON SCHEMA public, ca FROM halal_app_user;
REVOKE CREATE ON SCHEMA public, ca FROM PUBLIC;

-- 4. Grant DML on operational tables
GRANT SELECT, INSERT, UPDATE, DELETE ON 
    paper_portfolios, paper_positions, paper_orders, paper_events, 
    paper_observations, paper_portfolio_snapshots, paper_sessions,
    portfolios, portfolio_positions, portfolio_trades,
    users, user_sessions, audit_logs,
    risk_calculations, stock_signals, support_resistance_levels, 
    candlestick_patterns, technical_indicators, quantitative_metrics,
    shariah_screenings,
    ipos, ipo_subscriptions, ipo_financials, ipo_gmp, ipo_predictions, 
    ipo_decisions, ipo_peer_comparisons, ipo_ingestion_runs,
    eod_ingestion_runs, data_quality_logs,
    backtest_strategies, backtest_runs, backtest_trades, backtest_configurations,
    financial_statements, financial_ratios, fundamental_anomalies, shareholding_patterns,
    ohlcv_intraday
TO halal_app_user;

-- 5. Grant DML on ohlcv_daily (UPDATE is guarded by fn_ohlcv_daily_quarantine_guard)
GRANT SELECT, INSERT, UPDATE ON ohlcv_daily TO halal_app_user;

-- 6. Grant DML on corporate action evidence tables (guarded by trg_ca_factor_guard, ca_factor_ingest_guard, and trg_ca_event_verification_guard)
GRANT SELECT, INSERT, UPDATE, DELETE ON 
    corporate_action_source_artifacts, 
    corporate_action_events, 
    corporate_action_adjustment_factors 
TO halal_app_user;

-- 7. Grant strict READ-ONLY on reference tables and views
GRANT SELECT ON 
    symbols, shariah_rule_sets, shariah_rule_versions,
    corporate_actions,
    ohlcv_daily_contamination_backup_20261014, ohlcv_daily_quarantine_p17, ohlcv_holiday_phantom_quarantine
TO halal_app_user;

-- 8. Grant SELECT on ca schema views
GRANT SELECT ON ALL TABLES IN SCHEMA ca TO halal_app_user;

-- 9. Grant sequence usage
GRANT USAGE, SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO halal_app_user;

-- 10. Grant procedure execution
GRANT EXECUTE ON FUNCTION ca.apply_verified_corporate_action(TEXT, DATE, DATE, NUMERIC, TEXT) TO halal_app_user;

COMMIT;
