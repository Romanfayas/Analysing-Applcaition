export default function PortfolioPage() {
  return (
    <div className="space-y-6">
      <div><h1 className="text-2xl font-bold text-white">Portfolio</h1><p className="text-sm text-slate-400 mt-1">Track your holdings and portfolio analytics</p></div>
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <div className="glass-card p-4"><p className="text-xs text-slate-500 uppercase">Total Value</p><p className="text-2xl font-bold text-white mt-1">₹5,00,000</p></div>
        <div className="glass-card p-4"><p className="text-xs text-slate-500 uppercase">P&L</p><p className="text-2xl font-bold text-brand-400 mt-1">+₹42,500</p></div>
        <div className="glass-card p-4"><p className="text-xs text-slate-500 uppercase">Positions</p><p className="text-2xl font-bold text-white mt-1">8</p></div>
        <div className="glass-card p-4"><p className="text-xs text-slate-500 uppercase">Shariah Compliant</p><p className="text-2xl font-bold text-brand-400 mt-1">100%</p></div>
      </div>
      <div className="glass-card p-6"><p className="text-slate-500">Portfolio tracking and analytics dashboard coming in Phase 6.</p></div>
    </div>
  );
}
