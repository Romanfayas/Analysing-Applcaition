"use client";

import { useState, useMemo } from "react";
import Link from "next/link";
import { useStocks, useMarketDataHealth } from "@/lib/api/hooks";
import { StockListItem } from "@/lib/api/types";

export default function StocksPage() {
  const { stocks, meta, isLoading, isError, mutate } = useStocks();
  const { health, isLoading: isHealthLoading } = useMarketDataHealth();

  const [searchQuery, setSearchQuery] = useState("");
  const [selectedSector, setSelectedSector] = useState("ALL");
  const [selectedShariah, setSelectedShariah] = useState("ALL");
  const [selectedFreshness, setSelectedFreshness] = useState("ALL");

  // Derive unique sectors from actual stock list
  const sectors = useMemo(() => {
    const set = new Set<string>();
    stocks.forEach((s) => {
      if (s.sector && s.sector.trim() !== "") {
        set.add(s.sector.trim());
      }
    });
    return Array.from(set).sort();
  }, [stocks]);

  // Filter stocks
  const filteredStocks = useMemo(() => {
    return stocks.filter((stock) => {
      // Search filter
      if (searchQuery.trim() !== "") {
        const q = searchQuery.toLowerCase();
        const matchesSym = stock.symbol.toLowerCase().includes(q);
        const matchesName = stock.name.toLowerCase().includes(q);
        if (!matchesSym && !matchesName) return false;
      }

      // Sector filter
      if (selectedSector !== "ALL") {
        if (stock.sector !== selectedSector) return false;
      }

      // Shariah filter
      if (selectedShariah !== "ALL") {
        if (stock.shariah !== selectedShariah) return false;
      }

      // Freshness filter
      if (selectedFreshness !== "ALL") {
        if (stock.freshness !== selectedFreshness) return false;
      }

      return true;
    });
  }, [stocks, searchQuery, selectedSector, selectedShariah, selectedFreshness]);

  // Format IST date and time
  const formatIST = (isoString: string | null) => {
    if (!isoString) return "—";
    try {
      const d = new Date(isoString);
      if (isNaN(d.getTime())) return "—";
      return d.toLocaleString("en-IN", {
        timeZone: "Asia/Kolkata",
        day: "2-digit",
        month: "short",
        hour: "2-digit",
        minute: "2-digit",
        hour12: true,
      }) + " IST";
    } catch {
      return "—";
    }
  };

  return (
    <div className="space-y-6">
      {/* Page Title & Actions */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-3xl font-extrabold tracking-tight text-white">Indian Equities</h1>
            {health?.market_session && (
              <span
                className={`px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase tracking-wide border ${
                  health.market_session === "OPEN"
                    ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/30"
                    : health.market_session === "PRE_MARKET"
                    ? "bg-amber-500/10 text-amber-400 border-amber-500/30"
                    : "bg-slate-800 text-slate-400 border-slate-700"
                }`}
              >
                Market {health.market_session}
              </span>
            )}
          </div>
          <p className="text-sm text-slate-400 mt-1">
            Real-time quantitative analysis, AAOIFI Shariah screening, and market telemetry
          </p>
        </div>

        <button
          onClick={() => mutate()}
          className="self-start md:self-auto px-4 py-2 rounded-xl text-xs font-medium bg-slate-800/80 hover:bg-slate-700 text-slate-200 border border-slate-700/60 transition flex items-center gap-2"
        >
          <svg className="w-3.5 h-3.5 text-slate-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          Refresh Feed
        </button>
      </div>

      {/* Telemetry / Market Data Health Banner */}
      <div className="glass-card p-4 border border-slate-800/80 bg-slate-900/60 rounded-2xl">
        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
          <div className="flex items-start sm:items-center gap-3">
            <div
              className={`w-3 h-3 rounded-full mt-1 sm:mt-0 animate-pulse ${
                health?.freshness_status === "FRESH"
                  ? "bg-emerald-500 shadow-lg shadow-emerald-500/50"
                  : health?.freshness_status === "LAST_AVAILABLE"
                  ? "bg-blue-500 shadow-lg shadow-blue-500/50"
                  : health?.freshness_status === "STALE"
                  ? "bg-amber-500 shadow-lg shadow-amber-500/50"
                  : "bg-rose-500 shadow-lg shadow-rose-500/50"
              }`}
            />
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-xs font-semibold text-slate-200">Market Data Telemetry</span>
                <span
                  className={`text-[10px] font-bold px-2 py-0.5 rounded border uppercase tracking-wider ${
                    health?.freshness_status === "FRESH"
                      ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/30"
                      : health?.freshness_status === "LAST_AVAILABLE"
                      ? "bg-blue-500/10 text-blue-400 border-blue-500/30"
                      : health?.freshness_status === "STALE"
                      ? "bg-amber-500/10 text-amber-400 border-amber-500/30"
                      : "bg-rose-500/10 text-rose-400 border-rose-500/30"
                  }`}
                >
                  {health?.freshness_status || "CHECKING"}
                </span>
                <span className="text-[10px] bg-slate-800 text-slate-400 px-2 py-0.5 rounded border border-slate-700">
                  Provider: {health?.provider || "Yahoo Finance"} (Delayed feed)
                </span>
                <span className="text-[10px] bg-amber-500/10 text-amber-400/90 px-2 py-0.5 rounded border border-amber-500/20">
                  Unverified / Non-Realtime
                </span>
              </div>
              <p className="text-[11px] text-slate-400 mt-1">
                Prices sourced via provider intraday intervals. Not guaranteed exchange-certified execution feed.
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-x-6 gap-y-2 text-xs border-t lg:border-t-0 pt-2 lg:pt-0 border-slate-800">
            <div>
              <span className="text-slate-500 block text-[10px] uppercase font-medium">Latest Market Candle</span>
              <span className="font-mono text-slate-200">{formatIST(health?.latest_market_timestamp || null)}</span>
            </div>
            <div>
              <span className="text-slate-500 block text-[10px] uppercase font-medium">Candle Retrieved At</span>
              <span className="font-mono text-slate-200">{formatIST(health?.latest_received_timestamp || null)}</span>
            </div>
            <div>
              <span className="text-slate-500 block text-[10px] uppercase font-medium">Latest DB Ingestion Activity</span>
              <span className="font-mono text-slate-200">{formatIST(health?.latest_ingestion_time || null)}</span>
            </div>
            {health?.total_candles !== undefined && (
              <div>
                <span className="text-slate-500 block text-[10px] uppercase font-medium">Intraday Series Count</span>
                <span className="font-mono text-slate-200">{health.total_candles.toLocaleString()} records</span>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Filter and Search Bar */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
        {/* Search */}
        <div className="relative">
          <input
            type="text"
            placeholder="Search symbol or name..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-full px-3.5 py-2 pl-9 rounded-xl bg-slate-900/80 border border-slate-800 text-sm text-slate-200 placeholder-slate-500 focus:outline-none focus:border-blue-500/60 focus:ring-1 focus:ring-blue-500/60 transition"
          />
          <svg className="w-4 h-4 text-slate-500 absolute left-3 top-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
        </div>

        {/* Sector Filter */}
        <select
          value={selectedSector}
          onChange={(e) => setSelectedSector(e.target.value)}
          className="px-3.5 py-2 rounded-xl bg-slate-900/80 border border-slate-800 text-sm text-slate-200 focus:outline-none focus:border-blue-500/60"
        >
          <option value="ALL">All Sectors ({stocks.length})</option>
          {sectors.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>

        {/* Shariah Filter */}
        <select
          value={selectedShariah}
          onChange={(e) => setSelectedShariah(e.target.value)}
          className="px-3.5 py-2 rounded-xl bg-slate-900/80 border border-slate-800 text-sm text-slate-200 focus:outline-none focus:border-blue-500/60"
        >
          <option value="ALL">All Shariah Statuses</option>
          <option value="PASS">Compliant (PASS)</option>
          <option value="FAIL">Non-Compliant (FAIL)</option>
          <option value="UNKNOWN">Pending / Unknown</option>
        </select>

        {/* Freshness Filter */}
        <select
          value={selectedFreshness}
          onChange={(e) => setSelectedFreshness(e.target.value)}
          className="px-3.5 py-2 rounded-xl bg-slate-900/80 border border-slate-800 text-sm text-slate-200 focus:outline-none focus:border-blue-500/60"
        >
          <option value="ALL">All Freshness States</option>
          <option value="FRESH">Fresh (&lt; 15m session)</option>
          <option value="LAST_AVAILABLE">Last Available (Closed session)</option>
          <option value="STALE">Stale Data</option>
          <option value="UNAVAILABLE">Unavailable</option>
        </select>
      </div>

      {/* Main Stock Data Table */}
      <div className="glass-card overflow-hidden border border-slate-800/80 rounded-2xl bg-slate-900/40">
        <div className="overflow-x-auto">
          <table className="w-full text-left border-collapse">
            <thead>
              <tr className="border-b border-slate-800/80 bg-slate-950/60 text-xs font-semibold text-slate-400 uppercase tracking-wider">
                <th className="px-5 py-3.5">Security</th>
                <th className="px-4 py-3.5">Sector</th>
                <th className="px-4 py-3.5 text-right">Price</th>
                <th className="px-4 py-3.5 text-right">24h Change</th>
                <th className="px-4 py-3.5">Market Timestamp (IST)</th>
                <th className="px-4 py-3.5">Ingested (IST)</th>
                <th className="px-4 py-3.5 text-center">Freshness</th>
                <th className="px-4 py-3.5 text-center">Shariah</th>
                <th className="px-4 py-3.5 text-center">Signal</th>
                <th className="px-5 py-3.5 text-center">Score</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/40 text-sm">
              {isLoading ? (
                <tr>
                  <td colSpan={10} className="px-6 py-16 text-center text-slate-500">
                    <div className="inline-block animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mb-3" />
                    <p>Loading market data from database...</p>
                  </td>
                </tr>
              ) : isError ? (
                <tr>
                  <td colSpan={10} className="px-6 py-16 text-center text-rose-400">
                    <p className="font-semibold">Failed to load market data</p>
                    <p className="text-xs text-slate-500 mt-1">Please ensure backend API and database services are reachable.</p>
                  </td>
                </tr>
              ) : filteredStocks.length === 0 ? (
                <tr>
                  <td colSpan={10} className="px-6 py-16 text-center text-slate-500">
                    No securities match the selected filter criteria.
                  </td>
                </tr>
              ) : (
                filteredStocks.map((stock) => {
                  const isChangeValid = stock.change !== null && stock.change_percent !== null;
                  const isPositive = isChangeValid && (stock.change as number) > 0;
                  const isNegative = isChangeValid && (stock.change as number) < 0;

                  return (
                    <tr
                      key={stock.symbol}
                      className="hover:bg-slate-800/40 transition-colors group cursor-pointer"
                    >
                      {/* Security */}
                      <td className="px-5 py-3.5 whitespace-nowrap">
                        <Link href={`/stocks/${stock.symbol}`} className="block">
                          <p className="font-bold text-white group-hover:text-blue-400 transition-colors">
                            {stock.symbol}
                          </p>
                          <p className="text-xs text-slate-500 truncate max-w-[160px]">
                            {stock.name}
                          </p>
                        </Link>
                      </td>

                      {/* Sector */}
                      <td className="px-4 py-3.5 text-xs text-slate-400 whitespace-nowrap">
                        {stock.sector || "—"}
                      </td>

                      {/* Price */}
                      <td className="px-4 py-3.5 text-right whitespace-nowrap font-mono font-medium">
                        {stock.price !== null ? (
                          <span className="text-white">
                            ₹{stock.price.toLocaleString("en-IN", { minimumFractionDigits: 2, maximumFractionDigits: 2 })}
                          </span>
                        ) : (
                          <span className="text-slate-500 italic text-xs">Unavailable</span>
                        )}
                      </td>

                      {/* Daily Change */}
                      <td className="px-4 py-3.5 text-right whitespace-nowrap font-mono text-xs">
                        {isChangeValid ? (
                          <span
                            className={`font-semibold ${
                              isPositive
                                ? "text-emerald-400"
                                : isNegative
                                ? "text-rose-400"
                                : "text-slate-400"
                            }`}
                          >
                            {isPositive ? "+" : ""}
                            {(stock.change as number).toFixed(2)} (
                            {isPositive ? "+" : ""}
                            {(stock.change_percent as number).toFixed(2)}%)
                          </span>
                        ) : (
                          <span className="text-slate-500 italic text-[11px]" title="Previous close price is missing or not comparable">
                            —
                          </span>
                        )}
                      </td>

                      {/* Market Timestamp */}
                      <td className="px-4 py-3.5 text-xs font-mono text-slate-400 whitespace-nowrap">
                        {formatIST(stock.marketTimestamp)}
                      </td>

                      {/* Retrieved At */}
                      <td className="px-4 py-3.5 text-xs font-mono text-slate-400 whitespace-nowrap">
                        {formatIST(stock.receivedAt)}
                      </td>

                      {/* Freshness Badge */}
                      <td className="px-4 py-3.5 text-center whitespace-nowrap">
                        <span
                          className={`inline-block px-2 py-0.5 rounded text-[10px] font-semibold border ${
                            stock.freshness === "FRESH"
                              ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/30"
                              : stock.freshness === "LAST_AVAILABLE"
                              ? "bg-blue-500/10 text-blue-400 border-blue-500/30"
                              : stock.freshness === "STALE"
                              ? "bg-amber-500/10 text-amber-400 border-amber-500/30"
                              : "bg-rose-500/10 text-rose-400 border-rose-500/30"
                          }`}
                        >
                          {stock.freshness}
                        </span>
                      </td>

                      {/* Shariah Status */}
                      <td className="px-4 py-3.5 text-center whitespace-nowrap">
                        <span
                          className={`inline-block px-2.5 py-0.5 rounded text-[10px] font-bold border uppercase tracking-wider ${
                            stock.shariah === "PASS"
                              ? "bg-emerald-500/15 text-emerald-400 border-emerald-500/40"
                              : stock.shariah === "FAIL"
                              ? "bg-rose-500/15 text-rose-400 border-rose-500/40"
                              : "bg-slate-800 text-slate-400 border-slate-700"
                          }`}
                        >
                          {stock.shariah}
                        </span>
                      </td>

                      {/* Signal */}
                      <td className="px-4 py-3.5 text-center whitespace-nowrap">
                        <span
                          className={`inline-block px-2 py-0.5 rounded text-[10px] font-bold border tracking-wide uppercase ${
                            stock.signal === "STRONG_BUY" || stock.signal === "BUY"
                              ? "bg-emerald-500/15 text-emerald-400 border-emerald-500/30"
                              : stock.signal === "WATCH"
                              ? "bg-blue-500/15 text-blue-400 border-blue-500/30"
                              : stock.signal === "AVOID" || stock.signal === "STRONG_AVOID"
                              ? "bg-rose-500/15 text-rose-400 border-rose-500/30"
                              : "bg-slate-800/80 text-slate-400 border-slate-700/60"
                          }`}
                        >
                          {stock.signal.replace(/_/g, " ")}
                        </span>
                      </td>

                      {/* Score */}
                      <td className="px-5 py-3.5 text-center whitespace-nowrap">
                        {stock.score !== null ? (
                          <div className="flex items-center justify-center gap-2">
                            <div className="w-14 h-1.5 rounded-full bg-slate-800 overflow-hidden">
                              <div
                                className={`h-full rounded-full ${
                                  stock.score >= 80
                                    ? "bg-emerald-400"
                                    : stock.score >= 60
                                    ? "bg-amber-400"
                                    : "bg-rose-400"
                                }`}
                                style={{ width: `${Math.min(100, Math.max(0, stock.score))}%` }}
                              />
                            </div>
                            <span className="font-mono text-xs font-semibold text-slate-300">
                              {Math.round(stock.score)}
                            </span>
                          </div>
                        ) : (
                          <span className="text-slate-600 text-xs">—</span>
                        )}
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>

        {/* Footer with counts */}
        <div className="px-5 py-3 border-t border-slate-800/80 bg-slate-950/40 flex flex-col sm:flex-row items-center justify-between gap-2 text-xs text-slate-400">
          <div>
            Showing <span className="font-semibold text-slate-200">{filteredStocks.length}</span> of{" "}
            <span className="font-semibold text-slate-200">{meta?.total ?? stocks.length}</span> securities
          </div>
          <div className="text-[11px] text-slate-500">
            Compliant: {stocks.filter((s) => s.shariah === "PASS").length} | Pending Review:{" "}
            {stocks.filter((s) => s.shariah === "UNKNOWN" || s.shariah === "PENDING").length}
          </div>
        </div>
      </div>

      {/* Quantitative & Legal Disclaimers */}
      <div className="p-4 rounded-xl border border-slate-800/60 bg-slate-900/30 text-[11px] text-slate-500 leading-relaxed space-y-1">
        <p>
          <strong className="text-slate-400">Data Integrity Notice:</strong> Daily change and percentage calculations require an authoritative prior trading-session closing price. If prior-close data is unavailable, change is marked unavailable rather than assumed zero. Signals require verified market data and Shariah screening prior to being actionable.
        </p>
        <p>
          <strong className="text-slate-400">Provider Disclaimer:</strong> Intraday feeds from Yahoo Finance are delayed and not exchange-certified. No live order execution is active (paper trading mode enabled).
        </p>
      </div>
    </div>
  );
}
