"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { 
  ChartBarIcon, 
  BanknotesIcon, 
  PresentationChartLineIcon,
  Cog6ToothIcon 
} from "@heroicons/react/24/outline";

const navigation = [
  { name: 'Dashboard', href: '/', icon: ChartBarIcon },
  { name: 'Screener', href: '/stocks', icon: PresentationChartLineIcon },
  { name: 'Upcoming IPOs', href: '/ipos', icon: BanknotesIcon },
  { name: 'Settings', href: '/settings', icon: Cog6ToothIcon },
];

export default function Sidebar() {
  const pathname = usePathname();

  return (
    <div className="w-64 h-full glass border-l-0 border-t-0 border-b-0 flex flex-col pt-8">
      <div className="px-6 mb-10 flex items-center gap-3">
        <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 to-emerald-400 flex items-center justify-center text-white font-heading font-bold text-lg shadow-[0_0_15px_rgba(16,185,129,0.5)]">
          H
        </div>
        <h1 className="text-xl font-heading font-bold tracking-tight text-white">
          Halal<span className="text-emerald-400">Equity</span>
        </h1>
      </div>

      <nav className="flex-1 px-4 space-y-2">
        {navigation.map((item) => {
          const isActive = pathname === item.href || (pathname.startsWith(item.href) && item.href !== '/');
          
          return (
            <Link
              key={item.name}
              href={item.href}
              className={`flex items-center gap-3 px-4 py-3 rounded-xl transition-all duration-300 group ${
                isActive 
                  ? 'bg-blue-500/10 text-blue-400 border border-blue-500/20 shadow-[inset_0_0_20px_rgba(59,130,246,0.1)]' 
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/50 hover:border hover:border-slate-700'
              } border border-transparent`}
            >
              <item.icon className={`w-5 h-5 transition-transform duration-300 group-hover:scale-110 ${isActive ? 'text-blue-400' : 'text-slate-400'}`} />
              <span className="font-medium text-sm">{item.name}</span>
            </Link>
          );
        })}
      </nav>

      <div className="p-6 mt-auto">
        <div className="glass-card p-4 flex items-center gap-3 border-emerald-500/30 bg-emerald-500/5">
          <div className="w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(16,185,129,0.8)] animate-pulse" />
          <div className="flex flex-col">
            <span className="text-xs text-slate-400 font-medium">Shariah Engine</span>
            <span className="text-sm text-emerald-400 font-bold">ONLINE</span>
          </div>
        </div>
      </div>
    </div>
  );
}
