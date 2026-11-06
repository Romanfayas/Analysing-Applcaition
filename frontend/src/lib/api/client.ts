import axios from 'axios';
import type { ApiEnvelope } from './types';

// The browser talks only to the authenticated Next.js proxy; the JWT never reaches client code.
export const API_BASE_URL = '/api/proxy';

export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  timeout: 20000,
  headers: { 'Content-Type': 'application/json' },
});

/** Raised for any non-2xx API response. `message` is the backend's error text when it supplied one. */
export class ApiError extends Error {
  constructor(message: string, public readonly status: number) {
    super(message);
    this.name = 'ApiError';
  }
}

function toApiError(error: unknown): ApiError {
  if (axios.isAxiosError(error)) {
    const status = error.response?.status ?? 0;
    const body = error.response?.data as { error?: string } | undefined;
    if (status === 401 && typeof window !== 'undefined' && window.location.pathname !== '/login') {
      // Hard navigation on purpose: it discards every cached SWR entry belonging to the expired session.
      // eslint-disable-next-line @next/next/no-location-assign-relative-destination
      window.location.assign(`/login?next=${encodeURIComponent(window.location.pathname)}`);
    }
    return new ApiError(body?.error ?? error.message, status);
  }
  return new ApiError(error instanceof Error ? error.message : 'Request failed', 0);
}

/** GET an endpoint and return the unwrapped `data` field of the standard `{success, data}` envelope. */
export const fetcher = async <T>(url: string): Promise<T> => {
  try {
    const res = await apiClient.get<ApiEnvelope<T>>(url);
    return res.data.data;
  } catch (e) {
    throw toApiError(e);
  }
};

/** Send a mutating request; resolves with the unwrapped `data`, rejects with ApiError. */
export async function send<T>(method: 'post' | 'put' | 'delete', url: string, body?: unknown): Promise<T> {
  try {
    const res = await apiClient.request<ApiEnvelope<T>>({ method, url, data: body });
    return res.data.data;
  } catch (e) {
    throw toApiError(e);
  }
}
