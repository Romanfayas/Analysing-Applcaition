import useSWR from 'swr';
import { fetcher } from './client';
import { StockDetail, IPO, SignalOverview, PortfolioPosition } from './types';

// ==========================================
// Stocks
// ==========================================

export function useStock(symbol: string) {
  const { data, error, isLoading } = useSWR<StockDetail>(
    symbol ? `/stocks/${symbol}` : null,
    fetcher
  );

  return {
    stock: data,
    isLoading,
    isError: error,
  };
}

export function useStocks() {
  const { data, error, isLoading } = useSWR<{ data: any[], meta: any }>('/stocks', fetcher);
  return {
    stocks: data?.data || [],
    meta: data?.meta,
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

export function useIPOs(status: 'upcoming' | 'active' | 'closed' = 'active') {
  const { data, error, isLoading } = useSWR<IPO[]>(`/ipos?status=${status}`, fetcher);
  return {
    ipos: data || [],
    isLoading,
    isError: error,
  };
}

// ==========================================
// Signals
// ==========================================

export function useRecentSignals() {
  const { data, error, isLoading } = useSWR<SignalOverview[]>('/signals', fetcher);
  return {
    signals: data || [],
    isLoading,
    isError: error,
  };
}

// ==========================================
// Portfolio
// ==========================================

export function usePortfolio() {
  const { data, error, isLoading } = useSWR<{ portfolio: any; positions: PortfolioPosition[] }>('/portfolio', fetcher);
  return {
    portfolio: data?.portfolio,
    positions: data?.positions || [],
    isLoading,
    isError: error,
  };
}
