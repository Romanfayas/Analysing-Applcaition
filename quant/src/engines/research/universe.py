"""
Universe Definition and Safe Coverage Management

Defines tiered universe models for NSE cash equities, tracks clean historical coverage,
and prevents unverified symbol expansion or candle fabrication.
"""

from dataclasses import dataclass, field
from typing import List, Dict, Optional, Any
from enum import Enum
import psycopg2
import os


class UniverseTier(str, Enum):
    TIER_1_CORE = "TIER_1_CORE"                # 47 Curated NIFTY 50 Large-Caps with verified 502-session history
    TIER_2_NEXT50 = "TIER_2_NEXT50"            # NIFTY Next 50 (50 liquid large/mid caps)
    TIER_3_MIDCAP100 = "TIER_3_MIDCAP100"      # NIFTY Midcap 100
    TIER_4_BROADER_NSE = "TIER_4_BROADER_NSE"  # Broader NSE Cash Equities (~2,000 listed securities)


# Curated, verified 47-symbol universe (Tier 1)
TIER_1_CORE_SYMBOLS = [
    "ADANIENT", "ADANIPORTS", "APOLLOHOSP", "ASIANPAINT", "AXISBANK",
    "BAJAJ-AUTO", "BAJAJFINSV", "BAJFINANCE", "BHARTIARTL", "BPCL",
    "BRITANNIA", "CIPLA", "COALINDIA", "DIVISLAB", "DRREDDY",
    "EICHERMOT", "GRASIM", "HCLTECH", "HDFCBANK", "HDFCLIFE",
    "HEROMOTOCO", "HINDALCO", "HINDUNILVR", "ICICIBANK", "INDUSINDBK",
    "INFY", "ITC", "JSWSTEEL", "KOTAKBANK", "LT",
    "M&M", "MARUTI", "NESTLEIND", "NTPC", "ONGC",
    "POWERGRID", "RELIANCE", "SBILIFE", "SBIN", "SUNPHARMA",
    "TATACONSUM", "TATASTEEL", "TCS", "TECHM", "TITAN",
    "ULTRACEMCO", "WIPRO"
]

# Corporate action tracked symbols (predecessors and successors)
CORPORATE_ACTION_TRACKED_SYMBOLS = {
    "LTIM": {
        "event": "SYMBOL_RENAME",
        "effective_date": "2026-02-27",
        "successor": "LTM",
        "notes": "Renamed from LTIM to LTM on NSE"
    },
    "TATAMOTORS": {
        "event": "DEMERGER",
        "effective_date": "2025-10-24",
        "successors": ["TMPV", "TMCV"],
        "cost_apportionment": {"TMPV": 0.6885, "TMCV": 0.3115},
        "notes": "Demerged into Passenger Vehicles (TMPV) and Commercial Vehicles (TMCV)"
    }
}


@dataclass
class SymbolCoverageDetail:
    symbol: str
    tier: str
    clean_bars_count: int
    min_date: Optional[str]
    max_date: Optional[str]
    missing_sessions_count: int
    data_integrity: str  # 'VERIFIED_CLEAN', 'INSUFFICIENT_BARS', 'MISSING_IN_DB'
    corporate_action_notes: Optional[str] = None


@dataclass
class UniverseCoverageReport:
    selected_tier: str
    total_symbols_in_tier: int
    reconciled_clean_symbols: int
    clean_daily_sessions_count: int
    total_clean_bars: int
    missing_sessions_count: int
    genuine_trading_sessions_count: int
    holiday_phantom_bars_count: int
    corporate_action_issues: List[Dict[str, Any]]
    feed_provenance: str
    disclosure: str
    arithmetic_reconciliation: str
    symbol_details: List[SymbolCoverageDetail]


class UniverseManager:
    """
    Manages universe configuration, database reconciliation, and safe coverage reporting.
    """

    def __init__(self, db_url: Optional[str] = None):
        self.db_url = db_url or os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")

    def get_tier_symbols(self, tier: UniverseTier) -> List[str]:
        if tier == UniverseTier.TIER_1_CORE:
            return list(TIER_1_CORE_SYMBOLS)
        elif tier == UniverseTier.TIER_2_NEXT50:
            # Representative subset for Tier 2 definition
            return [
                "ABB", "ADANIENSOL", "ADANIGREEN", "AMBUJACEM", "ATGL",
                "BANKBARODA", "BEL", "BOSCHLTD", "CANBK", "CHOLAFIN",
                "COLPAL", "DLF", "GAIL", "GODREJCP", "HAL",
                "HAVELLS", "ICICIGI", "ICICIPRULI", "IOC", "IRCTC",
                "JINDALSTEL", "JIOFIN", "LICI", "MARICO", "PIDILITIND",
                "PFC", "PNB", "RECLTD", "SHRIRAMFIN", "SIEMENS",
                "SRF", "TORNTPHARM", "TRENT", "TVSMOTOR", "UNITDSPR",
                "VBL", "VEDL", "ZOMATO", "ZYDUSLIFE"
            ]
        elif tier == UniverseTier.TIER_3_MIDCAP100:
            return ["AUBANK", "BHARATFORG", "FEDERALBNK", "IDFCFIRSTB", "POLYCAB", "VOLTAS"]
        elif tier == UniverseTier.TIER_4_BROADER_NSE:
            return list(TIER_1_CORE_SYMBOLS) + ["TMPV", "TMCV", "LTM"]
        return list(TIER_1_CORE_SYMBOLS)

    def audit_coverage(self, tier: UniverseTier = UniverseTier.TIER_1_CORE) -> UniverseCoverageReport:
        """Coverage computed from live DB queries against the versioned exchange calendar (P1.7).

        There is no fallback: if the database is unreachable this raises instead of reporting clean data.
        """
        from src.engines.research.data_audit import audit_daily

        symbols = self.get_tier_symbols(tier)
        conn = psycopg2.connect(self.db_url)
        try:
            audit = audit_daily(conn, symbol_filter=symbols)
            cur = conn.cursor()
            cur.execute("SELECT symbol FROM symbols WHERE symbol = ANY(%s)", (symbols,))
            known = {r[0] for r in cur.fetchall()}
        except RuntimeError:
            audit = None  # no rows at all for this tier
            known = set()
        finally:
            conn.close()

        per = audit["per_symbol"] if audit else {}
        details: List[SymbolCoverageDetail] = []
        clean_symbols = 0
        for sym in symbols:
            p = per.get(sym)
            corp = CORPORATE_ACTION_TRACKED_SYMBOLS.get(sym, {}).get("notes")
            if not p:
                details.append(SymbolCoverageDetail(sym, tier.value, 0, None, None, audit["expected_sessions_in_range"] if audit else 0,
                                                    "MISSING_IN_DB", corp))
                continue
            status = "VERIFIED_CLEAN" if not p["missing_sessions"] and not p["unexpected_dates"] else "GAPS_PRESENT"
            clean_symbols += status == "VERIFIED_CLEAN"
            details.append(SymbolCoverageDetail(sym, tier.value, p["bars"], audit["date_range"][0], audit["date_range"][1],
                                                len(p["missing_sessions"]), status, corp))

        corp_issues = [
            {"symbol": sym, "event": d["event"], "effective_date": d["effective_date"], "resolution": d["notes"]}
            for sym, d in CORPORATE_ACTION_TRACKED_SYMBOLS.items()
        ]
        total_bars = audit["total_bars"] if audit else 0
        expected = audit["expected_sessions_in_range"] if audit else 0
        missing = audit["missing_bars"] if audit else 0
        recon = (
            f"{total_bars} daily bars in ohlcv_daily for {len(per)} symbols; calendar {audit['calendar_version']} expects "
            f"{expected} sessions/symbol ({audit['expected_bars']} bars): missing={missing}, unexpected={audit['unexpected_bars']}, "
            f"holiday={audit['holiday_bars']}, zero-volume flat={audit['zero_volume_flat_bars']}, "
            f"quarantined(P1.7)={audit['quarantined_p17']}. Last bar {audit['last_bar_date']}."
        ) if audit else "No daily bars found for this tier."
        disclosure = (
            f"{clean_symbols}/{len(symbols)} symbols have complete session coverage in {tier.value}. "
            "adjusted_close is NULL (provider-adjusted series not stored); 'close' is the provider's split-adjusted, "
            "dividend-unadjusted close. Symbols affected by the TATAMOTORS demerger and LTIM->LTM rename are excluded from Tier 1. "
            "Calendar is hand-curated from broker-republished NSE circulars, not an official NSE feed."
        )
        return UniverseCoverageReport(
            selected_tier=tier.value,
            total_symbols_in_tier=len(symbols),
            reconciled_clean_symbols=clean_symbols,
            clean_daily_sessions_count=expected,
            total_clean_bars=total_bars,
            missing_sessions_count=missing,
            genuine_trading_sessions_count=expected,
            holiday_phantom_bars_count=audit["holiday_bars"] if audit else 0,
            corporate_action_issues=corp_issues,
            feed_provenance="TimescaleDB ohlcv_daily (live audit vs versioned NSE calendar)",
            disclosure=disclosure,
            arithmetic_reconciliation=recon,
            symbol_details=details,
        )
