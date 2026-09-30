import axios from 'axios';

// The base URL routes through the Next.js API proxy to keep secrets on the server
export const API_BASE_URL = '/api/proxy';

// Create a centralized Axios instance
export const apiClient = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Interceptor for attaching dynamic data (like CSRF tokens) if needed in the future
apiClient.interceptors.request.use(
  (config) => {
    return config;
  },
  (error) => Promise.reject(error)
);

apiClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    // Handle global API errors (e.g., logging, redirect to login on 401)
    return Promise.reject(error?.response?.data || error);
  }
);

// Standard fetcher for SWR
export const fetcher = async <T>(url: string): Promise<T> => {
  const response = await apiClient.get<any, any>(url);
  return response as T;
};
