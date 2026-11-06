"use client";

import { useState } from 'react';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { 
  ShieldCheckIcon, 
  ShieldExclamationIcon, 
  QuestionMarkCircleIcon,
  InformationCircleIcon 
} from '@heroicons/react/24/outline';
import { useShariahScreenings } from '@/lib/api/hooks';

export default function ShariahPage() {
  const { screenings, isLoading } = useShariahScreenings();
  type Filter = 'ALL' | 'PASS' | 'FAIL' | 'REVIEW';
  const FILTERS: Filter[] = ['ALL', 'PASS', 'FAIL', 'REVIEW'];
  const [filter, setFilter] = useState<Filter>('ALL');

  const passCount = screenings.filter((s) => s.status === 'PASS').length;
  const failCount = screenings.filter((s) => s.status === 'FAIL').length;
  const reviewCount = screenings.filter((s) => ['REVIEW_REQUIRED', 'DATA_INSUFFICIENT', 'UNKNOWN'].includes(s.status)).length;

  const filteredScreenings = screenings.filter((s) => {
    if (filter === 'ALL') return true;
    if (filter === 'PASS') return s.status === 'PASS';
    if (filter === 'FAIL') return s.status === 'FAIL';
    if (filter === 'REVIEW') return ['REVIEW_REQUIRED', 'DATA_INSUFFICIENT', 'UNKNOWN'].includes(s.status);
    return true;
  });

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Shariah Compliance Screening</h1>
          <p className="text-slate-400 mt-1">
            Auditable AAOIFI Standard financial ratio screening with strict trading restrictions
          </p>
        </div>
      </div>

      {/* Dynamic Summary Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-6">
        <div 
          onClick={() => setFilter('PASS')}
          className={`p-5 rounded-2xl bg-slate-900/60 border cursor-pointer transition-all ${
            filter === 'PASS' ? 'border-emerald-500 ring-1 ring-emerald-500' : 'border-slate-800 hover:border-slate-700'
          }`}
        >
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Compliant (PASS)</span>
            <ShieldCheckIcon className="w-5 h-5 text-emerald-400" />
          </div>
          <p className="text-3xl font-bold text-emerald-400 mt-2">{passCount}</p>
          <p className="text-xs text-slate-500 mt-1">Eligible for algorithmic breakout recommendation</p>
        </div>

        <div 
          onClick={() => setFilter('FAIL')}
          className={`p-5 rounded-2xl bg-slate-900/60 border cursor-pointer transition-all ${
            filter === 'FAIL' ? 'border-rose-500 ring-1 ring-rose-500' : 'border-slate-800 hover:border-slate-700'
          }`}
        >
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Non-Compliant (FAIL)</span>
            <ShieldExclamationIcon className="w-5 h-5 text-rose-400" />
          </div>
          <p className="text-3xl font-bold text-rose-400 mt-2">{failCount}</p>
          <p className="text-xs text-slate-500 mt-1">Hard trading restriction enforced (0% allocation)</p>
        </div>

        <div 
          onClick={() => setFilter('REVIEW')}
          className={`p-5 rounded-2xl bg-slate-900/60 border cursor-pointer transition-all ${
            filter === 'REVIEW' ? 'border-amber-500 ring-1 ring-amber-500' : 'border-slate-800 hover:border-slate-700'
          }`}
        >
          <div className="flex justify-between items-center text-slate-400">
            <span className="text-xs font-semibold uppercase tracking-wider">Review / Insufficient</span>
            <QuestionMarkCircleIcon className="w-5 h-5 text-amber-400" />
          </div>
          <p className="text-3xl font-bold text-amber-400 mt-2">{reviewCount}</p>
          <p className="text-xs text-slate-500 mt-1">Treated as ineligible until filings are verified</p>
        </div>
      </div>

      {/* AAOIFI Criteria Reference Box */}
      <Card>
        <CardHeader 
          title="Active Rule Set: AAOIFI Financial Screening Criteria" 
          subtitle="Strict financial statement limits applied to 12-month trailing filings" 
        />
        <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 mt-4">
          <div className="p-3.5 rounded-xl bg-slate-800/50 border border-slate-700/50">
            <p className="text-[11px] text-slate-400">Debt / Market Cap</p>
            <p className="text-lg font-bold text-white mt-1">&le; 33.0%</p>
            <p className="text-[10px] text-slate-500 mt-0.5">Interest-bearing debt</p>
          </div>
          <div className="p-3.5 rounded-xl bg-slate-800/50 border border-slate-700/50">
            <p className="text-[11px] text-slate-400">Interest Income / Revenue</p>
            <p className="text-lg font-bold text-white mt-1">&le; 5.0%</p>
            <p className="text-[10px] text-slate-500 mt-0.5">Prohibited income ratio</p>
          </div>
          <div className="p-3.5 rounded-xl bg-slate-800/50 border border-slate-700/50">
            <p className="text-[11px] text-slate-400">Cash & Deposits / Cap</p>
            <p className="text-lg font-bold text-white mt-1">&le; 33.0%</p>
            <p className="text-[10px] text-slate-500 mt-0.5">Interest-yielding liquid assets</p>
          </div>
          <div className="p-3.5 rounded-xl bg-slate-800/50 border border-slate-700/50">
            <p className="text-[11px] text-slate-400">Receivables / Market Cap</p>
            <p className="text-lg font-bold text-white mt-1">&le; 49.0%</p>
            <p className="text-[10px] text-slate-500 mt-0.5">Accounts receivable limit</p>
          </div>
        </div>

        <div className="mt-4 p-3 rounded-lg bg-slate-800/30 border border-slate-700/50 flex items-start gap-2.5 text-xs text-slate-400">
          <InformationCircleIcon className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
          <span>
            <strong>Hard Gate Restriction:</strong> Shariah failure is non-overridable. No technical momentum, volume breakout, or high quant score can bypass an ineligible ruling. Unknown or unverified records are safely treated as ineligible.
          </span>
        </div>
      </Card>

      {/* Screenings Ledger Table */}
      <Card>
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 border-b border-slate-800 pb-4">
          <div>
            <h3 className="text-lg font-bold text-white">Evaluated Equity Universe</h3>
            <p className="text-xs text-slate-400 mt-0.5">Stored screening records. DATA_INSUFFICIENT means unevaluated, never compliant.</p>
          </div>
          <div className="flex items-center gap-1.5 text-xs">
            {FILTERS.map((f) => (
              <button
                key={f}
                onClick={() => setFilter(f)}
                className={`px-3 py-1.5 rounded-lg font-medium transition-colors ${
                  filter === f ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-white'
                }`}
              >
                {f}
              </button>
            ))}
          </div>
        </div>

        <div className="overflow-x-auto mt-4">
          <table className="w-full text-left border-collapse text-xs">
            <thead>
              <tr className="border-b border-slate-800 text-slate-400 font-semibold uppercase text-[10px]">
                <th className="pb-3 pl-2">Symbol</th>
                <th className="pb-3">Sector</th>
                <th className="pb-3 text-center">Status</th>
                <th className="pb-3 text-right">Debt / Cap</th>
                <th className="pb-3 text-right">Interest / Rev</th>
                <th className="pb-3 text-right">Cash / Cap</th>
                <th className="pb-3 text-right">Receivables / Cap</th>
                <th className="pb-3 text-right pr-2">Screening Date</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {isLoading ? (
                <tr>
                  <td colSpan={8} className="py-8 text-center text-slate-500">Loading screening database...</td>
                </tr>
              ) : filteredScreenings.length > 0 ? (
                filteredScreenings.map((s) => (
                  <tr key={s.id || s.symbol} className="hover:bg-slate-800/20 transition-colors">
                    <td className="py-3 pl-2 font-bold text-white">{s.symbol}</td>
                    <td className="py-3 text-slate-400">{s.sector || 'NSE Cash'}</td>
                    <td className="py-3 text-center">
                      <Badge variant={s.status === 'PASS' ? 'pass' : (s.status === 'FAIL' ? 'fail' : 'neutral')}>
                        {s.status}
                      </Badge>
                    </td>
                    <td className={`py-3 text-right ${(s.debt_ratio ?? 0) > 0.33 ? 'text-rose-400 font-semibold' : 'text-slate-300'}`}>
                      {s.debt_ratio != null ? `${(s.debt_ratio * 100).toFixed(1)}%` : 'N/A'}
                    </td>
                    <td className={`py-3 text-right ${(s.interest_income_ratio ?? 0) > 0.05 ? 'text-rose-400 font-semibold' : 'text-slate-300'}`}>
                      {s.interest_income_ratio != null ? `${(s.interest_income_ratio * 100).toFixed(2)}%` : 'N/A'}
                    </td>
                    <td className={`py-3 text-right ${(s.cash_deposit_ratio ?? 0) > 0.33 ? 'text-rose-400 font-semibold' : 'text-slate-300'}`}>
                      {s.cash_deposit_ratio != null ? `${(s.cash_deposit_ratio * 100).toFixed(1)}%` : 'N/A'}
                    </td>
                    <td className={`py-3 text-right ${(s.receivables_ratio ?? 0) > 0.49 ? 'text-rose-400 font-semibold' : 'text-slate-300'}`}>
                      {s.receivables_ratio != null ? `${(s.receivables_ratio * 100).toFixed(1)}%` : 'N/A'}
                    </td>
                    <td className="py-3 text-right pr-2 text-slate-500 font-mono text-[11px]">
                      {s.screening_date ? new Date(s.screening_date).toLocaleDateString() : '—'}
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td colSpan={8} className="py-10 text-center text-slate-500">
                    No securities match the active filter.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  );
}
