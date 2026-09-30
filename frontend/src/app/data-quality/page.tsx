'use client';

import { useState, useEffect } from 'react';

export default function DataQualityPage() {
  const [datasets, setDatasets] = useState<any[]>([]);
  const [issues, setIssues] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // In a real app we would fetch from Next.js API route that proxies to Go backend
    setTimeout(() => {
      setDatasets([
        {
          dataset: 'OHLCV Market Data',
          provider: 'yahoo_finance',
          latest_data: '2023-10-01T00:00:00Z',
          last_successful_fetch: '2023-10-02T05:00:00Z',
          valid_records: 1250000,
          invalid_records: 145,
          status: 'PARTIAL'
        },
        {
          dataset: 'Financial Statements',
          provider: 'Mock_Fundamentals',
          latest_data: '2023-06-30T00:00:00Z',
          last_successful_fetch: '2023-10-02T05:05:00Z',
          valid_records: 12400,
          invalid_records: 0,
          status: 'VALID'
        }
      ]);
      setIssues([
        { id: 1, symbol: 'RELIANCE', check_type: 'gap_detected', severity: 'WARNING', message: 'trading gap of 6 days', recorded_at: '2023-10-02T05:00:10Z' },
        { id: 2, symbol: 'TCS', check_type: 'spike_detected', severity: 'CRITICAL', message: 'extreme daily price change: 25.4%', recorded_at: '2023-10-02T05:00:15Z' },
        { id: 3, symbol: 'HDFCBANK', check_type: 'accounting_equation', severity: 'CRITICAL', message: 'Assets (0.00) != Liabilities (100.00) + Equity (100.00)', recorded_at: '2023-10-02T05:05:00Z' }
      ]);
      setLoading(false);
    }, 800);
  }, []);

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-white">Data Quality & Provenance</h1>
        <p className="text-sm text-slate-400 mt-1">Monitor ingestion health, data anomalies, and provider reliability.</p>
      </div>

      <div className="glass-card">
        <div className="p-4 border-b border-white/5">
          <h2 className="text-lg font-semibold text-white">Dataset Health</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-400">
            <thead className="text-xs uppercase bg-white/5 text-slate-300">
              <tr>
                <th className="px-4 py-3">Dataset</th>
                <th className="px-4 py-3">Provider</th>
                <th className="px-4 py-3">Latest Data</th>
                <th className="px-4 py-3">Valid Records</th>
                <th className="px-4 py-3">Invalid Records</th>
                <th className="px-4 py-3">Status</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={6} className="text-center py-4">Loading...</td></tr>
              ) : (
                datasets.map((d, i) => (
                  <tr key={i} className="border-b border-white/5 hover:bg-white/5">
                    <td className="px-4 py-3 text-white font-medium">{d.dataset}</td>
                    <td className="px-4 py-3">{d.provider}</td>
                    <td className="px-4 py-3">{new Date(d.latest_data).toLocaleDateString()}</td>
                    <td className="px-4 py-3 text-success-400">{d.valid_records.toLocaleString()}</td>
                    <td className="px-4 py-3 text-error-400">{d.invalid_records.toLocaleString()}</td>
                    <td className="px-4 py-3">
                      <span className={`px-2 py-1 rounded-full text-xs ${
                        d.status === 'VALID' ? 'bg-success-500/20 text-success-400' :
                        d.status === 'PARTIAL' ? 'bg-warn-500/20 text-warn-400' :
                        'bg-error-500/20 text-error-400'
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

      <div className="glass-card">
        <div className="p-4 border-b border-white/5">
          <h2 className="text-lg font-semibold text-white">Recent Anomalies & Logs</h2>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm text-slate-400">
            <thead className="text-xs uppercase bg-white/5 text-slate-300">
              <tr>
                <th className="px-4 py-3">Time</th>
                <th className="px-4 py-3">Symbol</th>
                <th className="px-4 py-3">Type</th>
                <th className="px-4 py-3">Severity</th>
                <th className="px-4 py-3 w-1/2">Message</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr><td colSpan={5} className="text-center py-4">Loading...</td></tr>
              ) : (
                issues.map((issue) => (
                  <tr key={issue.id} className="border-b border-white/5 hover:bg-white/5">
                    <td className="px-4 py-3 whitespace-nowrap">{new Date(issue.recorded_at).toLocaleString()}</td>
                    <td className="px-4 py-3 font-medium text-white">{issue.symbol}</td>
                    <td className="px-4 py-3">{issue.check_type}</td>
                    <td className="px-4 py-3">
                       <span className={`px-2 py-1 rounded-full text-xs ${
                        issue.severity === 'CRITICAL' ? 'bg-error-500/20 text-error-400' : 'bg-warn-500/20 text-warn-400'
                      }`}>
                        {issue.severity}
                      </span>
                    </td>
                    <td className="px-4 py-3">{issue.message}</td>
                  </tr>
                ))
              )}
              {!loading && issues.length === 0 && (
                <tr><td colSpan={5} className="text-center py-4">No recent anomalies detected.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
