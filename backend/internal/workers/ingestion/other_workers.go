package ingestion

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/marketdata"
)

// IngestIPO fetches and stores IPO data.
func (p *Pipeline) IngestIPO(ctx context.Context, provider marketdata.IPODataProvider) error {
	log.Printf("[INGEST-IPO] Fetching upcoming IPOs via %s", provider.Name())

	dataPoint, err := provider.GetUpcomingIPOs(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch IPOs: %w", err)
	}

	if dataPoint == nil || len(dataPoint.Data) == 0 {
		log.Printf("[INGEST-IPO] No upcoming IPOs found")
		return nil
	}

	// This is a stub for the DB logic since I am currently implementing the pipeline architecture.
	// We would loop over dataPoint.Data and upsert to p.db.UpsertIPO(ctx, ipo)
	log.Printf("[INGEST-IPO] Successfully prepared %d IPOs for upsert", len(dataPoint.Data))
	
	// Example DB upsert logic:
	/*
	for _, ipo := range dataPoint.Data {
		err := p.db.UpsertIPO(ctx, ipo)
		if err != nil {
			log.Printf("[INGEST-IPO] WARN: failed to store IPO %s: %v", ipo.CompanyName, err)
		}
	}
	*/
	
	return nil
}

// IngestCorporateActions fetches and stores corporate actions for a symbol.
func (p *Pipeline) IngestCorporateActions(ctx context.Context, provider marketdata.CorporateActionsProvider, symbol string) error {
	log.Printf("[INGEST-CORP-ACTION] Fetching corporate actions for %s via %s", symbol, provider.Name())

	// Fetch actions for the past year incrementally
	from := time.Now().AddDate(-1, 0, 0)
	to := time.Now()

	dataPoint, err := provider.GetCorporateActions(ctx, symbol, "NSE", from, to)
	if err != nil {
		return fmt.Errorf("failed to fetch corporate actions for %s: %w", symbol, err)
	}

	if dataPoint == nil || len(dataPoint.Data) == 0 {
		log.Printf("[INGEST-CORP-ACTION] No corporate actions found for %s", symbol)
		return nil
	}

	// Example DB upsert logic:
	/*
	err = p.db.UpsertCorporateActions(ctx, symbolID, dataPoint.Data)
	*/
	
	log.Printf("[INGEST-CORP-ACTION] Successfully prepared %d corporate actions for %s", len(dataPoint.Data), symbol)
	return nil
}
