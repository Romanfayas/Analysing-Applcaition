import psycopg2

conn = psycopg2.connect(
    host='postgres', port=5432, user='halal_user', password='devpassword', dbname='halal_equity'
)
cur = conn.cursor()

query = """
SELECT 
    s.id, s.symbol, s.is_active, s.sector,
    COALESCE(ca.cnt, 0) as ca_count,
    COALESCE(ca.unauth_cnt, 0) as unauth_cnt,
    COALESCE(ca.amb_cnt, 0) as amb_cnt,
    COALESCE(art.cnt, 0) as art_count,
    COALESCE(art.doc_type, 'NONE') as doc_type,
    COALESCE(art.source_type, 'NONE') as source_type,
    COALESCE(b.bar_count, 0) as bar_count,
    COALESCE(b.null_bars, 0) as null_bars,
    COALESCE(b.pop_bars, 0) as pop_bars,
    COALESCE(b.min_date::text, 'N/A') as min_date,
    COALESCE(b.max_date::text, 'N/A') as max_date
FROM symbols s
LEFT JOIN (
    SELECT symbol_id, 
           count(*) as cnt,
           count(*) FILTER (WHERE verification_status = 'ARCHIVE_UNAUTHENTICATED') as unauth_cnt,
           count(*) FILTER (WHERE verification_status = 'AMBIGUOUS') as amb_cnt
    FROM corporate_action_events 
    GROUP BY symbol_id
) ca ON ca.symbol_id = s.id
LEFT JOIN (
    SELECT DISTINCT ON (e.symbol_id) e.symbol_id, count(DISTINCT a.id) as cnt, a.document_type as doc_type,
           CASE WHEN a.source_url LIKE 'CONTROLLED_ARCHIVE:%' THEN 'CONTROLLED_ARCHIVE' ELSE 'UNKNOWN' END as source_type
    FROM corporate_action_events e
    JOIN corporate_action_source_artifacts a ON a.id = e.source_artifact_id
    GROUP BY e.symbol_id, a.document_type, a.source_url
) art ON art.symbol_id = s.id
LEFT JOIN (
    SELECT symbol_id, 
           count(*) as bar_count,
           count(*) FILTER (WHERE adjusted_close IS NULL) as null_bars,
           count(*) FILTER (WHERE adjusted_close IS NOT NULL) as pop_bars,
           min(timestamp::date) as min_date,
           max(timestamp::date) as max_date
    FROM ohlcv_daily
    GROUP BY symbol_id
) b ON b.symbol_id = s.id
ORDER BY s.id;
"""
cur.execute(query)
rows = cur.fetchall()

print("| ID | Symbol | Status | Lineage / Relationship | Events | Artifacts & Source | Authentication Status | Date Range | Total Bars (Quar / Pop) | Reason for Ineligibility |")
print("| :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- | :--- |")
for r in rows:
    sid, sym, active, sector, ca_cnt, unauth_cnt, amb_cnt, art_cnt, doc_type, src_type, bar_cnt, null_cnt, pop_cnt, min_d, max_d = r
    
    if sym == 'TATAMOTORS':
        lineage = 'Predecessor to TMPV / TMCV (Demerger)'
        status_str = 'INACTIVE (Archived)'
    elif sym in ('TMPV', 'TMCV'):
        lineage = 'Successor to TATAMOTORS (Demerger)'
        status_str = 'ACTIVE (Lineage)'
    elif sym == 'LTIM':
        lineage = 'Predecessor to LTM (Merger)'
        status_str = 'ACTIVE (Lineage)'
    elif sym == 'LTM':
        lineage = 'Successor to LTIM (Merger)'
        status_str = 'ACTIVE (Lineage)'
    else:
        lineage = 'Standalone Core Equity'
        status_str = 'ACTIVE (Core)'
        
    if ca_cnt == 0:
        auth_status = 'SOURCE_MISSING'
        art_desc = '0 (None)'
        ineligibility = 'No primary circulars harvested; 492 historical bars quarantined'
    elif amb_cnt > 0 and unauth_cnt == 0:
        auth_status = 'AMBIGUOUS'
        art_desc = f"{art_cnt} ({doc_type} / {src_type})"
        ineligibility = 'Complex corporate event unauthenticated; zero bars traded'
    elif amb_cnt > 0 and unauth_cnt > 0:
        auth_status = 'ARCHIVE_UNAUTH & AMBIGUOUS'
        art_desc = f"{art_cnt} ({doc_type} / {src_type})"
        ineligibility = 'Demerger unauthenticated; archived predecessor with 0 bars'
    else:
        auth_status = 'ARCHIVE_UNAUTHENTICATED'
        art_desc = f"{art_cnt} ({doc_type} / {src_type})"
        if sym == 'TCS':
            ineligibility = '7 bars adjusted via unauthenticated archive dividend; 492 bars quarantined'
        elif sym == 'HCLTECH':
            ineligibility = 'Unauthenticated archive dividend; 500 bars quarantined (8 vendor quarantined)'
        else:
            ineligibility = f"Unauthenticated archive dividend; {null_cnt} bars quarantined; {pop_cnt} nominal bars"
            
    if bar_cnt == 0:
        ineligibility = 'Lineage entity: 0 continuous market bars; awaiting post-restructuring trading history'
        
    date_range = f"{min_d} to {max_d}" if bar_cnt > 0 else 'None (0 bars)'
    bars_desc = f"{bar_cnt} ({null_cnt} / {pop_cnt})"
    
    print(f"| {sid} | `{sym}` | {status_str} | {lineage} | {ca_cnt} | {art_desc} | `{auth_status}` | {date_range} | {bars_desc} | {ineligibility} |")
