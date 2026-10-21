'use client';

import { useState } from 'react';
import useSWR from 'swr';
import Link from 'next/link';
import { fetcher } from '@/lib/api/client';
import { useStocks, usePortfolio } from '@/lib/api/hooks';
import { calculateDeliveryCosts } from '@/lib/costs';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { 
  WalletIcon, 
  ArrowTrendingUpIcon, 
  ShieldCheckIcon, 
  BuildingOfficeIcon,
  CheckCircleIcon,
  ExclamationTriangleIcon,
  ArrowPathIcon
} from '@heroicons/react/24/outline';

export default function PaperTradingPage() {
  const { portfolio, positions, isLoading: isPortfolioLoading } = usePortfolio();
  const { stocks, isLoading: isStocksLoading } = useStocks();
  const { data: tradesData, mutate: mutateTrades } = useSWR<any>('/portfolio/trades', fetcher);

  const trades: any[] = Array.isArray(tradesData) ? tradesData : (tradesData?.data && Array.isArray(tradesData.data) ? tradesData.data : []);

  // Form state
  const [selectedSymbol, setSelectedSymbol] = useState<string>('TCS');
  const [tradeType, setTradeType] = useState<'BUY' | 'SELL'>('BUY');
  const [shares, setShares] = useState<number>(10);
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [feedback, setFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);

  // Selected stock details
  const activeStock = stocks.find((s) => s.symbol === selectedSymbol);
  const cmp = activeStock?.price ? Number(activeStock.price) : 0;
  const isShariahPass = activeStock?.shariah === 'PASS';
  const grossValue = shares * cmp;
  const costBreakdown = calculateDeliveryCosts(tradeType, grossValue);
  const fees = costBreakdown.totalFees;
  const totalCost = tradeType === 'BUY' ? grossValue + fees : Math.max(0, grossValue - fees);

  const cashBalance = Number(portfolio?.cash_balance ?? 1000000);
  const investedValue = positions.reduce((acc: number, pos: any) => acc + (Number(pos.shares || 0) * Number(pos.last_price || pos.avg_cost_basis || 0)), 0);
  const totalEquity = cashBalance + investedValue;
  const totalPnl = positions.reduce((acc: number, pos: any) => acc + Number(pos.unrealized_pnl || 0), 0);
  const totalPnlPct = totalEquity > 0 ? (totalPnl / totalEquity) * 100 : 0;

  // Execute paper trade
  const handleExecuteTrade = async (e: React.FormEvent) => {
    e.preventDefault();
    if (shares <= 0) {
      setFeedback({ type: 'error', message: 'Shares must be greater than 0.' });
      return;
    }
    if (tradeType === 'BUY' && !isShariahPass) {
      setFeedback({ type: 'error', message: `Shariah Gate: Cannot BUY ${selectedSymbol} because its screening status is ${activeStock?.shariah || 'UNKNOWN'}. Only PASS stocks are investable.` });
      return;
    }
    if (tradeType === 'BUY' && totalCost > cashBalance) {
      setFeedback({ type: 'error', message: `Insufficient cash balance. Required: ₹${totalCost.toFixed(2)}, Available: ₹${cashBalance.toFixed(2)}` });
      return;
    }

    setIsSubmitting(true);
    setFeedback(null);

    try {
      const res = await fetch('/api/proxy/portfolio/trade', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          symbol: selectedSymbol,
          trade_type: tradeType,
          shares: shares,
          price: cmp,
        }),
      });

      const data = await res.json();
      if (!res.ok) {
        throw new Error(data.error || 'Paper trade execution failed');
      }

      setFeedback({
        type: 'success',
        message: `Successfully executed paper ${tradeType} of ${shares} shares of ${selectedSymbol} at ₹${cmp.toFixed(2)}!`,
      });
      mutateTrades();
      // Reload window location or revalidate after short delay to sync positions
      setTimeout(() => {
        window.location.reload();
      }, 1500);
    } catch (err: any) {
      setFeedback({ type: 'error', message: err.message || 'Execution error.' });
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
            Simulated institutional order execution with real-time portfolio accounting and Shariah compliance gates.
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
          <p className="text-2xl font-bold text-emerald-400 mt-2">100.0% Compliant</p>
          <p className="text-xs text-slate-500 mt-1">Spot equities only · Zero margin</p>
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
                  {s.symbol} — {s.name} ({s.shariah})
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
              disabled={isSubmitting || (tradeType === 'BUY' && !isShariahPass)}
              className={`w-full py-2.5 rounded-xl font-bold text-sm transition-all shadow-lg ${
                tradeType === 'BUY'
                  ? isShariahPass
                    ? 'bg-emerald-500 hover:bg-emerald-400 text-white shadow-emerald-500/20'
                    : 'bg-slate-800 text-slate-500 cursor-not-allowed border border-slate-700'
                  : 'bg-rose-500 hover:bg-rose-400 text-white shadow-rose-500/20'
              } disabled:opacity-50`}
            >
              {isSubmitting ? 'Executing...' : tradeType === 'BUY' ? (isShariahPass ? `Execute Paper Buy` : 'Shariah Blocked') : `Execute Paper Sell`}
            </button>
          </div>
        </form>

        {/* Order Preview Info */}
        <div className="mt-4 pt-4 border-t border-slate-800 flex flex-col gap-2 text-xs">
          <div className="flex flex-wrap items-center justify-between gap-4">
            <div className="flex flex-wrap items-center gap-4 text-slate-400">
              <span>Market CMP: <strong className="text-white font-mono">₹{cmp.toFixed(2)}</strong></span>
              <span>Turnover: <strong className="text-white font-mono">₹{grossValue.toFixed(2)}</strong></span>
              <span>Statutory Levies: <strong className="text-amber-400 font-mono">₹{fees.toFixed(2)}</strong></span>
              <span>{tradeType === 'BUY' ? 'Total Cash Required' : 'Net Cash Realized'}: <strong className="text-emerald-400 font-mono">₹{totalCost.toFixed(2)}</strong></span>
            </div>
            <div>
              <span className={`px-2.5 py-1 rounded-full text-xs font-semibold ${
                isShariahPass ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30' : 'bg-rose-500/20 text-rose-400 border border-rose-500/30'
              }`}>
                Shariah: {activeStock?.shariah || 'UNKNOWN'}
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
                positions.map((pos: any) => {
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
                <th className="p-4 text-right">Fees Paid</th>
                <th className="p-4 text-right">Realized P&L</th>
              </tr>
            </thead>
            <tbody className="text-sm divide-y divide-slate-800/60">
              {trades.length === 0 ? (
                <tr>
                  <td colSpan={7} className="text-center py-8 text-slate-500">
                    No execution records in paper trade ledger yet.
                  </td>
                </tr>
              ) : (
                trades.map((t: any) => (
                  <tr key={t.id} className="hover:bg-slate-800/30 transition-colors">
                    <td className="p-4 text-xs font-mono text-slate-400">{new Date(t.trade_date).toLocaleString('en-IN')}</td>
                    <td className="p-4 font-bold text-white">{t.symbol}</td>
                    <td className="p-4">
                      <span className={`px-2 py-0.5 rounded text-xs font-bold ${
                        t.trade_type === 'BUY' ? 'bg-emerald-500/20 text-emerald-400' : 'bg-rose-500/20 text-rose-400'
                      }`}>
                        {t.trade_type}
                      </span>
                    </td>
                    <td className="p-4 text-right font-mono text-slate-300">{t.shares}</td>
                    <td className="p-4 text-right font-mono text-white">₹{Number(t.price).toFixed(2)}</td>
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
