package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/luxfi/broker/pkg/accounts"
	"github.com/luxfi/broker/pkg/admin"
	"github.com/luxfi/broker/pkg/audit"
	"github.com/luxfi/broker/pkg/auth"
	"github.com/luxfi/broker/pkg/funding"
	"github.com/luxfi/broker/pkg/marketdata"
	"github.com/luxfi/broker/pkg/provider"
	"github.com/luxfi/broker/pkg/risk"
	"github.com/luxfi/broker/pkg/router"
	"github.com/luxfi/broker/pkg/types"
	"github.com/luxfi/broker/pkg/ws"
)

type Server struct {
	registry   *provider.Registry
	resolver   *accounts.Resolver
	sor        *router.Router
	riskEng    *risk.Engine
	auditLog   *audit.Log
	authStore  *auth.Store
	adminStore *admin.Store
	feed       *marketdata.Feed
	stream     *ws.Server
	funding    *funding.Service
	router     chi.Router
	server     *http.Server
}

func NewServer(registry *provider.Registry, listenAddr string) *Server {
	jwtSecret := os.Getenv("ADMIN_SECRET")
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production"
	}

	s := &Server{
		registry:   registry,
		resolver:   accounts.NewResolver(),
		sor:        router.New(registry),
		riskEng:    risk.NewEngine(risk.DefaultLimits()),
		auditLog:   audit.NewLog(),
		authStore:  auth.NewStore(),
		adminStore: admin.NewStore(jwtSecret),
		feed:       marketdata.NewFeed(),
	}
	s.stream = ws.NewServer(s.feed)

	// Register API keys from environment
	if key := os.Getenv("BROKER_API_KEY"); key != "" {
		s.authStore.Add(&auth.APIKey{
			Key:         key,
			Name:        "default",
			OrgID:       os.Getenv("BROKER_ORG_ID"),
			Permissions: []string{"admin"},
			CreatedAt:   time.Now(),
		})
	}

	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"https://lux.financial", "https://app.lux.financial", "https://mpc.lux.network", "http://localhost:3000", "http://localhost:3001"},
		AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(auth.Middleware(s.authStore))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":    "ok",
			"providers": registry.List(),
		})
	})

	r.Route("/v1", func(r chi.Router) {
		r.Get("/providers", s.handleListProviders)
		r.Get("/providers/capabilities", s.handleGetCapabilities)

		r.Get("/accounts", s.handleListAccounts)
		r.Post("/accounts", s.handleCreateAccount)
		r.Get("/accounts/{provider}/{accountId}", s.handleGetAccount)
		r.Get("/accounts/{provider}/{accountId}/portfolio", s.handleGetPortfolio)

		r.Get("/accounts/{provider}/{accountId}/orders", s.handleListOrders)
		r.Post("/accounts/{provider}/{accountId}/orders", s.handleCreateOrder)
		r.Delete("/accounts/{provider}/{accountId}/orders", s.handleCancelAllOrders)
		r.Get("/accounts/{provider}/{accountId}/orders/{orderId}", s.handleGetOrder)
		r.Patch("/accounts/{provider}/{accountId}/orders/{orderId}", s.handleReplaceOrder)
		r.Delete("/accounts/{provider}/{accountId}/orders/{orderId}", s.handleCancelOrder)

		r.Get("/accounts/{provider}/{accountId}/positions/{symbol}", s.handleGetPosition)
		r.Delete("/accounts/{provider}/{accountId}/positions/{symbol}", s.handleClosePosition)
		r.Delete("/accounts/{provider}/{accountId}/positions", s.handleCloseAllPositions)

		r.Get("/accounts/{provider}/{accountId}/transfers", s.handleListTransfers)
		r.Post("/accounts/{provider}/{accountId}/transfers", s.handleCreateTransfer)

		r.Get("/accounts/{provider}/{accountId}/bank-relationships", s.handleListBankRelationships)
		r.Post("/accounts/{provider}/{accountId}/bank-relationships", s.handleCreateBankRelationship)

		r.Get("/assets/{provider}", s.handleListAssets)
		r.Get("/assets/{provider}/{symbolOrId}", s.handleGetAsset)

		// Smart Order Routing
		r.Get("/route/{symbol}", s.handleGetRoutes)
		r.Get("/route/{symbol}/{quote}", s.handleGetRoutesPair)
		r.Get("/route/{symbol}/split", s.handleGetSplitPlan)
		r.Get("/route/{symbol}/{quote}/split", s.handleGetSplitPlanPair)
		r.Get("/assets", s.handleAggregatedAssets)
		r.Post("/smart-order", s.handleSmartOrder)
		r.Post("/smart-order/split", s.handleExecuteSplit)

		// Market Data
		r.Get("/market/{provider}/snapshot/{symbol}", s.handleGetSnapshot)
		r.Get("/market/{provider}/snapshots", s.handleGetSnapshots)
		r.Get("/market/{provider}/bars/{symbol}", s.handleGetBars)
		r.Get("/market/{provider}/trades/latest", s.handleGetLatestTrades)
		r.Get("/market/{provider}/quotes/latest", s.handleGetLatestQuotes)
		r.Get("/market/{provider}/clock", s.handleGetClock)
		r.Get("/market/{provider}/calendar", s.handleGetCalendar)

		// Consolidated Market Data
		r.Get("/bbo/{symbol}", s.handleGetBBO)
		r.Get("/bbo/{symbol}/{quote}", s.handleGetBBOPair)
		r.Get("/stream", s.stream.HandleSSE)

		// Risk & Audit
		r.Get("/risk/check", s.handleRiskCheck)
		r.Get("/audit", s.handleAuditQuery)
		r.Get("/audit/stats", s.handleAuditStats)
		r.Get("/audit/export", s.handleAuditExport)

		// Funding (deposit/withdraw via payment processors)
		r.Route("/fund", func(r chi.Router) {
			r.Post("/deposit", s.handleDeposit)
			r.Post("/withdraw", s.handleWithdraw)
			r.Post("/webhook/{processor}", s.handleFundingWebhook)
			r.Get("/processors", s.handleListProcessors)
		})

		// Extended Account Management
		r.Patch("/accounts/{provider}/{accountId}", s.handleUpdateAccount)
		r.Delete("/accounts/{provider}/{accountId}", s.handleCloseAccount)
		r.Get("/accounts/{provider}/{accountId}/activities", s.handleGetAccountActivities)

		// Documents
		r.Post("/accounts/{provider}/{accountId}/documents", s.handleUploadDocument)
		r.Get("/accounts/{provider}/{accountId}/documents", s.handleListDocuments)
		r.Get("/accounts/{provider}/{accountId}/documents/{documentId}", s.handleGetDocument)
		r.Get("/accounts/{provider}/{accountId}/documents/{documentId}/download", s.handleDownloadDocument)

		// Journals (inter-account transfers)
		r.Post("/journals/{provider}", s.handleCreateJournal)
		r.Get("/journals/{provider}", s.handleListJournals)
		r.Get("/journals/{provider}/{journalId}", s.handleGetJournal)
		r.Delete("/journals/{provider}/{journalId}", s.handleDeleteJournal)
		r.Post("/journals/{provider}/batch", s.handleCreateBatchJournal)
		r.Post("/journals/{provider}/reverse_batch", s.handleReverseBatchJournal)

		// Transfer Extended (cancel, ACH delete, wire banks)
		r.Delete("/accounts/{provider}/{accountId}/transfers/{transferId}", s.handleCancelTransfer)
		r.Delete("/accounts/{provider}/{accountId}/ach-relationships/{achId}", s.handleDeleteACHRelationship)
		r.Post("/accounts/{provider}/{accountId}/recipient-banks", s.handleCreateRecipientBank)
		r.Get("/accounts/{provider}/{accountId}/recipient-banks", s.handleListRecipientBanks)
		r.Delete("/accounts/{provider}/{accountId}/recipient-banks/{bankId}", s.handleDeleteRecipientBank)

		// Crypto Market Data
		r.Get("/market/{provider}/crypto/bars", s.handleGetCryptoBars)
		r.Get("/market/{provider}/crypto/quotes", s.handleGetCryptoQuotes)
		r.Get("/market/{provider}/crypto/trades", s.handleGetCryptoTrades)
		r.Get("/market/{provider}/crypto/snapshots", s.handleGetCryptoSnapshots)

		// Portfolio History
		r.Get("/accounts/{provider}/{accountId}/portfolio/history", s.handleGetPortfolioHistory)

		// Watchlists
		r.Post("/accounts/{provider}/{accountId}/watchlists", s.handleCreateWatchlist)
		r.Get("/accounts/{provider}/{accountId}/watchlists", s.handleListWatchlists)
		r.Get("/accounts/{provider}/{accountId}/watchlists/{watchlistId}", s.handleGetWatchlist)
		r.Put("/accounts/{provider}/{accountId}/watchlists/{watchlistId}", s.handleUpdateWatchlist)
		r.Delete("/accounts/{provider}/{accountId}/watchlists/{watchlistId}", s.handleDeleteWatchlist)
		r.Post("/accounts/{provider}/{accountId}/watchlists/{watchlistId}/assets", s.handleAddWatchlistAsset)
		r.Delete("/accounts/{provider}/{accountId}/watchlists/{watchlistId}/{symbol}", s.handleRemoveWatchlistAsset)

		// Event Streaming (SSE)
		r.Get("/events/{provider}/trades", s.handleStreamTradeEvents)
		r.Get("/events/{provider}/accounts", s.handleStreamAccountEvents)
		r.Get("/events/{provider}/transfers", s.handleStreamTransferEvents)
		r.Get("/events/{provider}/journals", s.handleStreamJournalEvents)

		// Exchange frontend API (provider-agnostic, user-resolved)
		r.Route("/exchange", func(r chi.Router) {
			r.Get("/assets", s.handleFrontendAssets)
			r.Get("/crypto-prices", s.handleCryptoPrices)
			r.Get("/charts/{symbol}", s.handleChartData)
			r.Get("/orders", s.handleFrontendOrders)
			r.Post("/orders", s.handleFrontendCreateOrder)
			r.Get("/positions", s.handleFrontendPositions)
			r.Get("/portfolio", s.handleFrontendPortfolio)
		})
	})

	s.router = r
	s.server = &http.Server{
		Addr:         listenAddr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// SetFunding attaches a funding service for deposit/withdraw operations.
func (s *Server) SetFunding(f *funding.Service) {
	s.funding = f
}

// Handler returns the HTTP handler for testing.
func (s *Server) Handler() http.Handler {
	return s.router
}

// AdminStore returns the admin store for external configuration.
func (s *Server) AdminStore() *admin.Store {
	return s.adminStore
}

// AuthStore returns the auth store for external configuration.
func (s *Server) AuthStore() *auth.Store {
	return s.authStore
}

// Resolver returns the account resolver for user-to-provider account mapping.
func (s *Server) Resolver() *accounts.Resolver {
	return s.resolver
}

func (s *Server) getProvider(r *http.Request) (provider.Provider, error) {
	return s.registry.Get(chi.URLParam(r, "provider"))
}

// --- Handlers ---

func (s *Server) handleListProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"providers": s.registry.List()})
}

func (s *Server) handleGetCapabilities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.sor.GetCapabilities())
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		all := make([]*types.Account, 0)
		for _, name := range s.registry.List() {
			p, _ := s.registry.Get(name)
			accts, err := p.ListAccounts(r.Context())
			if err == nil {
				all = append(all, accts...)
			}
		}
		writeJSON(w, http.StatusOK, all)
		return
	}
	p, err := s.registry.Get(providerName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	accts, err := p.ListAccounts(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, accts)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider      string   `json:"provider"`
		GivenName     string   `json:"given_name"`
		FamilyName    string   `json:"family_name"`
		DateOfBirth   string   `json:"date_of_birth"`
		TaxID         string   `json:"tax_id"`
		TaxIDType     string   `json:"tax_id_type"`
		CountryOfTax  string   `json:"country_of_tax_residence"`
		Email         string   `json:"email"`
		Phone         string   `json:"phone"`
		Street        []string `json:"street"`
		City          string   `json:"city"`
		State         string   `json:"state"`
		PostalCode    string   `json:"postal_code"`
		Country       string   `json:"country"`
		EnabledAssets []string `json:"enabled_assets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Provider == "" {
		req.Provider = "alpaca"
	}

	p, err := s.registry.Get(req.Provider)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	acct, err := p.CreateAccount(r.Context(), &types.CreateAccountRequest{
		Provider: req.Provider,
		Identity: &types.Identity{
			GivenName: req.GivenName, FamilyName: req.FamilyName,
			DateOfBirth: req.DateOfBirth, TaxID: req.TaxID,
			TaxIDType: req.TaxIDType, CountryOfTax: req.CountryOfTax,
		},
		Contact: &types.Contact{
			Email: req.Email, Phone: req.Phone, Street: req.Street,
			City: req.City, State: req.State, PostalCode: req.PostalCode, Country: req.Country,
		},
		EnabledAssets: req.EnabledAssets,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.auditLog.Record(audit.Entry{
		Action:    audit.ActionAccountCreate,
		Provider:  req.Provider,
		AccountID: acct.ID,
		Status:    "success",
	})

	writeJSON(w, http.StatusCreated, acct)
}

func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	acct, err := p.GetAccount(r.Context(), chi.URLParam(r, "accountId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, acct)
}

func (s *Server) handleGetPortfolio(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	portfolio, err := p.GetPortfolio(r.Context(), chi.URLParam(r, "accountId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, portfolio)
}

func (s *Server) handleListOrders(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	accountID := chi.URLParam(r, "accountId")
	q := r.URL.Query()

	// If provider supports filtered listing and query params are provided, use it
	if ext, ok := p.(provider.TradingExtended); ok {
		params := &types.ListOrdersParams{
			Status:    q.Get("status"),
			After:     q.Get("after"),
			Until:     q.Get("until"),
			Direction: q.Get("direction"),
			Nested:    q.Get("nested") == "true",
		}
		if l := q.Get("limit"); l != "" {
			params.Limit, _ = strconv.Atoi(l)
		}
		orders, err := ext.ListOrdersFiltered(r.Context(), accountID, params)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, orders)
		return
	}

	orders, err := p.ListOrders(r.Context(), accountID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (s *Server) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	accountID := chi.URLParam(r, "accountId")

	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req types.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Pre-trade risk check
	riskResult := s.riskEng.PreTradeCheck(risk.CheckRequest{
		Provider:  providerName,
		AccountID: accountID,
		Symbol:    req.Symbol,
		Side:      req.Side,
		Qty:       req.Qty,
		OrderType: req.Type,
	})
	if !riskResult.Allowed {
		s.auditLog.Record(audit.Entry{
			Action:    audit.ActionOrderReject,
			Provider:  providerName,
			AccountID: accountID,
			Symbol:    req.Symbol,
			Side:      req.Side,
			Qty:       req.Qty,
			Status:    "rejected",
			Metadata:  map[string]interface{}{"risk_errors": riskResult.Errors},
		})
		writeJSON(w, http.StatusForbidden, map[string]interface{}{
			"error":    "risk check failed",
			"details":  riskResult.Errors,
			"warnings": riskResult.Warnings,
		})
		return
	}

	start := time.Now()
	order, err := p.CreateOrder(r.Context(), accountID, &req)
	latency := time.Since(start)

	status := "success"
	if err != nil {
		status = "failure"
	}

	s.auditLog.RecordOrder(r.Context(), audit.ActionOrderCreate,
		providerName, accountID, req.Symbol, req.Side, req.Qty,
		"", "", status, latency, err)

	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	// Estimate order value for risk tracking: qty * limit_price, or notional, or qty alone
	var estimatedValue float64
	if qty, err := strconv.ParseFloat(req.Qty, 64); err == nil && qty > 0 {
		if price, err := strconv.ParseFloat(req.LimitPrice, 64); err == nil && price > 0 {
			estimatedValue = qty * price
		} else if price, err := strconv.ParseFloat(req.StopPrice, 64); err == nil && price > 0 {
			estimatedValue = qty * price
		}
	}
	if estimatedValue == 0 {
		if notional, err := strconv.ParseFloat(req.Notional, 64); err == nil && notional > 0 {
			estimatedValue = notional
		}
	}
	s.riskEng.RecordOrder(providerName, accountID, estimatedValue)
	writeJSON(w, http.StatusCreated, order)
}

func (s *Server) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	order, err := p.GetOrder(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "orderId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	accountID := chi.URLParam(r, "accountId")
	orderID := chi.URLParam(r, "orderId")

	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := p.CancelOrder(r.Context(), accountID, orderID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.auditLog.Record(audit.Entry{
		Action:    audit.ActionOrderCancel,
		Provider:  providerName,
		AccountID: accountID,
		OrderID:   orderID,
		Status:    "success",
	})

	s.riskEng.RecordFill(providerName, accountID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handleReplaceOrder(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ext, ok := p.(provider.TradingExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support order replacement")
		return
	}
	accountID := chi.URLParam(r, "accountId")
	orderID := chi.URLParam(r, "orderId")
	var req types.ReplaceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	order, err := ext.ReplaceOrder(r.Context(), accountID, orderID, &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) handleCancelAllOrders(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ext, ok := p.(provider.TradingExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support cancel all orders")
		return
	}
	accountID := chi.URLParam(r, "accountId")
	if err := ext.CancelAllOrders(r.Context(), accountID); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func (s *Server) handleGetPosition(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ext, ok := p.(provider.TradingExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support get position")
		return
	}
	position, err := ext.GetPosition(r.Context(), chi.URLParam(r, "accountId"), chi.URLParam(r, "symbol"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, position)
}

func (s *Server) handleClosePosition(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ext, ok := p.(provider.TradingExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support close position")
		return
	}
	accountID := chi.URLParam(r, "accountId")
	symbol := chi.URLParam(r, "symbol")
	var qty *float64
	if qStr := r.URL.Query().Get("qty"); qStr != "" {
		if q, err := strconv.ParseFloat(qStr, 64); err == nil {
			qty = &q
		}
	}
	order, err := ext.ClosePosition(r.Context(), accountID, symbol, qty)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) handleCloseAllPositions(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ext, ok := p.(provider.TradingExtended)
	if !ok {
		writeError(w, http.StatusNotImplemented, "provider does not support close all positions")
		return
	}
	orders, err := ext.CloseAllPositions(r.Context(), chi.URLParam(r, "accountId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (s *Server) handleListTransfers(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	transfers, err := p.ListTransfers(r.Context(), chi.URLParam(r, "accountId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, transfers)
}

func (s *Server) handleCreateTransfer(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")
	accountID := chi.URLParam(r, "accountId")

	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req types.CreateTransferRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	transfer, err := p.CreateTransfer(r.Context(), accountID, &req)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.auditLog.Record(audit.Entry{
		Action:    audit.ActionTransfer,
		Provider:  providerName,
		AccountID: accountID,
		Status:    "success",
		Metadata:  map[string]interface{}{"type": req.Type, "direction": req.Direction, "amount": req.Amount},
	})

	writeJSON(w, http.StatusCreated, transfer)
}

func (s *Server) handleListBankRelationships(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rels, err := p.ListBankRelationships(r.Context(), chi.URLParam(r, "accountId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rels)
}

func (s *Server) handleCreateBankRelationship(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		AccountOwnerName  string `json:"account_owner_name"`
		BankAccountType   string `json:"bank_account_type"`
		BankAccountNumber string `json:"bank_account_number"`
		BankRoutingNumber string `json:"bank_routing_number"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rel, err := p.CreateBankRelationship(r.Context(), chi.URLParam(r, "accountId"),
		req.AccountOwnerName, req.BankAccountType, req.BankAccountNumber, req.BankRoutingNumber)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rel)
}

func (s *Server) handleListAssets(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	class := r.URL.Query().Get("class")
	assets, err := p.ListAssets(r.Context(), class)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if r.URL.Query().Get("all") == "" {
		tradable := make([]*types.Asset, 0, len(assets))
		for _, a := range assets {
			if a.Tradable {
				tradable = append(tradable, a)
			}
		}
		writeJSON(w, http.StatusOK, tradable)
		return
	}
	writeJSON(w, http.StatusOK, assets)
}

func (s *Server) handleGetAsset(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	asset, err := p.GetAsset(r.Context(), chi.URLParam(r, "symbolOrId"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

// --- Smart Order Routing Handlers ---

func (s *Server) handleGetRoutes(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	side := r.URL.Query().Get("side")
	if side == "" {
		side = "buy"
	}
	routes, err := s.sor.GetAllRoutes(r.Context(), symbol, side)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, routes)
}

func (s *Server) handleGetRoutesPair(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol") + "/" + chi.URLParam(r, "quote")
	side := r.URL.Query().Get("side")
	if side == "" {
		side = "buy"
	}
	routes, err := s.sor.GetAllRoutes(r.Context(), symbol, side)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, routes)
}

func (s *Server) handleGetSplitPlan(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	q := r.URL.Query()
	side := q.Get("side")
	if side == "" {
		side = "buy"
	}
	qty := q.Get("qty")
	if qty == "" {
		writeError(w, http.StatusBadRequest, "qty query param required")
		return
	}

	plan, err := s.sor.BuildSplitPlan(r.Context(), symbol, side, qty)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.auditLog.Record(audit.Entry{
		Action:    audit.ActionSplitPlan,
		Symbol:    symbol,
		Side:      side,
		Qty:       qty,
		Algorithm: plan.Algorithm,
		Status:    "success",
	})

	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleGetSplitPlanPair(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol") + "/" + chi.URLParam(r, "quote")
	q := r.URL.Query()
	side := q.Get("side")
	if side == "" {
		side = "buy"
	}
	qty := q.Get("qty")
	if qty == "" {
		writeError(w, http.StatusBadRequest, "qty query param required")
		return
	}

	plan, err := s.sor.BuildSplitPlan(r.Context(), symbol, side, qty)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) handleSmartOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Symbol      string            `json:"symbol"`
		Qty         string            `json:"qty"`
		Side        string            `json:"side"`
		Type        string            `json:"type"`
		TimeInForce string            `json:"time_in_force"`
		LimitPrice  string            `json:"limit_price,omitempty"`
		Accounts    map[string]string `json:"accounts"` // provider -> accountID
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Type == "" {
		req.Type = "market"
	}
	if req.TimeInForce == "" {
		req.TimeInForce = "day"
	}

	start := time.Now()
	order, err := s.sor.SmartOrder(r.Context(), req.Accounts, &types.CreateOrderRequest{
		Symbol:      req.Symbol,
		Qty:         req.Qty,
		Side:        req.Side,
		Type:        req.Type,
		TimeInForce: req.TimeInForce,
		LimitPrice:  req.LimitPrice,
	})
	latency := time.Since(start)

	status := "success"
	var providerUsed string
	if err != nil {
		status = "failure"
	} else {
		providerUsed = order.Provider
	}

	s.auditLog.RecordOrder(r.Context(), audit.ActionRouteDecision,
		providerUsed, "", req.Symbol, req.Side, req.Qty,
		"", "smart_route", status, latency, err)

	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (s *Server) handleExecuteSplit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Symbol   string            `json:"symbol"`
		Qty      string            `json:"qty"`
		Side     string            `json:"side"`
		Accounts map[string]string `json:"accounts"` // provider -> accountID
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	plan, err := s.sor.BuildSplitPlan(r.Context(), req.Symbol, req.Side, req.Qty)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	result, err := s.sor.ExecuteSplitPlan(r.Context(), plan, req.Accounts)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	s.auditLog.Record(audit.Entry{
		Action:    audit.ActionSplitExecute,
		Symbol:    req.Symbol,
		Side:      req.Side,
		Qty:       req.Qty,
		Algorithm: "split",
		Status:    result.Status,
		Metadata: map[string]interface{}{
			"plan_id":    result.PlanID,
			"vwap":       result.VWAP,
			"total_fees": result.TotalFees,
			"legs":       len(result.Legs),
		},
	})

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAggregatedAssets(w http.ResponseWriter, r *http.Request) {
	assets, err := s.sor.AggregatedAssets(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, assets)
}

// --- Market Data Handlers ---

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	snap, err := p.GetSnapshot(r.Context(), chi.URLParam(r, "symbol"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handleGetSnapshots(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	syms := r.URL.Query().Get("symbols")
	if syms == "" {
		writeError(w, http.StatusBadRequest, "symbols query param required")
		return
	}
	snaps, err := p.GetSnapshots(r.Context(), strings.Split(syms, ","))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snaps)
}

func (s *Server) handleGetBars(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	timeframe := q.Get("timeframe")
	if timeframe == "" {
		timeframe = "1Day"
	}
	limit := 0
	if l := q.Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}
	bars, err := p.GetBars(r.Context(), chi.URLParam(r, "symbol"), timeframe, q.Get("start"), q.Get("end"), limit)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, bars)
}

func (s *Server) handleGetLatestTrades(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	syms := r.URL.Query().Get("symbols")
	if syms == "" {
		writeError(w, http.StatusBadRequest, "symbols query param required")
		return
	}
	trades, err := p.GetLatestTrades(r.Context(), strings.Split(syms, ","))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, trades)
}

func (s *Server) handleGetLatestQuotes(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	syms := r.URL.Query().Get("symbols")
	if syms == "" {
		writeError(w, http.StatusBadRequest, "symbols query param required")
		return
	}
	quotes, err := p.GetLatestQuotes(r.Context(), strings.Split(syms, ","))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, quotes)
}

func (s *Server) handleGetClock(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	clock, err := p.GetClock(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, clock)
}

func (s *Server) handleGetCalendar(w http.ResponseWriter, r *http.Request) {
	p, err := s.getProvider(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	days, err := p.GetCalendar(r.Context(), q.Get("start"), q.Get("end"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, days)
}

// --- Consolidated BBO ---

func (s *Server) handleGetBBO(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	t, err := s.feed.GetTicker(symbol)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) handleGetBBOPair(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol") + "/" + chi.URLParam(r, "quote")
	t, err := s.feed.GetTicker(symbol)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// --- Risk & Audit ---

func (s *Server) handleRiskCheck(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result := s.riskEng.PreTradeCheck(risk.CheckRequest{
		Provider:  q.Get("provider"),
		AccountID: q.Get("account_id"),
		Symbol:    q.Get("symbol"),
		Side:      q.Get("side"),
		Qty:       q.Get("qty"),
		Price:     q.Get("price"),
		OrderType: q.Get("type"),
	})
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAuditQuery(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := audit.Filter{
		Action:    audit.Action(q.Get("action")),
		Provider:  q.Get("provider"),
		AccountID: q.Get("account_id"),
		Symbol:    q.Get("symbol"),
	}
	if since := q.Get("since"); since != "" {
		filter.Since, _ = time.Parse(time.RFC3339, since)
	}
	if until := q.Get("until"); until != "" {
		filter.Until, _ = time.Parse(time.RFC3339, until)
	}
	entries := s.auditLog.Query(filter)
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleAuditStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.auditLog.Stats())
}

func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	data, err := s.auditLog.Export()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", "attachment; filename=audit_export.json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
