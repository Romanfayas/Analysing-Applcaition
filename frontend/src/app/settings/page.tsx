'use client';

import { useState } from 'react';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { send } from '@/lib/api/client';
import { useSettings } from '@/lib/api/hooks';
import type { SettingsResponse, UserPreferences } from '@/lib/api/types';

const field = 'w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm';

export default function SettingsPage() {
  const { settings, isLoading, isError, mutate } = useSettings();
  if (isLoading) return <p className="text-slate-500">Loading settings…</p>;
  if (isError || !settings) return <p className="text-rose-400">Could not load settings.</p>;
  return <SettingsForm settings={settings} onSaved={() => mutate()} />;
}

// Mounted only once the saved preferences exist, so the editable copy starts from real values.
function SettingsForm({ settings, onSaved }: { settings: SettingsResponse; onSaved: () => Promise<unknown> }) {
  const [form, setForm] = useState<UserPreferences>(settings.preferences);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<{ type: 'ok' | 'error'; text: string } | null>(null);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    setMessage(null);
    try {
      await send('put', '/settings', form);
      await onSaved();
      setMessage({ type: 'ok', text: 'Settings saved.' });
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Could not save settings.' });
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="space-y-6 pb-12">
      <div>
        <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Settings</h1>
        <p className="text-sm text-slate-400 mt-1">Your personal preferences. Platform configuration is managed on the server.</p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card>
          <CardHeader title="Preferences" />
          <form onSubmit={save} className="space-y-4">
            <label className="block text-xs text-slate-500 uppercase">
              Default benchmark
              <select className={field} value={form.default_benchmark} onChange={(e) => setForm({ ...form, default_benchmark: e.target.value })}>
                <option>NIFTY 50</option>
                <option>NIFTY 500</option>
                <option>SENSEX</option>
              </select>
            </label>
            <label className="block text-xs text-slate-500 uppercase">
              Default risk per trade (%)
              <input
                className={field}
                type="number"
                min="0.1"
                max="5"
                step="0.1"
                value={form.default_risk_pct}
                onChange={(e) => setForm({ ...form, default_risk_pct: Number(e.target.value) })}
                required
              />
            </label>
            <label className="block text-xs text-slate-500 uppercase">
              Display timezone
              <select className={field} value={form.display_timezone} onChange={(e) => setForm({ ...form, display_timezone: e.target.value })}>
                <option value="Asia/Kolkata">Asia/Kolkata (IST)</option>
                <option value="UTC">UTC</option>
              </select>
            </label>
            <button disabled={saving} className="px-5 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-60 text-white text-sm font-semibold">
              {saving ? 'Saving…' : 'Save preferences'}
            </button>
            {message && (
              <p role={message.type === 'error' ? 'alert' : 'status'} className={`text-sm ${message.type === 'error' ? 'text-rose-400' : 'text-emerald-400'}`}>
                {message.text}
              </p>
            )}
          </form>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader title="Notifications" />
            <div className="flex items-center justify-between text-sm">
              <span className="text-slate-300">Telegram delivery</span>
              <Badge variant={settings.notifications.telegram_configured ? 'pass' : 'neutral'}>
                {settings.notifications.telegram_configured ? 'Configured' : 'Not configured'}
              </Badge>
            </div>
            <p className="text-xs text-slate-500 mt-3">{settings.notifications.note}</p>
          </Card>

          <Card>
            <CardHeader title="Execution safety" subtitle="Fixed by server configuration" />
            <ul className="space-y-2 text-sm text-slate-300">
              <li className="flex justify-between"><span>Paper trading only</span><Badge variant="pass">{settings.execution_safety.paper_trading_only ? 'Yes' : 'No'}</Badge></li>
              <li className="flex justify-between"><span>Live order execution</span><Badge variant="neutral">{settings.execution_safety.live_order_execution ? 'On' : 'Off'}</Badge></li>
              <li className="flex justify-between"><span>Broker trading</span><Badge variant="neutral">{settings.execution_safety.broker_trading_enabled ? 'On' : 'Off'}</Badge></li>
            </ul>
          </Card>
        </div>
      </div>
    </div>
  );
}
