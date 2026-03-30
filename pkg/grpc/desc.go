package grpc

import (
	"context"

	ggrpc "google.golang.org/grpc"
)

// BrokerServiceServer is the service interface that gRPC v1.79+ requires
// for RegisterService. HandlerType must be an interface, not a concrete type.
type BrokerServiceServer interface {
	GetQuote(context.Context, *GetQuoteRequest) (*QuoteResponse, error)
	GetBBO(context.Context, *GetBBORequest) (*BBOResponse, error)
	GetRoutes(context.Context, *GetRoutesRequest) (*GetRoutesResponse, error)
	PlaceOrder(context.Context, *PlaceOrderRequest) (*OrderResponse, error)
	SmartOrder(context.Context, *SmartOrderRequest) (*OrderResponse, error)
	CancelOrder(context.Context, *CancelOrderRequest) (*CancelOrderResponse, error)
	GetSplitPlan(context.Context, *GetSplitPlanRequest) (*SplitPlanResponse, error)
	ExecuteSplit(context.Context, *ExecuteSplitRequest) (*ExecutionResultResponse, error)
	StartTWAP(context.Context, *StartTWAPRequest) (*TWAPResponse, error)
	CancelTWAP(context.Context, *CancelTWAPRequest) (*TWAPResponse, error)
	GetTWAP(context.Context, *GetTWAPRequest) (*TWAPResponse, error)
	ScanArbitrage(context.Context, *ScanArbitrageRequest) (*ScanArbitrageResponse, error)
	ListProviders(context.Context, *ListProvidersRequest) (*ListProvidersResponse, error)
	HealthCheck(context.Context, *HealthCheckRequest) (*HealthCheckResponse, error)
	StreamQuotes(*StreamQuotesRequest, ggrpc.ServerStream) error
}

// brokerServiceDesc is the gRPC ServiceDesc for the BrokerService.
// It registers unary handlers for all RPCs defined in proto/broker.proto.
// The StreamQuotes server-streaming RPC is registered as a stream handler.
//
// This will be replaced by protoc-generated registration once brokerpb/ is generated.
var brokerServiceDesc = ggrpc.ServiceDesc{
	ServiceName: "broker.v1.BrokerService",
	HandlerType: (*BrokerServiceServer)(nil),
	Methods: []ggrpc.MethodDesc{
		{
			MethodName: "GetQuote",
			Handler:    handleGetQuote,
		},
		{
			MethodName: "GetBBO",
			Handler:    handleGetBBO,
		},
		{
			MethodName: "GetRoutes",
			Handler:    handleGetRoutes,
		},
		{
			MethodName: "PlaceOrder",
			Handler:    handlePlaceOrder,
		},
		{
			MethodName: "SmartOrder",
			Handler:    handleSmartOrder,
		},
		{
			MethodName: "CancelOrder",
			Handler:    handleCancelOrder,
		},
		{
			MethodName: "GetSplitPlan",
			Handler:    handleGetSplitPlan,
		},
		{
			MethodName: "ExecuteSplit",
			Handler:    handleExecuteSplit,
		},
		{
			MethodName: "StartTWAP",
			Handler:    handleStartTWAP,
		},
		{
			MethodName: "CancelTWAP",
			Handler:    handleCancelTWAP,
		},
		{
			MethodName: "GetTWAP",
			Handler:    handleGetTWAP,
		},
		{
			MethodName: "ScanArbitrage",
			Handler:    handleScanArbitrage,
		},
		{
			MethodName: "ListProviders",
			Handler:    handleListProviders,
		},
		{
			MethodName: "HealthCheck",
			Handler:    handleHealthCheck,
		},
	},
	Streams: []ggrpc.StreamDesc{
		{
			StreamName:    "StreamQuotes",
			Handler:       handleStreamQuotes,
			ServerStreams:  true,
			ClientStreams:  false,
		},
	},
	Metadata: "proto/broker.proto",
}

// --- Unary handler adapters ---

func handleGetQuote(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(GetQuoteRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).GetQuote(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/GetQuote"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).GetQuote(ctx, req.(*GetQuoteRequest))
	})
}

func handleGetBBO(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(GetBBORequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).GetBBO(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/GetBBO"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).GetBBO(ctx, req.(*GetBBORequest))
	})
}

func handleGetRoutes(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(GetRoutesRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).GetRoutes(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/GetRoutes"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).GetRoutes(ctx, req.(*GetRoutesRequest))
	})
}

func handlePlaceOrder(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(PlaceOrderRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).PlaceOrder(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/PlaceOrder"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).PlaceOrder(ctx, req.(*PlaceOrderRequest))
	})
}

func handleSmartOrder(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(SmartOrderRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).SmartOrder(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/SmartOrder"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).SmartOrder(ctx, req.(*SmartOrderRequest))
	})
}

func handleCancelOrder(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(CancelOrderRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).CancelOrder(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/CancelOrder"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).CancelOrder(ctx, req.(*CancelOrderRequest))
	})
}

func handleGetSplitPlan(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(GetSplitPlanRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).GetSplitPlan(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/GetSplitPlan"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).GetSplitPlan(ctx, req.(*GetSplitPlanRequest))
	})
}

func handleExecuteSplit(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(ExecuteSplitRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).ExecuteSplit(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/ExecuteSplit"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).ExecuteSplit(ctx, req.(*ExecuteSplitRequest))
	})
}

func handleStartTWAP(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(StartTWAPRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).StartTWAP(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/StartTWAP"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).StartTWAP(ctx, req.(*StartTWAPRequest))
	})
}

func handleCancelTWAP(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(CancelTWAPRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).CancelTWAP(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/CancelTWAP"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).CancelTWAP(ctx, req.(*CancelTWAPRequest))
	})
}

func handleGetTWAP(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(GetTWAPRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).GetTWAP(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/GetTWAP"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).GetTWAP(ctx, req.(*GetTWAPRequest))
	})
}

func handleScanArbitrage(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(ScanArbitrageRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).ScanArbitrage(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/ScanArbitrage"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).ScanArbitrage(ctx, req.(*ScanArbitrageRequest))
	})
}

func handleListProviders(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(ListProvidersRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).ListProviders(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/ListProviders"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).ListProviders(ctx, req.(*ListProvidersRequest))
	})
}

func handleHealthCheck(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor ggrpc.UnaryServerInterceptor) (interface{}, error) {
	req := new(HealthCheckRequest)
	if err := dec(req); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(*Server).HealthCheck(ctx, req)
	}
	info := &ggrpc.UnaryServerInfo{Server: srv, FullMethod: "/broker.v1.BrokerService/HealthCheck"}
	return interceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(*Server).HealthCheck(ctx, req.(*HealthCheckRequest))
	})
}

// --- Stream handler ---

func handleStreamQuotes(srv interface{}, stream ggrpc.ServerStream) error {
	req := new(StreamQuotesRequest)
	if err := stream.RecvMsg(req); err != nil {
		return err
	}
	return srv.(*Server).StreamQuotes(req, stream)
}
