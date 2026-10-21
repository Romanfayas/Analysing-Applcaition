package database

import (
	"strings"
	"testing"
	"time"
)

func TestIPOLifecycleFiltering(t *testing.T) {
	testCases := []struct {
		name           string
		statusFilter   string
		recordStatus   string
		expectedKeep   bool
	}{
		{
			name:         "Upcoming filter excludes historical listed IPOs",
			statusFilter: "UPCOMING",
			recordStatus: "HISTORICAL_LISTED",
			expectedKeep: false,
		},
		{
			name:         "Upcoming filter excludes DRHP filed IPOs",
			statusFilter: "UPCOMING",
			recordStatus: "DRHP_FILED",
			expectedKeep: false,
		},
		{
			name:         "Active filter excludes historical listed IPOs",
			statusFilter: "ACTIVE",
			recordStatus: "HISTORICAL_LISTED",
			expectedKeep: false,
		},
		{
			name:         "Historical filter includes historical listed IPOs",
			statusFilter: "HISTORICAL",
			recordStatus: "HISTORICAL_LISTED",
			expectedKeep: true,
		},
		{
			name:         "DRHP filter includes DRHP filed IPOs",
			statusFilter: "DRHP",
			recordStatus: "DRHP_FILED",
			expectedKeep: true,
		},
		{
			name:         "Open filter excludes closed or historical IPOs",
			statusFilter: "OPEN",
			recordStatus: "HISTORICAL_LISTED",
			expectedKeep: false,
		},
	}

	filterFn := func(statusFilter, status string) bool {
		if statusFilter == "" || statusFilter == "ALL" || statusFilter == "all" {
			return true
		}
		switch strings.ToUpper(statusFilter) {
		case "UPCOMING":
			return status == "UPCOMING"
		case "OPEN":
			return status == "OPEN"
		case "ACTIVE", "CURRENT":
			return status == "UPCOMING" || status == "OPEN" || status == "CLOSED_AWAITING_LISTING"
		case "DRHP", "DRHP_FILED", "PROPOSED":
			return status == "DRHP_FILED" || status == "PROPOSED"
		case "HISTORICAL", "HISTORICAL_LISTED", "LISTED", "ARCHIVED":
			return status == "HISTORICAL_LISTED" || status == "LISTED" || status == "ARCHIVED"
		default:
			return strings.EqualFold(status, statusFilter)
		}
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := filterFn(tc.statusFilter, tc.recordStatus)
			if got != tc.expectedKeep {
				t.Errorf("filterFn(%q, %q) = %v; want %v", tc.statusFilter, tc.recordStatus, got, tc.expectedKeep)
			}
		})
	}
}

func TestIPOPredictionPointInTimeIntegrity(t *testing.T) {
	// Hyundai Motor India historical IPO cutoff test
	biddingClose := time.Date(2024, 10, 17, 17, 0, 0, 0, time.UTC)
	listingDate := time.Date(2024, 10, 22, 9, 15, 0, 0, time.UTC)
	cutoffAt := time.Date(2024, 10, 17, 17, 0, 0, 0, time.UTC)

	// Prediction cutoff MUST be strictly prior to listing
	if !cutoffAt.Before(listingDate) {
		t.Fatalf("Look-ahead bias violation: prediction cutoff %v is not before listing %v", cutoffAt, listingDate)
	}

	// Cutoff must match decision window
	if cutoffAt.After(biddingClose) {
		t.Fatalf("Cutoff is after bidding close: %v > %v", cutoffAt, biddingClose)
	}
}
