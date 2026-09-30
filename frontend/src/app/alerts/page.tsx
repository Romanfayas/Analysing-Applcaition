export default function AlertsPage() {
  return (
    <div className="space-y-6">
      <div><h1 className="text-2xl font-bold text-white">Alerts</h1><p className="text-sm text-slate-400 mt-1">Telegram, email, and web push notifications</p></div>
      <div className="glass-card p-6"><p className="text-slate-500">Alert system with Telegram, email, and web push notifications coming in Phase 9. Alerts for: new BUY signals, signal changes, stop loss reached, target reached, IPO events, Shariah status changes, and data quality failures. Every alert contains the reason and timestamp.</p></div>
    </div>
  );
}
