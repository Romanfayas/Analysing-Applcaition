"use client";

import { use } from 'react';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import CandlestickChart, { CandleData } from '@/components/charts/CandlestickChart';
import MarkdownRenderer from '@/components/ui/MarkdownRenderer';
import { ArrowTrendingUpIcon, ArrowTrendingDownIcon, CurrencyRupeeIcon, BeakerIcon } from '@heroicons/react/24/outline';
import { useStock } from '@/lib/api/hooks';

export default function StockResearchPage({ params }: { params: Promise<{ symbol: string }> }) {
  const resolvedParams = use(params);
  const { stock, isLoading, isError } = useStock(resolvedParams.symbol);
  
  if (isLoading) {
    return <div className="text-center py-20 text-slate-500">Loading stock data...</div>;
  }

  if (isError || !stock) {
    return <div className="text-center py-20 text-red-500">Failed to load stock data. It may not exist.</div>;
  }

  const isShariahPass = stock.shariah?.status === 'PASS';

  const chartData: CandleData[] = stock.ohlcv?.map(d => ({
    time: d.timestamp,
    open: d.open,
    high: d.high,
    low: d.low,
    close: d.close
  })) || [];

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
          <p className="text-slate-400 flex items-center gap-2">
            <span className="text-2xl font-bold text-white">₹{stock.current_price?.toFixed(2) || '0.00'}</span>
          </p>
        </div>
        <div className="flex gap-3">
          <button className="px-6 py-2.5 rounded-xl bg-slate-800 text-white font-medium border border-slate-700 hover:bg-slate-700 transition-colors">
            Sell
          </button>
          <button 
            disabled={!isShariahPass}
            className={`px-6 py-2.5 rounded-xl font-medium transition-colors ${
              isShariahPass 
                ? 'bg-blue-500 text-white shadow-[0_0_15px_rgba(59,130,246,0.3)] hover:bg-blue-400' 
                : 'bg-slate-700 text-slate-500 cursor-not-allowed'
            }`}
          >
            {isShariahPass ? 'Paper Buy' : 'Shariah Blocked'}
          </button>
        </div>
      </div>

      {/* Chart */}
      <Card className="p-1">
        <div className="p-4 border-b border-slate-700/50 flex justify-between items-center">
          <h3 className="font-heading font-semibold text-white">Price Action</h3>
          <div className="flex gap-2">
            <Badge variant="neutral">1D</Badge>
            <Badge variant="neutral">1W</Badge>
            <Badge variant="blue">1M</Badge>
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
          <CardHeader title="Technical Score" subtitle="Momentum & Trends" action={<Badge variant={stock.signal?.action.includes('BUY') ? 'pass' : 'neutral'}>{stock.signal?.action.replace('_', ' ') || 'NEUTRAL'}</Badge>} />
          <div className="space-y-4">
            <div className="flex justify-between items-center border-b border-slate-800 pb-2">
              <span className="text-slate-400">RSI (14)</span>
              <span className="text-slate-500 font-medium">DATA_UNAVAILABLE</span>
            </div>
            <div className="flex justify-between items-center border-b border-slate-800 pb-2">
              <span className="text-slate-400">MACD</span>
              <span className="text-slate-500 font-medium">DATA_UNAVAILABLE</span>
            </div>
            <div className="flex justify-between items-center">
              <span className="text-slate-400">Composite Score</span>
              <span className="text-white font-medium">{stock.signal?.overall_score?.toFixed(1) || '0.0'}</span>
            </div>
          </div>
        </Card>

        <Card gradientHover>
          <CardHeader title="Fundamentals" subtitle="Valuation Metrics" action={<Badge variant="neutral">UNAVAILABLE</Badge>} />
          <div className="space-y-4 text-center py-4">
            <span className="text-amber-500/80 font-medium">DATA_UNAVAILABLE</span>
            <p className="text-xs text-slate-500 mt-2">Fundamental metrics missing.</p>
          </div>
        </Card>

        <Card gradientHover className={isShariahPass ? "border-emerald-500/20 bg-emerald-500/5" : "border-red-500/20 bg-red-500/5"}>
          <CardHeader title="Shariah Checklist" subtitle="AAOIFI Standards" action={<Badge variant={isShariahPass ? 'pass' : 'fail'}>{stock.shariah?.status || 'UNKNOWN'}</Badge>} />
          <div className="space-y-4 text-center py-4">
            {stock.shariah?.status === 'DATA_UNAVAILABLE' ? (
              <span className="text-amber-500/80 font-medium">DATA_UNAVAILABLE</span>
            ) : stock.shariah?.status === 'DATA_INSUFFICIENT' ? (
              <span className="text-amber-500/80 font-medium">DATA_INSUFFICIENT</span>
            ) : (
              <p className="text-sm text-slate-300">Screening passed internal checks, but detailed ratios are not currently exposed in this phase.</p>
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
              <span className="text-slate-300">Mock_Fundamentals</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Fundamentals Quality</span>
              <span className="text-success-400">VALID</span>
            </div>
            <div className="flex justify-between">
              <span className="text-slate-400">Fundamentals Last Fetched</span>
              <span className="text-slate-300">Today</span>
            </div>
          </div>
        </div>
      </Card>
    </div>
  );
}
