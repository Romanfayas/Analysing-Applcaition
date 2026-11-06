'use client';

import { useState } from 'react';
import type { IpoCategoryBook, IpoView } from '@/lib/api/types';
import { XMarkIcon } from '@heroicons/react/24/outline';

const num = (v: number | null | undefined) => (v == null ? '—' : Math.round(v).toLocaleString('en-IN'));
const cr = (v: number | null | undefined) => (v == null ? '—' : `₹${v.toLocaleString('en-IN', { maximumFractionDigits: 2 })} Cr`);
const times = (v: number | null | undefined) => (v == null ? '—' : `${v.toFixed(2)}x`);
const ORDER = ['QIB', 'BHNI', 'SHNI', 'NII', 'RETAIL', 'TOTAL'];
const TABS = ['Subscription', 'Allocation', 'Issue', 'Timeline', 'Financials'] as const;
type Tab = (typeof TABS)[number];

const BASIS: Record<IpoCategoryBook['basis'], string> = {
  NSE_CONSOLIDATED: 'NSE, all exchanges',
  NSE_ONLY: 'NSE window only',
  DERIVED: 'Shares bid from NSE, shares offered from IPOWatch',
};

function Kv({ rows }: { rows: [string, React.ReactNode][] }) {
  return (
    <dl className="divide-y divide-slate-800/70 text-sm">
      {rows.filter(([, v]) => v != null && v !== '' && v !== '—').map(([k, v]) => (
        <div key={k} className="flex justify-between gap-6 py-1.5"><dt className="text-slate-400">{k}</dt><dd className="text-white text-right">{v}</dd></div>
      ))}
    </dl>
  );
}

function Records({ rows }: { rows?: Record<string, string>[] }) {
  if (!rows?.length) return null;
  const heads = Object.keys(rows[0]);
  return (
    <table className="w-full text-xs text-left">
      <thead><tr className="text-slate-400 border-b border-slate-800">{heads.map((h) => <th key={h} className="py-1.5 pr-3">{h}</th>)}</tr></thead>
      <tbody className="divide-y divide-slate-800/60">{rows.map((r, i) => <tr key={i}>{heads.map((h) => <td key={h} className="py-1.5 pr-3 text-slate-200">{r[h]}</td>)}</tr>)}</tbody>
    </table>
  );
}

/** The full record of an issue. Official NSE figures and unofficial ones are always labelled apart. */
export default function IpoDetailsModal({ ipo, onClose }: { ipo: IpoView; onClose: () => void }) {
  const [tab, setTab] = useState<Tab>('Subscription');
  const d = ipo.details;
  const u = d?.unofficial;
  const o = d?.official;
  const cats = [...(d?.categories ?? [])].sort((a, b) => ORDER.indexOf(a.category) - ORDER.indexOf(b.category));

  return (
    <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-start justify-center p-4 overflow-y-auto" onClick={onClose}>
      <div role="dialog" aria-modal="true" aria-label={`${ipo.name} details`} className="w-full max-w-3xl my-8 rounded-2xl bg-slate-900 border border-slate-700 shadow-2xl" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-start justify-between gap-4 p-5 border-b border-slate-800">
          <div>
            <h3 className="text-xl font-bold text-white">{ipo.name}</h3>
            <p className="text-xs text-slate-400 mt-0.5">{[ipo.symbol, ipo.listing_at ?? ipo.board, ipo.price_band, ipo.issue_size].filter(Boolean).join(' · ')}</p>
          </div>
          <button onClick={onClose} aria-label="Close" className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800"><XMarkIcon className="w-5 h-5" /></button>
        </div>
        <div className="flex gap-1 px-5 pt-3 flex-wrap">
          {TABS.map((t) => (
            <button key={t} onClick={() => setTab(t)} className={`px-3 py-1.5 rounded-lg text-xs font-semibold ${tab === t ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-white'}`}>{t}</button>
          ))}
        </div>
        <div className="p-5 space-y-4">
          {!d && <p className="text-sm text-slate-400">Details for this issue have not been loaded. They are refreshed hourly for upcoming, open and recently listed issues.</p>}

          {d && tab === 'Subscription' && (
            cats.length === 0 ? <p className="text-sm text-slate-400">No bids have been published yet.</p> : (
              <>
                <div className="overflow-x-auto">
                  <table className="w-full text-sm text-left">
                    <thead><tr className="text-xs uppercase text-slate-400 border-b border-slate-800"><th className="py-2">Category</th><th className="text-right">Shares offered</th><th className="text-right">Shares bid</th><th className="text-right">Subscribed</th></tr></thead>
                    <tbody className="divide-y divide-slate-800/60 font-mono">
                      {cats.map((c) => (
                        <tr key={c.category} className={c.category === 'TOTAL' ? 'font-bold' : ''}>
                          <td className="py-2 font-sans text-white" title={BASIS[c.basis]}>{c.label}{c.basis === 'DERIVED' && <span className="text-amber-300"> *</span>}</td>
                          <td className="text-right text-slate-300">{num(c.shares_offered)}</td>
                          <td className="text-right text-slate-300">{num(c.shares_bid)}</td>
                          <td className="text-right text-emerald-400">{times(c.times)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <p className="text-[11px] text-slate-500">
                  bHNI: non-institutional bids above ₹10 lakh. sHNI: ₹2-10 lakh. Bids from NSE{o?.book_updated_at ? `, updated ${o.book_updated_at}` : ''}.
                  {cats.some((c) => c.basis === 'DERIVED') && ' * NSE publishes no shares offered for SME issues, so these multiples are worked out from IPOWatch\'s share counts; bHNI and sHNI assume the standard 2:1 split.'}
                </p>
              </>
            )
          )}

          {d && tab === 'Allocation' && (
            <>
              <h4 className="text-sm font-semibold text-white">Shares set aside for each category</h4>
              {u?.reservations?.length ? (
                <table className="w-full text-sm text-left">
                  <thead><tr className="text-xs uppercase text-slate-400 border-b border-slate-800"><th className="py-2">Category</th><th className="text-right">Shares</th><th className="text-right">Share of issue</th></tr></thead>
                  <tbody className="divide-y divide-slate-800/60 font-mono">
                    {u.reservations.map((r) => <tr key={r.category}><td className="py-2 font-sans text-white">{r.category.replace(' Shares Offered', '')}</td><td className="text-right text-slate-300">{num(r.shares)}</td><td className="text-right text-slate-300">{r.percent == null ? '—' : `${r.percent.toFixed(2)}%`}</td></tr>)}
                  </tbody>
                </table>
              ) : <p className="text-sm text-slate-400">Category shares are not available for this issue.</p>}
              {u?.lot_tiers?.length ? (
                <>
                  <h4 className="text-sm font-semibold text-white pt-2">Minimum application by investor type</h4>
                  <table className="w-full text-sm text-left">
                    <thead><tr className="text-xs uppercase text-slate-400 border-b border-slate-800"><th className="py-2">Application</th><th className="text-right">Lots</th><th className="text-right">Shares</th><th className="text-right">Amount</th></tr></thead>
                    <tbody className="divide-y divide-slate-800/60 font-mono">{u.lot_tiers.map((l) => <tr key={l.application}><td className="py-2 font-sans text-white">{l.application}</td><td className="text-right text-slate-300">{l.lots}</td><td className="text-right text-slate-300">{l.shares}</td><td className="text-right text-slate-300">{l.amount}</td></tr>)}</tbody>
                  </table>
                </>
              ) : null}
              <p className="text-[11px] text-slate-500">
                This is how shares are divided before allotment. How many each applicant receives is decided by the registrar&apos;s basis of allotment, which is not published as data.
                Source: IPOWatch (unofficial).
              </p>
            </>
          )}

          {d && tab === 'Issue' && (
            <>
              <Kv rows={[
                ['Total issue size', cr(d.issue_size_cr)],
                ['Fresh issue (money to the company)', cr(d.fresh_issue_cr)],
                ['Offer for sale (money to existing holders)', d.ofs_cr != null ? cr(d.ofs_cr) : d.fresh_issue_cr != null ? 'None' : '—'],
                ['Fresh issue shares', num(o?.structure.fresh_shares)],
                ['Offer for sale shares', num(o?.structure.ofs_shares)],
                ['Anchor portion', o?.structure.anchor_shares ? `${num(o.structure.anchor_shares)} shares` : null],
                ['Market maker portion', o?.structure.market_maker_shares ? `${num(o.structure.market_maker_shares)} shares` : null],
                ['Lot size', d.lot_size ? `${d.lot_size} shares` : null],
                ['Minimum investment', d.min_investment ? `₹${d.min_investment.toLocaleString('en-IN')}` : null],
                ['Face value', o?.face_value], ['Issue type', o?.issue_type], ['Lead manager', o?.lead_managers], ['Registrar', o?.registrar], ['Sponsor bank', o?.sponsor_bank],
              ]} />
              {o?.issue_size_text && <p className="text-[11px] text-slate-500">NSE: {o.issue_size_text}</p>}
              <div className="flex gap-4 text-xs">
                {o?.prospectus_url && <a href={o.prospectus_url} target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:text-blue-300">Red Herring Prospectus ↗</a>}
                {o?.anchor_report_url && <a href={o.anchor_report_url} target="_blank" rel="noopener noreferrer" className="text-blue-400 hover:text-blue-300">Anchor allocation report ↗</a>}
              </div>
            </>
          )}

          {d && tab === 'Timeline' && (u?.timeline ? <Kv rows={Object.entries(u.timeline)} /> : <p className="text-sm text-slate-400">The timetable is not available.</p>)}

          {d && tab === 'Financials' && (
            <>
              <Records rows={u?.financials} />
              {u?.kpis && <Kv rows={Object.entries(u.kpis)} />}
              {u?.shareholding?.length ? <><h4 className="text-sm font-semibold text-white pt-2">Shareholding</h4><Records rows={u.shareholding} /></> : null}
              {u?.objects?.length ? <><h4 className="text-sm font-semibold text-white pt-2">Use of proceeds (₹ crore)</h4><Records rows={u.objects} /></> : null}
              <p className="text-[11px] text-slate-500">Source: IPOWatch (unofficial). Confirm against the Red Herring Prospectus.</p>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
