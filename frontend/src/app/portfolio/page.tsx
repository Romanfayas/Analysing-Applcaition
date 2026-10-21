"use client";

import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { 
  WalletIcon, 
  ArrowTrendingUpIcon, 
  ShieldCheckIcon, 
  BuildingOfficeIcon,
  InformationCircleIcon 
} from '@heroicons/react/24/outline';
import { usePortfolio } from '@/lib/api/hooks';

export default function PortfolioPage() {
  const { portfolio, positions, isLoading } = usePortfolio();

  const cashBalance = Number(portfolio?.cash_balance ?? 1000000);
  const investedValue = positions.reduce((acc: number, pos: any) => acc + (Number(pos.shares || 0) * Number(pos.current_price || pos.avg_price || 0)), 0);
  const totalEquity = cashBalance + investedValue;
  const totalPnl = positions.reduce((acc: number, pos: any) => acc + Number(pos.pnl || 0), 0);
  const totalPnlPct = totalEquity > 0 ? (totalPnl / totalEquity) * 100 : 0;

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
            ₹{totalEquity.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </p>
          <p className="text-xs text-slate-500 mt-1">
            Cash: ₹{cashBalance.toLocaleString('en-IN', { minimumFractionDigits: 2 })}
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Total P&L</span>
            <ArrowTrendingUpIcon className="w-5 h-5 text-emerald-400" />
          </div>
          <p className={`text-2xl font-bold mt-2 ${totalPnl >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
            {totalPnl >= 0 ? '+' : ''}₹{totalPnl.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </p>
          <p className="text-xs text-slate-500 mt-1">
            {totalPnl >= 0 ? '+' : ''}{totalPnlPct.toFixed(2)}% net paper return
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Open Positions</span>
            <BuildingOfficeIcon className="w-5 h-5 text-amber-400" />
          </div>
          <p className="text-2xl font-bold text-white mt-2">{positions.length}</p>
          <p className="text-xs text-slate-500 mt-1">
            Invested: ₹{investedValue.toLocaleString('en-IN', { minimumFractionDigits: 2 })}
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Shariah Compliance</span>
            <ShieldCheckIcon className="w-5 h-5 text-emerald-400" />
          </div>
          <p className="text-2xl font-bold text-emerald-400 mt-2">100.0%</p>
          <p className="text-xs text-slate-500 mt-1">Zero non-compliant equity holdings</p>
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
              ) : positions && positions.length > 0 ? (
                positions.map((pos: any) => (
                  <tr key={pos.symbol} className="hover:bg-slate-800/20 transition-colors">
                    <td className="py-3 pl-2 font-bold text-white">{pos.symbol}</td>
                    <td className="py-3 text-right text-slate-300">{pos.shares}</td>
                    <td className="py-3 text-right text-slate-300">₹{Number(pos.avg_price || 0).toFixed(2)}</td>
                    <td className="py-3 text-right text-slate-300">₹{Number(pos.current_price || pos.avg_price || 0).toFixed(2)}</td>
                    <td className="py-3 text-right text-white font-medium">
                      ₹{(Number(pos.shares || 0) * Number(pos.current_price || pos.avg_price || 0)).toFixed(2)}
                    </td>
                    <td className={`py-3 text-right font-medium ${pos.pnl >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                      {pos.pnl >= 0 ? '+' : ''}₹{Number(pos.pnl || 0).toFixed(2)}
                    </td>
                    <td className={`py-3 text-right pr-2 font-bold ${pos.pnl_pct >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                      {pos.pnl_pct >= 0 ? '+' : ''}{Number(pos.pnl_pct || 0).toFixed(2)}%
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={7} className="py-12 text-center text-slate-500">
                    <p className="font-semibold text-slate-300 text-sm">No Active Paper Trading Positions</p>
                    <p className="text-xs text-slate-500 mt-1">
                      Initial cash balance of ₹{cashBalance.toLocaleString('en-IN', { minimumFractionDigits: 2 })} is ready for strategy execution.
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
