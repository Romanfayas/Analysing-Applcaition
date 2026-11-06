'use client';

import { useState } from 'react';
import Link from 'next/link';
import useSWR from 'swr';
import { fetcher, send } from '@/lib/api/client';
import type { AddStockResponse, OnboardStatusResponse } from '@/lib/api/types';

const STEP_LABEL: Record<string, string> = {
  daily_history: 'Daily price history (2 years)',
  intraday_1m: 'Recent 1-minute prices',
  intraday_5m: '5-minute prices (up to 60 sessions)',
  shariah_screening: 'Shariah screening from reported financials',
  timeout: 'Loading',
};

/** Lets the owner follow any NSE stock (e.g. PCJEWELLER): the server validates it, loads its data and screens it. */
export default function AddStockPanel({ onAdded }: { onAdded?: () => void }) {
  const [input, setInput] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [tracking, setTracking] = useState<string | null>(null);

  const { data: progress } = useSWR<OnboardStatusResponse>(
    tracking ? `/stocks/custom/${tracking}/status` : null,
    fetcher,
    { refreshInterval: (d) => (d?.status?.state === 'RUNNING' || !d?.status ? 2000 : 0), revalidateOnFocus: false },
  );
  const state = progress?.status?.state;

  const add = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setNote(null);
    if (!input.trim()) return;
    setBusy(true);
    try {
      const out = await send<AddStockResponse>('post', '/stocks/custom', { symbol: input.trim() });
      if (out.already_tracked) {
        setNote(out.message ?? `${out.symbol} is already tracked.`);
        setTracking(null);
      } else {
        setTracking(out.symbol);
        setInput('');
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Could not add the stock.');
    } finally {
      setBusy(false);
    }
  };

  // Tell the list to reload once loading has finished.
  const [notified, setNotified] = useState<string | null>(null);
  if (tracking && state && state !== 'RUNNING' && notified !== tracking) {
    setNotified(tracking);
    onAdded?.();
  }

  return (
    <div className="glass-card p-5 rounded-2xl border border-slate-800/80 bg-slate-900/60 space-y-3">
      <div>
        <h2 className="text-sm font-semibold text-white">Add a stock</h2>
        <p className="text-xs text-slate-400 mt-0.5">
          Type an NSE ticker that is not listed here (for example <span className="font-mono text-slate-300">PCJEWELLER</span>). The platform checks it
          exists, loads its price history, and screens it for Shariah compliance, so you can study it and paper-trade it.
        </p>
      </div>
      <form onSubmit={add} className="flex flex-col sm:flex-row gap-3">
        <input
          value={input}
          onChange={(e) => setInput(e.target.value.toUpperCase())}
          placeholder="NSE ticker, e.g. PCJEWELLER"
          aria-label="NSE ticker to add"
          maxLength={24}
          className="flex-1 px-3.5 py-2 rounded-xl bg-slate-900 border border-slate-700 text-sm text-slate-100 placeholder-slate-500 font-mono focus:outline-none focus:border-blue-500/60"
        />
        <button type="submit" disabled={busy || !input.trim() || state === 'RUNNING'}
          className="px-5 py-2 rounded-xl bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium disabled:opacity-50">
          {busy ? 'Checking…' : 'Add stock'}
        </button>
      </form>
      {error && <p className="text-sm text-amber-300">{error}</p>}
      {note && <p className="text-sm text-slate-300">{note}</p>}

      {tracking && (
        <div className="rounded-xl bg-slate-950/50 border border-slate-800 p-3 text-xs space-y-1.5">
          <p className="text-slate-200 font-semibold">
            {tracking}: {state === 'RUNNING' || !state ? 'loading data…' : state === 'DONE' ? 'ready' : 'loaded with problems'}
          </p>
          {(progress?.status?.steps ?? []).map((s) => (
            <p key={s.name} className={s.ok ? 'text-slate-400' : 'text-amber-300'}>
              {s.ok ? '✓' : '✗'} {STEP_LABEL[s.name] ?? s.name}: {s.detail}
            </p>
          ))}
          {(state === 'DONE' || state === 'FAILED') && (
            <p className="pt-1 flex gap-4">
              <Link href={`/stocks/${tracking}`} className="text-blue-400 hover:text-blue-300">Open {tracking}</Link>
              <Link href={`/paper-trading?symbol=${tracking}`} className="text-blue-400 hover:text-blue-300">Paper-trade it</Link>
            </p>
          )}
        </div>
      )}
    </div>
  );
}
