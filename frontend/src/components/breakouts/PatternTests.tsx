'use client';

import { useState } from 'react';
import Link from '@/components/Link';
import { usePatternReplay, useReplayTrades } from '@/lib/api/hooks';
import type { ReplaySetup, ReplayStats, ReplaySymbolStats, ReplayTrade } from '@/lib/api/types';

export const SETUP_LABEL: Record<ReplaySetup, string> = { CLOSEST: 'Closest to breaking out', UPCOMING: 'Upcoming breakout', CONFIRMED: 'Confirmed (already predicted)' };
const OUTCOME: Record<ReplayTrade['outcome'], { text: string; cls: string }> = {
  HIT_TARGET: { text: 'Hit target', cls: 'text-emerald-400' }, STOPPED: { text: 'Stopped out', cls: 'text-rose-400' }, TIME_EXIT: { text: 'Sold after 10 sessions', cls: 'text-slate-300' },
  NOT_TRIGGERED: { text: 'Never broke out (no trade)', cls: 'text-slate-500' }, FAILED_BEFORE_ENTRY: { text: 'Fell to stop first (no trade)', cls: 'text-slate-500' },
  SKIPPED: { text: 'Skipped (no trade)', cls: 'text-slate-500' }, OPEN: { text: 'Still held', cls: 'text-sky-300' }, PENDING: { text: 'Waiting', cls: 'text-sky-300' },
};
const VERDICT: Record<ReplayStats['verdict'], { text: string; cls: string }> = {
  INSUFFICIENT_DATA: { text: 'Too few trades to judge', cls: 'border-slate-500/40 text-slate-300 bg-slate-500/10' },
  POSITIVE_AFTER_COSTS: { text: 'Positive after costs', cls: 'border-emerald-500/40 text-emerald-300 bg-emerald-500/10' },
  NEGATIVE_AFTER_COSTS: { text: 'Negative after costs', cls: 'border-rose-500/40 text-rose-300 bg-rose-500/10' },
  NO_EDGE_DEMONSTRATED: { text: 'No edge shown', cls: 'border-amber-500/40 text-amber-300 bg-amber-500/10' },
};

const pct = (v: number | null | undefined, d = 2) => (v == null ? '—' : `${v >= 0 ? '+' : ''}${v.toFixed(d)}%`);
const tone = (v: number | null | undefined) => (v == null ? 'text-slate-400' : v > 0 ? 'text-emerald-400' : v < 0 ? 'text-rose-400' : 'text-slate-300');
const rupees = (v: number) => `${v >= 0 ? '+' : '−'}₹${Math.abs(Math.round(v)).toLocaleString('en-IN')}`;
const px = (v: number | null) => (v == null ? '—' : `₹${v.toFixed(2)}`);
const when = (d: string | null, t: string | null) => (d ? `${d}${t ? ` ${t}` : ''}` : '—');

export function TradesTable({ trades, showSymbol }: { trades: ReplayTrade[]; showSymbol: boolean }) {
  if (trades.length === 0) return <div className="text-sm text-slate-400">No past trades for this selection.</div>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-xs">
        <thead>
          <tr className="text-left text-slate-500 border-b border-white/10">
            {showSymbol && <th className="py-1.5 pr-3">Stock</th>}
            <th className="py-1.5 pr-3">Signal day</th><th className="pr-3">Bought (date, IST time)</th><th className="pr-3 text-right">Buy price</th>
            <th className="pr-3 text-right">Stop / Target</th><th className="pr-3">Sold (date, IST time)</th><th className="pr-3 text-right">Sell price</th>
            <th className="pr-3">Result</th><th className="pr-3 text-right">Days</th><th className="text-right">Net after costs</th>
          </tr>
        </thead>
        <tbody>
          {trades.map((t, i) => {
            const bought = t.entry_price != null;
            const open = t.outcome === 'OPEN';
            return (
              <tr key={`${t.symbol}-${t.setup}-${t.signal_date}-${i}`} className="border-b border-white/5" title={t.note ?? undefined}>
                {showSymbol && <td className="py-1.5 pr-3"><Link href={`/stocks/${encodeURIComponent(t.symbol)}`} className="text-sky-300 hover:underline">{t.symbol}</Link></td>}
                <td className="py-1.5 pr-3 font-mono text-slate-300">{t.signal_date}{t.distance_pct != null && t.setup !== 'CONFIRMED' && <span className="ml-1 text-[10px] text-slate-500">{t.distance_pct.toFixed(1)}% under</span>}</td>
                <td className="pr-3 font-mono text-slate-300">{bought ? when(t.entry_date, t.entry_time) : '—'}</td><td className="pr-3 text-right font-mono">{px(t.entry_price)}</td>
                <td className="pr-3 text-right font-mono text-slate-400">{bought && t.stop != null && t.target_1 != null ? `${t.stop.toFixed(2)} / ${t.target_1.toFixed(2)}` : '—'}</td>
                <td className="pr-3 font-mono text-slate-300">{open ? 'not sold yet' : bought ? when(t.exit_date, t.exit_time) : '—'}</td>
                <td className="pr-3 text-right font-mono">{open && t.exit_price != null ? <span className="text-slate-400" title="Latest close, not a sale">{px(t.exit_price)}*</span> : px(t.exit_price)}</td>
                <td className={`pr-3 ${OUTCOME[t.outcome].cls}`}>{OUTCOME[t.outcome].text}</td><td className="pr-3 text-right font-mono">{t.bars_held ?? '—'}</td>
                <td className={`text-right font-mono font-semibold ${tone(t.net_pct)}`}>{pct(t.net_pct)}{open && t.net_pct != null ? '*' : ''}</td>
              </tr>);
          })}
        </tbody>
      </table>
      {trades.some((t) => t.outcome === 'OPEN') && <p className="mt-1 text-[10px] text-slate-500">* still held: marked at the latest close, not sold, and left out of the statistics.</p>}
    </div>
  );
}

function StatGrid({ s, setup }: { s: ReplayStats; setup: ReplaySetup }) {
  const items: [string, string, string?][] = [
    ['Times this pattern showed up', String(s.signals)],
    [setup === 'CONFIRMED' ? 'Trades taken' : 'Price actually broke through resistance', s.triggered_pct == null ? '—' : `${s.triggered} (${s.triggered_pct}%)`],
    ['Hit target / stopped / timed out', `${s.hit_target} / ${s.stopped} / ${s.time_exit}`],
    ['Trades that made money (net)', s.win_rate_pct == null ? '—' : `${s.win_rate_pct}%`, tone(s.win_rate_pct == null ? null : s.win_rate_pct - 50)],
    ['Average per trade, after costs', pct(s.mean_net_pct), tone(s.mean_net_pct)],
    ['Total on ₹1 lakh per trade', rupees(s.total_net_inr_on_1L_each), tone(s.total_net_inr_on_1L_each)],
    ['Average vs holding the Nifty', pct(s.mean_excess_vs_nifty_pct), tone(s.mean_excess_vs_nifty_pct)],
    ['Average held', s.mean_days_held == null ? '—' : `${s.mean_days_held} sessions`],
    ['No trade: never reached / cancelled / skipped', `${s.not_triggered} / ${s.failed_before_entry} / ${s.skipped}`],
  ];
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-2 text-xs">
      {items.map(([k, v, c]) => (<div key={k} className="rounded-lg bg-slate-900/60 p-2"><div className="text-slate-500">{k}</div><div className={`font-mono text-sm ${c ?? 'text-slate-100'}`}>{v}</div></div>))}
    </div>
  );
}

/** Paper-test of every stage on the stocks' own past: the three result cards, the detail of the chosen one, and every simulated trade. */
export default function PatternTests({ today }: { today?: Record<ReplaySetup, string[]> }) {
  const { data, isLoading, error } = usePatternReplay();
  const [setup, setSetup] = useState<ReplaySetup>('CLOSEST');
  const [stock, setStock] = useState('');
  const [view, setView] = useState<'all' | 'recent'>('all');

  if (isLoading) return <div className="glass-card p-4 text-sm text-slate-400">Loading the paper-test results…</div>;
  if (error || !data) return <div className="glass-card p-4 text-sm text-rose-300">Could not load the paper-test results.</div>;
  if (!data.available || !data.setups || !data.rules) return <div className="glass-card p-4 text-sm text-slate-300">{data.reason ?? 'The paper test is not ready yet.'}</div>;

  const cur = data.setups[setup];
  const st = view === 'recent' ? cur.recent : cur.all;
  const nowList = today?.[setup] ?? [];
  return (
    <section className="space-y-3">
      <div>
        <h2 className="text-lg font-semibold text-white">Paper-test on past data</h2>
        <p className="text-xs text-slate-400 mt-1">
          Every day in the stored history the engine&apos;s reading is replayed using only the data up to that day. Each time a stock was in one of these states, a simulated trade is opened and followed on the bars that came after:
          bought only if the price goes through resistance (within {data.rules.trigger_sessions} sessions), exit at the engine&apos;s stop or Target 1, otherwise after {data.rules.hold_sessions} sessions. Costs, charges and slippage are included; ₹{data.rules.notional_per_trade_inr.toLocaleString('en-IN')} per trade. Data through {data.as_of}.
        </p>
      </div>

      <div className="grid md:grid-cols-3 gap-3">
        {(['CLOSEST', 'UPCOMING', 'CONFIRMED'] as ReplaySetup[]).map((k) => {
          const a = data.setups![k].all;
          const v = VERDICT[a.verdict];
          return (
            <button key={k} onClick={() => { setSetup(k); setStock(''); }} aria-pressed={setup === k}
              className={`text-left glass-card p-3 space-y-2 border ${setup === k ? 'border-sky-400/60' : 'border-transparent hover:border-white/20'}`}>
              <div className="flex items-center justify-between gap-2"><span className="font-semibold text-white text-sm">{SETUP_LABEL[k]}</span><span className={`text-[10px] font-bold px-2 py-0.5 rounded-full border ${v.cls}`}>{v.text}</span></div>
              <div className="grid grid-cols-3 gap-2 text-xs">
                <div><div className="text-slate-500">Times seen</div><div className="font-mono text-slate-100">{a.signals}</div></div>
                <div><div className="text-slate-500">Broke out</div><div className="font-mono text-slate-100">{a.triggered_pct == null ? '—' : `${a.triggered_pct}%`}</div></div>
                <div><div className="text-slate-500">Avg net/trade</div><div className={`font-mono ${tone(a.mean_net_pct)}`}>{pct(a.mean_net_pct)}</div></div>
              </div>
            </button>);
        })}
      </div>

      <div className="glass-card p-4 space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-semibold text-white">{SETUP_LABEL[setup]}</h3>
          <span className={`text-[10px] font-bold px-2 py-0.5 rounded-full border ${VERDICT[st.verdict].cls}`}>{VERDICT[st.verdict].text}</span>
          <div className="ml-auto flex gap-1 text-xs" role="tablist" aria-label="Period">
            {([['all', 'All history'], ['recent', `Since ${data.recent_from}`]] as const).map(([id, label]) => (
              <button key={id} role="tab" aria-selected={view === id} onClick={() => setView(id)} className={`px-3 py-1 rounded-lg border ${view === id ? 'bg-sky-500/20 border-sky-400/60 text-sky-200' : 'border-white/10 text-slate-400 hover:text-white'}`}>{label}</button>))}
          </div>
        </div>
        <p className="text-xs text-slate-400">{data.rules.text[setup]}</p>
        <StatGrid s={st} setup={setup} />
        {st.net_ci95 && <p className="text-[11px] text-slate-500">95% range of the average net result: {pct(st.net_ci95.low)} to {pct(st.net_ci95.high)}. {cur.verdict_text}</p>}
        {!st.net_ci95 && <p className="text-[11px] text-slate-500">{cur.verdict_text}</p>}
        {nowList.length > 0 && <p className="text-xs text-slate-300">In this state today: {nowList.map((s, i) => (<span key={s}>{i > 0 && ', '}<button className="text-sky-300 hover:underline" onClick={() => setStock(s)}>{s}</button></span>))} (tap a name to open its paper trades below).</p>}
        {cur.engine_levels_unchanged && (
          <p className="text-[11px] text-slate-500">For comparison, with the engine&apos;s stop and target exactly as published (anchored at resistance even when the stock is bought higher): {cur.engine_levels_unchanged.triggered} trades, average {pct(cur.engine_levels_unchanged.mean_net_pct)} after costs, {cur.engine_levels_unchanged.skipped} skipped because the price had already passed the target. The numbers above place the stop and target around the price actually paid.</p>)}

        <StockTable setup={setup} bySymbol={cur.by_symbol} today={nowList} open={stock} onOpen={setStock} />
        <p className="text-[11px] text-slate-500 leading-relaxed">
          Buy and sell days come from daily exchange bars; the time of day comes from 1-minute bars, which exist only since 6 Aug 2026, so older trades show no time. If one day reaches both the stop and the target, the stop is assumed first. A trade still running is left out of the numbers.
          A good past result is not a forecast: the engine is judged by whether the net result after costs is clearly above zero, and the stocks and period here are limited.
        </p>
      </div>
    </section>
  );
}

/** One stock's own past trades for a setup, shown inside its card. */
export function StockHistory({ symbol, setup }: { symbol: string; setup: ReplaySetup }) {
  const { data } = usePatternReplay();
  const [open, setOpen] = useState(false);
  const trades = useReplayTrades(open ? setup : null, symbol, 100);
  const s = data?.setups?.[setup]?.by_symbol?.[symbol];
  return (
    <div className="rounded-lg border border-white/10 bg-slate-900/40 p-3 space-y-2">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
        <span className="uppercase tracking-wide text-slate-500">Same pattern in {symbol}&apos;s past</span>
        {!data?.available ? <span className="text-slate-500">paper test not ready yet</span>
          : !s || s.signals === 0 ? <span className="text-slate-400">never showed this exact state before</span>
          : <span className="text-slate-300">seen <b className="text-white">{s.signals}</b>× · broke out <b className="text-white">{s.triggered}</b> ({s.triggered_pct ?? 0}%) · hit target {s.hit_target}, stopped {s.stopped}, timed out {s.time_exit}
            {s.mean_net_pct != null && <> · avg <b className={tone(s.mean_net_pct)}>{pct(s.mean_net_pct)}</b> per trade after costs ({rupees(s.total_net_inr_on_1L_each)} on ₹1L each)</>}</span>}
        {s && s.signals > 0 && <button onClick={() => setOpen((o) => !o)} className="ml-auto text-sky-300 hover:underline">{open ? 'Hide trades' : 'Show every paper trade'}</button>}
      </div>
      {open && (trades.isLoading ? <div className="text-xs text-slate-400">Loading…</div> : <TradesTable trades={trades.data?.trades ?? []} showSymbol={false} />)}
    </div>
  );
}

/** One row per stock; opening a row shows that stock's own paper trades in place. */
function StockTable({ setup, bySymbol, today, open, onOpen }: { setup: ReplaySetup; bySymbol: Record<string, ReplaySymbolStats>; today: string[]; open: string; onOpen: (s: string) => void }) {
  const rows = Object.entries(bySymbol).sort(([a, x], [b, y]) => (today.includes(b) ? 1 : 0) - (today.includes(a) ? 1 : 0) || y.signals - x.signals || a.localeCompare(b));
  const trades = useReplayTrades(open ? setup : null, open || undefined, 200);
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-xs">
        <thead>
          <tr className="text-left text-slate-500 border-b border-white/10">
            <th className="py-1.5 pr-3">Stock (tap to open its paper trades)</th><th className="pr-3 text-right">Times seen</th><th className="pr-3 text-right">Broke out</th>
            <th className="pr-3 text-right">Hit / stopped / timed out</th><th className="pr-3 text-right">Made money</th><th className="pr-3 text-right">Avg per trade</th><th className="text-right">Total on ₹1L each</th>
          </tr>
        </thead>
        <tbody>
          {rows.map(([sym, r]) => {
            const isOpen = open === sym;
            return [
              <tr key={sym} onClick={() => onOpen(isOpen ? '' : sym)} className={`border-b border-white/5 cursor-pointer hover:bg-white/5 ${isOpen ? 'bg-sky-500/10' : ''}`} aria-expanded={isOpen}>
                <td className="py-2 pr-3"><span className="text-slate-500 mr-1">{isOpen ? '▾' : '▸'}</span><span className="font-semibold text-slate-100">{sym}</span>
                  {today.includes(sym) && <span className="ml-2 text-[10px] px-1.5 py-0.5 rounded-full border border-amber-500/40 text-amber-300 bg-amber-500/10">in this state today</span>}</td>
                <td className="pr-3 text-right font-mono">{r.signals}{r.pending > 0 ? <span className="text-sky-300"> +{r.pending} waiting</span> : ''}</td>
                <td className="pr-3 text-right font-mono">{r.triggered_pct == null ? '—' : `${r.triggered} (${r.triggered_pct}%)`}</td>
                <td className="pr-3 text-right font-mono">{r.hit_target} / {r.stopped} / {r.time_exit}</td>
                <td className="pr-3 text-right font-mono">{r.win_rate_pct == null ? '—' : `${r.win_rate_pct}%`}</td>
                <td className={`pr-3 text-right font-mono ${tone(r.mean_net_pct)}`}>{pct(r.mean_net_pct)}</td>
                <td className={`text-right font-mono ${tone(r.total_net_inr_on_1L_each)}`}>{r.triggered > 0 ? rupees(r.total_net_inr_on_1L_each) : '—'}</td>
              </tr>,
              isOpen && (
                <tr key={`${sym}-trades`} className="bg-slate-950/40">
                  <td colSpan={7} className="p-3">
                    <div className="mb-2 flex items-center gap-3 text-xs">
                      <span className="font-semibold text-white">{sym}: every paper trade, {SETUP_LABEL[setup].toLowerCase()}</span>
                      <Link href={`/stocks/${encodeURIComponent(sym)}?tab=breakouts`} className="text-sky-300 hover:underline">Open {sym} page →</Link>
                    </div>
                    {trades.isLoading ? <div className="text-xs text-slate-400">Loading…</div> : <TradesTable trades={trades.data?.trades ?? []} showSymbol={false} />}
                  </td>
                </tr>),
            ];
          })}
        </tbody>
      </table>
      {rows.length === 0 && <div className="text-sm text-slate-400">No stock has shown this state yet.</div>}
    </div>
  );
}
