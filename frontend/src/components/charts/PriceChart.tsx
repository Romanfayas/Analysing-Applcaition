'use client';

import { useEffect, useRef, useState } from 'react';
import { createChart, createSeriesMarkers, ColorType, CandlestickSeries, HistogramSeries, type IChartApi, type ISeriesApi, type SeriesMarker, type Time, type UTCTimestamp } from 'lightweight-charts';
import type { ChartCandle, LargeMinute } from '@/lib/api/types';

const IST_OFFSET = 19800; // the chart draws Unix time as UTC, so shift it to show Indian time

const timeOf = (c: { ts?: number; date?: string }): Time => (c.ts != null ? ((c.ts + IST_OFFSET) as UTCTimestamp) : (c.date as string));

export default function PriceChart({ candles, large = [], intraday, height = 420 }: { candles: ChartCandle[]; large?: LargeMinute[]; intraday: boolean; height?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);
  const volRef = useRef<ISeriesApi<'Histogram'> | null>(null);
  const [showVolume, setShowVolume] = useState(true);
  const [legend, setLegend] = useState<ChartCandle | null>(null);
  const last = candles.length > 0 ? candles[candles.length - 1] : null;

  useEffect(() => {
    if (!ref.current) return;
    const chart = createChart(ref.current, {
      layout: { background: { type: ColorType.Solid, color: 'transparent' }, textColor: '#94a3b8' },
      grid: { vertLines: { color: 'rgba(51, 65, 85, 0.4)' }, horzLines: { color: 'rgba(51, 65, 85, 0.4)' } },
      width: ref.current.clientWidth, height,
      timeScale: { borderColor: 'rgba(51, 65, 85, 0.8)', timeVisible: intraday, secondsVisible: false },
      rightPriceScale: { borderColor: 'rgba(51, 65, 85, 0.8)' },
    });
    chartRef.current = chart;
    const price = chart.addSeries(CandlestickSeries, { upColor: '#10B981', downColor: '#F43F5E', borderVisible: false, wickUpColor: '#10B981', wickDownColor: '#F43F5E' });
    price.setData(candles.map((c) => ({ time: timeOf(c), open: c.o, high: c.h, low: c.l, close: c.c })));

    const vol = chart.addSeries(HistogramSeries, { priceFormat: { type: 'volume' }, priceScaleId: 'vol', lastValueVisible: false, priceLineVisible: false });
    chart.priceScale('vol').applyOptions({ scaleMargins: { top: 0.82, bottom: 0 } });
    volRef.current = vol;
    vol.setData(candles.map((c) => ({ time: timeOf(c), value: c.v, color: c.c >= c.o ? 'rgba(16,185,129,0.45)' : 'rgba(244,63,94,0.45)' })));

    if (large.length > 0) {
      const markers: SeriesMarker<Time>[] = large.map((m) => ({
        time: (m.ts + IST_OFFSET) as UTCTimestamp,
        position: (m.candle === 'DOWN' ? 'aboveBar' : 'belowBar') as 'aboveBar' | 'belowBar',
        shape: (m.candle === 'DOWN' ? 'arrowDown' : m.candle === 'UP' ? 'arrowUp' : 'circle') as 'arrowDown' | 'arrowUp' | 'circle',
        color: m.candle === 'DOWN' ? '#F43F5E' : m.candle === 'UP' ? '#10B981' : '#94a3b8',
        text: `${m.times_normal.toFixed(0)}x`,
      })).sort((a, b) => (a.time as number) - (b.time as number));
      createSeriesMarkers(price, markers);
    }
    chart.timeScale().fitContent();

    // hovering a candle shows its open, high, low, close and volume (like a broker app)
    const byTime = new Map<string, ChartCandle>();
    candles.forEach((c) => byTime.set(String(timeOf(c)), c));
    chart.subscribeCrosshairMove((p) => setLegend(p.time != null ? byTime.get(String(p.time)) ?? null : null));

    const onResize = () => chart.applyOptions({ width: ref.current?.clientWidth });
    window.addEventListener('resize', onResize);
    return () => { window.removeEventListener('resize', onResize); chart.remove(); };
  }, [candles, large, intraday, height]);

  useEffect(() => { volRef.current?.applyOptions({ visible: showVolume }); }, [showVolume, candles]);

  const shown = legend ?? last;
  const up = shown ? shown.c >= shown.o : true;
  const f = (v: number) => v.toFixed(2);
  return (
    <div className="relative">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 px-1 pb-2 text-xs font-mono">
        {shown && (
          <>
            <span className="text-slate-500 font-sans">{shown.ts != null ? new Date((shown.ts + IST_OFFSET) * 1000).toISOString().slice(0, 16).replace('T', ' ') : shown.date}</span>
            {([['O', shown.o], ['H', shown.h], ['L', shown.l], ['C', shown.c]] as const).map(([k, v]) => (
              <span key={k}><span className="text-slate-500">{k} </span><span className={up ? 'text-emerald-400' : 'text-rose-400'}>{f(v)}</span></span>))}
            <span><span className="text-slate-500">Vol </span><span className="text-slate-200">{shown.v.toLocaleString('en-IN')}</span></span>
          </>)}
        <label className="ml-auto flex items-center gap-1.5 font-sans text-slate-300 cursor-pointer select-none">
          <input type="checkbox" checked={showVolume} onChange={(e) => setShowVolume(e.target.checked)} /> Volume
        </label>
      </div>
      <div ref={ref} className="w-full" />
    </div>
  );
}
