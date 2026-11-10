"use client";

import Link from "next/link";
import { useRouter, usePathname } from "next/navigation";
import useSWR from "swr";
import {
  ChartBarIcon,
  BanknotesIcon,
  PresentationChartLineIcon,
  BoltIcon,
  SparklesIcon,
  ShieldCheckIcon,
  BriefcaseIcon,
  ClipboardDocumentCheckIcon,
  ScaleIcon,
  BeakerIcon,
  CircleStackIcon,
  AcademicCapIcon,
  ArrowTrendingUpIcon,
  BellIcon,
  Cog6ToothIcon,
  ArrowRightStartOnRectangleIcon,
} from "@heroicons/react/24/outline";

const navigation = [
  { name: "Dashboard", href: "/", icon: ChartBarIcon },
  { name: "Screener", href: "/stocks", icon: PresentationChartLineIcon },
  { name: "Predictions", href: "/predictions", icon: SparklesIcon },
  { name: "Signals", href: "/signals", icon: BoltIcon },
  { name: "Breakouts", href: "/breakouts", icon: ArrowTrendingUpIcon },
  { name: "Upcoming IPOs", href: "/ipos", icon: BanknotesIcon },
  { name: "Shariah", href: "/shariah", icon: ShieldCheckIcon },
  { name: "Portfolio", href: "/portfolio", icon: BriefcaseIcon },
  { name: "Paper Trading", href: "/paper-trading", icon: ClipboardDocumentCheckIcon },
  { name: "Risk", href: "/risk", icon: ScaleIcon },
  { name: "Backtesting", href: "/backtesting", icon: BeakerIcon },
  { name: "Learning", href: "/learning", icon: AcademicCapIcon },
  { name: "Data Quality", href: "/data-quality", icon: CircleStackIcon },
  { name: "Alerts", href: "/alerts", icon: BellIcon },
  { name: "Settings", href: "/settings", icon: Cog6ToothIcon },
];

interface Me {
  user: { name: string; email: string };
}

const meFetcher = async (url: string): Promise<Me> => {
  const res = await fetch(url, { cache: "no-store" });
  if (!res.ok) throw new Error("unauthenticated");
  return res.json();
};

export default function Sidebar() {
  const pathname = usePathname();
  const router = useRouter();
  const { data: me } = useSWR<Me>("/api/auth/me", meFetcher, { shouldRetryOnError: false });

  async function signOut() {
    await fetch("/api/auth/logout", { method: "POST" });
    router.replace("/login");
    router.refresh();
  }

  return (
    <div className="w-64 h-full glass border-l-0 border-t-0 border-b-0 flex flex-col pt-8">
      <div className="px-6 mb-8 flex items-center gap-3">
        <div className="w-8 h-8 rounded-lg bg-gradient-to-br from-blue-500 to-emerald-400 flex items-center justify-center text-white font-heading font-bold text-lg shadow-[0_0_15px_rgba(16,185,129,0.5)]">
          H
        </div>
        <h1 className="text-xl font-heading font-bold tracking-tight text-white">
          Halal<span className="text-emerald-400">Equity</span>
        </h1>
      </div>

      <nav className="flex-1 px-4 space-y-1 overflow-y-auto">
        {navigation.map((item) => {
          const isActive = item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
          return (
            <Link
              key={item.name}
              href={item.href}
              aria-current={isActive ? "page" : undefined}
              className={`flex items-center gap-3 px-4 py-2.5 rounded-xl transition-all duration-300 group border ${
                isActive
                  ? "bg-blue-500/10 text-blue-400 border-blue-500/20 shadow-[inset_0_0_20px_rgba(59,130,246,0.1)]"
                  : "text-slate-400 hover:text-slate-200 hover:bg-slate-800/50 hover:border-slate-700 border-transparent"
              }`}
            >
              <item.icon className={`w-5 h-5 transition-transform duration-300 group-hover:scale-110 ${isActive ? "text-blue-400" : "text-slate-400"}`} />
              <span className="font-medium text-sm">{item.name}</span>
            </Link>
          );
        })}
      </nav>

      <div className="p-4 mt-2 border-t border-slate-800/60">
        <div className="flex items-center gap-3 px-2 py-2">
          <div className="w-8 h-8 rounded-full bg-gradient-to-br from-blue-500 to-emerald-400 flex items-center justify-center text-white text-xs font-bold shrink-0">
            {(me?.user.name ?? "?").charAt(0).toUpperCase()}
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-sm text-white font-medium truncate">{me?.user.name ?? "…"}</p>
            <p className="text-[11px] text-slate-500 truncate">{me?.user.email ?? ""}</p>
          </div>
          <button
            onClick={signOut}
            title="Sign out"
            aria-label="Sign out"
            className="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800"
          >
            <ArrowRightStartOnRectangleIcon className="w-5 h-5" />
          </button>
        </div>
      </div>
    </div>
  );
}
