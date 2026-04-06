package router

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/types"
)

// Router provides institutional-grade smart order routing across multiple providers.
// Features: best-execution routing, order splitting, fee-aware net-price routing,
// TWAP scheduling, provider capability tracking.
type Router struct {
	registry     *provider.Registry
	fees         map[string]*types.ProviderFees // provider -> fee schedule
	capabilities map[string]*types.ProviderCapability
	mu           sync.RWMutex
}

func New(registry *provider.Registry) *Router {
	return &Router{
		registry:     registry,
		fees:         defaultFees(),
		capabilities: defaultCapabilities(),
	}
}

// SetFees configures fee schedule for a provider (used in net-price routing).
func (r *Router) SetFees(provider string, makerBps, takerBps float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fees[provider] = &types.ProviderFees{Provider: provider, MakerBps: makerBps, TakerBps: takerBps}
}

// SetCapability registers a provider's capabilities.
func (r *Router) SetCapability(cap *types.ProviderCapability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[cap.Name] = cap
}

// GetCapabilities returns all registered provider capabilities.
func (r *Router) GetCapabilities() []*types.ProviderCapability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	caps := make([]*types.ProviderCapability, 0, len(r.capabilities))
	for _, c := range r.capabilities {
		cp := *c
		if fees, ok := r.fees[cp.Name]; ok {
			cp.MakerFee = fees.MakerBps
			cp.TakerFee = fees.TakerBps
		}
		caps = append(caps, &cp)
	}
	sort.Slice(caps, func(i, j int) bool { return caps[i].Name < caps[j].Name })
	return caps
}

// RouteResult contains routing decision details.
type RouteResult struct {
	Provider    string  `json:"provider"`
	Symbol      string  `json:"symbol"`
	BidPrice    float64 `json:"bid_price,omitempty"`
	AskPrice    float64 `json:"ask_price,omitempty"`
	Spread      float64 `json:"spread,omitempty"`
	SpreadBps   float64 `json:"spread_bps,omitempty"`
	MakerFeeBps float64 `json:"maker_fee_bps,omitempty"`
	TakerFeeBps float64 `json:"taker_fee_bps,omitempty"`
	NetPrice    float64 `json:"net_price,omitempty"`  // price + fees
	Score       float64 `json:"score"`                // lower = better
}

// FindBestProvider returns the best provider for trading a given symbol.
func (r *Router) FindBestProvider(ctx context.Context, symbol, side string) (*RouteResult, error) {
	routes, err := r.GetAllRoutes(ctx, symbol, side)
	if err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no provider supports symbol %s", symbol)
	}
	return routes[0], nil
}

// GetAllRoutes returns all available routes for a symbol, ranked by net execution price.
func (r *Router) GetAllRoutes(ctx context.Context, symbol, side string) ([]*RouteResult, error) {
	providerNames := r.registry.List()
	if len(providerNames) == 0 {
		return nil, fmt.Errorf("no providers registered")
	}

	type result struct {
		route *RouteResult
	}

	ch := make(chan result, len(providerNames))
	var wg sync.WaitGroup

	for _, name := range providerNames {
		wg.Add(1)
		go func(provName string) {
			defer wg.Done()
			p, err := r.registry.Get(provName)
			if err != nil {
				return
			}

			asset, err := p.GetAsset(ctx, symbol)
			if err != nil || !asset.Tradable {
				return
			}

			route := &RouteResult{
				Provider: provName,
				Symbol:   symbol,
				Score:    1000,
			}

			// Get fee schedule
			r.mu.RLock()
			fees := r.fees[provName]
			r.mu.RUnlock()
			if fees != nil {
				route.MakerFeeBps = fees.MakerBps
				route.TakerFeeBps = fees.TakerBps
			}

			snap, err := p.GetSnapshot(ctx, symbol)
			if err == nil && snap.LatestQuote != nil {
				q := snap.LatestQuote
				route.BidPrice = q.BidPrice
				route.AskPrice = q.AskPrice
				if q.AskPrice > 0 && q.BidPrice > 0 {
					route.Spread = q.AskPrice - q.BidPrice
					mid := (q.AskPrice + q.BidPrice) / 2
					if mid > 0 {
						route.SpreadBps = (route.Spread / mid) * 10000
					}

					// Net-price routing: factor in taker fees
					feeBps := route.TakerFeeBps
					if strings.EqualFold(side, "buy") {
						route.NetPrice = q.AskPrice * (1 + feeBps/10000)
						route.Score = route.NetPrice
					} else {
						route.NetPrice = q.BidPrice * (1 - feeBps/10000)
						route.Score = -route.NetPrice // negate: higher net = better for sells
					}
				}
			} else if err == nil && snap.LatestTrade != nil {
				route.Score = 500
			} else {
				route.Score = 999
			}

			ch <- result{route: route}
		}(name)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var routes []*RouteResult
	for res := range ch {
		if res.route != nil {
			routes = append(routes, res.route)
		}
	}

	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Score < routes[j].Score
	})

	return routes, nil
}

// BuildSplitPlan creates an execution plan that splits an order across providers.
// This minimizes market impact for large orders and captures best price across venues.
func (r *Router) BuildSplitPlan(ctx context.Context, symbol, side, qty string) (*types.SplitPlan, error) {
	routes, err := r.GetAllRoutes(ctx, symbol, side)
	if err != nil {
		return nil, err
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("no providers support %s", symbol)
	}

	totalQty, _ := strconv.ParseFloat(qty, 64)
	if totalQty <= 0 {
		return nil, fmt.Errorf("invalid quantity")
	}

	plan := &types.SplitPlan{
		Symbol:    symbol,
		Side:      side,
		TotalQty:  qty,
		Algorithm: "split",
	}

	// For routes with quotes, distribute proportionally to spread quality
	quotedRoutes := make([]*RouteResult, 0)
	for _, route := range routes {
		if route.BidPrice > 0 || route.AskPrice > 0 {
			quotedRoutes = append(quotedRoutes, route)
		}
	}

	if len(quotedRoutes) == 0 {
		// No quotes — send all to first available
		plan.Legs = []types.SplitLeg{{
			Provider:       routes[0].Provider,
			Qty:            qty,
			EstimatedPrice: 0,
		}}
		return plan, nil
	}

	// Score-weighted split: better scores get more volume
	// Invert scores so lower score = higher weight
	var totalWeight float64
	weights := make([]float64, len(quotedRoutes))
	for i, route := range quotedRoutes {
		if route.Score > 0 {
			weights[i] = 1.0 / route.Score
		} else {
			weights[i] = 1.0 / math.Abs(route.Score)
		}
		totalWeight += weights[i]
	}

	var estimatedVWAP, estimatedFees, totalAllocated float64
	for i, route := range quotedRoutes {
		proportion := weights[i] / totalWeight
		legQty := totalQty * proportion

		// Last leg gets remainder to avoid rounding issues
		if i == len(quotedRoutes)-1 {
			legQty = totalQty - totalAllocated
		}
		totalAllocated += legQty

		var price float64
		if strings.EqualFold(side, "buy") {
			price = route.AskPrice
		} else {
			price = route.BidPrice
		}

		feePct := route.TakerFeeBps / 10000
		legFee := legQty * price * feePct

		plan.Legs = append(plan.Legs, types.SplitLeg{
			Provider:       route.Provider,
			Qty:            fmt.Sprintf("%.8f", legQty),
			EstimatedPrice: price,
			EstimatedFee:   legFee,
			BidPrice:       route.BidPrice,
			AskPrice:       route.AskPrice,
		})

		estimatedVWAP += price * legQty
		estimatedFees += legFee
	}

	if totalQty > 0 {
		plan.EstimatedVWAP = estimatedVWAP / totalQty
	}
	plan.EstimatedFees = estimatedFees
	plan.EstimatedNet = plan.EstimatedVWAP + (estimatedFees / totalQty)

	// Calculate savings vs single venue (worst route)
	if len(quotedRoutes) > 1 {
		worstPrice := quotedRoutes[len(quotedRoutes)-1].NetPrice
		if worstPrice != 0 && plan.EstimatedNet != 0 {
			plan.Savings = math.Abs((worstPrice-plan.EstimatedNet)/plan.EstimatedNet) * 10000
		}
	}

	// Crypto aggregate notional limit: Alpaca restricts crypto orders to $200K.
	// Individual leg checks in CreateOrder are insufficient — the aggregate
	// across all legs must also respect the limit.
	isCryptoSymbol := strings.Contains(symbol, "/")
	if isCryptoSymbol && plan.EstimatedVWAP > 0 {
		aggregateNotional := plan.EstimatedVWAP * totalQty
		if aggregateNotional > 200000 {
			return nil, fmt.Errorf("crypto split plan aggregate notional $%.2f exceeds $200,000 limit", aggregateNotional)
		}
	}

	return plan, nil
}

// ExecuteSplitPlan executes all legs of a split plan in parallel.
func (r *Router) ExecuteSplitPlan(ctx context.Context, plan *types.SplitPlan, accounts map[string]string) (*types.ExecutionResult, error) {
	start := time.Now()
	result := &types.ExecutionResult{
		PlanID:    fmt.Sprintf("exec_%d", start.UnixNano()),
		Symbol:    plan.Symbol,
		Side:      plan.Side,
		Algorithm: plan.Algorithm,
		TotalQty:  plan.TotalQty,
		StartedAt: start,
	}

	type legResult struct {
		leg types.ExecutionLeg
		err error
	}

	ch := make(chan legResult, len(plan.Legs))
	var wg sync.WaitGroup

	for _, leg := range plan.Legs {
		wg.Add(1)
		go func(l types.SplitLeg) {
			defer wg.Done()
			legStart := time.Now()

			accountID, ok := accounts[l.Provider]
			if !ok {
				ch <- legResult{err: fmt.Errorf("no account for provider %s", l.Provider)}
				return
			}

			p, err := r.registry.Get(l.Provider)
			if err != nil {
				ch <- legResult{err: err}
				return
			}

			order, err := p.CreateOrder(ctx, accountID, &types.CreateOrderRequest{
				Symbol:      plan.Symbol,
				Qty:         l.Qty,
				Side:        plan.Side,
				Type:        "market",
				TimeInForce: "ioc",
			})

			el := types.ExecutionLeg{
				Provider: l.Provider,
				Qty:      l.Qty,
				Latency:  time.Since(legStart).String(),
			}

			if err != nil {
				el.Status = "failed"
				ch <- legResult{leg: el, err: err}
				return
			}

			el.OrderID = order.ProviderID
			el.FilledQty = order.FilledQty
			el.Status = order.Status
			if order.FilledAvgPrice != "" {
				el.Price, _ = strconv.ParseFloat(order.FilledAvgPrice, 64)
			}
			ch <- legResult{leg: el}
		}(leg)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var totalFilled, weightedPrice float64
	allFilled := true
	for lr := range ch {
		result.Legs = append(result.Legs, lr.leg)
		filled, _ := strconv.ParseFloat(lr.leg.FilledQty, 64)
		totalFilled += filled
		weightedPrice += lr.leg.Price * filled
		if lr.leg.Status != "filled" {
			allFilled = false
		}
	}

	result.FilledQty = fmt.Sprintf("%.8f", totalFilled)
	if totalFilled > 0 {
		result.VWAP = weightedPrice / totalFilled
	}
	result.CompletedAt = time.Now()
	result.Latency = time.Since(start).String()

	if allFilled {
		result.Status = "filled"
	} else if totalFilled > 0 {
		result.Status = "partial"
	} else {
		result.Status = "failed"
	}

	return result, nil
}

// SmartOrder places an order using the best available provider.
func (r *Router) SmartOrder(ctx context.Context, accountsByProvider map[string]string, req *types.CreateOrderRequest) (*types.Order, error) {
	best, err := r.FindBestProvider(ctx, req.Symbol, req.Side)
	if err != nil {
		return nil, fmt.Errorf("routing failed: %w", err)
	}

	accountID, ok := accountsByProvider[best.Provider]
	if !ok {
		// Auto-resolve: use first available account for this provider
		p, err := r.registry.Get(best.Provider)
		if err != nil {
			return nil, err
		}
		accts, err := p.ListAccounts(ctx)
		if err != nil || len(accts) == 0 {
			return nil, fmt.Errorf("no account configured for provider %s", best.Provider)
		}
		accountID = accts[0].ID
	}

	p, err := r.registry.Get(best.Provider)
	if err != nil {
		return nil, err
	}

	order, err := p.CreateOrder(ctx, accountID, req)
	if err != nil {
		return nil, fmt.Errorf("order via %s failed: %w", best.Provider, err)
	}
	return order, nil
}

// OptionRouteResult contains routing decision details for an options order.
type OptionRouteResult struct {
	Provider  string  `json:"provider"`
	Contract  string  `json:"contract"`
	Bid       float64 `json:"bid,omitempty"`
	Ask       float64 `json:"ask,omitempty"`
	Spread    float64 `json:"spread,omitempty"`
	SpreadBps float64 `json:"spread_bps,omitempty"`
	IV        float64 `json:"implied_volatility,omitempty"`
	NetPrice  float64 `json:"net_price,omitempty"`
	Score     float64 `json:"score"`
}

// RouteOptionOrder finds the best provider for an options contract.
// It queries all providers that implement OptionsProvider, compares quotes, and
// returns routes ranked by net execution price (spread + fees).
func (r *Router) RouteOptionOrder(ctx context.Context, contractSymbol, side string) ([]*OptionRouteResult, error) {
	providerNames := r.registry.List()
	if len(providerNames) == 0 {
		return nil, fmt.Errorf("no providers registered")
	}

	type result struct {
		route *OptionRouteResult
	}
	ch := make(chan result, len(providerNames))
	var wg sync.WaitGroup

	for _, name := range providerNames {
		wg.Add(1)
		go func(provName string) {
			defer wg.Done()

			p, err := r.registry.Get(provName)
			if err != nil {
				return
			}

			// Check if provider supports options
			op, ok := p.(provider.OptionsProvider)
			if !ok {
				return
			}

			quote, err := op.GetOptionQuote(ctx, contractSymbol)
			if err != nil {
				return
			}

			route := &OptionRouteResult{
				Provider: provName,
				Contract: contractSymbol,
				Bid:      quote.Bid,
				Ask:      quote.Ask,
				IV:       quote.Greeks.IV,
				Score:    1000,
			}

			// Get fee schedule
			r.mu.RLock()
			fees := r.fees[provName]
			r.mu.RUnlock()

			feeBps := 0.0
			if fees != nil {
				feeBps = fees.TakerBps
			}

			if quote.Ask > 0 && quote.Bid > 0 {
				route.Spread = quote.Ask - quote.Bid
				mid := (quote.Ask + quote.Bid) / 2
				if mid > 0 {
					route.SpreadBps = (route.Spread / mid) * 10000
				}

				if strings.EqualFold(side, "buy") {
					route.NetPrice = quote.Ask * (1 + feeBps/10000)
					route.Score = route.NetPrice
				} else {
					route.NetPrice = quote.Bid * (1 - feeBps/10000)
					route.Score = -route.NetPrice
				}
			}

			ch <- result{route: route}
		}(name)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var routes []*OptionRouteResult
	for res := range ch {
		if res.route != nil {
			routes = append(routes, res.route)
		}
	}

	if len(routes) == 0 {
		return nil, fmt.Errorf("no provider supports options contract %s", contractSymbol)
	}

	sort.Slice(routes, func(i, j int) bool {
		return routes[i].Score < routes[j].Score
	})

	return routes, nil
}

// AggregatedAssets returns all tradable assets across all providers, deduplicated.
func (r *Router) AggregatedAssets(ctx context.Context) ([]*AggregatedAsset, error) {
	providerNames := r.registry.List()
	assetMap := make(map[string]*AggregatedAsset)

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, name := range providerNames {
		wg.Add(1)
		go func(provName string) {
			defer wg.Done()
			p, _ := r.registry.Get(provName)
			assets, err := p.ListAssets(ctx, "")
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, a := range assets {
				if !a.Tradable {
					continue
				}
				sym := strings.ToUpper(a.Symbol)
				if existing, ok := assetMap[sym]; ok {
					existing.Providers = append(existing.Providers, provName)
				} else {
					assetMap[sym] = &AggregatedAsset{
						Symbol:    sym,
						Name:      a.Name,
						Class:     a.Class,
						Providers: []string{provName},
					}
				}
			}
		}(name)
	}
	wg.Wait()

	result := make([]*AggregatedAsset, 0, len(assetMap))
	for _, a := range assetMap {
		sort.Strings(a.Providers)
		result = append(result, a)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Symbol < result[j].Symbol
	})
	return result, nil
}

// AggregatedAsset is an asset available across multiple providers.
type AggregatedAsset struct {
	Symbol    string   `json:"symbol"`
	Name      string   `json:"name"`
	Class     string   `json:"class"`
	Providers []string `json:"providers"`
}

// --- Default configurations ---

func defaultFees() map[string]*types.ProviderFees {
	return map[string]*types.ProviderFees{
		"alpaca":   {Provider: "alpaca", MakerBps: 0, TakerBps: 0},         // commission-free equities
		"sfox":     {Provider: "sfox", MakerBps: 15, TakerBps: 25},         // 15/25 bps
		"coinbase": {Provider: "coinbase", MakerBps: 40, TakerBps: 60},     // 40/60 bps
		"kraken":   {Provider: "kraken", MakerBps: 16, TakerBps: 26},       // 16/26 bps
		"binance":  {Provider: "binance", MakerBps: 10, TakerBps: 10},      // 10/10 bps
		"gemini":   {Provider: "gemini", MakerBps: 20, TakerBps: 40},       // 20/40 bps
		"bitgo":    {Provider: "bitgo", MakerBps: 25, TakerBps: 25},        // estimate
		"falcon":   {Provider: "falcon", MakerBps: 5, TakerBps: 10},        // institutional
		"ibkr":     {Provider: "ibkr", MakerBps: 0.5, TakerBps: 0.5},      // per-share, approximated
		"finix":         {Provider: "finix", MakerBps: 0, TakerBps: 0},          // payment processor
		"fireblocks":    {Provider: "fireblocks", MakerBps: 0, TakerBps: 0},      // custody, no trading fees
		"circle":        {Provider: "circle", MakerBps: 0, TakerBps: 0},          // stablecoin transfers
		"tradier":       {Provider: "tradier", MakerBps: 0, TakerBps: 0},         // commission-free equities
		"polygon":       {Provider: "polygon", MakerBps: 0, TakerBps: 0},         // market data only
		"currencycloud": {Provider: "currencycloud", MakerBps: 3, TakerBps: 5},   // tight FX spreads
		"lmax":          {Provider: "lmax", MakerBps: 2, TakerBps: 3},            // institutional CLOB
	}
}

func defaultCapabilities() map[string]*types.ProviderCapability {
	return map[string]*types.ProviderCapability{
		"alpaca": {
			Name: "alpaca", Status: "active",
			AssetClasses: []string{"us_equity", "us_option", "crypto"},
			OrderTypes:   []string{"market", "limit", "stop", "stop_limit", "trailing_stop"},
			Features:     []string{"ach", "wire", "fractional", "extended_hours", "margin", "options", "multi_leg"},
		},
		"sfox": {
			Name: "sfox", Status: "active",
			AssetClasses: []string{"crypto"},
			OrderTypes:   []string{"market", "limit", "stop", "smart_route", "twap", "sniper", "hare", "tortoise", "polar_bear", "gorilla"},
			Features:     []string{"smart_routing", "dark_pool", "custody", "150+_pairs"},
		},
		"coinbase": {
			Name: "coinbase", Status: "active",
			AssetClasses: []string{"crypto"},
			OrderTypes:   []string{"market", "limit", "stop"},
			Features:     []string{"staking", "custody", "250+_pairs"},
		},
		"kraken": {
			Name: "kraken", Status: "active",
			AssetClasses: []string{"crypto"},
			OrderTypes:   []string{"market", "limit", "stop", "stop_limit", "trailing_stop"},
			Features:     []string{"staking", "margin", "futures", "200+_pairs"},
		},
		"binance": {
			Name: "binance", Status: "active",
			AssetClasses: []string{"crypto"},
			OrderTypes:   []string{"market", "limit", "stop", "oco"},
			Features:     []string{"margin", "futures", "staking", "600+_pairs"},
		},
		"gemini": {
			Name: "gemini", Status: "active",
			AssetClasses: []string{"crypto"},
			OrderTypes:   []string{"market", "limit", "stop"},
			Features:     []string{"custody", "staking", "100+_pairs"},
		},
		"ibkr": {
			Name: "ibkr", Status: "active",
			AssetClasses: []string{"us_equity", "us_option", "futures", "forex", "bond", "crypto"},
			OrderTypes:   []string{"market", "limit", "stop", "stop_limit", "trailing_stop", "algo"},
			Features:     []string{"margin", "short_selling", "options", "multi_leg", "futures", "global_markets"},
		},
		"bitgo": {
			Name: "bitgo", Status: "active",
			AssetClasses: []string{"crypto"},
			OrderTypes:   []string{"market", "limit"},
			Features:     []string{"custody", "multisig", "staking", "prime_trading"},
		},
		"falcon": {
			Name: "falcon", Status: "active",
			AssetClasses: []string{"crypto"},
			OrderTypes:   []string{"rfq"},
			Features:     []string{"institutional", "otc", "settlement"},
		},
		"finix": {
			Name: "finix", Status: "active",
			AssetClasses: []string{"payments"},
			OrderTypes:   []string{},
			Features:     []string{"ach", "wire", "card", "apple_pay", "google_pay"},
		},
		"fireblocks": {
			Name: "fireblocks", Status: "active",
			AssetClasses: []string{"crypto"},
			OrderTypes:   []string{"transfer"},
			Features:     []string{"custody", "multisig", "mpc", "defi", "staking", "nft", "500+_assets"},
		},
		"circle": {
			Name: "circle", Status: "active",
			AssetClasses: []string{"stablecoin"},
			OrderTypes:   []string{"transfer"},
			Features:     []string{"usdc", "eurc", "mint", "burn", "cross_chain"},
		},
		"tradier": {
			Name: "tradier", Status: "active",
			AssetClasses: []string{"us_equity", "us_option"},
			OrderTypes:   []string{"market", "limit", "stop", "stop_limit"},
			Features:     []string{"commission_free", "options", "multi_leg", "streaming", "extended_hours"},
		},
		"polygon": {
			Name: "polygon", Status: "active",
			AssetClasses: []string{"us_equity", "crypto", "forex", "us_option"},
			OrderTypes:   []string{},
			Features:     []string{"market_data", "snapshots", "bars", "trades", "quotes", "real_time"},
		},
		"currencycloud": {
			Name: "currencycloud", Status: "active",
			AssetClasses: []string{"forex"},
			OrderTypes:   []string{"spot", "forward"},
			Features:     []string{"fx_trading", "cross_border", "35+_currencies", "competitive_spreads", "payments"},
		},
		"lmax": {
			Name: "lmax", Status: "active",
			AssetClasses: []string{"forex", "crypto", "commodity"},
			OrderTypes:   []string{"market", "limit", "stop"},
			Features:     []string{"institutional", "clob", "fca_regulated", "sub_ms_matching", "precious_metals"},
		},
	}
}
