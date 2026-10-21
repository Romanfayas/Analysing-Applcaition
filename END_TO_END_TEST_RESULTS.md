# HalalEquity — End-to-End Test Results & Validation Audit

**Audit Date:** 2026-10-21T16:35:00+05:30  
**Harnesses Executed:**
1. Automated Python Workflow Test Harness (`scripts/test_workflows_live.py`)
2. Automated Python E2E Integration Suite (`scripts/verify_e2e_delivery.py`)
3. Go Backend Test Harness (`cd backend && go test -count=1 -v ./...`)
4. Python Quant Engine Pytest Harness (`docker exec halal_equity_quant pytest -v`)
5. Frontend Turbopack Production Build Harness (`cd frontend && npm run build`)

---

## 1. Live Workflow Execution Audit (`scripts/test_workflows_live.py`)

**Command:** `python3 scripts/test_workflows_live.py`  
**Exit Code:** `0` (Success)  
**Output Log:**

```
=== 1. Stock Universe & Market Overview ===
Stocks List: Code 200, Active Count=51
Market Overview: Code 200, ActiveEquities=51, Sectors=9

=== 2. Multiple Stock Detail Pages ===
Stock TCS       : Code 200, Price=₹2529, Freshness=LAST_AVAILABLE, Shariah=DATA_INSUFFICIENT, Active=True
Stock INFY      : Code 200, Price=₹1168, Freshness=LAST_AVAILABLE, Shariah=DATA_INSUFFICIENT, Active=True
Stock TATAMOTORS: Code 200, Price=₹None, Freshness=UNAVAILABLE, Shariah=UNKNOWN, Active=False
Stock TMPV      : Code 200, Price=₹275.7, Freshness=LAST_AVAILABLE, Shariah=FAIL, Active=True
Stock TMCV      : Code 200, Price=₹417.5, Freshness=LAST_AVAILABLE, Shariah=DATA_INSUFFICIENT, Active=True
Stock DIVISLAB  : Code 200, Price=₹9190, Freshness=LAST_AVAILABLE, Shariah=DATA_INSUFFICIENT, Active=True

=== 3. Shariah Screenings Evidence ===
Shariah Screenings: Code 200, Total Records=130
Sample Screening (ADANIENT): DebtRatio=0.2893, Status=DATA_INSUFFICIENT, Notes=Core non-financial operating business

=== 4. IPO Dashboard ===
IPOs: Code 200, Total Records=8
Sample IPO (PARASHLTH): Status=UPCOMING, PriceBand=, Shariah=UNKNOWN

=== 5. Position Sizing & Risk Calculation ===
Risk Calculate: Code 200, SuggestedShares=150, RiskPerShare=₹100, R:R=3, CapitalExposure=37.5%

=== 6. Paper BUY Gating (Fail-Closed Rejections) ===
BUY TCS (DATA_INSUFFICIENT): Code 400, Error=Shariah compliance gate blocked: cannot BUY TCS because screening status is DATA_INSUFFICIENT (must be certified PASS)
BUY TATASTEEL (FAIL): Code 400, Error=Shariah compliance gate blocked: cannot BUY TATASTEEL because screening status is FAIL (must be certified PASS)

=== 7. Paper SELL Gating (Short-Selling Restriction) ===
SELL TCS (0 Owned Shares): Code 400, Error=insufficient shares: attempting to sell 5 but only own 0 (Short selling is strictly prohibited)

=== 8. Portfolio Balances & Ledger State ===
Portfolio: Code 200, Cash=₹1000000, Equity=₹1000000

=== 9. Backtesting Research Gate ===
Backtesting Gate: Code 200, Status=RESEARCH_BLOCKED
Reason: Historical backtest blocked: 23132 historical daily bars remain quarantined (NULL adjusted close) and 27 core equities lack complete primary-source corporate-action reconciliation. Backtesting is gated to prevent survivorship and adjustment bias until all corporate actions are verified from primary exchange disclosures.
Quarantined Bars Invariant: 23132
Frozen Strategy SHA256: 92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780

=== 10. Data Quality Dashboard & Quarantine Counts ===
Data Quality: Code 200, Datasets Count=4
 - OHLCV Market Data: Status=PARTIAL, Valid=23686, Invalid=2
 - Corporate Actions Primary Evidence: Status=VALID, Valid=0, Invalid=0
 - Financial Statements (PIT): Status=VALID, Valid=50, Invalid=0
 - IPO Offerings & Subscriptions: Status=VALID, Valid=8, Invalid=0
Quarantine Issues API: Code 200, Issues Count=25

=== 11. Security & Negative Input Tests ===
Direct Backend Without JWT: Code 401, Error=missing authorization header
Nonexistent Stock: Code 404, Error=None
Stop Loss >= Entry: Code 400, Error=stop_loss must be positive and lower than entry_price (Long-only)
Invalid Trade Type: Code 400, Error=trade_type must be BUY or SELL
Negative Shares: Code 400, Error=shares must be positive
```

---

## 2. E2E Delivery Verification Suite (`scripts/verify_e2e_delivery.py`)

**Command:** `python3 scripts/verify_e2e_delivery.py`  
**Exit Code:** `0` (Success)  
**Summary:** 18/18 checks passed.

```
======================================================================
HALALEQUITY END-TO-END DELIVERY VERIFICATION
======================================================================
[PASS] Backend Health: Code 200, status=healthy
[PASS] Quant Engine Health: Code 200, status=healthy
[PASS] Market Data Health: Code 200
[PASS] Stock Universe List (via Proxy): Found 51 active securities
[PASS] Market Overview Route: TotalEquities=51, Sectors=9
[PASS] TCS Stock Detail: Price=₹2529, Shariah=DATA_INSUFFICIENT
[PASS] Shariah Screening Universe: Retrieved 130 screenings
[PASS] IPO Pipeline Ingestion: Retrieved 8 IPO records
[PASS] Quant Breakout Signals: Retrieved 0 signals
[PASS] Portfolio Mark-to-Market: Cash=₹1000000, Equity=₹1000000
[PASS] Shariah Gate Enforcement (BUY Blocked for DATA_INSUFFICIENT): Result=Shariah compliance gate blocked...
[PASS] Short Selling Restriction Enforcement: Result=insufficient shares: attempting to sell 5 but only own 0 (Short selling is strictly prohibited)
[PASS] Paper Trading Session Safety State: PaperTrading=True, LiveExecution=False
[PASS] Risk Position Sizing Calculator: SuggestedShares=150, R:R=3, RiskPerShare=₹100
[PASS] Portfolio Risk Metrics: ExecutionMode=PAPER_TRADING, MaxHeat=10%, MaxRiskTrade=2%
[PASS] Data Quality Dashboard: Datasets=4
[PASS] Data Quality Quarantine Issues List: Retrieved 25 quarantine records
[PASS] Strategy V2 Research Gate & Parameter Verification: Status=RESEARCH_BLOCKED, QuarantinedBars=23132, SHA256=92ab27c24e4bc4c6...
======================================================================
VERIFICATION SUMMARY: 18/18 checks passed.
======================================================================
```

---

## 3. Go Backend Test Execution

**Command:** `cd backend && go test -count=1 -v ./...`  
**Exit Code:** `0` (Success)  

Key integration test suites passing:
- `TestDeliveryFeeCalculationBreakdown`: Verified exact rupee-and-paisa match for BUY (₹118.62) and SELL (₹119.55).
- `TestPaperTradingLedgerAccountingAndFriction`: Asserts cash deductions, `INDIA_EQ_DELIVERY_FEES_2026Q4_V2` statutory friction, average cost basis, realized P&L, and short-selling block.
- `TestPaperTradingExactThreeStepLifecycle`: Asserts exact 3-step lifecycle reproduction:
  - Step 1: BUY 100 @ ₹1,000 $\rightarrow$ Fees ₹118.62, Cash ₹899,881.38, Basis ₹1,001.1862/sh.
  - Step 2: SELL 40 @ ₹1,200 $\rightarrow$ Fees ₹65.68 (vs erroneous report claim ₹57.67), Realized P&L +₹7,886.87, Cash ₹947,815.70.
  - Step 3: SELL 60 @ ₹1,100 $\rightarrow$ Fees ₹84.33 (vs erroneous report claim ₹78.86), Realized P&L +₹5,844.50, Cash ₹1,013,731.37.
  - Zero cash discrepancy: Initial ₹1,000,000 + Cumulative P&L ₹13,731.37 = Final Cash ₹1,013,731.37.
- `TestConcurrentOrdersAtomicity`: 5 concurrent goroutines competing for cash under row-level database lock (`SELECT FOR UPDATE`). Exactly 2 succeed, 3 fail on funds, zero double-spending, zero ledger corruption.
- `TestIsTradingHolidayNSECalendar2024to2026`: Validates the official exchange holiday calendar engine and weekend handling across 2024-2026.
- `TestUniverseCoverageReconciliation52Symbols`: Asserts 52 canonical universe instruments and 64 total events.
- `TestP1813DataReadinessAndAdjustedPriceAudit`: Asserts 23,688 daily bars, 556 populated, 23,132 quarantined.
- `TestP1814TCSMultiplicativeFactorAuditAndConsumerIsolation`: Asserts factor 0.99471203 across 7 cum-dividend bars and nominal view isolation (0 rows).
- `TestP1815UniverseAndCorporateActionReconciliation`: Asserts 64 corporate action events, 23 source artifacts.
- `TestActivePriceAdjustmentFactorsIsolation`: Asserts only TCS Revision 3 is active; TATAMOTORS demerger tax factor rejected.

---

## 4. Python Quant Engine Test Execution

**Command:** `docker exec halal_equity_quant pytest -v`  
**Exit Code:** `0` (Success)  
**Summary:** 265 passed, 2 warnings in 62.54s.

Key test areas covered:
- `test_frozen_strategy_v2_canonical_hash_lock`: PASS (Asserts hash `92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780`).
- `test_research_gate_blocks_backtesting_on_incomplete_evidence`: PASS.
- `test_historical_quarantine_metrics`: PASS (Asserts 23,132 quarantined bars).
- `test_nominal_vs_total_return_consumer_separation_invariants`: PASS.
- `test_shariah_fail_cannot_be_overridden`: PASS.
- `test_missing_required_metrics_cannot_infer_zero`: PASS.
- `test_position_capped_by_capital`: PASS.

---

## 5. Frontend Production Build Audit

**Command:** `cd frontend && npm run build`  
**Exit Code:** `0` (Success)  
**Engine:** Next.js 16.3.5 with Turbopack (Compiled successfully in 1413ms)  
**Static / Dynamic Routes Verified:**
- `/` (Static)
- `/stocks` (Static)
- `/stocks/[symbol]` (Dynamic)
- `/paper-trading` (Static)
- `/backtesting` (Static)
- `/data-quality` (Static)
- `/shariah` (Static)
- `/risk` (Static)
- `/signals` (Static)
- `/ipos` (Static)
