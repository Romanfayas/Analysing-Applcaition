"use client";

import {
  ArrowTrendingUpIcon,
  WalletIcon,
  ShieldCheckIcon,
  BoltIcon,
  ExclamationTriangleIcon,
} from '@heroicons/react/24/outline';
import Link from '@/components/Link';
import StatCard from '@/components/ui/StatCard';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { useBreakouts, useMarketDataHealth, useMarketOverview, usePortfolio, useStocks } from '@/lib/api/hooks';
import { shariahLabel } from '@/lib/shariah';
import type { LivePosition } from '@/lib/api/types';

const inr = (n: number) => `₹${n.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
const DASH = '—';

const LIVE_TEXT: Record<LivePosition['status'], string> = {
  IN_ENTRY_ZONE: 'In the entry zone now',
  ABOVE_ENTRY_ZONE: 'Above the entry zone: do not chase',
  BELOW_ENTRY_ZONE: 'Below the entry zone',
  INVALIDATED: 'Setup failed',
  TARGET_1_REACHED: 'Target 1 already reached',
  TARGET_2_REACHED: 'Target 2 already reached',
};

export default function Dashboard() {
  const { candidates, stageCounts, isLoading: isLoadingBreakouts, isError: breakoutsFailed } = useBreakouts();
  const { portfolio, positions, isLoading: isLoadingPortfolio, isError: portfolioFailed } = usePortfolio();
  const { stocks } = useStocks();
  const { health } = useMarketDataHealth();
  const { data: overview } = useMarketOverview();

  const activeOpportunities = (stageCounts.CONFIRMED_BREAKOUT || 0) + (stageCounts.BREAKOUT_CANDIDATE || 0);

  // Shariah screening is not automated yet: report what the data actually says, and never
  // present an unevaluated instrument as compliant.
  const certified = stocks.filter((s) => s.shariah === 'PASS').length;
  const failed = stocks.filter((s) => s.shariah === 'FAIL').length;
  const unevaluated = stocks.length - certified - failed;

  const stale = health && health.freshness_status !== 'FRESH' && health.freshness_status !== 'LAST_AVAILABLE';

  return (
    <div className="space-y-8 animate-in fade-in duration-500">
      <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Executive Research Overview</h1>
          <p className="text-slate-400 mt-1">Traceable Shariah-conscious quantitative research and paper ledger</p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-slate-800/80 border border-slate-700/60 text-xs text-slate-300">
          <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>Paper Trading Active (Broker Execution Disabled)</span>
        </div>
      </div>

      {stale && (
        <div role="status" className="flex items-start gap-3 p-4 rounded-xl bg-amber-500/10 border border-amber-500/30 text-amber-200 text-sm">
          <ExclamationTriangleIcon className="w-5 h-5 shrink-0 mt-0.5" />
          <div>
            <strong>Market data is {health?.freshness_status?.toLowerCase()}.</strong> The most recent stored bar is from{' '}
            {health?.latest_market_timestamp ? new Date(health.latest_market_timestamp).toLocaleString() : 'an unknown time'}.
            Prices and freshness-gated signals below may not reflect the current market.
          </div>
        </div>
      )}

      {overview && overview.indices.some((x) => x.value != null) && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          {overview.indices.filter((x) => x.value != null).map((x) => (
            <div key={x.symbol} className="px-4 py-3 rounded-xl bg-slate-900/60 border border-slate-800">
              <div className="flex justify-between text-xs text-slate-400"><span>{x.name}</span><span>{x.pe != null ? `P/E ${x.pe}` : ''}</span></div>
              <p className="font-mono text-lg text-white mt-0.5">{x.value?.toLocaleString('en-IN')}
                {x.change_percent != null && <span className={`text-xs ml-2 ${x.change_percent >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>{x.change_percent >= 0 ? '+' : ''}{x.change_percent.toFixed(2)}%</span>}
              </p>
              <p className="text-[10px] text-slate-500">NSE close of {x.as_of}</p>
            </div>
          ))}
        </div>
      )}

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <StatCard
          title="Total Paper Equity"
          value={portfolio ? inr(portfolio.total_equity) : DASH}
          change={portfolio ? (portfolio.invested_value > 0 ? `${((portfolio.invested_value / portfolio.total_equity) * 100).toFixed(1)}% allocated` : '100% cash') : undefined}
          icon={<WalletIcon className="w-6 h-6" />}
        />
        <StatCard
          title="Unrealized P&L"
          value={portfolio ? `${portfolio.unrealized_pnl >= 0 ? '+' : ''}${inr(portfolio.unrealized_pnl)}` : DASH}
          change={portfolio ? `${portfolio.unrealized_pnl >= 0 ? '+' : ''}${portfolio.unrealized_pnl_pct.toFixed(2)}%` : undefined}
          changeLabel="of initial capital"
          isPositive={portfolio ? portfolio.unrealized_pnl >= 0 : undefined}
          icon={<ArrowTrendingUpIcon className="w-6 h-6" />}
        />
        <StatCard
          title="Breakout Setups"
          value={breakoutsFailed ? DASH : String(activeOpportunities)}
          change={breakoutsFailed ? 'scan unavailable' : `${stageCounts.CONFIRMED_BREAKOUT || 0} confirmed · ${stageCounts.BREAKOUT_CANDIDATE || 0} upcoming`}
          icon={<BoltIcon className="w-6 h-6 text-amber-400" />}
        />
        <StatCard
          title="Shariah Screening"
          value={stocks.length === 0 ? DASH : `${certified} compliant`}
          change={stocks.length === 0 ? undefined : `${failed} not compliant · ${unevaluated} not evaluated of ${stocks.length}; from vendor figures, not an audited ruling`}
          icon={<ShieldCheckIcon className="w-6 h-6 text-emerald-400" />}
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2">
          <Card className="h-full">
            <CardHeader title="Paper Trading Ledger" subtitle="Your active spot positions" />
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-slate-700/50 text-sm font-medium text-slate-400">
                    <th className="pb-3 pl-2">Symbol</th>
                    <th className="pb-3 text-right">Shares</th>
                    <th className="pb-3 text-right">Avg Cost</th>
                    <th className="pb-3 text-right">Last</th>
                    <th className="pb-3 text-right pr-2">Unrealized</th>
                  </tr>
                </thead>
                <tbody className="text-sm">
                  {isLoadingPortfolio ? (
                    <tr><td colSpan={5} className="py-8 text-center text-slate-500">Loading positions...</td></tr>
                  ) : portfolioFailed ? (
                    <tr><td colSpan={5} className="py-8 text-center text-rose-400">Could not load the paper ledger.</td></tr>
                  ) : positions.length > 0 ? (
                    positions.map((pos) => {
                      const last = pos.last_price ?? pos.avg_cost_basis;
                      const pct = pos.avg_cost_basis > 0 ? ((last - pos.avg_cost_basis) / pos.avg_cost_basis) * 100 : 0;
                      return (
                        <tr key={pos.symbol} className="border-b border-slate-800/50 hover:bg-slate-800/20 transition-colors">
                          <td className="py-4 pl-2 font-semibold text-white">{pos.symbol}</td>
                          <td className="py-4 text-right text-slate-300">{pos.shares}</td>
                          <td className="py-4 text-right text-slate-300">{inr(pos.avg_cost_basis)}</td>
                          <td className="py-4 text-right text-slate-300">{inr(last)}</td>
                          <td className={`py-4 text-right pr-2 font-medium ${pct >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                            {pct >= 0 ? '+' : ''}{pct.toFixed(2)}%
                          </td>
                        </tr>
                      );
                    })
                  ) : (
                    <tr>
                      <td colSpan={5} className="py-8 text-center text-slate-500">
                        <p className="font-medium text-slate-400">No active paper trading positions</p>
                        {portfolio && <p className="text-xs text-slate-500 mt-1">Cash balance: {inr(portfolio.cash_balance)}</p>}
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </Card>
        </div>

        <div>
          <Card className="h-full">
            <CardHeader title="Top Breakout Setups" subtitle="From the last close. Historically these lost money after costs: study them, don't chase them." />
            <div className="space-y-4">
              {isLoadingBreakouts ? (
                <div className="py-4 text-center text-slate-500">Scanning universe...</div>
              ) : breakoutsFailed ? (
                <div className="py-4 text-center text-rose-400">The breakout scan is unavailable.</div>
              ) : candidates.length > 0 ? (
                [...candidates].sort((a, b) => (a.stage === 'CONFIRMED_BREAKOUT' || a.stage === 'BREAKOUT_CANDIDATE' ? 0 : 1) - (b.stage === 'CONFIRMED_BREAKOUT' || b.stage === 'BREAKOUT_CANDIDATE' ? 0 : 1)).slice(0, 5).map((cand) => (
                  <div key={cand.symbol} className="p-4 rounded-xl bg-slate-800/30 border border-slate-700/50 hover:border-blue-500/30 transition-colors group">
                    <div className="flex justify-between items-start mb-2">
                      <div>
                        <Link href={`/stocks/${cand.symbol}?tab=breakouts`} className="font-bold text-white group-hover:text-blue-400 transition-colors">{cand.symbol}</Link>
                        {(cand.live?.price ?? cand.current_price) !== undefined && <span className="text-xs text-slate-400 ml-2">{inr(cand.live?.price ?? cand.current_price)}</span>}
                        {cand.live && <span className={`block text-[11px] mt-0.5 ${cand.live.status === 'IN_ENTRY_ZONE' ? 'text-emerald-400' : cand.live.status === 'INVALIDATED' ? 'text-rose-400' : 'text-amber-300'}`}>{LIVE_TEXT[cand.live.status]}</span>}
                      </div>
                      <Badge variant={cand.stage === 'CONFIRMED_BREAKOUT' ? 'pass' : cand.stage === 'BREAKOUT_CANDIDATE' ? 'blue' : 'neutral'}>
                        {cand.stage.replaceAll('_', ' ')}
                      </Badge>
                    </div>
                    {cand.reasons?.[0] && <div className="text-[11px] text-slate-300 mb-2">{cand.reasons[0]}</div>}
                    <div className="flex justify-between items-center text-xs text-slate-400">
                      <span>Setup score: <strong className="text-white">{cand.structure_score != null ? `${cand.structure_score}/100` : '—'}</strong></span>
                      <span>RVOL: <strong className="text-white">{cand.rvol !== undefined ? `${cand.rvol.toFixed(2)}x` : DASH}</strong></span>
                      <span className={cand.shariah_status === 'PASS' ? 'text-emerald-400 font-medium' : cand.shariah_status === 'FAIL' ? 'text-rose-400 font-medium' : 'text-slate-400 font-medium'}>
                        {shariahLabel(cand.shariah_status).text}
                      </span>
                    </div>
                  </div>
                ))
              ) : (
                <div className="py-4 text-center text-slate-500">No active breakout setups identified.</div>
              )}
              <Link href="/breakouts" className="block text-center text-xs text-blue-400 hover:text-blue-300">All upcoming and confirmed breakouts, with reasons and results →</Link>
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
