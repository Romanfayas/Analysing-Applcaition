# Phase 14: Controlled Live Architecture

## Status: IMPLEMENTED (DISABLED BY DEFAULT)

### Overview
This document outlines the architecture for real-money live order execution. 

**WARNING: LIVE EXECUTION IS STRICTLY DISABLED IN THIS PHASE.**

### Environment Variables Enforced
The following flags must remain in this state:
```env
PAPER_TRADING_ENABLED=true
LIVE_ORDER_EXECUTION=false
BROKER_TRADING_ENABLED=false
```

### Architecture
- **Broker Interface**: A generic interface for broker integrations.
- **Adapters**: Future support for `Groww` and `Zerodha` APIs.
- **Safety Pipeline**:
  - Strategy Authorization -> Risk Engine -> Position Limits -> Daily Loss Limit -> Data Freshness Check -> Broker Availability -> Idempotency -> Order Confirmation.
- **Kill Switch**: The system supports an emergency `TRADING_ENABLED=false` bypass which halts all live and paper signal evaluations.

### Compliance
No hidden bypasses exist. The live execution path fails closed.
