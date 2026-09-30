# PHASE 10 DEPLOYMENT CHECKLIST

### Before Market Open (Before 09:15 IST)
- [ ] PostgreSQL healthy
- [ ] Go API healthy
- [ ] Python Quant Engine healthy
- [ ] Frontend healthy
- [ ] Market provider reachable (Test outbound connection)
- [ ] Strategy V2 hash verified (Ensure no parameter modifications)
- [ ] Paper mode enabled (`PAPER_TRADING_ENABLED=true`)
- [ ] Live execution disabled (`LIVE_ORDER_EXECUTION=false`, `BROKER_TRADING_ENABLED=false`)
- [ ] Universe loaded (NIFTY 50)
- [ ] Database writable (No read-only locks)

### During Market (09:15 IST - 15:30 IST)
- [ ] Market data flowing (Check logs for active ingestion)
- [ ] Timestamps valid (Ensure timezone aligns with IST)
- [ ] Session valid
- [ ] Signals processed (Check quant engine logs for calculation completions)
- [ ] PENDING orders persisted (Sync from Python -> Go -> DB)
- [ ] Fallback monitored (Verify REST fallbacks are activating only when primary fails)
- [ ] Errors monitored (Watch for unhandled exceptions or data gaps)

### At T+1 Open (Next Day 09:15 IST)
- [ ] Paper orders filled (Check state transitions from PENDING -> FILLED)
- [ ] Execution prices recorded
- [ ] Slippage recorded (Using baseline 10 bps model)
- [ ] Costs recorded (Using `india_equity_delivery_v1` model)

### At Market Close (15:30 IST)
- [ ] Portfolio reconciled (Cash + Positions = Closing Equity)
- [ ] Snapshots persisted (`paper_portfolio_snapshots` updated)
- [ ] P&L calculated (Unrealized + Realized - Costs)
- [ ] Drawdown calculated
- [ ] Session closed
- [ ] Evidence exported (Dump database tables or docker logs)
