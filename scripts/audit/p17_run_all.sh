#!/bin/sh
# Reproduce the P1.7 verification. Run from the repo root with the compose stack up. Read-only except the pytest scratch DB it creates/drops.
set -u
PG="docker exec -i halal_equity_postgres psql -U halal_user -d halal_equity -X"
echo "### 1. SQL data-isolation audit";            $PG < scripts/audit/p17_data_isolation_audit.sql
echo "### 2. cost reconciliation";                 docker cp scripts/audit/p17_cost_reconciliation.py halal_equity_quant:/tmp/p17_cost.py && docker exec halal_equity_quant python /tmp/p17_cost.py
echo "### 3. strategy runtime config";             curl -s localhost:8000/api/v1/governance/strategy; echo
echo "### 4. quant tests";                          docker cp backend/migrations halal_equity_quant:/tmp/p17_migrations && docker exec -e P17_MIGRATIONS_DIR=/tmp/p17_migrations halal_equity_quant pytest -q -rs -p no:cacheprovider
echo "### 5. go tests";                             (cd backend && go vet ./... && go test -count=1 ./...)
echo "### 6. frontend build";                       (cd frontend && npm run build)
echo "### 7. safety flags";                         docker exec halal_equity_backend env | grep -E "PAPER_TRADING|LIVE_ORDER|BROKER_TRADING"; docker exec halal_equity_worker env | grep -E "PAPER_TRADING|LIVE_ORDER|BROKER_TRADING"; docker exec halal_equity_quant env | grep -E "PAPER_TRADING|LIVE_ORDER|BROKER_TRADING"
