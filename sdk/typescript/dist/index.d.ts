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
    baseUrl: string;
    token?: string;
    headers?: Record<string, string>;
}
export interface Account {
    id: string;
    provider: string;
    provider_id: string;
    org_id: string;
    user_id?: string;
    account_number: string;
    status: string;
    currency: string;
    account_type?: string;
    enabled_assets?: string[];
    identity?: Identity;
    contact?: Contact;
    created_at: string;
    updated_at: string;
}
export interface Identity {
    given_name: string;
    family_name: string;
    date_of_birth: string;
    tax_id?: string;
    tax_id_type?: string;
    country_of_tax_residence?: string;
}
export interface Contact {
    email: string;
    phone?: string;
    street?: string[];
    city?: string;
    state?: string;
    postal_code?: string;
    country?: string;
}
export interface Portfolio {
    account_id: string;
    cash: string;
    equity: string;
    buying_power: string;
    portfolio_value: string;
    positions: Position[];
}
export interface Position {
    symbol: string;
    qty: string;
    avg_entry_price: string;
    market_value: string;
    current_price: string;
    unrealized_pl: string;
    side: string;
    asset_class: string;
}
export interface Order {
    id: string;
    provider: string;
    provider_id: string;
    account_id: string;
    symbol: string;
    qty?: string;
    notional?: string;
    side: string;
    type: string;
    time_in_force: string;
    limit_price?: string;
    stop_price?: string;
    status: string;
    filled_qty?: string;
    filled_avg_price?: string;
    asset_class?: string;
    created_at: string;
    filled_at?: string;
}
export interface CreateOrderRequest {
    symbol: string;
    qty?: string;
    notional?: string;
    side: 'buy' | 'sell';
    type: 'market' | 'limit' | 'stop' | 'stop_limit';
    time_in_force: 'day' | 'gtc' | 'ioc' | 'fok';
    limit_price?: string;
    stop_price?: string;
}
export interface Asset {
    id: string;
    provider: string;
    symbol: string;
    name: string;
    class: string;
    exchange?: string;
    status: string;
    tradable: boolean;
    fractionable: boolean;
}
export interface AggregatedAsset {
    symbol: string;
    name: string;
    class: string;
    providers: string[];
}
export interface RouteResult {
    provider: string;
    symbol: string;
    bid_price?: number;
    ask_price?: number;
    spread?: number;
    score: number;
}
export interface Transfer {
    id: string;
    provider: string;
    provider_id: string;
    account_id: string;
    type: string;
    direction: string;
    amount: string;
    currency: string;
    status: string;
    created_at: string;
}
export interface CreateTransferRequest {
    type: string;
    direction: string;
    amount: string;
    relationship_id?: string;
}
export interface BankRelationship {
    id: string;
    provider: string;
    provider_id: string;
    account_id: string;
    bank_name?: string;
    account_owner_name: string;
    bank_account_type: string;
    status: string;
}
export interface MarketSnapshot {
    symbol: string;
    latest_trade?: Trade;
    latest_quote?: Quote;
    minute_bar?: Bar;
    daily_bar?: Bar;
    prev_daily_bar?: Bar;
}
export interface Trade {
    timestamp: string;
    price: number;
    size: number;
    exchange?: string;
}
export interface Quote {
    timestamp: string;
    bid_price: number;
    bid_size: number;
    ask_price: number;
    ask_size: number;
}
export interface Bar {
    timestamp: string;
    open: number;
    high: number;
    low: number;
    close: number;
    volume: number;
    vwap?: number;
    trade_count?: number;
}
export interface MarketClock {
    timestamp: string;
    is_open: boolean;
    next_open: string;
    next_close: string;
}
declare class BrokerError extends Error {
    status: number;
    constructor(status: number, message: string);
}
export declare class BrokerClient {
    private baseUrl;
    private token?;
    private headers;
    constructor(config: BrokerClientConfig);
    private request;
    listProviders(): Promise<string[]>;
    listAccounts(provider?: string): Promise<Account[]>;
    getAccount(provider: string, accountId: string): Promise<Account>;
    createAccount(params: {
        provider?: string;
        given_name: string;
        family_name: string;
        email: string;
        date_of_birth?: string;
        tax_id?: string;
        phone?: string;
        street?: string[];
        city?: string;
        state?: string;
        postal_code?: string;
        country?: string;
    }): Promise<Account>;
    getPortfolio(provider: string, accountId: string): Promise<Portfolio>;
    listOrders(provider: string, accountId: string): Promise<Order[]>;
    createOrder(provider: string, accountId: string, order: CreateOrderRequest): Promise<Order>;
    getOrder(provider: string, accountId: string, orderId: string): Promise<Order>;
    cancelOrder(provider: string, accountId: string, orderId: string): Promise<void>;
    listTransfers(provider: string, accountId: string): Promise<Transfer[]>;
    createTransfer(provider: string, accountId: string, transfer: CreateTransferRequest): Promise<Transfer>;
    listBankRelationships(provider: string, accountId: string): Promise<BankRelationship[]>;
    createBankRelationship(provider: string, accountId: string, params: {
        account_owner_name: string;
        bank_account_type: string;
        bank_account_number: string;
        bank_routing_number: string;
    }): Promise<BankRelationship>;
    listAssets(provider: string, opts?: {
        class?: string;
        all?: boolean;
    }): Promise<Asset[]>;
    getAsset(provider: string, symbolOrId: string): Promise<Asset>;
    getRoutes(symbol: string, side?: 'buy' | 'sell'): Promise<RouteResult[]>;
    listAggregatedAssets(): Promise<AggregatedAsset[]>;
    getSnapshot(provider: string, symbol: string): Promise<MarketSnapshot>;
    getSnapshots(provider: string, symbols: string[]): Promise<Record<string, MarketSnapshot>>;
    getBars(provider: string, symbol: string, opts?: {
        timeframe?: string;
        start?: string;
        end?: string;
        limit?: number;
    }): Promise<Bar[]>;
    getLatestTrades(provider: string, symbols: string[]): Promise<Record<string, Trade>>;
    getLatestQuotes(provider: string, symbols: string[]): Promise<Record<string, Quote>>;
    getClock(provider: string): Promise<MarketClock>;
}
export { BrokerError };
export default BrokerClient;
