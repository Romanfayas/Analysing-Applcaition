'use client';

import { useState } from 'react';
import { useLearning } from '@/lib/api/hooks';
import { send } from '@/lib/api/client';
import type { LearningBlock, PnlAudit, StrategyVersion } from '@/lib/api/types';

const inr = (v: number | undefined | null) => (v == null ? '—' : `₹${Math.round(v).toLocaleString('en-IN')}`);
const pct = (v: number | null | undefined, d = 2) => (v == null ? '—' : `${v >= 0 ? '+' : ''}${v.toFixed(d)}%`);
const short = (name: string) => name.replace('BO::', '').replaceAll('::', ' · ').replaceAll('_', ' ');

const DECISIONS: Record<string, { label: string; tone: 'ok' | 'warn' | 'bad' | 'info'; text: string }> = {
  PROMOTED: { label: 'A new strategy version was promoted on paper', tone: 'ok', text: 'Every gate passed on data the learner had not seen. It now trades on paper next to the frozen control, on probation.' },
  KEPT_ACTIVE: { label: 'The active version stays', tone: 'ok', text: 'It is on probation and has not fallen behind the control.' },
  ROLLED_BACK_PROBATION: { label: 'The active version failed probation and was rolled back', tone: 'bad', text: "Its forward trades fell behind the frozen control, so the control is back in charge. That version can't be promoted again." },
  NO_CHANGE_GATES_NOT_MET: { label: 'No change: nothing beat the current plan', tone: 'info', text: 'There was enough data to judge, and the learner’s picks did not clear every gate. The frozen control stays in charge. This is a result, not an error.' },
  NO_CHANGE_INSUFFICIENT_DATA: { label: 'No change: not enough data yet', tone: 'warn', text: 'The learner needs more finished trades before it may promote anything.' },
};
const TONE = { ok: 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200', warn: 'border-amber-500/30 bg-amber-500/10 text-amber-200', bad: 'border-rose-500/30 bg-rose-500/10 text-rose-200', info: 'border-sky-500/30 bg-sky-500/10 text-sky-200' };

function Stat({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="glass-card p-4">
      <div className="text-xs uppercase tracking-wide text-slate-400">{label}</div>
      <div className="text-2xl font-semibold text-white mt-1">{value}</div>
      {sub && <div className="text-xs text-slate-500 mt-1">{sub}</div>}
    </div>
  );
}

function Audit({ a }: { a: PnlAudit }) {
  if (!a.trades) return null;
  const net = a.net_pnl_inr ?? 0;
  return (
    <div className="glass-card p-4 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-white">{short(a.label)}</h3>
        <span className={`text-xs px-2 py-0.5 rounded-full border ${a.reconciled_to_ledger ? 'border-emerald-500/30 text-emerald-300' : 'border-rose-500/30 text-rose-300'}`}>
          {a.reconciled_to_ledger ? 'reconciles to the cost ledger' : 'DOES NOT RECONCILE'}
        </span>
      </div>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
        <div><div className="text-slate-500 text-xs">Trades</div><div className="text-white font-mono">{a.trades}</div></div>
        <div><div className="text-slate-500 text-xs">Gross P&amp;L</div><div className="font-mono text-slate-200">{inr(a.gross_pnl_inr)}</div></div>
        <div><div className="text-slate-500 text-xs">Costs &amp; slippage</div><div className="font-mono text-amber-300">−{inr(a.total_costs_inr)}</div></div>
        <div><div className="text-slate-500 text-xs">Net P&amp;L</div><div className={`font-mono font-semibold ${net >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>{inr(net)}</div></div>
      </div>
      <div className="text-xs text-slate-400">
        ₹{(a.capital_inr ?? 0).toLocaleString('en-IN')} account, ₹{(a.notional_per_trade_inr ?? 0).toLocaleString('en-IN')} per trade, ending {inr(a.ending_equity_inr)}.
        {a.max_drawdown && <> Worst fall from a peak: {a.max_drawdown.max_drawdown_pct.toFixed(1)}% ({a.max_drawdown.peak_date} to {a.max_drawdown.trough_date}).</>}
      </div>
      {a.capital_warning && <div className="text-xs text-amber-300">{a.capital_warning}</div>}
      {a.label.startsWith('SELECTED') && (
        <div className="text-xs text-amber-300">In-sample: this plan was picked with hindsight, using the same years it is shown on. It is not evidence of profit. The quarter-by-quarter table above is the fair test.</div>
      )}
    </div>
  );
}

export default function LearningPage() {
  const { learning, isLoading, isError, mutate } = useLearning();
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  const run = learning?.last_run;
  const decision = learning?.last_decision ? DECISIONS[learning.last_decision] : null;

  async function rollback(v: StrategyVersion) {
    if (!window.confirm(`Roll back ${short(v.name)}? The frozen control takes over paper trading again, and this version can never be promoted again.`)) return;
    setBusy(true);
    setMsg(null);
    try {
      await send('post', `/learning/versions/${v.id}/rollback`, { reason: 'rolled back from the Learning page' });
      setMsg('Rolled back. The frozen control is in charge again.');
      await mutate();
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'Rollback failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      <div>
        <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Learning from outcomes</h1>
        <p className="text-sm text-slate-400 mt-1 max-w-3xl">
          Every breakout is followed to its result and a lesson is stored. Nightly, the system re-tests a fixed menu of 24 trade plans quarter by quarter, choosing only from
          trades that had already finished, and scores each choice on the next quarter it never saw. Paper trading only; nothing here places a real order.
        </p>
      </div>

      {isError && <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-sm">Failed to load the learning summary.</div>}
      {msg && <div className="p-3 rounded-xl bg-slate-800/80 border border-slate-700/60 text-sm text-slate-200">{msg}</div>}
      {isLoading && <div className="text-slate-500 text-sm">Loading…</div>}

      {learning && !run && (
        <div className="p-4 rounded-xl border border-amber-500/30 bg-amber-500/10 text-amber-200 text-sm">
          The learner has not run yet. It runs nightly after the exchange data for the day is in; the first result appears here after that.
        </div>
      )}

      {run && decision && (
        <div className={`p-4 rounded-xl border ${TONE[decision.tone]}`}>
          <div className="font-semibold">{decision.label}</div>
          <div className="text-sm mt-1 opacity-90">{decision.text}</div>
          <div className="text-xs mt-2 opacity-70">Run #{learning?.last_run_id} · data through {run.data_through}{learning?.last_run_at ? ` · ${new Date(learning.last_run_at).toLocaleString('en-IN')}` : ''}</div>
        </div>
      )}

      {learning && (
        <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
          <Stat label="In charge of paper trading" value={learning.active ? short(learning.active.name) : 'Frozen control'} sub={learning.active ? 'on probation against the control' : 'target 1 or stop, 10 sessions, no filters'} />
          <Stat label="Picked-by-the-learner, out of sample" value={run ? pct(run.policy_oos.mean_net_pct) : '—'} sub={run ? `${run.policy_oos.trades} trades · control ${pct(run.control_oos.mean_net_pct)} over ${run.control_oos.trades}` : undefined} />
          <Stat label="Lessons stored" value={String(learning.lessons_total)} sub="one per resolved breakout" />
        </div>
      )}

      {learning?.active && (
        <div className="glass-card p-4 flex flex-col md:flex-row md:items-center justify-between gap-3">
          <div>
            <div className="text-white font-semibold">{short(learning.active.name)}</div>
            <div className="text-xs text-slate-400">{learning.active.status_reason}</div>
          </div>
          <button disabled={busy} onClick={() => rollback(learning.active!)} className="px-4 py-2 rounded-lg bg-rose-600/80 hover:bg-rose-600 text-white text-sm disabled:opacity-50">
            Roll back to the frozen control
          </button>
        </div>
      )}

      {run && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-white">The gates a version must pass to be promoted</h2>
          <div className="glass-card divide-y divide-white/5">
            {Object.entries(run.gates).map(([k, g]) => (
              <div key={k} className="p-3 flex items-start gap-3">
                <span className={`mt-0.5 text-xs font-bold px-2 py-0.5 rounded-full ${g.pass ? 'bg-emerald-500/20 text-emerald-300' : 'bg-rose-500/20 text-rose-300'}`}>{g.pass ? 'PASS' : 'FAIL'}</span>
                <div className="text-sm">
                  <div className="text-slate-200">{g.detail}</div>
                  {g.interval && <div className="text-xs text-slate-500 font-mono mt-0.5">interval {pct(g.interval.low)} to {pct(g.interval.high)}</div>}
                </div>
              </div>
            ))}
          </div>
          <p className="text-xs text-slate-500">{run.multiple_testing_note}</p>
        </section>
      )}

      {run && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-white">Quarter by quarter: what the learner picked and how it did</h2>
          <div className="glass-card overflow-x-auto">
            <table className="w-full text-left text-sm text-slate-400">
              <thead className="text-xs uppercase bg-white/5 text-slate-300">
                <tr><th className="px-4 py-3">Quarter starting</th><th className="px-4 py-3">Picked (before seeing the quarter)</th><th className="px-4 py-3">Its trades</th><th className="px-4 py-3">Its net return</th><th className="px-4 py-3">Control net return</th></tr>
              </thead>
              <tbody>
                {run.walk_forward_blocks.map((b: LearningBlock) => (
                  <tr key={b.origin} className="border-b border-white/5">
                    <td className="px-4 py-2 text-white">{b.origin}{!b.completed && <span className="ml-2 text-xs text-amber-300">in progress, not counted</span>}</td>
                    <td className="px-4 py-2 text-xs">{short(b.chosen)}</td>
                    <td className="px-4 py-2 font-mono">{b.policy_trades}</td>
                    <td className={`px-4 py-2 font-mono ${(b.policy_mean_net_pct ?? 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>{pct(b.policy_mean_net_pct)}</td>
                    <td className={`px-4 py-2 font-mono ${(b.control_mean_net_pct ?? 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>{pct(b.control_mean_net_pct)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {run && run.pnl_audit.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-white">Paper P&amp;L on past data, checked against the ledger</h2>
          <p className="text-xs text-slate-500">Each trade is re-priced through the itemized cost ledger (statutory charges, broker charges and assumed slippage) and replayed in date order. The ending balance must equal the account plus the ledger’s net P&amp;L.</p>
          <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">{run.pnl_audit.map((a) => <Audit key={a.label} a={a} />)}</div>
        </section>
      )}

      {learning && learning.lesson_tags.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-white">What the stored lessons show</h2>
          <p className="text-xs text-slate-500">Descriptive counts only. A tag becomes a rule only if it passes the same out-of-sample test as everything else, and with few signals these patterns are mostly noise.</p>
          <div className="glass-card overflow-x-auto">
            <table className="w-full text-left text-sm text-slate-400">
              <thead className="text-xs uppercase bg-white/5 text-slate-300"><tr><th className="px-4 py-3">Tag</th><th className="px-4 py-3">Signals</th><th className="px-4 py-3">Hit target</th><th className="px-4 py-3">Stopped out</th></tr></thead>
              <tbody>
                {learning.lesson_tags.map((t) => (
                  <tr key={t.tag} className="border-b border-white/5"><td className="px-4 py-2 text-white">{t.tag.replaceAll('_', ' ').toLowerCase()}</td><td className="px-4 py-2 font-mono">{t.n}</td><td className="px-4 py-2 font-mono text-emerald-400">{t.worked}</td><td className="px-4 py-2 font-mono text-rose-400">{t.failed}</td></tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {learning && learning.recent_lessons.length > 0 && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-white">Recent lessons</h2>
          <div className="glass-card divide-y divide-white/5">
            {learning.recent_lessons.map((l, i) => (
              <div key={i} className="p-3 text-sm">
                <div className="flex items-center gap-2">
                  <span className="text-white font-medium">{l.symbol}</span>
                  <span className="text-xs text-slate-500">{l.signal_date}</span>
                  <span className="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-300">{l.origin === 'LIVE' ? 'live signal' : 'replayed from history'}</span>
                  <span className="text-xs px-2 py-0.5 rounded-full bg-slate-800 text-slate-300">{l.verdict.replaceAll('_', ' ').toLowerCase()}</span>
                </div>
                <div className="text-slate-300 mt-1">{l.lesson}</div>
              </div>
            ))}
          </div>
        </section>
      )}

      {learning && (
        <section className="space-y-3">
          <h2 className="text-lg font-semibold text-white">Strategy versions</h2>
          <div className="glass-card overflow-x-auto">
            <table className="w-full text-left text-sm text-slate-400">
              <thead className="text-xs uppercase bg-white/5 text-slate-300"><tr><th className="px-4 py-3">Version</th><th className="px-4 py-3">Status</th><th className="px-4 py-3">Why</th></tr></thead>
              <tbody>
                {learning.versions.filter((v) => v.status !== 'CANDIDATE').map((v) => (
                  <tr key={v.id} className="border-b border-white/5"><td className="px-4 py-2 text-white text-xs">{short(v.name)}</td><td className="px-4 py-2 text-xs">{v.status.replaceAll('_', ' ')}</td><td className="px-4 py-2 text-xs">{v.status_reason}</td></tr>
                ))}
                <tr><td className="px-4 py-2 text-slate-500 text-xs" colSpan={3}>{learning.versions.filter((v) => v.status === 'CANDIDATE').length} further candidates in the pre-registered menu are waiting; none has passed the gates.</td></tr>
              </tbody>
            </table>
          </div>
          {learning.recent_status_events.length > 0 && (
            <div className="text-xs text-slate-500">Recent changes: {learning.recent_status_events.slice(0, 5).map((e) => `${new Date(e.at).toLocaleDateString('en-IN')} ${short(e.version)} → ${e.status.replaceAll('_', ' ').toLowerCase()} (${e.actor.toLowerCase()})`).join(' · ')}</div>
          )}
        </section>
      )}

      {run && (
        <section className="text-xs text-slate-500 space-y-1">
          <div className="font-semibold text-slate-400">Limits of this evidence</div>
          <ul className="list-disc pl-5 space-y-1">{run.limitations.map((x, i) => <li key={i}>{x}</li>)}</ul>
        </section>
      )}
    </div>
  );
}
