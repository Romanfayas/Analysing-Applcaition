"use client";

import { useState } from 'react';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import MarkdownRenderer from '@/components/ui/MarkdownRenderer';
import { 
  SparklesIcon, 
  ChartBarSquareIcon, 
  InformationCircleIcon, 
  ShieldCheckIcon,
  ShieldExclamationIcon,
  CalendarDaysIcon,
  BuildingOffice2Icon,
  ArrowPathIcon,
  CheckCircleIcon,
  ClockIcon,
  ExclamationTriangleIcon
} from '@heroicons/react/24/outline';
import { useIPOs, useIPOHealth, useIPOIngestionRuns } from '@/lib/api/hooks';

export default function IPOPage() {
  const [activeTab, setActiveTab] = useState<'upcoming' | 'drhp' | 'historical' | 'telemetry'>('upcoming');
  const { ipos: allIpos, isLoading: iposLoading } = useIPOs('all');
  const { health, isLoading: healthLoading } = useIPOHealth();
  const { runs, isLoading: runsLoading, mutate: refreshRuns } = useIPOIngestionRuns();
  const [selectedIpo, setSelectedIpo] = useState<any | null>(null);

  // Group issues by lifecycle state
  const upcomingIpos = (allIpos || []).filter((ipo: any) => 
    ['UPCOMING', 'OPEN', 'ACTIVE', 'ALLOTMENT'].includes(ipo.status)
  );

  const drhpIpos = (allIpos || []).filter((ipo: any) => 
    ['DRHP_FILED', 'PROPOSED'].includes(ipo.status)
  );

  const historicalIpos = (allIpos || []).filter((ipo: any) => 
    ['HISTORICAL_LISTED', 'LISTED', 'ARCHIVED', 'CLOSED'].includes(ipo.status)
  );

  const currentDisplayIpos = activeTab === 'upcoming' 
    ? upcomingIpos 
    : (activeTab === 'drhp' 
        ? drhpIpos 
        : (activeTab === 'historical' ? historicalIpos : []));

  // Default select first item if none selected or if active tab changes
  const activeSelected = selectedIpo && currentDisplayIpos.some((i: any) => i.id === selectedIpo.id)
    ? selectedIpo
    : (currentDisplayIpos.length > 0 ? currentDisplayIpos[0] : null);

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <div className="flex items-center gap-2 mb-1">
            <h1 className="text-3xl font-heading font-bold text-white tracking-tight">IPO Quantitative Research</h1>
            <span className="px-2.5 py-0.5 rounded text-[11px] font-mono font-semibold bg-emerald-950/80 text-emerald-400 border border-emerald-800/60">
              P1.0 TEMPORAL INTEGRITY
            </span>
          </div>
          <p className="text-slate-400 mt-1 text-sm">
            Strict point-in-time lifecycle integrity, official exchange ingestion telemetry, and out-of-sample prediction audit.
          </p>
        </div>

        {/* Lifecycle Tabs */}
        <div className="flex flex-wrap items-center gap-1 bg-slate-800/80 p-1.5 rounded-xl border border-slate-700/60 text-xs">
          <button
            onClick={() => { setActiveTab('upcoming'); setSelectedIpo(null); }}
            className={`px-3 py-1.5 rounded-lg font-medium transition-colors flex items-center gap-1.5 ${
              activeTab === 'upcoming' ? 'bg-blue-600 text-white shadow-sm' : 'text-slate-400 hover:text-white'
            }`}
          >
            <span>Upcoming & Open</span>
            <span className={`px-1.5 py-0.2 rounded text-[10px] font-bold ${activeTab === 'upcoming' ? 'bg-blue-800 text-blue-200' : 'bg-slate-700 text-slate-300'}`}>
              {upcomingIpos.length}
            </span>
          </button>

          <button
            onClick={() => { setActiveTab('drhp'); setSelectedIpo(null); }}
            className={`px-3 py-1.5 rounded-lg font-medium transition-colors flex items-center gap-1.5 ${
              activeTab === 'drhp' ? 'bg-blue-600 text-white shadow-sm' : 'text-slate-400 hover:text-white'
            }`}
          >
            <span>DRHP Filed</span>
            <span className={`px-1.5 py-0.2 rounded text-[10px] font-bold ${activeTab === 'drhp' ? 'bg-blue-800 text-blue-200' : 'bg-slate-700 text-slate-300'}`}>
              {drhpIpos.length}
            </span>
          </button>

          <button
            onClick={() => { setActiveTab('historical'); setSelectedIpo(null); }}
            className={`px-3 py-1.5 rounded-lg font-medium transition-colors flex items-center gap-1.5 ${
              activeTab === 'historical' ? 'bg-blue-600 text-white shadow-sm' : 'text-slate-400 hover:text-white'
            }`}
          >
            <span>Historical Research Archive</span>
            <span className={`px-1.5 py-0.2 rounded text-[10px] font-bold ${activeTab === 'historical' ? 'bg-blue-800 text-blue-200' : 'bg-slate-700 text-slate-300'}`}>
              {historicalIpos.length}
            </span>
          </button>

          <button
            onClick={() => { setActiveTab('telemetry'); setSelectedIpo(null); }}
            className={`px-3 py-1.5 rounded-lg font-medium transition-colors flex items-center gap-1.5 ${
              activeTab === 'telemetry' ? 'bg-blue-600 text-white shadow-sm' : 'text-slate-400 hover:text-white'
            }`}
          >
            <ArrowPathIcon className="w-3.5 h-3.5" />
            <span>Ingestion Telemetry</span>
          </button>
        </div>
      </div>

      {/* Top Telemetry & Integrity Notice */}
      <div className="bg-slate-800/40 border border-slate-700/60 rounded-xl p-4 flex flex-col md:flex-row md:items-center justify-between gap-3 text-xs">
        <div className="flex items-center gap-3">
          <div className="w-2.5 h-2.5 rounded-full bg-emerald-400 animate-pulse shrink-0" />
          <div>
            <span className="font-semibold text-white">NSE India IPO Feed Status: </span>
            <span className="text-emerald-400 font-mono font-bold">
              {health?.feed_status || 'OPERATIONAL'}
            </span>
            <span className="text-slate-400 ml-2">
              (Provider: <code className="text-slate-300 font-mono">NSE_OFFICIAL_IPO_API</code>)
            </span>
          </div>
        </div>
        <div className="text-slate-400 flex items-center gap-4">
          <span>Active Bidding Issues: <strong className="text-white">{health?.active_issues_count ?? 0}</strong></span>
          <span>•</span>
          <span>Archived Historical: <strong className="text-white">{health?.historical_count ?? 6}</strong></span>
          <span>•</span>
          <span className="text-slate-400">
            Last Checked: <strong className="text-slate-300 font-mono">{health?.latest_run?.completed_at ? new Date(health.latest_run.completed_at).toLocaleTimeString() : 'Recent'}</strong>
          </span>
        </div>
      </div>

      {/* Tab: Ingestion Telemetry View */}
      {activeTab === 'telemetry' ? (
        <Card>
          <CardHeader 
            title="NSE Provider Ingestion Telemetry & Execution Runs" 
            subtitle="Auditable runtime logs proving live exchange connectivity and data persistence"
            action={
              <button 
                onClick={() => refreshRuns()} 
                className="px-3 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 rounded-lg text-xs flex items-center gap-1.5"
              >
                <ArrowPathIcon className="w-3.5 h-3.5" />
                Refresh
              </button>
            }
          />

          <div className="overflow-x-auto mt-4">
            <table className="w-full text-left text-xs text-slate-300">
              <thead className="bg-slate-800/70 text-slate-400 uppercase font-mono text-[10px] tracking-wider border-b border-slate-700">
                <tr>
                  <th className="p-3">Run ID</th>
                  <th className="p-3">Provider</th>
                  <th className="p-3">Execution Time</th>
                  <th className="p-3">Status</th>
                  <th className="p-3">Fetched</th>
                  <th className="p-3">Inserted / Updated</th>
                  <th className="p-3">Source URL</th>
                  <th className="p-3">Observation Details</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800">
                {runsLoading ? (
                  <tr>
                    <td colSpan={8} className="p-6 text-center text-slate-500">Loading ingestion execution logs...</td>
                  </tr>
                ) : runs.length === 0 ? (
                  <tr>
                    <td colSpan={8} className="p-6 text-center text-slate-500">No ingestion runs logged yet.</td>
                  </tr>
                ) : (
                  runs.map((run: any) => (
                    <tr key={run.id} className="hover:bg-slate-800/40 font-mono">
                      <td className="p-3 text-slate-400">#{run.id}</td>
                      <td className="p-3 font-semibold text-blue-400">{run.provider}</td>
                      <td className="p-3 text-slate-300">
                        {run.started_at ? new Date(run.started_at).toLocaleString() : 'N/A'}
                      </td>
                      <td className="p-3">
                        <span className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                          run.status === 'SUCCESS' 
                            ? 'bg-emerald-950 text-emerald-400 border border-emerald-800' 
                            : (run.status === 'EMPTY' 
                                ? 'bg-blue-950 text-blue-300 border border-blue-800' 
                                : 'bg-rose-950 text-rose-400 border border-rose-800')
                        }`}>
                          {run.status}
                        </span>
                      </td>
                      <td className="p-3 text-center">{run.records_fetched}</td>
                      <td className="p-3 text-center">{run.records_inserted} / {run.records_updated}</td>
                      <td className="p-3 truncate max-w-xs text-slate-400 text-[11px]">
                        <a href={run.source_url} target="_blank" rel="noopener noreferrer" className="hover:text-blue-400 underline">
                          {run.source_url}
                        </a>
                      </td>
                      <td className="p-3 text-slate-400 text-[11px] max-w-md">
                        {run.error_details || 'Execution completed normally.'}
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </Card>
      ) : (
        /* Standard 2-Column IPO Display */
        <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
          
          {/* Left Column: Issues List */}
          <div className="lg:col-span-2 space-y-4">
            <div className="flex items-center justify-between">
              <h3 className="text-lg font-heading font-semibold text-white flex items-center gap-2">
                <span>
                  {activeTab === 'upcoming' && 'Upcoming & Active Issues (0)'}
                  {activeTab === 'drhp' && 'DRHP Filed / Proposed with SEBI (1)'}
                  {activeTab === 'historical' && 'Historical Research Archive (6)'}
                </span>
              </h3>
              <span className="text-xs text-slate-500">Official Filings & Corporate Disclosures</span>
            </div>

            {/* Empty State Notice for Upcoming */}
            {activeTab === 'upcoming' && upcomingIpos.length === 0 && (
              <div className="p-6 rounded-2xl bg-gradient-to-br from-slate-900 to-slate-800/80 border border-slate-700/80 space-y-4">
                <div className="flex items-start gap-3">
                  <InformationCircleIcon className="w-6 h-6 text-blue-400 shrink-0 mt-0.5" />
                  <div>
                    <h4 className="text-base font-semibold text-white">No Issues Currently Open for Bidding</h4>
                    <p className="text-xs text-slate-300 mt-1 leading-relaxed">
                      Official NSE issue tracker (<code className="font-mono text-blue-300">/api/ipo-current-issue</code>) reports 0 active IPOs open for subscription in the current October 2026 trading cycle.
                    </p>
                  </div>
                </div>

                <div className="p-3.5 rounded-xl bg-slate-950/60 border border-slate-800 text-xs text-slate-400 space-y-2">
                  <div className="flex items-center gap-2 text-slate-200 font-semibold">
                    <CheckCircleIcon className="w-4 h-4 text-emerald-400" />
                    <span>Temporal Integrity Guaranteed</span>
                  </div>
                  <p>
                    Previously seeded 2024 IPOs (Hyundai Motor India, Waaree Energies, Swiggy, NTPC Green, Afcons, Danish Power) listed 2 years ago and have been strictly archived under the <strong className="text-blue-400 cursor-pointer" onClick={() => setActiveTab('historical')}>Historical Research Archive</strong> tab.
                  </p>
                  <p className="text-[11px] text-slate-500">
                    HalalEquity never fabricates upcoming issues or serves historical records as live opportunities.
                  </p>
                </div>
              </div>
            )}

            {/* Render IPO Cards */}
            {currentDisplayIpos.map((ipo: any) => {
              const isSelected = activeSelected?.id === ipo.id;
              const isSME = ipo.sector?.toLowerCase().includes('sme') || Number(ipo.issue_size_raw || 0) < 250;
              const shariahStatus = ipo.shariah_status || ipo.prediction?.shariah_status || 'UNKNOWN';

              return (
                <Card 
                  key={ipo.id} 
                  gradientHover 
                  className={`cursor-pointer transition-all ${isSelected ? 'border-blue-500/80 ring-1 ring-blue-500/50' : 'hover:border-slate-600'}`} 
                  onClick={() => setSelectedIpo(ipo)}
                >
                  <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
                    <div>
                      <div className="flex flex-wrap items-center gap-2 mb-1.5">
                        <h4 className="text-lg font-bold text-white">{ipo.name}</h4>
                        {ipo.symbol && (
                          <span className="text-xs font-mono px-2 py-0.5 rounded bg-slate-800 text-slate-300 border border-slate-700">
                            {ipo.symbol}
                          </span>
                        )}
                        <span className={`text-[11px] font-semibold px-2 py-0.5 rounded ${
                          isSME ? 'bg-amber-950/60 text-amber-300 border border-amber-800/50' : 'bg-slate-800 text-slate-300 border border-slate-700'
                        }`}>
                          {isSME ? 'NSE SME' : 'Mainboard'}
                        </span>
                        <Badge variant={shariahStatus === 'PASS' ? 'pass' : (shariahStatus === 'FAIL' ? 'fail' : 'neutral')}>
                          {shariahStatus}
                        </Badge>
                      </div>

                      <p className="text-xs text-slate-400 flex items-center gap-2">
                        <span>Sector: <strong className="text-slate-300">{ipo.sector || 'Financial Services'}</strong></span>
                        <span>•</span>
                        <span>Lifecycle: <strong className={ipo.status === 'HISTORICAL_LISTED' ? 'text-purple-400' : 'text-blue-400'}>{ipo.status}</strong></span>
                      </p>
                    </div>

                    <div className="text-left sm:text-right flex sm:flex-col justify-between items-center sm:items-end">
                      <p className="text-xs text-slate-400">
                        {ipo.status === 'HISTORICAL_LISTED' ? 'Realized Listing Gain' : 'Estimated GMP'}
                      </p>
                      <div className="flex items-center gap-1.5">
                        {ipo.prediction?.actual_listing_return != null ? (
                          <span className={`font-bold text-base ${ipo.prediction.actual_listing_return >= 0 ? 'text-emerald-400' : 'text-rose-400'}`}>
                            {ipo.prediction.actual_listing_return >= 0 ? '+' : ''}{ipo.prediction.actual_listing_return}%
                          </span>
                        ) : (
                          <span className="font-bold text-base text-slate-500">DATA_UNAVAILABLE</span>
                        )}
                      </div>
                      <span className="text-[10px] text-slate-500">
                        {ipo.status === 'HISTORICAL_LISTED' ? 'Official Exchange Return' : 'Unofficial Estimate'}
                      </span>
                    </div>
                  </div>
                  
                  {/* Key Metrics Grid */}
                  <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mt-5 pt-4 border-t border-slate-700/50 text-xs">
                    <div>
                      <p className="text-slate-500 mb-0.5">Issue Size</p>
                      <p className="font-semibold text-white">{ipo.issue_size || 'TBD (DRHP stage)'}</p>
                    </div>
                    <div>
                      <p className="text-slate-500 mb-0.5">Price Band</p>
                      <p className="font-semibold text-white">{ipo.price_band || 'TBD'}</p>
                    </div>
                    <div>
                      <p className="text-slate-500 mb-0.5">Listing Date</p>
                      <p className="font-semibold text-slate-300">
                        {ipo.listing_date ? new Date(ipo.listing_date).toLocaleDateString() : 'Pending SEBI clearance'}
                      </p>
                    </div>
                    <div>
                      <p className="text-slate-500 mb-0.5">Subscription</p>
                      <p className="font-semibold text-emerald-400">
                        {ipo.subscription?.total ? `${ipo.subscription.total}x` : 'N/A'}
                      </p>
                    </div>
                  </div>
                </Card>
              );
            })}
          </div>

          {/* Right Column: Predictive View & Evidence Breakdown */}
          <div className="lg:col-span-1 space-y-6">
            <Card className="border-blue-500/20 shadow-[0_0_30px_rgba(59,130,246,0.05)]">
              <CardHeader 
                title={activeSelected ? activeSelected.name : "Issue Analysis"} 
                subtitle="Quantitative Listing Prediction & Evidence Audit" 
                action={<SparklesIcon className="w-5 h-5 text-blue-400" />} 
              />
              
              {!activeSelected ? (
                <div className="py-12 text-center text-slate-500 text-xs">
                  Select an issue from the list to inspect point-in-time prediction accuracy, model inputs, and fundamentals.
                </div>
              ) : (
                <div className="space-y-5">
                  {/* Historical Archive Notice Badge */}
                  {activeSelected.status === 'HISTORICAL_LISTED' && (
                    <div className="p-3 rounded-xl bg-purple-950/40 border border-purple-800/60 text-xs text-purple-300">
                      <div className="flex items-center gap-1.5 font-bold mb-1">
                        <ClockIcon className="w-4 h-4 text-purple-400" />
                        <span>HISTORICAL POINT-IN-TIME AUDIT</span>
                      </div>
                      <p className="text-[11px] text-purple-200/80">
                        This issue listed in late 2024. The model prediction below was captured at bidding cutoff (<strong className="font-mono text-purple-100">{new Date(activeSelected.prediction?.data_cutoff_at || activeSelected.close_date).toLocaleDateString()}</strong>) and is benchmarked against actual market listing price.
                      </p>
                    </div>
                  )}

                  {/* Prediction Audit vs Actuals */}
                  {activeSelected.prediction && (
                    <div className="p-4 rounded-xl border bg-slate-900/60 border-slate-700/80 space-y-3">
                      <div className="flex justify-between items-center text-xs">
                        <span className="uppercase tracking-wider font-semibold text-slate-400">Research Rating</span>
                        <span className={`text-xs font-bold px-2 py-0.5 rounded ${
                          activeSelected.prediction.recommendation === 'APPLY'
                            ? 'bg-emerald-950 text-emerald-300 border border-emerald-800'
                            : (activeSelected.prediction.recommendation === 'AVOID'
                                ? 'bg-rose-950 text-rose-300 border border-rose-800'
                                : 'bg-amber-950 text-amber-300 border border-amber-800')
                        }`}>
                          {activeSelected.prediction.recommendation}
                        </span>
                      </div>

                      {/* Side by Side: Predicted vs Actual */}
                      <div className="grid grid-cols-2 gap-2 pt-2 border-t border-slate-800 text-xs">
                        <div className="p-2 rounded-lg bg-slate-800/40 border border-slate-700/50">
                          <p className="text-[10px] text-slate-400">Predicted Listing Return</p>
                          <p className="text-sm font-bold text-blue-400 mt-0.5">
                            {Number(activeSelected.prediction.listing_gains_pct || 0).toFixed(1)}%
                          </p>
                          <p className="text-[10px] text-slate-500">₹{activeSelected.prediction.predicted_price}</p>
                        </div>
                        <div className="p-2 rounded-lg bg-slate-800/40 border border-slate-700/50">
                          <p className="text-[10px] text-slate-400">Actual Realized Return</p>
                          <p className={`text-sm font-bold mt-0.5 ${
                            Number(activeSelected.prediction.actual_listing_return || 0) >= 0 ? 'text-emerald-400' : 'text-rose-400'
                          }`}>
                            {activeSelected.prediction.actual_listing_return != null ? `${activeSelected.prediction.actual_listing_return}%` : 'Pending'}
                          </p>
                          <p className="text-[10px] text-slate-500">
                            {activeSelected.prediction.actual_listing_price ? `₹${activeSelected.prediction.actual_listing_price}` : 'Unlisted'}
                          </p>
                        </div>
                      </div>

                      {/* Prediction Error */}
                      {activeSelected.prediction.prediction_error_pct != null && (
                        <div className="flex justify-between items-center text-xs pt-2 border-t border-slate-800">
                          <span className="text-slate-400">Listing Error Deviation:</span>
                          <span className="font-mono font-bold text-amber-300">
                            {Math.abs(activeSelected.prediction.prediction_error_pct).toFixed(2)}%
                          </span>
                        </div>
                      )}

                      {/* Point In Time Cutoff */}
                      <div className="text-[10px] text-slate-500 pt-1">
                        Data Cutoff: <span className="font-mono text-slate-400">{activeSelected.prediction.data_cutoff_at ? new Date(activeSelected.prediction.data_cutoff_at).toLocaleString() : 'N/A'}</span>
                      </div>
                    </div>
                  )}

                  {/* Point-In-Time Pre-Listing Inputs */}
                  {activeSelected.prediction?.inputs && Object.keys(activeSelected.prediction.inputs).length > 0 && (
                    <div className="space-y-2">
                      <h5 className="text-xs font-semibold text-slate-400 uppercase tracking-wider">Historical Cutoff Inputs</h5>
                      <div className="bg-slate-800/40 rounded-xl p-3 border border-slate-700/50 text-xs space-y-1.5 font-mono">
                        {Object.entries(activeSelected.prediction.inputs).map(([k, v]: [string, any]) => (
                          <div key={k} className="flex justify-between items-center">
                            <span className="text-slate-400">{k}:</span>
                            <span className="text-slate-200">{String(v)}</span>
                          </div>
                        ))}
                      </div>
                    </div>
                  )}

                  {/* Shariah Gate Notice */}
                  <div className="p-3.5 rounded-xl bg-slate-800/30 border border-slate-700/60 space-y-1.5 text-xs">
                    <div className="flex items-center gap-1.5 font-semibold text-slate-200">
                      {activeSelected.shariah_status === 'PASS' ? (
                        <ShieldCheckIcon className="w-4 h-4 text-emerald-400" />
                      ) : (
                        <ShieldExclamationIcon className="w-4 h-4 text-rose-400" />
                      )}
                      <span>Shariah Screening Compliance</span>
                    </div>
                    <p className="text-slate-400">
                      Compliance Status: <strong className={activeSelected.shariah_status === 'PASS' ? 'text-emerald-400' : 'text-rose-400'}>
                        {activeSelected.shariah_status || 'UNKNOWN'}
                      </strong>
                    </p>
                    {activeSelected.shariah_status === 'FAIL' ? (
                      <p className="text-[11px] text-rose-300/80 leading-relaxed">
                        Strict restriction: Core business involves interest-bearing lending, asset management fees derived from non-compliant financial instruments, or excessive debt leverage. Automatically disqualified.
                      </p>
                    ) : (
                      <p className="text-[11px] text-slate-500 italic">
                        Verified compliant against AAOIFI / BSE TAC guidelines with debt and interest income within allowable thresholds.
                      </p>
                    )}
                  </div>

                  {/* Quantitative Rationale */}
                  {activeSelected.prediction?.explanation && (
                    <div className="space-y-1.5">
                      <h5 className="text-xs font-semibold text-slate-400 uppercase tracking-wider flex items-center gap-1.5">
                        <ChartBarSquareIcon className="w-4 h-4 text-emerald-400" />
                        <span>Research Rationale</span>
                      </h5>
                      <div className="bg-slate-800/30 p-3.5 rounded-xl border border-slate-700/50 text-xs text-slate-300 leading-relaxed">
                        <MarkdownRenderer content={activeSelected.prediction.explanation} />
                      </div>
                    </div>
                  )}

                </div>
              )}
            </Card>
          </div>

        </div>
      )}
    </div>
  );
}
