"""
Deterministic Backtesting Engine.

Simulates trading strategies over historical data.
Strictly processes data sequentially to prevent look-ahead bias.
Accounts for slippage and transaction costs (STT, brokerage, GST).
"""

from pydantic import BaseModel
from typing import List, Optional, Callable
from src.engines.risk.engine import RiskEngine
from src.engines.backtest.metrics import AdvancedMetrics, calculate_portfolio_metrics

class CostModelConfig(BaseModel):
    model_version: str = "india_equity_delivery_v1"
    brokerage_flat: float = 0.0  # ₹0 for equity delivery (Zerodha/Groww)
    stt_percent: float = 0.1  # 0.1% on buy and sell
    exchange_txn_percent: float = 0.00345  # NSE charge
    sebi_turnover_percent: float = 0.0001  # ₹10 per crore
    gst_percent: float = 18.0  # 18% on (brokerage + exchange + sebi)
    stamp_duty_percent: float = 0.015  # 0.015% on buy only

class BacktestConfig(BaseModel):
    initial_capital: float = 100000.0
    slippage_percent: float = 0.1  # 0.1% slippage on entry/exit
    position_size_atr_multiplier: float = 2.0
    risk_per_trade_percent: float = 2.0 # 2% risk of total capital per trade
    cost_model: CostModelConfig = CostModelConfig()


class DailyData(BaseModel):
    timestamp: str
    open: float
    high: float
    low: float
    close: float
    volume: int
    signal_score: float  # Pre-calculated signal score to base decisions on


class Trade(BaseModel):
    symbol: str
    entry_date: str
    entry_price: float
    exit_date: Optional[str] = None
    exit_price: Optional[float] = None
    shares: int
    cost_basis: float
    pnl: Optional[float] = None
    pnl_percent: Optional[float] = None
    fees_paid: float
    exit_reason: Optional[str] = None
    stop_loss: Optional[float] = None
    take_profit: Optional[float] = None

class PortfolioEquity(BaseModel):
    date: str
    total_equity: float
    cash: float
    invested_value: float
    drawdown: float

class BacktestResult(BaseModel):
    initial_capital: float
    final_capital: float
    total_return_percent: float
    max_drawdown_percent: float
    win_rate_percent: float
    total_trades: int
    trades: List[Trade]
    equity_curve: List[PortfolioEquity]
    advanced_metrics: Optional[AdvancedMetrics] = None


class BacktestEngine:
    def __init__(self, config: BacktestConfig):
        self.config = config
        
    def calculate_fees(self, trade_value: float, is_buy: bool) -> float:
        c = self.config.cost_model
        brokerage = c.brokerage_flat
        stt = trade_value * (c.stt_percent / 100.0)
        exch_txn = trade_value * (c.exchange_txn_percent / 100.0)
        sebi = trade_value * (c.sebi_turnover_percent / 100.0)
        gst = (brokerage + exch_txn + sebi) * (c.gst_percent / 100.0)
        stamp_duty = trade_value * (c.stamp_duty_percent / 100.0) if is_buy else 0.0
        
        return brokerage + stt + exch_txn + sebi + gst + stamp_duty

    def run(self, data: dict[str, List[DailyData]], buy_threshold: float, sell_threshold: float) -> BacktestResult:
        if not data:
            raise ValueError("No historical data provided for backtest")

        capital = self.config.initial_capital
        equity_curve = []
        trades = []
        open_positions = {}  # symbol -> Trade
        
        peak_capital = capital
        max_drawdown = 0.0
        
        risk_engine = RiskEngine()
        
        pending_buy_orders = []
        pending_sell_orders = []

        # 1. Align global dates
        all_dates = set()
        data_by_symbol_and_date = {}
        for sym, bars in data.items():
            data_by_symbol_and_date[sym] = {}
            for bar in bars:
                all_dates.add(bar.timestamp)
                data_by_symbol_and_date[sym][bar.timestamp] = bar
                
        sorted_dates = sorted(list(all_dates))

        for today in sorted_dates:
            daily_realized_cash = 0.0
            
            # --- PHASE 1: EXECUTE T-1 PENDING ORDERS AT T OPEN ---
            
            # A. Process Scheduled Exits (SELL signals from T-1)
            remaining_sells = []
            for sym in pending_sell_orders:
                if sym in open_positions and sym in data_by_symbol_and_date and today in data_by_symbol_and_date[sym]:
                    bar = data_by_symbol_and_date[sym][today]
                    exit_price = bar.open * (1 - self.config.slippage_percent / 100.0)
                    pos = open_positions.pop(sym)
                    
                    gross_proceeds = pos.shares * exit_price
                    fees = self.calculate_fees(gross_proceeds, is_buy=False)
                    net_proceeds = gross_proceeds - fees
                    
                    capital += net_proceeds
                    pos.exit_date = today
                    pos.exit_price = exit_price
                    pos.fees_paid += fees
                    pos.pnl = net_proceeds - pos.cost_basis
                    pos.pnl_percent = (pos.pnl / pos.cost_basis) * 100
                    pos.exit_reason = "Signal"
                    trades.append(pos)
                else:
                    remaining_sells.append(sym)
            pending_sell_orders = remaining_sells

            # B. Intraday SL/TP processing (for positions held during the day)
            closed_intraday = []
            for sym, pos in open_positions.items():
                if sym in data_by_symbol_and_date and today in data_by_symbol_and_date[sym]:
                    bar = data_by_symbol_and_date[sym][today]
                    triggered_sl = pos.stop_loss is not None and bar.low <= pos.stop_loss
                    triggered_tp = pos.take_profit is not None and bar.high >= pos.take_profit
                    
                    # Gap logic: if open is already past SL/TP, execute at open
                    if triggered_sl:
                        exec_price = min(bar.open, pos.stop_loss)
                        exit_reason = "Stop-Loss"
                    elif triggered_tp:
                        exec_price = max(bar.open, pos.take_profit)
                        exit_reason = "Take-Profit"
                    else:
                        continue
                        
                    gross_proceeds = pos.shares * exec_price
                    fees = self.calculate_fees(gross_proceeds, is_buy=False)
                    net_proceeds = gross_proceeds - fees
                    
                    capital += net_proceeds
                    pos.exit_date = today
                    pos.exit_price = exec_price
                    pos.fees_paid += fees
                    pos.pnl = net_proceeds - pos.cost_basis
                    pos.pnl_percent = (pos.pnl / pos.cost_basis) * 100
                    pos.exit_reason = exit_reason
                    trades.append(pos)
                    closed_intraday.append(sym)
                    
            for sym in closed_intraday:
                del open_positions[sym]

            # C. Process Scheduled Entries (BUY signals from T-1)
            # Allocate equally among pending buy orders
            if pending_buy_orders:
                max_exposure = capital * 1.0 # 100% max portfolio exposure
                available_for_new = max_exposure
                
                num_orders = len(pending_buy_orders)
                allocated_per_trade = available_for_new / num_orders
                
                remaining_buys = []
                for sym in pending_buy_orders:
                    if sym not in open_positions and sym in data_by_symbol_and_date and today in data_by_symbol_and_date[sym]:
                        bar = data_by_symbol_and_date[sym][today]
                        entry_price = bar.open * (1 + self.config.slippage_percent / 100.0)
                        
                        atr_approx = (bar.high - bar.low)
                        if atr_approx == 0:
                            atr_approx = bar.open * 0.02
                            
                        stop_loss = entry_price - (atr_approx * self.config.position_size_atr_multiplier)
                        
                        try:
                            # Use minimum of allocated capital or risk limits
                            pos_result = risk_engine.calculate_position_size(
                                capital=capital,
                                risk_percentage=self.config.risk_per_trade_percent,
                                entry_price=entry_price,
                                stop_loss=stop_loss
                            )
                            
                            max_shares_by_alloc = int(allocated_per_trade / entry_price)
                            actual_shares = min(pos_result.position_size, max_shares_by_alloc)
                            
                            if actual_shares > 0:
                                trade_value = actual_shares * entry_price
                                fees = self.calculate_fees(trade_value, is_buy=True)
                                cost = trade_value + fees
                                
                                if cost <= capital:
                                    capital -= cost
                                    open_positions[sym] = Trade(
                                        symbol=sym,
                                        entry_date=today,
                                        entry_price=entry_price,
                                        shares=actual_shares,
                                        cost_basis=cost,
                                        fees_paid=fees,
                                        stop_loss=stop_loss
                                    )
                        except ValueError:
                            pass # SL >= entry
                    else:
                        remaining_buys.append(sym)
                pending_buy_orders = [] # Clear daily

            # --- PHASE 2: RECORD EQUITY AT T CLOSE ---
            invested_value = 0.0
            for sym, pos in open_positions.items():
                if sym in data_by_symbol_and_date and today in data_by_symbol_and_date[sym]:
                    invested_value += pos.shares * data_by_symbol_and_date[sym][today].close
                else:
                    # If no data today, use last known value roughly, or just cost basis
                    invested_value += pos.shares * pos.entry_price

            current_equity = capital + invested_value
            
            if current_equity > peak_capital:
                peak_capital = current_equity
            
            current_drawdown = 0.0
            if peak_capital > 0:
                current_drawdown = (peak_capital - current_equity) / peak_capital
                if current_drawdown > max_drawdown:
                    max_drawdown = current_drawdown
            
            equity_curve.append(PortfolioEquity(
                date=today,
                total_equity=current_equity,
                cash=capital,
                invested_value=invested_value,
                drawdown=current_drawdown * 100.0
            ))
            
            # --- PHASE 3: GENERATE SIGNALS AT T CLOSE (FOR T+1) ---
            for sym, bars in data_by_symbol_and_date.items():
                if today in bars:
                    bar = bars[today]
                    if sym not in open_positions:
                        if bar.signal_score >= buy_threshold:
                            pending_buy_orders.append(sym)
                    else:
                        if bar.signal_score <= sell_threshold:
                            pending_sell_orders.append(sym)

        # Force close open positions at the end of the backtest
        if open_positions:
            last_date = sorted_dates[-1]
            for sym, pos in list(open_positions.items()):
                if sym in data_by_symbol_and_date and last_date in data_by_symbol_and_date[sym]:
                    bar = data_by_symbol_and_date[sym][last_date]
                    exit_price = bar.close * (1 - self.config.slippage_percent / 100.0)
                    
                    gross_proceeds = pos.shares * exit_price
                    fees = self.calculate_fees(gross_proceeds, is_buy=False)
                    net_proceeds = gross_proceeds - fees
                    
                    capital += net_proceeds
                    pos.exit_date = last_date
                    pos.exit_price = exit_price
                    pos.fees_paid += fees
                    pos.pnl = net_proceeds - pos.cost_basis
                    pos.pnl_percent = (pos.pnl / pos.cost_basis) * 100
                    pos.exit_reason = "End of Backtest"
                    trades.append(pos)
            
            if equity_curve:
                equity_curve[-1].cash = capital
                equity_curve[-1].invested_value = 0.0
                equity_curve[-1].total_equity = capital

        # Calculate metrics
        winning_trades = sum(1 for t in trades if t.pnl > 0)
        total_trades = len(trades)
        win_rate = (winning_trades / total_trades * 100) if total_trades > 0 else 0.0
        
        total_return = ((capital - self.config.initial_capital) / self.config.initial_capital) * 100

        adv_metrics = None
        if equity_curve:
            eq_vals = [e.total_equity for e in equity_curve]
            dates = [e.date for e in equity_curve]
            adv_metrics = calculate_portfolio_metrics(eq_vals, dates)

        return BacktestResult(
            initial_capital=self.config.initial_capital,
            final_capital=round(capital, 2),
            total_return_percent=round(total_return, 2),
            max_drawdown_percent=round(max_drawdown * 100, 2),
            win_rate_percent=round(win_rate, 2),
            total_trades=total_trades,
            trades=trades,
            equity_curve=equity_curve,
            advanced_metrics=adv_metrics
        )
