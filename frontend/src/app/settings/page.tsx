export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <div><h1 className="text-2xl font-bold text-white">Settings</h1><p className="text-sm text-slate-400 mt-1">Configure platform preferences and integrations</p></div>
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="glass-card p-6">
          <h2 className="text-lg font-semibold text-white mb-4">General</h2>
          <div className="space-y-4">
            <div><label className="text-xs text-slate-500 uppercase">Default Benchmark</label><select className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm"><option>NIFTY 50</option><option>NIFTY 500</option><option>SENSEX</option></select></div>
            <div><label className="text-xs text-slate-500 uppercase">Default Risk %</label><input type="number" defaultValue={1} className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm" /></div>
            <div><label className="text-xs text-slate-500 uppercase">Display Timezone</label><select className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm"><option>Asia/Kolkata (IST)</option></select></div>
          </div>
        </div>
        <div className="glass-card p-6">
          <h2 className="text-lg font-semibold text-white mb-4">Notifications</h2>
          <div className="space-y-4">
            <div><label className="text-xs text-slate-500 uppercase">Telegram Bot Token</label><input type="password" placeholder="••••••••" className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm" /></div>
            <div><label className="text-xs text-slate-500 uppercase">Telegram Chat ID</label><input type="text" placeholder="Enter chat ID" className="w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm" /></div>
          </div>
          <p className="text-[10px] text-slate-600 mt-3 italic">Secrets are stored encrypted. Never stored in source code.</p>
        </div>
      </div>
    </div>
  );
}
