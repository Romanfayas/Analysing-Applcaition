'use client';

import { useDataQuality, useHistoricalReadiness, useIntradayFeedHealth } from '@/lib/api/hooks';

export default function DataQualityPage() {
  const { datasets, issues, isLoading: loading, isError } = useDataQuality();
  const dashError = isError;
  const { readiness, isError: readinessError } = useHistoricalReadiness();
  const { feed } = useIntradayFeedHealth();
  const blocked = readiness?.symbols.filter((s) => s.status === 'BLOCKED').length ?? 0;
  const sum = (k: 'exchange_bars' | 'nominal_match' | 'vendor_adjusted' | 'mismatch' | 'missing_price_changing_actions' | 'events_confirmed_pending_review' | 'parser_fragments') =>
    readiness?.symbols.reduce((a, s) => a + s[k], 0) ?? 0;

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Data Quality & Provenance</h1>
          <p className="text-sm text-slate-400 mt-1">Live database provenance audit, quarantine enforcement, and feed verification.</p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-slate-800/80 border border-slate-700/60 text-xs text-slate-300">
          <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>Every figure traces to a stored source</span>
        </div>
      </div>

      {dashError && (
        <div className="p-4 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-sm">
          Failed to load data quality datasets from API.
        </div>
      )}

      {/* Dataset Health Table */}
      <div className="glass-card overflow-hidden">
        <div className="p-4 border-b border-white/5 bg-slate-900/40">
          <h2 className="text-lg font-semibold text-white">Dataset Verification & Provenance</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-400">
            <thead className="text-xs uppercase bg-white/5 text-slate-300">
              <tr>
                <th className="px-4 py-3">Dataset</th>
                <th className="px-4 py-3">Provider</th>
                <th className="px-4 py-3">Latest Market Date</th>
                <th className="px-4 py-3">Valid Records</th>
                <th className="px-4 py-3">Quarantined / Excluded</th>
                <th className="px-4 py-3">Integrity Status</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={6} className="text-center py-8 text-slate-500">Querying live dataset metrics...</td></tr>
              ) : datasets.length === 0 ? (
                <tr><td colSpan={6} className="text-center py-8 text-slate-500">No dataset metrics returned.</td></tr>
              ) : (
                datasets.map((d, i) => (
                  <tr key={i} className="border-b border-white/5 hover:bg-white/5 transition-colors">
                    <td className="px-4 py-3 text-white font-medium">{d.dataset}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-300">{d.provider}</td>
                    <td className="px-4 py-3">{d.latest_data ? new Date(d.latest_data).toLocaleDateString('en-IN') : '—'}</td>
                    <td className="px-4 py-3 text-emerald-400 font-mono font-semibold">{Number(d.valid_records || 0).toLocaleString()}</td>
                    <td className="px-4 py-3 text-rose-400 font-mono">{Number(d.invalid_records || 0).toLocaleString()}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2.5 py-1 rounded-full text-xs font-semibold ${
                        d.status === 'VALID' ? 'bg-emerald-500/20 text-emerald-400 border border-emerald-500/30' :
                        d.status === 'PARTIAL' || d.status === 'UNVERIFIED' || d.status === 'PENDING_REVIEW' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/30' :
                        'bg-rose-500/20 text-rose-400 border border-rose-500/30'
                      }`}>
                        {(d.status ?? '').replaceAll('_', ' ')}
                      </span>
                    </td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Anomalies and Quarantine Logs */}
      <div className="glass-card overflow-hidden">
        <div className="p-4 border-b border-white/5 bg-slate-900/40 flex justify-between items-center">
          <h2 className="text-lg font-semibold text-white">Quarantine & Data Integrity Audit Trail</h2>
          <span className="text-xs text-slate-400 font-mono">{issues.filter((i) => i.severity !== 'RESOLVED').length} open · {issues.filter((i) => i.severity === 'RESOLVED').length} resolved</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-400">
            <thead className="text-xs uppercase bg-white/5 text-slate-300">
              <tr>
                <th className="px-4 py-3">Time</th>
                <th className="px-4 py-3">Symbol</th>
                <th className="px-4 py-3">Classification</th>
                <th className="px-4 py-3">Severity</th>
                <th className="px-4 py-3 w-1/2">Audit Finding / Reason</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={5} className="text-center py-8 text-slate-500">Loading audit trail...</td></tr>
              ) : issues.length === 0 ? (
                <tr><td colSpan={5} className="text-center py-8 text-slate-500">No active anomalies in quarantine.</td></tr>
              ) : (
                issues.map((issue) => (
                  <tr key={issue.id} className="border-b border-white/5 hover:bg-white/5 transition-colors">
                    <td className="px-4 py-3 whitespace-nowrap text-xs font-mono">{issue.recorded_at ? new Date(issue.recorded_at).toLocaleString('en-IN') : '—'}</td>
                    <td className="px-4 py-3 font-semibold text-white">{issue.symbol}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-300">{(issue.check_type ?? '').replaceAll('_', ' ')}</td>
                    <td className="px-4 py-3">
                       <span className={`px-2 py-0.5 rounded-full text-[11px] font-semibold ${
                        issue.severity === 'CRITICAL' ? 'bg-rose-500/20 text-rose-400 border border-rose-500/30'
                        : issue.severity === 'RESOLVED' ? 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/30'
                        : 'bg-amber-500/20 text-amber-300 border border-amber-500/30'
                      }`}>
                        {issue.severity}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-300">{issue.message}</td>
                  </tr>
                ))
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* P1.9.24: exchange evidence and research readiness */}
      <div className="glass-card p-6 space-y-4">
        <div>
          <h2 className="text-lg font-semibold text-white">Historical research readiness</h2>
          <p className="text-xs text-slate-400 mt-1">
            Vendor history compared with NSE&apos;s own end-of-day record (bhavcopy) and NSE&apos;s corporate-actions feed. Exchange evidence is stored as
            authenticity-pending and is never approved automatically.
          </p>
        </div>
        {readinessError && <p className="text-rose-400 text-sm">Could not load readiness.</p>}
        {readiness?.research_series && (
          <div className="p-4 rounded-xl bg-emerald-500/10 border border-emerald-500/30 text-sm text-emerald-100">
            <p className="font-semibold">Research data is ready: {readiness.research_series.series.bars.toLocaleString('en-IN')} daily bars for {readiness.research_series.series.symbols} stocks, {readiness.research_series.series.from} to {readiness.research_series.series.to}</p>
            <p className="text-xs text-emerald-200/80 mt-1">
              Built from NSE&apos;s own files. {readiness.research_series.series.adjustments_confirmed} of {readiness.research_series.series.adjustments_total} splits, bonuses and demergers were confirmed against the actual price step on the ex-date, with {readiness.research_series.series.unexplained_breaks.length} unexplained breaks.
              The Backtesting page and the breakout monitor use this series. The rows below show the separate vendor history and what its formal certification still needs; they do not stop that research.
            </p>
          </div>
        )}
        {readiness && (
          <>
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3 text-sm">
              <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800"><span className="text-xs text-slate-400 block">Awaiting formal certification</span><span className="text-xl font-mono text-amber-400">{blocked} / {readiness.symbols.length}</span></div>
              <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800"><span className="text-xs text-slate-400 block">Exchange bars loaded</span><span className="text-xl font-mono text-white">{sum('exchange_bars').toLocaleString()}</span></div>
              <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800"><span className="text-xs text-slate-400 block">Vendor bars: match / back-adjusted / mismatch</span><span className="text-sm font-mono text-white">{sum('nominal_match').toLocaleString()} / {sum('vendor_adjusted').toLocaleString()} / {sum('mismatch').toLocaleString()}</span></div>
              <div className="p-3 rounded-xl bg-slate-900/60 border border-slate-800"><span className="text-xs text-slate-400 block">Unrecorded bonus/split/demerger</span><span className="text-xl font-mono text-rose-400">{sum('missing_price_changing_actions')}</span></div>
            </div>
            <p className="text-xs text-amber-200/80">{readiness.policy_note}</p>
            <p className="text-xs text-slate-400">
              Next-day prediction model (not built yet; needs certified bars): <strong className="text-white">{readiness.next_day_prediction_research.status.replaceAll('_', ' ').toLowerCase()}</strong>{' '}
              ({readiness.next_day_prediction_research.symbols_meeting_requirement} of {readiness.next_day_prediction_research.required_symbols} required instruments have ≥{' '}
              {readiness.next_day_prediction_research.required_eligible_sessions} research-eligible sessions)
            </p>
            <div className="overflow-x-auto max-h-96">
              <table className="w-full text-left text-xs">
                <thead className="sticky top-0 bg-slate-900">
                  <tr className="text-slate-400 uppercase">
                    <th className="py-2 px-2">Symbol</th><th className="text-right">Eligible</th><th className="text-right">Quarantined</th><th className="text-right">Exchange bars</th>
                    <th className="text-right">Match</th><th className="text-right">Back-adj.</th><th className="text-right">Mismatch</th><th className="text-right">Missing CA</th>
                    <th className="text-right">Pending review</th><th className="px-2">Blockers</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-800/60 font-mono">
                  {readiness.symbols.map((s) => (
                    <tr key={s.symbol}>
                      <td className="py-1.5 px-2 font-sans font-semibold text-white">{s.symbol}{!s.is_active && <span className="text-slate-500 font-normal"> (inactive)</span>}</td>
                      <td className="text-right text-slate-300">{s.research_eligible}</td>
                      <td className="text-right text-amber-300">{s.quarantined}</td>
                      <td className="text-right text-slate-300">{s.exchange_bars}</td>
                      <td className="text-right text-slate-300">{s.nominal_match}</td>
                      <td className={`text-right ${s.vendor_adjusted ? 'text-rose-300' : 'text-slate-500'}`}>{s.vendor_adjusted}</td>
                      <td className={`text-right ${s.mismatch ? 'text-rose-300' : 'text-slate-500'}`}>{s.mismatch}</td>
                      <td className={`text-right ${s.missing_price_changing_actions ? 'text-rose-300' : 'text-slate-500'}`}>{s.missing_price_changing_actions}</td>
                      <td className="text-right text-slate-300">{s.events_confirmed_pending_review}</td>
                      <td className="px-2 font-sans text-slate-400 max-w-md">{s.blockers.join('; ')}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </div>

      <div className="glass-card p-6 space-y-4">
        <div>
          <h2 className="text-lg font-semibold text-white">Intraday feed health</h2>
          <p className="text-xs text-slate-400 mt-1">{feed?.provider}</p>
        </div>
        {feed && (
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 text-xs">
            <div className="space-y-1">
              <h3 className="text-slate-300 font-semibold">Coverage by session</h3>
              {feed.sessions.slice(0, 10).map((d) => (
                <div key={d.date} className="flex justify-between font-mono text-slate-400">
                  <span>{d.date}</span><span>{d.symbols} sym · {d.coverage_pct_of_375_minutes.toFixed(1)}% · {d.suspect_bars} suspect</span>
                </div>
              ))}
            </div>
            <div className="space-y-1">
              <h3 className="text-slate-300 font-semibold">Structural gaps (minute missing on &gt;20% of days)</h3>
              <p className="text-slate-400 font-mono break-words">{feed.structural_gaps.map((g) => g.minute_ist).join(', ') || 'none'}</p>
              <p className="text-slate-500">{feed.recovery}</p>
            </div>
            <div className="space-y-1">
              <h3 className="text-slate-300 font-semibold">Latency and providers</h3>
              {(['bar_receipt', 'poll_freshness'] as const).map((k) => {
                const l = feed.latency[k];
                return (
                  <p key={k} className="text-slate-400">
                    <span className="text-slate-300">{k === 'bar_receipt' ? 'Bar receipt' : 'Poll freshness'}:</span>{' '}
                    {l.status === 'NOT_MEASURED' || l.median_seconds == null
                      ? <span className="text-amber-300">{l.status}</span>
                      : `median ${Math.round(l.median_seconds)} s · p95 ${Math.round(l.p95_seconds ?? 0)} s · p99 ${Math.round(l.p99_seconds ?? 0)} s (${l.observations} obs${l.status === 'MEASURED_LOW_SAMPLE' ? ', low sample' : ''})`}
                  </p>
                );
              })}
              <p className="text-slate-500">{feed.latency.definition}</p>
              <p className="text-slate-400">TrueData: <span className="text-white">{feed.truedata.status}</span></p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
