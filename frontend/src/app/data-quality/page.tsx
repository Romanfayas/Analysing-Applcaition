'use client';

import useSWR from 'swr';
import { fetcher } from '@/lib/api/client';

export default function DataQualityPage() {
  const { data: dashData, error: dashError, isLoading: dashLoading } = useSWR<any>('/data-quality', fetcher);
  const { data: issuesData, error: issuesError, isLoading: issuesLoading } = useSWR<any>('/data-quality/issues', fetcher);

  const datasets: any[] = Array.isArray(dashData) ? dashData : (dashData?.data && Array.isArray(dashData.data) ? dashData.data : []);
  const issues: any[] = Array.isArray(issuesData) ? issuesData : (issuesData?.data && Array.isArray(issuesData.data) ? issuesData.data : []);
  const loading = dashLoading || issuesLoading;

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Data Quality & Provenance</h1>
          <p className="text-sm text-slate-400 mt-1">Live database provenance audit, quarantine enforcement, and feed verification.</p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-slate-800/80 border border-slate-700/60 text-xs text-slate-300">
          <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>Zero Fabricated Data Guarantee</span>
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
                        d.status === 'PARTIAL' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/30' :
                        'bg-rose-500/20 text-rose-400 border border-rose-500/30'
                      }`}>
                        {d.status}
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
          <span className="text-xs text-slate-400 font-mono">{issues.length} Recorded Items</span>
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
                    <td className="px-4 py-3 font-mono text-xs text-slate-300">{issue.check_type}</td>
                    <td className="px-4 py-3">
                       <span className={`px-2 py-0.5 rounded-full text-[11px] font-semibold ${
                        issue.severity === 'CRITICAL' ? 'bg-rose-500/20 text-rose-400 border border-rose-500/30' : 'bg-amber-500/20 text-amber-300 border border-amber-500/30'
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
    </div>
  );
}
