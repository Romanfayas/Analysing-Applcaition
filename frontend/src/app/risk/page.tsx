export default function RiskPage() {
  return (
    <div className="space-y-6">
      <div><h1 className="text-2xl font-bold text-white">Risk Engine</h1><p className="text-sm text-slate-400 mt-1">Position sizing, stop loss, and portfolio risk management</p></div>
      <div className="glass-card p-6">
        <h2 className="text-lg font-semibold text-white mb-4">Position Size Calculator</h2>
        <div className="grid grid-cols-2 gap-4 max-w-lg">
          <div><label className="text-xs text-slate-500">Capital (₹)</label><input type="number" defaultValue={50000} className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm" /></div>
          <div><label className="text-xs text-slate-500">Risk %</label><input type="number" defaultValue={1} className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm" /></div>
          <div><label className="text-xs text-slate-500">Entry Price (₹)</label><input type="number" defaultValue={500} className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm" /></div>
          <div><label className="text-xs text-slate-500">Stop Loss (₹)</label><input type="number" defaultValue={475} className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm" /></div>
        </div>
        <div className="mt-4 p-4 rounded-lg bg-slate-800/50">
          <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div><p className="text-xs text-slate-500">Max Risk</p><p className="text-lg font-bold text-white">₹500</p></div>
            <div><p className="text-xs text-slate-500">Risk/Share</p><p className="text-lg font-bold text-white">₹25</p></div>
            <div><p className="text-xs text-slate-500">Position Size</p><p className="text-lg font-bold text-brand-400">20 shares</p></div>
            <div><p className="text-xs text-slate-500">Position Value</p><p className="text-lg font-bold text-white">₹10,000</p></div>
          </div>
        </div>
        <p className="text-[10px] text-slate-600 mt-3 italic">Never recommends a position larger than available capital.</p>
      </div>
    </div>
  );
}
