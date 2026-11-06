"""Shared Shariah invariant for the release-gate tests (P1.9.25).

Earlier tests pinned exact counts (0 PASS / 20 FAIL / 31 DATA_INSUFFICIENT). Those counts encoded a defect - interest
income and receivables were never fetched, so no stock could pass - and they also drift with live market caps. The real
invariant is that every stored verdict is explained by its own figures.
"""
import os

import psycopg2

DB_URL = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
LIMITS = {"debt": 0.33, "cash": 0.33, "interest": 0.05, "recv": 0.49}


def assert_shariah_verdicts_consistent() -> int:
    """Check the latest screening of every active symbol against its own ratios. Returns how many were checked."""
    conn = psycopg2.connect(DB_URL)
    try:
        cur = conn.cursor()
        cur.execute("""
            SELECT s.symbol, sc.status::text, sc.business_activity_pass,
                   sc.debt_ratio, sc.cash_deposit_ratio, sc.interest_income_ratio, sc.receivables_ratio,
                   COALESCE(sc.details->>'verification_status', ''), COALESCE((sc.details->>'stale')::boolean, false)
            FROM (SELECT DISTINCT ON (symbol_id) * FROM shariah_screenings
                  ORDER BY symbol_id, screening_date DESC, id DESC) sc
            JOIN symbols s ON s.id = sc.symbol_id WHERE s.is_active
        """)
        rows = cur.fetchall()
    finally:
        conn.close()
    assert rows, "no active screenings found"
    for sym, status, biz, debt, cash, interest, recv, verification, stale in rows:
        vals = [debt, cash, interest, recv]
        complete = all(v is not None for v in vals)
        breached = any(v is not None and float(v) > lim for v, lim in zip(vals, LIMITS.values()))
        if status == "PASS":
            assert biz is True, f"{sym}: PASS needs a permissible business"
            assert complete, f"{sym}: PASS needs every ratio present"
            assert not breached, f"{sym}: PASS with a breached ratio"
            assert verification == "UNVERIFIED_THIRD_PARTY_AGGREGATOR", f"{sym}: a PASS from vendor figures must say so"
        elif status == "FAIL":
            assert biz is False or breached, f"{sym}: FAIL needs a prohibited business or a breached ratio"
        elif status == "DATA_INSUFFICIENT":
            assert not (biz is True and complete and not breached and not stale), \
                f"{sym}: complete, current, passing figures must not be recorded as DATA_INSUFFICIENT"
        else:
            raise AssertionError(f"{sym}: unexpected status {status}")
    return len(rows)
