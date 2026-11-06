"""
Fundamentals ingestion and Shariah screening runner.

The rules and evaluation live in `screening.py` (P1.9.25 replaced the earlier version, which never requested interest
income or receivables and therefore could not give any permissible business more than DATA_INSUFFICIENT).
"""
import logging
import os
import time
from typing import Dict, Iterable, Optional

import psycopg2

from src.engines.fundamental.screening import YahooFundamentals, load_rules, screen_and_store

logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
logger = logging.getLogger("verified_fundamentals")

DB_URL = os.environ.get("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")


def screen_symbols(tickers: Optional[Iterable[str]] = None) -> Dict[str, str]:
    """Screen the given tickers (default: every active symbol). Returns {ticker: status}."""
    conn = psycopg2.connect(DB_URL)
    try:
        cur = conn.cursor()
        if tickers:
            cur.execute("SELECT id, symbol FROM symbols WHERE is_active AND symbol = ANY(%s) ORDER BY symbol", (list(tickers),))
        else:
            cur.execute("SELECT id, symbol FROM symbols WHERE is_active ORDER BY symbol")
        rows = cur.fetchall()
        rules = load_rules(cur)
        client = YahooFundamentals()
        out: Dict[str, str] = {}
        for sym_id, sym in rows:
            ev = screen_and_store(cur, client, sym_id, sym, rules)
            conn.commit()  # one symbol's failure must not discard the others
            out[sym] = ev["status"]
            logger.info("[%s] %s: %s", ev["status"], sym, ev["reason"])
            time.sleep(0.4)  # be gentle with the public endpoint
        return out
    finally:
        conn.close()


def run_ingestion_and_screening() -> Dict[str, int]:
    statuses = screen_symbols()
    counts = {"PASS": 0, "FAIL": 0, "DATA_INSUFFICIENT": 0}
    for s in statuses.values():
        counts[s] = counts.get(s, 0) + 1
    logger.info("Screening outcomes: %s", counts)
    return counts


if __name__ == "__main__":
    run_ingestion_and_screening()
