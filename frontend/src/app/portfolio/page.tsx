"use client";

import Card, { CardHeader } from '@/components/ui/Card';
import { 
  WalletIcon, 
  ArrowTrendingUpIcon, 
  ShieldCheckIcon, 
  BuildingOfficeIcon,
  InformationCircleIcon 
} from '@heroicons/react/24/outline';
import { usePortfolio, useStocks } from '@/lib/api/hooks';

const inr = (n: number, d = 2) => `₹${n.toLocaleString('en-IN', { minimumFractionDigits: d, maximumFractionDigits: d })}`;

export default function PortfolioPage() {
  const { portfolio, positions, isLoading, isError } = usePortfolio();
  const { stocks } = useStocks();

  // Holdings whose *current* screening is a certified PASS. BUY orders are gated on PASS, but status can change afterwards.
  const statusBySymbol = new Map(stocks.map((st) => [st.symbol, st.shariah]));
  const certifiedHeld = positions.filter((p) => statusBySymbol.get(p.symbol) === 'PASS').length;

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Paper Trading Portfolio</h1>
          <p className="text-slate-400 mt-1">
            Simulated paper execution ledger with strict risk parameters and real-time ledger accounting
          </p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-slate-800/80 border border-slate-700/60 text-xs text-slate-300">
          <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>PAPER_TRADING_ENABLED=true • LIVE_ORDER_EXECUTION=false</span>
        </div>
      </div>

      {/* Accounting Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Total Equity</span>
            <WalletIcon className="w-5 h-5 text-blue-400" />
          </div>
          <p className="text-2xl font-bold text-white mt-2">
            {portfolio ? inr(portfolio.total_equity) : '—'}
          </p>
          <p className="text-xs text-slate-500 mt-1">
            {portfolio ? `Cash: ${inr(portfolio.cash_balance)}` : isError ? 'Ledger unavailable' : 'Loading…'}
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Unrealized P&L</span>
            <ArrowTrendingUpIcon className="w-5 h-5 text-emerald-400" />
          </div>
          <p className={`text-2xl font-bold mt-2 ${(portfolio?.unrealized_pnl ?? 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
            {portfolio ? `${portfolio.unrealized_pnl >= 0 ? '+' : ''}${inr(portfolio.unrealized_pnl)}` : '—'}
          </p>
          <p className="text-xs text-slate-500 mt-1">
            {portfolio ? `${portfolio.unrealized_pnl >= 0 ? '+' : ''}${portfolio.unrealized_pnl_pct.toFixed(2)}% of initial capital, vs cost basis` : ''}
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Open Positions</span>
            <BuildingOfficeIcon className="w-5 h-5 text-amber-400" />
          </div>
          <p className="text-2xl font-bold text-white mt-2">{positions.length}</p>
          <p className="text-xs text-slate-500 mt-1">
            {portfolio ? `Invested: ${inr(portfolio.invested_value)}` : ''}
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Shariah Compliance</span>
            <ShieldCheckIcon className="w-5 h-5 text-emerald-400" />
          </div>
          <p className="text-2xl font-bold text-white mt-2">{positions.length === 0 ? '—' : `${certifiedHeld}/${positions.length}`}</p>
          <p className="text-xs text-slate-500 mt-1">
            {positions.length === 0 ? 'Paper BUYs are not Shariah-gated; each trade records the status' : 'holdings currently certified PASS'}
          </p>
        </div>
      </div>

      {/* Safety Notice */}
      <div className="p-4 rounded-xl bg-slate-800/40 border border-slate-700/60 flex items-start gap-3 text-xs text-slate-400">
        <InformationCircleIcon className="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
        <div>
          <strong className="text-slate-200 block mb-0.5">Execution Safeguards Enforced</strong>
          Broker execution is strictly disabled in application configuration. All trade orders are recorded in the paper trading ledger for statistical walk-forward evaluation without risking capital.
        </div>
      </div>

      {/* Positions Ledger Table */}
      <Card>
        <CardHeader 
          title="Paper Positions Ledger" 
          subtitle="Real-time marked-to-market positions from database" 
        />
        <div className="overflow-x-auto mt-4">
          <table className="w-full text-left border-collapse text-xs">
            <thead>
              <tr className="border-b border-slate-800 text-slate-400 font-semibold uppercase text-[10px]">
                <th className="pb-3 pl-2">Symbol</th>
                <th className="pb-3 text-right">Shares</th>
                <th className="pb-3 text-right">Avg Entry</th>
                <th className="pb-3 text-right">Current Price</th>
                <th className="pb-3 text-right">Position Value</th>
                <th className="pb-3 text-right">P&L (₹)</th>
                <th className="pb-3 text-right pr-2">Return (%)</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {isLoading ? (
                <tr>
                  <td colSpan={7} className="py-8 text-center text-slate-500">Loading ledger positions...</td>
                </tr>
              ) : isError ? (
                <tr>
                  <td colSpan={7} className="py-8 text-center text-rose-400">Could not load the paper ledger.</td>
                </tr>
              ) : positions.length > 0 ? (
                positions.map((pos) => {
                  const last = pos.last_price ?? pos.avg_cost_basis;
                  const value = pos.shares * last;
                  const pnl = pos.unrealized_pnl ?? 0;
                  const pct = pos.avg_cost_basis > 0 ? ((last - pos.avg_cost_basis) / pos.avg_cost_basis) * 100 : 0;
                  return (
                    <tr key={pos.symbol} className="hover:bg-slate-800/20 transition-colors">
                      <td className="py-3 pl-2 font-bold text-white">{pos.symbol}</td>
                      <td className="py-3 text-right text-slate-300">{pos.shares}</td>
                      <td className="py-3 text-right text-slate-300">{inr(pos.avg_cost_basis)}</td>
                      <td className="py-3 text-right text-slate-300">{inr(last)}</td>
                      <td className="py-3 text-right text-white font-medium">{inr(value)}</td>
                      <td className={`py-3 text-right font-medium ${pnl >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                        {pnl >= 0 ? '+' : ''}{inr(pnl)}
                      </td>
                      <td className={`py-3 text-right pr-2 font-bold ${pct >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                        {pct >= 0 ? '+' : ''}{pct.toFixed(2)}%
                      </td>
                    </tr>
                  );
                })
              ) : (
                <tr>
                  <td colSpan={7} className="py-12 text-center text-slate-500">
                    <p className="font-semibold text-slate-300 text-sm">No Active Paper Trading Positions</p>
                    <p className="text-xs text-slate-500 mt-1">
                      {portfolio ? `Cash balance of ${inr(portfolio.cash_balance)} is available for paper execution.` : ''}
                    </p>
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
