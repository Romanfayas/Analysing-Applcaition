package nse

import (
	"time"

	"github.com/halal-equity/backend/internal/models"
)

// RawNSECorporateAction represents a single corporate action item returned by the official NSE API
// endpoint: https://www.nseindia.com/api/corporates-corporateActions?index=equities&symbol=<SYMBOL>
type RawNSECorporateAction struct {
	Symbol          string  `json:"symbol"`
	Comp            string  `json:"comp"`
	Series          string  `json:"series"`
	FaceVal         string  `json:"faceVal"`
	ISIN            string  `json:"isin"`
	Subject         string  `json:"subject"`
	ExDate          string  `json:"exDate"`
	RecDate         string  `json:"recDate"`
	BcStartDate     string  `json:"bcStartDate"`
	BcEndDate       string  `json:"bcEndDate"`
	NdStartDate     string  `json:"ndStartDate"`
	NdEndDate       string  `json:"ndEndDate"`
	CaBroadcastDate *string `json:"caBroadcastDate"`
	Ind             string  `json:"ind"`
}

// HarvestResult contains the raw artifact and the normalized events for a symbol.
type HarvestResult struct {
	Symbol           string
	Artifact         *models.CorporateActionSourceArtifact
	Events           []models.CorporateActionEvent
	DiscoveryStatus  string // "DISCOVERED_EVENTS", "NO_EVENT_CONFIRMED", "SOURCE_UNAVAILABLE", "ERROR"
	ErrorMessage     string
	HTTPStatusCode   int
	RetrievalLatency time.Duration
}
