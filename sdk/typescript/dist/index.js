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
class BrokerError extends Error {
    status;
    constructor(status, message) {
        super(message);
        this.status = status;
        this.name = 'BrokerError';
    }
}
export class BrokerClient {
    baseUrl;
    token;
    headers;
    constructor(config) {
        this.baseUrl = config.baseUrl.replace(/\/$/, '');
        this.token = config.token;
        this.headers = config.headers ?? {};
    }
    async request(method, path, body) {
        const headers = {
            'Content-Type': 'application/json',
            ...this.headers,
        };
        if (this.token) {
            headers['Authorization'] = `Bearer ${this.token}`;
        }
        const res = await fetch(`${this.baseUrl}/api/v1${path}`, {
            method,
            headers,
            body: body ? JSON.stringify(body) : undefined,
        });
        if (!res.ok) {
            const text = await res.text();
            throw new BrokerError(res.status, text);
        }
        return res.json();
    }
    // --- Providers ---
    async listProviders() {
        const res = await this.request('GET', '/providers');
        return res.providers;
    }
    // --- Accounts ---
    async listAccounts(provider) {
        const qs = provider ? `?provider=${provider}` : '';
        return this.request('GET', `/accounts${qs}`);
    }
    async getAccount(provider, accountId) {
        return this.request('GET', `/accounts/${provider}/${accountId}`);
    }
    async createAccount(params) {
        return this.request('POST', '/accounts', params);
    }
    // --- Portfolio ---
    async getPortfolio(provider, accountId) {
        return this.request('GET', `/accounts/${provider}/${accountId}/portfolio`);
    }
    // --- Orders ---
    async listOrders(provider, accountId) {
        return this.request('GET', `/accounts/${provider}/${accountId}/orders`);
    }
    async createOrder(provider, accountId, order) {
        return this.request('POST', `/accounts/${provider}/${accountId}/orders`, order);
    }
    async getOrder(provider, accountId, orderId) {
        return this.request('GET', `/accounts/${provider}/${accountId}/orders/${orderId}`);
    }
    async cancelOrder(provider, accountId, orderId) {
        await this.request('DELETE', `/accounts/${provider}/${accountId}/orders/${orderId}`);
    }
    // --- Transfers ---
    async listTransfers(provider, accountId) {
        return this.request('GET', `/accounts/${provider}/${accountId}/transfers`);
    }
    async createTransfer(provider, accountId, transfer) {
        return this.request('POST', `/accounts/${provider}/${accountId}/transfers`, transfer);
    }
    // --- Bank Relationships ---
    async listBankRelationships(provider, accountId) {
        return this.request('GET', `/accounts/${provider}/${accountId}/bank-relationships`);
    }
    async createBankRelationship(provider, accountId, params) {
        return this.request('POST', `/accounts/${provider}/${accountId}/bank-relationships`, params);
    }
    // --- Assets ---
    async listAssets(provider, opts) {
        const params = new URLSearchParams();
        if (opts?.class)
            params.set('class', opts.class);
        if (opts?.all)
            params.set('all', '1');
        const qs = params.toString() ? `?${params}` : '';
        return this.request('GET', `/assets/${provider}${qs}`);
    }
    async getAsset(provider, symbolOrId) {
        return this.request('GET', `/assets/${provider}/${encodeURIComponent(symbolOrId)}`);
    }
    // --- Smart Order Routing ---
    async getRoutes(symbol, side = 'buy') {
        // Handle crypto pairs with / separator
        const path = symbol.includes('/') ? `/route/${symbol}` : `/route/${symbol}`;
        return this.request('GET', `${path}?side=${side}`);
    }
    async listAggregatedAssets() {
        return this.request('GET', '/assets');
    }
    // --- Market Data ---
    async getSnapshot(provider, symbol) {
        return this.request('GET', `/market/${provider}/snapshot/${symbol}`);
    }
    async getSnapshots(provider, symbols) {
        return this.request('GET', `/market/${provider}/snapshots?symbols=${symbols.join(',')}`);
    }
    async getBars(provider, symbol, opts) {
        const params = new URLSearchParams();
        if (opts?.timeframe)
            params.set('timeframe', opts.timeframe);
        if (opts?.start)
            params.set('start', opts.start);
        if (opts?.end)
            params.set('end', opts.end);
        if (opts?.limit)
            params.set('limit', String(opts.limit));
        const qs = params.toString() ? `?${params}` : '';
        return this.request('GET', `/market/${provider}/bars/${symbol}${qs}`);
    }
    async getLatestTrades(provider, symbols) {
        return this.request('GET', `/market/${provider}/trades/latest?symbols=${symbols.join(',')}`);
    }
    async getLatestQuotes(provider, symbols) {
        return this.request('GET', `/market/${provider}/quotes/latest?symbols=${symbols.join(',')}`);
    }
    async getClock(provider) {
        return this.request('GET', `/market/${provider}/clock`);
    }
}
export { BrokerError };
export default BrokerClient;
