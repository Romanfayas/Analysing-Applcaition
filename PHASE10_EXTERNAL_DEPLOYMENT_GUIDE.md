# PHASE 10 EXTERNAL DEPLOYMENT GUIDE

## Prerequisites
- **OS**: Linux VPS (Ubuntu 22.04 LTS recommended), Cloud VM, or an always-on local machine.
- **Docker**: Docker Engine 24.0+ and Docker Compose v2.
- **Network**: Port 80/443 exposed for Frontend API access if public, otherwise restricted to VPN/Localhost.
- **Timezone**: Set system time to IST (Indian Standard Time, UTC+5:30) to align logs and schedules correctly.

## 1. Environment Variables Configuration
Copy `.env.example` to `.env` and fill in necessary secrets.
*Ensure that security requirements for Phase 10 are met:*
```env
PAPER_TRADING_ENABLED=true
LIVE_ORDER_EXECUTION=false
BROKER_TRADING_ENABLED=false
```

## 2. Infrastructure Startup
Start the entire stack using Docker Compose:
```bash
docker compose up -d postgres redis
```
Wait 30 seconds for the database to fully initialize and apply `01-init.sql`.

## 3. Start Core Services
```bash
docker compose up -d quant backend worker
```
Wait for the services to indicate "healthy" status.

## 4. Start Frontend & Telemetry
```bash
docker compose up -d frontend prometheus grafana
```

## 5. Health Checks
Check logs to verify the system is running smoothly:
```bash
docker compose logs -f backend
docker compose logs -f quant
```
Ensure there are no DB connection errors or initialization failures.

## 6. Shutdown Procedure
To safely stop the system during non-market hours (if desired) without data loss:
```bash
docker compose stop
```
(Do NOT use `docker compose down -v` as it destroys the database volumes).

## 7. Restart Procedure
```bash
docker compose restart
```

## 8. Log Locations
- Docker handles log rotation automatically.
- To export logs for evidence: `docker compose logs > session_evidence.log`

## Troubleshooting
- **Database Connection Error**: Verify `POSTGRES_PASSWORD` matches between `.env` and `docker-compose.yml`.
- **Market Data Errors**: Ensure the machine has stable outbound internet access to Yahoo Finance/NSE APIs.
- **Paper Orders Not Syncing**: Check the `worker` container logs.
