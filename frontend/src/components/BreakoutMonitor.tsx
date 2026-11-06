'use client';

import Link from 'next/link';
import { useBreakoutMonitor } from '@/lib/api/hooks';
import type { TrackedBreakout } from '@/lib/api/types';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';

const pct = (v: number | null | undefined, d = 2) => (v == null ? '—' : `${v >= 0 ? '+' : ''}${v.toFixed(d)}%`);
const inr = (v: number | null | undefined) => (v == null ? '—' : `₹${v.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`);

const VERDICT: Record<TrackedBreakout['verdict'], { text: string; variant: 'pass' | 'fail' | 'warning' | 'neutral' | 'blue' }> = {
  PENDING: { text: 'Running', variant: 'blue' },
  WORKED: { text: 'Worked: hit Target 1', variant: 'pass' },
  FAILED: { text: 'Failed: hit stop', variant: 'fail' },
  NO_FOLLOW_THROUGH: { text: 'No follow-through', variant: 'warning' },
  NOT_TRADEABLE: { text: 'Opened above target', variant: 'neutral' },
};

/** Did the confirmed breakouts actually happen? Every signal is recorded with its plan frozen, then followed. */
export default function BreakoutMonitor() {
  const { monitor: m, isLoading, isError } = useBreakoutMonitor();
  if (isLoading) return null;
  if (isError || !m) return <p className="text-sm text-amber-300">The breakout monitor could not be loaded.</p>;
  const h = m.history;
  const c = m.scorecard;
  const decided = c.worked + c.failed + c.no_follow_through;

  return (
    <Card>
      <CardHeader
        title="Breakout Monitor: did it actually happen?"
        subtitle="Every confirmed breakout is recorded once with its plan frozen, entered at the next session's open, then followed for 10 sessions"
      />
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 text-sm mb-4">
        <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
          <span className="text-xs text-slate-400 block">Being followed now</span>
          <span className="text-xl font-mono text-white">{c.open}</span>
          <span className="text-[11px] text-slate-500 block">of {c.tracked} recorded</span>
        </div>
        <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
          <span className="text-xs text-slate-400 block">Finished so far</span>
          <span className="text-xl font-mono text-white">{c.worked} worked · {c.failed} failed</span>
          <span className="text-[11px] text-slate-500 block">{c.no_follow_through} faded · {c.not_tradeable} opened above target</span>
        </div>
        <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800">
          <span className="text-xs text-slate-400 block">Average result of finished signals</span>
          <span className={`text-xl font-mono ${(c.avg_return_pct_closed ?? 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>{pct(c.avg_return_pct_closed)}</span>
          <span className="text-[11px] text-slate-500 block">{decided} signals, before costs{c.avg_excess_vs_nifty_pct_closed != null ? `; ${pct(c.avg_excess_vs_nifty_pct_closed)} vs NIFTY` : ''}</span>
        </div>
        {h && (
          <div className="p-3 rounded-xl bg-amber-500/10 border border-amber-500/30">
            <span className="text-xs text-amber-200 block">Five-year record on NSE data</span>
            <span className="text-xl font-mono text-amber-300">{pct(h.overall.avg_net_return_pct)}</span>
            <span className="text-[11px] text-amber-200/80 block">avg per trade after costs, {h.overall.trades} trades; {h.overall.target_hit_pct?.toFixed(0)}% hit Target 1</span>
          </div>
        )}
      </div>

      {h && (
        <p className="text-xs text-slate-300 mb-4 rounded-lg bg-slate-800/40 border border-slate-700/60 px-3 py-2">
          A confirmed breakout has not been a profitable signal on its own: most reach Target 1, but the losers are larger and costs take the rest. In the recent
          period (2025 onward) the average was {pct(h.by_period.HOLDOUT.avg_net_return_pct)} per trade, and {pct(h.by_period.HOLDOUT.avg_excess_vs_nifty_pct)} against NIFTY over the same days.
          Treat these as stocks to study, not buy signals. <Link href="/backtesting" className="text-blue-400 hover:text-blue-300">See the full study</Link>
        </p>
      )}

      <div className="overflow-x-auto max-h-[28rem]">
        <table className="w-full text-left text-xs">
          <thead className="sticky top-0 bg-slate-900">
            <tr className="text-slate-400 uppercase border-b border-slate-800">
              <th className="py-2">Stock</th><th>Signal</th><th className="text-right">Entry (next open)</th><th className="text-right">Stop</th><th className="text-right">Target 1</th>
              <th className="text-right">Now / exit</th><th className="text-right">Result</th><th className="text-right" title="Best and worst move since entry">Best / worst</th>
              <th className="text-right">vs NIFTY</th><th className="text-right">Sessions</th><th className="pl-3">Status</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-800/60 font-mono">
            {m.signals.map((s) => {
              const v = VERDICT[s.verdict] ?? VERDICT.PENDING;
              const excess = s.return_pct != null && s.nifty_return_pct != null ? s.return_pct - s.nifty_return_pct : null;
              return (
                <tr key={`${s.symbol}-${s.signal_date}`}>
                  <td className="py-2 font-sans font-semibold text-white">
                    <Link href={`/stocks/${s.symbol}`} className="hover:text-blue-400">{s.symbol}</Link>
                    {s.origin === 'HISTORY_REPLAY' && <span className="block text-[10px] font-normal text-slate-500" title="Found by replaying the engine on data up to that day, before the monitor existed">replayed</span>}
                  </td>
                  <td className="text-slate-400">{s.signal_date}</td>
                  <td className="text-right text-white">{inr(s.entry_price)}</td>
                  <td className="text-right text-rose-400">{inr(s.stop_level)}</td>
                  <td className="text-right text-emerald-400">{inr(s.target_1)}</td>
                  <td className="text-right text-slate-300">{inr(s.exit_price ?? s.last_price)}</td>
                  <td className={`text-right ${(s.return_pct ?? 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>{pct(s.return_pct)}</td>
                  <td className="text-right text-slate-400">{pct(s.best_pct, 1)} / {pct(s.worst_pct, 1)}</td>
                  <td className={`text-right ${(excess ?? 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>{pct(excess)}</td>
                  <td className="text-right text-slate-400">{s.sessions}</td>
                  <td className="font-sans pl-3">
                    <Badge variant={v.variant}>{v.text}</Badge>
                    {s.closed_below_at && s.verdict === 'PENDING' && <span className="block text-[10px] text-amber-300 mt-0.5">closed back below the breakout level</span>}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </Card>
  );
}
