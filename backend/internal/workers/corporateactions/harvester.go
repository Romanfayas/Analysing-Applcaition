package corporateactions

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/corporateactions/nse"
)

// HarvesterOptions configures the corporate action harvesting worker.
type HarvesterOptions struct {
	Symbols   []string
	DryRun    bool
	Throttle  time.Duration
	SourceDir string
	FilePath  string
}

// HarvesterSummary reports aggregate metrics of a harvesting execution run.
type HarvesterSummary struct {
	StartTime          time.Time                     `json:"start_time"`
	EndTime            time.Time                     `json:"end_time"`
	DurationSeconds    float64                       `json:"duration_seconds"`
	SymbolsTargeted    int                           `json:"symbols_targeted"`
	SymbolsProcessed   int                           `json:"symbols_processed"`
	ArtifactsRetrieved int                           `json:"artifacts_retrieved"`
	EventsParsed       int                           `json:"events_parsed"`
	EventsVerified     int                           `json:"events_verified"`
	EventsPending      int                           `json:"events_pending_review"`
	RateLimitHits      int                           `json:"rate_limit_hits"`
	ErrorsCount        int                           `json:"errors_count"`
	SymbolResults      map[string]*nse.HarvestResult `json:"symbol_results"`
}

// Harvester orchestrates the discovery, retrieval, validation, and storage of corporate action evidence.
type Harvester struct {
	db      *database.DB
	adapter *nse.Adapter
}

// NewHarvester creates a new corporate actions harvester.
func NewHarvester(db *database.DB, adapter *nse.Adapter) *Harvester {
	if adapter == nil {
		adapter = nse.NewAdapter(nil)
	}
	return &Harvester{
		db:      db,
		adapter: adapter,
	}
}

// Run executes the harvesting process.
// If SourceDir or FilePath is provided, ingests from the controlled statutory archive.
// Otherwise, attempts live discovery via official exchange endpoints.
func (h *Harvester) Run(ctx context.Context, opts HarvesterOptions) (*HarvesterSummary, error) {
	startTime := time.Now()
	summary := &HarvesterSummary{
		StartTime:     startTime,
		SymbolResults: make(map[string]*nse.HarvestResult),
	}

	if opts.FilePath != "" {
		res, err := h.IngestArtifactFile(ctx, opts.FilePath, opts.DryRun)
		summary.EndTime = time.Now()
		summary.DurationSeconds = summary.EndTime.Sub(startTime).Seconds()
		if res != nil {
			summary.SymbolsTargeted = 1
			summary.SymbolsProcessed = 1
			summary.ArtifactsRetrieved = 1
			summary.EventsParsed = len(res.Events)
			summary.SymbolResults[res.Symbol] = res
			for _, ev := range res.Events {
				if ev.VerificationStatus == "VERIFIED_EXCHANGE_PRIMARY" {
					summary.EventsVerified++
				} else {
					summary.EventsPending++
				}
			}
		}
		if err != nil {
			summary.ErrorsCount++
		}
		return summary, err
	}

	if opts.SourceDir != "" {
		return h.IngestArtifactDirectory(ctx, opts.SourceDir, opts.DryRun)
	}

	// Live discovery
	symbolsToProcess := opts.Symbols
	if len(symbolsToProcess) == 0 {
		query := `
			SELECT DISTINCT s.symbol
			FROM ohlcv_daily o
			JOIN symbols s ON o.symbol_id = s.id
			WHERE s.is_active = true
			ORDER BY s.symbol ASC;
		`
		rows, err := h.db.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to query active symbols for harvesting: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var sym string
			if err := rows.Scan(&sym); err == nil {
				symbolsToProcess = append(symbolsToProcess, sym)
			}
		}
	}

	summary.SymbolsTargeted = len(symbolsToProcess)
	log.Printf("[HARVESTER] Starting corporate-action harvesting for %d symbols (dryRun=%v)", len(symbolsToProcess), opts.DryRun)

	for idx, symbol := range symbolsToProcess {
		select {
		case <-ctx.Done():
			log.Printf("[HARVESTER] Context cancelled. Halting harvesting.")
			summary.EndTime = time.Now()
			summary.DurationSeconds = summary.EndTime.Sub(startTime).Seconds()
			return summary, ctx.Err()
		default:
		}

		var symbolID int
		err := h.db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = $1 LIMIT 1;", symbol).Scan(&symbolID)
		if err != nil {
			log.Printf("[HARVESTER] [%d/%d] Symbol %s not found in database: %v", idx+1, len(symbolsToProcess), symbol, err)
			summary.ErrorsCount++
			continue
		}

		log.Printf("[HARVESTER] [%d/%d] Harvesting primary evidence for %s...", idx+1, len(symbolsToProcess), symbol)
		res, err := h.adapter.DiscoverAndHarvest(ctx, symbol, symbolID)
		summary.SymbolResults[symbol] = res

		if err != nil {
			log.Printf("[HARVESTER] Error harvesting %s: %v (status=%s, code=%d)", symbol, err, res.DiscoveryStatus, res.HTTPStatusCode)
			summary.ErrorsCount++
			if res.HTTPStatusCode == 429 {
				summary.RateLimitHits++
			}
			continue
		}

		summary.SymbolsProcessed++
		if res.Artifact != nil {
			summary.ArtifactsRetrieved++

			if !opts.DryRun {
				artID, aErr := h.db.InsertSourceArtifact(ctx, res.Artifact)
				if aErr != nil {
					log.Printf("[HARVESTER] Failed to persist source artifact for %s: %v", symbol, aErr)
					summary.ErrorsCount++
					continue
				}

				for i := range res.Events {
					ev := &res.Events[i]
					ev.SourceArtifactID = &artID
					_, eErr := h.db.InsertCorporateActionEvent(ctx, ev)
					if eErr != nil {
						log.Printf("[HARVESTER] Failed to persist event %s: %v", ev.EventID, eErr)
						summary.ErrorsCount++
					} else {
						summary.EventsParsed++
						if ev.VerificationStatus == "VERIFIED_EXCHANGE_PRIMARY" {
							summary.EventsVerified++
						} else {
							summary.EventsPending++
						}
					}
				}
			} else {
				for _, ev := range res.Events {
					summary.EventsParsed++
					if ev.VerificationStatus == "VERIFIED_EXCHANGE_PRIMARY" {
						summary.EventsVerified++
					} else {
						summary.EventsPending++
					}
				}
			}
		}

		log.Printf("[HARVESTER] Completed %s: %d events found (%s, latency=%v)",
			symbol, len(res.Events), res.DiscoveryStatus, res.RetrievalLatency)
	}

	summary.EndTime = time.Now()
	summary.DurationSeconds = summary.EndTime.Sub(startTime).Seconds()
	log.Printf("[HARVESTER] Live harvesting complete: %d symbols, %d artifacts, %d events (%d verified, %d pending) in %.2fs",
		summary.SymbolsProcessed, summary.ArtifactsRetrieved, summary.EventsParsed, summary.EventsVerified, summary.EventsPending, summary.DurationSeconds)

	return summary, nil
}

// IngestArtifactFile ingests a single controlled statutory artifact file into the provenance ledger.
func (h *Harvester) IngestArtifactFile(ctx context.Context, filePath string, dryRun bool) (*nse.HarvestResult, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open artifact file %s: %w", filePath, err)
	}
	defer file.Close()

	rawBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact file %s: %w", filePath, err)
	}

	rawActions, err := nse.DecodeRawJSON(rawBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to decode JSON in %s: %w", filePath, err)
	}

	if len(rawActions) == 0 {
		return nil, fmt.Errorf("no corporate action events found in %s", filePath)
	}

	symbol := rawActions[0].Symbol
	var symbolID int
	err = h.db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = $1 LIMIT 1;", symbol).Scan(&symbolID)
	if err != nil {
		return nil, fmt.Errorf("symbol %s from artifact %s not found in symbols table: %w", symbol, filePath, err)
	}

	artifact := nse.BuildArtifact(symbol, "CONTROLLED_ARCHIVE:"+filepath.Base(filePath), rawBytes, time.Now())
	artifact.ValidationStatus = "PARSED_VALIDATED"

	var artID int
	if !dryRun {
		artID, err = h.db.InsertSourceArtifact(ctx, artifact)
		if err != nil {
			return nil, fmt.Errorf("failed to persist source artifact: %w", err)
		}
	}

	var parsedEvents []models.CorporateActionEvent
	for _, raw := range rawActions {
		var pArtID *int
		if artID > 0 {
			pArtID = &artID
		}
		events, pErr := nse.ParseRawCorporateAction(raw, symbolID, pArtID)
		if pErr != nil {
			log.Printf("[HARVESTER] Warning: skipping malformed action for %s: %v", symbol, pErr)
			continue
		}
		for i := range events {
			ev := events[i]
			if !dryRun {
				_, eErr := h.db.InsertCorporateActionEvent(ctx, &ev)
				if eErr != nil {
					log.Printf("[HARVESTER] Warning: event insert failed for %s: %v", ev.EventID, eErr)
				}
			}
			parsedEvents = append(parsedEvents, ev)
		}
	}

	res := &nse.HarvestResult{
		Symbol:          symbol,
		Artifact:        artifact,
		Events:          parsedEvents,
		DiscoveryStatus: "DISCOVERED_EVENTS",
		HTTPStatusCode:  200,
	}
	return res, nil
}

// IngestArtifactDirectory scans a controlled directory and ingests all .json artifact files.
func (h *Harvester) IngestArtifactDirectory(ctx context.Context, dirPath string, dryRun bool) (*HarvesterSummary, error) {
	startTime := time.Now()
	summary := &HarvesterSummary{
		StartTime:     startTime,
		SymbolResults: make(map[string]*nse.HarvestResult),
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact directory %s: %w", dirPath, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		fullPath := filepath.Join(dirPath, entry.Name())
		log.Printf("[HARVESTER] Processing controlled artifact: %s", entry.Name())
		res, err := h.IngestArtifactFile(ctx, fullPath, dryRun)
		if err != nil {
			log.Printf("[HARVESTER] Error ingesting artifact %s: %v", entry.Name(), err)
			summary.ErrorsCount++
			continue
		}

		summary.SymbolsProcessed++
		summary.ArtifactsRetrieved++
		summary.EventsParsed += len(res.Events)
		summary.SymbolResults[res.Symbol] = res

		for _, ev := range res.Events {
			if ev.VerificationStatus == "VERIFIED_EXCHANGE_PRIMARY" {
				summary.EventsVerified++
			} else {
				summary.EventsPending++
			}
		}
	}

	summary.EndTime = time.Now()
	summary.DurationSeconds = summary.EndTime.Sub(startTime).Seconds()
	log.Printf("[HARVESTER] Directory ingestion complete: %d artifacts, %d events (%d verified, %d pending) in %.2fs",
		summary.ArtifactsRetrieved, summary.EventsParsed, summary.EventsVerified, summary.EventsPending, summary.DurationSeconds)

	return summary, nil
}
