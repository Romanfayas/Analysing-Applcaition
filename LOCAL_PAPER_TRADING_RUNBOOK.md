# Local Paper Trading Runbook

## Start Environment

```bash
cd "/Users/fayas/Downloads/ICO-Application/analysis application"
docker compose down
docker compose up -d --build
```

## Verify Health & Freshness

```bash
# Check Backend API Health
curl http://localhost:8081/api/v1/health

# Check Market Data Freshness
curl http://localhost:8081/api/v1/market-data/health

# Check Quant Engine Strategy Hash
curl http://localhost:8000/health
```

## Monitor Trading

- Open the Next.js Frontend at `http://localhost:3000`
- Open Grafana at `http://localhost:3001` to view metrics
- Observe the API logs to track ingestion: `docker compose logs -f worker`

## Stop Safely

```bash
docker compose down
```

**Notice:** 
- The system will NOT place real orders. 
- You must wait for the actual NSE market session (09:15 - 15:30 IST) to observe valid paper trading signals.
- Ensure the `wallet-to` or other services on port `8080` do not conflict; the `.env` provided shifts the API to `8081`.
