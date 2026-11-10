'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useBreakoutsOverview, useAutoPaper } from '@/lib/api/hooks';
import BreakoutCard, { Reasons, ScoreBars, inr, pct, tone } from '@/components/breakouts/BreakoutCard';
import type { AutoPaperPortfolio, ScoreGroup, TrackedSignal, WatchFlag } from '@/lib/api/types';

type Tab = 'today' | 'upcoming' | 'confirmed' | 'auto';
const TABS: { id: Tab; label: string }[] = [
  { id: 'today', label: 'Today' }, { id: 'upcoming', label: 'Upcoming: tracked' }, { id: 'confirmed', label: 'Confirmed: tracked' }, { id: 'auto', label: 'Auto paper trading' },
];

const WATCH_LABEL: Record<WatchFlag['status'], { text: string; cls: string }> = {
  WATCHING: { text: 'Watching', cls: 'border-sky-500/40 text-sky-300' }, BROKE_OUT: { text: 'Broke out', cls: 'border-emerald-500/40 text-emerald-300' },
  INVALIDATED: { text: 'Invalidated', cls: 'border-rose-500/40 text-rose-300' }, EXPIRED: { text: 'Expired (10 sessions)', cls: 'border-slate-500/40 text-slate-300' },
};
const VERDICT_LABEL: Record<string, string> = { PENDING: 'Open', WORKED: 'Hit target 1', FAILED: 'Stopped out', NO_FOLLOW_THROUGH: 'Faded', NOT_TRADEABLE: 'Opened past target' };

function Group({ title, g }: { title: string; g: ScoreGroup }) {
  return (
    <div className="glass-card p-4">
      <div className="text-xs uppercase tracking-wide text-slate-400">{title}</div>
      {g.closed === 0 ? <div className="text-slate-500 text-sm mt-2">No closed trades yet.</div> : (
        <div className="mt-2 grid grid-cols-2 gap-2 text-sm">
          <div><div className="text-slate-500 text-xs">Closed trades</div><div className="text-white font-mono">{g.closed}</div></div>
          <div><div className="text-slate-500 text-xs">Net win rate</div><div className="text-white font-mono">{g.net_win_rate_pct == null ? '—' : `${g.net_win_rate_pct.toFixed(0)}%`}</div></div>
          <div><div className="text-slate-500 text-xs">Avg before costs</div><div className={`font-mono ${tone(g.avg_gross_pct)}`}>{pct(g.avg_gross_pct)}</div></div>
          <div><div className="text-slate-500 text-xs">Avg after costs</div><div className={`font-mono font-semibold ${tone(g.avg_net_pct)}`}>{pct(g.avg_net_pct)}</div></div>
        </div>
      )}
    </div>
  );
}

function FlagRow({ f }: { f: WatchFlag }) {
  const [open, setOpen] = useState(false);
  const st = WATCH_LABEL[f.status];
  return (
    <>
      <tr className={`border-b border-white/5 hover:bg-white/5 cursor-pointer ${f.counts_in_stats ? '' : 'opacity-50'}`} onClick={() => setOpen(!open)}>
        <td className="px-3 py-2"><Link href={`/stocks/${f.symbol}?tab=breakouts`} onClick={(e) => e.stopPropagation()} className="text-white font-medium hover:text-sky-300">{f.symbol}</Link></td>
        <td className="px-3 py-2 text-xs">{f.flagged_date}<span className="ml-2 text-[10px] text-slate-500">{f.origin === 'LIVE' ? 'live' : 'replayed'}</span></td>
        <td className="px-3 py-2"><span className={`text-[11px] px-2 py-0.5 rounded-full border ${st.cls}`}>{st.text}</span>{f.sessions_to_breakout != null && <span className="ml-2 text-xs text-slate-400">in {f.sessions_to_breakout} session(s)</span>}</td>
        <td className={`px-3 py-2 font-mono text-xs ${tone(f.ret_5d_pct)}`}>{pct(f.ret_5d_pct)}</td>
        <td className={`px-3 py-2 font-mono text-xs ${tone(f.ret_10d_pct)}`}>{pct(f.ret_10d_pct)}</td>
        <td className="px-3 py-2 font-mono text-xs text-slate-300">{f.status === 'WATCHING' ? `${f.sessions_elapsed}/10` : '—'}</td>
        <td className="px-3 py-2 text-xs text-slate-500">{f.counts_in_stats ? '' : 'repeat flag, not counted'}</td>
      </tr>
      {open && (
        <tr className="border-b border-white/5 bg-slate-900/40"><td colSpan={7} className="px-4 py-3 space-y-2">
          <Reasons reasons={f.reasons} />
          <div className="text-xs text-slate-400">Resistance {inr(f.resistance_level)} · Stop {inr(f.stop_level)} · Target 1 {inr(f.target_1)} · Close that day {inr(f.flagged_close)} · RVOL {f.rvol ?? '—'} · Score {f.structure_score ?? '—'} · Best move {pct(f.max_gain_pct)} · Worst {pct(f.max_drawdown_pct)}</div>
          <ScoreBars breakdown={f.score_breakdown} />
        </td></tr>
      )}
    </>
  );
}

function SignalRow({ s }: { s: TrackedSignal }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <tr className={`border-b border-white/5 hover:bg-white/5 cursor-pointer ${s.counts_in_stats ? '' : 'opacity-50'}`} onClick={() => setOpen(!open)}>
        <td className="px-3 py-2"><Link href={`/stocks/${s.symbol}?tab=breakouts`} onClick={(e) => e.stopPropagation()} className="text-white font-medium hover:text-sky-300">{s.symbol}</Link></td>
        <td className="px-3 py-2 text-xs">{s.signal_date}<span className="ml-2 text-[10px] text-slate-500">{s.origin === 'LIVE' ? 'live' : 'replayed'}</span></td>
        <td className="px-3 py-2 text-xs">{VERDICT_LABEL[s.verdict] ?? s.verdict}</td>
        <td className={`px-3 py-2 font-mono text-xs ${tone(s.return_pct)}`}>{pct(s.return_pct)}</td>
        <td className={`px-3 py-2 font-mono text-xs ${tone(s.net_return_pct)}`}>{pct(s.net_return_pct)}</td>
        <td className="px-3 py-2 font-mono text-xs text-slate-300">{s.sessions}</td>
        <td className="px-3 py-2 text-xs text-slate-500">{s.counts_in_stats ? '' : 'repeat signal, not counted'}</td>
      </tr>
      {open && (
        <tr className="border-b border-white/5 bg-slate-900/40"><td colSpan={7} className="px-4 py-3 space-y-2">
          <Reasons reasons={s.reasons} />
          <div className="text-xs text-slate-400">Entry zone {inr(s.entry_low)}–{inr(s.entry_high)} · Stop {inr(s.stop_level)} · Target 1 {inr(s.target_1)} · Target 2 {inr(s.target_2)} · Entered {inr(s.entry_price)} · Exit {inr(s.exit_price)} · Best {pct(s.best_pct)} · Worst {pct(s.worst_pct)} · vs NIFTY {pct(s.nifty_return_pct)}</div>
          {s.excluded_reason && <div className="text-xs text-amber-300">{s.excluded_reason}</div>}
          <ScoreBars breakdown={s.score_breakdown} />
        </td></tr>
      )}
    </>
  );
}

function AutoCard({ p }: { p: AutoPaperPortfolio }) {
  const [more, setMore] = useState(false);
  return (
    <div className="glass-card p-4 space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <h3 className="text-white font-semibold">{p.label}</h3>
        <span className="text-xs text-slate-400">plan: {p.plan.plan} · {p.plan.entry} · {p.plan.market}</span>
        <span className={`ml-auto text-[11px] px-2 py-0.5 rounded-full border ${p.cash_reconciles_to_ledger ? 'border-emerald-500/30 text-emerald-300' : 'border-rose-500/30 text-rose-300'}`}>{p.cash_reconciles_to_ledger ? 'cash reconciles to the ledger' : 'DOES NOT RECONCILE'}</span>
      </div>
      <div className="grid grid-cols-2 md:grid-cols-5 gap-3 text-sm">
        <div><div className="text-slate-500 text-xs">Closed trades</div><div className="text-white font-mono">{p.closed_count} ({p.wins} wins)</div></div>
        <div><div className="text-slate-500 text-xs">Net P&amp;L (closed)</div><div className={`font-mono font-semibold ${tone(p.net_pnl_closed)}`}>{inr(p.net_pnl_closed, 0)}</div></div>
        <div><div className="text-slate-500 text-xs">Return on {inr(p.initial_capital, 0)}</div><div className={`font-mono ${tone(p.return_on_capital_pct)}`}>{pct(p.return_on_capital_pct)}</div></div>
        <div><div className="text-slate-500 text-xs">Charges paid</div><div className="font-mono text-amber-300">{inr(p.total_fees, 0)}</div></div>
        <div><div className="text-slate-500 text-xs">Worst fall (closed trades)</div><div className="font-mono text-slate-200">{pct(p.max_drawdown_realized_pct)}</div></div>
      </div>
      <div className="text-xs text-slate-500">Processed through {p.processed_through || '—'}. Cash {inr(p.cash, 2)}; the ledger says {inr(p.expected_cash_from_trades, 2)}. {p.note}</div>
      {p.open_positions.length > 0 && (
        <div><div className="text-xs uppercase tracking-wide text-slate-500 mb-1">Open positions</div>
          <div className="flex flex-wrap gap-2">{p.open_positions.map((o) => <span key={o.symbol} className="text-xs rounded-lg bg-slate-900/60 px-2 py-1 text-slate-200">{o.symbol} · {o.shares} @ {inr(o.avg_cost)} since {o.entry_date}</span>)}</div></div>
      )}
      <button onClick={() => setMore(!more)} className="text-xs text-sky-300 hover:underline">{more ? 'Hide' : 'Show'} closed trades ({p.closed_trades.length}) and skipped signals ({p.skipped.length})</button>
      {more && (
        <div className="space-y-4">
          <div className="overflow-x-auto"><table className="w-full text-left text-xs text-slate-400">
            <thead className="uppercase bg-white/5 text-slate-300"><tr>{['Stock', 'Entered', 'Exited', 'Shares', 'Buy', 'Sell', 'Charges', 'Net P&L', 'Net %', 'Why'].map((h) => <th key={h} className="px-2 py-2">{h}</th>)}</tr></thead>
            <tbody>{p.closed_trades.map((c, i) => (
              <tr key={i} className="border-b border-white/5"><td className="px-2 py-1 text-white">{c.symbol}<span className="ml-1 text-[10px] text-slate-500">{c.origin === 'LIVE' ? 'live' : 'replayed'}</span></td><td className="px-2 py-1">{c.entry_date}</td><td className="px-2 py-1">{c.exit_date}</td><td className="px-2 py-1 font-mono">{c.shares}</td>
                <td className="px-2 py-1 font-mono">{c.entry_price.toFixed(2)}</td><td className="px-2 py-1 font-mono">{c.exit_price.toFixed(2)}</td><td className="px-2 py-1 font-mono">{c.fees.toFixed(2)}</td>
                <td className={`px-2 py-1 font-mono ${tone(c.net_pnl)}`}>{c.net_pnl.toFixed(2)}</td><td className={`px-2 py-1 font-mono ${tone(c.net_pct)}`}>{pct(c.net_pct)}</td><td className="px-2 py-1">{c.exit_reason.replaceAll('_', ' ').toLowerCase()}</td></tr>))}</tbody></table></div>
          {p.skipped.length > 0 && <div className="text-xs text-slate-400 space-y-0.5">{p.skipped.map((k, i) => <div key={i}>{k.symbol} ({k.signal_date}): not traded, {k.reason.replaceAll('_', ' ').toLowerCase()}</div>)}</div>}
        </div>
      )}
    </div>
  );
}

export default function BreakoutsPage() {
  const { overview, isLoading, isError } = useBreakoutsOverview();
  const { portfolios } = useAutoPaper();
  const [tab, setTab] = useState<Tab>('today');
  const [showReplay, setShowReplay] = useState(true);
  const today = overview?.today;
  const lift = overview?.candidate_lift?.result;
  const flags = (overview?.upcoming.flags ?? []).filter((f) => showReplay || f.origin === 'LIVE');
  const signals = (overview?.confirmed.signals ?? []).filter((s) => showReplay || s.origin === 'LIVE');
  const sc = overview?.confirmed.scorecard;
  const wLive = overview?.upcoming.scorecard_live, wAll = overview?.upcoming.scorecard_all;

  return (
    <div className="space-y-6 animate-in fade-in duration-500 pb-12">
      <div>
        <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Breakouts</h1>
        <p className="text-sm text-slate-400 mt-1 max-w-3xl">
          Stocks the engine sees about to break out (<b>upcoming</b>) and stocks that already have (<b>confirmed</b>), each with the reasons, and what actually happened afterwards. Every call is frozen on the day it is made and followed to its result, so the record cannot be flattered later.
        </p>
      </div>

      {isError && <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-sm">Failed to load breakouts.</div>}
      {isLoading && <div className="text-slate-500 text-sm">Loading…</div>}

      {overview && (
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          <div className="glass-card p-4 space-y-2">
            <div className="text-xs uppercase tracking-wide text-slate-400">Do upcoming flags precede breakouts?</div>
            {lift && lift.candidate_hit_next5_pct != null ? (
              <>
                <div className="text-sm text-slate-200">A flagged stock was followed by the engine's own confirmed breakout within 5 sessions <b className="text-white">{lift.candidate_hit_next5_pct.toFixed(0)}%</b> of the time, against <b className="text-white">{lift.base_rate_next5_pct?.toFixed(0)}%</b> for any stock-day ({lift.candidate_flags} flags, data to {lift.data_through}).</div>
                <div className="text-sm text-slate-200">But the average price move over the next 10 sessions was <b className={tone(lift.candidate_mean_10d_move_pct)}>{pct(lift.candidate_mean_10d_move_pct)}</b>: foreseeing a breakout is not the same as making money.</div>
              </>
            ) : <div className="text-sm text-slate-500">The nightly study has not produced this yet.</div>}
          </div>
          {sc && <Group title="Confirmed signals, after costs: live" g={sc.live} />}
          {sc && <Group title="Confirmed signals, after costs: replayed history" g={sc.history_replay} />}
        </div>
      )}

      <div className="flex flex-wrap gap-2 border-b border-white/10 pb-2">
        {TABS.map((t) => <button key={t.id} onClick={() => setTab(t.id)} className={`px-4 py-2 rounded-lg text-sm ${tab === t.id ? 'bg-sky-500/20 text-sky-200 border border-sky-500/40' : 'text-slate-400 hover:text-white'}`}>{t.label}</button>)}
        {(tab === 'upcoming' || tab === 'confirmed') && <label className="ml-auto text-xs text-slate-400 flex items-center gap-2"><input type="checkbox" checked={showReplay} onChange={(e) => setShowReplay(e.target.checked)} /> include replayed history (found by running the engine over past data)</label>}
      </div>

      {tab === 'today' && overview && (
        <div className="space-y-6">
          {!today?.available ? <div className="p-4 rounded-xl border border-amber-500/30 bg-amber-500/10 text-amber-200 text-sm">{today?.error ?? 'Today\'s scan is unavailable.'}</div> : (
            <>
              <div className="text-xs text-slate-400">Latest completed session: <b className="text-slate-200">{today.data_as_of}</b> · data {today.data_quality?.toLowerCase()} · {today.total_screened} stocks screened · {Object.entries(today.stage_counts ?? {}).map(([k, v]) => `${v} ${k.replaceAll('_', ' ').toLowerCase()}`).join(', ')}. Levels come from completed daily bars; the “Now” line shows where the price is at present.</div>
              <section className="space-y-3"><h2 className="text-lg font-semibold text-white">Upcoming breakouts ({today.upcoming?.length ?? 0})</h2>
                {(today.upcoming?.length ?? 0) === 0 ? <div className="text-sm text-slate-500 glass-card p-4">No stock is flagged as an upcoming breakout on the latest session.</div> : <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">{today.upcoming!.map((r) => <BreakoutCard key={r.symbol} r={r} />)}</div>}</section>
              <section className="space-y-3"><h2 className="text-lg font-semibold text-white">Confirmed breakouts ({today.confirmed?.length ?? 0})</h2>
                {(today.confirmed?.length ?? 0) === 0 ? <div className="text-sm text-slate-500 glass-card p-4">No confirmed breakouts on the latest session.</div> : <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">{today.confirmed!.map((r) => <BreakoutCard key={r.symbol} r={r} />)}</div>}</section>
              {(today.excluded?.length ?? 0) > 0 && <div className="text-xs text-slate-500">Not scanned: {today.excluded!.map((e) => `${e.symbol} (${e.reason})`).join('; ')}</div>}
            </>
          )}
        </div>
      )}

      {tab === 'upcoming' && overview && (
        <div className="space-y-4">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {[['Live flags', wLive], ['All flags (incl. replayed)', wAll]].map(([t, w]) => { const x = w as typeof wLive; return (
              <div key={t as string} className="glass-card p-4 text-sm"><div className="text-xs uppercase tracking-wide text-slate-400">{t as string}</div>
                {!x ? <div className="text-slate-500 mt-2">—</div> : <div className="mt-2 text-slate-200">Watching {x.watching} · broke out {x.broke_out} · invalidated {x.invalidated} · expired {x.expired}<br />Hit rate {x.hit_rate_pct == null ? '—' : `${x.hit_rate_pct.toFixed(0)}%`} · avg 10-session move <span className={tone(x.avg_10d_move_pct)}>{pct(x.avg_10d_move_pct)}</span><br /><span className="text-xs text-slate-500">{x.repeat_flags_excluded} repeat flag(s) not counted</span></div>}</div>); })}
            <div className="glass-card p-4 text-xs text-slate-400">{wAll?.note}</div>
          </div>
          <div className="glass-card overflow-x-auto"><table className="w-full text-left text-sm text-slate-400">
            <thead className="text-xs uppercase bg-white/5 text-slate-300"><tr><th className="px-3 py-3">Stock</th><th className="px-3 py-3">Flagged</th><th className="px-3 py-3">Outcome</th><th className="px-3 py-3">5-session move</th><th className="px-3 py-3">10-session move</th><th className="px-3 py-3">Watching</th><th className="px-3 py-3" /></tr></thead>
            <tbody>{flags.length === 0 ? <tr><td colSpan={7} className="px-3 py-6 text-center text-slate-500">No flags tracked yet.</td></tr> : flags.map((f) => <FlagRow key={`${f.symbol}${f.flagged_date}`} f={f} />)}</tbody></table></div>
          <div className="text-xs text-slate-500">Click a row for the reasons and levels frozen when it was flagged.</div>
        </div>
      )}

      {tab === 'confirmed' && overview && (
        <div className="space-y-4">
          {sc && <div className="grid grid-cols-1 md:grid-cols-3 gap-4"><Group title="All closed trades, after costs" g={sc.all} /><div className="glass-card p-4 text-sm text-slate-200 md:col-span-2">Tracked {sc.tracked} · open {sc.open} · hit target 1 {sc.worked} · stopped {sc.failed} · faded {sc.no_follow_through} · opened past target {sc.not_tradeable}.<br /><span className="text-xs text-slate-500">{sc.repeat_signals_excluded} repeat signal(s) are shown greyed and excluded. {sc.note}</span></div></div>}
          <div className="glass-card overflow-x-auto"><table className="w-full text-left text-sm text-slate-400">
            <thead className="text-xs uppercase bg-white/5 text-slate-300"><tr><th className="px-3 py-3">Stock</th><th className="px-3 py-3">Signal day</th><th className="px-3 py-3">Result</th><th className="px-3 py-3">Before costs</th><th className="px-3 py-3">After costs</th><th className="px-3 py-3">Sessions</th><th className="px-3 py-3" /></tr></thead>
            <tbody>{signals.length === 0 ? <tr><td colSpan={7} className="px-3 py-6 text-center text-slate-500">No signals tracked yet.</td></tr> : signals.map((s) => <SignalRow key={`${s.symbol}${s.signal_date}`} s={s} />)}</tbody></table></div>
        </div>
      )}

      {tab === 'auto' && (
        <div className="space-y-4">
          <div className="text-sm text-slate-300 max-w-3xl">The system buys and sells its own confirmed breakouts on paper, with no human in the loop: entry at the next session's official open, then the plan's stop, target or time limit. The frozen current plan always runs; if the learner ever promotes a better plan it gets its own portfolio beside it. Every trade is in the paper ledger, and every skipped signal is listed with its reason.</div>
          {portfolios.length === 0 ? <div className="glass-card p-4 text-slate-500 text-sm">The automatic portfolios start with the next evening's data load.</div> : portfolios.map((p) => <AutoCard key={p.portfolio_id} p={p} />)}
        </div>
      )}
    </div>
  );
}
