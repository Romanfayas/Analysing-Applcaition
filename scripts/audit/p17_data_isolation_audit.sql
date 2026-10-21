-- P1.7 repeatable daily/intraday isolation audit. Read-only. Run:
--   docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X < scripts/audit/p17_data_isolation_audit.sql
\echo '== A1 row counts (all tables involved)'
SELECT 'ohlcv_daily' t, count(*) FROM ohlcv_daily UNION ALL
SELECT 'ohlcv_intraday', count(*) FROM ohlcv_intraday UNION ALL
SELECT 'contamination_backup_20261014', count(*) FROM ohlcv_daily_contamination_backup_20261014 UNION ALL
SELECT 'holiday_phantom_quarantine (P1.6)', count(*) FROM ohlcv_holiday_phantom_quarantine UNION ALL
SELECT 'daily_quarantine_p17', count(*) FROM ohlcv_daily_quarantine_p17;

\echo '== A2 duplicates: (symbol, IST date) and PK'
SELECT count(*) AS dup_symbol_dates FROM (SELECT symbol_id,(timestamp AT TIME ZONE 'Asia/Kolkata')::date d FROM ohlcv_daily GROUP BY 1,2 HAVING count(*)>1) x;

\echo '== A3 daily stamp (must be only 03:45 UTC = 09:15 IST)'
SELECT (timestamp AT TIME ZONE 'UTC')::time AS utc_time, count(*) FROM ohlcv_daily GROUP BY 1;

\echo '== A4 OHLCV invariants + phantom + adjusted_close'
SELECT count(*) FILTER (WHERE high<low OR open<=0 OR close<=0 OR high<greatest(open,close) OR low>least(open,close) OR volume<0) invalid_ohlc,
       count(*) FILTER (WHERE volume=0 AND open=close AND high=low) zero_vol_flat,
       count(*) FILTER (WHERE adjusted_close=0) adj_zero,
       count(*) FILTER (WHERE adjusted_close IS NULL) adj_null,
       count(*) FILTER (WHERE quality_status<>'VALID') non_valid
FROM ohlcv_daily;

\echo '== A5 per-symbol bars (min/max/count of symbols) and bars per symbol'
SELECT min(c) min_bars, max(c) max_bars, count(*) symbols, sum(c) total FROM (SELECT symbol_id,count(*) c FROM ohlcv_daily GROUP BY 1) x;
SELECT s.symbol, count(*) bars, min(timestamp)::date first, max(timestamp)::date last FROM ohlcv_daily o JOIN symbols s ON s.id=o.symbol_id GROUP BY 1 ORDER BY 2,1 LIMIT 5;

\echo '== A6 weekdays 2024-10-07..last bar with no bar for ANY symbol (compare to calendar holiday list)'
SELECT d::date AS missing_weekday FROM generate_series('2024-10-07'::date,(SELECT max((timestamp AT TIME ZONE 'Asia/Kolkata')::date) FROM ohlcv_daily),'1 day') d
WHERE extract(isodow FROM d)<6 AND d::date NOT IN (SELECT (timestamp AT TIME ZONE 'Asia/Kolkata')::date FROM ohlcv_daily) ORDER BY 1;

\echo '== A7 dates with fewer than 47 symbols (symbol-level session gaps)'
SELECT (timestamp AT TIME ZONE 'Asia/Kolkata')::date d, count(*) n FROM ohlcv_daily GROUP BY 1 HAVING count(*)<>47 ORDER BY 1;

\echo '== A8 weekend bars (should be only calendar special sessions)'
SELECT (timestamp AT TIME ZONE 'Asia/Kolkata')::date d, count(*) FROM ohlcv_daily WHERE extract(isodow FROM timestamp AT TIME ZONE 'Asia/Kolkata')>5 GROUP BY 1;

\echo '== A9 intraday intervals present and overlap with daily table'
SELECT interval_minutes, count(*), min(timestamp), max(timestamp) FROM ohlcv_intraday GROUP BY 1;

\echo '== A10 recoverability: backup rows still present in intraday / quarantine rows not in daily'
SELECT count(*) AS backup_rows_missing_from_intraday FROM ohlcv_daily_contamination_backup_20261014 b
 WHERE NOT EXISTS (SELECT 1 FROM ohlcv_intraday i WHERE i.symbol_id=b.symbol_id AND i."timestamp"=b."timestamp");
SELECT count(*) AS quarantine_rows_still_in_daily FROM ohlcv_daily_quarantine_p17 q JOIN ohlcv_daily o USING (symbol_id,"timestamp");
SELECT count(*) AS p16_quarantine_rows_still_in_daily FROM ohlcv_holiday_phantom_quarantine q JOIN ohlcv_daily o USING (symbol_id,"timestamp");
SELECT quarantine_reason, (timestamp AT TIME ZONE 'Asia/Kolkata')::date d, count(*) FROM ohlcv_daily_quarantine_p17 GROUP BY 1,2 ORDER BY 2;

\echo '== A11 views/matviews that could expose intraday as daily'
SELECT count(*) AS user_views FROM pg_views WHERE schemaname='public';
SELECT count(*) AS user_matviews FROM pg_matviews WHERE schemaname='public';

\echo '== A12 corporate actions vs symbols with daily data'
SELECT ca.action_type, ca.ex_date, s.symbol, (SELECT count(*) FROM ohlcv_daily o WHERE o.symbol_id=ca.symbol_id) daily_bars FROM corporate_actions ca JOIN symbols s ON s.id=ca.symbol_id;
SELECT s.symbol FROM symbols s WHERE NOT EXISTS (SELECT 1 FROM ohlcv_daily o WHERE o.symbol_id=s.id) ORDER BY 1;

\echo '== A13 large close-to-close moves (>20%) needing corporate-action review'
SELECT s.symbol,d,close,prev,round((close/prev-1)*100,1) pct FROM (SELECT symbol_id,(timestamp AT TIME ZONE 'Asia/Kolkata')::date d,close,lag(close) OVER (PARTITION BY symbol_id ORDER BY timestamp) prev FROM ohlcv_daily) o JOIN symbols s ON s.id=o.symbol_id WHERE prev>0 AND abs(close/prev-1)>0.20 ORDER BY d;
