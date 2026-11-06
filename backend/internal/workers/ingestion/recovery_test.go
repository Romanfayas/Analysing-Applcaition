package ingestion

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/services/market"
)

func TestRecoveryStartFindsTheEarliestIncompleteSessionInsideTheWindow(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, market.Location) // Friday
	bars := map[string]int{"2026-10-26": 362, "2026-10-27": 0, "2026-10-28": 362, "2026-10-29": 120}
	count := func(_ context.Context, _ int, from, _ time.Time) (int, error) {
		return bars[from.In(market.Location).Format("2006-01-02")], nil
	}
	got, err := recoveryStart(context.Background(), 1, now, count)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "2026-10-27 09:15", got.In(market.Location).Format("2006-01-02 15:04"), "an outage day is re-requested")
}

func TestRecoveryStartIgnoresTheStructuralClosingGapAndWeekends(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, market.Location)
	count := func(_ context.Context, _ int, from, _ time.Time) (int, error) {
		if wd := from.In(market.Location).Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("weekend %s should not be checked", from)
		}
		return 362, nil // complete apart from the provider's missing 15:16-15:28
	}
	got, err := recoveryStart(context.Background(), 1, now, count)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestRecoveryStartDoesNotReachBeyondTheProviderWindow(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, market.Location)
	var earliest time.Time
	count := func(_ context.Context, _ int, from, _ time.Time) (int, error) {
		if earliest.IsZero() || from.Before(earliest) {
			earliest = from
		}
		return 362, nil
	}
	_, _ = recoveryStart(context.Background(), 1, now, count)
	assert.True(t, now.Sub(earliest) < providerIntradayWindow, "a hole the provider can no longer serve is not requested")
}
