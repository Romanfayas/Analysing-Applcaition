package paper

import "math"

// ImpactCoefficient scales the square-root impact law. 1.0 is deliberately on the cautious side of published
// estimates; it is a stated assumption of the simulation, not an observed fill.
const ImpactCoefficient = 1.0

// BaseSlippageBps is the fixed per-side slippage already used by the platform's cost model (5 bps).
const BaseSlippageBps = 5.0

// Liquidity describes how large an order is relative to what the stock normally trades.
type Liquidity struct {
	ADVShares        float64 `json:"adv_shares"`           // average daily volume over the look-back sessions
	DailyVolatilityP float64 `json:"daily_volatility_pct"` // standard deviation of daily returns, in percent
	Sessions         int     `json:"sessions"`
	Participation    float64 `json:"participation_pct"` // order shares as a percent of ADV
	ImpactBps        float64 `json:"impact_bps"`        // modelled extra slippage on each side
}

// ImpactBps is the square-root market-impact law: daily volatility (in bps) x coefficient x sqrt(order / ADV).
// It rises with order size and with volatility, and is zero for a negligible order.
func ImpactBps(dailyVolPct, orderShares, advShares float64) float64 {
	if dailyVolPct <= 0 || orderShares <= 0 || advShares <= 0 {
		return 0
	}
	return ImpactCoefficient * dailyVolPct * 100 * math.Sqrt(orderShares/advShares)
}

// NewLiquidity builds the summary, or returns nil when there is no volume history to judge against.
func NewLiquidity(advShares, dailyVolPct float64, sessions int, orderShares float64) *Liquidity {
	if advShares <= 0 || sessions <= 0 {
		return nil
	}
	return &Liquidity{
		ADVShares: advShares, DailyVolatilityP: dailyVolPct, Sessions: sessions,
		Participation: orderShares / advShares * 100, ImpactBps: ImpactBps(dailyVolPct, orderShares, advShares),
	}
}
