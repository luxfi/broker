package alpaca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

const (
	SandboxURL     = "https://broker-api.sandbox.alpaca.markets"
	ProductionURL  = "https://broker-api.alpaca.markets"
	DataURL        = "https://data.alpaca.markets"
	DataSandboxURL = "https://data.sandbox.alpaca.markets"
)

// Config for the Alpaca provider.
type Config struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}

// Provider implements the broker Provider interface for Alpaca.
type Provider struct {
	cfg     Config
	client  *http.Client
	dataURL string
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxURL
	}
	dataURL := DataURL
	if strings.Contains(cfg.BaseURL, "sandbox") {
		dataURL = DataSandboxURL
	}
	return &Provider{
		cfg:     cfg,
		dataURL: dataURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (p *Provider) Name() string { return "alpaca" }

// --- HTTP helpers ---

func (p *Provider) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(p.cfg.APIKey, p.cfg.APISecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Message != "" {
			return nil, resp.StatusCode, fmt.Errorf("alpaca %d: %s", apiErr.Code, apiErr.Message)
		}
		return nil, resp.StatusCode, fmt.Errorf("alpaca %d: %s", resp.StatusCode, string(data))
	}

	return data, resp.StatusCode, nil
}

func decode[T any](data []byte) (*T, error) {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

func decodeSlice[T any](data []byte) ([]*T, error) {
	var v []T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	ptrs := make([]*T, len(v))
	for i := range v {
		ptrs[i] = &v[i]
	}
	return ptrs, nil
}

// --- Accounts ---

type alpacaAccount struct {
	ID            string `json:"id"`
	AccountNumber string `json:"account_number"`
	Status        string `json:"status"`
	Currency      string `json:"currency"`
	AccountType   string `json:"account_type"`
	CreatedAt     string `json:"created_at"`
	Contact       *struct {
		EmailAddress string   `json:"email_address"`
		PhoneNumber  string   `json:"phone_number"`
		StreetAddress []string `json:"street_address"`
		City         string   `json:"city"`
		State        string   `json:"state"`
		PostalCode   string   `json:"postal_code"`
		Country      string   `json:"country"`
	} `json:"contact"`
	Identity *struct {
		GivenName             string `json:"given_name"`
		FamilyName            string `json:"family_name"`
		DateOfBirth           string `json:"date_of_birth"`
		TaxIDType             string `json:"tax_id_type"`
		CountryOfTaxResidence string `json:"country_of_tax_residence"`
	} `json:"identity"`
	EnabledAssets []string `json:"enabled_assets"`
}

func (a *alpacaAccount) toUnified() *types.Account {
	acct := &types.Account{
		Provider:      "alpaca",
		ProviderID:    a.ID,
		AccountNumber: a.AccountNumber,
		Status:        a.Status,
		Currency:      a.Currency,
		AccountType:   a.AccountType,
		EnabledAssets: a.EnabledAssets,
	}
	if t, err := time.Parse(time.RFC3339Nano, a.CreatedAt); err == nil {
		acct.CreatedAt = t
	}
	if a.Identity != nil {
		acct.Identity = &types.Identity{
			GivenName:    a.Identity.GivenName,
			FamilyName:   a.Identity.FamilyName,
			DateOfBirth:  a.Identity.DateOfBirth,
			TaxIDType:    a.Identity.TaxIDType,
			CountryOfTax: a.Identity.CountryOfTaxResidence,
		}
	}
	if a.Contact != nil {
		acct.Contact = &types.Contact{
			Email:      a.Contact.EmailAddress,
			Phone:      a.Contact.PhoneNumber,
			Street:     a.Contact.StreetAddress,
			City:       a.Contact.City,
			State:      a.Contact.State,
			PostalCode: a.Contact.PostalCode,
			Country:    a.Contact.Country,
		}
	}
	return acct
}

func (p *Provider) CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error) {
	// Use request values with sensible defaults for compliance fields
	fundingSources := req.FundingSources
	if len(fundingSources) == 0 {
		fundingSources = []string{"employment_income"}
	}

	ipAddress := req.IPAddress
	if ipAddress == "" {
		ipAddress = "0.0.0.0"
	}

	// Disclosures: use request values if provided, default to false
	isControlPerson := false
	isAffiliatedExchangeFinra := false
	isPoliticallyExposed := false
	immediateFamilyExposed := false
	if req.Disclosures != nil {
		if req.Disclosures.IsControlPerson != nil {
			isControlPerson = *req.Disclosures.IsControlPerson
		}
		if req.Disclosures.IsAffiliatedExchangeFinra != nil {
			isAffiliatedExchangeFinra = *req.Disclosures.IsAffiliatedExchangeFinra
		}
		if req.Disclosures.IsPoliticallyExposed != nil {
			isPoliticallyExposed = *req.Disclosures.IsPoliticallyExposed
		}
		if req.Disclosures.ImmediateFamilyExposed != nil {
			immediateFamilyExposed = *req.Disclosures.ImmediateFamilyExposed
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	body := map[string]interface{}{
		"contact": map[string]interface{}{
			"email_address":  req.Contact.Email,
			"phone_number":   req.Contact.Phone,
			"street_address": req.Contact.Street,
			"city":           req.Contact.City,
			"state":          req.Contact.State,
			"postal_code":    req.Contact.PostalCode,
			"country":        req.Contact.Country,
		},
		"identity": map[string]interface{}{
			"given_name":                req.Identity.GivenName,
			"family_name":              req.Identity.FamilyName,
			"date_of_birth":            req.Identity.DateOfBirth,
			"tax_id":                   req.Identity.TaxID,
			"tax_id_type":              req.Identity.TaxIDType,
			"country_of_tax_residence": req.Identity.CountryOfTax,
			"funding_source":           fundingSources,
		},
		"disclosures": map[string]interface{}{
			"is_control_person":               isControlPerson,
			"is_affiliated_exchange_or_finra": isAffiliatedExchangeFinra,
			"is_politically_exposed":          isPoliticallyExposed,
			"immediate_family_exposed":        immediateFamilyExposed,
		},
		"agreements": []map[string]interface{}{
			{"agreement": "margin_agreement", "signed_at": now, "ip_address": ipAddress},
			{"agreement": "account_agreement", "signed_at": now, "ip_address": ipAddress},
			{"agreement": "customer_agreement", "signed_at": now, "ip_address": ipAddress},
			{"agreement": "crypto_agreement", "signed_at": now, "ip_address": ipAddress},
		},
	}
	if len(req.EnabledAssets) > 0 {
		body["enabled_assets"] = req.EnabledAssets
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts", body)
	if err != nil {
		return nil, err
	}
	raw, err := decode[alpacaAccount](data)
	if err != nil {
		return nil, err
	}
	return raw.toUnified(), nil
}

func (p *Provider) GetAccount(ctx context.Context, providerAccountID string) (*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+providerAccountID, nil)
	if err != nil {
		return nil, err
	}
	raw, err := decode[alpacaAccount](data)
	if err != nil {
		return nil, err
	}
	return raw.toUnified(), nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts", nil)
	if err != nil {
		return nil, err
	}
	var raw []alpacaAccount
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	accts := make([]*types.Account, len(raw))
	for i := range raw {
		accts[i] = raw[i].toUnified()
	}
	return accts, nil
}

// --- Portfolio & Positions ---

func (p *Provider) GetPortfolio(ctx context.Context, providerAccountID string) (*types.Portfolio, error) {
	// Fetch trading account
	tData, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+providerAccountID+"/account", nil)
	if err != nil {
		return nil, err
	}
	var ta struct {
		Cash           string `json:"cash"`
		Equity         string `json:"equity"`
		BuyingPower    string `json:"buying_power"`
		PortfolioValue string `json:"portfolio_value"`
	}
	if err := json.Unmarshal(tData, &ta); err != nil {
		return nil, err
	}

	// Fetch positions
	pData, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+providerAccountID+"/positions", nil)
	if err != nil {
		return nil, err
	}
	var rawPositions []struct {
		Symbol        string `json:"symbol"`
		Qty           string `json:"qty"`
		AvgEntryPrice string `json:"avg_entry_price"`
		MarketValue   string `json:"market_value"`
		CurrentPrice  string `json:"current_price"`
		UnrealizedPL  string `json:"unrealized_pl"`
		Side          string `json:"side"`
		AssetClass    string `json:"asset_class"`
	}
	if err := json.Unmarshal(pData, &rawPositions); err != nil {
		return nil, err
	}

	positions := make([]types.Position, len(rawPositions))
	for i, rp := range rawPositions {
		positions[i] = types.Position{
			Symbol:        rp.Symbol,
			Qty:           rp.Qty,
			AvgEntryPrice: rp.AvgEntryPrice,
			MarketValue:   rp.MarketValue,
			CurrentPrice:  rp.CurrentPrice,
			UnrealizedPL:  rp.UnrealizedPL,
			Side:          rp.Side,
			AssetClass:    rp.AssetClass,
		}
	}

	return &types.Portfolio{
		AccountID:      providerAccountID,
		Cash:           ta.Cash,
		Equity:         ta.Equity,
		BuyingPower:    ta.BuyingPower,
		PortfolioValue: ta.PortfolioValue,
		Positions:      positions,
	}, nil
}

// --- Orders ---

func (p *Provider) CreateOrder(ctx context.Context, providerAccountID string, req *types.CreateOrderRequest) (*types.Order, error) {
	body := map[string]interface{}{
		"symbol":        req.Symbol,
		"side":          req.Side,
		"type":          req.Type,
		"time_in_force": req.TimeInForce,
	}
	if req.Qty != "" {
		body["qty"] = req.Qty
	}
	if req.Notional != "" {
		body["notional"] = req.Notional
	}
	if req.LimitPrice != "" {
		body["limit_price"] = req.LimitPrice
	}
	if req.StopPrice != "" {
		body["stop_price"] = req.StopPrice
	}
	if req.ClientOrderID != "" {
		body["client_order_id"] = req.ClientOrderID
	}
	if req.TrailPrice != "" {
		body["trail_price"] = req.TrailPrice
	}
	if req.TrailPercent != "" {
		body["trail_percent"] = req.TrailPercent
	}
	if req.ExtendedHours {
		body["extended_hours"] = true
	}
	if req.OrderClass != "" {
		body["order_class"] = req.OrderClass
	}
	if req.TakeProfit != nil {
		body["take_profit"] = map[string]interface{}{
			"limit_price": req.TakeProfit.LimitPrice,
		}
	}
	if req.StopLoss != nil {
		sl := map[string]interface{}{
			"stop_price": req.StopLoss.StopPrice,
		}
		if req.StopLoss.LimitPrice != "" {
			sl["limit_price"] = req.StopLoss.LimitPrice
		}
		body["stop_loss"] = sl
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+providerAccountID+"/orders", body)
	if err != nil {
		return nil, err
	}
	return p.parseOrder(data)
}

func (p *Provider) ListOrders(ctx context.Context, providerAccountID string) ([]*types.Order, error) {
	return p.ListOrdersFiltered(ctx, providerAccountID, nil)
}

func (p *Provider) ListOrdersFiltered(ctx context.Context, providerAccountID string, params *types.ListOrdersParams) ([]*types.Order, error) {
	path := "/v1/trading/accounts/" + providerAccountID + "/orders"
	sep := "?"
	if params != nil {
		if params.Status != "" {
			path += sep + "status=" + params.Status
			sep = "&"
		}
		if params.Limit > 0 {
			path += sep + "limit=" + strconv.Itoa(params.Limit)
			sep = "&"
		}
		if params.After != "" {
			path += sep + "after=" + params.After
			sep = "&"
		}
		if params.Until != "" {
			path += sep + "until=" + params.Until
			sep = "&"
		}
		if params.Direction != "" {
			path += sep + "direction=" + params.Direction
			sep = "&"
		}
		if params.Nested {
			path += sep + "nested=true"
		}
	}
	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	orders := make([]*types.Order, 0, len(raw))
	for _, r := range raw {
		o, err := p.parseOrder(r)
		if err != nil {
			continue
		}
		orders = append(orders, o)
	}
	return orders, nil
}

func (p *Provider) GetOrder(ctx context.Context, providerAccountID, providerOrderID string) (*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+providerAccountID+"/orders/"+providerOrderID, nil)
	if err != nil {
		return nil, err
	}
	return p.parseOrder(data)
}

func (p *Provider) CancelOrder(ctx context.Context, providerAccountID, providerOrderID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/trading/accounts/"+providerAccountID+"/orders/"+providerOrderID, nil)
	return err
}

func (p *Provider) ReplaceOrder(ctx context.Context, providerAccountID, providerOrderID string, req *types.ReplaceOrderRequest) (*types.Order, error) {
	body := make(map[string]interface{})
	if req.Qty != nil {
		body["qty"] = fmt.Sprintf("%g", *req.Qty)
	}
	if req.TimeInForce != "" {
		body["time_in_force"] = req.TimeInForce
	}
	if req.LimitPrice != nil {
		body["limit_price"] = fmt.Sprintf("%g", *req.LimitPrice)
	}
	if req.StopPrice != nil {
		body["stop_price"] = fmt.Sprintf("%g", *req.StopPrice)
	}
	if req.TrailPrice != nil {
		body["trail"] = fmt.Sprintf("%g", *req.TrailPrice)
	}
	if req.TrailPercent != nil {
		body["trail_percent"] = fmt.Sprintf("%g", *req.TrailPercent)
	}
	if req.ClientOrderID != "" {
		body["client_order_id"] = req.ClientOrderID
	}
	data, _, err := p.do(ctx, http.MethodPatch, "/v1/trading/accounts/"+providerAccountID+"/orders/"+providerOrderID, body)
	if err != nil {
		return nil, err
	}
	return p.parseOrder(data)
}

func (p *Provider) CancelAllOrders(ctx context.Context, providerAccountID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/trading/accounts/"+providerAccountID+"/orders", nil)
	return err
}

func (p *Provider) GetPosition(ctx context.Context, providerAccountID, symbol string) (*types.Position, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+providerAccountID+"/positions/"+symbol, nil)
	if err != nil {
		return nil, err
	}
	return p.parsePosition(data)
}

func (p *Provider) ClosePosition(ctx context.Context, providerAccountID, symbol string, qty *float64) (*types.Order, error) {
	path := "/v1/trading/accounts/" + providerAccountID + "/positions/" + symbol
	if qty != nil {
		path += "?qty=" + fmt.Sprintf("%g", *qty)
	}
	data, _, err := p.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	return p.parseOrder(data)
}

func (p *Provider) CloseAllPositions(ctx context.Context, providerAccountID string) ([]*types.Order, error) {
	data, _, err := p.do(ctx, http.MethodDelete, "/v1/trading/accounts/"+providerAccountID+"/positions", nil)
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	orders := make([]*types.Order, 0, len(raw))
	for _, r := range raw {
		o, err := p.parseOrder(r)
		if err != nil {
			continue
		}
		orders = append(orders, o)
	}
	return orders, nil
}

func (p *Provider) parsePosition(data []byte) (*types.Position, error) {
	var raw struct {
		Symbol        string `json:"symbol"`
		Qty           string `json:"qty"`
		AvgEntryPrice string `json:"avg_entry_price"`
		MarketValue   string `json:"market_value"`
		CurrentPrice  string `json:"current_price"`
		UnrealizedPL  string `json:"unrealized_pl"`
		Side          string `json:"side"`
		AssetClass    string `json:"asset_class"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.Position{
		Symbol:        raw.Symbol,
		Qty:           raw.Qty,
		AvgEntryPrice: raw.AvgEntryPrice,
		MarketValue:   raw.MarketValue,
		CurrentPrice:  raw.CurrentPrice,
		UnrealizedPL:  raw.UnrealizedPL,
		Side:          raw.Side,
		AssetClass:    raw.AssetClass,
	}, nil
}

func (p *Provider) parseOrder(data []byte) (*types.Order, error) {
	var raw struct {
		ID             string  `json:"id"`
		Symbol         string  `json:"symbol"`
		Qty            string  `json:"qty"`
		Notional       string  `json:"notional"`
		Side           string  `json:"side"`
		Type           string  `json:"type"`
		TimeInForce    string  `json:"time_in_force"`
		LimitPrice     string  `json:"limit_price"`
		StopPrice      string  `json:"stop_price"`
		Status         string  `json:"status"`
		FilledQty      string  `json:"filled_qty"`
		FilledAvgPrice string  `json:"filled_avg_price"`
		AssetClass     string  `json:"asset_class"`
		CreatedAt      string  `json:"created_at"`
		FilledAt       *string `json:"filled_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	o := &types.Order{
		Provider:       "alpaca",
		ProviderID:     raw.ID,
		Symbol:         raw.Symbol,
		Qty:            raw.Qty,
		Notional:       raw.Notional,
		Side:           raw.Side,
		Type:           raw.Type,
		TimeInForce:    raw.TimeInForce,
		LimitPrice:     raw.LimitPrice,
		StopPrice:      raw.StopPrice,
		Status:         raw.Status,
		FilledQty:      raw.FilledQty,
		FilledAvgPrice: raw.FilledAvgPrice,
		AssetClass:     raw.AssetClass,
	}
	if t, err := time.Parse(time.RFC3339Nano, raw.CreatedAt); err == nil {
		o.CreatedAt = t
	}
	if raw.FilledAt != nil {
		if t, err := time.Parse(time.RFC3339Nano, *raw.FilledAt); err == nil {
			o.FilledAt = &t
		}
	}
	return o, nil
}

// --- Transfers ---

func (p *Provider) CreateTransfer(ctx context.Context, providerAccountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	body := map[string]interface{}{
		"transfer_type": req.Type,
		"amount":        req.Amount,
		"direction":     req.Direction,
	}
	if req.RelationshipID != "" {
		body["relationship_id"] = req.RelationshipID
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts/"+providerAccountID+"/transfers", body)
	if err != nil {
		return nil, err
	}
	return p.parseTransfer(data)
}

func (p *Provider) ListTransfers(ctx context.Context, providerAccountID string) ([]*types.Transfer, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+providerAccountID+"/transfers", nil)
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	transfers := make([]*types.Transfer, 0, len(raw))
	for _, r := range raw {
		t, err := p.parseTransfer(r)
		if err != nil {
			continue
		}
		transfers = append(transfers, t)
	}
	return transfers, nil
}

func (p *Provider) parseTransfer(data []byte) (*types.Transfer, error) {
	var raw struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
		Type      string `json:"type"`
		Status    string `json:"status"`
		Amount    string `json:"amount"`
		Direction string `json:"direction"`
		Currency  string `json:"currency"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	t := &types.Transfer{
		Provider:   "alpaca",
		ProviderID: raw.ID,
		AccountID:  raw.AccountID,
		Type:       raw.Type,
		Direction:  raw.Direction,
		Amount:     raw.Amount,
		Currency:   raw.Currency,
		Status:     raw.Status,
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw.CreatedAt); err == nil {
		t.CreatedAt = ts
	}
	if ts, err := time.Parse(time.RFC3339Nano, raw.UpdatedAt); err == nil {
		t.UpdatedAt = ts
	}
	return t, nil
}

// --- Bank Relationships ---

func (p *Provider) CreateBankRelationship(ctx context.Context, providerAccountID string, ownerName, accountType, accountNumber, routingNumber string) (*types.BankRelationship, error) {
	body := map[string]interface{}{
		"account_owner_name":  ownerName,
		"bank_account_type":   accountType,
		"bank_account_number": accountNumber,
		"bank_routing_number": routingNumber,
	}
	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts/"+providerAccountID+"/ach_relationships", body)
	if err != nil {
		return nil, err
	}
	return p.parseBankRelationship(data)
}

func (p *Provider) ListBankRelationships(ctx context.Context, providerAccountID string) ([]*types.BankRelationship, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+providerAccountID+"/ach_relationships", nil)
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	rels := make([]*types.BankRelationship, 0, len(raw))
	for _, r := range raw {
		br, err := p.parseBankRelationship(r)
		if err != nil {
			continue
		}
		rels = append(rels, br)
	}
	return rels, nil
}

func (p *Provider) parseBankRelationship(data []byte) (*types.BankRelationship, error) {
	var raw struct {
		ID               string `json:"id"`
		AccountID        string `json:"account_id"`
		AccountOwnerName string `json:"account_owner_name"`
		BankAccountType  string `json:"bank_account_type"`
		Status           string `json:"status"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.BankRelationship{
		Provider:         "alpaca",
		ProviderID:       raw.ID,
		AccountID:        raw.AccountID,
		AccountOwnerName: raw.AccountOwnerName,
		BankAccountType:  raw.BankAccountType,
		Status:           raw.Status,
	}, nil
}

// --- Assets ---

func (p *Provider) ListAssets(ctx context.Context, class string) ([]*types.Asset, error) {
	path := "/v1/assets?status=active"
	if class != "" {
		path += "&asset_class=" + class
	}
	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID           string `json:"id"`
		Symbol       string `json:"symbol"`
		Name         string `json:"name"`
		Class        string `json:"class"`
		Exchange     string `json:"exchange"`
		Status       string `json:"status"`
		Tradable     bool   `json:"tradable"`
		Fractionable bool   `json:"fractionable"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	assets := make([]*types.Asset, len(raw))
	for i, r := range raw {
		assets[i] = &types.Asset{
			ID:           r.ID,
			Provider:     "alpaca",
			Symbol:       r.Symbol,
			Name:         r.Name,
			Class:        r.Class,
			Exchange:     r.Exchange,
			Status:       r.Status,
			Tradable:     r.Tradable,
			Fractionable: r.Fractionable,
		}
	}
	return assets, nil
}

func (p *Provider) GetAsset(ctx context.Context, symbolOrID string) (*types.Asset, error) {
	// Alpaca uses no-slash symbols for API calls (BTC/USD → BTCUSD)
	apiSymbol := strings.ReplaceAll(symbolOrID, "/", "")
	data, _, err := p.do(ctx, http.MethodGet, "/v1/assets/"+apiSymbol, nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		ID           string `json:"id"`
		Symbol       string `json:"symbol"`
		Name         string `json:"name"`
		Class        string `json:"class"`
		Exchange     string `json:"exchange"`
		Status       string `json:"status"`
		Tradable     bool   `json:"tradable"`
		Fractionable bool   `json:"fractionable"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.Asset{
		ID:           raw.ID,
		Provider:     "alpaca",
		Symbol:       raw.Symbol,
		Name:         raw.Name,
		Class:        raw.Class,
		Exchange:     raw.Exchange,
		Status:       raw.Status,
		Tradable:     raw.Tradable,
		Fractionable: raw.Fractionable,
	}, nil
}

// --- Market Data (uses data API, sandbox-aware) ---

// isCryptoSymbol returns true for crypto pairs (e.g. "BTC/USD").
func isCryptoSymbol(symbol string) bool {
	return strings.Contains(symbol, "/")
}

// stocksOrCryptoPath returns the correct base path for market data requests.
func stocksOrCryptoPath(symbol string) string {
	if isCryptoSymbol(symbol) {
		return "/v1beta3/crypto/us"
	}
	return "/v2/stocks"
}

func (p *Provider) doData(ctx context.Context, method, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.dataURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(p.cfg.APIKey, p.cfg.APISecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("alpaca data %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

func (p *Provider) GetSnapshot(ctx context.Context, symbol string) (*types.MarketSnapshot, error) {
	if isCryptoSymbol(symbol) {
		// Alpaca sandbox doesn't support the single crypto snapshot endpoint
		// (/v1beta3/crypto/us/{sym}/snapshot returns 404). Use the batch endpoint instead.
		path := "/v1beta3/crypto/us/snapshots?symbols=" + symbol
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var wrapper struct {
			Snapshots map[string]json.RawMessage `json:"snapshots"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, fmt.Errorf("parse crypto snapshots: %w", err)
		}
		raw, ok := wrapper.Snapshots[symbol]
		if !ok {
			return nil, fmt.Errorf("crypto snapshot not found for %s", symbol)
		}
		return p.parseSnapshot(symbol, raw)
	}
	// Stock: single snapshot endpoint works
	data, _, err := p.doData(ctx, http.MethodGet, "/v2/stocks/"+symbol+"/snapshot")
	if err != nil {
		return nil, err
	}
	return p.parseSnapshot(symbol, data)
}

func (p *Provider) GetSnapshots(ctx context.Context, symbols []string) (map[string]*types.MarketSnapshot, error) {
	// Split symbols into stock and crypto groups.
	var stocks, cryptos []string
	for _, s := range symbols {
		if isCryptoSymbol(s) {
			cryptos = append(cryptos, s)
		} else {
			stocks = append(stocks, s)
		}
	}

	result := make(map[string]*types.MarketSnapshot)

	if len(stocks) > 0 {
		path := "/v2/stocks/snapshots?symbols=" + strings.Join(stocks, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, d := range raw {
			snap, err := p.parseSnapshot(sym, d)
			if err != nil {
				continue
			}
			result[sym] = snap
		}
	}

	if len(cryptos) > 0 {
		path := "/v1beta3/crypto/us/snapshots?symbols=" + strings.Join(cryptos, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var wrapper struct {
			Snapshots map[string]json.RawMessage `json:"snapshots"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			// Try direct map (API format varies)
			var raw map[string]json.RawMessage
			if err2 := json.Unmarshal(data, &raw); err2 != nil {
				return nil, err
			}
			for sym, d := range raw {
				snap, err := p.parseSnapshot(sym, d)
				if err != nil {
					continue
				}
				result[sym] = snap
			}
		} else {
			for sym, d := range wrapper.Snapshots {
				snap, err := p.parseSnapshot(sym, d)
				if err != nil {
					continue
				}
				result[sym] = snap
			}
		}
	}

	return result, nil
}

func (p *Provider) parseSnapshot(symbol string, data []byte) (*types.MarketSnapshot, error) {
	var raw struct {
		LatestTrade *struct {
			T string  `json:"t"`
			P float64 `json:"p"`
			S float64 `json:"s"`
			X string  `json:"x"`
		} `json:"latestTrade"`
		LatestQuote *struct {
			T  string  `json:"t"`
			BP float64 `json:"bp"`
			BS float64 `json:"bs"`
			AP float64 `json:"ap"`
			AS float64 `json:"as"`
		} `json:"latestQuote"`
		MinuteBar *struct {
			T  string  `json:"t"`
			O  float64 `json:"o"`
			H  float64 `json:"h"`
			L  float64 `json:"l"`
			C  float64 `json:"c"`
			V  float64 `json:"v"`
			VW float64 `json:"vw"`
			N  int     `json:"n"`
		} `json:"minuteBar"`
		DailyBar *struct {
			T  string  `json:"t"`
			O  float64 `json:"o"`
			H  float64 `json:"h"`
			L  float64 `json:"l"`
			C  float64 `json:"c"`
			V  float64 `json:"v"`
			VW float64 `json:"vw"`
			N  int     `json:"n"`
		} `json:"dailyBar"`
		PrevDailyBar *struct {
			T  string  `json:"t"`
			O  float64 `json:"o"`
			H  float64 `json:"h"`
			L  float64 `json:"l"`
			C  float64 `json:"c"`
			V  float64 `json:"v"`
			VW float64 `json:"vw"`
			N  int     `json:"n"`
		} `json:"prevDailyBar"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	snap := &types.MarketSnapshot{Symbol: symbol}
	if raw.LatestTrade != nil {
		snap.LatestTrade = &types.Trade{Timestamp: raw.LatestTrade.T, Price: raw.LatestTrade.P, Size: raw.LatestTrade.S, Exchange: raw.LatestTrade.X}
	}
	if raw.LatestQuote != nil {
		snap.LatestQuote = &types.Quote{Timestamp: raw.LatestQuote.T, BidPrice: raw.LatestQuote.BP, BidSize: raw.LatestQuote.BS, AskPrice: raw.LatestQuote.AP, AskSize: raw.LatestQuote.AS}
	}
	if raw.MinuteBar != nil {
		snap.MinuteBar = &types.Bar{Timestamp: raw.MinuteBar.T, Open: raw.MinuteBar.O, High: raw.MinuteBar.H, Low: raw.MinuteBar.L, Close: raw.MinuteBar.C, Volume: raw.MinuteBar.V, VWAP: raw.MinuteBar.VW, TradeCount: raw.MinuteBar.N}
	}
	if raw.DailyBar != nil {
		snap.DailyBar = &types.Bar{Timestamp: raw.DailyBar.T, Open: raw.DailyBar.O, High: raw.DailyBar.H, Low: raw.DailyBar.L, Close: raw.DailyBar.C, Volume: raw.DailyBar.V, VWAP: raw.DailyBar.VW, TradeCount: raw.DailyBar.N}
	}
	if raw.PrevDailyBar != nil {
		snap.PrevDailyBar = &types.Bar{Timestamp: raw.PrevDailyBar.T, Open: raw.PrevDailyBar.O, High: raw.PrevDailyBar.H, Low: raw.PrevDailyBar.L, Close: raw.PrevDailyBar.C, Volume: raw.PrevDailyBar.V, VWAP: raw.PrevDailyBar.VW, TradeCount: raw.PrevDailyBar.N}
	}
	return snap, nil
}

func (p *Provider) GetBars(ctx context.Context, symbol, timeframe, start, end string, limit int) ([]*types.Bar, error) {
	basePath := stocksOrCryptoPath(symbol)

	var allBars []*types.Bar
	nextPageToken := ""
	maxPages := 100 // safety valve

	for page := 0; page < maxPages; page++ {
		path := basePath + "/" + symbol + "/bars?timeframe=" + timeframe
		if start != "" {
			path += "&start=" + start
		}
		if end != "" {
			path += "&end=" + end
		}
		if limit > 0 {
			path += "&limit=" + strconv.Itoa(limit)
		}
		if nextPageToken != "" {
			path += "&page_token=" + nextPageToken
		}

		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}

		var raw struct {
			Bars []struct {
				T  string  `json:"t"`
				O  float64 `json:"o"`
				H  float64 `json:"h"`
				L  float64 `json:"l"`
				C  float64 `json:"c"`
				V  float64 `json:"v"`
				VW float64 `json:"vw"`
				N  int     `json:"n"`
			} `json:"bars"`
			NextPageToken string `json:"next_page_token"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}

		for _, b := range raw.Bars {
			allBars = append(allBars, &types.Bar{
				Timestamp:  b.T,
				Open:       b.O,
				High:       b.H,
				Low:        b.L,
				Close:      b.C,
				Volume:     b.V,
				VWAP:       b.VW,
				TradeCount: b.N,
			})
		}

		if raw.NextPageToken == "" {
			break
		}
		// If caller specified a limit and we have enough, stop.
		if limit > 0 && len(allBars) >= limit {
			allBars = allBars[:limit]
			break
		}
		nextPageToken = raw.NextPageToken
	}

	return allBars, nil
}

func (p *Provider) GetLatestTrades(ctx context.Context, symbols []string) (map[string]*types.Trade, error) {
	result := make(map[string]*types.Trade)
	// Split by asset class
	var stocks, cryptos []string
	for _, s := range symbols {
		if isCryptoSymbol(s) {
			cryptos = append(cryptos, s)
		} else {
			stocks = append(stocks, s)
		}
	}
	if len(stocks) > 0 {
		path := "/v2/stocks/trades/latest?symbols=" + strings.Join(stocks, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw struct {
			Trades map[string]struct {
				T string  `json:"t"`
				P float64 `json:"p"`
				S float64 `json:"s"`
				X string  `json:"x"`
			} `json:"trades"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, t := range raw.Trades {
			result[sym] = &types.Trade{Timestamp: t.T, Price: t.P, Size: t.S, Exchange: t.X}
		}
	}
	if len(cryptos) > 0 {
		path := "/v1beta3/crypto/us/latest/trades?symbols=" + strings.Join(cryptos, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw struct {
			Trades map[string]struct {
				T string  `json:"t"`
				P float64 `json:"p"`
				S float64 `json:"s"`
			} `json:"trades"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, t := range raw.Trades {
			result[sym] = &types.Trade{Timestamp: t.T, Price: t.P, Size: t.S}
		}
	}
	return result, nil
}

func (p *Provider) GetLatestQuotes(ctx context.Context, symbols []string) (map[string]*types.Quote, error) {
	result := make(map[string]*types.Quote)
	var stocks, cryptos []string
	for _, s := range symbols {
		if isCryptoSymbol(s) {
			cryptos = append(cryptos, s)
		} else {
			stocks = append(stocks, s)
		}
	}
	if len(stocks) > 0 {
		path := "/v2/stocks/quotes/latest?symbols=" + strings.Join(stocks, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw struct {
			Quotes map[string]struct {
				T  string  `json:"t"`
				BP float64 `json:"bp"`
				BS float64 `json:"bs"`
				AP float64 `json:"ap"`
				AS float64 `json:"as"`
			} `json:"quotes"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, q := range raw.Quotes {
			result[sym] = &types.Quote{Timestamp: q.T, BidPrice: q.BP, BidSize: q.BS, AskPrice: q.AP, AskSize: q.AS}
		}
	}
	if len(cryptos) > 0 {
		path := "/v1beta3/crypto/us/latest/quotes?symbols=" + strings.Join(cryptos, ",")
		data, _, err := p.doData(ctx, http.MethodGet, path)
		if err != nil {
			return nil, err
		}
		var raw struct {
			Quotes map[string]struct {
				T  string  `json:"t"`
				BP float64 `json:"bp"`
				BS float64 `json:"bs"`
				AP float64 `json:"ap"`
				AS float64 `json:"as"`
			} `json:"quotes"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		for sym, q := range raw.Quotes {
			result[sym] = &types.Quote{Timestamp: q.T, BidPrice: q.BP, BidSize: q.BS, AskPrice: q.AP, AskSize: q.AS}
		}
	}
	return result, nil
}

func (p *Provider) GetClock(ctx context.Context) (*types.MarketClock, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/clock", nil)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Timestamp string `json:"timestamp"`
		IsOpen    bool   `json:"is_open"`
		NextOpen  string `json:"next_open"`
		NextClose string `json:"next_close"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.MarketClock{
		Timestamp: raw.Timestamp,
		IsOpen:    raw.IsOpen,
		NextOpen:  raw.NextOpen,
		NextClose: raw.NextClose,
	}, nil
}

func (p *Provider) GetCalendar(ctx context.Context, start, end string) ([]*types.MarketCalendarDay, error) {
	path := "/v1/calendar"
	sep := "?"
	if start != "" {
		path += sep + "start=" + start
		sep = "&"
	}
	if end != "" {
		path += sep + "end=" + end
	}
	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		Date  string `json:"date"`
		Open  string `json:"open"`
		Close string `json:"close"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	days := make([]*types.MarketCalendarDay, len(raw))
	for i, d := range raw {
		days[i] = &types.MarketCalendarDay{Date: d.Date, Open: d.Open, Close: d.Close}
	}
	return days, nil
}
