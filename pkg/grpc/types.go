// Package grpc types — hand-written message types matching proto/broker.proto.
// These will be replaced by protoc-generated code in brokerpb/ once CI runs
// `protoc --go_out=. --go-grpc_out=. proto/broker.proto`.
package grpc

// --- Request types ---

type GetQuoteRequest struct {
	Symbol string
}

type GetBBORequest struct {
	Symbol string
}

type GetRoutesRequest struct {
	Symbol string
	Side   string
}

type PlaceOrderRequest struct {
	Provider    string
	AccountID   string
	Symbol      string
	Qty         string
	Side        string
	Type        string
	TimeInForce string
	LimitPrice  string
	StopPrice   string
}

type SmartOrderRequest struct {
	Symbol      string
	Qty         string
	Side        string
	Type        string
	TimeInForce string
	Accounts    map[string]string
}

type CancelOrderRequest struct {
	Provider  string
	AccountID string
	OrderID   string
}

type GetSplitPlanRequest struct {
	Symbol string
	Side   string
	Qty    string
}

type ExecuteSplitRequest struct {
	Symbol   string
	Side     string
	TotalQty string
	Legs     []*SplitLeg
	Accounts map[string]string
}

type StartTWAPRequest struct {
	Symbol          string
	Side            string
	TotalQty        float64
	DurationSeconds int64
	Slices          int32
	MaxSlippageBps  float64
	Accounts        map[string]string
}

type CancelTWAPRequest struct {
	ExecutionID string
}

type GetTWAPRequest struct {
	ExecutionID string
}

type ScanArbitrageRequest struct {
	ThresholdBps float64
	Symbol       string
}

type StreamQuotesRequest struct {
	Symbols []string
}

type ListProvidersRequest struct{}

type HealthCheckRequest struct{}

// --- Response types ---

type QuoteResponse struct {
	Symbol      string  `json:"symbol"`
	BidPrice    float64 `json:"bid_price"`
	AskPrice    float64 `json:"ask_price"`
	BidSize     float64 `json:"bid_size"`
	AskSize     float64 `json:"ask_size"`
	BidProvider string  `json:"bid_provider"`
	AskProvider string  `json:"ask_provider"`
	SpreadBps   float64 `json:"spread_bps"`
	Timestamp   string  `json:"timestamp"`
}

type BBOResponse struct {
	Symbol          string  `json:"symbol"`
	BestBid         float64 `json:"best_bid"`
	BestAsk         float64 `json:"best_ask"`
	BestBidProvider string  `json:"best_bid_provider"`
	BestAskProvider string  `json:"best_ask_provider"`
	Spread          float64 `json:"spread"`
	SpreadBps       float64 `json:"spread_bps"`
}

type GetRoutesResponse struct {
	Routes []*RouteResult `json:"routes"`
}

type RouteResult struct {
	Provider    string  `json:"provider"`
	Symbol      string  `json:"symbol"`
	BidPrice    float64 `json:"bid_price"`
	AskPrice    float64 `json:"ask_price"`
	SpreadBps   float64 `json:"spread_bps"`
	MakerFeeBps float64 `json:"maker_fee_bps"`
	TakerFeeBps float64 `json:"taker_fee_bps"`
	NetPrice    float64 `json:"net_price"`
	Score       float64 `json:"score"`
}

type OrderResponse struct {
	ID             string `json:"id"`
	Provider       string `json:"provider"`
	ProviderID     string `json:"provider_id"`
	Symbol         string `json:"symbol"`
	Status         string `json:"status"`
	FilledQty      string `json:"filled_qty"`
	FilledAvgPrice string `json:"filled_avg_price"`
}

type CancelOrderResponse struct {
	Success bool `json:"success"`
}

type SplitPlanResponse struct {
	Symbol        string     `json:"symbol"`
	Side          string     `json:"side"`
	TotalQty      string     `json:"total_qty"`
	Algorithm     string     `json:"algorithm"`
	Legs          []*SplitLeg `json:"legs"`
	EstimatedVWAP float64    `json:"estimated_vwap"`
	EstimatedFees float64    `json:"estimated_fees"`
	EstimatedNet  float64    `json:"estimated_net"`
	SavingsBps    float64    `json:"savings_bps"`
}

type SplitLeg struct {
	Provider       string  `json:"provider"`
	Qty            string  `json:"qty"`
	EstimatedPrice float64 `json:"estimated_price"`
	EstimatedFee   float64 `json:"estimated_fee"`
	BidPrice       float64 `json:"bid_price"`
	AskPrice       float64 `json:"ask_price"`
}

type ExecutionResultResponse struct {
	PlanID    string          `json:"plan_id"`
	Symbol    string          `json:"symbol"`
	Side      string          `json:"side"`
	Status    string          `json:"status"`
	FilledQty string          `json:"filled_qty"`
	VWAP      float64         `json:"vwap"`
	Latency   string          `json:"latency"`
	Legs      []*ExecutionLeg `json:"legs"`
}

type ExecutionLeg struct {
	Provider  string  `json:"provider"`
	OrderID   string  `json:"order_id"`
	Qty       string  `json:"qty"`
	FilledQty string  `json:"filled_qty"`
	Price     float64 `json:"price"`
	Status    string  `json:"status"`
	Latency   string  `json:"latency"`
}

type TWAPResponse struct {
	ID           string  `json:"id"`
	Status       string  `json:"status"`
	SlicesFilled int32   `json:"slices_filled"`
	TotalFilled  float64 `json:"total_filled"`
	VWAP         float64 `json:"vwap"`
	StartedAt    string  `json:"started_at"`
	CompletedAt  string  `json:"completed_at"`
}

type ScanArbitrageResponse struct {
	Opportunities []*ArbitrageOpportunity `json:"opportunities"`
}

type ArbitrageOpportunity struct {
	Symbol     string  `json:"symbol"`
	BuyVenue   string  `json:"buy_venue"`
	SellVenue  string  `json:"sell_venue"`
	BuyPrice   float64 `json:"buy_price"`
	SellPrice  float64 `json:"sell_price"`
	SpreadAbs  float64 `json:"spread_abs"`
	SpreadBps  float64 `json:"spread_bps"`
	DetectedAt string  `json:"detected_at"`
}

type ListProvidersResponse struct {
	Providers []string `json:"providers"`
}

type HealthCheckResponse struct {
	Status    string   `json:"status"`
	Providers []string `json:"providers"`
}
