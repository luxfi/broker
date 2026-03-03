/**
 * Lux Broker API Client
 *
 * Unified client for the Lux federated broker API.
 * Connects to brokerd which routes across Alpaca, IBKR, BitGo, FalconX, Finix.
 *
 * Usage:
 *   const broker = new BrokerClient({ baseUrl: 'https://broker.lux.financial' })
 *   const accounts = await broker.listAccounts()
 *   const routes = await broker.getRoutes('AAPL', 'buy')
 *   const order = await broker.createOrder('alpaca', accountId, { symbol: 'AAPL', qty: '1', side: 'buy', type: 'market', time_in_force: 'day' })
 */

export interface BrokerClientConfig {
  baseUrl: string
  token?: string
  headers?: Record<string, string>
}

export interface Account {
  id: string
  provider: string
  provider_id: string
  org_id: string
  user_id?: string
  account_number: string
  status: string
  currency: string
  account_type?: string
  enabled_assets?: string[]
  identity?: Identity
  contact?: Contact
  created_at: string
  updated_at: string
}

export interface Identity {
  given_name: string
  family_name: string
  date_of_birth: string
  tax_id?: string
  tax_id_type?: string
  country_of_tax_residence?: string
}

export interface Contact {
  email: string
  phone?: string
  street?: string[]
  city?: string
  state?: string
  postal_code?: string
  country?: string
}

export interface Portfolio {
  account_id: string
  cash: string
  equity: string
  buying_power: string
  portfolio_value: string
  positions: Position[]
}

export interface Position {
  symbol: string
  qty: string
  avg_entry_price: string
  market_value: string
  current_price: string
  unrealized_pl: string
  side: string
  asset_class: string
}

export interface Order {
  id: string
  provider: string
  provider_id: string
  account_id: string
  symbol: string
  qty?: string
  notional?: string
  side: string
  type: string
  time_in_force: string
  limit_price?: string
  stop_price?: string
  status: string
  filled_qty?: string
  filled_avg_price?: string
  asset_class?: string
  created_at: string
  filled_at?: string
}

export interface CreateOrderRequest {
  symbol: string
  qty?: string
  notional?: string
  side: 'buy' | 'sell'
  type: 'market' | 'limit' | 'stop' | 'stop_limit'
  time_in_force: 'day' | 'gtc' | 'ioc' | 'fok'
  limit_price?: string
  stop_price?: string
}

export interface Asset {
  id: string
  provider: string
  symbol: string
  name: string
  class: string
  exchange?: string
  status: string
  tradable: boolean
  fractionable: boolean
}

export interface AggregatedAsset {
  symbol: string
  name: string
  class: string
  providers: string[]
}

export interface RouteResult {
  provider: string
  symbol: string
  bid_price?: number
  ask_price?: number
  spread?: number
  score: number
}

export interface Transfer {
  id: string
  provider: string
  provider_id: string
  account_id: string
  type: string
  direction: string
  amount: string
  currency: string
  status: string
  created_at: string
}

export interface CreateTransferRequest {
  type: string
  direction: string
  amount: string
  relationship_id?: string
}

export interface BankRelationship {
  id: string
  provider: string
  provider_id: string
  account_id: string
  bank_name?: string
  account_owner_name: string
  bank_account_type: string
  status: string
}

export interface MarketSnapshot {
  symbol: string
  latest_trade?: Trade
  latest_quote?: Quote
  minute_bar?: Bar
  daily_bar?: Bar
  prev_daily_bar?: Bar
}

export interface Trade {
  timestamp: string
  price: number
  size: number
  exchange?: string
}

export interface Quote {
  timestamp: string
  bid_price: number
  bid_size: number
  ask_price: number
  ask_size: number
}

export interface Bar {
  timestamp: string
  open: number
  high: number
  low: number
  close: number
  volume: number
  vwap?: number
  trade_count?: number
}

export interface MarketClock {
  timestamp: string
  is_open: boolean
  next_open: string
  next_close: string
}

class BrokerError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
    this.name = 'BrokerError'
  }
}

export class BrokerClient {
  private baseUrl: string
  private token?: string
  private headers: Record<string, string>

  constructor(config: BrokerClientConfig) {
    this.baseUrl = config.baseUrl.replace(/\/$/, '')
    this.token = config.token
    this.headers = config.headers ?? {}
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...this.headers,
    }
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`
    }

    const res = await fetch(`${this.baseUrl}/api/v1${path}`, {
      method,
      headers,
      body: body ? JSON.stringify(body) : undefined,
    })

    if (!res.ok) {
      const text = await res.text()
      throw new BrokerError(res.status, text)
    }

    return res.json() as Promise<T>
  }

  // --- Providers ---

  async listProviders(): Promise<string[]> {
    const res = await this.request<{ providers: string[] }>('GET', '/providers')
    return res.providers
  }

  // --- Accounts ---

  async listAccounts(provider?: string): Promise<Account[]> {
    const qs = provider ? `?provider=${provider}` : ''
    return this.request('GET', `/accounts${qs}`)
  }

  async getAccount(provider: string, accountId: string): Promise<Account> {
    return this.request('GET', `/accounts/${provider}/${accountId}`)
  }

  async createAccount(params: {
    provider?: string
    given_name: string
    family_name: string
    email: string
    date_of_birth?: string
    tax_id?: string
    phone?: string
    street?: string[]
    city?: string
    state?: string
    postal_code?: string
    country?: string
  }): Promise<Account> {
    return this.request('POST', '/accounts', params)
  }

  // --- Portfolio ---

  async getPortfolio(provider: string, accountId: string): Promise<Portfolio> {
    return this.request('GET', `/accounts/${provider}/${accountId}/portfolio`)
  }

  // --- Orders ---

  async listOrders(provider: string, accountId: string): Promise<Order[]> {
    return this.request('GET', `/accounts/${provider}/${accountId}/orders`)
  }

  async createOrder(provider: string, accountId: string, order: CreateOrderRequest): Promise<Order> {
    return this.request('POST', `/accounts/${provider}/${accountId}/orders`, order)
  }

  async getOrder(provider: string, accountId: string, orderId: string): Promise<Order> {
    return this.request('GET', `/accounts/${provider}/${accountId}/orders/${orderId}`)
  }

  async cancelOrder(provider: string, accountId: string, orderId: string): Promise<void> {
    await this.request('DELETE', `/accounts/${provider}/${accountId}/orders/${orderId}`)
  }

  // --- Transfers ---

  async listTransfers(provider: string, accountId: string): Promise<Transfer[]> {
    return this.request('GET', `/accounts/${provider}/${accountId}/transfers`)
  }

  async createTransfer(provider: string, accountId: string, transfer: CreateTransferRequest): Promise<Transfer> {
    return this.request('POST', `/accounts/${provider}/${accountId}/transfers`, transfer)
  }

  // --- Bank Relationships ---

  async listBankRelationships(provider: string, accountId: string): Promise<BankRelationship[]> {
    return this.request('GET', `/accounts/${provider}/${accountId}/bank-relationships`)
  }

  async createBankRelationship(
    provider: string,
    accountId: string,
    params: {
      account_owner_name: string
      bank_account_type: string
      bank_account_number: string
      bank_routing_number: string
    },
  ): Promise<BankRelationship> {
    return this.request('POST', `/accounts/${provider}/${accountId}/bank-relationships`, params)
  }

  // --- Assets ---

  async listAssets(provider: string, opts?: { class?: string; all?: boolean }): Promise<Asset[]> {
    const params = new URLSearchParams()
    if (opts?.class) params.set('class', opts.class)
    if (opts?.all) params.set('all', '1')
    const qs = params.toString() ? `?${params}` : ''
    return this.request('GET', `/assets/${provider}${qs}`)
  }

  async getAsset(provider: string, symbolOrId: string): Promise<Asset> {
    return this.request('GET', `/assets/${provider}/${encodeURIComponent(symbolOrId)}`)
  }

  // --- Smart Order Routing ---

  async getRoutes(symbol: string, side: 'buy' | 'sell' = 'buy'): Promise<RouteResult[]> {
    // Handle crypto pairs with / separator
    const path = symbol.includes('/') ? `/route/${symbol}` : `/route/${symbol}`
    return this.request('GET', `${path}?side=${side}`)
  }

  async listAggregatedAssets(): Promise<AggregatedAsset[]> {
    return this.request('GET', '/assets')
  }

  // --- Market Data ---

  async getSnapshot(provider: string, symbol: string): Promise<MarketSnapshot> {
    return this.request('GET', `/market/${provider}/snapshot/${symbol}`)
  }

  async getSnapshots(provider: string, symbols: string[]): Promise<Record<string, MarketSnapshot>> {
    return this.request('GET', `/market/${provider}/snapshots?symbols=${symbols.join(',')}`)
  }

  async getBars(
    provider: string,
    symbol: string,
    opts?: { timeframe?: string; start?: string; end?: string; limit?: number },
  ): Promise<Bar[]> {
    const params = new URLSearchParams()
    if (opts?.timeframe) params.set('timeframe', opts.timeframe)
    if (opts?.start) params.set('start', opts.start)
    if (opts?.end) params.set('end', opts.end)
    if (opts?.limit) params.set('limit', String(opts.limit))
    const qs = params.toString() ? `?${params}` : ''
    return this.request('GET', `/market/${provider}/bars/${symbol}${qs}`)
  }

  async getLatestTrades(provider: string, symbols: string[]): Promise<Record<string, Trade>> {
    return this.request('GET', `/market/${provider}/trades/latest?symbols=${symbols.join(',')}`)
  }

  async getLatestQuotes(provider: string, symbols: string[]): Promise<Record<string, Quote>> {
    return this.request('GET', `/market/${provider}/quotes/latest?symbols=${symbols.join(',')}`)
  }

  async getClock(provider: string): Promise<MarketClock> {
    return this.request('GET', `/market/${provider}/clock`)
  }
}

export { BrokerError }
export default BrokerClient
