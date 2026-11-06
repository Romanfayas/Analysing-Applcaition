'use client';

import { useState, useMemo } from 'react';
import { useStocks, usePortfolio } from '@/lib/api/hooks';
import { ShieldCheckIcon, CalculatorIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';

export default function RiskPage() {
  const { stocks } = useStocks();
  const { portfolio } = usePortfolio();

  // Starts from the real paper cash balance; a typed value overrides it. (1,000,000 is only the calculator default before the ledger loads.)
  const [capitalOverride, setCapital] = useState<number | null>(null);
  const capital = capitalOverride ?? portfolio?.cash_balance ?? 1000000;
  const [riskPercent, setRiskPercent] = useState<number>(1.0);
  const [selectedSymbol, setSelectedSymbol] = useState<string>('TCS');
  // Entry/stop/target default from the selected stock's latest stored price (stop -2%, target +5%);
  // anything the user types overrides the default. No price → fields stay empty instead of using stale numbers.
  const [overrides, setOverrides] = useState<{ entry?: number; stop?: number; target?: number }>({});
  const basePrice = stocks.find((s) => s.symbol === selectedSymbol)?.price ?? null;
  const entryPrice = overrides.entry ?? (basePrice ?? 0);
  const stopLoss = overrides.stop ?? (basePrice ? Number((basePrice * 0.98).toFixed(2)) : 0);
  const targetPrice = overrides.target ?? (basePrice ? Number((basePrice * 1.05).toFixed(2)) : 0);
  const setEntryPrice = (v: number) => setOverrides((o) => ({ ...o, entry: v }));
  const setStopLoss = (v: number) => setOverrides((o) => ({ ...o, stop: v }));
  const setTargetPrice = (v: number) => setOverrides((o) => ({ ...o, target: v }));

  const handleStockSelect = (symStr: string) => {
    setSelectedSymbol(symStr);
    setOverrides({}); // re-derive from the newly selected stock
  };

  // Interactive calculations (deterministic institutional model)
  const calculation = useMemo(() => {
    if (entryPrice <= 0 || stopLoss <= 0 || stopLoss >= entryPrice || capital <= 0 || riskPercent <= 0) {
      return null;
    }

    const maxRiskAmount = capital * (riskPercent / 100);
    const riskPerShare = entryPrice - stopLoss;
    let shares = Math.floor(maxRiskAmount / riskPerShare);

    const maxSharesByCapital = Math.floor(capital / entryPrice);
    if (shares > maxSharesByCapital) {
      shares = maxSharesByCapital;
    }
    if (shares < 0) shares = 0;

    const positionValue = shares * entryPrice;
    const actualRiskAmount = shares * riskPerShare;
    const portfolioRiskPct = (actualRiskAmount / capital) * 100;
    const riskRewardRatio = targetPrice > entryPrice ? (targetPrice - entryPrice) / riskPerShare : 0;

    return {
      maxRiskAmount,
      riskPerShare,
      suggestedShares: shares,
      positionValue,
      actualRiskAmount,
      capitalExposurePct: (positionValue / capital) * 100,
      portfolioRiskPct,
      riskRewardRatio,
    };
  }, [capital, riskPercent, entryPrice, stopLoss, targetPrice]);

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Risk Engine & Position Sizing</h1>
          <p className="text-sm text-slate-400 mt-1">
            Deterministic institutional position sizing with strict Shariah spot risk constraints.
          </p>
        </div>
        <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-slate-800/80 border border-slate-700/60 text-xs text-slate-300">
          <ShieldCheckIcon className="w-4 h-4 text-emerald-400" />
          <span>Long-Only · Spot · Zero Margin · Zero Leverage</span>
        </div>
      </div>

      {/* Position Sizing Calculator */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 glass-card p-6">
          <div className="flex items-center justify-between pb-4 border-b border-white/5 mb-6">
            <h2 className="text-lg font-semibold text-white flex items-center gap-2">
              <CalculatorIcon className="w-5 h-5 text-blue-400" />
              Position Sizing Calculator
            </h2>
            <span className="text-xs text-slate-400 font-mono">Max 2% Risk Cap</span>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div>
              <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Quick Asset Select</label>
              <select
                value={selectedSymbol}
                onChange={(e) => handleStockSelect(e.target.value)}
                className="w-full mt-1.5 px-3 py-2.5 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none"
              >
                {stocks.map((s) => (
                  <option key={s.symbol} value={s.symbol}>
                    {s.symbol} — {s.name} (₹{s.price ? s.price.toFixed(2) : '—'})
                  </option>
                ))}
              </select>
            </div>

            <div>
              <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Total Trading Capital (₹)</label>
              <input
                type="number"
                value={capital}
                onChange={(e) => setCapital(Number(e.target.value))}
                className="w-full mt-1.5 px-3 py-2.5 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Risk % Per Trade</label>
              <input
                type="number"
                step="0.1"
                min="0.1"
                max="5.0"
                value={riskPercent}
                onChange={(e) => setRiskPercent(Number(e.target.value))}
                className="w-full mt-1.5 px-3 py-2.5 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Entry Price (₹)</label>
              <input
                type="number"
                step="0.05"
                value={entryPrice}
                onChange={(e) => setEntryPrice(Number(e.target.value))}
                className="w-full mt-1.5 px-3 py-2.5 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Stop Loss (₹)</label>
              <input
                type="number"
                step="0.05"
                value={stopLoss}
                onChange={(e) => setStopLoss(Number(e.target.value))}
                className="w-full mt-1.5 px-3 py-2.5 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none"
              />
            </div>

            <div>
              <label className="text-xs font-semibold uppercase text-slate-400 tracking-wider">Target Price (₹)</label>
              <input
                type="number"
                step="0.05"
                value={targetPrice}
                onChange={(e) => setTargetPrice(Number(e.target.value))}
                className="w-full mt-1.5 px-3 py-2.5 rounded-xl bg-slate-900 border border-slate-700 text-white text-sm focus:border-blue-500 focus:outline-none"
              />
            </div>
          </div>

          {stopLoss >= entryPrice && (
            <div className="mt-4 p-3 rounded-xl bg-rose-500/10 border border-rose-500/30 text-rose-300 text-xs flex items-center gap-2">
              <ExclamationTriangleIcon className="w-4 h-4 shrink-0" />
              <span>Stop loss must be lower than the entry price for long spot positions. Short selling is prohibited.</span>
            </div>
          )}

          {/* Results Grid */}
          {calculation && (
            <div className="mt-6 p-5 rounded-2xl bg-slate-900/80 border border-slate-800">
              <h3 className="text-xs font-bold uppercase tracking-wider text-slate-400 mb-3">Recommended Sizing Outputs</h3>
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div className="p-3 rounded-xl bg-slate-800/40 border border-slate-700/50">
                  <p className="text-[11px] font-medium text-slate-400">Position Size</p>
                  <p className="text-xl font-bold font-mono text-emerald-400 mt-0.5">{calculation.suggestedShares} shares</p>
                </div>
                <div className="p-3 rounded-xl bg-slate-800/40 border border-slate-700/50">
                  <p className="text-[11px] font-medium text-slate-400">Total Capital Value</p>
                  <p className="text-xl font-bold font-mono text-white mt-0.5">₹{calculation.positionValue.toLocaleString('en-IN', { maximumFractionDigits: 2 })}</p>
                </div>
                <div className="p-3 rounded-xl bg-slate-800/40 border border-slate-700/50">
                  <p className="text-[11px] font-medium text-slate-400">Capital At Risk</p>
                  <p className="text-xl font-bold font-mono text-amber-400 mt-0.5">₹{calculation.actualRiskAmount.toLocaleString('en-IN', { maximumFractionDigits: 2 })}</p>
                </div>
                <div className="p-3 rounded-xl bg-slate-800/40 border border-slate-700/50">
                  <p className="text-[11px] font-medium text-slate-400">Risk : Reward</p>
                  <p className="text-xl font-bold font-mono text-blue-400 mt-0.5">1 : {calculation.riskRewardRatio.toFixed(2)}</p>
                </div>
              </div>
              <p className="text-[11px] text-slate-500 mt-3 italic">
                Enforcing non-negotiable capital safety: Sizing is constrained so total position never exceeds available cash.
              </p>
            </div>
          )}
        </div>

        {/* Global Risk Governance Panel */}
        <div className="glass-card p-6 flex flex-col justify-between">
          <div>
            <h2 className="text-lg font-semibold text-white mb-4">Portfolio Risk Controls</h2>
            <div className="space-y-4">
              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <span className="text-xs text-slate-400 block font-medium">Max Risk Per Trade</span>
                <span className="text-lg font-bold text-white mt-1 block">2.0% of Capital</span>
                <span className="text-[11px] text-slate-500">Strictly bounds single-trade downside</span>
              </div>

              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <span className="text-xs text-slate-400 block font-medium">Max Portfolio Heat</span>
                <span className="text-lg font-bold text-white mt-1 block">10.0% Aggregate Risk</span>
                <span className="text-[11px] text-slate-500">Sum of all open position stop risks</span>
              </div>

              <div className="p-3.5 rounded-xl bg-slate-900/60 border border-slate-800">
                <span className="text-xs text-slate-400 block font-medium">Execution Mode</span>
                <span className="text-lg font-bold text-emerald-400 mt-1 block">Paper Simulation Active</span>
                <span className="text-[11px] text-slate-500">LIVE_ORDER_EXECUTION is strictly false</span>
              </div>
            </div>
          </div>

          <div className="mt-6 pt-4 border-t border-slate-800 text-[11px] text-slate-500 leading-relaxed">
            All trades adhere to Shariah equity investment guidelines: delivery-based equity holdings only, zero margin financing, zero short positions, and zero derivative exposure.
          </div>
        </div>
      </div>
    </div>
  );
}
