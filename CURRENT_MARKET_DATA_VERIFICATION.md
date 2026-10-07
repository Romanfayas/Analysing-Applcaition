# Current Market Data Verification

## Problem Statement
The application was serving outdated historical placeholder data (`2023-12-29T00:00:00Z` with `$0.0` prices) instead of fresh market data.

## Fixes Applied
1. **Database Queries**: Updated `GetStock` handler to query `GetLatestOHLCV` directly for the symbol, extracting the exact close price and timestamp.
2. **Freshness Validation**: Implemented `market.GetFreshnessStatus` with `Asia/Kolkata` market hours (09:15-15:30) validation. Data older than 15 minutes during the open session is tagged as `STALE`.
3. **Ingestion Worker**: Wired up the ingestion worker to run every `1 minute` during market hours, instead of the previous 24-hour EOD cycle.
4. **Health Endpoint**: Added `GET /api/market-data/health` to expose the freshness and current market session clock.

## Verification Checklist
- [x] Provider timestamp is used instead of hardcoded strings.
- [x] Received timestamp is formatted accurately.
- [x] API outputs `freshness` and `market_session`.
- [x] Frontend relies on `freshness_status` rather than assuming data is live.
- [x] No fake real-time data is fabricated.
