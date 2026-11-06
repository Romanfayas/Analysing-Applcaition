"use client";

import useSWR from 'swr';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { fetcher } from '@/lib/api/client';

interface Basis {
  eligible_bars: number;
  quarantined_bars_excluded: number;
  last_bar: string | null;
}

interface TechnicalsResponse {
  status: 'AVAILABLE' | 'PARTIAL' | 'DATA_INSUFFICIENT';
  basis: Basis;
  required_bars?: number;
  indicators: Array<{ indicator: string; parameters: Record<string, number>; signal: string | null; latest: number | null }>;
  skipped: Array<{ indicator: string; min_bars: number }>;
}

interface RiskResponse {
  status: 'AVAILABLE' | 'DATA_INSUFFICIENT';
  observations: number;
  required_observations?: number;
  metrics: { volatility_daily: number | null; volatility_annual: number | null; max_drawdown: number | null; var_95: number | null; sharpe_ratio: number | null } | null;
}

const pct = (v: number | null | undefined) => (v === null || v === undefined ? '—' : `${(v * 100).toFixed(2)}%`);

/** Technicals and risk computed only from research-eligible daily bars. Quarantined history is
 *  never used, so a young verified history shows up here as "insufficient", not as a guess. */
export default function ResearchDepthCard({ symbol }: { symbol: string }) {
  const tech = useSWR<TechnicalsResponse>(`/stocks/${symbol}/technicals`, fetcher);
  const risk = useSWR<RiskResponse>(`/stocks/${symbol}/risk`, fetcher);
  const basis = tech.data?.basis;

  return (
    <Card>
      <CardHeader
        title="Technicals & Risk"
        subtitle="Computed from research-eligible bars only"
        action={<Badge variant={tech.data?.status === 'AVAILABLE' ? 'pass' : 'neutral'}>{tech.data?.status ?? '…'}</Badge>}
      />
      {tech.error || risk.error ? (
        <p className="text-sm text-rose-400">Could not compute analytics right now.</p>
      ) : (
        <div className="space-y-4 text-sm">
          {basis && (
            <p className="text-xs text-slate-500">
              {basis.eligible_bars} verified daily bars used · {basis.quarantined_bars_excluded.toLocaleString()} quarantined bars excluded
            </p>
          )}

          {tech.data?.indicators.length ? (
            <div className="space-y-2">
              {tech.data.indicators.map((i) => (
                <div key={`${i.indicator}-${JSON.stringify(i.parameters)}`} className="flex justify-between border-b border-slate-800 pb-2">
                  <span className="text-slate-400 uppercase">
                    {i.indicator}
                    {i.parameters?.period ? ` (${i.parameters.period})` : ''}
                  </span>
                  <span className="text-white font-mono">
                    {i.latest === null ? '—' : i.latest.toFixed(2)}
                    {i.signal ? <span className="text-xs text-slate-500 ml-2">{i.signal}</span> : null}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            tech.data && <p className="text-amber-500/80">Not enough verified history for any indicator (needs {tech.data.required_bars}+ bars).</p>
          )}

          {tech.data && tech.data.skipped.length > 0 && tech.data.indicators.length > 0 && (
            <p className="text-xs text-slate-500">
              Waiting for more history: {tech.data.skipped.map((s) => `${s.indicator} (${s.min_bars} bars)`).join(', ')}
            </p>
          )}

          {risk.data &&
            (risk.data.status === 'AVAILABLE' && risk.data.metrics ? (
              <div className="grid grid-cols-2 gap-2 pt-2 border-t border-slate-800">
                <span className="text-slate-400">Daily volatility</span>
                <span className="text-white font-mono text-right">{pct(risk.data.metrics.volatility_daily)}</span>
                <span className="text-slate-400">Max drawdown</span>
                <span className="text-white font-mono text-right">{pct(risk.data.metrics.max_drawdown)}</span>
                <span className="text-slate-400">VaR (95%)</span>
                <span className="text-white font-mono text-right">{pct(risk.data.metrics.var_95)}</span>
              </div>
            ) : (
              <p className="text-amber-500/80 pt-2 border-t border-slate-800">
                Risk metrics need {risk.data.required_observations}+ verified daily returns; {risk.data.observations} available.
              </p>
            ))}
        </div>
      )}
    </Card>
  );
}
