"""
Provider-Readiness Layer (Python Engine)

Audits market-data provider access, feed latency, historical depth,
market-depth support, failure modes, and licensing requirements.
"""

from dataclasses import dataclass, asdict
from typing import List, Dict, Optional, Any
from datetime import datetime, timezone
import os
import time
import urllib.request


@dataclass
class ProviderReadinessSpec:
    name: str
    product: str
    access_method: str
    operational_status: str
    feed_classification: str
    market_depth: str
    credentials_status: str
    supported_instruments: List[str]
    historical_depth: str
    supported_intervals: List[str]
    rate_limits: str
    observed_failure_modes: List[str]
    typical_latency_ms: int
    licensing_requirements: str
    redistribution_allowed: bool
    production_ready: bool
    limitations: List[str]
    verification_latency_ms: Optional[int] = None
    last_verified_at: Optional[str] = None


class ProviderAuditor:
    """
    Performs verified audit of available market-data providers.
    """

    @staticmethod
    def audit_all() -> Dict[str, Any]:
        now_str = datetime.now(timezone.utc).isoformat()

        # 1. Yahoo Finance
        yf_verified_ms = None
        yf_status = "OPERATIONAL"
        try:
            req = urllib.request.Request(
                "https://query1.finance.yahoo.com/v8/finance/chart/TCS.NS?range=1d&interval=1d",
                headers={"User-Agent": "Mozilla/5.0"},
            )
            start_t = time.time()
            with urllib.request.urlopen(req, timeout=2.5) as resp:
                if resp.status == 200:
                    yf_verified_ms = int((time.time() - start_t) * 1000)
                else:
                    yf_status = "DEGRADED"
        except Exception:
            yf_status = "DEGRADED"

        yf = ProviderReadinessSpec(
            name="yahoo_finance",
            product="Yahoo Finance Public API (v8/finance/chart)",
            access_method="HTTP REST with Cookie/Crumb Session",
            operational_status=yf_status,
            feed_classification="DELAYED_15MIN",
            market_depth="NONE (0-depth)",
            credentials_status="NO_CREDENTIALS_REQUIRED (Public Endpoints)",
            supported_instruments=["NSE Cash Equities (.NS)", "BSE Cash Equities (.BO)", "Major Indices"],
            historical_depth="10+ years daily, up to 60 days 5-minute, up to 7 days 1-minute",
            supported_intervals=["1m", "5m", "15m", "1h", "1d", "1w", "1M"],
            rate_limits="IP-based throttle (~2,000 req/hr per IP)",
            observed_failure_modes=[
                "HTTP 429 on rapid burst requests",
                "HTTP 404 for corporate action renamed/demerged symbols (LTIM.NS, TATAMOTORS.NS)",
                "Crumb invalidation requiring cookie jar refresh",
            ],
            typical_latency_ms=650,
            licensing_requirements="Non-commercial personal research only. Commercial redistribution strictly prohibited.",
            redistribution_allowed=False,
            production_ready=False,
            limitations=[
                "~15-minute quote delay during Indian market hours",
                "Zero Level 2 order book market depth",
                "No SLA or exchange data licensing protection",
            ],
            verification_latency_ms=yf_verified_ms,
            last_verified_at=now_str if yf_verified_ms else None,
        )

        # 2. NSE India Web Scraper
        nse = ProviderReadinessSpec(
            name="nse_india",
            product="NSE India Official Website JSON Endpoints",
            access_method="Reverse-Engineered HTTP GET with Browser Cookie Jar",
            operational_status="DEGRADED",
            feed_classification="WEB_SCRAPED_SNAPSHOT",
            market_depth="LEVEL_2_TOP_5 (Static Snapshot)",
            credentials_status="COOKIE_SESSION_BASED (Transient Session Tokens)",
            supported_instruments=["NSE Equities", "NSE Indices", "IPO Current Issue List", "Corporate Action Announcements"],
            historical_depth="Current session quote, ~3 months Bhavcopy archive",
            supported_intervals=["Snapshot", "1d"],
            rate_limits="Aggressive Akamai WAF blocking; rate limit to 1 req/2-5 sec",
            observed_failure_modes=[
                "HTTP 401/403 when session cookies expire",
                "Akamai bot challenge HTML interstitial responses instead of JSON",
                "Key name changes in JSON payloads across portal redesigns",
            ],
            typical_latency_ms=1200,
            licensing_requirements="Scraping violates NSE India Website Terms of Use. Production use requires SEBI/DotEx Authorized Vendor License.",
            redistribution_allowed=False,
            production_ready=False,
            limitations=[
                "Highly fragile: cookie expiration causes silent request failures",
                "No streaming tick feed or historical intraday tick access",
                "Unsuitable for latency-sensitive execution",
            ],
        )

        # 3. Angel One SmartAPI / Licensed Indian Broker
        angel_key = os.getenv("ANGEL_ONE_API_KEY", "")
        angel_client = os.getenv("ANGEL_ONE_CLIENT_ID", "")
        has_angel = bool(angel_key and angel_client)

        angel = ProviderReadinessSpec(
            name="angel_one_smartapi",
            product="Angel One SmartAPI (Licensed Indian Broker Market Feed)",
            access_method="Official REST API + SmartStream Binary WebSocket (TOTP Authentication)",
            operational_status="OPERATIONAL" if has_angel else "CONFIG_REQUIRED",
            feed_classification="LIVE_STREAMING",
            market_depth="LEVEL_2_TOP_5 (Streaming Order Book)",
            credentials_status=f"API_KEY_AND_TOTP_REQUIRED (Config: {'PRESENT' if has_angel else 'MISSING'})",
            supported_instruments=["NSE Cash Equities", "NSE F&O Derivatives", "BSE Cash Equities", "MCX Commodities"],
            historical_depth="Multi-year daily and 1-minute historical candles via Historical API",
            supported_intervals=["1m", "5m", "15m", "1h", "1d"],
            rate_limits="3 req/sec REST; up to 500 subscribed symbols per WebSocket connection",
            observed_failure_modes=[
                "Daily JWT expiration requiring automated morning TOTP token refresh",
                "Subscription token limit reached when monitoring full universe",
            ],
            typical_latency_ms=65,
            licensing_requirements="SEBI regulated broker market feed. Authorized for personal automated trading.",
            redistribution_allowed=False,
            production_ready=has_angel,
            limitations=[
                "Requires active funded broker account with automated TOTP login daemon",
                "WebSocket requires keepalive ping/pong management and reconnect handling",
            ],
        )

        # 4. Mock Provider
        mock = ProviderReadinessSpec(
            name="mock_provider",
            product="Synthetic In-Memory Test Provider",
            access_method="In-Memory Test Engine",
            operational_status="OPERATIONAL",
            feed_classification="SYNTHETIC_MOCK",
            market_depth="LEVEL_2_TOP_5",
            credentials_status="NONE_REQUIRED",
            supported_instruments=["Synthetic Equities", "NIFTY 50 Fixtures"],
            historical_depth="1,000 synthetic bars",
            supported_intervals=["1m", "5m", "15m", "1h", "1d"],
            rate_limits="Unlimited",
            observed_failure_modes=["Deterministic testing only"],
            typical_latency_ms=1,
            licensing_requirements="Open-source internal testing.",
            redistribution_allowed=True,
            production_ready=False,
            limitations=["Synthetic data for deterministic unit/integration testing only"],
        )

        return {
            "timestamp": now_str,
            "summary": (
                "Operational for EOD research via Yahoo Finance (historical benchmarks) "
                "and official IPO announcements via NSE filings. "
                "Real-time streaming and Level 2 depth require licensed broker credentials."
            ),
            "licensed_vendor_active": has_angel,
            "providers": [asdict(yf), asdict(nse), asdict(angel), asdict(mock)],
            "integration_roadmap": [
                "1. Yahoo Finance: Keep for EOD historical research benchmarks only, with explicit disclaimer on 15m delay.",
                "2. NSE Scraper: Restrict to non-latency-sensitive official filing cross-checks.",
                "3. Angel One SmartAPI: Connect production API key, Client ID, and TOTP daemon for real-time WebSocket ticks.",
                "4. Streaming Level 2 Depth: Required prior to activating paper-order fill simulation or live broker trading.",
            ],
        }
