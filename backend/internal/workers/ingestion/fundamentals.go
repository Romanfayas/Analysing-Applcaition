package ingestion

import (
	"context"
	"fmt"
	"log"

	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/marketdata"
)

// IngestFundamentals fetches and stores fundamental data for a symbol.
// It enforces point-in-time constraints by using the PeriodEnd as the primary historical anchor,
// and correctly sets the retrieved_at timestamp for provenance.
func (p *Pipeline) IngestFundamentals(ctx context.Context, provider marketdata.FundamentalDataProvider, symbol string) error {
	log.Printf("[INGEST-FUNDAMENTALS] Fetching fundamentals for %s via %s", symbol, provider.Name())

	// Ensure symbol exists in DB and get ID
	symbolID, err := p.db.UpsertSymbol(ctx, symbol, symbol, "NSE")
	if err != nil {
		return fmt.Errorf("failed to upsert symbol %s: %w", symbol, err)
	}

	// Fetch fundamentals (annual for now)
	dataPoint, err := provider.GetFinancialStatements(ctx, symbol, "NSE", "ANNUAL")
	if err != nil {
		return fmt.Errorf("failed to fetch fundamentals for %s: %w", symbol, err)
	}

	if dataPoint == nil || len(dataPoint.Data) == 0 {
		log.Printf("[INGEST-FUNDAMENTALS] No fundamental data found for %s", symbol)
		return nil
	}

	// Prepare records for DB
	for i := range dataPoint.Data {
		dataPoint.Data[i].SymbolID = symbolID
		dataPoint.Data[i].Source = dataPoint.Source
		dataPoint.Data[i].RetrievedAt = dataPoint.RetrievedAt
		dataPoint.Data[i].QualityStatus = models.DataQualityValid
	}
	
	err = p.db.UpsertFinancialStatements(ctx, dataPoint.Data)
	if err != nil {
		return fmt.Errorf("failed to store fundamentals for %s: %w", symbol, err)
	}

	log.Printf("[INGEST-FUNDAMENTALS] Successfully stored %d financial periods for %s", len(dataPoint.Data), symbol)
	return nil
}
