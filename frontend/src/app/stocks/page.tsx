"use client";

import Link from "next/link";
import { useStocks } from "@/lib/api/hooks";
import { formatDistanceToNow } from "date-fns";

export default function StocksPage() {
  const { stocks, isLoading, isError } = useStocks();

  if (isLoading) {
    return <div className="text-center py-20 text-slate-500">Loading market data...</div>;
  }

  if (isError) {
    return <div className="text-center py-20 text-red-500">Failed to load market data.</div>;
  }
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">Stocks</h1>
          <p className="text-sm text-slate-400 mt-1">Indian equity analysis with Shariah screening</p>
        </div>
        <div className="flex items-center gap-3">
          <select className="px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200">
            <option>All Sectors</option>
            <option>Technology</option>
            <option>Energy</option>
            <option>Financial Services</option>
            <option>FMCG</option>
          </select>
          <select className="px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-sm text-slate-200">
            <option>All Shariah</option>
            <option>PASS Only</option>
            <option>FAIL Only</option>
          </select>
        </div>
      </div>

      <div className="glass-card overflow-hidden">
        <table className="w-full">
          <thead>
            <tr className="border-b border-slate-800">
              <th className="text-left px-4 py-3 text-xs font-medium text-slate-500 uppercase">Symbol</th>
              <th className="text-left px-4 py-3 text-xs font-medium text-slate-500 uppercase">Sector</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-slate-500 uppercase">Price</th>
              <th className="text-right px-4 py-3 text-xs font-medium text-slate-500 uppercase">Status</th>
              <th className="text-center px-4 py-3 text-xs font-medium text-slate-500 uppercase">Shariah</th>
              <th className="text-center px-4 py-3 text-xs font-medium text-slate-500 uppercase">Signal</th>
              <th className="text-center px-4 py-3 text-xs font-medium text-slate-500 uppercase">Score</th>
            </tr>
          </thead>
          <tbody>
            {stocks.map((stock) => (
              <tr key={stock.symbol} className="border-b border-slate-800/50 hover:bg-slate-800/30 transition-colors">
                <td className="px-4 py-3">
                  <Link href={`/stocks/${stock.symbol}`} className="group">
                    <p className="text-sm font-medium text-white group-hover:text-brand-400">{stock.symbol}</p>
                    <p className="text-xs text-slate-500">{stock.name}</p>
                  </Link>
                </td>
                <td className="px-4 py-3 text-sm text-slate-400">{stock.sector}</td>
                <td className="px-4 py-3 text-sm font-medium text-white text-right">
                  {stock.freshness === "UNAVAILABLE" ? "DATA_UNAVAILABLE" : `₹${stock.price?.toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }) || '0.00'}`}
                  {stock.marketTimestamp && (
                    <p className="text-[10px] text-slate-500 font-normal mt-0.5 whitespace-nowrap">
                      {stock.marketStatus === "CLOSED" ? "Last updated:" : "As of"} {new Date(stock.marketTimestamp).toLocaleString('en-IN', { timeZone: 'Asia/Kolkata', dateStyle: 'medium', timeStyle: 'short' })} IST
                      <br/>
                      Market: {stock.marketStatus}
                    </p>
                  )}
                </td>
                <td className="px-4 py-3 text-right">
                  <span className={`inline-block px-2 py-0.5 rounded text-[10px] font-medium ${
                    stock.freshness === 'STALE' ? 'bg-warn-400/20 text-warn-400' :
                    stock.freshness === 'FRESH' ? 'bg-brand-400/20 text-brand-400' : 'bg-slate-700 text-slate-400'
                  }`}>
                    {stock.freshness}
                  </span>
                </td>
                <td className="px-4 py-3 text-center">
                  <span className={`inline-block px-2 py-0.5 rounded text-[10px] font-medium ${
                    stock.shariah === "PASS" ? "shariah-pass" : "shariah-fail"
                  }`}>
                    {stock.shariah}
                  </span>
                </td>
                <td className="px-4 py-3 text-center">
                  <span className={`inline-block px-2 py-0.5 rounded text-[10px] font-semibold ${
                    stock.signal === "STRONG_BUY" ? "signal-strong-buy" :
                    stock.signal === "BUY" ? "signal-buy" :
                    stock.signal === "WATCH" ? "signal-watch" :
                    "signal-avoid"
                  }`}>
                    {stock.signal.replace("_", " ")}
                  </span>
                </td>
                <td className="px-4 py-3 text-center">
                  <div className="flex items-center justify-center gap-2">
                    <div className="w-16 h-1.5 rounded-full bg-slate-800 overflow-hidden">
                      <div
                        className={`h-full rounded-full ${
                          stock.score >= 80 ? "bg-brand-500" :
                          stock.score >= 60 ? "bg-warn-400" :
                          "bg-danger-400"
                        }`}
                        style={{ width: `${stock.score}%` }}
                      />
                    </div>
                    <span className="text-xs text-slate-400">{stock.score}</span>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <p className="text-[10px] text-slate-600 italic">
        Signals are based on statistical analysis and do not constitute investment advice. Past performance does not guarantee future results.
      </p>
    </div>
  );
}
