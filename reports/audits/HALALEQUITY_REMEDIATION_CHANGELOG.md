# HalalEquity Remediation Changelog & Fix Inventory

**Audit Phase**: P1.9.14 — Independent Forensic Evidence & Release-Readiness Audit  
**Changelog Scope**: Software Defect Remediation, Security Hardening, and Parameter Verification  
**Audit Target**: `HALALEQUITY_COMPREHENSIVE_REMEDIATION_AND_RELEASE_READINESS_REPORT.md`  

---

## 1. Inventory of Remediated Software Defects

### Fix 1: Strategy V2 Canonical Parameter Reification & Conflation Resolution
- **Issue & Root Cause**:
  In Section 8.1 of `HALALEQUITY_COMPREHENSIVE_REMEDIATION_AND_RELEASE_READINESS_REPORT.md`, the delivery report stated:
  `Parameters: min_volume = 500,000, rvol = 1.30, adx = 20.0, atr_multiple = 1.50, risk_reward = 3.0, initial_capital = ₹1,000,000`.
  However, the canonical SHA-256 hash `92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780` is generated exclusively from the frozen canonical rule specification:
  `{"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}`.
  The delivery report conflated the breakout opportunity screener filters (`rvol >= 1.30, adx >= 20.0`) with the Strategy V2 rule configuration and misstated `atr_multiple = 1.50`.
- **Affected Files**:
  - `HALALEQUITY_COMPREHENSIVE_REMEDIATION_AND_RELEASE_READINESS_REPORT.md` (Section 8.1)
  - `quant/tests/test_comprehensive_remediation_release_gate.py`
  - `backend/internal/database/comprehensive_remediation_release_gate_test.go`
  - `scripts/verify_comprehensive_release_readiness.py`
- **Before vs After Code Behavior**:
  - *Before*: The test suites merely checked `assert len(canonical_hash) == 64` without recalculating the hash from serialized JSON, leaving the door open to parameter drift and ambiguity.
  - *After*: 
    1. Both Go and Python release gate suites directly compute `sha256(canonical_json)` and assert exact match with `92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780`.
    2. Negative regression tests verify that changing `ATR_MULTIPLIER` to `1.50` produces a different hash and causes `enforce_request()` to raise `StrategyDrift`.
    3. The delivery report was corrected to clearly distinguish the frozen canonical rule parameters (`BUY_THRESHOLD=80.0, SELL_THRESHOLD=45.0, ATR_MULTIPLIER=2.0`) from the operational screener filters.
- **Verification Command & Output**:
  ```bash
  docker exec halal_equity_quant pytest tests/test_comprehensive_remediation_release_gate.py -k test_comprehensive_strategy_v2_canonical_lock_and_gate_status
  ```
  *Output*: `1 passed in 0.12s` (100% PASS).
- **Safety Justification**: Eliminates ambiguity, prevents accidental backtesting on unauthorized strategy variants, and ensures absolute reproducibility across backtesting and forward execution engines.

---

### Fix 2: Automated Release Gate Evidence Generation & Schema Alignment
- **Issue & Root Cause**:
  `scripts/verify_comprehensive_release_readiness.py` printed evaluation results to stdout but did not write `HALALEQUITY_RELEASE_GATE_EVIDENCE.json`. Furthermore, Criterion 9 relied on a hardcoded string comparison rather than independent SHA-256 calculation.
- **Affected Files**:
  - `scripts/verify_comprehensive_release_readiness.py`
  - `reports/audits/HALALEQUITY_RELEASE_GATE_EVIDENCE.json`
- **Before vs After Code Behavior**:
  - *Before*: The script only printed a plain ASCII table. No machine-readable artifact was produced.
  - *After*: The script computes the canonical hash dynamically from serialized JSON, validates all 13 criteria, aggregates counts dynamically from record objects, and automatically writes `reports/audits/HALALEQUITY_RELEASE_GATE_EVIDENCE.json` containing complete audit metadata, git revision, timestamp with timezone, safety invariants, external blockers, and per-criterion audit evidence.
- **Verification Command & Output**:
  ```bash
  ADMIN_DATABASE_URL="postgres://halal_user:devpassword@192.168.18.43:5432/halal_equity?sslmode=disable" DATABASE_URL="postgres://halal_app_user:app_password@192.168.18.43:5432/halal_equity?sslmode=disable" ./quant/.venv/bin/python scripts/verify_comprehensive_release_readiness.py
  ```
  *Output*:
  ```
  AUDIT SUMMARY: 9 PASS | 4 DATA_INSUFFICIENT | 0 FAIL
  SOFTWARE BUILD STATUS : PASS (100% reproducible software & security boundaries)
  MARKET DATA STATUS     : DATA_INSUFFICIENT (Quarantined pending authenticated exchange circulars & provider authorization)
  PLATFORM RESEARCH GATE: RESEARCH_BLOCKED (Strictly Maintained)
  [EVIDENCE EXPORT] Machine-readable evidence written to: /Users/fayas/Downloads/ICO-Application/analysis application/reports/audits/HALALEQUITY_RELEASE_GATE_EVIDENCE.json
  ```
- **Safety Justification**: Guarantees that machine-readable evidence is generated directly from automated audit execution rather than manual transcription, eliminating human error or false claims.

---

### Fix 3: Full Integration of TrueData Provider into Backend Dependency Graph
- **Issue & Root Cause**:
  While the TrueData Velocity adapter existed in `backend/internal/providers/marketdata/truedata`, it was an isolated package that was not wired into `backend/internal/workers/ingestion/pipeline.go` or `backend/internal/config/config.go`.
- **Affected Files**:
  - `backend/internal/config/config.go`
  - `backend/internal/workers/ingestion/pipeline.go`
- **Before vs After Code Behavior**:
  - *Before*: `Pipeline` only instantiated Yahoo Finance and NSE mock/scraped providers. `Config` lacked TrueData configuration keys.
  - *After*:
    1. Added `TrueDataBaseURL`, `TrueDataUsername`, `TrueDataPassword`, `TrueDataAPIKey` to `Config` with environment variable loading.
    2. Wired `truedataProvider marketdata.MarketDataProvider` into `Pipeline` struct and added accessor `TrueDataProvider()`.
    3. Confirmed that when unconfigured, the provider fails closed with `ErrSubscriptionUnauthorized` (`DATA_INSUFFICIENT_PENDING_SUBSCRIPTION_AUTHORIZATION`), ensuring zero live calls and zero expenditure.
- **Verification Command & Output**:
  ```bash
  go test ./cmd/... ./internal/...
  ```
  *Output*: All 26 backend packages build and pass tests cleanly.
- **Safety Justification**: Transitions the provider from dead code to an active, properly wired architecture component while strictly preventing unauthorized commercial network calls or costs.

---

## 2. Items Audited and Left Untouched (Technical Justifications)

1. **Quarantined 23,132 Historical OHLCV Bars**:
   - *Technical Justification*: Left as `adjusted_close IS NULL`. The bars lack primary-verified corporate action adjustment factors. Unquarantining them or synthesising adjustments would corrupt quantitative research with unauthenticated data.
2. **58 Unauthenticated Corporate Action Archives**:
   - *Technical Justification*: Left as `ARCHIVE_UNAUTHENTICATED`. Local scraped JSON archives do not possess digital signatures (DSC/PKI) from the exchange. Promoting them to `VERIFIED_EXCHANGE_PRIMARY` based on string URLs would violate cryptographic trust boundaries.
3. **31 Operating Equities with `DATA_INSUFFICIENT` Shariah Screenings**:
   - *Technical Justification*: Left as `DATA_INSUFFICIENT`. Under AAOIFI Shariah Standard No. 21, absent footnote interest income disclosures cannot be presumed to be zero. Promoting them to `PASS` without audited annual report footnote schedules would fabricate compliance.
4. **Platform Research Gate Status (`RESEARCH_BLOCKED`)**:
   - *Technical Justification*: Left strictly locked. Because 97.27% of historical market data is quarantined and corporate actions lack primary exchange authentication, releasing the research gate for backtesting would violate platform integrity invariants.

---

## 3. Explicit List of Prohibited Actions Avoided

1. **No Header Spoofing or WAF Bypasses**:
   - Did not implement custom HTTP headers (`User-Agent: Mozilla/...`, `Referer: nseindia.com`) to scrape protected exchange endpoints.
2. **No Paid Subscriptions or Trial Activations**:
   - Incurred ₹0.00 commercial expenses. Did not activate paid TrueData or exchange feed subscriptions without explicit user authorization.
3. **No Unlawful Database Role Grants**:
   - Preserved strict append-only least privilege for `halal_app_user`. Did not grant `UPDATE` or `DELETE` on corporate action or audit tables.
4. **No Artificial Price Modifications on Demerger Lines**:
   - Maintained `is_price_adjustment = false` for Tata Motors and LTIMindtree restructuring lineage rows. Tax cost allocations under Section 49(2C) of Income Tax Act 1961 are balance sheet basis allocations, not market price adjustments.
