export default async function IPODetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">TechVentures Ltd</h1>
          <p className="text-sm text-slate-400 mt-1">Technology • IPO #{id} • NSE</p>
        </div>
        <div className="flex items-center gap-3">
          <span className="shariah-pass px-2 py-0.5 rounded text-xs font-medium">SHARIAH PASS</span>
          <span className="signal-buy px-3 py-1.5 rounded-md text-sm font-bold">APPLY</span>
        </div>
      </div>

      {/* IPO Details */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="glass-card p-6">
          <h2 className="text-lg font-semibold text-white mb-4">Issue Details</h2>
          <div className="space-y-2.5">
            {[
              ["Price Band", "₹180 - ₹195"],
              ["Lot Size", "75 shares"],
              ["Min Investment", "₹14,625"],
              ["Issue Size", "₹1,200 Cr"],
              ["Fresh Issue", "₹800 Cr"],
              ["OFS", "₹400 Cr"],
              ["Open Date", "Sep 15, 2026"],
              ["Close Date", "Sep 17, 2026"],
              ["Listing Date", "Sep 22, 2026"],
            ].map(([label, value]) => (
              <div key={label} className="flex justify-between">
                <span className="text-xs text-slate-500">{label}</span>
                <span className="text-sm font-medium text-white">{value}</span>
              </div>
            ))}
          </div>
        </div>

        <div className="glass-card p-6">
          <h2 className="text-lg font-semibold text-white mb-4">Listing Prediction</h2>
          <div className="text-center mb-4">
            <p className="text-3xl font-bold gradient-text">₹228</p>
            <p className="text-xs text-slate-500 mt-1">Predicted Midpoint</p>
            <p className="text-xs text-brand-400 mt-0.5">+16.9% expected return</p>
          </div>
          <div className="space-y-3">
            <div className="flex justify-between items-center p-2 rounded bg-brand-500/5">
              <span className="text-xs text-slate-400">Bull Case</span>
              <span className="text-sm font-medium text-brand-400">₹250</span>
            </div>
            <div className="flex justify-between items-center p-2 rounded bg-accent-500/5">
              <span className="text-xs text-slate-400">Base Case</span>
              <span className="text-sm font-medium text-accent-400">₹228</span>
            </div>
            <div className="flex justify-between items-center p-2 rounded bg-danger-500/5">
              <span className="text-xs text-slate-400">Bear Case</span>
              <span className="text-sm font-medium text-danger-400">₹205</span>
            </div>
          </div>
          <div className="mt-4 text-center">
            <span className="text-xs text-slate-500">Confidence: </span>
            <span className="text-sm font-bold text-accent-400">72/100</span>
          </div>
          <p className="text-[10px] text-slate-600 mt-3 italic">
            Predictions are statistical estimates, NOT guaranteed listing prices.
          </p>
        </div>

        <div className="glass-card p-6">
          <h2 className="text-lg font-semibold text-white mb-4">Subscription</h2>
          <div className="space-y-3">
            {[
              ["QIB", "18.5x"],
              ["NII", "12.2x"],
              ["Retail", "8.4x"],
              ["Employee", "2.1x"],
              ["Total", "12.5x"],
            ].map(([label, value]) => (
              <div key={label} className="flex justify-between items-center">
                <span className="text-xs text-slate-400">{label}</span>
                <span className="text-sm font-bold text-white">{value}</span>
              </div>
            ))}
          </div>
          <div className="mt-4 pt-4 border-t border-slate-800">
            <p className="text-xs text-slate-500">Current GMP</p>
            <p className="text-2xl font-bold text-brand-400">+₹42</p>
            <p className="text-[10px] text-slate-600 italic mt-1">
              GMP is unofficial market sentiment.
            </p>
          </div>
        </div>
      </div>
    </div>
  );
}
