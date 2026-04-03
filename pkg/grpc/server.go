// Package grpc provides a gRPC server for the broker's smart order routing,
// market data, and order execution services. It wraps the same core packages
// (router, marketdata, provider) that the REST API uses.
//
// The protobuf service definition lives in proto/broker.proto. This file
// implements the server-side handlers. Generated code goes in brokerpb/ and
// is produced by CI via `protoc --go_out=. --go-grpc_out=. proto/broker.proto`.
//
// Until generated stubs are available, the server registers handlers manually
// using the grpc package's generic service registration.
package grpc

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/auth"
	"github.com/luxfi/broker/pkg/marketdata"
	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/router"
	"github.com/luxfi/broker/pkg/types"

	ggrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Server wraps a gRPC server with broker-specific handlers.
type Server struct {
	registry  *provider.Registry
	router    *router.Router
	twap      *router.TWAPScheduler
	feed      *marketdata.Feed
	arbDet    *marketdata.ArbitrageDetector
	grpcSrv   *ggrpc.Server
	listener  net.Listener
}

// Config holds gRPC server configuration.
type Config struct {
	ListenAddr        string
	IAMEndpoint       string
	Registry          *provider.Registry
	Router            *router.Router
	TWAPScheduler     *router.TWAPScheduler
	Feed              *marketdata.Feed
	ArbitrageDetector *marketdata.ArbitrageDetector
}

// NewServer creates a gRPC server with all broker services registered.
func NewServer(cfg Config) (*Server, error) {
	lis, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("grpc listen: %w", err)
	}

	iamEndpoint := cfg.IAMEndpoint
	if iamEndpoint == "" {
		iamEndpoint = os.Getenv("IAM_ENDPOINT")
		if iamEndpoint == "" {
			iamEndpoint = "http://localhost:8000"
		}
	}

	grpcSrv := ggrpc.NewServer(
		ggrpc.ChainUnaryInterceptor(
			authInterceptor(iamEndpoint),
			loggingInterceptor,
		),
	)

	s := &Server{
		registry: cfg.Registry,
		router:   cfg.Router,
		twap:     cfg.TWAPScheduler,
		feed:     cfg.Feed,
		arbDet:   cfg.ArbitrageDetector,
		grpcSrv:  grpcSrv,
		listener: lis,
	}

	// Register the health service.
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("broker.v1.BrokerService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)

	// Enable reflection for grpcurl / grpc-client tooling.
	reflection.Register(grpcSrv)

	// Register our broker service. The actual generated RegisterBrokerServiceServer
	// call goes here once protoc output is available. For now we use the
	// ServiceRegistrar interface with a manually constructed ServiceDesc.
	grpcSrv.RegisterService(&brokerServiceDesc, s)

	return s, nil
}

// Serve starts the gRPC server. Blocks until Stop is called.
func (s *Server) Serve() error {
	return s.grpcSrv.Serve(s.listener)
}

// Stop gracefully stops the gRPC server.
func (s *Server) Stop() {
	s.grpcSrv.GracefulStop()
}

// Addr returns the listener address (useful in tests).
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// --- RPC Handlers ---

// GetQuote returns the consolidated quote for a symbol.
func (s *Server) GetQuote(ctx context.Context, req *GetQuoteRequest) (*QuoteResponse, error) {
	if req.Symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}
	ticker, err := s.feed.GetTicker(req.Symbol)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no data for %s", req.Symbol)
	}
	return &QuoteResponse{
		Symbol:      req.Symbol,
		BidPrice:    ticker.BestBid,
		AskPrice:    ticker.BestAsk,
		BidProvider: ticker.BestBidProvider,
		AskProvider: ticker.BestAskProvider,
		SpreadBps:   ticker.SpreadBps,
		Timestamp:   ticker.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// GetBBO returns best bid/offer for a symbol.
func (s *Server) GetBBO(ctx context.Context, req *GetBBORequest) (*BBOResponse, error) {
	if req.Symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}
	bid, ask, bidProv, askProv, err := s.feed.GetBBO(req.Symbol)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no data for %s", req.Symbol)
	}
	spread := ask - bid
	var spreadBps float64
	if mid := (ask + bid) / 2; mid > 0 {
		spreadBps = (spread / mid) * 10000
	}
	return &BBOResponse{
		Symbol:          req.Symbol,
		BestBid:         bid,
		BestAsk:         ask,
		BestBidProvider: bidProv,
		BestAskProvider: askProv,
		Spread:          spread,
		SpreadBps:       spreadBps,
	}, nil
}

// GetRoutes returns all available routes for a symbol ranked by net price.
func (s *Server) GetRoutes(ctx context.Context, req *GetRoutesRequest) (*GetRoutesResponse, error) {
	if req.Symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol is required")
	}
	routes, err := s.router.GetAllRoutes(ctx, req.Symbol, req.Side)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "routing failed: %v", err)
	}
	resp := &GetRoutesResponse{}
	for _, r := range routes {
		resp.Routes = append(resp.Routes, &RouteResult{
			Provider:    r.Provider,
			Symbol:      r.Symbol,
			BidPrice:    r.BidPrice,
			AskPrice:    r.AskPrice,
			SpreadBps:   r.SpreadBps,
			MakerFeeBps: r.MakerFeeBps,
			TakerFeeBps: r.TakerFeeBps,
			NetPrice:    r.NetPrice,
			Score:       r.Score,
		})
	}
	return resp, nil
}

// PlaceOrder places an order at a specific provider.
func (s *Server) PlaceOrder(ctx context.Context, req *PlaceOrderRequest) (*OrderResponse, error) {
	if req.Provider == "" || req.AccountID == "" || req.Symbol == "" {
		return nil, status.Error(codes.InvalidArgument, "provider, account_id, and symbol are required")
	}
	p, err := s.registry.Get(req.Provider)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "provider %s not found", req.Provider)
	}
	order, err := p.CreateOrder(ctx, req.AccountID, &types.CreateOrderRequest{
		Symbol:      req.Symbol,
		Qty:         req.Qty,
		Side:        req.Side,
		Type:        req.Type,
		TimeInForce: req.TimeInForce,
		LimitPrice:  req.LimitPrice,
		StopPrice:   req.StopPrice,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "order failed: %v", err)
	}
	return orderToResponse(order), nil
}

// SmartOrder routes to the best-price venue.
func (s *Server) SmartOrder(ctx context.Context, req *SmartOrderRequest) (*OrderResponse, error) {
	if req.Symbol == "" || req.Side == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol and side are required")
	}
	order, err := s.router.SmartOrder(ctx, req.Accounts, &types.CreateOrderRequest{
		Symbol:      req.Symbol,
		Qty:         req.Qty,
		Side:        req.Side,
		Type:        req.Type,
		TimeInForce: req.TimeInForce,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "smart order failed: %v", err)
	}
	return orderToResponse(order), nil
}

// CancelOrder cancels a pending order.
func (s *Server) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*CancelOrderResponse, error) {
	if req.Provider == "" || req.AccountID == "" || req.OrderID == "" {
		return nil, status.Error(codes.InvalidArgument, "provider, account_id, and order_id are required")
	}
	p, err := s.registry.Get(req.Provider)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "provider %s not found", req.Provider)
	}
	if err := p.CancelOrder(ctx, req.AccountID, req.OrderID); err != nil {
		return nil, status.Errorf(codes.Internal, "cancel failed: %v", err)
	}
	return &CancelOrderResponse{Success: true}, nil
}

// GetSplitPlan returns a VWAP split plan.
func (s *Server) GetSplitPlan(ctx context.Context, req *GetSplitPlanRequest) (*SplitPlanResponse, error) {
	if req.Symbol == "" || req.Qty == "" {
		return nil, status.Error(codes.InvalidArgument, "symbol and qty are required")
	}
	plan, err := s.router.BuildSplitPlan(ctx, req.Symbol, req.Side, req.Qty)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "split plan failed: %v", err)
	}
	resp := &SplitPlanResponse{
		Symbol:        plan.Symbol,
		Side:          plan.Side,
		TotalQty:      plan.TotalQty,
		Algorithm:     plan.Algorithm,
		EstimatedVWAP: plan.EstimatedVWAP,
		EstimatedFees: plan.EstimatedFees,
		EstimatedNet:  plan.EstimatedNet,
		SavingsBps:    plan.Savings,
	}
	for _, leg := range plan.Legs {
		resp.Legs = append(resp.Legs, &SplitLeg{
			Provider:       leg.Provider,
			Qty:            leg.Qty,
			EstimatedPrice: leg.EstimatedPrice,
			EstimatedFee:   leg.EstimatedFee,
			BidPrice:       leg.BidPrice,
			AskPrice:       leg.AskPrice,
		})
	}
	return resp, nil
}

// ExecuteSplit executes a split plan.
func (s *Server) ExecuteSplit(ctx context.Context, req *ExecuteSplitRequest) (*ExecutionResultResponse, error) {
	plan := &types.SplitPlan{
		Symbol:   req.Symbol,
		Side:     req.Side,
		TotalQty: req.TotalQty,
	}
	for _, leg := range req.Legs {
		plan.Legs = append(plan.Legs, types.SplitLeg{
			Provider: leg.Provider,
			Qty:      leg.Qty,
		})
	}
	result, err := s.router.ExecuteSplitPlan(ctx, plan, req.Accounts)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "execution failed: %v", err)
	}
	resp := &ExecutionResultResponse{
		PlanID:    result.PlanID,
		Symbol:    result.Symbol,
		Side:      result.Side,
		Status:    result.Status,
		FilledQty: result.FilledQty,
		VWAP:      result.VWAP,
		Latency:   result.Latency,
	}
	for _, leg := range result.Legs {
		resp.Legs = append(resp.Legs, &ExecutionLeg{
			Provider:  leg.Provider,
			OrderID:   leg.OrderID,
			Qty:       leg.Qty,
			FilledQty: leg.FilledQty,
			Price:     leg.Price,
			Status:    leg.Status,
			Latency:   leg.Latency,
		})
	}
	return resp, nil
}

// StartTWAP starts a TWAP execution.
func (s *Server) StartTWAP(ctx context.Context, req *StartTWAPRequest) (*TWAPResponse, error) {
	if s.twap == nil {
		return nil, status.Error(codes.Unimplemented, "TWAP scheduler not configured")
	}
	exec, err := s.twap.Start(ctx, router.TWAPConfig{
		Symbol:      req.Symbol,
		Side:        req.Side,
		TotalQty:    req.TotalQty,
		Duration:    time.Duration(req.DurationSeconds) * time.Second,
		Slices:      int(req.Slices),
		MaxSlippage: req.MaxSlippageBps,
	}, req.Accounts)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "twap start failed: %v", err)
	}
	return twapToResponse(exec), nil
}

// CancelTWAP cancels a running TWAP.
func (s *Server) CancelTWAP(ctx context.Context, req *CancelTWAPRequest) (*TWAPResponse, error) {
	if s.twap == nil {
		return nil, status.Error(codes.Unimplemented, "TWAP scheduler not configured")
	}
	if err := s.twap.Cancel(req.ExecutionID); err != nil {
		return nil, status.Errorf(codes.Internal, "cancel failed: %v", err)
	}
	exec, _ := s.twap.Get(req.ExecutionID)
	return twapToResponse(exec), nil
}

// GetTWAP returns the state of a TWAP execution.
func (s *Server) GetTWAP(ctx context.Context, req *GetTWAPRequest) (*TWAPResponse, error) {
	if s.twap == nil {
		return nil, status.Error(codes.Unimplemented, "TWAP scheduler not configured")
	}
	exec, err := s.twap.Get(req.ExecutionID)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return twapToResponse(exec), nil
}

// ScanArbitrage scans for cross-venue arbitrage opportunities.
func (s *Server) ScanArbitrage(ctx context.Context, req *ScanArbitrageRequest) (*ScanArbitrageResponse, error) {
	if s.arbDet == nil {
		return nil, status.Error(codes.Unimplemented, "arbitrage detector not configured")
	}
	var opps []*marketdata.ArbitrageOpportunity
	if req.Symbol != "" {
		opps = s.arbDet.CheckSymbol(req.Symbol)
	} else {
		opps = s.arbDet.Scan()
	}
	resp := &ScanArbitrageResponse{}
	for _, opp := range opps {
		resp.Opportunities = append(resp.Opportunities, &ArbitrageOpportunity{
			Symbol:     opp.Symbol,
			BuyVenue:   opp.BuyVenue,
			SellVenue:  opp.SellVenue,
			BuyPrice:   opp.BuyPrice,
			SellPrice:  opp.SellPrice,
			SpreadAbs:  opp.SpreadAbs,
			SpreadBps:  opp.SpreadBps,
			DetectedAt: opp.DetectedAt.Format(time.RFC3339),
		})
	}
	return resp, nil
}

// StreamQuotes streams consolidated BBO updates.
func (s *Server) StreamQuotes(req *StreamQuotesRequest, stream ggrpc.ServerStream) error {
	if len(req.Symbols) == 0 {
		return status.Error(codes.InvalidArgument, "at least one symbol is required")
	}

	// Subscribe to all requested symbols.
	type sub struct {
		ch    <-chan *marketdata.Ticker
		unsub func()
	}
	subs := make([]sub, len(req.Symbols))
	for i, sym := range req.Symbols {
		ch, unsub := s.feed.Subscribe(sym)
		subs[i] = sub{ch: ch, unsub: unsub}
	}
	defer func() {
		for _, s := range subs {
			s.unsub()
		}
	}()

	// Multiplex all subscription channels.
	ctx := stream.Context()
	for {
		for _, s := range subs {
			select {
			case <-ctx.Done():
				return nil
			case ticker, ok := <-s.ch:
				if !ok {
					continue
				}
				resp := &QuoteResponse{
					Symbol:      ticker.Symbol,
					BidPrice:    ticker.BestBid,
					AskPrice:    ticker.BestAsk,
					BidProvider: ticker.BestBidProvider,
					AskProvider: ticker.BestAskProvider,
					SpreadBps:   ticker.SpreadBps,
					Timestamp:   ticker.UpdatedAt.Format(time.RFC3339),
				}
				if err := stream.SendMsg(resp); err != nil {
					return err
				}
			default:
			}
		}
		// Yield to avoid busy-spin.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// ListProviders returns all registered provider names.
func (s *Server) ListProviders(ctx context.Context, req *ListProvidersRequest) (*ListProvidersResponse, error) {
	return &ListProvidersResponse{Providers: s.registry.List()}, nil
}

// HealthCheck returns service health.
func (s *Server) HealthCheck(ctx context.Context, req *HealthCheckRequest) (*HealthCheckResponse, error) {
	return &HealthCheckResponse{
		Status:    "ok",
		Providers: s.registry.List(),
	}, nil
}

// --- Helpers ---

func orderToResponse(o *types.Order) *OrderResponse {
	return &OrderResponse{
		ID:             o.ID,
		Provider:       o.Provider,
		ProviderID:     o.ProviderID,
		Symbol:         o.Symbol,
		Status:         o.Status,
		FilledQty:      o.FilledQty,
		FilledAvgPrice: o.FilledAvgPrice,
	}
}

func twapToResponse(exec *router.TWAPExecution) *TWAPResponse {
	resp := &TWAPResponse{
		ID:           exec.ID,
		Status:       exec.Status,
		SlicesFilled: int32(exec.SlicesFilled),
		TotalFilled:  exec.TotalFilled,
		VWAP:         exec.VWAP,
		StartedAt:    exec.StartedAt.Format(time.RFC3339),
	}
	if exec.CompletedAt != nil {
		resp.CompletedAt = exec.CompletedAt.Format(time.RFC3339)
	}
	return resp
}

// authInterceptor validates Bearer tokens from gRPC metadata via IAM JWKS.
// Health and reflection RPCs are excluded.
func authInterceptor(iamEndpoint string) ggrpc.UnaryServerInterceptor {
	jwksURL := iamEndpoint + "/.well-known/jwks"

	return func(ctx context.Context, req interface{}, info *ggrpc.UnaryServerInfo, handler ggrpc.UnaryHandler) (interface{}, error) {
		// Skip auth for health checks and reflection.
		if strings.HasPrefix(info.FullMethod, "/grpc.health.v1.") ||
			strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		vals := md.Get("authorization")
		if len(vals) == 0 {
			return nil, status.Error(codes.Unauthenticated, "authorization required")
		}
		token := vals[0]
		if !strings.HasPrefix(token, "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "bearer token required")
		}
		token = strings.TrimPrefix(token, "Bearer ")

		claims, err := auth.ValidateJWT(token, jwksURL)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		// Propagate identity via context metadata for downstream handlers.
		md.Set("x-user-id", auth.ClaimStr(claims, "sub"))
		md.Set("x-org-id", auth.ClaimStr(claims, "owner"))
		ctx = metadata.NewIncomingContext(ctx, md)

		return handler(ctx, req)
	}
}

func loggingInterceptor(ctx context.Context, req interface{}, info *ggrpc.UnaryServerInfo, handler ggrpc.UnaryHandler) (interface{}, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	_ = start // logging would go here via zerolog; kept minimal for now
	return resp, err
}

