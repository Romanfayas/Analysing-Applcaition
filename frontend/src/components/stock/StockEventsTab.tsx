'use client';

import { useStockEvents } from '@/lib/api/hooks';
import type { StockEvent } from '@/lib/api/types';

const KIND: Record<StockEvent['kind'], string> = { BOARD_MEETING: 'Board meeting', CORPORATE_ACTION: 'Corporate action' };
const day = (d: string) => new Date(d + 'T00:00:00').toLocaleDateString('en-IN', { day: '2-digit', month: 'short', year: 'numeric' });

function List({ items, empty }: { items: StockEvent[]; empty: string }) {
  if (items.length === 0) return <div className="glass-card p-4 text-sm text-slate-400">{empty}</div>;
  return (
    <div className="glass-card divide-y divide-white/5">
      {items.map((e, i) => (
        <div key={i} className="p-4 flex gap-4">
          <div className="w-28 shrink-0"><div className="text-sm font-mono text-white">{day(e.event_date)}</div><div className="text-[11px] text-slate-500">{KIND[e.kind]}</div></div>
          <div><div className="text-sm text-slate-100">{e.title}</div>{e.detail && <div className="text-xs text-slate-400 mt-0.5">{e.detail}</div>}</div>
        </div>))}
    </div>
  );
}

export default function StockEventsTab({ symbol }: { symbol: string }) {
  const { data, isLoading, isError } = useStockEvents(symbol);
  if (isLoading) return <div className="text-slate-500 text-sm py-8">Fetching events…</div>;
  if (isError || !data) return <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-sm">Failed to load events for {symbol}.</div>;
  const err = data.status.events?.last_error;
  return (
    <div className="space-y-6">
      {err && <div className="text-xs rounded-lg border border-amber-500/30 bg-amber-500/10 text-amber-200 px-3 py-2">The latest refresh failed ({err}). Showing what was stored earlier.</div>}
      <section className="space-y-2"><h2 className="text-lg font-semibold text-white">Upcoming</h2><List items={data.upcoming} empty="No upcoming board meetings or corporate actions announced." /></section>
      <section className="space-y-2"><h2 className="text-lg font-semibold text-white">Recent</h2><List items={data.recent} empty="Nothing in the recent past." /></section>
      <div className="text-[11px] text-slate-500">{data.source}</div>
    </div>
  );
}
