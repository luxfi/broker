package alpaca_omnisub

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

// CreateTransfer initiates a fund transfer on the omnibus master account.
// In the OmniSub model, ACH/wire flows through the omnibus — not the
// sub-account directly. The caller passes subAccountID for audit, but the
// actual transfer is booked on the omnibus master. Internal ledger
// credit/debit to the sub-account is handled via journals.
func (p *Provider) CreateTransfer(ctx context.Context, subAccountID string, req *types.CreateTransferRequest) (*types.Transfer, error) {
	body := map[string]interface{}{
		"transfer_type": req.Type,
		"amount":        req.Amount,
		"direction":     req.Direction,
	}
	if req.RelationshipID != "" {
		body["relationship_id"] = req.RelationshipID
	}

	// Transfers go through the omnibus master.
	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts/"+p.cfg.OmnibusAccountID+"/transfers", body)
	if err != nil {
		return nil, err
	}
	return p.parseTransfer(data)
}

// ListTransfers lists transfers on the omnibus master.
// Per the OmniSub model, all external fund movements are on the omnibus.
func (p *Provider) ListTransfers(ctx context.Context, _ string) ([]*types.Transfer, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+p.cfg.OmnibusAccountID+"/transfers", nil)
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

// CreateBankRelationship creates an ACH relationship on the omnibus master.
func (p *Provider) CreateBankRelationship(ctx context.Context, _ string, ownerName, accountType, accountNumber, routingNumber string) (*types.BankRelationship, error) {
	body := map[string]interface{}{
		"account_owner_name":  ownerName,
		"bank_account_type":   accountType,
		"bank_account_number": accountNumber,
		"bank_routing_number": routingNumber,
	}
	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts/"+p.cfg.OmnibusAccountID+"/ach_relationships", body)
	if err != nil {
		return nil, err
	}
	return p.parseBankRelationship(data)
}

// ListBankRelationships lists ACH relationships on the omnibus master.
func (p *Provider) ListBankRelationships(ctx context.Context, _ string) ([]*types.BankRelationship, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+p.cfg.OmnibusAccountID+"/ach_relationships", nil)
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
		Provider:   "alpaca_omnisub",
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
		Provider:         "alpaca_omnisub",
		ProviderID:       raw.ID,
		AccountID:        raw.AccountID,
		AccountOwnerName: raw.AccountOwnerName,
		BankAccountType:  raw.BankAccountType,
		Status:           raw.Status,
	}, nil
}
