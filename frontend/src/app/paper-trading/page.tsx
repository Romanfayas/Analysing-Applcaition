'use client';

import { Suspense, useState } from 'react';
import Link from '@/components/Link';
import { useSearchParams } from 'next/navigation';
import useSWR from 'swr';
import { send, fetcher } from '@/lib/api/client';
import type { PriceAtResult } from '@/lib/api/types';
import WhatIfSimulator from '@/components/WhatIfSimulator';
import PerformanceReportCard from '@/components/PerformanceReportCard';
import { SOURCE_LABEL, nowIstInput, asIst, money } from '@/lib/paperTime';
import { shariahLabel } from '@/lib/shariah';
import { useStocks, usePortfolio, usePaperTrades } from '@/lib/api/hooks';
import { calculateDeliveryCosts } from '@/lib/costs';
import Card, { CardHeader } from '@/components/ui/Card';
import { 
  WalletIcon, 
  ArrowTrendingUpIcon, 
  ShieldCheckIcon, 
  BuildingOfficeIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon
} from '@heroicons/react/24/outline';

export default function PaperTradingPage() {
  // useSearchParams needs a Suspense boundary so the page can still be prerendered.
  return (
    <Suspense fallback={<div className="text-sm text-slate-400">Loading…</div>}>
      <PaperTradingTerminal />
    </Suspense>
  );
}

function PaperTradingTerminal() {
  const preselected = useSearchParams().get('symbol')?.toUpperCase();
  const { portfolio, positions, mutate: mutatePortfolio } = usePortfolio();
  const { stocks } = useStocks();
  const { data: rules } = useSWR<{ limits: { max_position_pct: number; max_daily_loss_pct: number; max_orders_per_day: number }; orders_today: number }>(
    '/paper-trading/risk-rules', fetcher, { revalidateOnFocus: false });
  const { trades, mutate: mutateTrades } = usePaperTrades();

  // Form state
  const [selectedSymbol, setSelectedSymbol] = useState<string>(preselected || 'TCS');
  const [tradeType, setTradeType] = useState<'BUY' | 'SELL'>('BUY');
  const [shares, setShares] = useState<number>(10);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // Execution time: the latest price, or a past moment replayed from stored history (no lookahead).
  const [timeMode, setTimeMode] = useState<'now' | 'past'>('now');
  const [maxWhen] = useState<string>(nowIstInput);
  const [when, setWhen] = useState<string>('');
  const pastKey = timeMode === 'past' && when
    ? `/paper-trading/price-at?symbol=${encodeURIComponent(selectedSymbol)}&at=${encodeURIComponent(asIst(when))}`
    : null;
  const { data: pastPrice, error: pastError, isLoading: pastLoading } = useSWR<PriceAtResult>(pastKey, fetcher, {
    shouldRetryOnError: false,
    revalidateOnFocus: false,
  });


  // Selected stock details
  const activeStock = stocks.find((s) => s.symbol === selectedSymbol);
  const latest = activeStock?.price ? Number(activeStock.price) : 0;
  const replaying = timeMode === 'past';
  // In replay mode the preview price is the one the server will use; with none resolved, nothing is estimated.
  const cmp = replaying ? (pastPrice?.price.price ?? 0) : latest;
  const isShariahPass = activeStock?.shariah === 'PASS';
  const grossValue = shares * cmp;
  const costBreakdown = calculateDeliveryCosts(tradeType, grossValue);
  const fees = costBreakdown.totalFees;
  const totalCost = tradeType === 'BUY' ? grossValue + fees : Math.max(0, grossValue - fees);

  // Undefined until the real ledger has loaded; never a made-up starting balance.
  const cashBalance = portfolio?.cash_balance ?? 0;
  const investedValue = portfolio?.invested_value ?? 0;
  const totalEquity = portfolio?.total_equity ?? 0;
  const totalPnl = portfolio?.unrealized_pnl ?? 0;
  const totalPnlPct = portfolio?.unrealized_pnl_pct ?? 0;

  // Execute paper trade
  const handleExecuteTrade = async (e: React.FormEvent) => {
    e.preventDefault();
    if (shares <= 0) {
      setFeedback({ type: 'error', message: 'Shares must be greater than 0.' });
      return;
    }
    if (replaying && !pastPrice) {
      setFeedback({ type: 'error', message: 'Pick a past time that has a stored price before placing a replay order.' });
      return;
    }
    if (tradeType === 'BUY' && totalCost > cashBalance) {
      setFeedback({ type: 'error', message: `Insufficient cash balance. Required: ₹${totalCost.toFixed(2)}, Available: ₹${cashBalance.toFixed(2)}` });
      return;
    }

    setIsSubmitting(true);
    setFeedback(null);

    try {
      // The server fills at its own stored market price; the client never supplies one.
      const result = await send<{ trade: { price: number; price_source?: string | null } }>('post', '/portfolio/trade', {
        symbol: selectedSymbol,
        trade_type: tradeType,
        shares,
        ...(replaying ? { execute_at: asIst(when) } : {}),
      });

      const basis = SOURCE_LABEL[result.trade.price_source ?? ''] ?? 'stored price';
      setFeedback({
        type: 'success',
        message: replaying
          ? `Replay ${tradeType} of ${shares} shares of ${selectedSymbol} as of ${when.replace('T', ' ')} IST filled at ₹${result.trade.price.toFixed(2)} (${basis}).`
          : `Paper ${tradeType} of ${shares} shares of ${selectedSymbol} filled at ₹${result.trade.price.toFixed(2)}.`,
      });
      await Promise.all([mutateTrades(), mutatePortfolio()]);
    } catch (err) {
      setFeedback({ type: 'error', message: err instanceof Error ? err.message : 'Execution error.' });
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Paper Trading Terminal</h1>
          <p className="text-slate-400 mt-1">
            Simulated order execution with real-time portfolio accounting. Each trade records the instrument&apos;s Shariah status; paper BUYs are not blocked by it.
          </p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-slate-800/80 border border-slate-700/60 text-xs text-slate-300">
          <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>PAPER_TRADING_ENABLED=true · LIVE_ORDER_EXECUTION=false</span>
        </div>
      </div>

      {/* Accounting Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Total Equity</span>
            <WalletIcon className="w-5 h-5 text-blue-400" />
          </div>
          <p className="text-2xl font-bold font-mono text-white mt-2">
            ₹{totalEquity.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </p>
          <p className="text-xs text-slate-500 mt-1">
            Cash: ₹{cashBalance.toLocaleString('en-IN', { minimumFractionDigits: 2 })}
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Unrealized P&L</span>
            <ArrowTrendingUpIcon className="w-5 h-5 text-emerald-400" />
          </div>
          <p className={`text-2xl font-bold font-mono mt-2 ${totalPnl >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
            {totalPnl >= 0 ? '+' : ''}₹{totalPnl.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
          </p>
          <p className="text-xs text-slate-500 mt-1">
            {totalPnl >= 0 ? '+' : ''}{totalPnlPct.toFixed(2)}% paper return
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Active Positions</span>
            <BuildingOfficeIcon className="w-5 h-5 text-amber-400" />
          </div>
          <p className="text-2xl font-bold font-mono text-white mt-2">{positions.length}</p>
          <p className="text-xs text-slate-500 mt-1">
            Invested: ₹{investedValue.toLocaleString('en-IN', { minimumFractionDigits: 2 })}
          </p>
        </div>

        <div className="p-5 rounded-2xl bg-slate-900/60 border border-slate-800">
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Shariah Compliance</span>
            <ShieldCheckIcon className="w-5 h-5 text-emerald-400" />
          </div>
          <p className="text-2xl font-bold text-white mt-2">
            {positions.length === 0 ? '—' : `${positions.filter((p) => stocks.find((s) => s.symbol === p.symbol)?.shariah === 'PASS').length}/${positions.length} certified`}
          </p>
          <p className="text-xs text-slate-500 mt-1">Paper BUYs are not Shariah-gated · status recorded per trade · spot only</p>
        </div>
      </div>

      {/* Trade Execution Form */}
      <div className="glass-card p-6">
        <h2 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
          <span>Place Paper Order</span>
          <span className="text-xs text-slate-400 font-normal">(Instant Simulated Execution)</span>
        </h2>

        {feedback && (
          <div className={`p-4 rounded-xl mb-4 text-sm flex items-start gap-2.5 ${
            feedback.type === 'success' 
              ? 'bg-emerald-500/10 border border-emerald-500/30 text-emerald-300' 
              : 'bg-rose-500/10 border border-rose-500/30 text-rose-300'
          }`}>
            {feedback.type === 'success' ? (
              <CheckCircleIcon className="w-5 h-5 shrink-0 text-emerald-400" />
            ) : (
              <ExclamationTriangleIcon className="w-5 h-5 shrink-0 text-rose-400" />
            )}
            <span>{feedback.message}</span>
          </div>
        )}

        <form onSubmit={handleExecuteTrade} className="grid grid-cols-1 md:grid-cols-4 gap-4 items-end">
          <div>
            <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Select Security</label>
            <select
              value={selectedSymbol}
              onChange={(e) => setSelectedSymbol(e.target.value)}
              className="w-full mt-1.5 px-3 py-2.5 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none"
            >
              {stocks.map((s) => (
                <option key={s.symbol} value={s.symbol}>
                  {s.symbol} — {s.name} · {shariahLabel(s.shariah).text}{s.freshness === 'STALE' || s.freshness === 'UNAVAILABLE' ? ' · no current price' : ''}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Order Action</label>
            <div className="grid grid-cols-2 gap-2 mt-1.5">
              <button
                type="button"
                onClick={() => setTradeType('BUY')}
                className={`py-2 rounded-xl text-xs font-bold transition-all ${
                  tradeType === 'BUY'
                    ? 'bg-emerald-500 text-white shadow-[0_0_12px_rgba(16,185,129,0.3)]'
                    : 'bg-slate-800 text-slate-400 hover:text-white'
                }`}
              >
                BUY
              </button>
              <button
                type="button"
                onClick={() => setTradeType('SELL')}
                className={`py-2 rounded-xl text-xs font-bold transition-all ${
                  tradeType === 'SELL'
                    ? 'bg-rose-500 text-white shadow-[0_0_12px_rgba(244,63,94,0.3)]'
                    : 'bg-slate-800 text-slate-400 hover:text-white'
                }`}
              >
                SELL
              </button>
            </div>
          </div>

          <div>
            <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Quantity (Shares)</label>
            <input
              type="number"
              min="1"
              value={shares}
              onChange={(e) => setShares(Math.max(1, parseInt(e.target.value) || 1))}
              className="w-full mt-1.5 px-3 py-2 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none"
            />
          </div>

          <div>
            <button
              type="submit"
              disabled={isSubmitting}
              className={`w-full py-2.5 rounded-xl font-bold text-sm transition-all shadow-lg ${
                tradeType === 'BUY'
                  ? 'bg-emerald-500 hover:bg-emerald-400 text-white shadow-emerald-500/20'
                  : 'bg-rose-500 hover:bg-rose-400 text-white shadow-rose-500/20'
              } disabled:opacity-50`}
            >
              {isSubmitting ? 'Executing...' : tradeType === 'BUY' ? 'Execute Paper Buy' : 'Execute Paper Sell'}
            </button>
          </div>
        </form>

        {/* Execution time */}
        <div className="mt-4 pt-4 border-t border-slate-800 space-y-3">
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Execution time</span>
            <div className="inline-flex rounded-xl bg-slate-800 p-1 text-xs font-semibold">
              <button type="button" onClick={() => setTimeMode('now')}
                className={`px-3 py-1.5 rounded-lg ${timeMode === 'now' ? 'bg-blue-500 text-white' : 'text-slate-400 hover:text-white'}`}>
                Now (latest price)
              </button>
              <button type="button" onClick={() => { setTimeMode('past'); if (!when) setWhen(maxWhen); }}
                className={`px-3 py-1.5 rounded-lg ${timeMode === 'past' ? 'bg-blue-500 text-white' : 'text-slate-400 hover:text-white'}`}>
                Past date &amp; time (replay)
              </button>
            </div>
            {replaying && (
              <input type="datetime-local" value={when} max={maxWhen} onChange={(e) => setWhen(e.target.value)}
                aria-label="Replay time (IST)"
                className="px-3 py-1.5 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none" />
            )}
            {replaying && <span className="text-xs text-slate-500">Indian market time (IST)</span>}
          </div>
          {replaying && (
            <div className="text-xs">
              {pastLoading && <span className="text-slate-400">Looking up the stored price…</span>}
              {pastError && (
                <span className="text-amber-300">
                  {pastError instanceof Error ? pastError.message : 'No stored price for that time.'} Pick a trading day between 09:15 and 15:30, or any time after the close for an official daily close.
                </span>
              )}
              {pastPrice && (
                <span className="text-slate-300">
                  Fill price <strong className="text-white font-mono">{money(pastPrice.price.price)}</strong>
                  {' · '}{SOURCE_LABEL[pastPrice.price.source] ?? pastPrice.price.source}
                  {pastPrice.price.note ? ` — ${pastPrice.price.note}` : ''}. Only prices that were already final at that time are used.
                </span>
              )}
              <p className="text-slate-500 mt-1">
                A past order goes into its place in your history and the ledger is recalculated. It is refused only if it could not have happened, such as selling shares you did not hold yet. Intraday prices are stored from 2026-08-06; earlier dates use a daily close.
              </p>
            </div>
          )}
        </div>

        {rules && (
          <p className="mt-3 text-[11px] text-slate-500">
            Account risk rules for buys:{' '}
            {rules.limits.max_position_pct > 0 && <>one stock at most {rules.limits.max_position_pct}% of your account · </>}
            {rules.limits.max_daily_loss_pct > 0 && <>buying pauses after a {rules.limits.max_daily_loss_pct}% realised loss in a day · </>}
            {rules.limits.max_orders_per_day > 0 && <>up to {rules.limits.max_orders_per_day} orders a day ({rules.orders_today} used) · </>}
            selling is never blocked.
          </p>
        )}

        {/* Order Preview Info */}
        <div className="mt-4 pt-4 border-t border-slate-800 flex flex-col gap-2 text-xs">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex flex-wrap items-center gap-4 text-slate-400">
              <span>{replaying ? 'Replay price' : 'Market CMP'}: <strong className="text-white font-mono">{cmp > 0 ? `₹${cmp.toFixed(2)}` : '—'}</strong></span>
              <span>Turnover: <strong className="text-white font-mono">₹{grossValue.toFixed(2)}</strong></span>
              <span>Statutory Levies: <strong className="text-amber-400 font-mono">₹{fees.toFixed(2)}</strong></span>
              <span>{tradeType === 'BUY' ? 'Total Cash Required' : 'Net Cash Realized'}: <strong className="text-emerald-400 font-mono">₹{totalCost.toFixed(2)}</strong></span>
            </div>
            <div>
              <span className={`px-2.5 py-1 rounded-full text-xs font-semibold ${
                isShariahPass ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30'
                : activeStock?.shariah === 'FAIL' ? 'bg-rose-500/20 text-rose-400 border border-rose-500/30'
                : 'bg-amber-500/20 text-amber-300 border border-amber-500/30'
              }`}>
                Shariah: {shariahLabel(activeStock?.shariah).text}
              </span>
            </div>
          </div>
          <div className="text-[11px] text-slate-500 flex flex-wrap gap-x-4 gap-y-1 pt-1 border-t border-slate-800/60 font-mono">
            <span>STT: ₹{costBreakdown.stt.toFixed(2)}</span>
            {tradeType === 'BUY' && <span>Stamp: ₹{costBreakdown.stampDuty.toFixed(2)}</span>}
            <span>Exch: ₹{costBreakdown.exchangeCharges.toFixed(2)}</span>
            <span>SEBI: ₹{costBreakdown.sebiCharges.toFixed(2)}</span>
            {tradeType === 'SELL' && <span>DP: ₹{costBreakdown.dpCharges.toFixed(2)}</span>}
            <span>GST: ₹{costBreakdown.gst.toFixed(2)}</span>
            <span className="text-slate-400 font-sans">({costBreakdown.modelVersion})</span>
          </div>
        </div>
      </div>

      <WhatIfSimulator symbol={selectedSymbol} />

      <PerformanceReportCard />

      {/* Active Paper Positions */}
      <Card>
        <CardHeader title="Open Paper Positions" subtitle="Real-time marked-to-market holdings from PostgreSQL" />
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-slate-800 text-xs font-semibold uppercase text-slate-400 tracking-wider">
                <th className="p-4">Symbol</th>
                <th className="p-4 text-right">Shares</th>
                <th className="p-4 text-right">Avg Cost</th>
                <th className="p-4 text-right">CMP</th>
                <th className="p-4 text-right">Position Value</th>
                <th className="p-4 text-right">Unrealized P&L</th>
                <th className="p-4 text-center">Action</th>
              </tr>
            </thead>
            <tbody className="text-sm divide-y divide-slate-800/60">
              {positions.length === 0 ? (
                <tr>
                  <td colSpan={7} className="text-center py-8 text-slate-500">
                    No active paper trading positions. Submit an order above to begin simulated tracking.
                  </td>
                </tr>
              ) : (
                positions.map((pos) => {
                  const pnl = Number(pos.unrealized_pnl || 0);
                  const pnlPct = pos.avg_cost_basis > 0 ? ((Number(pos.last_price || pos.avg_cost_basis) - pos.avg_cost_basis) / pos.avg_cost_basis) * 100 : 0;
                  const posVal = Number(pos.shares || 0) * Number(pos.last_price || pos.avg_cost_basis);
                  return (
                    <tr key={pos.id} className="hover:bg-slate-800/30 transition-colors">
                      <td className="p-4 font-bold text-white">
                        <Link href={`/stocks/${pos.symbol}`} className="hover:text-blue-400 transition">
                          {pos.symbol}
                        </Link>
                      </td>
                      <td className="p-4 text-right font-mono text-slate-300">{pos.shares}</td>
                      <td className="p-4 text-right font-mono text-slate-300">₹{Number(pos.avg_cost_basis).toFixed(2)}</td>
                      <td className="p-4 text-right font-mono text-white font-semibold">₹{Number(pos.last_price || pos.avg_cost_basis).toFixed(2)}</td>
                      <td className="p-4 text-right font-mono text-white font-semibold">₹{posVal.toLocaleString('en-IN', { maximumFractionDigits: 2 })}</td>
                      <td className={`p-4 text-right font-mono font-bold ${pnl >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                        {pnl >= 0 ? '+' : ''}₹{pnl.toFixed(2)} ({pnl >= 0 ? '+' : ''}{pnlPct.toFixed(2)}%)
                      </td>
                      <td className="p-4 text-center">
                        <button
                          onClick={() => {
                            setSelectedSymbol(pos.symbol);
                            setTradeType('SELL');
                            setShares(pos.shares);
                          }}
                          className="px-3 py-1 rounded-lg bg-rose-500/20 text-rose-300 hover:bg-rose-500/30 text-xs font-semibold border border-rose-500/30 transition"
                        >
                          Quick Sell
                        </button>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Trade Execution Audit Log */}
      <Card>
        <CardHeader title="Paper Trade Execution Log" subtitle="Audit trail of simulated orders and fills" />
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-slate-800 text-xs font-semibold uppercase text-slate-400 tracking-wider">
                <th className="p-4">Time</th>
                <th className="p-4">Symbol</th>
                <th className="p-4">Side</th>
                <th className="p-4 text-right">Shares</th>
                <th className="p-4 text-right">Execution Price</th>
                <th className="p-4">Price basis</th>
                <th className="p-4 text-right">Fees Paid</th>
                <th className="p-4 text-right">Realized P&L</th>
              </tr>
            </thead>
            <tbody className="text-sm divide-y divide-slate-800/60">
              {trades.length === 0 ? (
                <tr>
                  <td colSpan={8} className="text-center py-8 text-slate-500">
                    No execution records in paper trade ledger yet.
                  </td>
                </tr>
              ) : (
                trades.map((t) => (
                  <tr key={t.id} className="hover:bg-slate-800/30 transition-colors">
                    <td className="p-4 text-xs font-mono text-slate-400">{new Date(t.trade_date).toLocaleString('en-IN')}</td>
                    <td className="p-4 font-bold text-white">
                      {t.symbol}
                      {t.shariah_status_at_trade && t.shariah_status_at_trade !== 'PASS' && (
                        <span className="block text-[10px] font-normal text-amber-300">
                          Shariah at trade: {t.shariah_status_at_trade === 'FAIL' ? 'FAIL' : 'not evaluated'}
                        </span>
                      )}
                    </td>
                    <td className="p-4">
                      <span className={`px-2 py-0.5 rounded text-xs font-bold ${
                        t.trade_type === 'BUY' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-rose-500/20 text-rose-400'
                      }`}>
                        {t.trade_type}
                      </span>
                    </td>
                    <td className="p-4 text-right font-mono text-slate-300">{t.shares}</td>
                    <td className="p-4 text-right font-mono text-white">₹{Number(t.price).toFixed(2)}</td>
                    <td className="p-4 text-xs text-slate-400">
                      {SOURCE_LABEL[t.price_source ?? ''] ?? (t.price_source || 'not recorded')}
                      {t.is_backdated && <span className="ml-1.5 px-1.5 py-0.5 rounded bg-blue-500/20 text-blue-300 font-semibold">replay</span>}
                    </td>
                    <td className="p-4 text-right font-mono text-slate-400">₹{Number(t.fees_paid || 0).toFixed(2)}</td>
                    <td className="p-4 text-right font-mono">
                      {t.realized_pnl !== null && t.realized_pnl !== undefined ? (
                        <span className={Number(t.realized_pnl) >= 0 ? 'text-emerald-400 font-bold' : 'text-rose-400 font-bold'}>
                          {Number(t.realized_pnl) >= 0 ? '+' : ''}₹{Number(t.realized_pnl).toFixed(2)}
                        </span>
                      ) : (
                        <span className="text-slate-600">—</span>
                      )}
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
