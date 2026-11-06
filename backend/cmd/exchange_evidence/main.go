// Command exchange_evidence is the OPERATOR tool that acquires exchange evidence and reconciles it.
//
//	exchange_evidence bhavcopy  -from 2024-10-01 -to 2026-10-30   load NSE bhavcopies (exchange end-of-day record)
//	exchange_evidence ca-feed   [-from 2024-09-01] [-symbols A,B]  store NSE corporate-actions feed artifacts
//	exchange_evidence reconcile [-from 2024-09-01]                record a canonical-vs-exchange reconciliation run
//	exchange_evidence filings                                     check primary filings for price-changing actions
//
// It writes only append-only evidence tables. Download, HTTPS and SHA-256 establish provenance and integrity of
// the bytes received; they are NOT treated as proof that a document is authentic.
// It writes only append-only evidence tables. It never modifies ohlcv_daily, corporate_action_events or
// adjustment factors, and it never promotes anything: feed artifacts are stored as AUTHENTICITY_PENDING and
// promotion remains a human reviewer's decision. Run it with an operator role (the runtime role cannot write
// evidence). If the exchange refuses the client (HTTP 401/403) it stops; it never impersonates a browser.
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/evidence"
	"github.com/halal-equity/backend/internal/providers/corporateactions/official"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: exchange_evidence bhavcopy|ca-feed|reconcile [flags]")
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fromS := fs.String("from", "2024-09-01", "first date (YYYY-MM-DD)")
	toS := fs.String("to", time.Now().Format("2006-01-02"), "last date (YYYY-MM-DD)")
	symbolsS := fs.String("symbols", "", "comma-separated tickers (default: every instrument with history)")
	delay := fs.Duration("delay", 1200*time.Millisecond, "minimum delay between requests to the exchange")
	weekends := fs.Bool("weekends", false, "also request Saturdays/Sundays (special sessions, e.g. Budget day 2025-02-01, Muhurat trading)")
	_ = fs.Parse(args)

	from, err1 := time.Parse("2006-01-02", *fromS)
	to, err2 := time.Parse("2006-01-02", *toS)
	if err1 != nil || err2 != nil || to.Before(from) {
		log.Fatalf("invalid -from/-to")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL (operator role) is required")
	}
	db, err := database.NewDB(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()
	pipe := official.NewPipeline(*delay)

	switch cmd {
	case "bhavcopy":
		st, err := evidence.LoadBhavcopies(ctx, db.DB, pipe, from, to, *weekends)
		fmt.Printf("bhavcopy: loaded=%d already_present=%d no_session(404)=%d failed=%d\n", st.Loaded, st.Skipped, st.NoSession, st.Failed)
		if err != nil {
			log.Fatal(err)
		}
	case "ca-feed":
		storeFeeds(ctx, db, pipe, from, to, *symbolsS)
	case "filings":
		checkFilings(ctx, db, pipe)
	case "reconcile":
		runID, summary, err := evidence.Reconcile(ctx, db.DB, from, to)
		if err != nil {
			log.Fatalf("reconcile: %v", err)
		}
		out, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Printf("reconciliation run %d\n%s\n", runID, out)
	default:
		log.Fatalf("unknown command %q", cmd)
	}
}

func storeFeeds(ctx context.Context, db *database.DB, pipe *official.Pipeline, from, to time.Time, symbolsS string) {
	var symbols []string
	if symbolsS != "" {
		for _, s := range strings.Split(symbolsS, ",") {
			if s = strings.TrimSpace(s); s != "" {
				symbols = append(symbols, s)
			}
		}
	} else {
		rows, err := db.QueryContext(ctx, `SELECT s.symbol FROM symbols s WHERE s.exchange = 'NSE' AND (
			EXISTS (SELECT 1 FROM ohlcv_daily d WHERE d.symbol_id = s.id) OR
			EXISTS (SELECT 1 FROM corporate_action_events e WHERE e.symbol_id = s.id)) ORDER BY 1`)
		if err != nil {
			log.Fatal(err)
		}
		for rows.Next() {
			var s string
			_ = rows.Scan(&s)
			symbols = append(symbols, s)
		}
		rows.Close()
	}
	stored, unchanged, failed := 0, 0, 0
	for _, sym := range symbols {
		u := evidence.FeedURL(sym, from, to)
		body, meta, err := pipe.AcquireFromOfficialURL(ctx, u)
		if errors.Is(err, official.ErrAccessRestricted) {
			log.Fatalf("exchange refused the client for %s: %v (stopping; no circumvention)", sym, err)
		}
		if err != nil {
			failed++
			log.Printf("%s: %v", sym, err)
			continue
		}
		var actions []evidence.FeedAction
		parsed := json.Unmarshal(body, &actions) == nil
		// Provenance only (see storeFile): never authenticity proof.
		val := official.EvaluateArtifactState(body, meta, parsed, fmt.Sprintf("%d actions", len(actions)), parsed, "",
			false, false)
		if existing, err := db.GetSourceArtifactByChecksum(ctx, val.SHA256Checksum); err == nil && existing != nil {
			unchanged++
			continue
		}
		art := official.BuildSourceArtifact(sym, meta, body, val, "EXCHANGE_API_JSON")
		art.ParserVersion = "p1.9.24"
		if _, err := db.InsertSourceArtifact(ctx, art); err != nil {
			failed++
			log.Printf("%s: store: %v", sym, err)
			continue
		}
		stored++
		log.Printf("%s: %d actions, status %s", sym, len(actions), val.AssignedStatus)
	}
	fmt.Printf("ca-feed: stored=%d unchanged=%d failed=%d (all stored as %s; promotion requires a human reviewer)\n",
		stored, unchanged, failed, "AUTHENTICITY_PENDING")
}

// checkFilings looks up the company filings behind every price-changing action of the latest reconciliation run,
// stores each PDF as an append-only AUTHENTICITY_PENDING artifact and records what its signature and content
// establish. It never promotes anything.
func checkFilings(ctx context.Context, db *database.DB, pipe *official.Pipeline) {
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT f.symbol_id, s.symbol, f.action_class, f.ex_date, COALESCE(f.feed_subject, '')
		FROM ca_reconciliation_findings f JOIN symbols s ON s.id = f.symbol_id
		WHERE f.run_id = (SELECT max(id) FROM ca_reconciliation_runs)
		  AND f.action_class IN ('BONUS', 'SPLIT', 'DEMERGER') AND f.ex_date IS NOT NULL
		ORDER BY s.symbol, f.ex_date`)
	if err != nil {
		log.Fatal(err)
	}
	type target struct {
		symID         int
		symbol, class string
		ex            time.Time
		subject       string
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.symID, &t.symbol, &t.class, &t.ex, &t.subject); err == nil {
			targets = append(targets, t)
		}
	}
	rows.Close()

	checked, none := 0, 0
	verdicts := map[string]int{}
	for _, t := range targets {
		body, _, err := pipe.AcquireFromOfficialURL(ctx, evidence.AnnouncementsURL(t.symbol, t.ex))
		if errors.Is(err, official.ErrAccessRestricted) {
			log.Fatalf("exchange refused the client: %v (stopping; no circumvention)", err)
		}
		var anns []evidence.Announcement
		if err != nil || json.Unmarshal(body, &anns) != nil {
			log.Printf("%s %s: announcements unavailable: %v", t.symbol, t.ex.Format("2006-01-02"), err)
			none++
			continue
		}
		picks := evidence.SelectFilings(anns, t.class, 2)
		if len(picks) == 0 {
			log.Printf("%s %s %s: no matching filing found", t.symbol, t.class, t.ex.Format("2006-01-02"))
			none++
			continue
		}
		c := evidence.Classify(evidence.FeedAction{Symbol: t.symbol, ExDate: t.ex.Format("02-Jan-2006"), Subject: t.subject})
		c.Class = t.class
		for _, a := range picks {
			pdf, meta, err := pipe.AcquireFromOfficialURL(ctx, a.File)
			if err != nil {
				log.Printf("%s: %s: %v", t.symbol, a.File, err)
				continue
			}
			sigs, err := evidence.VerifyPDFSignatures(pdf)
			if err != nil {
				log.Fatalf("signature checks unavailable: %v", err)
			}
			checks := evidence.ContentChecks(evidence.ExtractText(pdf), c)
			verdict := evidence.DocumentVerdict(sigs, checks)
			// Provenance only: authenticity proof stays false (see storeFile).
			val := official.EvaluateArtifactState(pdf, meta, true, verdict, true, "", false, false)
			art := official.BuildSourceArtifact(t.symbol, meta, []byte("base64-pdf:"+base64.StdEncoding.EncodeToString(pdf)), val, "STATUTORY_FILING")
			art.SHA256Checksum = val.SHA256Checksum // checksum of the PDF bytes, not of the base64 wrapper
			art.ParserVersion = "p1.9.24-pdf"
			artID, err := db.InsertSourceArtifact(ctx, art)
			if err != nil {
				log.Printf("%s: store artifact: %v", t.symbol, err)
				continue
			}
			all, covers := len(sigs) > 0, false
			var signer, org, root, when string
			for i, sg := range sigs {
				all = all && sg.Intact
				if i == len(sigs)-1 {
					covers, signer, org, root, when = sg.CoversDocument, sg.SignerSubject, sg.SignerOrg, sg.ChainRoot, sg.SigningTime
				}
			}
			cj, _ := json.Marshal(checks)
			if _, err := db.ExecContext(ctx, `INSERT INTO primary_document_checks (artifact_id, symbol_id, action_class, ex_date,
				filing_date, filing_description, signature_count, all_signatures_intact, final_signature_covers_document,
				signer_subject, signer_organisation, chain_root_subject, signing_time, content_checks, verdict)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,''),NULLIF($11,''),NULLIF($12,''),NULLIF($13,''),$14,$15)`,
				artID, t.symID, t.class, t.ex, a.Date, strings.TrimSpace(a.Desc+": "+a.Text), len(sigs), all, covers,
				signer, org, root, when, cj, verdict); err != nil {
				log.Printf("%s: store check: %v", t.symbol, err)
				continue
			}
			checked++
			verdicts[verdict]++
			log.Printf("%s %s %s: %s (signer org %q)", t.symbol, t.class, t.ex.Format("2006-01-02"), verdict, org)
		}
	}
	out, _ := json.Marshal(verdicts)
	fmt.Printf("filings: actions=%d documents_checked=%d actions_without_filing=%d verdicts=%s (none promoted)\n",
		len(targets), checked, none, out)
}
