"use client";

import { 
  ArrowTrendingUpIcon, 
  WalletIcon, 
  ShieldCheckIcon, 
  BoltIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon
} from '@heroicons/react/24/outline';
import StatCard from '@/components/ui/StatCard';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';

import { useBreakouts, usePortfolio, useShariahScreenings } from '@/lib/api/hooks';

export default function Dashboard() {
  const { candidates, stageCounts, isLoading: isLoadingBreakouts } = useBreakouts();
  const { portfolio, positions, isLoading: isLoadingPortfolio } = usePortfolio();
  const { screenings, isLoading: isLoadingShariah } = useShariahScreenings();

  // Accounting reconciliation:
  // Total equity = portfolio.cash_balance + sum(shares * cmp)
  const cashBalance = Number(portfolio?.cash_balance ?? 1000000);
  const investedValue = positions.reduce((acc: number, pos: any) => acc + (Number(pos.shares || 0) * Number(pos.current_price || pos.avg_price || 0)), 0);
  const totalEquity = cashBalance + investedValue;
  const dailyPnl = positions.reduce((acc: number, pos: any) => acc + Number(pos.pnl || 0), 0);
  const dailyPnlPct = totalEquity > 0 ? (dailyPnl / totalEquity) * 100 : 0;

  // Active breakout opportunities:
  const activeOpportunities = (stageCounts?.CONFIRMED_BREAKOUT || 0) + (stageCounts?.BREAKOUT_CANDIDATE || 0);

  // Shariah pass rate:
  const totalScreened = screenings.length;
  const passCount = screenings.filter((s: any) => s.status === 'PASS').length;
  const passRate = totalScreened > 0 ? ((passCount / totalScreened) * 100).toFixed(1) : '100.0';

  return (
    <div className="space-y-8 animate-in fade-in duration-500">
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Executive Research Overview</h1>
          <p className="text-slate-400 mt-1">Traceable Shariah-compliant quantitative research and paper ledger</p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-slate-800/80 border border-slate-700/60 text-xs text-slate-300">
          <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>Paper Trading Active (Broker Execution Disabled)</span>
        </div>
      </div>

      {/* High-level Verified Stats */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <StatCard 
          title="Total Paper Equity" 
          value={`₹${totalEquity.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`} 
          change={investedValue > 0 ? `${((investedValue / totalEquity) * 100).toFixed(1)}% Allocated` : "100% Cash"} 
          isPositive={true}
          icon={<WalletIcon className="w-6 h-6" />} 
        />
        <StatCard 
          title="Daily P&L" 
          value={`${dailyPnl >= 0 ? '+' : ''}₹${dailyPnl.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`} 
          change={`${dailyPnl >= 0 ? '+' : ''}${dailyPnlPct.toFixed(2)}%`} 
          isPositive={dailyPnl >= 0}
          icon={<ArrowTrendingUpIcon className="w-6 h-6" />} 
        />
        <StatCard 
          title="Breakout Setups" 
          value={String(activeOpportunities)} 
          change={`${stageCounts?.CONFIRMED_BREAKOUT || 0} Confirmed`}
          isPositive={true}
          icon={<BoltIcon className="w-6 h-6 text-amber-400" />} 
        />
        <StatCard 
          title="Shariah Pass Rate" 
          value={`${passRate}%`} 
          change={`${passCount}/${totalScreened} Screened`}
          isPositive={true}
          icon={<ShieldCheckIcon className="w-6 h-6 text-emerald-400" />} 
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Active Positions */}
        <div className="lg:col-span-2">
          <Card className="h-full">
            <CardHeader title="Paper Trading Ledger" subtitle="Your active spot positions" />
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-slate-700/50 text-sm font-medium text-slate-400">
                    <th className="pb-3 pl-2">Symbol</th>
                    <th className="pb-3 text-right">Shares</th>
                    <th className="pb-3 text-right">Avg Price</th>
                    <th className="pb-3 text-right">CMP</th>
                    <th className="pb-3 text-right pr-2">Return</th>
                  </tr>
                </thead>
                <tbody className="text-sm">
                  {isLoadingPortfolio ? (
                    <tr>
                      <td colSpan={5} className="py-8 text-center text-slate-500">Loading positions...</td>
                    </tr>
                  ) : positions && positions.length > 0 ? (
                    positions.map((pos: any) => (
                      <tr key={pos.symbol} className="border-b border-slate-800/50 hover:bg-slate-800/20 transition-colors">
                        <td className="py-4 pl-2 font-semibold text-white">{pos.symbol}</td>
                        <td className="py-4 text-right text-slate-300">{pos.shares}</td>
                        <td className="py-4 text-right text-slate-300">₹{Number(pos.avg_price || 0).toFixed(2)}</td>
                        <td className="py-4 text-right text-slate-300">₹{Number(pos.current_price || pos.avg_price || 0).toFixed(2)}</td>
                        <td className="py-4 text-right pr-2 text-emerald-400 font-medium">{(pos.pnl_pct || 0) > 0 ? '+' : ''}{(pos.pnl_pct || 0).toFixed(2)}%</td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={5} className="py-8 text-center text-slate-500">
                        <p className="font-medium text-slate-400">No active paper trading positions</p>
                        <p className="text-xs text-slate-500 mt-1">Cash balance: ₹{cashBalance.toLocaleString('en-IN', { minimumFractionDigits: 2 })} • Ready for execution signals</p>
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </Card>
        </div>

        {/* Recent Breakout Setups */}
        <div>
          <Card className="h-full">
            <CardHeader title="Top Breakout Setups" subtitle="Quantitative scan candidates" />
            <div className="space-y-4">
              {isLoadingBreakouts ? (
                <div className="py-4 text-center text-slate-500">Scanning universe...</div>
              ) : candidates && candidates.length > 0 ? (
                candidates.slice(0, 5).map((cand: any) => (
                  <div key={cand.symbol} className="p-4 rounded-xl bg-slate-800/30 border border-slate-700/50 hover:border-blue-500/30 transition-colors group cursor-pointer">
                    <div className="flex justify-between items-start mb-2">
                      <div>
                        <span className="font-bold text-white group-hover:text-blue-400 transition-colors">{cand.symbol}</span>
                        <span className="text-xs text-slate-400 ml-2">₹{cand.current_price?.toFixed(2)}</span>
                      </div>
                      <Badge variant={cand.stage === 'CONFIRMED_BREAKOUT' ? 'pass' : (cand.stage === 'BREAKOUT_CANDIDATE' ? 'blue' : 'neutral')}>
                        {cand.stage.replace('_', ' ')}
                      </Badge>
                    </div>
                    <div className="flex justify-between items-center text-xs text-slate-400">
                      <span>Score: <strong className="text-white">{cand.overall_score}/100</strong></span>
                      <span>RVOL: <strong className="text-white">{cand.rvol?.toFixed(2)}x</strong></span>
                      <span className={cand.shariah_status === 'PASS' ? 'text-emerald-400 font-medium' : 'text-rose-400 font-medium'}>
                        {cand.shariah_status}
                      </span>
                    </div>
                    <div className="mt-2 text-[11px] text-slate-500 truncate">
                      Entry: ₹{cand.entry_zone?.[0]?.toFixed(2)} - ₹{cand.entry_zone?.[1]?.toFixed(2)} | Target: ₹{cand.target_1?.toFixed(2)}
                    </div>
                  </div>
                ))
              ) : (
                <div className="py-4 text-center text-slate-500">No active breakout setups identified.</div>
              )}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
