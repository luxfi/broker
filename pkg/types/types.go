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
	AccountID   string `json:"account_id"`
	Symbol      string `json:"symbol"`
	Qty         string `json:"qty,omitempty"`
	Notional    string `json:"notional,omitempty"`
	Side        string `json:"side"`
	Type        string `json:"type"`
	TimeInForce string `json:"time_in_force"`
	LimitPrice  string `json:"limit_price,omitempty"`
	StopPrice   string `json:"stop_price,omitempty"`
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
	Provider     string    `json:"provider"` // which broker to use
	OrgID        string    `json:"-"`        // set from auth context
	UserID       string    `json:"-"`
	Identity     *Identity `json:"identity"`
	Contact      *Contact  `json:"contact"`
	EnabledAssets []string `json:"enabled_assets,omitempty"`
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
