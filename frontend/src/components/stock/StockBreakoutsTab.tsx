'use client';

import { useStockBreakouts } from '@/lib/api/hooks';
import BreakoutCard, { Reasons, inr, pct, tone } from '@/components/breakouts/BreakoutCard';

const VERDICT: Record<string, { text: string; cls: string }> = {
  INSUFFICIENT_DATA: { text: 'Not enough history to judge', cls: 'border-slate-500/40 text-slate-300' },
  POSITIVE_AFTER_COSTS: { text: 'Positive after costs (past result)', cls: 'border-emerald-500/40 text-emerald-300' },
  NEGATIVE_AFTER_COSTS: { text: 'Negative after costs (past result)', cls: 'border-rose-500/40 text-rose-300' },
  NO_EDGE_DEMONSTRATED: { text: 'No edge demonstrated', cls: 'border-amber-500/40 text-amber-300' },
};
const VERDICT_WORD: Record<string, string> = { WORKED: 'hit target 1', FAILED: 'stopped out', NO_FOLLOW_THROUGH: 'faded', NOT_TRADEABLE: 'opened past target', PENDING: 'open' };

export default function StockBreakoutsTab({ symbol }: { symbol: string }) {
  const { data, isLoading, isError } = useStockBreakouts(symbol);
  if (isLoading) return <div className="text-slate-500 text-sm py-8">Loading breakout record…</div>;
  if (isError || !data) return <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-sm">Failed to load the breakout record for {symbol}.</div>;
  const t = data.test?.result;
  const v = t ? VERDICT[t.verdict] : null;
  const cb = t?.confirmed_breakouts, up = t?.upcoming_flags;
  return (
    <div className="space-y-6">
      <section className="space-y-2">
        <h2 className="text-lg font-semibold text-white">Today</h2>
        {data.today?.available && data.today.reading ? (
          data.today.reading.stage === 'CONFIRMED_BREAKOUT' || data.today.reading.stage === 'BREAKOUT_CANDIDATE'
            ? <BreakoutCard r={data.today.reading} linkSymbol={false} />
            : <div className="glass-card p-4 text-sm text-slate-300">On the latest session ({data.today.data_as_of}) {symbol} is <b>{data.today.reading.stage.replaceAll('_', ' ').toLowerCase()}</b>: no breakout call. {data.today.reading.reasons?.[0] ?? ''}</div>
        ) : <div className="glass-card p-4 text-sm text-slate-400">Not scanned today: {data.today?.reason ?? 'unavailable'}.</div>}
      </section>

      <section className="space-y-2">
        <h2 className="text-lg font-semibold text-white">How this stock's breakout signals have done</h2>
        {!t ? <div className="glass-card p-4 text-sm text-slate-400">The nightly per-stock test has not run for {symbol} yet.</div> : (
          <div className="glass-card p-4 space-y-3">
            <div className="flex flex-wrap items-center gap-3"><span className={`text-xs px-2 py-1 rounded-full border ${v?.cls}`}>{v?.text}</span><span className="text-xs text-slate-500">history to {t.series_through} · tested {data.test?.as_of}</span></div>
            <div className="text-sm text-slate-300">{t.verdict_text}</div>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
              <div><div className="text-xs text-slate-500">Past breakout trades</div><div className="font-mono text-white">{cb?.trades ?? 0} ({cb?.wins ?? 0} winners)</div></div>
              <div><div className="text-xs text-slate-500">Average after costs</div><div className={`font-mono ${tone(cb?.mean_net_pct)}`}>{pct(cb?.mean_net_pct)}</div></div>
              <div><div className="text-xs text-slate-500">95% range of that average</div><div className="font-mono text-slate-200">{cb?.net_ci95 ? `${pct(cb.net_ci95.low)} to ${pct(cb.net_ci95.high)}` : 'needs 10+ trades'}</div></div>
              <div><div className="text-xs text-slate-500">Upcoming flags that broke out</div><div className="font-mono text-white">{up?.resolved_flags ? `${up.broke_out}/${up.resolved_flags}` : '—'}{up?.hit_rate_pct != null ? ` (${up.hit_rate_pct.toFixed(0)}%)` : ''}</div></div>
            </div>
            {cb && <div className="text-xs text-slate-400">Before 2025: {cb.by_period.DISCOVERY?.trades ?? 0} trades, {pct(cb.by_period.DISCOVERY?.mean_net_pct)} · 2025 onward: {cb.by_period.HOLDOUT?.trades ?? 0} trades, {pct(cb.by_period.HOLDOUT?.mean_net_pct)}</div>}
            {cb && cb.last_trades.length > 0 && <div className="text-xs text-slate-400">Latest: {cb.last_trades.map((x) => `${x.entry} → ${x.exit} ${x.why.replaceAll('_', ' ').toLowerCase()} ${pct(x.net_pct)}`).join(' · ')}</div>}
            <div className="text-[11px] text-slate-500">{t.plan}. {up?.note} Small samples are noise: a single stock rarely has enough breakouts to prove anything, and a past result is not a forecast.</div>
          </div>
        )}
      </section>

      <section className="space-y-2">
        <h2 className="text-lg font-semibold text-white">Every signal recorded for {symbol}</h2>
        {data.signals.length === 0 && data.flags.length === 0 ? <div className="glass-card p-4 text-sm text-slate-400">No breakout calls recorded for this stock yet.</div> : (
          <div className="space-y-3">
            {data.signals.map((s) => (
              <div key={`s${s.signal_date}`} className={`glass-card p-4 space-y-2 ${s.counts_in_stats ? '' : 'opacity-60'}`}>
                <div className="flex flex-wrap items-center gap-3 text-sm"><b className="text-white">Confirmed {s.signal_date}</b><span className="text-xs text-slate-500">{s.origin === 'LIVE' ? 'live' : 'replayed'}</span>
                  <span className="text-slate-200">{VERDICT_WORD[s.verdict] ?? s.verdict}</span><span className={`font-mono ${tone(s.return_pct)}`}>{pct(s.return_pct)} before costs</span><span className={`font-mono ${tone(s.net_return_pct)}`}>{pct(s.net_return_pct)} after</span></div>
                <Reasons reasons={s.reasons} />
                <div className="text-xs text-slate-400">Stop {inr(s.stop_level)} · Target 1 {inr(s.target_1)} · Entered {inr(s.entry_price)} · Exit {inr(s.exit_price)}{s.excluded_reason ? ` · ${s.excluded_reason}` : ''}</div>
              </div>))}
            {data.flags.map((f) => (
              <div key={`f${f.flagged_date}`} className={`glass-card p-4 space-y-2 ${f.counts_in_stats ? '' : 'opacity-60'}`}>
                <div className="flex flex-wrap items-center gap-3 text-sm"><b className="text-white">Upcoming flag {f.flagged_date}</b><span className="text-xs text-slate-500">{f.origin === 'LIVE' ? 'live' : 'replayed'}</span>
                  <span className="text-slate-200">{f.status === 'BROKE_OUT' ? `broke out after ${f.sessions_to_breakout} session(s)` : f.status.toLowerCase().replaceAll('_', ' ')}</span><span className={`font-mono ${tone(f.ret_10d_pct)}`}>{pct(f.ret_10d_pct)} over 10 sessions</span></div>
                <Reasons reasons={f.reasons} />
              </div>))}
          </div>
        )}
      </section>
    </div>
  );
}
