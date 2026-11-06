package ingestion

import (
	"context"
	"time"

	"github.com/halal-equity/backend/internal/services/market"
)

const (
	// providerIntradayWindow is how far back the free provider still serves 1-minute bars.
	providerIntradayWindow = 7 * 24 * time.Hour
	// minCompleteSessionBars: a normal session yields ~362 one-minute bars from this provider (the minutes
	// 15:16-15:28 are structurally absent). Below this a session is treated as incomplete and re-requested.
	minCompleteSessionBars = 300
)

// barCounter returns how many VALID 1-minute bars a symbol has in [from, to).
type barCounter func(ctx context.Context, symbolID int, from, to time.Time) (int, error)

// recoveryStart returns the start of the earliest incomplete past trading session still inside the provider
// window, or nil when every such session is complete. The incremental fetch only moves forward from the newest
// bar, so without this an outage hole is never re-requested and is lost once it leaves the window.
func recoveryStart(ctx context.Context, symbolID int, now time.Time, count barCounter) (*time.Time, error) {
	nowIST := now.In(market.Location)
	today := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 0, 0, 0, 0, market.Location)
	for d := today.Add(-providerIntradayWindow).AddDate(0, 0, 1); d.Before(today); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if holiday, _ := market.IsTradingHoliday(d); holiday {
			continue
		}
		open := d.Add(9*time.Hour + 15*time.Minute)
		n, err := count(ctx, symbolID, open, d.Add(15*time.Hour+30*time.Minute))
		if err != nil {
			return nil, err
		}
		if n < minCompleteSessionBars {
			return &open, nil
		}
	}
	return nil, nil
}
