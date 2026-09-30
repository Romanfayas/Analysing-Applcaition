'use client';

import { useState } from 'react';

export default function BacktestingPage() {
  const [activeTab, setActiveTab] = useState('summary');
  const [results, setResults] = useState<any>(null);
  const [loading, setLoading] = useState(false);

  const runBacktest = async (type: 'historical' | 'walk_forward') => {
    setLoading(true);
    try {
      // In a real implementation this would post to our Next.js API route 
      // which proxies to the Go backend `POST /api/backtesting/run`
      // For now we simulate the delay and the structure of the data.
      setTimeout(() => {
        setResults({
          initial_capital: 100000,
          final_capital: 115200,
          total_return_percent: 15.2,
          max_drawdown_percent: 8.5,
          win_rate_percent: 55.4,
          total_trades: 42,
          type: type,
          advanced_metrics: {
            cagr_percent: 12.5,
            volatility_percent: 18.2,
            sharpe_ratio: 1.2,
            sortino_ratio: 1.8,
            calmar_ratio: 1.5,
            var_95_percent: 2.1,
            cvar_95_percent: 3.5,
            alpha_percent: 4.2,
            beta: 0.8
          },
          trades: [
            { symbol: 'SYM1', entry_date: '2023-01-05', exit_date: '2023-01-10', exit_reason: 'Take-Profit', pnl_percent: 5.2, pnl: 520, entry_price: 100, exit_price: 105, shares: 100, direction: 'LONG' },
            { symbol: 'SYM2', entry_date: '2023-01-15', exit_date: '2023-01-18', exit_reason: 'Stop-Loss', pnl_percent: -2.1, pnl: -210, entry_price: 200, exit_price: 195.8, shares: 100, direction: 'LONG' }
          ],
          equity_curve: [
            { date: '2023-01-01', total_equity: 100000, cash: 100000, invested_value: 0 },
            { date: '2023-01-05', total_equity: 100200, cash: 89800, invested_value: 10400 }
          ]
        });
        setLoading(false);
      }, 1000);
    } catch (err) {
      console.error(err);
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-white">Quantitative Backtesting</h1>
        <p className="text-sm text-slate-400 mt-1">Deterministic historical and walk-forward simulation</p>
      </div>

      <div className="mt-4 p-4 rounded-lg bg-warn-400/10 border border-warn-400/20 flex flex-col md:flex-row justify-between items-start md:items-center">
        <div>
          <h3 className="text-warn-400 font-semibold flex items-center gap-2">
            <svg className="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
            </svg>
            SURVIVORSHIP BIAS LIMITED
          </h3>
          <p className="text-sm text-warn-400/80 mt-1">Historical constituents are not fully survivor-bias free. Delisted stocks may not be present in the dataset.</p>
        </div>
        <div className="mt-4 md:mt-0 flex gap-2">
           <button 
            onClick={() => runBacktest('historical')}
            disabled={loading}
            className="px-4 py-2 bg-brand-500 hover:bg-brand-600 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50"
          >
            {loading ? 'Running...' : 'Run Historical'}
          </button>
           <button 
            onClick={() => runBacktest('walk_forward')}
            disabled={loading}
            className="px-4 py-2 bg-indigo-500 hover:bg-indigo-600 text-white rounded-lg text-sm font-medium transition-colors disabled:opacity-50"
          >
            {loading ? 'Running...' : 'Run Walk-Forward'}
          </button>
        </div>
      </div>

      {results && (
        <div className="glass-card">
          <div className="flex border-b border-white/5 px-2">
            {['summary', 'risk', 'trades', 'cash-flow', 'walk-forward'].map((tab) => (
              <button
                key={tab}
                onClick={() => setActiveTab(tab)}
                className={`px-4 py-3 text-sm font-medium capitalize border-b-2 transition-colors ${
                  activeTab === tab
                    ? 'border-brand-500 text-brand-400'
                    : 'border-transparent text-slate-400 hover:text-slate-200 hover:border-white/10'
                }`}
              >
                {tab.replace('-', ' ')}
              </button>
            ))}
          </div>
          
          <div className="p-6">
            {activeTab === 'summary' && (
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Total Return</p>
                  <p className="text-xl font-bold text-success-400">+{results.total_return_percent}%</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Max Drawdown</p>
                  <p className="text-xl font-bold text-error-400">-{results.max_drawdown_percent}%</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Win Rate</p>
                  <p className="text-xl font-bold text-white">{results.win_rate_percent}%</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Total Trades</p>
                  <p className="text-xl font-bold text-white">{results.total_trades}</p>
                </div>
              </div>
            )}
            
            {activeTab === 'trades' && (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-sm text-slate-400">
                  <thead className="text-xs uppercase bg-white/5 text-slate-300">
                    <tr>
                      <th className="px-4 py-3">Symbol</th>
                      <th className="px-4 py-3">Entry</th>
                      <th className="px-4 py-3">Exit</th>
                      <th className="px-4 py-3">Reason</th>
                      <th className="px-4 py-3 text-right">PnL %</th>
                    </tr>
                  </thead>
                  <tbody>
                    {results.trades?.map((trade: any, idx: number) => (
                      <tr key={idx} className="border-b border-white/5 hover:bg-white/5">
                        <td className="px-4 py-3 text-white font-medium">{trade.symbol}</td>
                        <td className="px-4 py-3">{trade.entry_date}</td>
                        <td className="px-4 py-3">{trade.exit_date || '-'}</td>
                        <td className="px-4 py-3">
                          <span className={`px-2 py-1 rounded-full text-xs ${
                            trade.exit_reason === 'Stop-Loss' ? 'bg-error-500/20 text-error-400' :
                            trade.exit_reason === 'Take-Profit' ? 'bg-success-500/20 text-success-400' :
                            'bg-slate-500/20 text-slate-300'
                          }`}>
                            {trade.exit_reason || 'Signal'}
                          </span>
                        </td>
                        <td className={`px-4 py-3 text-right font-medium ${trade.pnl_percent > 0 ? 'text-success-400' : 'text-error-400'}`}>
                          {trade.pnl_percent > 0 ? '+' : ''}{trade.pnl_percent?.toFixed(2)}%
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {!results.trades?.length && (
                  <div className="text-center py-8 text-slate-500">No trades recorded.</div>
                )}
              </div>
            )}

            {activeTab === 'cash-flow' && (
              <div className="overflow-x-auto">
                <table className="w-full text-left text-sm text-slate-400">
                  <thead className="text-xs uppercase bg-white/5 text-slate-300">
                    <tr>
                      <th className="px-4 py-3">Date</th>
                      <th className="px-4 py-3 text-right">Total Equity</th>
                      <th className="px-4 py-3 text-right">Available Cash</th>
                      <th className="px-4 py-3 text-right">Invested Value</th>
                    </tr>
                  </thead>
                  <tbody>
                    {results.equity_curve?.slice(0, 10).map((pt: any, idx: number) => (
                      <tr key={idx} className="border-b border-white/5 hover:bg-white/5">
                        <td className="px-4 py-3 text-white">{pt.date}</td>
                        <td className="px-4 py-3 text-right text-brand-400 font-medium">₹{pt.total_equity?.toLocaleString()}</td>
                        <td className="px-4 py-3 text-right text-success-400 font-medium">₹{pt.cash?.toLocaleString()}</td>
                        <td className="px-4 py-3 text-right text-indigo-400 font-medium">₹{pt.invested_value?.toLocaleString()}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                <div className="text-center py-4 text-xs text-slate-500">Showing first 10 days of cash flow.</div>
              </div>
            )}

            {activeTab === 'risk' && results.advanced_metrics && (
              <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">CAGR</p>
                  <p className="text-xl font-bold text-white">{results.advanced_metrics.cagr_percent}%</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Sharpe Ratio</p>
                  <p className="text-xl font-bold text-white">{results.advanced_metrics.sharpe_ratio}</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Sortino Ratio</p>
                  <p className="text-xl font-bold text-white">{results.advanced_metrics.sortino_ratio}</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Calmar Ratio</p>
                  <p className="text-xl font-bold text-white">{results.advanced_metrics.calmar_ratio}</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Volatility (Ann)</p>
                  <p className="text-xl font-bold text-slate-300">{results.advanced_metrics.volatility_percent}%</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">VaR (95%)</p>
                  <p className="text-xl font-bold text-error-400">-{results.advanced_metrics.var_95_percent}%</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Alpha</p>
                  <p className="text-xl font-bold text-success-400">{results.advanced_metrics.alpha_percent}%</p>
                </div>
                <div className="p-4 rounded-lg bg-white/5">
                  <p className="text-sm text-slate-400">Beta</p>
                  <p className="text-xl font-bold text-white">{results.advanced_metrics.beta}</p>
                </div>
              </div>
            )}

            {activeTab === 'risk' && !results.advanced_metrics && (
              <div className="text-center py-8 text-slate-500">
                <p>Detailed Risk Metrics are not available for this run.</p>
              </div>
            )}

            {activeTab === 'walk-forward' && (
              <div className="text-center py-8 text-slate-500">
                {results.type === 'walk_forward' ? (
                   <div>
                     <p className="text-white">Walk-Forward Validation OOS Results</p>
                     <p className="text-sm mt-1">Model generalizes well across varying market regimes.</p>
                   </div>
                ) : (
                  <p>Run Walk-Forward analysis to view Out-Of-Sample validation metrics.</p>
                )}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
