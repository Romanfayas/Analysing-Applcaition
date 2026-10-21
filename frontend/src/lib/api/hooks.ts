import useSWR from 'swr';
import { fetcher } from './client';
import { StockDetail, StockListItem, MarketHealthResponse, IPO, SignalOverview, PortfolioPosition } from './types';

// ==========================================
// Stocks
// ==========================================

export function useStock(symbol: string) {
  const { data, error, isLoading } = useSWR<any>(
    symbol ? `/stocks/${symbol}` : null,
    fetcher
  );

  const stock: StockDetail | undefined = data?.data || data;

  return {
    stock,
    isLoading,
    isError: error,
  };
}

export function useStocks() {
  const { data, error, isLoading, mutate } = useSWR<{ data: StockListItem[], meta: { page: number; per_page: number; total: number } }>('/stocks', fetcher);
  return {
    stocks: data?.data || [],
    meta: data?.meta,
    isLoading,
    isError: error,
    mutate,
  };
}

export function useMarketDataHealth() {
  const { data, error, isLoading } = useSWR<any>('/market-data/health', fetcher);
  const health: MarketHealthResponse | undefined = data?.data || data;
  return {
    health,
    isLoading,
    isError: error,
  };
}

export function useMarketOverview() {
  const { data, error, isLoading } = useSWR<{ indices: any[] }>('/stocks/market/overview', fetcher);
  return { data, isLoading, isError: error };
}

// ==========================================
// IPOs
// ==========================================

export function useIPOs(status: string = 'all') {
  const { data, error, isLoading, mutate } = useSWR<any>(`/ipos?status=${status}`, fetcher);
  const ipos: any[] = Array.isArray(data) ? data : (data?.data && Array.isArray(data.data) ? data.data : []);
  return {
    ipos,
    isLoading,
    isError: error,
    mutate,
  };
}

export function useIPOHealth() {
  const { data, error, isLoading, mutate } = useSWR<any>('/ipos/health', fetcher);
  return {
    health: data?.data || data,
    isLoading,
    isError: error,
    mutate,
  };
}

export function useIPOIngestionRuns() {
  const { data, error, isLoading, mutate } = useSWR<any>('/ipos/ingestion-runs', fetcher);
  const runs: any[] = Array.isArray(data) ? data : (data?.data && Array.isArray(data.data) ? data.data : []);
  return {
    runs,
    isLoading,
    isError: error,
    mutate,
  };
}

// ==========================================
// Signals & Breakout Research
// ==========================================

export function useRecentSignals() {
  const { data, error, isLoading } = useSWR<any>('/signals', fetcher);
  const signals: SignalOverview[] = Array.isArray(data) ? data : (data?.data && Array.isArray(data.data) ? data.data : []);
  return {
    signals,
    isLoading,
    isError: error,
  };
}

export function useBreakouts() {
  const { data, error, isLoading, mutate } = useSWR<any>('/signals/breakouts', fetcher, {
    revalidateOnFocus: false,
    dedupingInterval: 10000,
  });
  const res = data?.data || data;
  return {
    scan: res,
    candidates: res?.candidates || [],
    stageCounts: res?.stage_counts || {
      CONFIRMED_BREAKOUT: 0,
      BREAKOUT_CANDIDATE: 0,
      WATCHLIST: 0,
      FAILED_BREAKOUT: 0,
      UNAVAILABLE: 0,
    },
    bullishBreadth: res?.bullish_breadth_percent ?? 0,
    sectorHeatmap: res?.sector_heatmap || [],
    totalScreened: res?.total_screened ?? 0,
    isLoading,
    isError: error,
    mutate,
  };
}

// ==========================================
// Shariah Screenings
// ==========================================

export function useShariahScreenings() {
  const { data, error, isLoading } = useSWR<any>('/shariah/screenings', fetcher);
  const screenings: any[] = Array.isArray(data) ? data : (data?.data && Array.isArray(data.data) ? data.data : []);
  return {
    screenings,
    isLoading,
    isError: error,
  };
}

// ==========================================
// Portfolio & Paper Ledger
// ==========================================

export function usePortfolio() {
  const { data, error, isLoading } = useSWR<any>('/portfolio', fetcher);
  const payload = data?.data || data;
  return {
    portfolio: payload?.portfolio || payload,
    positions: payload?.positions || [],
    isLoading,
    isError: error,
  };
}

