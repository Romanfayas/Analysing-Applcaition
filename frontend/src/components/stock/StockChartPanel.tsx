'use client';

import { useState } from 'react';
import Card from '@/components/ui/Card';
import PriceChart from '@/components/charts/PriceChart';
import { useStockChart, useStockDeals } from '@/lib/api/hooks';
import type { ChartRange, LargeDeal } from '@/lib/api/types';

const RANGES: { id: ChartRange; label: string }[] = [
  { id: '1D', label: '1D' }, { id: '1W', label: '1W' }, { id: '1M', label: '1M' }, { id: '6M', label: '6M' }, { id: '1Y', label: '1Y' }, { id: 'ALL', label: 'All' },
];
const RANGE_NOTE: Record<ChartRange, string> = {
  '1D': 'Every 1-minute candle of the latest session, with volume.', '1W': 'Last 5 sessions, 15-minute candles.', '1M': 'Last month, daily candles.',
  '6M': 'Last 6 months, daily candles.', '1Y': 'Last year, daily candles.', ALL: 'All stored history, daily candles.',
};
const TYPE_LABEL: Record<LargeDeal['client_type'], string> = {
  MUTUAL_FUND: 'Mutual fund', INSURANCE: 'Insurer / pension', FOREIGN_INVESTOR: 'Foreign fund', BANK_OR_FINANCIAL: 'Bank / broker', OTHER: 'Other',
};
const INSTITUTIONAL = new Set(['MUTUAL_FUND', 'INSURANCE', 'FOREIGN_INVESTOR']);

const cr = (v: number) => (Math.abs(v) >= 1e7 ? `₹${(v / 1e7).toFixed(2)} Cr` : `₹${(v / 1e5).toFixed(1)} L`);
const num = (v: number) => v.toLocaleString('en-IN');

function Deals({ symbol }: { symbol: string }) {
  const { data, isLoading, error } = useStockDeals(symbol);
  const isError = !!error;
  if (isLoading) return <div className="text-sm text-slate-500">Loading official deals…</div>;
  if (isError || !data) return <div className="text-sm text-rose-300">Could not load the official deal record.</div>;
  const net = data.institution_net_inr;
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-baseline gap-x-6 gap-y-1">
        <h4 className="font-heading font-semibold text-white">Who bought big? Official NSE bulk &amp; block deals</h4>
        <span className="text-[11px] text-slate-500">Record starts {data.record_starts || 'today'}; last {data.days} days</span>
      </div>
      {data.deals.length === 0 ? (
        <div className="rounded-lg border border-white/10 bg-slate-900/60 p-3 text-sm text-slate-300">
          No bulk or block deal has been reported in {data.symbol} since the app began recording (NSE publishes only the latest day, so earlier days cannot be recovered). Large buyers are only named when a trade crosses the exchange&apos;s reporting limits.
        </div>
      ) : (
        <>
          <div className={`rounded-lg border p-3 text-sm ${net > 0 ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200' : net < 0 ? 'border-rose-500/30 bg-rose-500/10 text-rose-200' : 'border-white/10 bg-slate-900/60 text-slate-300'}`}>
            Institution-type clients (mutual funds, insurers, foreign funds): bought <b>{cr(data.institution_buy_inr)}</b>, sold <b>{cr(data.institution_sell_inr)}</b>, net <b>{net >= 0 ? '+' : '−'}{cr(Math.abs(net))}</b>.
          </div>
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead><tr className="text-left text-slate-500 border-b border-white/10"><th className="py-1.5 pr-3">Date</th><th className="pr-3">Client</th><th className="pr-3">Type (guess)</th><th className="pr-3">Side</th><th className="pr-3 text-right">Quantity</th><th className="pr-3 text-right">Price</th><th className="text-right">Value</th></tr></thead>
              <tbody>
                {data.deals.map((d, i) => (
                  <tr key={i} className="border-b border-white/5">
                    <td className="py-1.5 pr-3 font-mono text-slate-300">{d.trade_date}<span className="ml-1 text-[10px] text-slate-500">{d.kind.toLowerCase()}</span></td>
                    <td className="pr-3 text-slate-100">{d.client_name}</td>
                    <td className="pr-3"><span className={INSTITUTIONAL.has(d.client_type) ? 'text-sky-300' : 'text-slate-400'}>{TYPE_LABEL[d.client_type]}</span></td>
                    <td className={`pr-3 font-semibold ${d.side === 'BUY' ? 'text-emerald-400' : 'text-rose-400'}`}>{d.side}</td>
                    <td className="pr-3 text-right font-mono">{num(d.quantity)}</td>
                    <td className="pr-3 text-right font-mono">₹{d.price.toFixed(2)}</td>
                    <td className="text-right font-mono text-slate-200">{cr(d.value_inr)}</td>
                  </tr>))}
              </tbody>
            </table>
          </div>
        </>
      )}
      <p className="text-[11px] text-slate-500">{data.caveat}</p>
    </div>
  );
}

export default function StockChartPanel({ symbol }: { symbol: string }) {
  const [range, setRange] = useState<ChartRange>('1D');
  const { data, isLoading, error, isValidating } = useStockChart(symbol, range);
  const isError = !!error;
  const intraday = range === '1D' || range === '1W';
  const s = data?.session;
  const large = data?.large_minutes ?? [];
  return (
    <Card className="p-1">
      <div className="p-4 border-b border-slate-700/50 flex flex-col sm:flex-row justify-between sm:items-center gap-2">
        <div>
          <h3 className="font-heading font-semibold text-white">Price chart</h3>
          <p className="text-[11px] text-slate-400 mt-0.5">{RANGE_NOTE[range]} {data && `(${data.candles.length} candles)`}{isValidating && ' · updating…'}</p>
        </div>
        {range === '1D' && <span className="text-[10px] text-slate-500 font-mono">Arrows = unusually large minutes (volume far above normal)</span>}
      </div>
      <div className="p-2 min-h-[420px]">
        {isLoading && !data ? <div className="h-[420px] flex items-center justify-center text-slate-500">Loading chart…</div>
          : isError || !data ? <div className="h-[420px] flex items-center justify-center text-rose-300">Could not load the chart.</div>
          : data.candles.length === 0 ? <div className="h-[420px] flex items-center justify-center text-slate-500">No candles stored for this range.</div>
          : <PriceChart key={range} candles={data.candles} large={large} intraday={intraday} />}
      </div>
      <div className="px-4 pb-4 flex gap-2" role="tablist" aria-label="Chart range">
        {RANGES.map((r) => (
          <button key={r.id} role="tab" aria-selected={range === r.id} onClick={() => setRange(r.id)}
            className={`px-4 py-1.5 rounded-lg text-sm font-medium border ${range === r.id ? 'bg-sky-500/20 border-sky-400/60 text-sky-200' : 'border-white/10 text-slate-400 hover:text-white hover:bg-white/5'}`}>{r.label}</button>))}
      </div>

      <div className="border-t border-slate-700/50 p-4 space-y-5">
        {range === '1D' && s && (
          <div className="space-y-3">
            <h4 className="font-heading font-semibold text-white">Big-money watch for {s.date}</h4>
            {s.baseline_sessions < 3 ? (
              <div className="text-sm text-slate-400">Not enough earlier sessions stored ({s.baseline_sessions}) to say what a normal minute looks like, so no minute can be called unusual yet.</div>
            ) : (
              <>
                <div className="grid grid-cols-2 md:grid-cols-4 gap-2 text-xs">
                  {[['Traded today', cr(s.total_notional_inr)], ['Unusually large minutes', String(s.large_minutes)],
                    ['Their share of the day', `${s.large_share_of_day_pct.toFixed(1)}%`], ['Volume in up-candles', `${s.up_candle_volume_pct.toFixed(0)}%`]].map(([k, v]) => (
                    <div key={k} className="rounded-lg bg-slate-900/60 p-2"><div className="text-slate-500">{k}</div><div className="font-mono text-slate-100 text-sm">{v}</div></div>))}
                </div>
                {large.length === 0 ? <div className="text-sm text-slate-400">No minute traded 5× its normal volume and ₹20 lakh or more in this session.</div> : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-xs">
                      <thead><tr className="text-left text-slate-500 border-b border-white/10"><th className="py-1.5 pr-3">Time (IST)</th><th className="pr-3 text-right">Shares</th><th className="pr-3 text-right">Value</th><th className="pr-3 text-right">× normal</th><th>Candle</th></tr></thead>
                      <tbody>{[...large].sort((a, b) => b.notional_inr - a.notional_inr).slice(0, 15).map((m) => (
                        <tr key={m.ts} className="border-b border-white/5"><td className="py-1.5 pr-3 font-mono">{m.time}</td><td className="pr-3 text-right font-mono">{num(m.volume)}</td><td className="pr-3 text-right font-mono">{cr(m.notional_inr)}</td>
                          <td className="pr-3 text-right font-mono">{m.times_normal.toFixed(1)}×</td><td className={m.candle === 'UP' ? 'text-emerald-400' : m.candle === 'DOWN' ? 'text-rose-400' : 'text-slate-400'}>{m.candle === 'UP' ? 'rose' : m.candle === 'DOWN' ? 'fell' : 'flat'}</td></tr>))}</tbody>
                    </table>
                  </div>)}
              </>
            )}
            <p className="text-[11px] text-slate-500 leading-relaxed">
              A 1-minute candle holds the total volume of every trade in that minute, not individual orders, and it never says who traded or whether it was a buy or a sale. A large minute means something big traded (one institution, many traders, or a fund rebalancing); the candle colour is only the price direction. Retail traders do not place ₹20 lakh orders, but on a liquid stock many small orders add up to far more than that every minute, so size alone cannot separate them. The official deals below are the only record that names the buyer.
            </p>
          </div>
        )}
        <Deals symbol={symbol} />
      </div>
    </Card>
  );
}
