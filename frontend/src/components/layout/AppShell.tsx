"use client";

import { usePathname } from "next/navigation";
import Sidebar from "@/components/layout/Sidebar";

// The sign-in page renders without navigation; everything else gets the sidebar shell.
export default function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  if (pathname === "/login") {
    return <main className="flex-1 overflow-y-auto">{children}</main>;
  }
  return (
    <>
      <Sidebar />
      <main className="flex-1 overflow-y-auto p-8 relative">
        <div className="max-w-7xl mx-auto">{children}</div>
      </main>
    </>
  );
}
