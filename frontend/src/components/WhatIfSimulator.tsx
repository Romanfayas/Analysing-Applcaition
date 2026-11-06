'use client';

import { useState } from 'react';
import { send } from '@/lib/api/client';
import type { SimulationResult } from '@/lib/api/types';
import Card, { CardHeader } from '@/components/ui/Card';
import { SOURCE_LABEL, nowIstInput, asIst, money } from '@/lib/paperTime';

/** Read-only "what if I had bought at one time and sold at another" on stored prices, with real delivery costs. */
export default function WhatIfSimulator({ symbol }: { symbol: string }) {
  const [maxWhen] = useState<string>(nowIstInput);
  const [simFrom, setSimFrom] = useState<string>('');
  const [simTo, setSimTo] = useState<string>('');
  const [simShares, setSimShares] = useState<number>(10);
  const [product, setProduct] = useState<'DELIVERY' | 'INTRADAY'>('DELIVERY');
  const [stopPct, setStopPct] = useState<string>('');
  const [targetPct, setTargetPct] = useState<string>('');
  const [trailPct, setTrailPct] = useState<string>('');
  const [simBusy, setSimBusy] = useState<boolean>(false);
  const [simError, setSimError] = useState<string | null>(null);
  const [sim, setSim] = useState<SimulationResult | null>(null);

  const runSimulation = async (e: React.FormEvent) => {
    e.preventDefault();
    setSimError(null);
    setSim(null);
    if (!simFrom) {
      setSimError('Choose a start time.');
      return;
    }
    setSimBusy(true);
    try {
      setSim(await send<SimulationResult>('post', '/paper-trading/simulate', {
        symbol,
        shares: simShares,
        from: asIst(simFrom),
        product,
        ...(simTo ? { to: asIst(simTo) } : {}),
        ...(Number(stopPct) > 0 ? { stop_loss_pct: Number(stopPct) } : {}),
        ...(Number(targetPct) > 0 ? { target_pct: Number(targetPct) } : {}),
        ...(Number(trailPct) > 0 ? { trailing_stop_pct: Number(trailPct) } : {}),
      }));
    } catch (err) {
      setSimError(err instanceof Error ? err.message : 'Simulation failed.');
    } finally {
      setSimBusy(false);
    }
  };

  return (
      <Card>
        <CardHeader title="What-if Simulator" subtitle={`Buy ${symbol} at one time, sell at another — with real delivery costs. Read-only: nothing is added to your ledger.`} />
        <form onSubmit={runSimulation} className="grid grid-cols-1 md:grid-cols-4 gap-4 items-end p-4">
          <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">From (IST)
            <input type="datetime-local" value={simFrom} max={maxWhen} onChange={(e) => setSimFrom(e.target.value)}
              className="w-full mt-1.5 px-3 py-2 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm normal-case" />
          </label>
          <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">To (IST, blank = latest price)
            <input type="datetime-local" value={simTo} max={maxWhen} onChange={(e) => setSimTo(e.target.value)}
              className="w-full mt-1.5 px-3 py-2 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm normal-case" />
          </label>
          <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Shares
            <input type="number" min="1" value={simShares} onChange={(e) => setSimShares(Math.max(1, parseInt(e.target.value) || 1))}
              className="w-full mt-1.5 px-3 py-2 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm" />
          </label>
          <button type="submit" disabled={simBusy}
            className="py-2.5 rounded-xl font-bold text-sm bg-blue-500 hover:bg-blue-400 text-white disabled:opacity-50">
            {simBusy ? 'Running…' : 'Run what-if'}
          </button>
        </form>
        <div className="px-4 pb-4 grid grid-cols-2 md:grid-cols-4 gap-4 -mt-1">
          <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Trade type
            <select value={product} onChange={(e) => setProduct(e.target.value as 'DELIVERY' | 'INTRADAY')}
              className="w-full mt-1.5 px-3 py-2 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm normal-case">
              <option value="DELIVERY">Delivery (hold overnight)</option>
              <option value="INTRADAY">Intraday (same session)</option>
            </select>
          </label>
          {([['Stop-loss %', stopPct, setStopPct], ['Target %', targetPct, setTargetPct], ['Trailing stop %', trailPct, setTrailPct]] as const).map(([label, val, set]) => (
            <label key={label} className="text-xs font-semibold uppercase text-slate-400 tracking-wider">{label} <span className="font-normal normal-case text-slate-500">(optional)</span>
              <input type="number" min="0" max="99" step="0.5" value={val} onChange={(e) => set(e.target.value)} placeholder="off"
                className="w-full mt-1.5 px-3 py-2 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm" />
            </label>
          ))}
        </div>
        {simError && <p className="px-4 pb-4 text-sm text-amber-300">{simError}</p>}
        {sim && (
          <div className="px-4 pb-4 space-y-3 text-sm">
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
              <div className="rounded-xl bg-slate-900/60 border border-slate-800 p-3">
                <p className="text-xs text-slate-500">Entry · {SOURCE_LABEL[sim.entry.source] ?? sim.entry.source}</p>
                <p className="font-mono text-white">{money(sim.entry.price)}</p>
              </div>
              <div className="rounded-xl bg-slate-900/60 border border-slate-800 p-3">
                <p className="text-xs text-slate-500">Exit · {SOURCE_LABEL[sim.exit.source] ?? sim.exit.source}</p>
                <p className="font-mono text-white">{money(sim.exit.price)}</p>
              </div>
              <div className="rounded-xl bg-slate-900/60 border border-slate-800 p-3">
                <p className="text-xs text-slate-500">Costs (buy + sell)</p>
                <p className="font-mono text-amber-300">{money(sim.buy_costs.total_fees + sim.sell_costs.total_fees)}</p>
              </div>
              <div className="rounded-xl bg-slate-900/60 border border-slate-800 p-3">
                <p className="text-xs text-slate-500">Net result</p>
                <p className={`font-mono font-bold ${sim.net_pnl >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                  {sim.net_pnl >= 0 ? '+' : ''}{money(sim.net_pnl)} ({sim.net_return_pct >= 0 ? '+' : ''}{sim.net_return_pct.toFixed(2)}%)
                </p>
              </div>
            </div>
            {sim.exit_reason !== 'END_OF_WINDOW' && (
              <div className="rounded-xl border border-blue-500/30 bg-blue-500/10 p-3 text-xs text-blue-100">
                <strong>{sim.exit_reason.replaceAll('_', ' ')}</strong> filled at {money(sim.exit.price)}
                {sim.protective_orders.outcome?.gap ? ' (the price gapped through the level, so this is the open)' : ''}.
                {sim.protective_orders.without_protection && (
                  <> Without it you would have held to the end: <strong>{sim.protective_orders.without_protection.net_pnl >= 0 ? '+' : ''}{money(sim.protective_orders.without_protection.net_pnl)}
                  ({sim.protective_orders.without_protection.net_return_pct.toFixed(2)}%)</strong>.</>
                )}
              </div>
            )}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
              <div className="rounded-xl bg-slate-900/60 border border-slate-800 p-3 space-y-0.5">
                <p className="text-slate-300 font-semibold">Versus NIFTY 50</p>
                {sim.benchmark.return_pct != null ? (
                  <>
                    <p className="text-slate-400">Index {sim.benchmark.return_pct >= 0 ? '+' : ''}{sim.benchmark.return_pct.toFixed(2)}% ({sim.benchmark.from_date} → {sim.benchmark.to_date})</p>
                    <p className={sim.benchmark.excess_return_pct != null && sim.benchmark.excess_return_pct >= 0 ? 'text-emerald-400' : 'text-rose-400'}>
                      This trade {sim.benchmark.excess_return_pct != null && sim.benchmark.excess_return_pct >= 0 ? 'beat' : 'lagged'} the index by {Math.abs(sim.benchmark.excess_return_pct ?? 0).toFixed(2)} points
                    </p>
                  </>
                ) : <p className="text-slate-500">{sim.benchmark.note}</p>}
              </div>
              <div className="rounded-xl bg-slate-900/60 border border-slate-800 p-3 space-y-0.5">
                <p className="text-slate-300 font-semibold">Liquidity and slippage</p>
                <p className="text-slate-400">{sim.slippage.total_bps_each_side.toFixed(1)} bps each side ({sim.slippage.base_bps} base
                  {sim.slippage.liquidity ? ` + ${sim.slippage.liquidity.impact_bps.toFixed(1)} market impact` : ''})</p>
                {sim.slippage.liquidity ? (
                  <p className="text-slate-500">Order is {sim.slippage.liquidity.participation_pct.toFixed(3)}% of the average daily volume
                    ({Math.round(sim.slippage.liquidity.adv_shares).toLocaleString('en-IN')} shares, {sim.slippage.liquidity.sessions} sessions)</p>
                ) : <p className="text-slate-500">No exchange volume history stored for this stock.</p>}
              </div>
            </div>
            {sim.corporate_actions.length > 0 && (
              <ul className="text-amber-300 text-xs list-disc pl-5">
                {sim.corporate_actions.map((a) => <li key={`${a.type}-${a.ex_date}`}>{a.type} with ex-date {a.ex_date}: {a.subject}</li>)}
              </ul>
            )}
            <ul className="text-xs text-slate-400 list-disc pl-5 space-y-0.5">
              {sim.warnings.map((w) => <li key={w}>{w}</li>)}
            </ul>
            <p className="text-xs text-slate-500">{sim.slippage.assumption} {sim.disclaimer}</p>
          </div>
        )}
      </Card>

  );
}
