'use client';

import { useResearchRun } from '@/lib/api/hooks';
import type { BcaInterval, EvidenceStats } from '@/lib/api/types';
import Badge from '@/components/ui/Badge';

const pct = (v: number | null | undefined, d = 2) => (v == null ? '—' : `${v >= 0 ? '+' : ''}${v.toFixed(d)}%`);
const range = (c?: BcaInterval | null) => (c ? `${pct(c.low)} to ${pct(c.high)}` : 'too few trades');

function PeriodCard({ title, sub, s }: { title: string; sub: string; s: EvidenceStats }) {
  const ret = s.avg_net_return_pct ?? 0;
  return (
    <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800 space-y-1.5 text-sm">
      <p className="text-white font-semibold">{title}</p>
      <p className="text-[11px] text-slate-500">{sub}</p>
      <p className={`text-2xl font-bold font-mono ${ret >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>{pct(s.avg_net_return_pct)}</p>
      <p className="text-[11px] text-slate-400">average per trade after costs · 95% range {range(s.net_return_ci)}</p>
      <dl className="grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs pt-1">
        <dt className="text-slate-500">Trades</dt><dd className="text-right font-mono text-slate-200">{s.trades}</dd>
        <dt className="text-slate-500">Won</dt><dd className="text-right font-mono text-slate-200">{s.win_rate_pct?.toFixed(1)}%</dd>
        <dt className="text-slate-500">Reached Target 1</dt><dd className="text-right font-mono text-slate-200">{s.target_hit_pct?.toFixed(1)}%</dd>
        <dt className="text-slate-500">Hit the stop</dt><dd className="text-right font-mono text-slate-200">{s.stopped_pct?.toFixed(1)}%</dd>
        <dt className="text-slate-500">Profit factor</dt><dd className="text-right font-mono text-slate-200">{s.profit_factor?.toFixed(2) ?? '—'}</dd>
        <dt className="text-slate-500">Versus NIFTY 50</dt><dd className="text-right font-mono text-slate-200">{pct(s.avg_excess_vs_nifty_pct)}</dd>
      </dl>
    </div>
  );
}

/** Real results on NSE's own price series. Clearly separate from the formal certification of Strategy V2. */
export default function ResearchResults() {
  const { run, isLoading, isError } = useResearchRun();
  if (isLoading) return <div className="glass-card p-6 text-sm text-slate-400">Loading results…</div>;
  if (isError) return <div className="glass-card p-6 text-sm text-amber-300">The research results could not be loaded.</div>;
  const r = run?.result;
  if (!run?.available || !r) return <div className="glass-card p-6 text-sm text-slate-400">No research run has been stored yet.</div>;
  const best = r.plans_shariah_pass.selected_on_discovery;

  return (
    <div className="glass-card p-6 space-y-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-lg font-semibold text-white">Breakout strategy, tested on NSE data</h2>
          <p className="text-sm text-slate-400 mt-1 max-w-3xl">
            {r.series.bars.toLocaleString('en-IN')} daily bars for {r.symbols_tested} stocks from {r.series.from} to {r.series.to}, taken from NSE&apos;s own files.
            Every confirmed breakout was bought at the next open, with the stop at the invalidation level, the target at Target 1 and a {r.rules_of_the_test.time_limit_sessions}-session limit, after delivery costs.
          </p>
        </div>
        <Badge variant="blue">Research grade</Badge>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <PeriodCard title="All stocks, all periods" sub={`${r.series.from} to ${r.series.to}`} s={r.overall} />
        <PeriodCard title="Recent period only" sub={`From ${r.rules_of_the_test.holdout_starts}`} s={r.by_period.HOLDOUT} />
        <PeriodCard title="Shariah-compliant stocks" sub="Today's screening status" s={r.shariah_pass_only.overall} />
      </div>

      <div className="rounded-xl bg-amber-500/10 border border-amber-500/30 p-4 text-sm text-amber-100 space-y-1">
        <p className="font-semibold">What this shows</p>
        <p>
          Buying confirmed breakouts has lost money after costs, and has trailed the NIFTY 50. Most trades reach Target 1, but the next open is usually already above the entry zone, so the
          target is closer and the stop further away than the plan assumed. The average loser is larger than the average winner.
        </p>
      </div>

      <div className="space-y-2">
        <h3 className="text-sm font-semibold text-white">Could a better plan fix it?</h3>
        <p className="text-xs text-slate-400">
          {r.plans.tested} combinations of exit, entry and market filter were tried. Each was chosen using data up to {r.rules_of_the_test.discovery_period_ends} only, then judged on
          later data it had never seen. With this many tries the winner on the earlier data is expected to look better than it really is, which is why only the unseen data counts.
        </p>
        {best && (
          <div className="rounded-xl bg-slate-900/60 border border-slate-800 p-3 text-xs space-y-1">
            <p className="text-slate-300">Best on the earlier data (Shariah-compliant stocks): <span className="text-white">{best.text}</span></p>
            <p className="font-mono text-slate-300">Earlier data: {pct(best.discovery.avg_net_return_pct)} per trade ({best.discovery.trades} trades, 95% range {range(best.discovery.net_return_ci)})</p>
            <p className="font-mono text-slate-300">Unseen data: {pct(best.holdout.avg_net_return_pct)} per trade ({best.holdout.trades} trades, 95% range {range(best.holdout.net_return_ci)})</p>
            <p className="text-amber-200 pt-1">
              {r.plans_shariah_pass.holdout_verdict === 'EDGE_CONFIRMED' ? 'This held up on unseen data.' : 'It did not hold up on unseen data, so no plan has been adopted. A rule that only worked in hindsight would have been a false promise.'}
            </p>
          </div>
        )}
      </div>

      <details className="text-xs text-slate-400">
        <summary className="cursor-pointer text-slate-300 font-semibold">Filters tested ({r.rules.length})</summary>
        <table className="w-full mt-2 text-left">
          <thead><tr className="text-slate-500 border-b border-slate-800"><th className="py-1.5">Filter</th><th className="text-right">Earlier data</th><th className="text-right">Unseen data</th><th className="pl-4">Decision</th></tr></thead>
          <tbody className="divide-y divide-slate-800/60">
            {r.rules.map((x) => (
              <tr key={x.rule}>
                <td className="py-1.5 text-slate-200">{x.description}</td>
                <td className="text-right font-mono">{pct(x.discovery.avg_net_return_pct)} ({x.discovery.trades})</td>
                <td className="text-right font-mono">{pct(x.holdout.avg_net_return_pct)} ({x.holdout.trades})</td>
                <td className="pl-4">{x.verdict === 'ADOPTED' ? 'Adopted' : x.why}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>

      <details className="text-xs text-slate-400">
        <summary className="cursor-pointer text-slate-300 font-semibold">How the prices were prepared ({r.series.adjustments_confirmed} of {r.series.adjustments_total} corporate actions confirmed)</summary>
        <table className="w-full mt-2 text-left">
          <thead><tr className="text-slate-500 border-b border-slate-800"><th className="py-1.5">Stock</th><th>Ex-date</th><th>Action</th><th className="text-right">NSE factor</th><th className="text-right">Actual price step</th><th className="pl-4">Check</th></tr></thead>
          <tbody className="divide-y divide-slate-800/60 font-mono">
            {r.adjustments.map((a) => (
              <tr key={`${a.symbol}-${a.ex_date}`}>
                <td className="py-1 font-sans text-slate-200">{a.symbol}</td><td>{a.ex_date}</td><td className="font-sans">{a.subjects.slice(0, 44)}</td>
                <td className="text-right">{a.factor.toFixed(4)}</td><td className="text-right">{a.observed_step?.toFixed(4) ?? '—'}</td><td className="pl-4 font-sans">{a.status === 'CONFIRMED' ? 'Confirmed' : a.status}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>

      <div className="text-[11px] text-slate-500 space-y-0.5">
        {r.limitations.map((l) => <p key={l}>{l}</p>)}
        {run.run_at && <p>Last run {new Date(run.run_at).toLocaleString('en-IN', { timeZone: 'Asia/Kolkata' })} IST. Refreshed every evening.</p>}
      </div>
    </div>
  );
}
