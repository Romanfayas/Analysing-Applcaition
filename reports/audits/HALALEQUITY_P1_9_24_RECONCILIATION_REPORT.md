# HalalEquity P1.9.24 — Vendor vs Exchange Daily Reconciliation

Generated 2026-11-02T09:25:58.841049+00:00 by `scripts/p1924_reconciliation_report.py` (read-only). Exchange record: NSE CM
bhavcopies; vendor record: `ohlcv_daily` (Yahoo). Data: `HALALEQUITY_P1_9_24_RECONCILIATION_REPORT.json`.

## Coverage
- Exchange files: **1196** (2022-01-03 → 2026-10-30); exchange bars: **58615** across 52 instruments.
- Vendor bars: **24017** (2024-10-07 → 2026-10-30), 47 instruments.
- **Vendor history begins 2024-10-07.** Exchange evidence for 2022-01-03 → 2024-10-07 has no vendor counterpart to reconcile; it is authenticated *nominal* coverage only.
- Weekdays with vendor bars but no exchange file loaded: 0.

| y | files |
|---|---|
| 2022 | 248 |
| 2023 | 246 |
| 2024 | 249 |
| 2025 | 249 |
| 2026 | 204 |

## Classification in the overlap (2024-10-07 → 2026-10-30)
| classification | n |
|---|---|
| NOMINAL_MATCH | 22928 |
| MISSING_VENDOR_BAR | 1310 |
| VENDOR_ADJUSTED | 1049 |
| MISMATCH | 40 |

- **Matched:** 22928 bars agree on open/high/low/close within 0.05%; 20335 have an identical close.

## Duplicates
- Vendor bars sharing a symbol and IST date: **0**.
- Exchange: 0 by construction: (symbol_id, trade_date) is the primary key of exchange_daily_bars.
_none_

## Timestamps
Vendor timestamp convention (UTC time of day):
| utc_time | n |
|---|---|
| 03:45:00 | 24017 |

- Vendor bars on dates with no exchange session file: **0**
_none_

- Vendor bars whose close matches an adjacent session (possible date shift): **1**
| symbol | vendor_date | exchange_date |
|---|---|---|
| JSWSTEEL | 2025-05-02 | 2025-05-06 |

## OHLCV differences on matched bars
| pairs | open_max_pct | high_max_pct | low_max_pct | close_max_pct | any_price_not_exact | volume_ratio_p05 | volume_ratio_median | volume_ratio_p95 | volume_diff_over_1pct |
|---|---|---|---|---|---|---|---|---|---|
| 22928 | 0.00000 | 0.00000 | 0.00000 | 0.04598 | 4328 | 1.0000 | 1.0000 | 1.0000 | 9 |

Close differences on matched bars:
| bucket | n | dates_with_real_differences |
|---|---|---|
| exact | 20335 |  |
| float storage noise (<0.0001%) | 2588 |  |
| <0.01% | 3 | 2025-05-02 |
| >=0.01% | 2 | 2025-05-02 |

### Vendor back-adjusted series (constant ratio to the traded price)
| symbol | first | last | bars | min_ratio | max_ratio |
|---|---|---|---|---|---|
| ADANIENT | 2025-11-10 | 2025-11-14 | 5 | 0.969485 | 0.969485 |
| BAJFINANCE | 2024-10-07 | 2025-06-13 | 170 | 0.100000 | 0.100068 |
| DRREDDY | 2024-10-07 | 2024-10-25 | 15 | 0.200000 | 0.200000 |
| HDFCBANK | 2024-10-07 | 2025-08-25 | 219 | 0.500000 | 0.500000 |
| ITC | 2024-10-07 | 2025-01-03 | 62 | 0.962915 | 0.962915 |
| KOTAKBANK | 2024-10-07 | 2026-01-13 | 316 | 0.199680 | 0.200000 |
| NESTLEIND | 2024-10-07 | 2025-08-07 | 208 | 0.500000 | 0.500000 |
| RELIANCE | 2024-10-07 | 2024-10-25 | 15 | 0.500000 | 0.500000 |
| WIPRO | 2024-10-07 | 2024-12-02 | 39 | 0.500000 | 0.500000 |

## Unresolved records
- Mismatches (prices disagree inconsistently): **40**
| symbol | trade_date | vendor_close | exchange_close | close_ratio |
|---|---|---|---|---|
| ADANIENT | 2025-05-02 | 2299.1001 | 2294.2000 | 1.002136 |
| ADANIPORTS | 2025-05-02 | 1263.1000 | 1267.1000 | 0.996843 |
| APOLLOHOSP | 2025-05-02 | 6928.0000 | 6954.5000 | 0.996190 |
| AXISBANK | 2025-05-02 | 1183.4000 | 1182.2000 | 1.001015 |
| BAJAJ-AUTO | 2025-05-02 | 7820.5000 | 7836.5000 | 0.997958 |
| BAJAJFINSV | 2025-05-02 | 1966.7000 | 1958.0000 | 1.004443 |
| BHARTIARTL | 2025-05-02 | 1848.2000 | 1851.9000 | 0.998002 |
| BPCL | 2025-05-02 | 311.4000 | 311.1500 | 1.000803 |
| BRITANNIA | 2025-05-02 | 5301.0000 | 5307.5000 | 0.998775 |
| CIPLA | 2025-05-02 | 1514.5000 | 1525.6000 | 0.992724 |
| COALINDIA | 2025-05-02 | 383.5000 | 384.4000 | 0.997659 |
| DIVISLAB | 2025-05-02 | 6079.5000 | 6104.0000 | 0.995986 |
| DRREDDY | 2025-05-02 | 1177.9000 | 1182.9000 | 0.995773 |
| GRASIM | 2025-05-02 | 2724.0000 | 2718.3000 | 1.002097 |
| HCLTECH | 2025-05-02 | 1572.4000 | 1576.5000 | 0.997399 |
| HDFCBANK | 2025-05-02 | 960.9500 | 1925.0000 | 0.499195 |
| HDFCLIFE | 2025-05-02 | 729.1500 | 727.7500 | 1.001924 |
| HEROMOTOCO | 2025-05-02 | 3737.3999 | 3740.5000 | 0.999171 |
| HINDALCO | 2025-05-02 | 630.5000 | 632.1000 | 0.997469 |
| HINDUNILVR | 2025-05-02 | 2318.6001 | 2323.9000 | 0.997719 |
| ICICIBANK | 2025-05-02 | 1430.1000 | 1432.4000 | 0.998394 |
| INDUSINDBK | 2025-05-02 | 856.4500 | 853.0000 | 1.004045 |
| ITC | 2025-05-02 | 429.5000 | 430.2500 | 0.998257 |
| JSWSTEEL | 2025-05-02 | 967.5000 | 973.2000 | 0.994143 |
| LT | 2025-05-02 | 3334.8999 | 3329.8000 | 1.001532 |
| M&M | 2025-05-02 | 2917.2000 | 2926.2000 | 0.996924 |
| NESTLEIND | 2025-05-02 | 1165.9000 | 2337.7000 | 0.498738 |
| NTPC | 2025-05-02 | 348.7000 | 348.4500 | 1.000717 |
| ONGC | 2025-05-02 | 242.7400 | 243.4200 | 0.997206 |
| POWERGRID | 2025-05-02 | 303.0500 | 303.9500 | 0.997039 |
| RELIANCE | 2025-05-02 | 1419.8000 | 1422.4000 | 0.998172 |
| SBILIFE | 2025-05-02 | 1762.2000 | 1764.7000 | 0.998583 |
| SBIN | 2025-05-02 | 796.7000 | 800.0000 | 0.995875 |
| SUNPHARMA | 2025-05-02 | 1824.6000 | 1829.4000 | 0.997376 |
| TATACONSUM | 2025-05-02 | 1154.7000 | 1156.8000 | 0.998185 |
| TATASTEEL | 2025-05-02 | 141.4000 | 141.1800 | 1.001558 |
| TCS | 2025-05-02 | 3431.0000 | 3444.7000 | 0.996023 |
| TECHM | 2025-05-02 | 1494.1000 | 1496.3000 | 0.998530 |
| ULTRACEMCO | 2025-05-02 | 11613.0000 | 11644.0000 | 0.997338 |
| WIPRO | 2025-05-02 | 241.6800 | 242.8700 | 0.995100 |

- Exchange bars with no vendor bar, inside the overlap:
| symbol | n | first | last |
|---|---|---|---|
| LTIM | 347 | 2024-10-07 | 2026-02-26 |
| TATAMOTORS | 261 | 2024-10-07 | 2025-10-23 |
| TMPV | 251 | 2025-10-24 | 2026-10-30 |
| TMCV | 239 | 2025-11-12 | 2026-10-30 |
| LTM | 165 | 2026-02-27 | 2026-10-30 |
| M&M | 2 | 2025-03-18 | 2025-09-08 |
| BAJAJFINSV | 1 | 2025-03-18 | 2025-03-18 |
| BAJFINANCE | 1 | 2025-03-18 | 2025-03-18 |
| BHARTIARTL | 1 | 2025-03-18 | 2025-03-18 |
| BPCL | 1 | 2025-03-18 | 2025-03-18 |
| BRITANNIA | 1 | 2025-03-18 | 2025-03-18 |
| CIPLA | 1 | 2025-03-18 | 2025-03-18 |
| COALINDIA | 1 | 2025-03-18 | 2025-03-18 |
| DIVISLAB | 1 | 2025-03-18 | 2025-03-18 |
| DRREDDY | 1 | 2025-03-18 | 2025-03-18 |
| EICHERMOT | 1 | 2025-03-18 | 2025-03-18 |
| GRASIM | 1 | 2025-03-18 | 2025-03-18 |
| HCLTECH | 1 | 2025-03-18 | 2025-03-18 |
| HDFCBANK | 1 | 2025-03-18 | 2025-03-18 |
| HDFCLIFE | 1 | 2025-03-18 | 2025-03-18 |
| HEROMOTOCO | 1 | 2025-03-18 | 2025-03-18 |
| HINDALCO | 1 | 2025-03-18 | 2025-03-18 |
| HINDUNILVR | 1 | 2025-03-18 | 2025-03-18 |
| ICICIBANK | 1 | 2025-03-18 | 2025-03-18 |
| INDUSINDBK | 1 | 2025-03-18 | 2025-03-18 |
| INFY | 1 | 2025-03-18 | 2025-03-18 |
| JSWSTEEL | 1 | 2025-03-18 | 2025-03-18 |
| KOTAKBANK | 1 | 2025-03-18 | 2025-03-18 |
| LT | 1 | 2025-03-18 | 2025-03-18 |
| MARUTI | 1 | 2025-03-18 | 2025-03-18 |
| NESTLEIND | 1 | 2025-03-18 | 2025-03-18 |
| NTPC | 1 | 2025-03-18 | 2025-03-18 |
| ONGC | 1 | 2025-03-18 | 2025-03-18 |
| POWERGRID | 1 | 2025-03-18 | 2025-03-18 |
| RELIANCE | 1 | 2025-03-18 | 2025-03-18 |
| SBILIFE | 1 | 2025-03-18 | 2025-03-18 |
| SBIN | 1 | 2025-03-18 | 2025-03-18 |
| SUNPHARMA | 1 | 2025-03-18 | 2025-03-18 |
| TATACONSUM | 1 | 2025-03-18 | 2025-03-18 |
| TATASTEEL | 1 | 2025-03-18 | 2025-03-18 |
| TCS | 1 | 2025-03-18 | 2025-03-18 |
| TECHM | 1 | 2025-03-18 | 2025-03-18 |
| TITAN | 1 | 2025-03-18 | 2025-03-18 |
| ULTRACEMCO | 1 | 2025-03-18 | 2025-03-18 |
| ADANIENT | 1 | 2025-03-18 | 2025-03-18 |
| WIPRO | 1 | 2025-03-18 | 2025-03-18 |
| ADANIPORTS | 1 | 2025-03-18 | 2025-03-18 |
| APOLLOHOSP | 1 | 2025-03-18 | 2025-03-18 |
| ASIANPAINT | 1 | 2025-03-18 | 2025-03-18 |
| AXISBANK | 1 | 2025-03-18 | 2025-03-18 |
| BAJAJ-AUTO | 1 | 2025-03-18 | 2025-03-18 |

- Vendor bars on an exchange session day that the exchange file does not list: **0**
_none_

## Eligibility check
Research-eligible vendor bars compared with the exchange record:
| source | eligible_bars | exchange_nominal_match | exchange_disagree | adjusted_equals_close | first_date |
|---|---|---|---|---|---|
| NSE_CORPORATE_ACTION | 7 | 7 | 0 | 0 | 2026-10-05 |
| yahoo_finance | 878 | 878 | 0 | 878 | 2026-10-05 |

- Quarantined bars: **23132** (unchanged; no record was released).
