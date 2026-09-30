package database

import (
	"context"
	"encoding/json"
	"time"
)

// BacktestConfig represents the database model for a backtest configuration
type BacktestConfig struct {
	ID                 int             `json:"id"`
	StrategyVersion    string          `json:"strategy_version"`
	Parameters         json.RawMessage `json:"parameters"`
	CostModelVersion   string          `json:"cost_model_version"`
	SlippageModel      json.RawMessage `json:"slippage_model"`
	CreatedAt          time.Time       `json:"created_at"`
}

// BacktestRun represents the database model for a backtest run summary
type BacktestRun struct {
	ID                       int       `json:"id"`
	SymbolID                 int       `json:"symbol_id"`
	ConfigurationID          int       `json:"configuration_id"`
	StartDate                time.Time `json:"start_date"`
	EndDate                  time.Time `json:"end_date"`
	InitialCapital           float64   `json:"initial_capital"`
	FinalCapital             float64   `json:"final_capital"`
	CAGR                     *float64  `json:"cagr"`
	MaxDrawdown              *float64  `json:"max_drawdown"`
	SharpeRatio              *float64  `json:"sharpe_ratio"`
	SortinoRatio             *float64  `json:"sortino_ratio"`
	WinRate                  *float64  `json:"win_rate"`
	TotalTrades              int       `json:"total_trades"`
	SurvivorshipBiasWarning  bool      `json:"survivorship_bias_warning"`
	CreatedAt                time.Time `json:"created_at"`
}

// EnsureBacktestConfiguration saves or returns an existing configuration ID
func (db *DB) EnsureBacktestConfiguration(ctx context.Context, cfg *BacktestConfig) (int, error) {
	query := `
		INSERT INTO backtest_configurations (strategy_version, parameters, cost_model_version, slippage_model)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (strategy_version, parameters, cost_model_version, slippage_model) DO UPDATE SET strategy_version = EXCLUDED.strategy_version
		RETURNING id
	`
	var id int
	err := db.QueryRowContext(ctx, query, cfg.StrategyVersion, cfg.Parameters, cfg.CostModelVersion, cfg.SlippageModel).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// SaveBacktestRun saves the summary of a backtest run
func (db *DB) SaveBacktestRun(ctx context.Context, run *BacktestRun) (int, error) {
	query := `
		INSERT INTO backtest_runs (
			symbol_id, configuration_id, start_date, end_date, initial_capital, final_capital,
			cagr, max_drawdown, sharpe_ratio, sortino_ratio, win_rate, total_trades, survivorship_bias_warning
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`
	var id int
	err := db.QueryRowContext(ctx, query,
		run.SymbolID, run.ConfigurationID, run.StartDate, run.EndDate,
		run.InitialCapital, run.FinalCapital, run.CAGR, run.MaxDrawdown,
		run.SharpeRatio, run.SortinoRatio, run.WinRate, run.TotalTrades, run.SurvivorshipBiasWarning,
	).Scan(&id)
	
	if err != nil {
		return 0, err
	}
	return id, nil
}
