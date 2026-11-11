import useSWR from 'swr';
import { fetcher } from './client';
import type {
  AlertEvent,
  BreakoutScan,
  DataQualityDataset,
  DataQualityIssue,
  HistoricalReadiness,
  IntradayFeedHealth,
  IpoHealth,
  IpoRun,
  IpoView,
  AutoPaperPortfolio,
  BreakoutsOverview,
  UpcomingBreakouts,
  ChartRange,
  StockChart,
  StockDeals,
  LearningSummary,
  StockBreakouts,
  StockEvents,
  StockNews,
  LedgerPosition,
  MarketHealthResponse,
  MarketOverview,
  PaperTrade,
  BreakoutMonitor,
  ResearchRun,
  Prediction,
  PredictionModel,
  PredictionTracking,
  PortfolioRiskMetrics,
  PortfolioSummary,
  SettingsResponse,
  ShariahScreeningRow,
  SignalOverview,
  StockDetail,
  StockListItem,
  UserAlert,
} from './types';

// Every hook returns `isError` so pages can distinguish "empty" from "failed to load".

// ---- Stocks ----

export function useStock(symbol: string) {
  const { data, error, isLoading } = useSWR<StockDetail>(symbol ? `/stocks/${symbol}` : null, fetcher);
  return { stock: data, isLoading, isError: error };
}

export function useStocks() {
  const { data, error, isLoading, mutate } = useSWR<StockListItem[]>('/stocks', fetcher);
  return { stocks: data ?? [], isLoading, isError: error, mutate };
}

export function useMarketDataHealth() {
  const { data, error, isLoading } = useSWR<MarketHealthResponse>('/market-data/health', fetcher);
  return { health: data, isLoading, isError: error };
}

export function useMarketOverview() {
  const { data, error, isLoading } = useSWR<MarketOverview>('/stocks/market/overview', fetcher);
  return { data, isLoading, isError: error };
}

// ---- IPOs ----

export function useIPOs(status: string = 'all') {
  const { data, error, isLoading, mutate } = useSWR<IpoView[]>(`/ipos?status=${status}`, fetcher);
  return { ipos: data ?? [], isLoading, isError: error, mutate };
}

export function useIPOHealth() {
  const { data, error, isLoading, mutate } = useSWR<IpoHealth>('/ipos/health', fetcher);
  return { health: data, isLoading, isError: error, mutate };
}

export function useIPOIngestionRuns() {
  const { data, error, isLoading, mutate } = useSWR<IpoRun[]>('/ipos/ingestion-runs', fetcher);
  return { runs: data ?? [], isLoading, isError: error, mutate };
}

// ---- Signals & breakout research ----

export function useRecentSignals() {
  const { data, error, isLoading } = useSWR<SignalOverview[]>('/signals', fetcher);
  return { signals: data ?? [], isLoading, isError: error };
}

export function useBreakouts() {
  const { data, error, isLoading, mutate, isValidating } = useSWR<BreakoutScan>('/signals/breakouts', fetcher, {
    revalidateOnFocus: true,
    dedupingInterval: 10000,
    refreshInterval: 5 * 60 * 1000, // the server recomputes at most once a minute; poll every 5 minutes
  });
  return {
    scan: data,
    candidates: data?.candidates ?? [],
    stageCounts: data?.stage_counts ?? {
      CONFIRMED_BREAKOUT: 0,
      BREAKOUT_CANDIDATE: 0,
      WATCHLIST: 0,
      FAILED_BREAKOUT: 0,
      UNAVAILABLE: 0,
    },
    bullishBreadth: data?.bullish_breadth_percent ?? 0,
    sectorHeatmap: data?.sector_heatmap ?? [],
    totalScreened: data?.total_screened ?? 0,
    dataAsOf: data?.data_as_of ?? null,
    computedAt: data?.computed_at ?? null,
    staleSessions: data?.stale_sessions ?? 0,
    dataQuality: data?.data_quality,
    basis: data?.basis,
    excluded: data?.excluded ?? [],
    isLoading,
    isValidating,
    isError: error,
    mutate,
  };
}

// ---- Shariah ----

export function useShariahScreenings() {
  const { data, error, isLoading } = useSWR<ShariahScreeningRow[]>('/shariah/screenings', fetcher);
  return { screenings: data ?? [], isLoading, isError: error };
}

// ---- Portfolio & paper ledger ----

interface PortfolioPayload {
  portfolio: PortfolioSummary;
  positions: LedgerPosition[];
}

export function usePortfolio() {
  const { data, error, isLoading, mutate } = useSWR<PortfolioPayload>('/portfolio', fetcher);
  return {
    // undefined (not a made-up balance) until the real one has loaded
    portfolio: data?.portfolio,
    positions: data?.positions ?? [],
    isLoading,
    isError: error,
    mutate,
  };
}

export function usePaperTrades() {
  const { data, error, isLoading, mutate } = useSWR<PaperTrade[]>('/portfolio/trades', fetcher);
  return { trades: data ?? [], isLoading, isError: error, mutate };
}

export function usePortfolioRisk() {
  const { data, error, isLoading } = useSWR<PortfolioRiskMetrics>('/risk/metrics', fetcher);
  return { metrics: data, isLoading, isError: error };
}

// ---- Data quality ----

export function useDataQuality() {
  const datasets = useSWR<DataQualityDataset[]>('/data-quality', fetcher);
  const issues = useSWR<DataQualityIssue[]>('/data-quality/issues', fetcher);
  return {
    datasets: datasets.data ?? [],
    issues: issues.data ?? [],
    isLoading: datasets.isLoading || issues.isLoading,
    isError: datasets.error ?? issues.error,
  };
}

// ---- Alerts & settings ----

export function useAlerts() {
  const { data, error, isLoading, mutate } = useSWR<UserAlert[]>('/alerts', fetcher);
  return { alerts: data ?? [], isLoading, isError: error, mutate };
}

export function useAlertHistory() {
  const { data, error, isLoading, mutate } = useSWR<AlertEvent[]>('/alerts/history', fetcher, { refreshInterval: 60000 });
  return { events: data ?? [], isLoading, isError: error, mutate };
}

export function useSettings() {
  const { data, error, isLoading, mutate } = useSWR<SettingsResponse>('/settings', fetcher);
  return { settings: data, isLoading, isError: error, mutate };
}

// ---- Intraday predictions ----

export function usePredictionModels() {
  const { data, error, isLoading } = useSWR<{ models: PredictionModel[]; disclaimer: string }>('/predictions/models', fetcher);
  return { models: data?.models ?? [], disclaimer: data?.disclaimer, isLoading, isError: error };
}

export function useLatestPredictions(horizon: number) {
  const { data, error, isLoading } = useSWR<{ predictions: Prediction[] }>(`/predictions/latest?horizon=${horizon}`, fetcher, {
    refreshInterval: 60000,
  });
  return { predictions: data?.predictions ?? [], isLoading, isError: error };
}

export function usePredictionTracking() {
  const { data, error, isLoading } = useSWR<{ horizons: PredictionTracking[] }>('/predictions/tracking', fetcher, {
    refreshInterval: 60000,
  });
  return { tracking: data?.horizons ?? [], isLoading, isError: error };
}

// ---- P1.9.24 data readiness ----

export function useHistoricalReadiness() {
  const { data, error, isLoading } = useSWR<HistoricalReadiness>('/data-quality/historical-readiness', fetcher);
  return { readiness: data, isLoading, isError: error };
}

export function useIntradayFeedHealth() {
  const { data, error, isLoading } = useSWR<IntradayFeedHealth>('/data-quality/intraday-feed', fetcher);
  return { feed: data, isLoading, isError: error };
}

export function useBreakoutMonitor() {
  const { data, error, isLoading } = useSWR<BreakoutMonitor>('/signals/breakouts/monitor', fetcher, { refreshInterval: 120000 });
  return { monitor: data, isLoading, isError: error };
}

export function useResearchRun() {
  const { data, error, isLoading } = useSWR<ResearchRun>('/backtesting/research', fetcher);
  return { run: data, isLoading, isError: error };
}

// ---- Learning loop ----
export function useLearning() {
  const { data, error, isLoading, mutate } = useSWR<LearningSummary>('/learning/summary', fetcher, { refreshInterval: 300000 });
  return { learning: data, isLoading, isError: error, mutate };
}

// ---- Breakouts, news, events, automatic paper trading ----
export function useBreakoutsOverview() {
  const { data, error, isLoading, mutate } = useSWR<BreakoutsOverview>('/breakouts/overview', fetcher, { refreshInterval: 120000 });
  return { overview: data, isLoading, isError: error, mutate };
}

export function useStockBreakouts(symbol: string) {
  const { data, error, isLoading } = useSWR<StockBreakouts>(symbol ? `/breakouts/stock/${symbol}` : null, fetcher, { refreshInterval: 300000 });
  return { data, isLoading, isError: error };
}

export function useStockNews(symbol: string) {
  const { data, error, isLoading } = useSWR<StockNews>(symbol ? `/stock-feed/${symbol}/news` : null, fetcher, { refreshInterval: 300000 });
  return { data, isLoading, isError: error };
}

export function useStockEvents(symbol: string) {
  const { data, error, isLoading } = useSWR<StockEvents>(symbol ? `/stock-feed/${symbol}/events` : null, fetcher, { refreshInterval: 600000 });
  return { data, isLoading, isError: error };
}

export function useAutoPaper() {
  const { data, error, isLoading } = useSWR<{ portfolios: AutoPaperPortfolio[] }>('/auto-paper/summary', fetcher, { refreshInterval: 300000 });
  return { portfolios: data?.portfolios ?? [], isLoading, isError: error };
}

export function useUpcomingBreakouts() {
  const { data, error, isLoading } = useSWR<UpcomingBreakouts>('/breakouts/upcoming', fetcher, { refreshInterval: 300000 });
  return { data, error, isLoading };
}

export function useStockChart(symbol: string | null, range: ChartRange) {
  // 1D is the live session: refresh it every minute; the daily ranges change once a day.
  const { data, error, isLoading, isValidating } = useSWR<StockChart>(symbol ? `/stocks/${encodeURIComponent(symbol)}/chart?range=${range}` : null, fetcher,
    { refreshInterval: range === '1D' ? 60000 : 0, revalidateOnFocus: false, keepPreviousData: true });
  return { data, error, isLoading, isValidating };
}

export function useStockDeals(symbol: string | null) {
  const { data, error, isLoading } = useSWR<StockDeals>(symbol ? `/stock-feed/${encodeURIComponent(symbol)}/deals` : null, fetcher, { refreshInterval: 600000, revalidateOnFocus: false });
  return { data, error, isLoading };
}
