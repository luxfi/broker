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
import type { Account, Order, Position, Portfolio, Asset, MarketSnapshot } from '../index.js';
export interface AlpacaCompatConfig {
    brokerUrl: string;
    accountId?: string;
    token?: string;
    provider?: string;
}
export declare class AlpacaCompat {
    private client;
    private provider;
    private accountId;
    constructor(config: AlpacaCompatConfig);
    setAccountId(id: string): void;
    getAccount(): Promise<Account>;
    getPositions(): Promise<Position[]>;
    getPosition(symbol: string): Promise<Position | undefined>;
    getOrders(params?: {
        status?: string;
        limit?: number;
    }): Promise<Order[]>;
    placeOrder(order: {
        symbol: string;
        qty?: number | string;
        notional?: number | string;
        side: 'buy' | 'sell';
        type: 'market' | 'limit' | 'stop' | 'stop_limit';
        time_in_force: 'day' | 'gtc' | 'ioc' | 'fok';
        limit_price?: number | string;
        stop_price?: number | string;
    }): Promise<Order>;
    cancelOrder(orderId: string): Promise<void>;
    getMarketData(symbol: string): Promise<MarketSnapshot>;
    getAssets(params?: {
        status?: string;
        asset_class?: string;
    }): Promise<Asset[]>;
    getPortfolio(): Promise<Portfolio>;
}
