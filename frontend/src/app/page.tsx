"use client";

import { 
  ArrowTrendingUpIcon, 
  WalletIcon, 
  ShieldCheckIcon, 
  BoltIcon 
} from '@heroicons/react/24/outline';
import StatCard from '@/components/ui/StatCard';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';

import { useRecentSignals, usePortfolio } from '@/lib/api/hooks';

export default function Dashboard() {
  const { signals: recentSignals, isLoading: isLoadingSignals } = useRecentSignals();
  const { positions: portfolio, isLoading: isLoadingPortfolio } = usePortfolio();

  return (
    <div className="space-y-8 animate-in fade-in duration-500">
      <div>
        <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Overview</h1>
        <p className="text-slate-400 mt-1">Your Shariah-compliant quantitative portfolio</p>
      </div>

      {/* High-level Stats */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <StatCard 
          title="Total Equity" 
          value="₹5,42,000" 
          change="12.5%" 
          isPositive={true}
          icon={<WalletIcon className="w-6 h-6" />} 
        />
        <StatCard 
          title="Daily P&L" 
          value="+₹4,520" 
          change="0.8%" 
          isPositive={true}
          icon={<ArrowTrendingUpIcon className="w-6 h-6" />} 
        />
        <StatCard 
          title="Active Signals" 
          value="14" 
          icon={<BoltIcon className="w-6 h-6 text-amber-400" />} 
        />
        <StatCard 
          title="Shariah Pass Rate" 
          value="98.5%" 
          change="Stable"
          isPositive={true}
          icon={<ShieldCheckIcon className="w-6 h-6 text-emerald-400" />} 
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Active Positions */}
        <div className="lg:col-span-2">
          <Card className="h-full">
            <CardHeader title="Paper Trading Ledger" subtitle="Your active spot positions" />
            <div className="overflow-x-auto">
              <table className="w-full text-left border-collapse">
                <thead>
                  <tr className="border-b border-slate-700/50 text-sm font-medium text-slate-400">
                    <th className="pb-3 pl-2">Symbol</th>
                    <th className="pb-3 text-right">Shares</th>
                    <th className="pb-3 text-right">Avg Price</th>
                    <th className="pb-3 text-right">CMP</th>
                    <th className="pb-3 text-right pr-2">Return</th>
                  </tr>
                </thead>
                <tbody className="text-sm">
                  {isLoadingPortfolio ? (
                    <tr>
                      <td colSpan={5} className="py-8 text-center text-slate-500">Loading positions...</td>
                    </tr>
                  ) : portfolio && portfolio.length > 0 ? (
                    portfolio.map((pos) => (
                      <tr key={pos.symbol} className="border-b border-slate-800/50 hover:bg-slate-800/20 transition-colors">
                        <td className="py-4 pl-2 font-semibold text-white">{pos.symbol}</td>
                        <td className="py-4 text-right text-slate-300">{pos.shares}</td>
                        <td className="py-4 text-right text-slate-300">₹{pos.avg_price?.toFixed(2) || '0.00'}</td>
                        <td className="py-4 text-right text-slate-300">₹{pos.current_price?.toFixed(2) || '0.00'}</td>
                        <td className="py-4 text-right pr-2 text-emerald-400 font-medium">{(pos.pnl_pct || 0) > 0 ? '+' : ''}{(pos.pnl_pct || 0).toFixed(2)}%</td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={5} className="py-8 text-center text-slate-500">No active positions found.</td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </Card>
        </div>

        {/* Recent Signals */}
        <div>
          <Card className="h-full">
            <CardHeader title="Quant Signals" subtitle="Latest high-conviction alerts" />
            <div className="space-y-4">
              {isLoadingSignals ? (
                <div className="py-4 text-center text-slate-500">Loading signals...</div>
              ) : recentSignals && recentSignals.length > 0 ? (
                recentSignals.map((signal) => (
                  <div key={signal.symbol} className="p-4 rounded-xl bg-slate-800/30 border border-slate-700/50 hover:border-blue-500/30 transition-colors group cursor-pointer">
                    <div className="flex justify-between items-start mb-2">
                      <span className="font-bold text-white group-hover:text-blue-400 transition-colors">{signal.symbol}</span>
                      <Badge variant={signal.action === 'STRONG_BUY' ? 'pass' : 'blue'}>
                        {signal.action.replace('_', ' ')}
                      </Badge>
                    </div>
                    <div className="flex justify-between items-center text-sm">
                      <span className="text-slate-400">Score: <span className="text-slate-200 font-medium">{signal.score?.toFixed(1) || '0.0'}/100</span></span>
                      <span className="text-slate-500 text-xs">{new Date(signal.timestamp).toLocaleDateString()}</span>
                    </div>
                  </div>
                ))
              ) : (
                <div className="py-4 text-center text-slate-500">No active signals found.</div>
              )}
            </div>
          </Card>
        </div>
      </div>
    </div>
  );
}
