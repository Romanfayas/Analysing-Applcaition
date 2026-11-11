'use client';

import { useState } from 'react';
import Link from '@/components/Link';
import { CheckCircleIcon, XCircleIcon, InformationCircleIcon } from '@heroicons/react/24/outline';
import Card, { CardHeader } from '@/components/ui/Card';
import Badge from '@/components/ui/Badge';
import { useHistoricalReadiness, useLatestPredictions, usePredictionModels, usePredictionTracking } from '@/lib/api/hooks';
import type { ConfidenceInterval, Prediction, PredictionModel, PredictionVerdict } from '@/lib/api/types';

const HORIZONS = [5, 15, 60];
const pct = (v: number | null | undefined, d = 2) => (v == null ? '—' : `${v >= 0 ? '+' : ''}${(v * 100).toFixed(d)}%`);
const inr = (v: number | null) => (v == null ? '—' : `₹${v.toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`);
const ci = (c?: ConfidenceInterval) => (c && c.estimate != null ? `${pct(c.estimate, 3)}  [${pct(c.low, 3)}, ${pct(c.high, 3)}]` : '—');

const CHECK_LABELS: Record<string, string> = {
  enough_independent_test_days: 'At least 20 independent test days',
  enough_trades: 'Enough signals to judge',
  probabilistic_skill_ci_above_zero: 'Probabilities beat the historical base rate',
  net_return_ci_above_zero: 'Signalled trades profitable after costs',
};

/** True once the test window is long enough that "no edge" is a finding rather than "not enough data yet". */
const wellTested = (m: PredictionModel) => m.metrics.edge_checks?.enough_independent_test_days === true;

function verdictBadge(v: PredictionVerdict, tested: boolean) {
  if (v === 'ACTIONABLE') return <Badge variant="pass">ACTIONABLE</Badge>;
  if (v === 'DATA_INSUFFICIENT') return <Badge variant="neutral">DATA INSUFFICIENT</Badge>;
  return tested ? <Badge variant="neutral">TESTED · NO EDGE AFTER COSTS</Badge> : <Badge variant="warning">TOO EARLY TO JUDGE</Badge>;
}

function ModelCard({ m }: { m: PredictionModel }) {
  const t = m.metrics.trading;
  return (
    <Card>
      <CardHeader title={`Next ${m.horizon_minutes} minutes`} subtitle={`${m.model_version} · trained ${new Date(m.trained_at).toLocaleString()}`} action={verdictBadge(m.verdict, wellTested(m))} />
      {m.verdict === 'DATA_INSUFFICIENT' ? (
        <p className="text-sm text-slate-400">{m.metrics.reason}</p>
      ) : (
        <div className="space-y-3 text-sm">
          <p className="text-xs text-slate-500">
            Data {m.data_from} → {m.data_to} · {m.n_symbols} symbols · tested out-of-sample on {m.metrics.test_days?.length ?? 0} days
            ({m.metrics.oos_rows?.toLocaleString()} forecasts) · round-trip cost {pct(m.cost_model.round_trip_cost_fraction, 3)}
          </p>
          <div className="grid grid-cols-2 gap-x-4 gap-y-1.5">
            <span className="text-slate-400">Signals</span><span className="text-white font-mono text-right">{t?.model_signals ?? 0}</span>
            <span className="text-slate-400">Net return / trade (95% CI)</span><span className="text-white font-mono text-right text-xs">{ci(t?.model_net_return_per_trade)}</span>
            <span className="text-slate-500" title="Each trade must first earn back the round-trip cost before it makes anything">Gross return / trade (before costs)</span>
            <span className="text-slate-400 font-mono text-right">{t?.model_net_return_per_trade.estimate != null ? pct(t.model_net_return_per_trade.estimate + m.cost_model.round_trip_cost_fraction, 3) : '—'}</span>
            <span className="text-slate-500">Round-trip cost / trade</span>
            <span className="text-slate-400 font-mono text-right">{pct(-m.cost_model.round_trip_cost_fraction, 3)}</span>
            <span className="text-slate-500" title="Very frequent trading is how costs eat a small edge">Signals per test day</span>
            <span className="text-slate-400 font-mono text-right">{m.metrics.test_days?.length ? ((t?.model_signals ?? 0) / m.metrics.test_days.length).toFixed(1) : '—'}</span>
            <span className="text-slate-400">Hit rate after costs</span><span className="text-white font-mono text-right">{t?.model_hit_rate_net == null ? '—' : `${(t.model_hit_rate_net * 100).toFixed(1)}%`}</span>
            <span className="text-slate-500">Baseline: momentum</span><span className="text-slate-400 font-mono text-right">{pct(t?.baseline_momentum_net_return_per_trade.estimate, 3)}</span>
            <span className="text-slate-500">Baseline: always long</span><span className="text-slate-400 font-mono text-right">{pct(t?.baseline_always_long_net_return_per_trade.estimate, 3)}</span>
            <span className="text-slate-500">Accuracy (model / base rate)</span>
            <span className="text-slate-400 font-mono text-right">
              {m.metrics.accuracy ? `${(m.metrics.accuracy.model * 100).toFixed(1)}% / ${(m.metrics.accuracy.climatology * 100).toFixed(1)}%` : '—'}
            </span>
          </div>
          {m.metrics.per_day && (
            <div className="text-xs text-slate-400 pt-2 border-t border-slate-800 space-y-0.5">
              <p>Net-positive test days: <span className="text-white font-mono">{m.metrics.positive_net_days ?? 0} / {m.metrics.per_day.length}</span></p>
              {Object.entries(m.metrics.regimes ?? {}).map(([reg, r]) => (
                <p key={reg} className="font-mono">
                  {reg.replaceAll('_', ' ')}: {r.days}d · {r.signals} signals · {pct(r.mean_net_return, 3)}
                </p>
              ))}
            </div>
          )}
          <ul className="pt-2 border-t border-slate-800 space-y-1">
            {Object.entries(m.metrics.edge_checks ?? {}).map(([k, ok]) => (
              <li key={k} className="flex items-center gap-2 text-xs">
                {ok ? <CheckCircleIcon className="w-4 h-4 text-emerald-400" /> : <XCircleIcon className="w-4 h-4 text-rose-400" />}
                <span className={ok ? 'text-slate-300' : 'text-slate-400'}>{CHECK_LABELS[k] ?? k}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </Card>
  );
}

function actionBadge(p: Prediction) {
  if (p.action === 'BUY_CANDIDATE') return <Badge variant="pass">BUY CANDIDATE</Badge>;
  if (p.action === 'EXIT_OR_AVOID') return <Badge variant="fail">EXIT / AVOID</Badge>;
  const view = p.action_reasons.find((r) => r.code === 'MODEL_VIEW')?.detail;
  return <Badge variant="neutral">NO TRADE{view && view !== 'NO_EDGE_AFTER_COSTS' ? ` (model: ${view.replaceAll('_', ' ').toLowerCase()})` : ''}</Badge>;
}

function ProbBar({ p }: { p: Prediction }) {
  return (
    <div className="w-36" title={`Down ${(p.p_down * 100).toFixed(0)}% · Flat ${(p.p_neutral * 100).toFixed(0)}% · Up ${(p.p_up * 100).toFixed(0)}%`}>
      <div className="flex h-2 rounded overflow-hidden bg-slate-800">
        <div className="bg-rose-500" style={{ width: `${p.p_down * 100}%` }} />
        <div className="bg-slate-500" style={{ width: `${p.p_neutral * 100}%` }} />
        <div className="bg-emerald-500" style={{ width: `${p.p_up * 100}%` }} />
      </div>
      <div className="flex justify-between text-[10px] text-slate-500 mt-0.5 font-mono">
        <span>↓{(p.p_down * 100).toFixed(0)}</span><span>↑{(p.p_up * 100).toFixed(0)}</span>
      </div>
    </div>
  );
}

export default function PredictionsPage() {
  const [horizon, setHorizon] = useState(60);
  const { models, disclaimer, isLoading: modelsLoading, isError: modelsError } = usePredictionModels();
  const { predictions, isLoading } = useLatestPredictions(horizon);
  const { tracking } = usePredictionTracking();
  const { readiness } = useHistoricalReadiness();
  const nextDay = readiness?.next_day_prediction_research;
  const anyActionable = models.some((m) => m.verdict === 'ACTIONABLE');
  const current = models.find((m) => m.horizon_minutes === horizon);
  const newest = predictions.reduce<string | null>((a, p) => (a && a > p.as_of ? a : p.as_of), null);
  const istDay = (iso: string) => new Date(iso).toLocaleDateString('en-IN', { timeZone: 'Asia/Kolkata', day: '2-digit', month: 'short', year: 'numeric' });
  const todayForecasts = newest != null && istDay(newest) === istDay(new Date().toISOString());

  return (
    <div className="space-y-8 pb-12">
      <div>
        <h1 className="text-3xl font-heading font-bold text-white tracking-tight">Intraday Predictions</h1>
        <p className="text-sm text-slate-400 mt-1">
          Probabilistic, cost-aware forecasts for the next 5, 15 and 60 minutes, validated walk-forward on days the model never saw.
        </p>
      </div>

      {!modelsLoading && !modelsError && !anyActionable && models.length > 0 && (() => {
        const tested = models.every(wellTested);
        const days = Math.min(...models.map((m) => m.metrics.test_days?.length ?? 0));
        return (
          <div role="status" className="flex items-start gap-3 p-4 rounded-xl bg-slate-800/60 border border-slate-700 text-slate-200 text-sm">
            <InformationCircleIcon className="w-5 h-5 shrink-0 mt-0.5 text-blue-400" />
            <div className="space-y-1">
              {tested ? (
                <>
                  <p>
                    <strong>Result of testing: no horizon is profitable after costs, so no BUY recommendations are issued.</strong>{' '}
                    This is a measured finding on {days}+ unseen trading days, not a fault. The model&apos;s probabilities are at best slightly better than
                    the base rate, which is not enough to pay the round-trip trading cost.
                  </p>
                  <p className="text-slate-400">
                    Forecasts are still recorded and checked against what happened. The models are re-tested after every trading day and a horizon
                    only becomes actionable if profit after costs holds up statistically.
                  </p>
                </>
              ) : (
                <p>
                  <strong>Too early to judge:</strong> only {days} independent test days so far (20 are needed). No BUY recommendations are issued meanwhile.
                </p>
              )}
            </div>
          </div>
        );
      })()}
      {modelsError && <p className="text-rose-400">Could not load model evidence.</p>}

      <details className="p-4 rounded-xl bg-slate-800/40 border border-slate-700/60 text-sm text-slate-300">
        <summary className="cursor-pointer font-semibold text-white">How the 5, 15 and 60 minute models work</summary>
        <div className="mt-3 space-y-2 text-xs text-slate-400 leading-relaxed">
          <p>There are three separate models. Each one answers a different question: from this 5-minute bar, will the stock move up, stay flat or fall over the next 5, 15 or 60 minutes, by more than it costs to trade (0.18% round trip)?</p>
          <p>The cards below are each model&apos;s exam result. Every day is tested using only earlier days, so the score reflects days the model never saw. The cards read alike because all three reached the same conclusion: on average, the move they catch (Gross return) is smaller than the cost of trading it (Round-trip cost).</p>
          <p>The table further down is the live part. While the market is open a new forecast is made every few minutes for every stock, and each one is stored unchanged. After its time is up, the real price is filled in beside it. Use the 5, 15 and 60 min buttons on the table to switch models.</p>
        </div>
      </details>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {models.map((m) => (
          <ModelCard key={m.horizon_minutes} m={m} />
        ))}
        {!modelsLoading && models.length === 0 && <p className="text-slate-500">No model has been trained yet.</p>}
      </div>

      {nextDay && (
        <Card>
          <CardHeader title="Next trading day" subtitle="Daily-horizon research" action={<Badge variant="neutral">{nextDay.status.replaceAll('_', ' ')}</Badge>} />
          <p className="text-sm text-slate-400">
            Needs ≥ {nextDay.required_eligible_sessions} research-eligible daily sessions for at least {nextDay.required_symbols} instruments;{' '}
            {nextDay.symbols_meeting_requirement} qualify today. {nextDay.note} See Data Quality → Historical research readiness for what blocks each instrument.
          </p>
        </Card>
      )}

      <Card>
        <CardHeader
          title="Latest forecasts"
          subtitle={`Each tab is a separate model: "what will this stock do in the next ${horizon} minutes", checked afterwards against what really happened`}
          action={
            <div className="flex gap-1">
              {HORIZONS.map((h) => (
                <button
                  key={h}
                  onClick={() => setHorizon(h)}
                  className={`px-3 py-1.5 rounded-lg text-xs font-medium ${horizon === h ? 'bg-blue-600 text-white' : 'bg-slate-800 text-slate-400 hover:text-white'}`}
                >
                  {h} min
                </button>
              ))}
            </div>
          }
        />
        {newest && (
          <p className={`mb-3 text-xs rounded-lg px-3 py-2 border ${todayForecasts ? 'bg-slate-800/40 border-slate-700/60 text-slate-300' : 'bg-amber-500/10 border-amber-500/30 text-amber-200'}`}>
            {todayForecasts
              ? `Showing today's newest ${horizon}-minute forecasts, made from data up to ${new Date(newest).toLocaleTimeString('en-IN', { timeZone: 'Asia/Kolkata', hour: '2-digit', minute: '2-digit' })} IST.`
              : `These are old: the newest ${horizon}-minute forecasts are from ${istDay(newest)}. Forecasts are only made while the market is open and this app is running, so nothing was issued since.`}
          </p>
        )}
        {isLoading ? (
          <p className="text-slate-500 text-sm">Loading…</p>
        ) : predictions.length === 0 ? (
          <p className="text-slate-500 text-sm">
            No {horizon}-minute forecasts yet. They are produced every few minutes while the market is open (09:15–15:30 IST, Mon–Fri).
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead>
                <tr className="text-slate-400 uppercase border-b border-slate-800">
                  <th className="py-2">Symbol</th><th>Action</th><th>Probabilities</th>
                  <th className="text-right" title="The stock's price when the forecast was made">Price then</th>
                  <th className="text-right">Expected</th>
                  <th className="text-right" title="Where the price would be at the 10th and 90th percentile of the model's own past results">Likely price range</th>
                  <th className="text-right" title="Only for an actionable BUY: entry, target and stop">Buy plan</th>
                  <th className="text-right">Data age</th><th>Shariah</th>
                  <th className="text-right" title="What the stock really did over this horizon, whether or not a trade was suggested">What happened</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-800/60">
                {predictions.map((p) => (
                  <tr key={p.id} className="align-top">
                    <td className="py-2.5 font-semibold text-white">
                      <Link href={`/stocks/${p.symbol}`} className="hover:text-blue-400">{p.symbol}</Link>
                      <div className="text-[10px] text-slate-500 font-normal">until {new Date(p.target_time).toLocaleTimeString()}</div>
                    </td>
                    <td className="py-2.5">
                      {actionBadge(p)}
                      <ul className="mt-1 space-y-0.5">
                        {p.action_reasons.filter((r) => r.code !== 'MODEL_VIEW').map((r) => (
                          <li key={r.code} className="text-[10px] text-slate-500 max-w-56" title={r.detail}>{r.code.replaceAll('_', ' ').toLowerCase()}</li>
                        ))}
                      </ul>
                    </td>
                    <td className="py-2.5"><ProbBar p={p} /></td>
                    <td className="py-2.5 text-right font-mono text-white">{inr(p.reference_price)}</td>
                    <td className="py-2.5 text-right font-mono text-slate-300">{pct(p.expected_return, 3)}</td>
                    <td className="py-2.5 text-right font-mono text-slate-400">
                      {inr(p.reference_price * (1 + p.return_q10))} – {inr(p.reference_price * (1 + p.return_q90))}
                      <div className="text-[10px] text-slate-500">{pct(p.return_q10)} … {pct(p.return_q90)}</div>
                    </td>
                    <td className="py-2.5 text-right font-mono">
                      {p.entry_price == null ? <span className="text-slate-600" title="No plan: this forecast is not an actionable buy">no plan</span> : (
                        <span className="text-white">{inr(p.entry_price)}<div className="text-[10px] text-emerald-400">target {inr(p.target_price)}</div><div className="text-[10px] text-rose-400">stop {inr(p.stop_price)}</div></span>
                      )}
                    </td>
                    <td className="py-2.5 text-right font-mono text-slate-400">{Math.round(p.data_latency_seconds / 60)} min</td>
                    <td className="py-2.5 text-slate-400">{p.shariah_status === 'PASS' || p.shariah_status === 'FAIL' ? p.shariah_status : 'Unevaluated'}</td>
                    <td className="py-2.5 text-right font-mono">
                      {p.realized_return == null ? <span className="text-slate-500">pending</span> : (
                        <>
                          <span className={p.realized_return >= 0 ? 'text-emerald-400' : 'text-rose-400'}>{pct(p.realized_return)}</span>
                          <div className="text-[10px] text-slate-500">{inr(p.reference_price)} → {inr(p.realized_price)}</div>
                        </>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {current && current.verdict !== 'ACTIONABLE' && (
              <p className="text-[11px] text-slate-500 mt-3">
                &quot;What happened&quot; is the stock&apos;s real move from the price when the forecast was made, recorded for every forecast so the model can be
                judged. It is not a trade result: a buy plan (entry, target, stop) only appears when a horizon has proven it can beat costs, and none has.
              </p>
            )}
          </div>
        )}
      </Card>

      <Card>
        <CardHeader title="Forward tracking" subtitle="How live forecasts actually turned out (net of the same costs). This evidence cannot be overfitted." />
        {tracking.length === 0 ? (
          <p className="text-slate-500 text-sm">No live forecasts have been issued yet.</p>
        ) : (
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="text-slate-400 text-xs uppercase border-b border-slate-800">
                <th className="py-2">Horizon</th><th className="text-right">Issued</th><th className="text-right">Resolved</th>
                <th className="text-right">Recommended buys</th><th className="text-right">Their net return</th><th className="text-right">Hit rate</th>
                <th className="text-right">Model-view buys net</th><th className="text-right">Always-long net</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-800/60 font-mono">
              {tracking.map((t) => (
                <tr key={t.horizon_minutes}>
                  <td className="py-2 font-sans text-white">{t.horizon_minutes} min</td>
                  <td className="text-right text-slate-300">{t.issued}</td>
                  <td className="text-right text-slate-300">{t.resolved}</td>
                  <td className="text-right text-slate-300">{t.recommended_buys.n}</td>
                  <td className="text-right text-slate-300">{pct(t.recommended_buys.mean_net_return, 3)}</td>
                  <td className="text-right text-slate-300">{t.recommended_buys.hit_rate_net == null ? '—' : `${(t.recommended_buys.hit_rate_net * 100).toFixed(1)}%`}</td>
                  <td className="text-right text-slate-400">{pct(t.model_view_buys.mean_net_return, 3)} ({t.model_view_buys.n})</td>
                  <td className="text-right text-slate-400">{pct(t.baseline_always_long_mean_net_return, 3)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <div className="p-4 rounded-xl bg-slate-800/40 border border-slate-700/60 flex items-start gap-3 text-xs text-slate-400">
        <InformationCircleIcon className="w-5 h-5 text-blue-400 shrink-0" />
        <div className="space-y-1">
          <p>{disclaimer}</p>
          <p>
            Short-horizon round trips are same-day trades. Many AAOIFI-aligned scholars require taking delivery (possession) of shares, so
            confirm the permissibility of intraday trading with your Shariah advisor; these forecasts can also be used purely to time the
            entry of delivery purchases.
          </p>
          <p>Prices come from a delayed, non-exchange-certified feed. Every order is placed manually by you; the platform never trades.</p>
        </div>
      </div>
    </div>
  );
}
