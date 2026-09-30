export default function PaperTradingPage() {
  return (
    <div className="space-y-6">
      <div><h1 className="text-2xl font-bold text-white">Paper Trading</h1><p className="text-sm text-slate-400 mt-1">Simulate trades without real money to validate signals</p></div>
      <div className="glass-card p-6"><p className="text-slate-500">Paper trading simulation engine coming in Phase 9. Tracks: predicted vs actual return, prediction error, signal accuracy, false positives/negatives, and slippage. Uses a broker abstraction so real execution can be added later.</p></div>
    </div>
  );
}
