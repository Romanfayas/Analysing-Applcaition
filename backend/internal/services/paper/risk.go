package paper

import (
	"fmt"
	"os"
	"strconv"
)

// Limits are the account-level risk rules applied before a paper BUY is filled. They exist to catch the mistakes that
// end real accounts (one oversized position, chasing losses, hyperactive trading), and apply to the paper ledger the
// same way a broker's risk system would. A SELL is never blocked: a rule must not trap someone in a position.
type Limits struct {
	MaxPositionPct  float64 `json:"max_position_pct"`   // one stock's cost as a percent of book equity, after the order
	MaxDailyLossPct float64 `json:"max_daily_loss_pct"` // realised loss today, as a percent of starting capital, after which new buys pause
	MaxOrdersPerDay int     `json:"max_orders_per_day"`
}

// Default limits. Each can be overridden by an environment variable of the same meaning; 0 disables that rule.
var DefaultLimits = Limits{MaxPositionPct: 25, MaxDailyLossPct: 5, MaxOrdersPerDay: 50}

// LimitsFromEnv reads PAPER_RISK_MAX_POSITION_PCT, PAPER_RISK_MAX_DAILY_LOSS_PCT and PAPER_RISK_MAX_ORDERS_PER_DAY.
func LimitsFromEnv() Limits {
	l := DefaultLimits
	if v, err := strconv.ParseFloat(os.Getenv("PAPER_RISK_MAX_POSITION_PCT"), 64); err == nil && v >= 0 {
		l.MaxPositionPct = v
	}
	if v, err := strconv.ParseFloat(os.Getenv("PAPER_RISK_MAX_DAILY_LOSS_PCT"), 64); err == nil && v >= 0 {
		l.MaxDailyLossPct = v
	}
	if v, err := strconv.Atoi(os.Getenv("PAPER_RISK_MAX_ORDERS_PER_DAY")); err == nil && v >= 0 {
		l.MaxOrdersPerDay = v
	}
	return l
}

// State is what the rules need to know at the moment of an order.
type State struct {
	IsBuy          bool
	OrderCost      float64 // cash the order would use, charges included
	SymbolCost     float64 // cost basis already held in this stock
	BookEquity     float64 // cash plus the cost basis of everything held
	InitialCapital float64
	RealizedToday  float64 // realised P&L (net of charges) on the order's trading day; negative is a loss
	OrdersToday    int
}

// Breach names the rule that stopped an order and says why in plain words.
type Breach struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Check returns the first rule an order breaks, or nil. Sells always pass.
func Check(l Limits, s State) *Breach {
	if !s.IsBuy {
		return nil
	}
	if l.MaxOrdersPerDay > 0 && s.OrdersToday >= l.MaxOrdersPerDay {
		return &Breach{"MAX_ORDERS_PER_DAY", fmt.Sprintf("risk rule: %d orders already placed today (limit %d). Frequent trading is how charges eat returns; try again tomorrow.", s.OrdersToday, l.MaxOrdersPerDay)}
	}
	if l.MaxDailyLossPct > 0 && s.InitialCapital > 0 && s.RealizedToday < 0 && -s.RealizedToday >= s.InitialCapital*l.MaxDailyLossPct/100 {
		return &Breach{"MAX_DAILY_LOSS", fmt.Sprintf("risk rule: realised losses today (%.0f) have reached %.1f%% of starting capital, so new buys are paused until tomorrow. Selling is still allowed.", -s.RealizedToday, l.MaxDailyLossPct)}
	}
	if l.MaxPositionPct > 0 && s.BookEquity > 0 {
		after := (s.SymbolCost + s.OrderCost) / s.BookEquity * 100
		if after > l.MaxPositionPct+1e-9 {
			return &Breach{"MAX_POSITION", fmt.Sprintf("risk rule: this would make one stock %.1f%% of your account (limit %.0f%%). Concentration in a single company is the commonest way to lose heavily.", after, l.MaxPositionPct)}
		}
	}
	return nil
}
