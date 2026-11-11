'use client';

import Link from '@/components/Link';
import type { BreakoutReading } from '@/lib/api/types';

export const inr = (v: number | null | undefined, d = 2) => (v == null ? '—' : `₹${v.toLocaleString('en-IN', { minimumFractionDigits: d, maximumFractionDigits: d })}`);
export const pct = (v: number | null | undefined, d = 2) => (v == null ? '—' : `${v >= 0 ? '+' : ''}${v.toFixed(d)}%`);
export const tone = (v: number | null | undefined) => (v == null ? 'text-slate-400' : v >= 0 ? 'text-emerald-400' : 'text-rose-400');

const BREAKDOWN_LABEL: Record<string, string> = {
  price_structure: 'Price structure', volume_participation: 'Volume', volatility_compression: 'Volatility squeeze', trend_momentum: 'Trend & momentum',
  relative_strength: 'Relative strength', risk_reward_quality: 'Reward vs risk',
};
const BREAKDOWN_MAX: Record<string, number> = { price_structure: 30, volume_participation: 25, volatility_compression: 15, trend_momentum: 15, relative_strength: 10, risk_reward_quality: 10 };

export function Shariah({ status }: { status: string | null | undefined }) {
  if (status === 'PASS') return <span className="text-emerald-400">Compliant</span>;
  if (status === 'FAIL') return <span className="text-rose-400">Not compliant</span>;
  return <span className="text-slate-400">Not screened</span>;
}

export function ScoreBars({ breakdown }: { breakdown: Record<string, number> | null | undefined }) {
  if (!breakdown || Object.keys(breakdown).length === 0) return null;
  return (
    <div className="space-y-1">
      {Object.entries(breakdown).map(([k, v]) => {
        const max = BREAKDOWN_MAX[k] ?? 30;
        return (
          <div key={k} className="flex items-center gap-2 text-[11px]">
            <span className="w-32 text-slate-400">{BREAKDOWN_LABEL[k] ?? k.replaceAll('_', ' ')}</span>
            <div className="flex-1 h-1.5 rounded bg-slate-800"><div className="h-1.5 rounded bg-sky-500/70" style={{ width: `${Math.min(100, (v / max) * 100)}%` }} /></div>
            <span className="w-10 text-right font-mono text-slate-300">{v}/{max}</span>
          </div>
        );
      })}
    </div>
  );
}

export function Reasons({ reasons, title = 'Why the engine flagged it' }: { reasons: string[] | null | undefined; title?: string }) {
  if (!reasons || reasons.length === 0) {
    return <div className="text-xs text-slate-500">Reasons were not stored for this signal.</div>;
  }
  return (
    <div>
      <div className="text-[11px] uppercase tracking-wide text-slate-500 mb-1">{title}</div>
      <ul className="space-y-1 text-sm text-slate-200 list-disc pl-5">{reasons.map((r, i) => <li key={i}>{r}</li>)}</ul>
    </div>
  );
}

export default function BreakoutCard({ r, linkSymbol = true }: { r: BreakoutReading; linkSymbol?: boolean }) {
  const confirmed = r.stage === 'CONFIRMED_BREAKOUT';
  const [lo, hi] = r.entry_zone?.length === 2 ? r.entry_zone : [null, null];
  return (
    <div className="glass-card p-4 space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        {linkSymbol ? <Link href={`/stocks/${r.symbol}?tab=breakouts`} className="text-lg font-semibold text-white hover:text-sky-300">{r.symbol}</Link> : <span className="text-lg font-semibold text-white">{r.symbol}</span>}
        <span className="font-mono text-slate-300">{inr(r.current_price)}</span>
        <span className={`text-[11px] font-bold px-2 py-0.5 rounded-full border ${confirmed ? 'border-emerald-500/40 text-emerald-300 bg-emerald-500/10' : 'border-sky-500/40 text-sky-300 bg-sky-500/10'}`}>
          {confirmed ? 'CONFIRMED BREAKOUT' : 'UPCOMING (CANDIDATE)'}
        </span>
        <span className="text-xs"><Shariah status={r.shariah_status} /></span>
        <span className="ml-auto text-xs text-slate-400">Score <b className="text-white">{r.structure_score ?? '—'}</b>/100 · RVOL <b className="text-white">{r.rvol.toFixed(2)}x</b></span>
      </div>
      <Reasons reasons={r.reasons} title={confirmed ? 'Why it counts as a breakout' : 'Why it may break out soon'} />
      <div className="grid grid-cols-2 md:grid-cols-5 gap-2 text-xs">
        {[['Resistance', inr(r.resistance_level)], ['Entry zone', lo == null ? '—' : `${inr(lo)} – ${inr(hi)}`], ['Stop (invalidation)', inr(r.invalidation_level)], ['Target 1', inr(r.target_1)], ['Target 2', inr(r.target_2)]].map(([k, v]) => (
          <div key={k} className="rounded-lg bg-slate-900/60 p-2"><div className="text-slate-500">{k}</div><div className="font-mono text-slate-100">{v}</div></div>
        ))}
      </div>
      {r.live && <div className={`text-xs rounded-lg px-3 py-2 border ${r.live.status === 'INVALIDATED' ? 'border-rose-500/30 text-rose-200 bg-rose-500/10' : r.live.status === 'IN_ENTRY_ZONE' ? 'border-emerald-500/30 text-emerald-200 bg-emerald-500/10' : 'border-amber-500/30 text-amber-200 bg-amber-500/10'}`}>Now: {r.live.message}</div>}
      <ScoreBars breakdown={r.score_breakdown} />
      {r.invalidation_conditions?.length > 0 && <div className="text-[11px] text-slate-500">Invalidated if: {r.invalidation_conditions.join('; ')}</div>}
      {!confirmed && <div className="text-[11px] text-slate-500">Reminder: the flag shows price squeezing under resistance. Whether such flags are followed by a breakout, and what the price does afterwards, is measured in the scorecard on this page; a breakout following is not the same as a profit.</div>}
    </div>
  );
}
