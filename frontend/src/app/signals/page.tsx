export default function SignalsPage() {
  return (
    <div className="space-y-6">
      <div><h1 className="text-2xl font-bold text-white">Active Signals</h1><p className="text-sm text-slate-400 mt-1">Composite weighted signals across all screened stocks</p></div>
      <div className="glass-card p-6"><p className="text-slate-500">Signal engine with configurable weights coming in Phase 3. Signals combine fundamental (30%), technical (20%), candlestick (10%), momentum (10%), volume (5%), valuation (10%), market regime (5%), risk (5%), and quality (5%) scores.</p></div>
      <p className="text-[10px] text-slate-600 italic">Signals are statistical analysis outputs. A failed Shariah screen always overrides any signal to AVOID.</p>
    </div>
  );
}
