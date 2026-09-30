"use client";

import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import PredictionBarChart from '@/components/charts/PredictionBarChart';
import MarkdownRenderer from '@/components/ui/MarkdownRenderer';
import { SparklesIcon, ChartBarSquareIcon } from '@heroicons/react/24/outline';

import { useIPOs } from '@/lib/api/hooks';
import { IPO } from '@/lib/api/types';
import { useState } from 'react';

export default function IPOPage() {
  const { ipos, isLoading } = useIPOs('active');
  const [selectedIpo, setSelectedIpo] = useState<IPO | null>(null);


  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-12">
      <div className="flex justify-between items-end mb-8">
        <div>
          <h1 className="text-3xl font-heading font-bold text-white tracking-tight">IPO Analysis</h1>
          <p className="text-slate-400 mt-1">Predictive listing models and GMP tracking</p>
        </div>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        
        {/* Left Column: IPO Grid */}
        <div className="lg:col-span-2 space-y-4">
          <h3 className="text-xl font-heading font-semibold text-white mb-4">Upcoming & Active Issues</h3>
          
          {isLoading ? (
            <div className="text-slate-500 py-8 text-center border border-slate-700/50 rounded-xl bg-slate-800/20">Loading IPOs...</div>
          ) : ipos && ipos.length > 0 ? (
            ipos.map((ipo) => (
              <Card key={ipo.id} gradientHover className={`cursor-pointer ${selectedIpo?.id === ipo.id ? 'border-blue-500/50' : ''}`} onClick={() => setSelectedIpo(ipo)}>
                <div className="flex justify-between items-start">
                  <div>
                    <div className="flex items-center gap-3 mb-1">
                      <h4 className="text-lg font-bold text-white">{ipo.name}</h4>
                      <Badge variant={ipo.prediction?.shariah_status === 'PASS' ? 'pass' : (ipo.prediction?.shariah_status === 'FAIL' ? 'fail' : 'neutral')}>
                        {ipo.prediction?.shariah_status || 'UNKNOWN'}
                      </Badge>
                    </div>
                    <p className="text-sm text-slate-400">{ipo.symbol}</p>
                  </div>
                  <div className="text-right">
                    <p className="text-sm text-slate-400">Current GMP</p>
                    <p className={`font-bold ${ipo.gmp ? 'text-emerald-400' : 'text-slate-500'}`}>
                      {ipo.gmp ? `₹${ipo.gmp.value}` : 'DATA_UNAVAILABLE'}
                    </p>
                  </div>
                </div>
                
                <div className="grid grid-cols-3 gap-4 mt-6 pt-4 border-t border-slate-700/50">
                  <div>
                    <p className="text-xs text-slate-500 mb-1">Issue Size</p>
                    <p className="text-sm font-medium text-slate-300">{ipo.issue_size}</p>
                  </div>
                  <div>
                    <p className="text-xs text-slate-500 mb-1">Price Band</p>
                    <p className="text-sm font-medium text-slate-300">{ipo.price_band}</p>
                  </div>
                  <div>
                    <p className="text-xs text-slate-500 mb-1">Subscription</p>
                    <p className="text-sm font-medium text-slate-300">{ipo.subscription?.total ? `${ipo.subscription.total}x` : 'N/A'}</p>
                  </div>
                </div>
              </Card>
            ))
          ) : (
            <div className="text-slate-500 py-8 text-center border border-slate-700/50 rounded-xl bg-slate-800/20">No active IPOs found.</div>
          )}
        </div>

        {/* Right Column: Predictive View */}
        <div className="lg:col-span-1 space-y-6">
          <Card className="border-blue-500/20 shadow-[0_0_30px_rgba(59,130,246,0.05)]">
            <CardHeader 
              title={selectedIpo ? selectedIpo.name : "Select an IPO"} 
              subtitle="Algorithmic Listing Prediction" 
              action={<SparklesIcon className="w-5 h-5 text-blue-400" />} 
            />
            
            {!selectedIpo ? (
              <div className="py-12 text-center text-slate-500">
                Select an IPO to view quantitative predictions.
              </div>
            ) : !selectedIpo.prediction ? (
              <div className="py-12 text-center text-amber-500/80">
                DATA_UNAVAILABLE
                <p className="text-sm mt-2 text-slate-400">Insufficient historical/GMP data to generate predictions.</p>
              </div>
            ) : (
              <>
                <div className="bg-slate-800/30 p-4 rounded-xl border border-slate-700/50">
                  <div className="flex justify-between items-center mb-2">
                    <span className="text-slate-400 text-sm">Recommendation</span>
                    <span className="text-white font-medium">{selectedIpo.prediction.recommendation}</span>
                  </div>
                  <div className="flex justify-between items-center">
                    <span className="text-slate-400 text-sm">Predicted Base Gain</span>
                    <span className="text-emerald-400 font-bold">+{selectedIpo.prediction.listing_gains_pct.toFixed(1)}%</span>
                  </div>
                </div>
              </>
            )}
          </Card>

          {selectedIpo && selectedIpo.prediction && (
            <Card>
              <div className="flex items-center gap-2 mb-4 border-b border-slate-700/50 pb-3">
                <ChartBarSquareIcon className="w-5 h-5 text-emerald-400" />
                <h4 className="font-heading font-semibold text-white">Quant Rationale</h4>
              </div>
              <MarkdownRenderer content={selectedIpo.prediction.explanation} />
            </Card>
          )}
        </div>

      </div>
    </div>
  );
}
