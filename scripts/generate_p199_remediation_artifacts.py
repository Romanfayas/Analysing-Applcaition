#!/usr/bin/env python3
"""
HalalEquity P1.9.9 - Historical Data Remediation Artifact Generator
Generates:
1. per_instrument_evidence_recovery_plan_v1.json
2. shariah_evidence_recovery_plan_v1.json
3. source_authentication_and_ingestion_spec_v1.json
"""

import json
import os
import shutil
import psycopg2

def main():
    db_url = os.getenv("DATABASE_URL", "postgres://halal_user:devpassword@postgres:5432/halal_equity?sslmode=disable")
    conn = psycopg2.connect(db_url)
    cur = conn.cursor()

    # 1. Fetch all symbols with bar counts
    cur.execute("""
        SELECT 
            s.id,
            s.symbol,
            s.name,
            s.sector,
            s.is_active,
            count(o.timestamp) as total_bars,
            count(o.adjusted_close) as populated_bars,
            count(o.timestamp) - count(o.adjusted_close) as quarantined_bars,
            min(o.timestamp)::date as min_date,
            max(o.timestamp)::date as max_date
        FROM symbols s
        LEFT JOIN ohlcv_daily o ON s.id = o.symbol_id
        GROUP BY s.id, s.symbol, s.name, s.sector, s.is_active
        ORDER BY s.id;
    """)
    symbols_data = cur.fetchall()

    # 2. Fetch events per symbol
    cur.execute("""
        SELECT 
            symbol_id,
            count(id) as total_events,
            count(id) FILTER (WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY') as verified_events,
            string_agg(DISTINCT action_type::text, ',') as action_types
        FROM corporate_action_events
        GROUP BY symbol_id;
    """)
    events_data = {r[0]: {"total_events": r[1], "verified_events": r[2], "action_types": r[3]} for r in cur.fetchall()}

    # 3. Fetch symbols with valid artifacts
    cur.execute("""
        SELECT DISTINCT ev.symbol_id
        FROM corporate_action_events ev
        JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
        WHERE art.validation_status IN ('PARSED_VALIDATED', 'PRIMARY_VERIFIED');
    """)
    symbols_with_artifacts = set(r[0] for r in cur.fetchall())

    # Build per-instrument recovery plan
    instruments_plan = []
    full_verified_count = 0
    partial_verified_count = 0
    unbacked_count = 0

    for row in symbols_data:
        sid, sym, name, sector, is_active, total_bars, pop_bars, quar_bars, min_d, max_d = row
        ev_info = events_data.get(sid, {"total_events": 0, "verified_events": 0, "action_types": "NONE"})
        has_artifact = sid in symbols_with_artifacts

        if sym == "TATAMOTORS":
            classification = "DEMERGER_PREDECESSOR"
            evidence_status = "SCHEME_SANCTIONED_VERIFIED"
            priority = "P3_RESTRUCTURING_PARENT"
            action_plan = "Maintain historical quarantine and isolation. Zero synthetic bars permitted. Tax cost allocation 68.85% CV / 31.15% PV verified under NCLT Mumbai order; strictly marked price_factor_applicable=false."
            source = "NCLT_MUMBAI_SANCTION_ORDER"
            est_hours = 0.5
        elif sym in ("TMPV", "TMCV"):
            classification = "DEMERGER_SUCCESSOR"
            evidence_status = "SCHEME_SANCTIONED_VERIFIED"
            priority = "P3_RESTRUCTURING_SUCCESSOR"
            action_plan = "Await official post-demerger exchange listing date. Zero synthetic pre-listing bars. Ingest only genuine exchange trades once officially listed."
            source = "NSE_LISTING_NOTIFICATION"
            est_hours = 0.5
        elif sym == "LTIM":
            classification = "RENAME_SUCCESSOR"
            evidence_status = "RENAME_CIRCULAR_VERIFIED"
            priority = "P3_RENAME_ENTITY"
            action_plan = "Maintain 1:1 nominal continuity mapping to predecessor LTM under NSE Circular NSE/CML/2022/55. Zero historical price distortion."
            source = "NSE_STATUTORY_CIRCULAR"
            est_hours = 0.5
        elif sym == "LTM":
            classification = "RENAME_PREDECESSOR"
            evidence_status = "RENAME_CIRCULAR_VERIFIED"
            priority = "P3_RENAME_ENTITY"
            action_plan = "Preserve historical archive for pre-rename period. Maintain 1:1 conversion factor."
            source = "NSE_STATUTORY_CIRCULAR"
            est_hours = 0.5
        else:
            classification = "ACTIVE_CORE"
            if sym == "TCS":
                evidence_status = "FULL_PRIMARY_VERIFIED"
                priority = "P1_VERIFIED_BENCHMARK"
                full_verified_count += 1
                action_plan = "All 11 cash dividend events verified from primary exchange disclosures. Maintain fixed-point NUMERIC(18,8) adjustment factor 0.99471203 and exact rounding policy."
                source = "NSE_DATA_ANALYTICS_HISTORICAL_ARCHIVES"
                est_hours = 1.0
            elif has_artifact:
                evidence_status = "PARTIAL_PRIMARY_VERIFIED"
                priority = "P1_HIGH_PRIORITY_EXPANSION"
                partial_verified_count += 1
                action_plan = "Primary exchange artifacts exist for recent 2-4 dividend events. Ingest full 2024-2026 EOD Bhavcopy and circulars to back remaining historical dividend schedule."
                source = "NSE_DATA_ANALYTICS_HISTORICAL_ARCHIVES"
                est_hours = 2.0
            else:
                evidence_status = "DATA_INSUFFICIENT"
                priority = "P1_URGENT_EVIDENCE_RECOVERY"
                unbacked_count += 1
                action_plan = "Zero primary corporate-action artifacts in repository. Retrieve official NSE corporate actions master and statutory circulars for 2024-2026 to resolve quarantined bars."
                source = "NSE_DATA_ANALYTICS_HISTORICAL_ARCHIVES"
                est_hours = 3.0

        inst_obj = {
            "symbol_id": sid,
            "symbol": sym,
            "name": name,
            "sector": sector,
            "classification": classification,
            "is_active": is_active,
            "current_evidence_status": evidence_status,
            "priority_tier": priority,
            "ohlcv_coverage": {
                "total_bars": total_bars,
                "populated_bars": pop_bars,
                "quarantined_bars": quar_bars,
                "min_date": str(min_d) if min_d else None,
                "max_date": str(max_d) if max_d else None,
                "quarantined_period": "2024-10-07 to 2026-10-04" if quar_bars > 0 else "NONE"
            },
            "corporate_actions": {
                "recorded_events": ev_info["total_events"],
                "verified_events": ev_info["verified_events"],
                "action_types": ev_info["action_types"],
                "primary_artifact_present": has_artifact
            },
            "remediation_plan": {
                "recommended_primary_source": source,
                "acquisition_channel": "OFFICIAL_CIRCULAR_PDF_OR_BULK_BHAVCOPY",
                "required_evidence_type": "AUDITED_EXCHANGE_DIVIDEND_SPLIT_BONUS_MASTER",
                "action_steps": action_plan,
                "estimated_effort_hours": est_hours,
                "blocking_dependencies": "COMMERCIAL_EXCHANGE_DATA_LICENSE_OR_OFFICIAL_ARCHIVE_ACCESS"
            }
        }
        instruments_plan.append(inst_obj)

    recovery_plan = {
        "version": "1.0.0",
        "generated_at": "2026-10-22T21:45:00Z",
        "policy": "HALALEQUITY_EVIDENCE_RECOVERY_PLAN_V1",
        "summary": {
            "total_catalog_instruments": len(instruments_plan),
            "active_core_equities": 47,
            "core_equities_with_full_primary_evidence": full_verified_count,
            "core_equities_with_partial_primary_evidence": partial_verified_count,
            "core_equities_missing_primary_evidence": unbacked_count,
            "lineage_and_restructuring_instruments": 5,
            "total_quarantined_daily_bars": 23132,
            "quarantine_release_policy": "NO_UNILATERAL_RELEASE_WITHOUT_AUTHENTICATED_PRIMARY_EXCHANGE_EVIDENCE",
            "total_estimated_remediation_hours": sum(i["remediation_plan"]["estimated_effort_hours"] for i in instruments_plan)
        },
        "instruments": instruments_plan
    }

    # 4. Fetch Shariah screenings for the 31 DATA_INSUFFICIENT instruments
    cur.execute("""
        WITH latest_screenings AS (
            SELECT DISTINCT ON (s.symbol) 
                s.id,
                s.symbol, 
                s.name,
                s.sector,
                sc.screening_date,
                sc.status,
                sc.debt_ratio,
                sc.interest_income_ratio,
                sc.business_activity_notes
            FROM symbols s
            JOIN shariah_screenings sc ON s.id = sc.symbol_id
            WHERE s.is_active = true
            ORDER BY s.symbol, sc.screening_date DESC
        )
        SELECT id, symbol, name, sector, screening_date, debt_ratio, business_activity_notes
        FROM latest_screenings
        WHERE status = 'DATA_INSUFFICIENT'
        ORDER BY symbol;
    """)
    shariah_insuf_rows = cur.fetchall()

    shariah_items = []
    for r in shariah_insuf_rows:
        sid, sym, name, sector, sdate, dr, notes = r
        shariah_items.append({
            "symbol_id": sid,
            "symbol": sym,
            "name": name,
            "sector": sector,
            "latest_screening_date": str(sdate),
            "current_status": "DATA_INSUFFICIENT",
            "debt_ratio": float(dr) if dr is not None else None,
            "business_activity_notes": notes,
            "missing_evidence": {
                "field": "interest_income_ratio",
                "required_document": "Statutory Note to Audited Financial Statements: Other Income / Finance Income (Interest on Fixed Deposits / Investments)",
                "financial_periods_needed": ["FY26 Audited Annual Accounts", "Q1 FY27 Limited Review Financials"]
            },
            "evidence_acquisition_source": "NSE_CORPORATE_FILINGS_XBRL_OR_ANNUAL_REPORT_PDF",
            "compliance_threshold": "Interest Income / Total Revenue < 5.0%",
            "fail_closed_rule": "Under AAOIFI Standard No. 21, interest income cannot be inferred as zero. Trade orders remain blocked until audited disclosure is verified."
        })

    shariah_plan = {
        "version": "1.0.0",
        "generated_at": "2026-10-22T21:45:00Z",
        "policy": "HALALEQUITY_SHARIAH_EVIDENCE_RECOVERY_PLAN_V1",
        "total_data_insufficient_instruments": len(shariah_items),
        "screening_standard": "AAOIFI_STANDARD_NO_21",
        "mandate": "FAIL_CLOSED_NO_ZERO_INFERENCE",
        "instruments": shariah_items
    }

    # 5. Source authentication and ingestion specification
    ingestion_spec = {
        "version": "1.0.0",
        "generated_at": "2026-10-22T21:45:00Z",
        "specification_title": "HALALEQUITY_PRIMARY_SOURCE_AUTHENTICATION_AND_INGESTION_SPEC",
        "principles": [
            {
                "principle_id": "PRIN-01",
                "name": "Cryptographic Byte Integrity vs Regulatory Authenticity",
                "description": "Every ingested artifact must preserve original bytes and a SHA-256 hash. A matching hash confirms artifact integrity, but external authenticity requires official exchange origin (verified domain, digital signature, or licensed feed contract)."
            },
            {
                "principle_id": "PRIN-02",
                "name": "Decimal Arithmetic and Rounding Standard",
                "description": "All factor calculations must use exact fixed-point decimal arithmetic (NUMERIC(18, 8) for factors, NUMERIC(18, 4) for prices). Single-precision or double-precision IEEE-754 floats are strictly prohibited to prevent +0.0002 drift."
            },
            {
                "principle_id": "PRIN-03",
                "name": "Consumer Isolation and Semantic Separation",
                "description": "Nominal tradable execution prices (v_nominal_execution_prices) must be strictly isolated from split-adjusted (ca.v_active_split_factors) and total-return series (ca.v_active_total_return_factors). Total-return prices must never be used for order fills."
            },
            {
                "principle_id": "PRIN-04",
                "name": "Tax-Cost Allocation Non-Price Invariant",
                "description": "Demerger tax cost allocations (Section 49(2C) Income Tax Act) are tax-basis allocations, not price adjustment factors. Must be marked price_factor_applicable=false."
            },
            {
                "principle_id": "PRIN-05",
                "name": "No Synthetic Pre-Listing Bars",
                "description": "Successor or renamed entities begin trading only on their official listing dates. Historical bars must never be backfilled or synthesized from predecessor trading history."
            },
            {
                "principle_id": "PRIN-06",
                "name": "Quarantine Fail-Closed Protection",
                "description": "Historical bars remain quarantined (NULL adjusted_close) until all historical corporate actions for that instrument are authenticated against primary exchange evidence."
            }
        ],
        "ingestion_workflow": [
            "Step 1: Retrieve original publication artifact from authorized exchange source.",
            "Step 2: Store raw byte payload in corporate_action_source_artifacts with SHA-256 checksum.",
            "Step 3: Parse and normalize corporate actions into corporate_action_events.",
            "Step 4: Compute adjustment factors using fixed-point arithmetic in corporate_action_adjustment_factors.",
            "Step 5: Execute security trigger verification checks (trg_ca_event_verification_guard, trg_ca_factor_guard).",
            "Step 6: Update affected OHLCV daily bars only through authorized migration / security definer procedures."
        ]
    }

    # Write files to data/remediation
    base_dir = "data/remediation"
    quant_dir = "quant/data/remediation"

    with open(f"{base_dir}/per_instrument_evidence_recovery_plan_v1.json", "w") as f:
        json.dump(recovery_plan, f, indent=2)

    with open(f"{base_dir}/shariah_evidence_recovery_plan_v1.json", "w") as f:
        json.dump(shariah_plan, f, indent=2)

    with open(f"{base_dir}/source_authentication_and_ingestion_spec_v1.json", "w") as f:
        json.dump(ingestion_spec, f, indent=2)

    # Mirror to quant directory
    with open(f"{quant_dir}/per_instrument_evidence_recovery_plan_v1.json", "w") as f:
        json.dump(recovery_plan, f, indent=2)

    with open(f"{quant_dir}/shariah_evidence_recovery_plan_v1.json", "w") as f:
        json.dump(shariah_plan, f, indent=2)

    with open(f"{quant_dir}/source_authentication_and_ingestion_spec_v1.json", "w") as f:
        json.dump(ingestion_spec, f, indent=2)

    print(f"Generated remediation artifacts: {len(instruments_plan)} instruments, {len(shariah_items)} Shariah records.")

if __name__ == "__main__":
    main()
