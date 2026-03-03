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
import { BrokerClient } from '../index.js';
export class AlpacaCompat {
    client;
    provider;
    accountId;
    constructor(config) {
        this.client = new BrokerClient({
            baseUrl: config.brokerUrl,
            token: config.token,
        });
        this.provider = config.provider ?? 'alpaca';
        this.accountId = config.accountId ?? '';
    }
    setAccountId(id) {
        this.accountId = id;
    }
    async getAccount() {
        return this.client.getAccount(this.provider, this.accountId);
    }
    async getPositions() {
        const portfolio = await this.client.getPortfolio(this.provider, this.accountId);
        return portfolio.positions;
    }
    async getPosition(symbol) {
        const positions = await this.getPositions();
        return positions.find((p) => p.symbol === symbol);
    }
    async getOrders(params) {
        return this.client.listOrders(this.provider, this.accountId);
    }
    async placeOrder(order) {
        return this.client.createOrder(this.provider, this.accountId, {
            symbol: order.symbol,
            qty: order.qty ? String(order.qty) : undefined,
            notional: order.notional ? String(order.notional) : undefined,
            side: order.side,
            type: order.type,
            time_in_force: order.time_in_force,
            limit_price: order.limit_price ? String(order.limit_price) : undefined,
            stop_price: order.stop_price ? String(order.stop_price) : undefined,
        });
    }
    async cancelOrder(orderId) {
        return this.client.cancelOrder(this.provider, this.accountId, orderId);
    }
    async getMarketData(symbol) {
        return this.client.getSnapshot(this.provider, symbol);
    }
    async getAssets(params) {
        return this.client.listAssets(this.provider, { class: params?.asset_class });
    }
    async getPortfolio() {
        return this.client.getPortfolio(this.provider, this.accountId);
    }
}
