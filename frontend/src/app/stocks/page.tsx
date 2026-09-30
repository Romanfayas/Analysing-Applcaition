import Link from "next/link";

export default function StocksPage() {
  const stocks = [
    { symbol: "RELIANCE", name: "Reliance Industries Ltd", sector: "Energy", price: 2450, change: 2.3, shariah: "PASS", signal: "STRONG_BUY", score: 85 },
    { symbol: "TCS", name: "Tata Consultancy Services", sector: "Technology", price: 3800, change: 1.1, shariah: "PASS", signal: "BUY", score: 74 },
    { symbol: "HDFCBANK", name: "HDFC Bank Ltd", sector: "Financial Services", price: 1650, change: -1.2, shariah: "FAIL", signal: "AVOID", score: 45 },
    { symbol: "INFY", name: "Infosys Ltd", sector: "Technology", price: 1520, change: -0.5, shariah: "PASS", signal: "WATCH", score: 62 },
    { symbol: "HINDUNILVR", name: "Hindustan Unilever", sector: "FMCG", price: 2380, change: 0.4, shariah: "PASS", signal: "BUY", score: 72 },
    { symbol: "ITC", name: "ITC Ltd", sector: "FMCG", price: 440, change: 0.8, shariah: "PASS", signal: "BUY", score: 71 },
    { symbol: "BHARTIARTL", name: "Bharti Airtel Ltd", sector: "Telecom", price: 1180, change: 1.5, shariah: "PASS", signal: "WATCH", score: 65 },
    { symbol: "WIPRO", name: "Wipro Ltd", sector: "Technology", price: 480, change: -0.3, shariah: "PASS", signal: "AVOID", score: 48 },
    { symbol: "BAJFINANCE", name: "Bajaj Finance Ltd", sector: "Financial Services", price: 7200, change: 2.1, shariah: "FAIL", signal: "AVOID", score: 40 },
    { symbol: "TATAMOTORS", name: "Tata Motors Ltd", sector: "Automobile", price: 680, change: 3.2, shariah: "PASS", signal: "STRONG_BUY", score: 82 },
  ];

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
              <th className="text-right px-4 py-3 text-xs font-medium text-slate-500 uppercase">Change</th>
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
                <td className="px-4 py-3 text-sm font-medium text-white text-right">₹{stock.price.toLocaleString()}</td>
                <td className={`px-4 py-3 text-sm font-medium text-right ${stock.change >= 0 ? "text-brand-400" : "text-danger-400"}`}>
                  {stock.change >= 0 ? "+" : ""}{stock.change}%
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
