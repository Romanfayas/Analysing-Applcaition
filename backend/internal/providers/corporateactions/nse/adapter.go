package nse

import (
	"context"
	"fmt"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

// Adapter implements the primary corporate-action discovery adapter for NSE India.
type Adapter struct {
	client *Client
}

// NewAdapter creates an NSE corporate action discovery adapter.
func NewAdapter(client *Client) *Adapter {
	if client == nil {
		client = NewClient(600 * time.Millisecond)
	}
	return &Adapter{client: client}
}

// DiscoverAndHarvest fetches primary evidence from the official NSE API and normalizes it.
func (a *Adapter) DiscoverAndHarvest(ctx context.Context, symbol string, symbolID int) (*HarvestResult, error) {
	start := time.Now()
	sourceURL, rawBytes, statusCode, err := a.client.FetchCorporateActionsRaw(ctx, symbol)
	latency := time.Since(start)

	res := &HarvestResult{
		Symbol:           symbol,
		HTTPStatusCode:   statusCode,
		RetrievalLatency: latency,
	}

	if err != nil {
		res.DiscoveryStatus = "SOURCE_UNAVAILABLE"
		res.ErrorMessage = err.Error()
		if statusCode == 403 || statusCode == 401 {
			res.DiscoveryStatus = "ACCESS_DENIED"
		}
		return res, err
	}

	// Build immutable source artifact with SHA-256
	artifact := BuildArtifact(symbol, sourceURL, rawBytes, start)
	res.Artifact = artifact

	// Decode raw JSON
	rawActions, err := DecodeRawJSON(rawBytes)
	if err != nil {
		res.DiscoveryStatus = "PARSE_ERROR"
		res.ErrorMessage = fmt.Sprintf("invalid JSON payload: %v", err)
		artifact.ValidationStatus = "PARSE_ERROR"
		return res, err
	}

	if len(rawActions) == 0 {
		res.DiscoveryStatus = "NO_EVENT_CONFIRMED"
		artifact.ValidationStatus = "PARSED_VALIDATED"
		return res, nil
	}

	// Normalize each raw action into events
	var allEvents []models.CorporateActionEvent
	parseErrors := 0
	for _, raw := range rawActions {
		events, pErr := ParseRawCorporateAction(raw, symbolID, nil) // sourceArtifactID will be linked upon DB insert
		if pErr != nil {
			parseErrors++
			continue
		}
		allEvents = append(allEvents, events...)
	}

	res.Events = allEvents
	if len(allEvents) > 0 {
		res.DiscoveryStatus = "DISCOVERED_EVENTS"
		artifact.ValidationStatus = "PARSED_VALIDATED"
	} else if parseErrors > 0 {
		res.DiscoveryStatus = "PARSE_ERROR"
		artifact.ValidationStatus = "PARSE_ERROR"
	} else {
		res.DiscoveryStatus = "NO_EVENT_CONFIRMED"
		artifact.ValidationStatus = "PARSED_VALIDATED"
	}

	return res, nil
}
