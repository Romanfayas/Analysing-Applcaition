import { ReactNode } from 'react';
import Card from './Card';

interface StatCardProps {
  title: string;
  value: string;
  change?: string;
  isPositive?: boolean;
  icon: ReactNode;
}

export default function StatCard({ title, value, change, isPositive, icon }: StatCardProps) {
  return (
    <Card gradientHover>
      <div className="flex items-start justify-between">
        <div>
          <p className="text-sm font-medium text-slate-400">{title}</p>
          <h3 className="text-3xl font-heading font-bold text-white mt-2 tracking-tight">{value}</h3>
          
          {change && (
            <div className={`flex items-center gap-1 mt-2 text-sm font-medium ${isPositive ? 'text-emerald-400' : 'text-rose-400'}`}>
              <span>{isPositive ? '↑' : '↓'}</span>
              <span>{change}</span>
              <span className="text-slate-500 ml-1">vs last month</span>
            </div>
          )}
        </div>
        <div className={`p-3 rounded-xl bg-slate-800/50 border border-slate-700/50 ${isPositive ? 'text-emerald-400 shadow-[inset_0_0_15px_rgba(16,185,129,0.05)]' : 'text-blue-400 shadow-[inset_0_0_15px_rgba(59,130,246,0.05)]'}`}>
          {icon}
        </div>
      </div>
    </Card>
  );
}
