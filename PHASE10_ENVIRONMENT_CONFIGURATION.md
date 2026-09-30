# PHASE 10 ENVIRONMENT CONFIGURATION

The target production-like paper environment must be configured exactly as follows to guarantee capital safety while providing realistic observation.

```env
# ==== APP MODE ====
APP_ENV=production
ENVIRONMENT=paper

# ==== TRADING ENGINE ====
PAPER_TRADING_ENABLED=true
LIVE_ORDER_EXECUTION=false
BROKER_TRADING_ENABLED=false

# ==== DATABASE ====
# Must point to the TimescaleDB instance defined in docker-compose
DATABASE_URL=postgres://halal_user:securepassword@postgres:5432/halal_equity?sslmode=disable
REDIS_URL=redis://redis:6379/0

# ==== STRATEGY PARAMS (IMMUTABLE) ====
# These are enforced in code, but documented here for transparency:
# BUY_THRESHOLD=80
# SELL_THRESHOLD=45
# ATR_MULTIPLIER=2.0
# UNIVERSE=NIFTY50

# ==== SECURITY ====
# Ensure JWT_SECRET is strong and not exposed to the frontend.
JWT_SECRET=super_secure_production_secret
JWT_EXPIRY_HOURS=24
```

### Security Considerations
- **NEVER** expose secrets through `NEXT_PUBLIC_*` variables.
- API keys, database credentials, JWT secrets, and provider secrets must remain exclusively on the backend/quant engine.
