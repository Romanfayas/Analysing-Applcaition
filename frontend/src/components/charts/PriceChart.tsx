'use client';

import { useEffect, useRef } from 'react';
import { createChart, createSeriesMarkers, ColorType, CandlestickSeries, HistogramSeries, type IChartApi, type SeriesMarker, type Time, type UTCTimestamp } from 'lightweight-charts';
import type { ChartCandle, LargeMinute } from '@/lib/api/types';

const IST_OFFSET = 19800; // the chart draws Unix time as UTC, so shift it to show Indian time

const timeOf = (c: { ts?: number; date?: string }): Time => (c.ts != null ? ((c.ts + IST_OFFSET) as UTCTimestamp) : (c.date as string));

export default function PriceChart({ candles, large = [], intraday, height = 420 }: { candles: ChartCandle[]; large?: LargeMinute[]; intraday: boolean; height?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const chartRef = useRef<IChartApi | null>(null);

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

    const onResize = () => chart.applyOptions({ width: ref.current?.clientWidth });
    window.addEventListener('resize', onResize);
    return () => { window.removeEventListener('resize', onResize); chart.remove(); };
  }, [candles, large, intraday, height]);

  return <div ref={ref} className="w-full" />;
}
