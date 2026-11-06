'use client';

import { useState } from 'react';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { send } from '@/lib/api/client';
import { useAlertHistory, useAlerts, useStocks } from '@/lib/api/hooks';
import type { UserAlert } from '@/lib/api/types';

const inr = (n: number) => `₹${n.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
const field = 'w-full mt-1 px-3 py-2 rounded-lg bg-slate-800 border border-slate-700 text-white text-sm';

export default function AlertsPage() {
  const { stocks } = useStocks();
  const { alerts, isLoading, isError, mutate } = useAlerts();
  const { events, mutate: mutateHistory } = useAlertHistory();

  const [symbol, setSymbol] = useState('');
  const [alertType, setAlertType] = useState<UserAlert['alert_type']>('PRICE_ABOVE');
  const [threshold, setThreshold] = useState('');
  const [channel, setChannel] = useState<UserAlert['channel']>('IN_APP');
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState<{ type: 'ok' | 'error'; text: string } | null>(null);

  const selected = stocks.find((s) => s.symbol === symbol);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setMessage(null);
    try {
      await send<UserAlert>('post', '/alerts', { symbol, alert_type: alertType, threshold: Number(threshold), channel });
      setMessage({ type: 'ok', text: `Alert created for ${symbol}.` });
      setThreshold('');
      await mutate();
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Could not create the alert.' });
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: number) {
    try {
      await send('delete', `/alerts/${id}`);
      await Promise.all([mutate(), mutateHistory()]);
    } catch (err) {
      setMessage({ type: 'error', text: err instanceof Error ? err.message : 'Could not delete the alert.' });
    }
  }

  return (
    <div className="space-y-8 pb-12">
      <div>
        <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Alerts</h1>
        <p className="text-sm text-slate-400 mt-1">
          Price alerts are checked every 5 minutes against the latest stored price and fire once. An alert does not fire on a price older than 4 days.
        </p>
      </div>

      <Card>
        <CardHeader title="New price alert" subtitle="Telegram delivery goes to the platform operator's configured chat" />
        <form onSubmit={create} className="grid grid-cols-1 md:grid-cols-5 gap-4 items-end">
          <label className="text-xs text-slate-500 uppercase">
            Symbol
            <select className={field} value={symbol} onChange={(e) => setSymbol(e.target.value)} required>
              <option value="">Select…</option>
              {stocks.map((s) => (
                <option key={s.symbol} value={s.symbol}>{s.symbol}</option>
              ))}
            </select>
          </label>
          <label className="text-xs text-slate-500 uppercase">
            Condition
            <select className={field} value={alertType} onChange={(e) => setAlertType(e.target.value as UserAlert['alert_type'])}>
              <option value="PRICE_ABOVE">Price at or above</option>
              <option value="PRICE_BELOW">Price at or below</option>
            </select>
          </label>
          <label className="text-xs text-slate-500 uppercase">
            Threshold (₹)
            <input className={field} type="number" min="0.01" step="0.01" value={threshold} onChange={(e) => setThreshold(e.target.value)} required />
            {selected?.price != null && <span className="block normal-case text-[11px] text-slate-500 mt-1">Last stored: {inr(selected.price)}</span>}
          </label>
          <label className="text-xs text-slate-500 uppercase">
            Channel
            <select className={field} value={channel} onChange={(e) => setChannel(e.target.value as UserAlert['channel'])}>
              <option value="IN_APP">In-app (history below)</option>
              <option value="TELEGRAM">Telegram</option>
            </select>
          </label>
          <button disabled={busy} className="py-2 rounded-lg bg-blue-600 hover:bg-blue-500 disabled:opacity-60 text-white text-sm font-semibold">
            {busy ? 'Saving…' : 'Create alert'}
          </button>
        </form>
        {message && (
          <p role={message.type === 'error' ? 'alert' : 'status'} className={`mt-4 text-sm ${message.type === 'error' ? 'text-rose-400' : 'text-emerald-400'}`}>
            {message.text}
          </p>
        )}
      </Card>

      <Card>
        <CardHeader title="Your alerts" />
        {isLoading ? (
          <p className="text-slate-500 text-sm">Loading…</p>
        ) : isError ? (
          <p className="text-rose-400 text-sm">Could not load alerts.</p>
        ) : alerts.length === 0 ? (
          <p className="text-slate-500 text-sm">No alerts yet.</p>
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="text-slate-400 text-xs uppercase border-b border-slate-800">
                <th className="pb-2">Symbol</th><th className="pb-2">Condition</th><th className="pb-2 text-right">Threshold</th>
                <th className="pb-2">Channel</th><th className="pb-2">Status</th><th className="pb-2" />
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60">
              {alerts.map((a) => (
                <tr key={a.id}>
                  <td className="py-3 font-semibold text-white">{a.symbol}</td>
                  <td className="py-3 text-slate-300">{a.alert_type === 'PRICE_ABOVE' ? 'At or above' : 'At or below'}</td>
                  <td className="py-3 text-right font-mono text-slate-300">{inr(a.threshold)}</td>
                  <td className="py-3 text-slate-300">{a.channel === 'TELEGRAM' ? 'Telegram' : 'In-app'}</td>
                  <td className="py-3">
                    {a.is_active ? <Badge variant="blue">Watching</Badge> : <Badge variant="neutral">Triggered</Badge>}
                  </td>
                  <td className="py-3 text-right">
                    <button onClick={() => remove(a.id)} className="text-xs text-slate-400 hover:text-rose-400" aria-label={`Delete alert for ${a.symbol}`}>
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <Card>
        <CardHeader title="Trigger history" subtitle="Every alert that has fired, with the price that triggered it" />
        {events.length === 0 ? (
          <p className="text-slate-500 text-sm">Nothing has triggered yet.</p>
        ) : (
          <ul className="space-y-3 text-sm">
            {events.map((ev) => (
              <li key={ev.id} className="border-b border-slate-800/60 pb-3">
                <p className="text-white">{ev.message}</p>
                <p className="text-xs text-slate-500 mt-0.5">Fired {new Date(ev.triggered_at).toLocaleString()}</p>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
