/**
 * Alpaca-compatible client backed by Lux Broker API.
 *
 * Drop-in replacement for direct Alpaca API clients.
 * Routes all calls through the Lux Broker API for unified multi-provider access.
 *
 * Usage:
 *   // Before: const alpaca = new AlpacaClient()
 *   // After:
 *   import { AlpacaCompat } from '@luxfi/broker/compat/alpaca'
 *   const alpaca = new AlpacaCompat({ brokerUrl: 'http://localhost:8090', accountId: '...' })
 *   // Same API surface as before
 *   const positions = await alpaca.getPositions()
 */

import { BrokerClient } from '../index.js'
import type { Account, Order, Position, Portfolio, Asset, MarketSnapshot } from '../index.js'

export interface AlpacaCompatConfig {
  brokerUrl: string
  accountId?: string
  token?: string
  provider?: string // defaults to 'alpaca'
}

export class AlpacaCompat {
  private client: BrokerClient
  private provider: string
  private accountId: string

  constructor(config: AlpacaCompatConfig) {
    this.client = new BrokerClient({
      baseUrl: config.brokerUrl,
      token: config.token,
    })
    this.provider = config.provider ?? 'alpaca'
    this.accountId = config.accountId ?? ''
  }

  setAccountId(id: string) {
    this.accountId = id
  }

  async getAccount(): Promise<Account> {
    return this.client.getAccount(this.provider, this.accountId)
  }

  async getPositions(): Promise<Position[]> {
    const portfolio = await this.client.getPortfolio(this.provider, this.accountId)
    return portfolio.positions
  }

  async getPosition(symbol: string): Promise<Position | undefined> {
    const positions = await this.getPositions()
    return positions.find((p) => p.symbol === symbol)
  }

  async getOrders(params?: { status?: string; limit?: number }): Promise<Order[]> {
    return this.client.listOrders(this.provider, this.accountId)
  }

  async placeOrder(order: {
    symbol: string
    qty?: number | string
    notional?: number | string
    side: 'buy' | 'sell'
    type: 'market' | 'limit' | 'stop' | 'stop_limit'
    time_in_force: 'day' | 'gtc' | 'ioc' | 'fok'
    limit_price?: number | string
    stop_price?: number | string
  }): Promise<Order> {
    return this.client.createOrder(this.provider, this.accountId, {
      symbol: order.symbol,
      qty: order.qty ? String(order.qty) : undefined,
      notional: order.notional ? String(order.notional) : undefined,
      side: order.side,
      type: order.type,
      time_in_force: order.time_in_force,
      limit_price: order.limit_price ? String(order.limit_price) : undefined,
      stop_price: order.stop_price ? String(order.stop_price) : undefined,
    })
  }

  async cancelOrder(orderId: string): Promise<void> {
    return this.client.cancelOrder(this.provider, this.accountId, orderId)
  }

  async getMarketData(symbol: string): Promise<MarketSnapshot> {
    return this.client.getSnapshot(this.provider, symbol)
  }

  async getAssets(params?: { status?: string; asset_class?: string }): Promise<Asset[]> {
    return this.client.listAssets(this.provider, { class: params?.asset_class })
  }

  async getPortfolio(): Promise<Portfolio> {
    return this.client.getPortfolio(this.provider, this.accountId)
  }
}
