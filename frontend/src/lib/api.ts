// API client for communicating with the Go backend
const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api/v1";

class ApiClient {
  private token: string | null = null;

  setToken(token: string) {
    this.token = token;
    if (typeof window !== "undefined") {
      localStorage.setItem("auth_token", token);
    }
  }

  getToken(): string | null {
    if (this.token) return this.token;
    if (typeof window !== "undefined") {
      this.token = localStorage.getItem("auth_token");
    }
    return this.token;
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      ...((options.headers as Record<string, string>) || {}),
    };

    const token = this.getToken();
    if (token) {
      headers["Authorization"] = `Bearer ${token}`;
    }

    const response = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers,
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: "Request failed" }));
      throw new Error(error.error || `HTTP ${response.status}`);
    }

    return response.json();
  }

  // Auth
  async login(email: string, password: string) {
    return this.request<{ data: { token: string } }>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
  }

  async register(email: string, password: string, name: string) {
    return this.request("/auth/register", {
      method: "POST",
      body: JSON.stringify({ email, password, name }),
    });
  }

  // Stocks
  async getStocks(params?: { page?: number; sector?: string }) {
    const query = new URLSearchParams(params as Record<string, string>);
    return this.request(`/stocks?${query}`);
  }

  async getStock(symbol: string) {
    return this.request(`/stocks/${symbol}`);
  }

  async getStockCandles(symbol: string, from?: string, to?: string) {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    return this.request(`/stocks/${symbol}/candles?${params}`);
  }

  async getStockFundamentals(symbol: string) {
    return this.request(`/stocks/${symbol}/fundamentals`);
  }

  async getStockTechnicals(symbol: string) {
    return this.request(`/stocks/${symbol}/technicals`);
  }

  async getStockShariah(symbol: string) {
    return this.request(`/stocks/${symbol}/shariah`);
  }

  async getStockSignal(symbol: string) {
    return this.request(`/stocks/${symbol}/signal`);
  }

  async getStockRisk(symbol: string) {
    return this.request(`/stocks/${symbol}/risk`);
  }

  async getStockAnalytics(symbol: string) {
    return this.request(`/stocks/${symbol}/analytics`);
  }

  // IPOs
  async getIPOs(status?: string) {
    const params = status ? `?status=${status}` : "";
    return this.request(`/ipos${params}`);
  }

  async getIPO(id: number) {
    return this.request(`/ipos/${id}`);
  }

  async getIPOPrediction(id: number) {
    return this.request(`/ipos/${id}/prediction`);
  }

  async getIPODecision(id: number) {
    return this.request(`/ipos/${id}/decision`);
  }

  // Signals
  async getSignals() {
    return this.request("/signals");
  }

  async getSignalWeights() {
    return this.request("/signals/weights");
  }

  // Shariah
  async getShariahScreenings() {
    return this.request("/shariah/screenings");
  }

  async getShariahRuleSets() {
    return this.request("/shariah/rule-sets");
  }

  // Backtesting
  async runBacktest(config: Record<string, unknown>) {
    return this.request("/backtesting/run", {
      method: "POST",
      body: JSON.stringify(config),
    });
  }

  // Risk
  async calculatePositionSize(data: {
    capital: number;
    risk_percentage: number;
    entry_price: number;
    stop_loss: number;
    target_price?: number;
  }) {
    return this.request("/risk/calculate", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }

  // Alerts
  async getAlerts() {
    return this.request("/alerts");
  }

  async createAlert(data: Record<string, unknown>) {
    return this.request("/alerts", {
      method: "POST",
      body: JSON.stringify(data),
    });
  }
}

export const api = new ApiClient();
