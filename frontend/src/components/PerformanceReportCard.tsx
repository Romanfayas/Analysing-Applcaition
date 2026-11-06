'use client';

import useSWR from 'swr';
import { fetcher } from '@/lib/api/client';
import type { BcaInterval, PerformanceReport } from '@/lib/api/types';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { money } from '@/lib/paperTime';

const pct = (v: number, d = 2) => `${v >= 0 ? '+' : ''}${v.toFixed(d)}%`;
const ci = (i?: BcaInterval | null, fmt: (v: number) => string = pct) =>
  i ? `${fmt(i.estimate)}  (95% range ${fmt(i.low)} to ${fmt(i.high)})` : 'not enough trades';

function Row({ label, value, hint }: { label: string; value: React.ReactNode; hint?: string }) {
  return (
    <div className="flex justify-between gap-4 py-1.5 border-b border-slate-800/60 last:border-0 text-sm">
      <span className="text-slate-400" title={hint}>{label}</span>
      <span className="text-white font-mono text-right text-xs sm:text-sm">{value}</span>
    </div>
  );
}

/** Honest statistics of the user's own closed paper trades: every headline number comes with its uncertainty. */
export default function PerformanceReportCard() {
  const { data: r, error, isLoading } = useSWR<PerformanceReport>('/paper-trading/performance', fetcher, { revalidateOnFocus: false });
  const variant = r?.reliability === 'ADEQUATE' ? 'pass' : r?.reliability === 'LOW' ? 'warning' : 'neutral';

  return (
    <Card>
      <CardHeader
        title="Your Paper-Trading Performance"
        subtitle="Closed trades only, net of all charges. Every figure is shown with how uncertain it is."
        action={r ? <Badge variant={variant}>{r.reliability === 'INSUFFICIENT' ? 'Too few trades' : r.reliability === 'LOW' ? 'Low sample' : 'Adequate sample'}</Badge> : undefined}
      />
      <div className="p-4 space-y-3">
        {isLoading && <p className="text-sm text-slate-400">Computing…</p>}
        {error && <p className="text-sm text-amber-300">The report could not be computed right now.</p>}
        {r && r.n_trades === 0 && <p className="text-sm text-slate-400">{r.message}</p>}
        {r && r.n_trades > 0 && (
          <>
            {r.verdict && <p className="text-sm text-slate-200 rounded-xl bg-slate-900/60 border border-slate-800 p-3">{r.verdict}</p>}
            <div>
              <Row label="Closed trades" value={r.n_trades} />
              <Row label="Net P&L" value={`${(r.total_net_pnl ?? 0) >= 0 ? '+' : ''}${money(r.total_net_pnl ?? 0)}`} />
              <Row label="Win rate" value={`${(r.win_rate_pct ?? 0).toFixed(1)}%`} />
              <Row label="Average net return per trade" value={ci(r.expectancy_pct)} hint="Bias-corrected bootstrap interval. If it spans zero, the record shows neither skill nor its absence." />
              <Row label="Profit factor" value={r.profit_factor != null ? `${r.profit_factor.toFixed(2)}${r.profit_factor_ci ? `  (${r.profit_factor_ci.low.toFixed(2)} to ${r.profit_factor_ci.high.toFixed(2)})` : ''}` : (r.profit_factor_note ?? '—')} hint="Gross profit divided by gross loss. Above 1 is profitable." />
              <Row label="Typical worst drawdown" value={r.max_drawdown ? `${r.max_drawdown.actual.toFixed(1)}% actual · up to ${r.max_drawdown.p95.toFixed(1)}% with unlucky ordering` : 'not enough trades'} hint="The same trades in random order show how bad ordering luck alone can make it." />
              <Row label="Charges as a share of gross profit" value={r.cost_drag_pct_of_gross_profit != null ? `${r.cost_drag_pct_of_gross_profit.toFixed(1)}%` : '—'} hint="In Indian delivery trading the round-trip cost is a large fixed hurdle; frequent small trades are the usual way to lose to it." />
              {r.avg_holding_days != null && <Row label="Average holding period" value={`${r.avg_holding_days.toFixed(1)} days`} />}
              <Row label="Versus NIFTY 50 over the same holding periods" value={r.excess_vs_benchmark_pct ? `${ci(r.excess_vs_benchmark_pct)} · beat it in ${(r.benchmark_beaten_pct_of_trades ?? 0).toFixed(0)}% of trades` : 'index history not available for every trade'} />
            </div>
            <p className="text-[11px] text-slate-500">
              {r.assumptions.trading_days_per_year} trading days a year (NSE). Risk-free rate {r.assumptions.risk_free_rate_pct}% is an assumption, not a fetched value. Past results, simulated or paper, do not predict future returns.
            </p>
          </>
        )}
      </div>
    </Card>
  );
}
