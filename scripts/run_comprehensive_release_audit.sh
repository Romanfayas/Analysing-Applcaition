#!/usr/bin/env bash
set -euo pipefail
# HalalEquity Comprehensive Release Readiness Runner
# Executes verify_comprehensive_release_readiness.py inside halal_equity_quant container
docker exec -i \
  -e ADMIN_DATABASE_URL="postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable" \
  -e FRONTEND_PROXY_URL="http://frontend:3000/api/proxy" \
  halal_equity_quant python3 < "$(dirname "$0")/verify_comprehensive_release_readiness.py"
