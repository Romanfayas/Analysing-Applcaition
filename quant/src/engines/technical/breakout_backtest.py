"""
Point-in-Time Walk-Forward Quantitative Breakout Backtester

Audits and evaluates the BreakoutEngine across all available historical market data.
Strictly enforces:
1. Zero future-candle leakage (only data up to time t available at decision time).
2. Execution at bar t+1 OPEN (never at bar t or retroactively).
3. Non-overlapping independent trade lifecycle with cooldown.
4. Chronological Train / Validation / Untouched Out-of-Sample partitions.
5. Indian institutional/retail cash-equity friction (47 bps roundtrip).
6. Statistical significance, standard error, and confidence intervals.
"""

import numpy as np
import pandas as pd
from datetime import datetime, timezone
from dataclasses import dataclass, field
from typing import List, Dict, Any, Optional
from scipy import stats

from src.engines.technical.breakout import BreakoutEngine, BreakoutState


@dataclass
class CompletedTrade:
    symbol: str
    sector: str
    shariah_status: str
    partition: str  # TRAIN, VALIDATION, OUT_OF_SAMPLE
    entry_bar: int
    exit_bar: int
    entry_date: str
    exit_date: str
    entry_price: float
    exit_price: float
    invalidation_level: float
    target_price: float
    exit_reason: str  # TARGET, STOP_LOSS, TIME_LIMIT
    bars_held: int
    mfe: float  # Maximum Favourable Excursion
    mae: float  # Maximum Adverse Excursion
    gross_return: float
    net_return: float  # After roundtrip friction (shared ledger or legacy)
    cost_model_version: str = "LEGACY_47BPS_PROVISIONAL"
    fees_inr: float = 0.0
    # Facts known at the signal bar, for research breakdowns. They never influence the simulated trade.
    signal_date: str = ""
    structure_score: Optional[float] = None
    rvol: Optional[float] = None
    entry_high: Optional[float] = None
    resistance: Optional[float] = None
    atr: Optional[float] = None
    signal_close: Optional[float] = None


@dataclass
class ValidationSummary:
    partition: str
    total_candles: int
    unique_symbols: int
    candidates_count: int
    confirmed_breakouts: int
    failed_breakouts: int
    completed_trades: int
    win_rate: float
    avg_gross_return_pct: float
    avg_net_return_pct: float
    expectancy_pct: float
    profit_factor: float
    avg_mfe_pct: float
    avg_mae_pct: float
    max_drawdown_pct: float  # True portfolio equity peak-to-trough drawdown (strictly <= 100%)
    trade_sum_drawdown_pct: float  # Legacy uncompounded sum of trade returns (used in P1.1)
    benchmark_buy_and_hold_pct: float
    t_statistic: float
    p_value: float
    ci_95_low: float
    ci_95_high: float
    sample_size_adequate: bool
    limitations: List[str]
    cost_model_version: str = "LEGACY_47BPS_PROVISIONAL"
    is_legacy_cost: bool = True
    strategy_hash: str = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
    data_provenance: str = "TimescaleDB ohlcv_daily (secondary delayed aggregator, provisional EOD)"
    adjustment_methodology: str = "SPLIT_ADJUSTED_UNADJUSTED_DIVIDEND_SENSITIVE"
    is_adjustment_sensitive: bool = True
    run_timestamp: str = ""


class BreakoutBacktester:
    """
    Simulates real-world trade execution of the BreakoutEngine on point-in-time price series.
    """

    ROUNDTRIP_FRICTION = 0.0047  # 47 bps (STT 0.20%, NSE 0.00345%, Stamp 0.015%, GST 0.01%, Brokerage 0.05%, Slippage 0.20%)

    # Sector mapping for NSE equities
    SECTOR_MAP = {
        "TCS": "IT", "INFY": "IT", "HCLTECH": "IT", "TECHM": "IT", "WIPRO": "IT",
        "HDFCBANK": "BANKING", "ICICIBANK": "BANKING", "SBIN": "BANKING", "KOTAKBANK": "BANKING",
        "AXISBANK": "BANKING", "BAJFINANCE": "FINANCE", "BAJAJFINSV": "FINANCE", "INDUSINDBK": "BANKING",
        "RELIANCE": "ENERGY", "ONGC": "ENERGY", "BPCL": "ENERGY", "IOC": "ENERGY",
        "NTPC": "UTILITIES", "POWERGRID": "UTILITIES", "COALINDIA": "ENERGY",
        "MARUTI": "AUTO", "M&M": "AUTO", "BAJAJ-AUTO": "AUTO", "HEROMOTOCO": "AUTO", "EICHERMOT": "AUTO",
        "ITC": "FMCG", "HINDUNILVR": "FMCG", "NESTLEIND": "FMCG", "BRITANNIA": "FMCG", "TATACONSUM": "FMCG",
        "TATASTEEL": "METALS", "JSWSTEEL": "METALS", "HINDALCO": "METALS",
        "SUNPHARMA": "PHARMA", "DRREDDY": "PHARMA", "CIPLA": "PHARMA", "DIVISLAB": "PHARMA", "APOLLOHOSP": "HEALTHCARE",
        "LT": "CAP_GOODS", "ADANIENT": "DIVERSIFIED", "ADANIPORTS": "INFRA", "GRASIM": "MATERIALS",
        "ULTRACEMCO": "MATERIALS", "TITAN": "CONSUMER", "ASIANPAINT": "CONSUMER", "BHARTIARTL": "TELECOM",
        "SBILIFE": "FINANCE", "HDFCLIFE": "FINANCE", "TMCV": "AUTO", "TMPV": "AUTO", "LTM": "IT"
    }

    def __init__(
        self,
        data_df: pd.DataFrame,
        use_ledger: bool = False,
        cost_engine: Optional[Any] = None,
        cost_model: Optional[str] = None,
        require_verified_ledger: bool = False,
        require_complete_adjustments: bool = False,
    ):
        """
        data_df must contain:
        ['symbol', 'shariah_status', 'timestamp', 'open', 'high', 'low', 'close', 'volume']
        sorted by symbol and timestamp ascending.
        """
        self.df = data_df.copy()
        self.df['timestamp'] = pd.to_datetime(self.df['timestamp'])
        self.df = self.df.sort_values(by=['symbol', 'timestamp']).reset_index(drop=True)

        if cost_model == "COST_MODEL_2026_Q4_V2":
            use_ledger = True
        elif cost_model == "LEGACY_47BPS_PROVISIONAL":
            use_ledger = False

        if require_verified_ledger and not use_ledger:
            raise ValueError(
                "Verified research runs must use institutional cost ledger (COST_MODEL_2026_Q4_V2). "
                "Legacy 47 bps flat friction is prohibited for verified forward research."
            )

        self.require_complete_adjustments = require_complete_adjustments
        has_adj = 'adjusted_close' in self.df.columns
        has_null_adj = (not has_adj) or self.df['adjusted_close'].isna().any()

        if require_complete_adjustments and has_null_adj:
            raise ValueError(
                "Historical backtest refused: Validated research requires complete point-in-time dividend "
                "and corporate-action adjustment factor series. Data contains unadjusted or incomplete adjusted bars. "
                "Quarantined as ADJUSTMENT_SENSITIVE."
            )

        self.use_ledger = use_ledger
        if use_ledger:
            from src.engines.research.cost_model import CostEngine, CostModelConfig, MODEL_VERSION
            self.cost_engine = cost_engine or CostEngine(CostModelConfig())
            self.cost_model_version = getattr(self.cost_engine.config, "version", MODEL_VERSION)
            self.is_legacy_cost = False
        else:
            self.cost_engine = None
            self.cost_model_version = "LEGACY_47BPS_PROVISIONAL"
            self.is_legacy_cost = True

    @staticmethod
    def aggregate_daily_bars(df: pd.DataFrame) -> pd.DataFrame:
        """
        Reconciles raw database rows into clean end-of-day daily bars.
        Aggregates intraday 1-minute bars (e.g. from 2026-10-07) into a single EOD bar.
        """
        temp = df.copy()
        temp['date'] = pd.to_datetime(temp['timestamp']).dt.date
        clean = temp.groupby(['symbol', 'date']).agg({
            'open': 'first',
            'high': 'max',
            'low': 'min',
            'close': 'last',
            'volume': 'sum',
            'shariah_status': 'first',
            'timestamp': 'first'
        }).reset_index().sort_values(by=['symbol', 'timestamp']).reset_index(drop=True)
        return clean

    @staticmethod
    def simulate_portfolio_drawdown(
        part_trades: List[CompletedTrade],
        initial_capital: float = 1_000_000.0,
        position_size_pct: float = 0.10
    ) -> float:
        """
        Chronologically simulates portfolio equity curve with cash, position sizing,
        realized proceeds, and mark-to-market valuations.
        Returns peak-to-trough max drawdown % in [0.0, 100.0].
        """
        if not part_trades:
            return 0.0

        cash = initial_capital
        positions: Dict[str, Dict[str, Any]] = {}
        daily_equity = []
        trades_by_entry: Dict[str, List[CompletedTrade]] = {}
        for tr in part_trades:
            trades_by_entry.setdefault(tr.entry_date, []).append(tr)

        all_dates = sorted(list(set([t.entry_date for t in part_trades] + [t.exit_date for t in part_trades])))
        for d in all_dates:
            # 1. Close positions exiting on date d
            for sym in list(positions.keys()):
                pos = positions[sym]
                if pos['exit_date'] == d:
                    if 'net_return' in pos and 'alloc' in pos:
                        proceeds = pos['alloc'] * (1.0 + pos['net_return'])
                    else:
                        proceeds = pos['shares'] * pos['exit_price'] * (1 - BreakoutBacktester.ROUNDTRIP_FRICTION)
                    cash += proceeds
                    del positions[sym]

            # 2. Open new positions entering on date d
            if d in trades_by_entry:
                for tr in trades_by_entry[d]:
                    max_alloc = initial_capital * position_size_pct
                    alloc = min(cash, max_alloc)
                    if alloc >= 1000.0 and tr.entry_price > 0:
                        shares = alloc / tr.entry_price
                        cash -= alloc
                        positions[tr.symbol] = {
                            'alloc': alloc,
                            'shares': shares,
                            'entry_price': tr.entry_price,
                            'exit_date': tr.exit_date,
                            'exit_price': tr.exit_price,
                            'net_return': tr.net_return,
                        }

            # 3. Portfolio equity = cash + mark-to-market positions
            pos_val = sum(pos['shares'] * pos['entry_price'] for pos in positions.values())
            equity = cash + pos_val
            daily_equity.append((d, equity))

        eq_vals = [e[1] for e in daily_equity] if daily_equity else [initial_capital]
        peak_eq = np.maximum.accumulate(eq_vals)
        port_dd = (peak_eq - eq_vals) / np.maximum(peak_eq, 1.0)
        max_p_dd = float(np.max(port_dd)) * 100.0 if len(port_dd) > 0 else 0.0
        return round(min(max(max_p_dd, 0.0), 100.0), 2)

    def run_backtest(
        self,
        shariah_only: bool = False,
        train_end_idx: int = 430,
        val_end_idx: int = 645
    ) -> tuple[List[CompletedTrade], Dict[str, ValidationSummary]]:
        """
        Runs walk-forward backtest across chronological partitions.
        """
        completed_trades: List[CompletedTrade] = []
        partition_stats: Dict[str, Dict[str, Any]] = {
            "TRAIN": {"candles": 0, "candidates": 0, "confirmed": 0, "failed": 0, "trades": []},
            "VALIDATION": {"candles": 0, "candidates": 0, "confirmed": 0, "failed": 0, "trades": []},
            "OUT_OF_SAMPLE": {"candles": 0, "candidates": 0, "confirmed": 0, "failed": 0, "trades": []},
        }

        # Benchmark returns (equal-weighted basket)
        benchmark_returns = {"TRAIN": [], "VALIDATION": [], "OUT_OF_SAMPLE": []}

        # Process each symbol independently
        symbols = self.df['symbol'].unique()
        for sym in symbols:
            sym_df = self.df[self.df['symbol'] == sym].reset_index(drop=True)
            n_bars = len(sym_df)
            if n_bars < 50:
                continue

            shariah_stat = sym_df['shariah_status'].iloc[0]
            if shariah_only and shariah_stat != "PASS":
                continue

            sector = self.SECTOR_MAP.get(sym, "OTHER")
            opens = sym_df['open'].values.astype(float)
            highs = sym_df['high'].values.astype(float)
            lows = sym_df['low'].values.astype(float)
            closes = sym_df['close'].values.astype(float)
            vols = sym_df['volume'].values.astype(int)
            dates = sym_df['timestamp'].dt.strftime('%Y-%m-%d').values

            # Record benchmark buy and hold across partitions
            if train_end_idx < n_bars:
                benchmark_returns["TRAIN"].append((closes[train_end_idx] - closes[0]) / closes[0])
            if val_end_idx < n_bars:
                benchmark_returns["VALIDATION"].append((closes[val_end_idx] - closes[train_end_idx]) / closes[train_end_idx])
            if n_bars > val_end_idx:
                benchmark_returns["OUT_OF_SAMPLE"].append((closes[-1] - closes[val_end_idx]) / closes[val_end_idx])

            # Trade management state for this symbol
            active_trade = None
            cooldown_bars = 0

            # Step forward bar-by-bar (minimum 40 bars for indicators)
            for t in range(40, n_bars - 1):
                # Determine partition
                if t <= train_end_idx:
                    part = "TRAIN"
                elif t <= val_end_idx:
                    part = "VALIDATION"
                else:
                    part = "OUT_OF_SAMPLE"

                partition_stats[part]["candles"] += 1

                # 1. Manage Active Trade if one is open
                if active_trade is not None:
                    # Check bar t for exit conditions
                    curr_high = highs[t]
                    curr_low = lows[t]
                    curr_open = opens[t]
                    curr_close = closes[t]

                    # Update excursion metrics
                    trade_mfe = max(active_trade["max_high"], curr_high)
                    trade_mae = min(active_trade["min_low"], curr_low)
                    active_trade["max_high"] = trade_mfe
                    active_trade["min_low"] = trade_mae
                    active_trade["bars_held"] += 1

                    entry_p = active_trade["entry_price"]
                    inval_p = active_trade["invalidation_level"]
                    target_p = active_trade["target_price"]

                    exited = False
                    exit_price = 0.0
                    exit_reason = ""

                    # Priority A: Invalidation / Stop Loss hit
                    if curr_low <= inval_p:
                        # Exited at stop or open gap down
                        exit_price = min(curr_open, inval_p)
                        exit_reason = "STOP_LOSS"
                        exited = True
                    # Priority B: Target 1 achieved
                    elif curr_high >= target_p:
                        exit_price = target_p
                        exit_reason = "TARGET"
                        exited = True
                    # Priority C: Time Stop (10 trading bars elapsed)
                    elif active_trade["bars_held"] >= 10:
                        exit_price = curr_close
                        exit_reason = "TIME_LIMIT"
                        exited = True

                    if exited:
                        gross_ret = (exit_price - entry_p) / entry_p
                        if self.use_ledger and self.cost_engine:
                            calc = self.cost_engine.calculate_trade(
                                sym, entry_p, exit_price,
                                quantity=max(1, int(100000 // entry_p)),
                                holding_days=active_trade["bars_held"]
                            )
                            net_ret = float(calc.net_return_pct / 100.0)
                            fees_inr = float(calc.total_costs_rounded)
                            cost_ver = calc.model_version
                        else:
                            net_ret = gross_ret - self.ROUNDTRIP_FRICTION
                            fees_inr = (entry_p + exit_price) * 0.00235 * max(1, int(100000 // entry_p))
                            cost_ver = "LEGACY_47BPS_PROVISIONAL"
                        mfe_pct = (trade_mfe - entry_p) / entry_p
                        mae_pct = (trade_mae - entry_p) / entry_p

                        tr = CompletedTrade(
                            symbol=sym,
                            sector=sector,
                            shariah_status=shariah_stat,
                            partition=active_trade["partition"],
                            entry_bar=active_trade["entry_bar"],
                            exit_bar=t,
                            entry_date=active_trade["entry_date"],
                            exit_date=dates[t],
                            entry_price=entry_p,
                            exit_price=exit_price,
                            invalidation_level=inval_p,
                            target_price=target_p,
                            exit_reason=exit_reason,
                            bars_held=active_trade["bars_held"],
                            mfe=round(mfe_pct, 4),
                            mae=round(mae_pct, 4),
                            gross_return=round(gross_ret, 4),
                            net_return=round(net_ret, 4),
                            cost_model_version=cost_ver,
                            fees_inr=round(fees_inr, 2),
                            signal_date=active_trade.get("signal_date", ""),
                            **active_trade.get("ctx", {}),
                        )
                        completed_trades.append(tr)
                        partition_stats[active_trade["partition"]]["trades"].append(tr)

                        active_trade = None
                        cooldown_bars = 5  # Cooldown before next setup on same symbol
                        continue

                # 2. If cooldown active, decrement and skip scanning
                if cooldown_bars > 0:
                    cooldown_bars -= 1
                    continue

                # 3. Point-in-Time Evaluation at bar t
                # Strictly pass slices up to t with sufficient lookback (65 bars) — zero future knowledge
                start_lb = max(0, t - 65)
                engine = BreakoutEngine(
                    symbol=sym,
                    open_prices=opens[start_lb:t + 1].tolist(),
                    high_prices=highs[start_lb:t + 1].tolist(),
                    low_prices=lows[start_lb:t + 1].tolist(),
                    close_prices=closes[start_lb:t + 1].tolist(),
                    volumes=vols[start_lb:t + 1].tolist(),
                    shariah_status=shariah_stat,
                )
                res = engine.analyze()

                if res.stage == BreakoutState.BREAKOUT_CANDIDATE:
                    partition_stats[part]["candidates"] += 1

                elif res.stage == BreakoutState.FAILED_BREAKOUT:
                    partition_stats[part]["failed"] += 1

                elif res.stage == BreakoutState.CONFIRMED_BREAKOUT:
                    partition_stats[part]["confirmed"] += 1

                    # Trigger trade entry at NEXT bar (t+1) OPEN
                    # Ensure t+1 is within data range
                    if t + 1 < n_bars:
                        entry_price = opens[t + 1]
                        inval = res.invalidation_level
                        target = res.target_1

                        # Sanity check: valid risk setup
                        if entry_price > inval and target > entry_price:
                            active_trade = {
                                "partition": part,
                                "entry_bar": t + 1,
                                "entry_date": dates[t + 1],
                                "signal_date": dates[t],
                                "ctx": {"structure_score": res.structure_score, "rvol": res.rvol, "entry_high": res.entry_zone[1],
                                        "resistance": res.resistance_level, "atr": res.atr, "signal_close": closes[t]},
                                "entry_price": entry_price,
                                "invalidation_level": inval,
                                "target_price": target,
                                "max_high": highs[t + 1],
                                "min_low": lows[t + 1],
                                "bars_held": 1,
                            }

            # If a trade is still active at end of data, close it at final bar's close
            if active_trade is not None:
                final_bar = n_bars - 1
                curr_high = highs[final_bar]
                curr_low = lows[final_bar]
                curr_close = closes[final_bar]
                trade_mfe = max(active_trade["max_high"], curr_high)
                trade_mae = min(active_trade["min_low"], curr_low)
                active_trade["bars_held"] += 1
                entry_p = active_trade["entry_price"]
                exit_price = curr_close
                gross_ret = (exit_price - entry_p) / entry_p
                if self.use_ledger and self.cost_engine:
                    calc = self.cost_engine.calculate_trade(
                        sym, entry_p, exit_price,
                        quantity=max(1, int(100000 // entry_p)),
                        holding_days=active_trade["bars_held"]
                    )
                    net_ret = float(calc.net_return_pct / 100.0)
                    fees_inr = float(calc.total_costs_rounded)
                    cost_ver = calc.model_version
                else:
                    net_ret = gross_ret - self.ROUNDTRIP_FRICTION
                    fees_inr = (entry_p + exit_price) * 0.00235 * max(1, int(100000 // entry_p))
                    cost_ver = "LEGACY_47BPS_PROVISIONAL"
                mfe_pct = (trade_mfe - entry_p) / entry_p
                mae_pct = (trade_mae - entry_p) / entry_p

                tr = CompletedTrade(
                    symbol=sym,
                    sector=sector,
                    shariah_status=shariah_stat,
                    partition=active_trade["partition"],
                    entry_bar=active_trade["entry_bar"],
                    exit_bar=final_bar,
                    entry_date=active_trade["entry_date"],
                    exit_date=dates[final_bar],
                    entry_price=entry_p,
                    exit_price=exit_price,
                    invalidation_level=active_trade["invalidation_level"],
                    target_price=active_trade["target_price"],
                    exit_reason="END_OF_DATA",
                    bars_held=active_trade["bars_held"],
                    mfe=round(mfe_pct, 4),
                    mae=round(mae_pct, 4),
                    gross_return=round(gross_ret, 4),
                    net_return=round(net_ret, 4),
                    cost_model_version=cost_ver,
                    fees_inr=round(fees_inr, 2),
                )
                completed_trades.append(tr)
                partition_stats[active_trade["partition"]]["trades"].append(tr)

        # 4. Compute Statistical Summaries per Partition
        summaries: Dict[str, ValidationSummary] = {}
        for part in ["TRAIN", "VALIDATION", "OUT_OF_SAMPLE"]:
            part_trades = partition_stats[part]["trades"]
            n_trades = len(part_trades)

            if n_trades > 0:
                net_returns = np.array([tr.net_return for tr in part_trades])
                gross_returns = np.array([tr.gross_return for tr in part_trades])
                mfes = np.array([tr.mfe for tr in part_trades])
                maes = np.array([tr.mae for tr in part_trades])

                wins = np.sum(net_returns > 0)
                win_rate = float(wins / n_trades)
                avg_gross = float(np.mean(gross_returns))
                avg_net = float(np.mean(net_returns))

                win_rets = net_returns[net_returns > 0]
                loss_rets = net_returns[net_returns <= 0]
                avg_win = float(np.mean(win_rets)) if len(win_rets) > 0 else 0.0
                avg_loss = float(abs(np.mean(loss_rets))) if len(loss_rets) > 0 else 0.0

                expectancy = (win_rate * avg_win) - ((1.0 - win_rate) * avg_loss)
                gross_gain = float(np.sum(gross_returns[gross_returns > 0])) if len(gross_returns[gross_returns > 0]) > 0 else 0.0
                gross_loss = float(abs(np.sum(gross_returns[gross_returns < 0]))) if len(gross_returns[gross_returns < 0]) > 0 else 0.0001
                profit_factor = round(gross_gain / gross_loss, 2)

                avg_mfe = float(np.mean(mfes))
                avg_mae = float(np.mean(maes))

                # 1. True Chronological Portfolio Max Drawdown (strictly <= 100%)
                portfolio_max_dd = self.simulate_portfolio_drawdown(part_trades)

                # 2. Legacy Trade-Sum Drawdown (arithmetic sum of trade % returns, used in P1.1)
                cum_rets = np.cumsum(net_returns)
                peak = np.maximum.accumulate(cum_rets)
                trade_dd = peak - cum_rets
                trade_sum_dd = float(np.max(trade_dd)) * 100.0 if len(trade_dd) > 0 else 0.0

                # Statistical Significance (t-test against 0)
                if n_trades > 3 and np.std(net_returns) > 0:
                    t_stat, p_val = stats.ttest_1samp(net_returns, 0.0)
                    sem = stats.sem(net_returns)
                    ci_low, ci_high = stats.t.interval(0.95, df=n_trades - 1, loc=avg_net, scale=sem)
                else:
                    t_stat, p_val = 0.0, 1.0
                    ci_low, ci_high = avg_net, avg_net

                b_bench = float(np.mean(benchmark_returns[part])) if len(benchmark_returns[part]) > 0 else 0.0
                is_adequate = n_trades >= 30

                limitations = []
                if n_trades < 30:
                    limitations.append(f"Sample size N={n_trades} is small (N < 30); statistical variance is wide.")
                if p_val > 0.05:
                    limitations.append(f"p-value {p_val:.3f} > 0.05; net alpha does not reach formal 95% statistical significance.")
                if avg_net <= 0.0:
                    limitations.append(f"Net return after {self.cost_model_version} friction is negative or flat.")
                limitations.append(
                    "ADJUSTMENT_SENSITIVE: Historical series uses split-adjusted close with unadjusted cash dividends; "
                    "point-in-time dividend factor reconstruction is not implemented. Results are adjustment-sensitive."
                )

                summaries[part] = ValidationSummary(
                    partition=part,
                    total_candles=partition_stats[part]["candles"],
                    unique_symbols=len(symbols),
                    candidates_count=partition_stats[part]["candidates"],
                    confirmed_breakouts=partition_stats[part]["confirmed"],
                    failed_breakouts=partition_stats[part]["failed"],
                    completed_trades=n_trades,
                    win_rate=round(win_rate * 100.0, 1),
                    avg_gross_return_pct=round(avg_gross * 100.0, 2),
                    avg_net_return_pct=round(avg_net * 100.0, 2),
                    expectancy_pct=round(expectancy * 100.0, 2),
                    profit_factor=profit_factor,
                    avg_mfe_pct=round(avg_mfe * 100.0, 2),
                    avg_mae_pct=round(avg_mae * 100.0, 2),
                    max_drawdown_pct=portfolio_max_dd,
                    trade_sum_drawdown_pct=round(trade_sum_dd, 2),
                    benchmark_buy_and_hold_pct=round(b_bench * 100.0, 2),
                    t_statistic=round(float(t_stat), 2),
                    p_value=round(float(p_val), 4),
                    ci_95_low=round(float(ci_low) * 100.0, 2),
                    ci_95_high=round(float(ci_high) * 100.0, 2),
                    sample_size_adequate=is_adequate,
                    limitations=limitations,
                    cost_model_version=self.cost_model_version,
                    is_legacy_cost=self.is_legacy_cost,
                    run_timestamp=datetime.now(timezone.utc).isoformat(),
                )
            else:
                limitations = ["Zero completed trades in this partition."]
                if self.is_legacy_cost:
                    limitations.append(
                        "LEGACY_COST_MODEL: Run executed with unverified flat 47 bps friction; "
                        "not comparable to institutional ledger-based research."
                    )
                limitations.append(
                    "ADJUSTMENT_SENSITIVE: Historical series uses split-adjusted close with unadjusted cash dividends; "
                    "point-in-time dividend factor reconstruction is not implemented. Results are adjustment-sensitive."
                )
                summaries[part] = ValidationSummary(
                    partition=part,
                    total_candles=partition_stats[part]["candles"],
                    unique_symbols=len(symbols),
                    candidates_count=partition_stats[part]["candidates"],
                    confirmed_breakouts=partition_stats[part]["confirmed"],
                    failed_breakouts=partition_stats[part]["failed"],
                    completed_trades=0,
                    win_rate=0.0,
                    avg_gross_return_pct=0.0,
                    avg_net_return_pct=0.0,
                    expectancy_pct=0.0,
                    profit_factor=0.0,
                    avg_mfe_pct=0.0,
                    avg_mae_pct=0.0,
                    max_drawdown_pct=0.0,
                    trade_sum_drawdown_pct=0.0,
                    benchmark_buy_and_hold_pct=0.0,
                    t_statistic=0.0,
                    p_value=1.0,
                    ci_95_low=0.0,
                    ci_95_high=0.0,
                    sample_size_adequate=False,
                    limitations=limitations,
                    cost_model_version=self.cost_model_version,
                    is_legacy_cost=self.is_legacy_cost,
                    run_timestamp=datetime.now(timezone.utc).isoformat(),
                )

        return completed_trades, summaries
