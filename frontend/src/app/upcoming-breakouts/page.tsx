'use client';

import Link from '@/components/Link';
import { useUpcomingBreakouts } from '@/lib/api/hooks';
import { Reasons, ScoreBars, Shariah, inr } from '@/components/breakouts/BreakoutCard';
import PatternTests, { StockHistory } from '@/components/breakouts/PatternTests';
import type { ReplaySetup, UpcomingItem } from '@/lib/api/types';

const ago = (iso: string | null) => {
  if (!iso) return '';
  const h = (Date.now() - new Date(iso).getTime()) / 36e5;
  if (h < 1) return `${Math.max(1, Math.round(h * 60))} min ago`;
  if (h < 48) return `${Math.round(h)} h ago`;
  return `${Math.round(h / 24)} d ago`;
};

const todayList = (cands: UpcomingItem[], closest: UpcomingItem[], confirmed: string[]): Record<ReplaySetup, string[]> => ({
  CLOSEST: closest.map((c) => c.result.symbol), UPCOMING: cands.map((c) => c.result.symbol), CONFIRMED: confirmed,
});

function StockCard({ item, kind }: { item: UpcomingItem; kind: 'candidate' | 'closest' }) {
  const r = item.result;
  const [lo, hi] = r.entry_zone?.length === 2 ? r.entry_zone : [null, null];
  return (
    <div className="glass-card p-4 space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <Link href={`/stocks/${r.symbol}?tab=breakouts`} className="text-lg font-semibold text-white hover:text-sky-300">{r.symbol}</Link>
        <span className="text-sm text-slate-400">{item.name}</span>
        <span className="font-mono text-slate-300">{inr(r.current_price)}</span>
        <span className={`text-[11px] font-bold px-2 py-0.5 rounded-full border ${kind === 'candidate' ? 'border-sky-500/40 text-sky-300 bg-sky-500/10' : 'border-amber-500/40 text-amber-300 bg-amber-500/10'}`}>
          {kind === 'candidate' ? 'UPCOMING (ENGINE CANDIDATE)' : 'CLOSEST TO BREAKING OUT'}
        </span>
        <span className="text-xs"><Shariah status={r.shariah_status} /></span>
        <span className="ml-auto text-xs text-slate-400">
          {r.distance_to_resistance_pct > 0 ? <><b className="text-white">{r.distance_to_resistance_pct.toFixed(2)}%</b> below resistance · </> : null}
          Score <b className="text-white">{r.structure_score ?? '—'}</b>/100 · RVOL <b className="text-white">{r.rvol.toFixed(2)}x</b>
        </span>
      </div>

      <div className="grid md:grid-cols-2 gap-5">
        <div className="space-y-3">
          <Reasons reasons={r.reasons} title="Why it may break out" />
          <div className="grid grid-cols-2 md:grid-cols-3 gap-2 text-xs">
            {[['Resistance (the level to clear)', inr(r.resistance_level)], ['Entry zone', lo == null ? '—' : `${inr(lo)} – ${inr(hi)}`], ['Stop (invalidation)', inr(r.invalidation_level)], ['Target 1', inr(r.target_1)], ['Target 2', inr(r.target_2)]].map(([k, v]) => (
              <div key={k} className="rounded-lg bg-slate-900/60 p-2"><div className="text-slate-500">{k}</div><div className="font-mono text-slate-100">{v}</div></div>
            ))}
          </div>
          <ScoreBars breakdown={r.score_breakdown} />
        </div>

        <div className="space-y-3">
          <div className="text-[11px] uppercase tracking-wide text-slate-500">Related news</div>
          {item.news.length === 0 ? <div className="text-xs text-slate-500">No headlines stored for this stock yet.</div> : (
            <ul className="space-y-2">
              {item.news.map((n) => (
                <li key={n.url} className="text-sm">
                  <a href={n.url} target="_blank" rel="noopener noreferrer" className="text-slate-100 hover:text-sky-300">{n.title}</a>
                  <div className="text-[11px] text-slate-500">{n.publisher ?? 'Unknown publisher'}{n.published_at ? ` · ${ago(n.published_at)}` : ''} · unofficial headline</div>
                </li>
              ))}
            </ul>
          )}
          {item.filings.length > 0 && (
            <div>
              <div className="text-[11px] uppercase tracking-wide text-slate-500 mb-1">Official NSE announcements</div>
              <ul className="space-y-1.5">
                {item.filings.map((f, i) => (
                  <li key={i} className="text-sm text-slate-200">
                    {f.attachment_url ? <a href={f.attachment_url} target="_blank" rel="noopener noreferrer" className="hover:text-sky-300">{f.subject}</a> : f.subject}
                    <div className="text-[11px] text-slate-500">{new Date(f.announced_at).toLocaleDateString('en-IN')}{f.category ? ` · ${f.category}` : ''}</div>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {item.next_event && (
            <div className="text-xs rounded-lg border border-amber-500/30 bg-amber-500/10 text-amber-200 px-3 py-2">
              Next event: {item.next_event.title} on {item.next_event.event_date}
            </div>
          )}
          <Link href={`/stocks/${r.symbol}?tab=news`} className="inline-block text-xs text-sky-300 hover:underline">All news, filings and events for {r.symbol} →</Link>
        </div>
      </div>
      <StockHistory symbol={r.symbol} setup={kind === 'candidate' ? 'UPCOMING' : 'CLOSEST'} />
      {r.invalidation_conditions?.length > 0 && <div className="text-[11px] text-slate-500">Invalidated if: {r.invalidation_conditions.join('; ')}</div>}
    </div>
  );
}

export default function UpcomingBreakoutsPage() {
  const { data, error, isLoading } = useUpcomingBreakouts();
  return (
    <div className="space-y-6 w-full">
      <div>
        <h1 className="text-2xl font-bold text-white">Upcoming Breakouts</h1>
        <p className="text-sm text-slate-400 mt-1">Stocks the engine expects may break out next, why, and what is being reported about them.</p>
      </div>

      {isLoading && <div className="text-slate-400 text-sm">Loading… (fetching the latest news for each stock can take a few seconds)</div>}
      {error && <div className="text-rose-400 text-sm">Could not load upcoming breakouts: {error.message}</div>}

      {data && (
        <>
          <div className="text-xs text-slate-400 rounded-lg border border-white/10 bg-slate-900/60 p-3 space-y-1">
            <div>Based on the last completed session: <b className="text-white">{data.data_as_of ?? 'unknown'}</b> · {data.total_screened} stocks screened. These levels come from daily bars, so the list is rebuilt each evening once the exchange file is loaded.</div>
            <div className="text-slate-500">{data.note}</div>
          </div>

          <PatternTests today={todayList(data.candidates, data.closest, data.confirmed_symbols ?? [])} />

          <section className="space-y-3">
            <h2 className="text-lg font-semibold text-white">Upcoming: engine candidates ({data.candidates.length})</h2>
            {data.candidates.length === 0
              ? <div className="glass-card p-4 text-sm text-slate-400">No stock meets the engine&apos;s upcoming-breakout conditions on this session. That is common: such flags are rare. The closest stocks are listed below.</div>
              : data.candidates.map((c) => <StockCard key={c.result.symbol} item={c} kind="candidate" />)}
          </section>

          <section className="space-y-3">
            <h2 className="text-lg font-semibold text-white">Closest to breaking out ({data.closest.length})</h2>
            <p className="text-xs text-slate-500">Stocks the engine only watches, ranked by how near the price is to its resistance. They have not met the conditions of an upcoming flag.</p>
            {data.closest.length === 0
              ? <div className="glass-card p-4 text-sm text-slate-400">No watched stock is within 6% of its resistance.</div>
              : data.closest.map((c) => <StockCard key={c.result.symbol} item={c} kind="closest" />)}
          </section>

          {data.watching_flags.length > 0 && (
            <section className="space-y-2">
              <h2 className="text-lg font-semibold text-white">Flags still being followed ({data.watching_flags.length})</h2>
              <div className="glass-card p-4 text-sm text-slate-300 flex flex-wrap gap-2">
                {data.watching_flags.map((f) => (
                  <Link key={`${f.symbol}-${f.flagged_date}`} href={`/stocks/${f.symbol}?tab=breakouts`} className="px-2 py-1 rounded-lg bg-slate-800 hover:bg-slate-700">{f.symbol} <span className="text-slate-500 text-xs">flagged {f.flagged_date} · {f.sessions_elapsed}/10</span></Link>
                ))}
              </div>
            </section>
          )}
        </>
      )}
    </div>
  );
}
