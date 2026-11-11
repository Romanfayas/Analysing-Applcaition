'use client';

import { useState } from 'react';
import Link from '@/components/Link';
import useSWR from 'swr';
import { fetcher, send } from '@/lib/api/client';
import { useStocks } from '@/lib/api/hooks';
import WhatIfSimulator from '@/components/WhatIfSimulator';
import ResearchResults from '@/components/ResearchResults';
import { shariahLabel } from '@/lib/shariah';
import type { BacktestResult, ResearchGate } from '@/lib/api/types';
import Badge from '@/components/ui/Badge';
import { CheckCircleIcon, XCircleIcon, LockClosedIcon } from '@heroicons/react/24/outline';

interface Requirement {
  id: string;
  title: string;
  met: boolean;
  measured: string;
  needed: string;
}

/** The gate's conditions, each stated as what was measured against what is required. */
function requirements(g: ResearchGate): Requirement[] {
  const strategyOk = g.is_strategy_hash_valid === true;
  return [
    {
      id: 'prices',
      title: 'Daily price history certified',
      met: g.quarantined_bars_count === 0,
      measured: `${g.quarantined_bars_count.toLocaleString('en-IN')} stored daily bars are quarantined across ${g.equities_with_quarantined_bars} of ${g.instruments_with_bars} instruments`,
      needed: 'No quarantined bars: each price must be tied to an authenticated corporate-action adjustment',
    },
    {
      id: 'actions',
      title: 'Corporate-action evidence authenticated',
      met: g.unauthenticated_demoted_events === 0 && g.primary_verified_events > 0,
      measured: `${g.primary_verified_events} verified from primary filings, ${g.unauthenticated_demoted_events} not yet authenticated`,
      needed: 'Every split, bonus and demerger event verified from an exchange primary source',
    },
    {
      id: 'strategy',
      title: 'Strategy V2 parameters frozen',
      met: strategyOk,
      measured: g.canonical_strategy_hash
        ? `Hash ${g.canonical_strategy_hash.slice(0, 12)}… ${strategyOk ? 'matches' : 'does NOT match'} the pinned value`
        : 'Attestation unavailable',
      needed: 'Runtime parameters identical to the pinned hash, so results cannot be tuned after the fact',
    },
  ];
}

export default function BacktestingPage() {
  const { data: gate, error: gateError, isLoading, mutate } = useSWR<ResearchGate>('/backtesting/gate', fetcher);
  const { stocks } = useStocks();
  const [testSymbol, setTestSymbol] = useState<string>('TCS');
  const [results, setResults] = useState<BacktestResult | null>(null);
  const [loading, setLoading] = useState<'historical' | 'walk_forward' | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);

  const reqs = gate ? requirements(gate) : [];
  const open = gate?.status === 'RESEARCH_ELIGIBLE' && reqs.every((r) => r.met);
  const unmet = reqs.filter((r) => !r.met);

  const runBacktest = async (type: 'historical' | 'walk_forward') => {
    setLoading(type);
    setErrorMsg(null);
    try {
      const endpoint = type === 'walk_forward' ? '/backtesting/run_walk_forward' : '/backtesting/run';
      const out = await send<BacktestResult>('post', endpoint, { type });
      setResults(out);
      if (out.status === 'RESEARCH_BLOCKED') await mutate(); // the server's gate is the source of truth
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : 'The backtesting service did not respond.');
    } finally {
      setLoading(null);
    }
  };

  const hasResults = results && results.status !== 'RESEARCH_BLOCKED' && results.total_return_percent !== undefined;

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Backtesting</h1>
          <p className="text-sm text-slate-400 mt-1 max-w-2xl">
            Real results on NSE&apos;s own five-year price record, a per-stock test you can run yourself, and what the formal Strategy V2 certification still needs.
          </p>
        </div>
        {gate && (
          <Badge variant={open ? 'pass' : 'warning'}>{open ? 'Strategy V2: certified' : 'Strategy V2: certification pending'}</Badge>
        )}
      </div>

      <ResearchResults />

      {isLoading && <div className="glass-card p-6 text-sm text-slate-400">Checking data readiness…</div>}

      {gateError && !gate && (
        <div className="glass-card p-6 border-l-4 border-l-rose-500 text-sm text-rose-300 flex items-center justify-between gap-4">
          <span>Could not read the data-readiness check, so backtests are kept off. {gateError instanceof Error ? gateError.message : ''}</span>
          <button onClick={() => mutate()} className="px-3 py-1.5 rounded-lg bg-slate-800 text-white text-xs font-semibold">Retry</button>
        </div>
      )}

      {gate && (
        <div className={`glass-card p-6 space-y-5 border-l-4 ${open ? 'border-l-emerald-500' : 'border-l-amber-500'}`}>
          <div className="flex items-start gap-3">
            <LockClosedIcon className={`w-6 h-6 shrink-0 mt-0.5 ${open ? 'text-emerald-400' : 'text-amber-400'}`} />
            <div>
              <h2 className="text-lg font-semibold text-white">
                {open
                  ? 'Strategy V2: all certification requirements are met'
                  : `Strategy V2 certification: ${unmet.length} of ${reqs.length} requirements still to meet`}
              </h2>
              <p className="text-sm text-slate-400 mt-1 max-w-3xl">
                {open
                  ? 'Backtests can run on certified history.'
                  : 'This is a separate, stricter check for the frozen Strategy V2. It needs the vendor history and corporate actions signed off by a person, which software cannot do. The results above do not depend on it.'}
              </p>
            </div>
          </div>

          <ul className="divide-y divide-slate-800/70 rounded-xl border border-slate-800 bg-slate-900/40">
            {reqs.map((r) => (
              <li key={r.id} className="p-4 flex items-start gap-3">
                {r.met
                  ? <CheckCircleIcon className="w-5 h-5 text-emerald-400 shrink-0 mt-0.5" />
                  : <XCircleIcon className="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />}
                <div className="text-sm">
                  <p className="font-semibold text-white">{r.title}</p>
                  <p className="text-slate-300 mt-0.5"><span className="text-slate-500">Now: </span>{r.measured}</p>
                  {!r.met && <p className="text-slate-400 mt-0.5"><span className="text-slate-500">Required: </span>{r.needed}</p>}
                </div>
              </li>
            ))}
          </ul>

          {!open && (
            <p className="text-xs text-slate-500">
              The checks above are measured live from the database. Evidence status and which items need review are on the{' '}
              <Link href="/data-quality" className="text-blue-400 hover:text-blue-300">Data Quality</Link> page.
            </p>
          )}
        </div>
      )}

      <div className="glass-card p-6 space-y-4">
        <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div>
            <h3 className="text-white font-semibold">Run the certified Strategy V2 backtest</h3>
            <p className="text-xs text-slate-400 mt-1">
              {open ? 'Runs on certified history with the frozen Strategy V2 parameters.' : 'Available once every requirement above is met.'}
            </p>
          </div>
          <div className="flex gap-3 shrink-0">
            <button
              onClick={() => runBacktest('historical')}
              disabled={!open || loading !== null}
              title={open ? undefined : 'Locked until the data requirements are met'}
              className="px-5 py-2.5 bg-blue-600 hover:bg-blue-500 text-white rounded-xl text-sm font-medium transition-colors disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-blue-600"
            >
              {loading === 'historical' ? 'Running…' : 'Run historical backtest'}
            </button>
            <button
              onClick={() => runBacktest('walk_forward')}
              disabled={!open || loading !== null}
              title={open ? undefined : 'Locked until the data requirements are met'}
              className="px-5 py-2.5 bg-slate-800 hover:bg-slate-700 text-white border border-slate-700 rounded-xl text-sm font-medium transition-colors disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-slate-800"
            >
              {loading === 'walk_forward' ? 'Running…' : 'Run walk-forward'}
            </button>
          </div>
        </div>

      </div>

      {/* Works today: a hypothetical trade on stored prices, with real costs. Not a strategy backtest. */}
      <div className="space-y-3">
        <div className="flex flex-col sm:flex-row sm:items-end gap-3">
          <div>
            <h3 className="text-white font-semibold">Test a stock&apos;s past performance</h3>
            <p className="text-xs text-slate-400 mt-1 max-w-2xl">
              Pick any stock (including ones you added) and a past window. It prices a buy and a sell from stored prices and deducts delivery costs.
              It is a single hypothetical trade, not a strategy backtest, and it works while the certification above is pending.
            </p>
          </div>
          <select value={testSymbol} onChange={(e) => setTestSymbol(e.target.value)} aria-label="Stock to test"
            className="sm:ml-auto px-3 py-2 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm max-w-xs">
            {stocks.map((st) => (
              <option key={st.symbol} value={st.symbol}>{st.symbol} — {st.name} · {shariahLabel(st.shariah).text}</option>
            ))}
          </select>
        </div>
        <WhatIfSimulator symbol={testSymbol} />
      </div>

      {errorMsg && (
        <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-sm">{errorMsg}</div>
      )}

      {hasResults && results && (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
          <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Total Return</span>
            <p className={`text-2xl font-bold font-mono mt-2 ${Number(results.total_return_percent) >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
              {Number(results.total_return_percent) >= 0 ? '+' : ''}{Number(results.total_return_percent).toFixed(2)}%
            </p>
          </div>
          <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Max Drawdown</span>
            <p className="text-2xl font-bold font-mono text-rose-400 mt-2">-{Math.abs(Number(results.max_drawdown_percent || 0)).toFixed(2)}%</p>
          </div>
          <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Win Rate</span>
            <p className="text-2xl font-bold font-mono text-white mt-2">{Number(results.win_rate_percent || 0).toFixed(1)}%</p>
          </div>
          <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
            <span className="text-xs font-semibold uppercase tracking-wider text-slate-400">Total Trades</span>
            <p className="text-2xl font-bold font-mono text-blue-400 mt-2">{results.total_trades || 0}</p>
          </div>
        </div>
      )}
    </div>
  );
}
