"use client";

import { use, useState } from 'react';
import Link from 'next/link';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import CandlestickChart, { CandleData } from '@/components/charts/CandlestickChart';
import MarkdownRenderer from '@/components/ui/MarkdownRenderer';
import { BeakerIcon, CheckCircleIcon, ExclamationTriangleIcon, XMarkIcon } from '@heroicons/react/24/outline';
import { useStock } from '@/lib/api/hooks';
import { send } from '@/lib/api/client';
import ResearchDepthCard from '@/components/stock/ResearchDepthCard';
import { calculateDeliveryCosts } from '@/lib/costs';

export default function StockResearchPage({ params }: { params: Promise<{ symbol: string }> }) {
  const resolvedParams = use(params);
  const { stock, isLoading, isError } = useStock(resolvedParams.symbol);

  const [tradeModalOpen, setTradeModalOpen] = useState(false);
  const [tradeSide, setTradeSide] = useState<'BUY' | 'SELL'>('BUY');
  const [tradeShares, setTradeShares] = useState(10);
  const [tradeLoading, setTradeLoading] = useState(false);
  const [tradeFeedback, setTradeFeedback] = useState<{ type: 'success' | 'error'; message: string } | null>(null);
  
  if (isLoading) {
    return <div className="text-center py-20 text-slate-500">Loading stock data...</div>;
  }

  if (isError || !stock) {
    return <div className="text-center py-20 text-red-500">Failed to load stock data. It may not exist.</div>;
  }

  const isShariahPass = stock.shariah?.status === 'PASS';
  const isFundamentalsAvailable = stock.fundamentals?.status === 'AVAILABLE' || stock.fundamentals?.status === 'VALID';

  const chartData: CandleData[] = stock.ohlcv?.map(d => ({
    time: d.timestamp,
    open: d.open,
    high: d.high,
    low: d.low,
    close: d.close
  })) || [];

  const handleExecuteTrade = async (e: React.FormEvent) => {
    e.preventDefault();
    if (tradeShares <= 0) {
      setTradeFeedback({ type: 'error', message: 'Shares must be greater than 0.' });
      return;
    }

    setTradeLoading(true);
    setTradeFeedback(null);

    try {
      // The server fills at its own stored market price; the client never supplies one.
      const result = await send<{ trade: { price: number } }>('post', '/portfolio/trade', {
        symbol: stock.symbol,
        trade_type: tradeSide,
        shares: Number(tradeShares),
      });

      setTradeFeedback({
        type: 'success',
        message: `Paper ${tradeSide} of ${tradeShares} shares of ${stock.symbol} filled at ₹${result.trade.price.toFixed(2)}.`,
      });
      setTimeout(() => {
        setTradeModalOpen(false);
        setTradeFeedback(null);
      }, 1800);
    } catch (err) {
      setTradeFeedback({ type: 'error', message: err instanceof Error ? err.message : 'Execution error.' });
    } finally {
      setTradeLoading(false);
    }
  };

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      {/* Header */}
      <div className="flex justify-between items-end">
        <div>
          <div className="flex items-center gap-4 mb-2">
            <h1 className="text-4xl font-heading font-bold text-white tracking-tight">{stock.symbol}</h1>
            <Badge variant={isShariahPass ? 'pass' : (stock.shariah?.status === 'FAIL' ? 'fail' : 'neutral')}>
              {stock.shariah?.status === 'PASS' ? 'SHARIAH COMPLIANT' : (stock.shariah?.status || 'UNKNOWN')}
            </Badge>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <span className="text-2xl font-bold font-mono text-white">
              {stock.current_price !== null && stock.current_price !== undefined
                ? `₹${stock.current_price.toFixed(2)}`
                : 'Price Unavailable'}
            </span>
            {stock.change !== null && stock.change !== undefined && stock.change_percent !== null && stock.change_percent !== undefined ? (
              <span
                className={`text-sm font-semibold font-mono ${
                  stock.change > 0
                    ? 'text-emerald-400'
                    : stock.change < 0
                    ? 'text-rose-400'
                    : 'text-slate-400'
                }`}
              >
                {stock.change > 0 ? '+' : ''}
                {stock.change.toFixed(2)} ({stock.change > 0 ? '+' : ''}
                {stock.change_percent.toFixed(2)}%)
              </span>
            ) : (
              <span className="text-xs text-slate-500 italic">Previous close unavailable</span>
            )}
            {stock.freshness && (
              <span className={`text-[10px] uppercase font-bold px-2 py-0.5 rounded border ${
                stock.freshness === 'FRESH'
                  ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30'
                  : stock.freshness === 'LAST_AVAILABLE'
                  ? 'bg-blue-500/10 text-blue-400 border-blue-500/30'
                  : stock.freshness === 'STALE'
                  ? 'bg-amber-500/10 text-amber-400 border-amber-500/30'
                  : 'bg-rose-500/10 text-rose-400 border-rose-500/30'
              }`}>
                {stock.freshness}
              </span>
            )}
          </div>
          {stock.timestamp && (
            <p className="text-[11px] text-slate-500 mt-1">
              Candle: {new Date(stock.timestamp).toLocaleString('en-IN', { timeZone: 'Asia/Kolkata' })} IST
              {stock.received_at && (
                <> · Ingested: {new Date(stock.received_at).toLocaleString('en-IN', { timeZone: 'Asia/Kolkata' })} IST</>
              )}
            </p>
          )}
        </div>
        <div className="flex gap-3">
          <button 
            onClick={() => {
              setTradeSide('SELL');
              setTradeFeedback(null);
              setTradeModalOpen(true);
            }}
            className="px-6 py-2.5 rounded-xl bg-slate-800 text-white font-medium border border-slate-700 hover:bg-slate-700 transition-colors"
          >
            Sell
          </button>
          <button
            onClick={() => {
              setTradeSide('BUY');
              setTradeFeedback(null);
              setTradeModalOpen(true);
            }}
            className="px-6 py-2.5 rounded-xl font-medium transition-colors bg-blue-500 text-white shadow-[0_0_15px_rgba(59,130,246,0.3)] hover:bg-blue-400"
          >
            Paper Buy
          </button>
        </div>
      </div>

      {/* Stale / Delayed Price Warning Notice */}
      {(stock.freshness === 'STALE' || stock.freshness === 'UNAVAILABLE') && (
        <div className="p-4 rounded-2xl border border-amber-500/40 bg-amber-500/10 text-amber-200 flex items-start gap-3">
          <ExclamationTriangleIcon className="w-5 h-5 text-amber-400 shrink-0 mt-0.5" />
          <div className="text-xs space-y-1">
            <div className="font-bold text-amber-300">
              {stock.freshness === 'STALE' ? 'Delayed / Stale Market Quote Notice' : 'Market Quote Currently Unavailable'}
            </div>
            <p className="text-amber-200/90 leading-relaxed">
              The displayed Current Market Price (CMP) is derived from provider intraday feeds (last candle: {stock.timestamp ? new Date(stock.timestamp).toLocaleString('en-IN', { timeZone: 'Asia/Kolkata' }) : 'Unknown'} IST). Because this quote exceeds the 15-minute freshness threshold during active market hours, it is flagged as STALE. Delayed aggregator quotes are unverified and do not guarantee exchange execution.
            </p>
          </div>
        </div>
      )}

      {/* Corporate Action Restructuring Notice */}
      {stock.corporate_action && (
        <div className="p-4 rounded-2xl border border-amber-500/30 bg-amber-500/10 text-amber-200">
          <div className="flex flex-wrap items-center gap-2 font-semibold text-sm">
            <span>Corporate Action: {stock.corporate_action.action_type}</span>
            <span className="text-xs bg-amber-500/20 px-2 py-0.5 rounded border border-amber-500/40">
              Effective: {stock.corporate_action.effective_date}
            </span>
            {stock.is_active === false && (
              <span className="text-xs bg-rose-500/20 text-rose-300 px-2 py-0.5 rounded border border-rose-500/40">
                Predecessor Line Inactive
              </span>
            )}
          </div>
          <p className="text-xs text-amber-300/90 mt-1">
            {stock.corporate_action.notes || "This security underwent corporate restructuring and direct trading has transitioned to successor entities."}
          </p>
          {stock.corporate_action.successors && stock.corporate_action.successors.length > 0 && (
            <div className="mt-3 pt-3 border-t border-amber-500/20">
              <span className="text-[11px] font-medium text-amber-400 block mb-1">Successor Securities on NSE:</span>
              <div className="flex flex-wrap gap-2">
                {stock.corporate_action.successors.map((s) => (
                  <Link
                    key={s.symbol}
                    href={`/stocks/${s.symbol}`}
                    className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-slate-900 hover:bg-slate-800 text-xs text-slate-200 border border-slate-700 hover:border-slate-500 transition"
                  >
                    <span className="font-bold text-white">{s.symbol}</span>
                    <span className="text-slate-400">({s.name})</span>
                    <span className="text-emerald-400 text-[10px] font-mono">[{s.ratio} ratio · {s.cost_split_pct}% cost]</span>
                  </Link>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* Chart */}
      <Card className="p-1">
        <div className="p-4 border-b border-slate-700/50 flex flex-col sm:flex-row justify-between sm:items-center gap-2">
          <div>
            <h3 className="font-heading font-semibold text-white">Price Action (Daily EOD Candles)</h3>
            <p className="text-[11px] text-slate-400 mt-0.5">
              Source: <span className="font-mono text-slate-300">ohlcv_daily</span> table (completed daily session bars). CMP above displays latest intraday tick from <span className="font-mono text-slate-300">ohlcv_intraday</span>.
            </p>
          </div>
          <div className="flex items-center gap-2">
            <span className="text-[10px] text-slate-500 font-mono">Timeframe: 1D</span>
            <Badge variant="blue">Historical Daily</Badge>
          </div>
        </div>
        <div className="p-2 h-[400px]">
          {chartData.length > 0 ? (
            <CandlestickChart data={chartData} />
          ) : (
            <div className="h-full flex items-center justify-center text-slate-500">
              DATA_UNAVAILABLE
            </div>
          )}
        </div>
      </Card>

      {/* Quant Gauges */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <Card gradientHover>
          <CardHeader title="Composite Signal" subtitle="Strategy V2 score (indicators: see Technicals & Risk)" action={<Badge variant={stock.signal?.action.includes('BUY') ? 'pass' : 'neutral'}>{stock.signal?.action.replace('_', ' ') || 'NEUTRAL'}</Badge>} />
          <div className="space-y-4">
            <p className="text-sm text-slate-400 border-b border-slate-800 pb-2">{stock.signal?.explanation ?? 'No signal generated'}</p>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Composite Score</span>
              <span className="text-white font-medium">{stock.signal?.overall_score != null ? stock.signal.overall_score.toFixed(1) : '—'}</span>
            </div>
          </div>
        </Card>

        <Card gradientHover>
          <CardHeader 
            title="Fundamentals" 
            subtitle="Valuation Metrics" 
            action={
              <Badge variant={isFundamentalsAvailable ? 'pass' : 'neutral'}>
                {isFundamentalsAvailable ? 'AVAILABLE' : 'UNAVAILABLE'}
              </Badge>
            } 
          />
          <div className="space-y-4 py-2">
            {isFundamentalsAvailable && stock.fundamentals ? (
              <div className="space-y-2 text-left px-4">
                <div className="flex justify-between items-center border-b border-slate-800 pb-2">
                  <span className="text-slate-400">Revenue</span>
                  <span className="text-white font-medium">{stock.fundamentals.revenue != null ? `₹${(stock.fundamentals.revenue / 1e7).toFixed(2)} Cr` : 'N/A'}</span>
                </div>
                <div className="flex justify-between items-center border-b border-slate-800 pb-2">
                  <span className="text-slate-400">PAT</span>
                  <span className="text-white font-medium">{stock.fundamentals.pat != null ? `₹${(stock.fundamentals.pat / 1e7).toFixed(2)} Cr` : 'N/A'}</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-slate-400">EPS</span>
                  <span className="text-white font-medium">{stock.fundamentals.eps != null ? `₹${stock.fundamentals.eps.toFixed(2)}` : 'N/A'}</span>
                </div>
              </div>
            ) : (
              <div className="text-center py-2">
                <span className="text-amber-500/80 font-medium">DATA_UNAVAILABLE</span>
                <p className="text-xs text-slate-500 mt-2">Fundamental metrics missing.</p>
              </div>
            )}
          </div>
        </Card>
        <ResearchDepthCard symbol={stock.symbol} />

        <Card gradientHover className={isShariahPass ? "border-emerald-500/20 bg-emerald-500/5" : "border-red-500/20 bg-red-500/5"}>
          <CardHeader title="Shariah Checklist" subtitle="AAOIFI Standards" action={<Badge variant={isShariahPass ? 'pass' : stock.shariah?.status === 'FAIL' ? 'fail' : 'neutral'}>{stock.shariah?.status || 'UNKNOWN'}</Badge>} />
          <div className="space-y-4 text-center py-4">
            {stock.shariah?.status === 'PASS' ? (
              <p className="text-sm text-slate-300">
                Screening passed. Screening date: {stock.shariah.screening_date ? new Date(stock.shariah.screening_date).toLocaleDateString() : 'N/A'}
              </p>
            ) : stock.shariah?.status === 'FAIL' ? (
              <p className="text-sm text-rose-300">Failed Shariah screening. Not eligible for purchase.</p>
            ) : (
              <p className="text-sm text-amber-300/90">
                Not evaluated ({stock.shariah?.status ?? 'UNKNOWN'}). Do not treat this instrument as compliant.
              </p>
            )}
          </div>
        </Card>
      </div>

      {/* Gemini AI Report */}
      <Card className="border-blue-500/20 shadow-[0_0_30px_rgba(59,130,246,0.05)]">
        <div className="flex items-center gap-3 mb-6 border-b border-slate-700/50 pb-4">
          <div className="p-2 bg-blue-500/10 rounded-lg text-blue-400">
            <BeakerIcon className="w-6 h-6" />
          </div>
          <div>
            <h3 className="text-xl font-heading font-semibold text-white">Automated Quant Thesis</h3>
            <p className="text-sm text-slate-400">Deterministic engine rationale</p>
          </div>
        </div>
        {stock.signal?.explanation ? (
           <MarkdownRenderer content={stock.signal.explanation} />
        ) : (
           <div className="text-slate-500 text-center py-4">DATA_UNAVAILABLE</div>
        )}
      </Card>

      {/* Data Provenance & Quality */}
      <Card>
        <div className="p-4 border-b border-slate-700/50">
          <h3 className="font-heading font-semibold text-white">Data Provenance & Quality</h3>
        </div>
        <div className="p-4 grid grid-cols-1 md:grid-cols-2 gap-4 text-sm">
          <div className="space-y-2">
            <div className="flex justify-between">
              <span className="text-slate-400">OHLCV Source</span>
              <span className="text-slate-300">yahoo_finance</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">OHLCV Quality</span>
              <span className="text-success-400">VALID</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">OHLCV Last Fetched</span>
              <span className="text-slate-300">Today</span>
            </div>
          </div>
          <div className="space-y-2">
            <div className="flex justify-between">
              <span className="text-slate-400">Fundamentals Source</span>
              <span className="text-slate-300">persisted_provider</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Fundamentals Quality</span>
              <span className={isFundamentalsAvailable ? "text-success-400" : "text-amber-400"}>
                {isFundamentalsAvailable ? 'AVAILABLE' : 'DATA_UNAVAILABLE'}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Fundamentals Last Fetched</span>
              <span className="text-slate-300">
                {isFundamentalsAvailable && stock.fundamentals?.period_end ? new Date(stock.fundamentals.period_end).toLocaleDateString() : 'N/A'}
              </span>
            </div>
          </div>
        </div>
      </Card>

      {/* Paper Trading Modal */}
      {tradeModalOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm animate-in fade-in duration-200">
          <div className="bg-slate-900 border border-slate-800 rounded-3xl w-full max-w-md p-6 shadow-2xl relative">
            <button
              onClick={() => {
                setTradeModalOpen(false);
                setTradeFeedback(null);
              }}
              className="absolute top-5 right-5 text-slate-400 hover:text-white"
            >
              <XMarkIcon className="w-6 h-6" />
            </button>

            <h3 className="text-xl font-heading font-bold text-white mb-1">
              Paper {tradeSide} · {stock.symbol}
            </h3>
            <p className="text-xs text-slate-400 mb-6">
              Simulated order with zero-capital risk and deterministic execution.
            </p>

            {tradeFeedback && (
              <div
                className={`p-4 rounded-xl text-xs flex items-center gap-2 mb-6 ${
                  tradeFeedback.type === 'success'
                    ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30'
                    : 'bg-rose-500/10 text-rose-400 border border-rose-500/30'
                }`}
              >
                {tradeFeedback.type === 'success' ? (
                  <CheckCircleIcon className="w-4 h-4 shrink-0" />
                ) : (
                  <ExclamationTriangleIcon className="w-4 h-4 shrink-0" />
                )}
                <span>{tradeFeedback.message}</span>
              </div>
            )}

            <form onSubmit={handleExecuteTrade} className="space-y-4">
              <div>
                <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
                  Order Side
                </label>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    type="button"
                    onClick={() => setTradeSide('BUY')}
                    className={`py-2 text-sm font-semibold rounded-xl border transition ${
                      tradeSide === 'BUY'
                        ? 'bg-emerald-600/30 text-emerald-300 border-emerald-500'
                        : 'bg-slate-800 text-slate-400 border-slate-700 hover:bg-slate-700'
                    }`}
                  >
                    BUY
                  </button>
                  <button
                    type="button"
                    onClick={() => setTradeSide('SELL')}
                    className={`py-2 text-sm font-semibold rounded-xl border transition ${
                      tradeSide === 'SELL'
                        ? 'bg-rose-600/30 text-rose-300 border-rose-500'
                        : 'bg-slate-800 text-slate-400 border-slate-700 hover:bg-slate-700'
                    }`}
                  >
                    SELL
                  </button>
                </div>
              </div>

              <div>
                <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
                  Number of Shares
                </label>
                <input
                  type="number"
                  min="1"
                  step="1"
                  value={tradeShares}
                  onChange={(e) => setTradeShares(Math.max(1, parseInt(e.target.value) || 1))}
                  className="w-full bg-slate-800/80 border border-slate-700 rounded-xl px-4 py-2.5 text-white font-mono text-sm focus:outline-none focus:border-blue-500"
                />
              </div>

              {/* Statutory Cost Breakdown */}
              {(() => {
                const turnover = (stock.current_price || 0) * tradeShares;
                const costBreakdown = calculateDeliveryCosts(tradeSide, turnover);
                const netAmount = tradeSide === 'BUY'
                  ? turnover + costBreakdown.totalFees
                  : Math.max(0, turnover - costBreakdown.totalFees);
                return (
                  <div className="bg-slate-900/60 border border-slate-800 rounded-xl p-3.5 space-y-2 text-xs">
                    <div className="flex justify-between text-slate-400">
                      <span>Price per Share</span>
                      <span className="font-mono text-white">
                        {stock.current_price ? `₹${stock.current_price.toFixed(2)}` : 'Market Price'}
                      </span>
                    </div>
                    <div className="flex justify-between text-slate-400">
                      <span>Gross Turnover</span>
                      <span className="font-mono text-white">
                        ₹{turnover.toFixed(2)}
                      </span>
                    </div>
                    <div className="pt-2 border-t border-slate-800/80 space-y-1 text-[11px] text-slate-400">
                      <div className="flex justify-between">
                        <span>STT (0.1% delivery)</span>
                        <span className="font-mono text-slate-300">₹{costBreakdown.stt.toFixed(2)}</span>
                      </div>
                      {tradeSide === 'BUY' && (
                        <div className="flex justify-between">
                          <span>Stamp Duty (0.015%)</span>
                          <span className="font-mono text-slate-300">₹{costBreakdown.stampDuty.toFixed(2)}</span>
                        </div>
                      )}
                      <div className="flex justify-between">
                        <span>Exchange Charges (0.00297%)</span>
                        <span className="font-mono text-slate-300">₹{costBreakdown.exchangeCharges.toFixed(2)}</span>
                      </div>
                      <div className="flex justify-between">
                        <span>SEBI Charges (0.0001%)</span>
                        <span className="font-mono text-slate-300">₹{costBreakdown.sebiCharges.toFixed(2)}</span>
                      </div>
                      {tradeSide === 'SELL' && (
                        <div className="flex justify-between">
                          <span>DP Depository Fee</span>
                          <span className="font-mono text-slate-300">₹{costBreakdown.dpCharges.toFixed(2)}</span>
                        </div>
                      )}
                      <div className="flex justify-between">
                        <span>GST (18% on fees)</span>
                        <span className="font-mono text-slate-300">₹{costBreakdown.gst.toFixed(2)}</span>
                      </div>
                      <div className="flex justify-between font-medium text-slate-300 pt-1 border-t border-slate-800/50">
                        <span>Total Statutory Charges</span>
                        <span className="font-mono text-amber-400">₹{costBreakdown.totalFees.toFixed(2)}</span>
                      </div>
                    </div>
                    <div className="pt-2 border-t border-slate-700/50 flex justify-between font-semibold">
                      <span className="text-slate-300">{tradeSide === 'BUY' ? 'Total Cash Required' : 'Net Cash Realized'}</span>
                      <span className="font-mono text-blue-400">
                        ₹{netAmount.toFixed(2)}
                      </span>
                    </div>
                    <div className="text-[10px] text-slate-500 text-right">
                      Schedule: {costBreakdown.modelVersion}
                    </div>
                  </div>
                );
              })()}

              <div className="flex gap-3 pt-2">
                <button
                  type="button"
                  onClick={() => {
                    setTradeModalOpen(false);
                    setTradeFeedback(null);
                  }}
                  className="flex-1 py-2.5 rounded-xl bg-slate-800 text-slate-300 font-semibold text-sm border border-slate-700 hover:bg-slate-700 transition"
                >
                  Cancel
                </button>
                {tradeSide === 'BUY' && !isShariahPass && (
                  <p role="status" className="basis-full text-[11px] text-amber-300 bg-amber-500/10 border border-amber-500/20 rounded-lg px-3 py-2">
                    Shariah status: {stock.shariah?.status === 'FAIL' ? 'FAIL (not compliant)' : 'not evaluated'}. Paper trades are allowed and the status is recorded on the trade.
                  </p>
                )}
                <button
                  type="submit"
                  disabled={tradeLoading}
                  className={`flex-1 py-2.5 rounded-xl font-semibold text-sm transition shadow-lg ${
                    tradeSide === 'BUY'
                      ? 'bg-blue-600 hover:bg-blue-500 text-white shadow-blue-500/20'
                      : 'bg-rose-600 hover:bg-rose-500 text-white shadow-rose-500/20'
                  } ${tradeLoading ? 'opacity-50 cursor-not-allowed' : ''}`}
                >
                  {tradeLoading ? 'Executing...' : `Confirm ${tradeSide}`}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
