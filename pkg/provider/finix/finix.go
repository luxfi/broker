package finix

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

const (
	SandboxURL = "https://finix.sandbox-payments-api.com"
	ProdURL    = "https://finix.live-payments-api.com"
)

// Config for Finix provider.
// Finix is a payments infrastructure platform (not a traditional broker).
// It handles payment processing, merchant onboarding, and fund transfers.
type Config struct {
	BaseURL  string `json:"base_url"`
	Username string `json:"username"` // API key
	Password string `json:"password"` // API secret
}

// Provider implements the broker Provider interface for Finix.
// Maps Finix's payment concepts to the unified broker interface:
// - Accounts → Finix Merchants/Identities
// - Transfers → Finix Transfers (debits/credits)
// - Bank Relationships → Finix Payment Instruments
type Provider struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = SandboxURL
	}
	return &Provider{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *Provider) Name() string { return "finix" }

func (p *Provider) do(ctx context.Context, method, path string, body interface{}) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth(p.cfg.Username, p.cfg.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Finix-Version", "2022-02-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("finix %d: %s", resp.StatusCode, string(data))
	}
	return data, resp.StatusCode, nil
}

// --- Accounts (Finix Identities → Merchants) ---

func (p *Provider) CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error) {
	// Create a Finix Identity (KYC entity)
	identity := map[string]interface{}{
		"entity": map[string]interface{}{
			"first_name":    req.Identity.GivenName,
			"last_name":     req.Identity.FamilyName,
			"email":         req.Contact.Email,
			"phone":         req.Contact.Phone,
			"personal_address": map[string]interface{}{
				"line1":       firstOrEmpty(req.Contact.Street),
				"city":        req.Contact.City,
				"region":      req.Contact.State,
				"postal_code": req.Contact.PostalCode,
				"country":     req.Contact.Country,
			},
		},
	}

	data, _, err := p.do(ctx, http.MethodPost, "/identities", identity)
	if err != nil {
		return nil, err
	}
	var resp struct {
		ID string `json:"id"`
	}
	json.Unmarshal(data, &resp)

	return &types.Account{
		Provider:   "finix",
		ProviderID: resp.ID,
		Status:     "active",
		Currency:   "USD",
		Identity: &types.Identity{
			GivenName:  req.Identity.GivenName,
			FamilyName: req.Identity.FamilyName,
		},
	}, nil
}

func (p *Provider) GetAccount(ctx context.Context, providerAccountID string) (*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/identities/"+providerAccountID, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		ID     string `json:"id"`
		Entity struct {
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			Email     string `json:"email"`
		} `json:"entity"`
	}
	json.Unmarshal(data, &resp)
	return &types.Account{
		Provider:   "finix",
		ProviderID: resp.ID,
		Status:     "active",
		Currency:   "USD",
		Identity: &types.Identity{
			GivenName:  resp.Entity.FirstName,
			FamilyName: resp.Entity.LastName,
		},
		Contact: &types.Contact{Email: resp.Entity.Email},
	}, nil
}

func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/identities", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Embedded struct {
			Identities []struct {
				ID     string `json:"id"`
				Entity struct {
					FirstName string `json:"first_name"`
					LastName  string `json:"last_name"`
				} `json:"entity"`
			} `json:"identities"`
		} `json:"_embedded"`
	}
	json.Unmarshal(data, &resp)

	accts := make([]*types.Account, len(resp.Embedded.Identities))
	for i, id := range resp.Embedded.Identities {
		accts[i] = &types.Account{
			Provider:      "finix",
			ProviderID:    id.ID,
			AccountNumber: id.Entity.FirstName + " " + id.Entity.LastName,
			Status:        "active",
			Currency:      "USD",
		}
	}
	return accts, nil
}

// --- Portfolio (balance of payment instruments) ---

func (p *Provider) GetPortfolio(_ context.Context, _ string) (*types.Portfolio, error) {
	return &types.Portfolio{Positions: []types.Position{}}, nil
}

// --- Orders (not applicable for payments) ---

func (p *Provider) CreateOrder(_ context.Context, _ string, _ *types.CreateOrderRequest) (*types.Order, error) {
	return nil, fmt.Errorf("finix: trading orders not supported — use transfers for payments")
}

func (p *Provider) ListOrders(_ context.Context, _ string) ([]*types.Order, error) {
	return []*types.Order{}, nil
}

func (p *Provider) GetOrder(_ context.Context, _, _ string) (*types.Order, error) {
	return nil, fmt.Errorf("finix: orders not supported")
}

func (p *Provider) CancelOrder(_ context.Context, _, _ string) error {
	return fmt.Errorf("finix: orders not supported")
}

// --- Transfers (Finix debits/credits) ---

func (p *Provider) CreateTransfer(ctx context.Context, providerAccountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	body := map[string]interface{}{
		"amount":   amountToCents(req.Amount),
		"currency": "USD",
		"source":   req.RelationshipID, // payment instrument ID
	}

	data, _, err := p.do(ctx, http.MethodPost, "/transfers", body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		ID     string `json:"id"`
		Amount int64  `json:"amount"`
		State  string `json:"state"`
	}
	json.Unmarshal(data, &resp)

	return &types.Transfer{
		Provider:   "finix",
		ProviderID: resp.ID,
		AccountID:  providerAccountID,
		Type:       "payment",
		Direction:  req.Direction,
		Amount:     fmt.Sprintf("%.2f", float64(resp.Amount)/100),
		Currency:   "USD",
		Status:     resp.State,
		CreatedAt:  time.Now(),
	}, nil
}

func (p *Provider) ListTransfers(ctx context.Context, providerAccountID string) ([]*types.Transfer, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/transfers", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Embedded struct {
			Transfers []struct {
				ID     string `json:"id"`
				Amount int64  `json:"amount"`
				State  string `json:"state"`
			} `json:"transfers"`
		} `json:"_embedded"`
	}
	json.Unmarshal(data, &resp)

	transfers := make([]*types.Transfer, len(resp.Embedded.Transfers))
	for i, t := range resp.Embedded.Transfers {
		transfers[i] = &types.Transfer{
			Provider:   "finix",
			ProviderID: t.ID,
			Type:       "payment",
			Amount:     fmt.Sprintf("%.2f", float64(t.Amount)/100),
			Currency:   "USD",
			Status:     t.State,
		}
	}
	return transfers, nil
}

// --- Bank Relationships (Finix Payment Instruments) ---

func (p *Provider) CreateBankRelationship(ctx context.Context, providerAccountID string, ownerName, accountType, accountNumber, routingNumber string) (*types.BankRelationship, error) {
	body := map[string]interface{}{
		"identity":       providerAccountID,
		"type":           "BANK_ACCOUNT",
		"account_type":   accountType,
		"name":           ownerName,
		"account_number": accountNumber,
		"bank_code":      routingNumber,
		"country":        "USA",
		"currency":       "USD",
	}

	data, _, err := p.do(ctx, http.MethodPost, "/payment_instruments", body)
	if err != nil {
		return nil, err
	}
	var resp struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		State string `json:"fingerprint"` // using fingerprint as proxy
	}
	json.Unmarshal(data, &resp)

	return &types.BankRelationship{
		Provider:         "finix",
		ProviderID:       resp.ID,
		AccountID:        providerAccountID,
		AccountOwnerName: ownerName,
		BankAccountType:  accountType,
		Status:           "active",
	}, nil
}

func (p *Provider) ListBankRelationships(ctx context.Context, providerAccountID string) ([]*types.BankRelationship, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/identities/"+providerAccountID+"/payment_instruments", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Embedded struct {
			Instruments []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				Name string `json:"name"`
			} `json:"payment_instruments"`
		} `json:"_embedded"`
	}
	json.Unmarshal(data, &resp)

	rels := make([]*types.BankRelationship, 0)
	for _, pi := range resp.Embedded.Instruments {
		if pi.Type == "BANK_ACCOUNT" {
			rels = append(rels, &types.BankRelationship{
				Provider:         "finix",
				ProviderID:       pi.ID,
				AccountID:        providerAccountID,
				AccountOwnerName: pi.Name,
				BankAccountType:  "BANK_ACCOUNT",
				Status:           "active",
			})
		}
	}
	return rels, nil
}

// --- Assets (not applicable) ---

func (p *Provider) ListAssets(_ context.Context, _ string) ([]*types.Asset, error) {
	return []*types.Asset{}, nil
}

func (p *Provider) GetAsset(_ context.Context, _ string) (*types.Asset, error) {
	return nil, fmt.Errorf("finix: assets not applicable for payment processing")
}

// --- Market Data ---

func (p *Provider) GetSnapshot(_ context.Context, _ string) (*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("finix: market data not applicable")
}

func (p *Provider) GetSnapshots(_ context.Context, _ []string) (map[string]*types.MarketSnapshot, error) {
	return nil, fmt.Errorf("finix: market data not applicable")
}

func (p *Provider) GetBars(_ context.Context, _, _, _, _ string, _ int) ([]*types.Bar, error) {
	return nil, fmt.Errorf("finix: market data not applicable")
}

func (p *Provider) GetLatestTrades(_ context.Context, _ []string) (map[string]*types.Trade, error) {
	return nil, fmt.Errorf("finix: market data not applicable")
}

func (p *Provider) GetLatestQuotes(_ context.Context, _ []string) (map[string]*types.Quote, error) {
	return nil, fmt.Errorf("finix: market data not applicable")
}

func (p *Provider) GetClock(_ context.Context) (*types.MarketClock, error) {
	return nil, fmt.Errorf("finix: clock not applicable")
}

func (p *Provider) GetCalendar(_ context.Context, _, _ string) ([]*types.MarketCalendarDay, error) {
	return nil, fmt.Errorf("finix: calendar not applicable")
}

// --- Helpers ---

func firstOrEmpty(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

func amountToCents(amount string) int64 {
	// Parse dollar amount to cents
	var f float64
	fmt.Sscanf(amount, "%f", &f)
	return int64(f * 100)
}
