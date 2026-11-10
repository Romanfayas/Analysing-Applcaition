'use client';

import { useStockNews } from '@/lib/api/hooks';

const ago = (iso: string | null) => {
  if (!iso) return '';
  const m = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000));
  if (m < 60) return `${m} min ago`;
  if (m < 1440) return `${Math.round(m / 60)} h ago`;
  return `${Math.round(m / 1440)} d ago`;
};
const when = (iso: string | null) => (iso ? new Date(iso).toLocaleString('en-IN', { timeZone: 'Asia/Kolkata', day: '2-digit', month: 'short', hour: '2-digit', minute: '2-digit' }) : '');

export default function StockNewsTab({ symbol }: { symbol: string }) {
  const { data, isLoading, isError } = useStockNews(symbol);
  if (isLoading) return <div className="text-slate-500 text-sm py-8">Fetching the latest news…</div>;
  if (isError || !data) return <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-sm">Failed to load news for {symbol}.</div>;
  const nerr = data.status.news?.last_error, ferr = data.status.filings?.last_error;
  return (
    <div className="grid grid-cols-1 xl:grid-cols-3 gap-6">
      <section className="xl:col-span-2 space-y-3">
        <div className="flex items-baseline gap-3"><h2 className="text-lg font-semibold text-white">News</h2><span className="text-xs text-slate-500">updated {ago(data.status.news?.last_success ?? null) || 'never'}</span></div>
        {nerr && <div className="text-xs rounded-lg border border-amber-500/30 bg-amber-500/10 text-amber-200 px-3 py-2">The latest refresh failed ({nerr}). Showing what was stored earlier.</div>}
        {data.headlines.length === 0 ? <div className="glass-card p-4 text-sm text-slate-400">No headlines found for {data.name || symbol} in the last 30 days.</div> : (
          <div className="glass-card divide-y divide-white/5">
            {data.headlines.map((n) => (
              <a key={n.url} href={n.url} target="_blank" rel="noopener noreferrer" className="block p-4 hover:bg-white/5">
                <div className="text-sm text-slate-100">{n.title}</div>
                <div className="text-xs text-slate-500 mt-1">{n.publisher ?? 'Unknown publisher'} · {when(n.published_at)} · {ago(n.published_at)}</div>
              </a>))}
          </div>
        )}
        <div className="text-[11px] text-slate-500">{data.sources.headlines}</div>
      </section>
      <section className="space-y-3">
        <div className="flex items-baseline gap-3"><h2 className="text-lg font-semibold text-white">Official announcements</h2><span className="text-xs text-slate-500">NSE</span></div>
        {ferr && <div className="text-xs rounded-lg border border-amber-500/30 bg-amber-500/10 text-amber-200 px-3 py-2">The latest refresh failed ({ferr}).</div>}
        {data.filings.length === 0 ? <div className="glass-card p-4 text-sm text-slate-400">No announcements in the last 90 days.</div> : (
          <div className="glass-card divide-y divide-white/5">
            {data.filings.map((f, i) => (
              <div key={i} className="p-3 space-y-1">
                <div className="text-[11px] uppercase tracking-wide text-slate-500">{f.category ?? 'Announcement'} · {when(f.announced_at)}</div>
                <div className="text-sm text-slate-200">{f.subject}</div>
                {f.attachment_url && <a href={f.attachment_url} target="_blank" rel="noopener noreferrer" className="text-xs text-sky-300 hover:underline">Open the filing</a>}
              </div>))}
          </div>
        )}
        <div className="text-[11px] text-slate-500">{data.sources.filings}</div>
      </section>
    </div>
  );
}
