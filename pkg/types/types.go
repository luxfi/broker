package types

import "time"

// Account is the unified brokerage account across all providers.
type Account struct {
	ID            string            `json:"id"`
	Provider      string            `json:"provider"`      // alpaca, ibkr, coinbase, etc.
	ProviderID    string            `json:"provider_id"`   // provider's account ID
	OrgID         string            `json:"org_id"`        // lux org tenant
	UserID        string            `json:"user_id,omitempty"`
	AccountNumber string            `json:"account_number"`
	Status        string            `json:"status"`
	Currency      string            `json:"currency"`
	AccountType   string            `json:"account_type,omitempty"`
	EnabledAssets []string          `json:"enabled_assets,omitempty"`
	Identity      *Identity         `json:"identity,omitempty"`
	Contact       *Contact          `json:"contact,omitempty"`
	Meta          map[string]string `json:"meta,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

type Identity struct {
	GivenName    string `json:"given_name"`
	FamilyName   string `json:"family_name"`
	DateOfBirth  string `json:"date_of_birth"`
	TaxID        string `json:"tax_id,omitempty"`
	TaxIDType    string `json:"tax_id_type,omitempty"`
	CountryOfTax string `json:"country_of_tax_residence,omitempty"`
}

type Contact struct {
	Email       string   `json:"email"`
	Phone       string   `json:"phone,omitempty"`
	Street      []string `json:"street,omitempty"`
	City        string   `json:"city,omitempty"`
	State       string   `json:"state,omitempty"`
	PostalCode  string   `json:"postal_code,omitempty"`
	Country     string   `json:"country,omitempty"`
}

// Portfolio is a snapshot of an account's holdings.
type Portfolio struct {
	AccountID     string     `json:"account_id"`
	Cash          string     `json:"cash"`
	Equity        string     `json:"equity"`
	BuyingPower   string     `json:"buying_power"`
	PortfolioValue string   `json:"portfolio_value"`
	Positions     []Position `json:"positions"`
}

// Position is a single holding.
type Position struct {
	Symbol        string `json:"symbol"`
	Qty           string `json:"qty"`
	AvgEntryPrice string `json:"avg_entry_price"`
	MarketValue   string `json:"market_value"`
	CurrentPrice  string `json:"current_price"`
	UnrealizedPL  string `json:"unrealized_pl"`
	Side          string `json:"side"`
	AssetClass    string `json:"asset_class"`
}

// Order is a unified trade order.
type Order struct {
	ID            string     `json:"id"`
	Provider      string     `json:"provider"`
	ProviderID    string     `json:"provider_id"`
	AccountID     string     `json:"account_id"`
	Symbol        string     `json:"symbol"`
	Qty           string     `json:"qty,omitempty"`
	Notional      string     `json:"notional,omitempty"`
	Side          string     `json:"side"`    // buy, sell
	Type          string     `json:"type"`    // market, limit, stop, stop_limit
	TimeInForce   string     `json:"time_in_force"`
	LimitPrice    string     `json:"limit_price,omitempty"`
	StopPrice     string     `json:"stop_price,omitempty"`
	Status        string     `json:"status"`
	FilledQty     string     `json:"filled_qty,omitempty"`
	FilledAvgPrice string   `json:"filled_avg_price,omitempty"`
	AssetClass    string     `json:"asset_class,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	FilledAt      *time.Time `json:"filled_at,omitempty"`
}

// CreateOrderRequest is the unified order creation request.
type CreateOrderRequest struct {
	AccountID     string      `json:"account_id"`
	Symbol        string      `json:"symbol"`
	Qty           string      `json:"qty,omitempty"`
	Notional      string      `json:"notional,omitempty"`
	Side          string      `json:"side"`
	Type          string      `json:"type"`
	TimeInForce   string      `json:"time_in_force"`
	LimitPrice    string      `json:"limit_price,omitempty"`
	StopPrice     string      `json:"stop_price,omitempty"`
	ClientOrderID string      `json:"client_order_id,omitempty"`
	TrailPrice    string      `json:"trail_price,omitempty"`
	TrailPercent  string      `json:"trail_percent,omitempty"`
	ExtendedHours bool        `json:"extended_hours,omitempty"`
	OrderClass    string      `json:"order_class,omitempty"` // simple, bracket, oco, oto
	TakeProfit    *TakeProfit `json:"take_profit,omitempty"`
	StopLoss      *StopLoss   `json:"stop_loss,omitempty"`
}

// TakeProfit for bracket orders.
type TakeProfit struct {
	LimitPrice string `json:"limit_price"`
}

// StopLoss for bracket orders.
type StopLoss struct {
	StopPrice  string `json:"stop_price"`
	LimitPrice string `json:"limit_price,omitempty"`
}

// Asset is a tradable instrument.
type Asset struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	Class        string `json:"class"` // us_equity, crypto
	Exchange     string `json:"exchange,omitempty"`
	Status       string `json:"status"`
	Tradable     bool   `json:"tradable"`
	Fractionable bool   `json:"fractionable"`
}

// Transfer is a fund movement (ACH, wire, crypto).
type Transfer struct {
	ID         string     `json:"id"`
	Provider   string     `json:"provider"`
	ProviderID string     `json:"provider_id"`
	AccountID  string     `json:"account_id"`
	Type       string     `json:"type"`      // ach, wire, crypto
	Direction  string     `json:"direction"` // incoming, outgoing
	Amount     string     `json:"amount"`
	Currency   string     `json:"currency"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// CreateTransferRequest for funding/withdrawals.
type CreateTransferRequest struct {
	AccountID      string `json:"account_id"`
	Type           string `json:"type"`
	Direction      string `json:"direction"`
	Amount         string `json:"amount"`
	RelationshipID string `json:"relationship_id,omitempty"`
}

// BankRelationship links an external bank account.
type BankRelationship struct {
	ID                string `json:"id"`
	Provider          string `json:"provider"`
	ProviderID        string `json:"provider_id"`
	AccountID         string `json:"account_id"`
	BankName          string `json:"bank_name,omitempty"`
	AccountOwnerName  string `json:"account_owner_name"`
	BankAccountType   string `json:"bank_account_type"`
	Status            string `json:"status"`
}

// MarketSnapshot is a real-time price snapshot for a symbol.
type MarketSnapshot struct {
	Symbol       string `json:"symbol"`
	LatestTrade  *Trade `json:"latest_trade,omitempty"`
	LatestQuote  *Quote `json:"latest_quote,omitempty"`
	MinuteBar    *Bar   `json:"minute_bar,omitempty"`
	DailyBar     *Bar   `json:"daily_bar,omitempty"`
	PrevDailyBar *Bar   `json:"prev_daily_bar,omitempty"`
}

// Trade is a single executed trade.
type Trade struct {
	Timestamp string  `json:"timestamp"`
	Price     float64 `json:"price"`
	Size      float64 `json:"size"`
	Exchange  string  `json:"exchange,omitempty"`
}

// Quote is a bid/ask quote.
type Quote struct {
	Timestamp string  `json:"timestamp"`
	BidPrice  float64 `json:"bid_price"`
	BidSize   float64 `json:"bid_size"`
	AskPrice  float64 `json:"ask_price"`
	AskSize   float64 `json:"ask_size"`
}

// Bar is an OHLCV candle.
type Bar struct {
	Timestamp string  `json:"timestamp"`
	TimeMs    int64   `json:"time_ms"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
	Volume    float64 `json:"volume"`
	VWAP      float64 `json:"vwap,omitempty"`
	TradeCount int    `json:"trade_count,omitempty"`
}

// MarketClock represents the trading clock.
type MarketClock struct {
	Timestamp string `json:"timestamp"`
	IsOpen    bool   `json:"is_open"`
	NextOpen  string `json:"next_open"`
	NextClose string `json:"next_close"`
}

// MarketCalendarDay is a single trading day's schedule.
type MarketCalendarDay struct {
	Date  string `json:"date"`
	Open  string `json:"open"`
	Close string `json:"close"`
}

// CreateAccountRequest for onboarding.
type CreateAccountRequest struct {
	Provider      string             `json:"provider"` // which broker to use
	OrgID         string             `json:"-"`        // set from auth context
	UserID        string             `json:"-"`
	Identity      *Identity          `json:"identity"`
	Contact       *Contact           `json:"contact"`
	EnabledAssets []string           `json:"enabled_assets,omitempty"`
	IPAddress     string             `json:"ip_address,omitempty"`
	FundingSources []string          `json:"funding_source,omitempty"`
	Disclosures   *AccountDisclosures `json:"disclosures,omitempty"`
}

// AccountDisclosures contains regulatory disclosure fields for account creation.
type AccountDisclosures struct {
	IsControlPerson            *bool `json:"is_control_person,omitempty"`
	IsAffiliatedExchangeFinra  *bool `json:"is_affiliated_exchange_or_finra,omitempty"`
	IsPoliticallyExposed       *bool `json:"is_politically_exposed,omitempty"`
	ImmediateFamilyExposed     *bool `json:"immediate_family_exposed,omitempty"`
}

// ReplaceOrderRequest for modifying an existing order.
type ReplaceOrderRequest struct {
	Qty           *float64 `json:"qty,omitempty"`
	TimeInForce   string   `json:"time_in_force,omitempty"`
	LimitPrice    *float64 `json:"limit_price,omitempty"`
	StopPrice     *float64 `json:"stop_price,omitempty"`
	TrailPrice    *float64 `json:"trail_price,omitempty"`
	TrailPercent  *float64 `json:"trail_percent,omitempty"`
	ClientOrderID string   `json:"client_order_id,omitempty"`
}

// ListOrdersParams controls order listing pagination and filtering.
type ListOrdersParams struct {
	Status    string `json:"status,omitempty"`    // open, closed, all
	Limit     int    `json:"limit,omitempty"`
	After     string `json:"after,omitempty"`     // cursor for pagination
	Until     string `json:"until,omitempty"`     // filter by date
	Direction string `json:"direction,omitempty"` // asc, desc
	Nested    bool   `json:"nested,omitempty"`    // include nested multi-leg orders
}

// --- Smart Order Routing Types ---

// SmartOrderRequest is a cross-provider order with algorithm selection.
type SmartOrderRequest struct {
	Symbol      string            `json:"symbol"`
	Qty         string            `json:"qty,omitempty"`
	Notional    string            `json:"notional,omitempty"`
	Side        string            `json:"side"`        // buy, sell
	Algorithm   string            `json:"algorithm"`   // smart_route, twap, sniper, limit, market
	LimitPrice  string            `json:"limit_price,omitempty"`
	TimeInForce string            `json:"time_in_force,omitempty"`
	Duration    string            `json:"duration,omitempty"`    // for TWAP: execution window
	Accounts    map[string]string `json:"accounts"`              // provider -> accountID
	MaxSlippage float64           `json:"max_slippage_bps,omitempty"` // max allowed slippage in bps
}

// SplitPlan describes how an order will be split across providers.
type SplitPlan struct {
	Symbol       string     `json:"symbol"`
	Side         string     `json:"side"`
	TotalQty     string     `json:"total_qty"`
	Algorithm    string     `json:"algorithm"`
	Legs         []SplitLeg `json:"legs"`
	EstimatedVWAP float64  `json:"estimated_vwap"`
	EstimatedFees float64  `json:"estimated_fees"`
	EstimatedNet  float64  `json:"estimated_net"` // VWAP + fees
	Savings       float64  `json:"savings_vs_single_venue"` // bps saved
}

// SplitLeg is one portion of a split order sent to a single provider.
type SplitLeg struct {
	Provider       string  `json:"provider"`
	AccountID      string  `json:"account_id"`
	Qty            string  `json:"qty"`
	EstimatedPrice float64 `json:"estimated_price"`
	EstimatedFee   float64 `json:"estimated_fee"` // in quote currency
	BidPrice       float64 `json:"bid_price,omitempty"`
	AskPrice       float64 `json:"ask_price,omitempty"`
	Liquidity      float64 `json:"available_liquidity,omitempty"`
}

// ExecutionResult is the outcome of executing a split plan.
type ExecutionResult struct {
	PlanID     string             `json:"plan_id"`
	Symbol     string             `json:"symbol"`
	Side       string             `json:"side"`
	Algorithm  string             `json:"algorithm"`
	TotalQty   string             `json:"total_qty"`
	FilledQty  string             `json:"filled_qty"`
	VWAP       float64            `json:"vwap"`
	TotalFees  float64            `json:"total_fees"`
	NetPrice   float64            `json:"net_price"` // VWAP + fees
	Legs       []ExecutionLeg     `json:"legs"`
	StartedAt  time.Time          `json:"started_at"`
	CompletedAt time.Time         `json:"completed_at"`
	Latency    string             `json:"latency"`
	Status     string             `json:"status"` // filled, partial, failed
}

// ExecutionLeg is the result of one split leg.
type ExecutionLeg struct {
	Provider   string  `json:"provider"`
	OrderID    string  `json:"order_id"`
	Qty        string  `json:"qty"`
	FilledQty  string  `json:"filled_qty"`
	Price      float64 `json:"price"`
	Fees       float64 `json:"fees"`
	Status     string  `json:"status"`
	Latency    string  `json:"latency"`
}

// ProviderCapability describes what a provider supports.
type ProviderCapability struct {
	Name         string   `json:"name"`
	AssetClasses []string `json:"asset_classes"` // crypto, us_equity, forex, etc.
	OrderTypes   []string `json:"order_types"`   // market, limit, stop, smart_route, twap, etc.
	Features     []string `json:"features"`      // ach, wire, custody, staking, etc.
	MakerFee     float64  `json:"maker_fee_bps"` // basis points
	TakerFee     float64  `json:"taker_fee_bps"`
	MinOrderUSD  float64  `json:"min_order_usd,omitempty"`
	MaxOrderUSD  float64  `json:"max_order_usd,omitempty"`
	Latency      string   `json:"latency,omitempty"` // typical latency
	Status       string   `json:"status"`            // active, degraded, down
}

// ProviderFees for net-price routing.
type ProviderFees struct {
	Provider string  `json:"provider"`
	MakerBps float64 `json:"maker_bps"` // maker fee in basis points
	TakerBps float64 `json:"taker_bps"` // taker fee in basis points
}

// --- Account Management Types ---

// UpdateAccountRequest for modifying account details.
type UpdateAccountRequest struct {
	Contact       *Contact  `json:"contact,omitempty"`
	Identity      *Identity `json:"identity,omitempty"`
	EnabledAssets []string  `json:"enabled_assets,omitempty"`
}

// Activity represents an account activity (fill, dividend, fee, etc).
type Activity struct {
	ID               string `json:"id"`
	AccountID        string `json:"account_id"`
	ActivityType     string `json:"activity_type"`
	Symbol           string `json:"symbol,omitempty"`
	Side             string `json:"side,omitempty"`
	Qty              string `json:"qty,omitempty"`
	Price            string `json:"price,omitempty"`
	CumQty           string `json:"cum_qty,omitempty"`
	LeavesQty        string `json:"leaves_qty,omitempty"`
	NetAmount        string `json:"net_amount,omitempty"`
	PerShareAmount   string `json:"per_share_amount,omitempty"`
	Description      string `json:"description,omitempty"`
	Status           string `json:"status,omitempty"`
	TransactionTime  string `json:"transaction_time,omitempty"`
	Date             string `json:"date,omitempty"`
}

// ActivityParams for filtering account activities.
type ActivityParams struct {
	ActivityTypes []string `json:"activity_types,omitempty"`
	Date          string   `json:"date,omitempty"`
	After         string   `json:"after,omitempty"`
	Until         string   `json:"until,omitempty"`
	Direction     string   `json:"direction,omitempty"` // asc, desc
	PageSize      int      `json:"page_size,omitempty"`
	PageToken     string   `json:"page_token,omitempty"`
}

// --- Document Types ---

// DocumentUpload for uploading account documents (W8-BEN, identity, etc).
type DocumentUpload struct {
	DocumentType    string `json:"document_type"`
	DocumentSubType string `json:"document_sub_type,omitempty"`
	Content         string `json:"content"`      // base64-encoded file content
	MimeType        string `json:"mime_type"`
}

// Document represents an uploaded or generated document.
type Document struct {
	ID              string `json:"id"`
	DocumentType    string `json:"document_type"`
	DocumentSubType string `json:"document_sub_type,omitempty"`
	Name            string `json:"name,omitempty"`
	Status          string `json:"status,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// DocumentParams for filtering documents.
type DocumentParams struct {
	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`
}

// --- Journal Types ---

// CreateJournalRequest for moving assets between accounts.
type CreateJournalRequest struct {
	EntryType     string `json:"entry_type"`     // JNLC (cash) or JNLS (security)
	FromAccount   string `json:"from_account"`
	ToAccount     string `json:"to_account"`
	Amount        string `json:"amount,omitempty"`  // for JNLC
	Symbol        string `json:"symbol,omitempty"`  // for JNLS
	Qty           string `json:"qty,omitempty"`     // for JNLS
	Description   string `json:"description,omitempty"`
}

// Journal represents a journal entry (inter-account transfer).
type Journal struct {
	ID            string `json:"id"`
	EntryType     string `json:"entry_type"`
	FromAccount   string `json:"from_account"`
	ToAccount     string `json:"to_account"`
	Symbol        string `json:"symbol,omitempty"`
	Qty           string `json:"qty,omitempty"`
	Price         string `json:"price,omitempty"`
	Amount        string `json:"amount,omitempty"`  // net_amount for JNLC
	Status        string `json:"status"`
	Description   string `json:"description,omitempty"`
	SettleDate    string `json:"settle_date,omitempty"`
	SystemDate    string `json:"system_date,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// JournalParams for filtering journals.
type JournalParams struct {
	After         string `json:"after,omitempty"`
	Before        string `json:"before,omitempty"`
	Status        string `json:"status,omitempty"`
	EntryType     string `json:"entry_type,omitempty"`
	ToAccount     string `json:"to_account,omitempty"`
	FromAccount   string `json:"from_account,omitempty"`
}

// BatchJournalRequest for creating multiple journal entries at once.
type BatchJournalRequest struct {
	EntryType   string             `json:"entry_type"`
	FromAccount string             `json:"from_account"`
	Entries     []BatchJournalEntry `json:"entries"`
	Description string             `json:"description,omitempty"`
}

// BatchJournalEntry is a single entry in a batch journal.
type BatchJournalEntry struct {
	ToAccount string `json:"to_account"`
	Amount    string `json:"amount,omitempty"`
	Symbol    string `json:"symbol,omitempty"`
	Qty       string `json:"qty,omitempty"`
}

// ReverseBatchJournalRequest for reversing batch journal entries.
type ReverseBatchJournalRequest struct {
	EntryType   string             `json:"entry_type"`
	FromAccount string             `json:"from_account"`
	Entries     []BatchJournalEntry `json:"entries"`
	Description string             `json:"description,omitempty"`
}

// --- Wire / Recipient Bank Types ---

// CreateBankRequest for creating a wire recipient bank.
type CreateBankRequest struct {
	Name             string `json:"name"`
	BankCode         string `json:"bank_code"`
	BankCodeType     string `json:"bank_code_type"` // ABA, BIC
	Country          string `json:"country,omitempty"`
	StateProvince    string `json:"state_province,omitempty"`
	PostalCode       string `json:"postal_code,omitempty"`
	City             string `json:"city,omitempty"`
	StreetAddress    string `json:"street_address,omitempty"`
	AccountNumber    string `json:"account_number"`
}

// RecipientBank represents a wire transfer recipient bank.
type RecipientBank struct {
	ID            string `json:"id"`
	AccountID     string `json:"account_id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	Country       string `json:"country,omitempty"`
	StateProvince string `json:"state_province,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	City          string `json:"city,omitempty"`
	StreetAddress string `json:"street_address,omitempty"`
	AccountNumber string `json:"account_number,omitempty"`
	BankCode      string `json:"bank_code,omitempty"`
	BankCodeType  string `json:"bank_code_type,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// --- Crypto Market Data Types ---

// CryptoBarsRequest for fetching crypto historical bars.
type CryptoBarsRequest struct {
	Symbols   []string `json:"symbols"`
	Timeframe string   `json:"timeframe"`
	Start     string   `json:"start,omitempty"`
	End       string   `json:"end,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	PageToken string   `json:"page_token,omitempty"`
}

// CryptoQuotesRequest for fetching crypto quotes.
type CryptoQuotesRequest struct {
	Symbols   []string `json:"symbols"`
	Start     string   `json:"start,omitempty"`
	End       string   `json:"end,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	PageToken string   `json:"page_token,omitempty"`
}

// CryptoTradesRequest for fetching crypto trades.
type CryptoTradesRequest struct {
	Symbols   []string `json:"symbols"`
	Start     string   `json:"start,omitempty"`
	End       string   `json:"end,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	PageToken string   `json:"page_token,omitempty"`
}

// BarsResponse contains paginated bars data.
type BarsResponse struct {
	Bars          map[string][]*Bar `json:"bars"`
	NextPageToken string            `json:"next_page_token,omitempty"`
}

// QuotesResponse contains paginated quotes data.
type QuotesResponse struct {
	Quotes        map[string][]*Quote `json:"quotes"`
	NextPageToken string              `json:"next_page_token,omitempty"`
}

// TradesResponse contains paginated trades data.
type TradesResponse struct {
	Trades        map[string][]*Trade `json:"trades"`
	NextPageToken string              `json:"next_page_token,omitempty"`
}

// CryptoSnapshot is a crypto-specific snapshot.
type CryptoSnapshot struct {
	LatestTrade *Trade `json:"latest_trade,omitempty"`
	LatestQuote *Quote `json:"latest_quote,omitempty"`
	MinuteBar   *Bar   `json:"minute_bar,omitempty"`
	DailyBar    *Bar   `json:"daily_bar,omitempty"`
	PrevDailyBar *Bar  `json:"prev_daily_bar,omitempty"`
}

// --- Event Streaming Types ---

// TradeEvent is emitted when a trade order changes state.
type TradeEvent struct {
	EventType string `json:"event_type"` // new, fill, partial_fill, canceled, expired, etc.
	EventID   string `json:"event_id"`
	AccountID string `json:"account_id"`
	Order     *Order `json:"order,omitempty"`
	Timestamp string `json:"timestamp"`
}

// AccountEvent is emitted when an account status changes.
type AccountEvent struct {
	EventType string   `json:"event_type"` // ACCOUNT_UPDATED, ACCOUNT_APPROVED, etc.
	EventID   string   `json:"event_id"`
	AccountID string   `json:"account_id"`
	Account   *Account `json:"account,omitempty"`
	Timestamp string   `json:"timestamp"`
}

// TransferEvent is emitted when a transfer changes state.
type TransferEvent struct {
	EventType  string    `json:"event_type"`
	EventID    string    `json:"event_id"`
	AccountID  string    `json:"account_id"`
	Transfer   *Transfer `json:"transfer,omitempty"`
	Timestamp  string    `json:"timestamp"`
}

// JournalEvent is emitted when a journal changes state.
type JournalEvent struct {
	EventType string   `json:"event_type"`
	EventID   string   `json:"event_id"`
	Journal   *Journal `json:"journal,omitempty"`
	Timestamp string   `json:"timestamp"`
}

// --- Portfolio History Types ---

// PortfolioHistory is a time series of portfolio equity.
type PortfolioHistory struct {
	Timestamp     []int64   `json:"timestamp"`
	Equity        []float64 `json:"equity"`
	ProfitLoss    []float64 `json:"profit_loss"`
	ProfitLossPct []float64 `json:"profit_loss_pct"`
	BaseValue     float64   `json:"base_value"`
	Timeframe     string    `json:"timeframe"`
}

// HistoryParams for fetching portfolio history.
type HistoryParams struct {
	Period       string `json:"period,omitempty"`       // 1D, 1W, 1M, 3M, 1A, all
	Timeframe    string `json:"timeframe,omitempty"`    // 1Min, 5Min, 15Min, 1H, 1D
	DateEnd      string `json:"date_end,omitempty"`
	ExtendedHours bool  `json:"extended_hours,omitempty"`
}

// --- Watchlist Types ---

// Watchlist is a named list of tracked symbols.
type Watchlist struct {
	ID        string           `json:"id"`
	AccountID string           `json:"account_id"`
	Name      string           `json:"name"`
	Assets    []WatchlistAsset `json:"assets,omitempty"`
	CreatedAt string           `json:"created_at,omitempty"`
	UpdatedAt string           `json:"updated_at,omitempty"`
}

// WatchlistAsset is an asset within a watchlist.
type WatchlistAsset struct {
	ID     string `json:"id"`
	Symbol string `json:"symbol"`
	Name   string `json:"name,omitempty"`
	Class  string `json:"class,omitempty"`
}

// CreateWatchlistRequest for creating a watchlist.
type CreateWatchlistRequest struct {
	Name    string   `json:"name"`
	Symbols []string `json:"symbols,omitempty"`
}

// UpdateWatchlistRequest for modifying a watchlist.
type UpdateWatchlistRequest struct {
	Name    string   `json:"name,omitempty"`
	Symbols []string `json:"symbols,omitempty"`
}

// --- Corporate Action Types ---

// CorporateAction represents a corporate action event (dividend, split, etc).
type CorporateAction struct {
	ID              string `json:"id"`
	Type            string `json:"type"` // dividend, split, merger, spinoff
	Symbol          string `json:"symbol"`
	SubType         string `json:"sub_type,omitempty"`
	Description     string `json:"description,omitempty"`
	RecordDate      string `json:"record_date,omitempty"`
	ExDate          string `json:"ex_date,omitempty"`
	PayableDate     string `json:"payable_date,omitempty"`
	ProcessDate     string `json:"process_date,omitempty"`
	NewRate         string `json:"new_rate,omitempty"`
	OldRate         string `json:"old_rate,omitempty"`
	CashAmount      string `json:"cash_amount,omitempty"`
}

// CorporateActionParams for filtering corporate actions.
type CorporateActionParams struct {
	Types  []string `json:"types,omitempty"` // dividend, split, merger, spinoff
	Since  string   `json:"since,omitempty"`
	Until  string   `json:"until,omitempty"`
	Symbol string   `json:"symbol,omitempty"`
}

// --- Options Types ---

// OptionChain is the full set of contracts for a symbol and expiration.
type OptionChain struct {
	Symbol     string           `json:"symbol"`
	Expiration string           `json:"expiration"`
	Calls      []OptionContract `json:"calls"`
	Puts       []OptionContract `json:"puts"`
}

// OptionContract is a single option contract in a chain.
type OptionContract struct {
	Symbol       string  `json:"symbol"`        // OCC symbol, e.g. AAPL260418C00150000
	Underlying   string  `json:"underlying"`
	ContractType string  `json:"contract_type"` // call, put
	Strike       float64 `json:"strike"`
	Expiration   string  `json:"expiration"`    // YYYY-MM-DD
	Style        string  `json:"style"`         // american, european
	Status       string  `json:"status"`
	Tradable     bool    `json:"tradable"`
	Bid          float64 `json:"bid"`
	Ask          float64 `json:"ask"`
	Last         float64 `json:"last"`
	Volume       int     `json:"volume"`
	OpenInterest int     `json:"open_interest"`
	Greeks       Greeks  `json:"greeks"`
}

// Greeks are the option sensitivity measures.
type Greeks struct {
	Delta float64 `json:"delta"`
	Gamma float64 `json:"gamma"`
	Theta float64 `json:"theta"`
	Vega  float64 `json:"vega"`
	Rho   float64 `json:"rho"`
	IV    float64 `json:"implied_volatility"`
}

// OptionQuote is a real-time quote for a single option contract.
type OptionQuote struct {
	Symbol       string  `json:"symbol"`
	Underlying   string  `json:"underlying"`
	ContractType string  `json:"contract_type"`
	Strike       float64 `json:"strike"`
	Expiration   string  `json:"expiration"`
	Bid          float64 `json:"bid"`
	Ask          float64 `json:"ask"`
	Last         float64 `json:"last"`
	Volume       int     `json:"volume"`
	OpenInterest int     `json:"open_interest"`
	Greeks       Greeks  `json:"greeks"`
}

// CreateOptionOrderRequest places a single-leg option order.
type CreateOptionOrderRequest struct {
	Symbol       string `json:"symbol"`        // underlying symbol, e.g. "AAPL"
	ContractSymbol string `json:"contract_symbol,omitempty"` // OCC symbol if known
	ContractType string `json:"contract_type"` // call, put
	Strike       string `json:"strike"`
	Expiration   string `json:"expiration"`    // YYYY-MM-DD
	Action       string `json:"action"`        // buy_to_open, buy_to_close, sell_to_open, sell_to_close
	Qty          string `json:"qty"`
	OrderType    string `json:"order_type"`    // market, limit, stop, stop_limit
	LimitPrice   string `json:"limit_price,omitempty"`
	StopPrice    string `json:"stop_price,omitempty"`
	TimeInForce  string `json:"time_in_force"` // day, gtc, ioc
}

// CreateMultiLegOrderRequest places a multi-leg strategy order.
type CreateMultiLegOrderRequest struct {
	Symbol       string      `json:"symbol"`        // underlying symbol
	StrategyType string      `json:"strategy_type"` // vertical, iron_condor, straddle, strangle, calendar, custom
	Legs         []OptionLeg `json:"legs"`
	OrderType    string      `json:"order_type"`    // limit, market
	LimitPrice   string      `json:"limit_price,omitempty"` // net debit/credit
	TimeInForce  string      `json:"time_in_force"`
}

// OptionLeg is a single leg of a multi-leg options strategy.
type OptionLeg struct {
	ContractSymbol string `json:"contract_symbol,omitempty"` // OCC symbol if known
	ContractType   string `json:"contract_type"` // call, put
	Strike         string `json:"strike"`
	Expiration     string `json:"expiration"`
	Action         string `json:"action"` // buy_to_open, buy_to_close, sell_to_open, sell_to_close
	Quantity       string `json:"qty"`
}

// MultiLegOrderResult is the result of placing a multi-leg order.
type MultiLegOrderResult struct {
	StrategyOrderID string   `json:"strategy_order_id"`
	LegOrders       []*Order `json:"leg_orders,omitempty"`
	NetPremium      string   `json:"net_premium,omitempty"`
	Status          string   `json:"status"`
}

// ExerciseOptionRequest exercises an option contract early.
type ExerciseOptionRequest struct {
	ContractSymbol string `json:"contract_symbol"`
	Qty            int    `json:"qty"`
}

// OptionPosition is a held option position.
type OptionPosition struct {
	Symbol        string  `json:"symbol"`        // OCC symbol
	Underlying    string  `json:"underlying"`
	ContractType  string  `json:"contract_type"`
	Strike        float64 `json:"strike"`
	Expiration    string  `json:"expiration"`
	Qty           string  `json:"qty"`
	AvgCost       string  `json:"avg_cost"`
	MarketValue   string  `json:"market_value"`
	CurrentPrice  string  `json:"current_price"`
	UnrealizedPnL string  `json:"unrealized_pnl"`
	Side          string  `json:"side"` // long, short
	Greeks        Greeks  `json:"greeks"`
}
