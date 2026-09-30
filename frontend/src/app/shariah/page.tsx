export default function ShariahPage() {
  return (
    <div className="space-y-6">
      <div><h1 className="text-2xl font-bold text-white">Shariah Screening</h1><p className="text-sm text-slate-400 mt-1">Versioned Shariah compliance screening with configurable rules</p></div>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="glass-card p-4 text-center"><p className="text-3xl font-bold text-brand-400">847</p><p className="text-xs text-slate-500 uppercase mt-1">Compliant</p></div>
        <div className="glass-card p-4 text-center"><p className="text-3xl font-bold text-danger-400">298</p><p className="text-xs text-slate-500 uppercase mt-1">Non-Compliant</p></div>
        <div className="glass-card p-4 text-center"><p className="text-3xl font-bold text-warn-400">55</p><p className="text-xs text-slate-500 uppercase mt-1">Review Required</p></div>
      </div>
      <div className="glass-card p-6">
        <h2 className="text-lg font-semibold text-white mb-3">Active Rule Set: AAOIFI Standard v1</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <div className="p-3 rounded-lg bg-slate-800/50"><p className="text-[10px] text-slate-500">Debt / Market Cap</p><p className="text-lg font-bold text-white">≤ 33%</p></div>
          <div className="p-3 rounded-lg bg-slate-800/50"><p className="text-[10px] text-slate-500">Interest Income / Revenue</p><p className="text-lg font-bold text-white">≤ 5%</p></div>
          <div className="p-3 rounded-lg bg-slate-800/50"><p className="text-[10px] text-slate-500">Cash & Deposits / Market Cap</p><p className="text-lg font-bold text-white">≤ 33%</p></div>
          <div className="p-3 rounded-lg bg-slate-800/50"><p className="text-[10px] text-slate-500">Receivables / Market Cap</p><p className="text-lg font-bold text-white">≤ 49%</p></div>
        </div>
        <p className="text-[10px] text-slate-600 mt-4 italic">Shariah failure is a hard trading restriction. The system never overrides a failed screen because the stock has a strong technical signal. Please consult a qualified Shariah advisor for definitive rulings.</p>
      </div>
    </div>
  );
}
