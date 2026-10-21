"use client";

import { useState, useMemo } from 'react';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { 
  BoltIcon, 
  FunnelIcon, 
  ShieldCheckIcon, 
  ShieldExclamationIcon, 
  ChartBarIcon, 
  InformationCircleIcon, 
  ArrowTrendingUpIcon, 
  ExclamationCircleIcon,
  CheckCircleIcon,
  XCircleIcon,
  ClockIcon
} from '@heroicons/react/24/outline';
import { useBreakouts } from '@/lib/api/hooks';

export default function SignalsPage() {
  const { 
    candidates, 
    stageCounts, 
    bullishBreadth, 
    sectorHeatmap, 
    totalScreened, 
    isLoading, 
    mutate 
  } = useBreakouts();

  const [selectedStage, setSelectedStage] = useState<string>('ALL');
  const [selectedSector, setSelectedSector] = useState<string>('ALL');
  const [shariahOnly, setShariahOnly] = useState<boolean>(true);
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [selectedCandidate, setSelectedCandidate] = useState<any | null>(null);

  // Filter candidates
  const filteredCandidates = useMemo(() => {
    return (candidates || []).filter((cand: any) => {
      if (selectedStage !== 'ALL' && cand.stage !== selectedStage) return false;
      if (selectedSector !== 'ALL' && cand.sector !== selectedSector) return false;
      if (shariahOnly && cand.shariah_status !== 'PASS') return false;
      if (searchQuery.trim() !== '') {
        const q = searchQuery.toLowerCase();
        if (!cand.symbol.toLowerCase().includes(q) && !(cand.sector || '').toLowerCase().includes(q)) {
          return false;
        }
      }
      return true;
    });
  }, [candidates, selectedStage, selectedSector, shariahOnly, searchQuery]);

  const activeSelected = selectedCandidate || (filteredCandidates.length > 0 ? filteredCandidates[0] : null);

  // Unique sectors for filter dropdown
  const availableSectors = useMemo(() => {
    const set = new Set<string>();
    (candidates || []).forEach((c: any) => {
      if (c.sector) set.add(c.sector);
    });
    return Array.from(set).sort();
  }, [candidates]);

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Breakout Research Engine</h1>
            <span className="px-2.5 py-0.5 rounded-full text-xs font-semibold bg-blue-900/60 text-blue-300 border border-blue-700/50">
              NSE Cash Universe
            </span>
          </div>
          <p className="text-slate-400 mt-1">
            Auditable price structure, volume accumulation proxies, support/resistance, and Shariah-gated setups
          </p>
        </div>

        <button 
          onClick={() => mutate()}
          className="self-start md:self-auto px-4 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-xs font-medium transition-colors flex items-center gap-2"
        >
          <ClockIcon className="w-4 h-4 text-blue-400" />
          <span>Refresh Live Scanner</span>
        </button>
      </div>

      {/* Stage Breakdown & Market Breadth Metrics */}
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4">
        <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
          <p className="text-xs text-slate-500 uppercase font-semibold">Universe Breadth</p>
          <p className="text-2xl font-bold text-white mt-1">{bullishBreadth}%</p>
          <p className="text-[11px] text-slate-400 mt-0.5">Bullish structure</p>
        </div>

        <div className="p-4 rounded-xl bg-emerald-950/20 border border-emerald-900/40">
          <p className="text-xs text-emerald-400 uppercase font-semibold">Confirmed Breakouts</p>
          <p className="text-2xl font-bold text-emerald-400 mt-1">{stageCounts?.CONFIRMED_BREAKOUT || 0}</p>
          <p className="text-[11px] text-slate-400 mt-0.5">Closed &gt; Resistance with RVOL</p>
        </div>

        <div className="p-4 rounded-xl bg-blue-950/20 border border-blue-900/40">
          <p className="text-xs text-blue-400 uppercase font-semibold">Breakout Candidates</p>
          <p className="text-2xl font-bold text-blue-400 mt-1">{stageCounts?.BREAKOUT_CANDIDATE || 0}</p>
          <p className="text-[11px] text-slate-400 mt-0.5">Tight consolidation &le; 1.5% away</p>
        </div>

        <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
          <p className="text-xs text-slate-400 uppercase font-semibold">Watchlist / Base</p>
          <p className="text-2xl font-bold text-slate-300 mt-1">{stageCounts?.WATCHLIST || 0}</p>
          <p className="text-[11px] text-slate-400 mt-0.5">Approaching key levels</p>
        </div>

        <div className="p-4 rounded-xl bg-slate-900/60 border border-slate-800">
          <p className="text-xs text-slate-500 uppercase font-semibold">Total Evaluated</p>
          <p className="text-2xl font-bold text-white mt-1">{totalScreened}</p>
          <p className="text-[11px] text-slate-400 mt-0.5">Active NSE securities</p>
        </div>
      </div>

      {/* Sector Strength Heatmap */}
      {sectorHeatmap && sectorHeatmap.length > 0 && (
        <Card>
          <CardHeader 
            title="Sector Performance & Relative Strength Heatmap" 
            subtitle="Aggregated breakout score and participation across industries" 
          />
          <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3 mt-4">
            {sectorHeatmap.map((sec: any) => {
              const isSelected = selectedSector === sec.sector;
              const strengthColor = sec.strength === 'STRONG' 
                ? 'bg-emerald-950/40 border-emerald-500/50 text-emerald-300' 
                : (sec.strength === 'MODERATE' 
                    ? 'bg-blue-950/40 border-blue-500/40 text-blue-300' 
                    : 'bg-slate-900/60 border-slate-800 text-slate-400');

              return (
                <div 
                  key={sec.sector}
                  onClick={() => setSelectedSector(isSelected ? 'ALL' : sec.sector)}
                  className={`p-3 rounded-xl border cursor-pointer transition-all ${strengthColor} ${
                    isSelected ? 'ring-2 ring-white/50 shadow-md scale-102' : 'hover:border-slate-600'
                  }`}
                >
                  <div className="flex justify-between items-start">
                    <span className="text-xs font-semibold truncate" title={sec.sector}>{sec.sector}</span>
                    <span className="text-[10px] uppercase font-mono px-1 rounded bg-black/40">{sec.count}</span>
                  </div>
                  <div className="mt-2 flex items-baseline justify-between">
                    <span className="text-lg font-bold">{sec.average_score.toFixed(0)}</span>
                    <span className="text-[10px] font-semibold">{sec.strength}</span>
                  </div>
                </div>
              );
            })}
          </div>
        </Card>
      )}

      {/* Main Grid: Controls + Candidates List + Setup Analysis */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        
        {/* Left 2 Cols: Controls & Candidates */}
        <div className="lg:col-span-2 space-y-4">
          {/* Filter Bar */}
          <div className="p-4 rounded-xl bg-slate-900/80 border border-slate-800 space-y-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex flex-wrap items-center gap-1.5 text-xs">
                {['ALL', 'CONFIRMED_BREAKOUT', 'BREAKOUT_CANDIDATE', 'WATCHLIST'].map((stage) => (
                  <button
                    key={stage}
                    onClick={() => setSelectedStage(stage)}
                    className={`px-3 py-1.5 rounded-lg font-medium transition-colors ${
                      selectedStage === stage 
                        ? 'bg-blue-600 text-white' 
                        : 'bg-slate-800 text-slate-400 hover:text-white'
                    }`}
                  >
                    {stage === 'ALL' ? 'All Stages' : stage.replace('_', ' ')}
                  </button>
                ))}
              </div>

              {/* Shariah Hard Gate Toggle */}
              <label className="flex items-center gap-2 cursor-pointer text-xs select-none">
                <input 
                  type="checkbox" 
                  checked={shariahOnly} 
                  onChange={(e) => setShariahOnly(e.target.checked)} 
                  className="rounded border-slate-700 bg-slate-800 text-emerald-500 focus:ring-emerald-500"
                />
                <span className="text-emerald-400 font-medium flex items-center gap-1">
                  <ShieldCheckIcon className="w-4 h-4" />
                  Shariah Compliant Only
                </span>
              </label>
            </div>

            <div className="flex flex-col sm:flex-row gap-3 pt-2 border-t border-slate-800 text-xs">
              <input 
                type="text" 
                placeholder="Search symbol (e.g. INFY, HCLTECH)..." 
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="flex-1 bg-slate-800/80 border border-slate-700 rounded-lg px-3 py-1.5 text-white placeholder-slate-500 focus:outline-none focus:border-blue-500"
              />
              <select
                value={selectedSector}
                onChange={(e) => setSelectedSector(e.target.value)}
                className="bg-slate-800/80 border border-slate-700 rounded-lg px-3 py-1.5 text-slate-200 focus:outline-none focus:border-blue-500"
              >
                <option value="ALL">All Sectors ({availableSectors.length})</option>
                {availableSectors.map((sec) => (
                  <option key={sec} value={sec}>{sec}</option>
                ))}
              </select>
            </div>
          </div>

          {/* Candidates List */}
          {isLoading ? (
            <div className="text-slate-500 py-16 text-center border border-slate-800 rounded-xl bg-slate-900/40">
              Evaluating candlestick compression, volume demand proxies, and resistance levels...
            </div>
          ) : filteredCandidates.length > 0 ? (
            <div className="space-y-3">
              {filteredCandidates.map((cand: any) => {
                const isSelected = activeSelected?.symbol === cand.symbol;
                const isConfirmed = cand.stage === 'CONFIRMED_BREAKOUT';
                const isCandidate = cand.stage === 'BREAKOUT_CANDIDATE';
                const isPass = cand.shariah_status === 'PASS';

                return (
                  <div
                    key={cand.symbol}
                    onClick={() => setSelectedCandidate(cand)}
                    className={`p-4 rounded-xl border transition-all cursor-pointer ${
                      isSelected 
                        ? 'bg-slate-800/90 border-blue-500 ring-1 ring-blue-500' 
                        : 'bg-slate-900/60 border-slate-800 hover:border-slate-700'
                    }`}
                  >
                    <div className="flex flex-col sm:flex-row sm:items-start justify-between gap-3">
                      <div>
                        <div className="flex flex-wrap items-center gap-2 mb-1">
                          <span className="text-lg font-bold text-white">{cand.symbol}</span>
                          <span className="text-xs text-slate-400 font-mono">₹{cand.current_price?.toFixed(2)}</span>
                          <span className={`text-[11px] font-semibold px-2 py-0.5 rounded ${
                            isConfirmed 
                              ? 'bg-emerald-950/60 text-emerald-300 border border-emerald-800/60' 
                              : (isCandidate 
                                  ? 'bg-blue-950/60 text-blue-300 border border-blue-800/60' 
                                  : 'bg-slate-800 text-slate-400 border border-slate-700')
                          }`}>
                            {cand.stage.replace('_', ' ')}
                          </span>
                          <Badge variant={isPass ? 'pass' : (cand.shariah_status === 'FAIL' ? 'fail' : 'neutral')}>
                            {cand.shariah_status}
                          </Badge>
                        </div>
                        <p className="text-xs text-slate-400">
                          Sector: <strong className="text-slate-300">{cand.sector || 'Unassigned'}</strong> • 
                          Dist to Resistance: <strong className={cand.distance_to_resistance_pct <= 0.5 ? 'text-emerald-400' : 'text-slate-300'}>
                            {cand.distance_to_resistance_pct > 0 ? `+${cand.distance_to_resistance_pct.toFixed(2)}%` : `${cand.distance_to_resistance_pct.toFixed(2)}%`}
                          </strong>
                        </p>
                      </div>

                      <div className="text-left sm:text-right flex sm:flex-col justify-between items-center sm:items-end">
                        <div className="flex items-center gap-1.5">
                          <span className="text-xs text-slate-400">Score:</span>
                          <span className={`text-base font-bold ${cand.overall_score >= 70 ? 'text-emerald-400' : (cand.overall_score > 0 ? 'text-blue-400' : 'text-slate-500')}`}>
                            {cand.overall_score}/100
                          </span>
                        </div>
                        <span className="text-[11px] text-slate-400">
                          RVOL: <strong className={cand.rvol >= 1.4 ? 'text-emerald-400' : 'text-slate-300'}>{cand.rvol?.toFixed(2)}x</strong>
                        </span>
                      </div>
                    </div>

                    {/* Trade Levels Grid */}
                    <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 mt-3 pt-3 border-t border-slate-800 text-xs">
                      <div>
                        <span className="text-slate-500 block text-[10px]">Entry Zone</span>
                        <span className="font-semibold text-white">₹{cand.entry_zone?.[0]?.toFixed(2)} - ₹{cand.entry_zone?.[1]?.toFixed(2)}</span>
                      </div>
                      <div>
                        <span className="text-slate-500 block text-[10px]">Invalidation Level</span>
                        <span className="font-semibold text-rose-400">₹{cand.invalidation_level?.toFixed(2)}</span>
                      </div>
                      <div>
                        <span className="text-slate-500 block text-[10px]">Target 1 (1.5R)</span>
                        <span className="font-semibold text-emerald-400">₹{cand.target_1?.toFixed(2)}</span>
                      </div>
                      <div>
                        <span className="text-slate-500 block text-[10px]">Target 2 (3.0R)</span>
                        <span className="font-semibold text-emerald-300">₹{cand.target_2?.toFixed(2)}</span>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          ) : (
            <div className="text-slate-500 py-16 text-center border border-slate-800 rounded-xl bg-slate-900/40">
              No breakout candidates matched your filters.
            </div>
          )}
        </div>

        {/* Right Col: Detailed Research & Evidence Dossier */}
        <div className="lg:col-span-1 space-y-6">
          <Card className="border-blue-500/20 shadow-[0_0_30px_rgba(59,130,246,0.05)]">
            <CardHeader 
              title={activeSelected ? `${activeSelected.symbol} Research Dossier` : "Setup Evidence"} 
              subtitle="Auditable signals, score breakdown, and risk parameters" 
              action={<BoltIcon className="w-5 h-5 text-amber-400" />} 
            />

            {!activeSelected ? (
              <div className="py-12 text-center text-slate-500">
                Select a candidate from the left to inspect its quantitative breakout evidence and trade boundaries.
              </div>
            ) : (
              <div className="space-y-5 text-xs">
                {/* Stage Banner */}
                <div className={`p-3.5 rounded-xl border ${
                  activeSelected.stage === 'CONFIRMED_BREAKOUT'
                    ? 'bg-emerald-950/40 border-emerald-500/40 text-emerald-300'
                    : (activeSelected.stage === 'BREAKOUT_CANDIDATE'
                        ? 'bg-blue-950/40 border-blue-500/40 text-blue-300'
                        : 'bg-slate-800/60 border-slate-700 text-slate-300')
                }`}>
                  <div className="flex justify-between items-center mb-1">
                    <span className="uppercase text-[11px] font-semibold tracking-wider">Classification Stage</span>
                    <span className="px-2 py-0.5 rounded bg-black/40 font-bold">{activeSelected.stage.replace('_', ' ')}</span>
                  </div>
                  <div className="mt-2 pt-2 border-t border-white/10 flex justify-between items-center">
                    <span>Overall Setup Score</span>
                    <span className="text-base font-bold text-white">{activeSelected.overall_score}/100</span>
                  </div>
                </div>

                {/* Score Breakdown (Transparent & Explainable) */}
                {activeSelected.scoreBreakdown && Object.keys(activeSelected.scoreBreakdown).length > 0 && (
                  <div className="space-y-2">
                    <h5 className="font-semibold text-slate-400 uppercase text-[11px] tracking-wider">Explainable Score Breakdown</h5>
                    <div className="bg-slate-800/40 p-3 rounded-xl border border-slate-700/50 space-y-2">
                      {Object.entries(activeSelected.scoreBreakdown).map(([key, val]: [string, any]) => (
                        <div key={key} className="flex justify-between items-center">
                          <span className="text-slate-400 capitalize">{key.replace('_', ' ')}</span>
                          <strong className="text-white">{val} pts</strong>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {/* Quantitative Parameters */}
                <div className="space-y-2">
                  <h5 className="font-semibold text-slate-400 uppercase text-[11px] tracking-wider">Structure & Volatility Metrics</h5>
                  <div className="bg-slate-800/40 p-3 rounded-xl border border-slate-700/50 space-y-2">
                    <div className="flex justify-between items-center">
                      <span className="text-slate-400">ATR (14 periods)</span>
                      <strong className="text-white">₹{activeSelected.atr?.toFixed(2)}</strong>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-slate-400">Relative Volume (RVOL)</span>
                      <strong className={activeSelected.rvol >= 1.4 ? 'text-emerald-400' : 'text-slate-200'}>
                        {activeSelected.rvol?.toFixed(2)}x
                      </strong>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-slate-400">Consolidation Length</span>
                      <strong className="text-white">{activeSelected.consolidation_bars} candles</strong>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-slate-400">Band Width Compression</span>
                      <strong className="text-white">{(activeSelected.bollinger_band_width * 100)?.toFixed(2)}%</strong>
                    </div>
                    <div className="flex justify-between items-center">
                      <span className="text-slate-400">Volume Demand Proxy</span>
                      <strong className="text-white">{activeSelected.volume_demand_proxy?.toFixed(3)}</strong>
                    </div>
                  </div>
                </div>

                {/* Level 2 Order Book Status Caveat */}
                <div className="p-3 rounded-lg bg-slate-800/30 border border-slate-700/50 text-slate-400 flex items-start gap-2">
                  <InformationCircleIcon className="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
                  <div>
                    <strong className="text-slate-200 block mb-0.5">Order-Book Depth Status</strong>
                    {activeSelected.order_book_depth_status || "DATA_UNAVAILABLE (No Level 2 License)"}.
                    <p className="mt-1 text-[11px] text-slate-500">
                      Demand proxy is calculated mathematically from bar closing location and volume accumulation, NOT from fabricated Level 2 depth.
                    </p>
                  </div>
                </div>

                {/* Setup Reasons & Evidence */}
                {activeSelected.reasons && activeSelected.reasons.length > 0 && (
                  <div className="space-y-2">
                    <h5 className="font-semibold text-slate-400 uppercase text-[11px] tracking-wider flex items-center gap-1.5">
                      <CheckCircleIcon className="w-4 h-4 text-emerald-400" />
                      <span>Setup Rationale</span>
                    </h5>
                    <ul className="space-y-1.5">
                      {activeSelected.reasons.map((r: string, idx: number) => (
                        <li key={idx} className="p-2.5 rounded-lg bg-slate-800/40 border border-slate-700/50 text-slate-300">
                          {r}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {/* Invalidation Conditions */}
                {activeSelected.invalidation_conditions && activeSelected.invalidation_conditions.length > 0 && (
                  <div className="space-y-2">
                    <h5 className="font-semibold text-slate-400 uppercase text-[11px] tracking-wider flex items-center gap-1.5">
                      <XCircleIcon className="w-4 h-4 text-rose-400" />
                      <span>Invalidation Rules</span>
                    </h5>
                    <ul className="space-y-1.5">
                      {activeSelected.invalidation_conditions.map((cond: string, idx: number) => (
                        <li key={idx} className="p-2.5 rounded-lg bg-rose-950/20 border border-rose-900/40 text-rose-300">
                          {cond}
                        </li>
                      ))}
                    </ul>
                  </div>
                )}

                {/* Shariah Hard Gate Notice */}
                <div className="p-3 rounded-lg bg-slate-800/30 border border-slate-700/50">
                  <div className="flex items-center gap-1.5 font-semibold text-slate-200 mb-1">
                    <ShieldCheckIcon className="w-4 h-4 text-emerald-400" />
                    <span>Shariah Hard Gate</span>
                  </div>
                  <p className="text-slate-400">
                    Status: <strong className={activeSelected.shariah_status === 'PASS' ? 'text-emerald-400' : 'text-rose-400'}>
                      {activeSelected.shariah_status}
                    </strong>
                  </p>
                  <p className="text-[11px] text-slate-500 mt-1">
                    {activeSelected.shariah_status === 'PASS'
                      ? "Passed AAOIFI financial ratio screens (Debt/Cap, Interest Income, Cash, Receivables)."
                      : "Strict trading block: Non-compliant securities receive a score of 0 and cannot be recommended regardless of technical breakout conviction."}
                  </p>
                </div>

              </div>
            )}
          </Card>
        </div>

      </div>
    </div>
  );
}
