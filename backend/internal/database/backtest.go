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

// ListBacktestRuns returns the most recent persisted backtest run summaries.
func (db *DB) ListBacktestRuns(ctx context.Context, limit int) ([]BacktestRun, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, COALESCE(symbol_id, 0), COALESCE(configuration_id, 0), start_date, end_date,
		       initial_capital, final_capital, cagr, max_drawdown, sharpe_ratio, sortino_ratio,
		       win_rate, COALESCE(total_trades, 0), COALESCE(survivorship_bias_warning, true),
		       COALESCE(created_at, now())
		FROM backtest_runs ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]BacktestRun, 0, limit)
	for rows.Next() {
		var r BacktestRun
		if err := rows.Scan(&r.ID, &r.SymbolID, &r.ConfigurationID, &r.StartDate, &r.EndDate,
			&r.InitialCapital, &r.FinalCapital, &r.CAGR, &r.MaxDrawdown, &r.SharpeRatio, &r.SortinoRatio,
			&r.WinRate, &r.TotalTrades, &r.SurvivorshipBiasWarning, &r.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetBacktestRun returns one persisted run, or sql.ErrNoRows.
func (db *DB) GetBacktestRun(ctx context.Context, id int) (*BacktestRun, error) {
	var r BacktestRun
	err := db.QueryRowContext(ctx, `
		SELECT id, COALESCE(symbol_id, 0), COALESCE(configuration_id, 0), start_date, end_date,
		       initial_capital, final_capital, cagr, max_drawdown, sharpe_ratio, sortino_ratio,
		       win_rate, COALESCE(total_trades, 0), COALESCE(survivorship_bias_warning, true),
		       COALESCE(created_at, now())
		FROM backtest_runs WHERE id = $1`, id).Scan(&r.ID, &r.SymbolID, &r.ConfigurationID, &r.StartDate,
		&r.EndDate, &r.InitialCapital, &r.FinalCapital, &r.CAGR, &r.MaxDrawdown, &r.SharpeRatio,
		&r.SortinoRatio, &r.WinRate, &r.TotalTrades, &r.SurvivorshipBiasWarning, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// BacktestStrategy is a named, versioned strategy definition.
type BacktestStrategy struct {
	ID          int             `json:"id"`
	Name        string          `json:"name"`
	Version     int             `json:"version"`
	Description string          `json:"description"`
	Config      json.RawMessage `json:"config"`
	CreatedAt   time.Time       `json:"created_at"`
}

// ListBacktestStrategies returns all stored strategy definitions, newest first.
func (db *DB) ListBacktestStrategies(ctx context.Context) ([]BacktestStrategy, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, version, COALESCE(description, ''), config, created_at
		FROM backtest_strategies ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BacktestStrategy{}
	for rows.Next() {
		var s BacktestStrategy
		if err := rows.Scan(&s.ID, &s.Name, &s.Version, &s.Description, &s.Config, &s.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// CreateBacktestStrategy stores a new strategy definition. Versions auto-increment per name and
// existing versions are never modified, so earlier results stay attributable.
func (db *DB) CreateBacktestStrategy(ctx context.Context, name, description string, config json.RawMessage, createdBy *string) (*BacktestStrategy, error) {
	var s BacktestStrategy
	err := db.QueryRowContext(ctx, `
		INSERT INTO backtest_strategies (name, version, description, config, created_by)
		VALUES ($1, COALESCE((SELECT max(version) FROM backtest_strategies WHERE name = $1), 0) + 1, $2, $3, (SELECT id FROM users WHERE id = $4::uuid))
		RETURNING id, name, version, COALESCE(description, ''), config, created_at`,
		name, description, config, createdBy).Scan(&s.ID, &s.Name, &s.Version, &s.Description, &s.Config, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}
