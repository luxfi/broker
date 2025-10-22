package alpaca

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/luxfi/broker/pkg/types"
)

// --- TransferExtended implementation ---

func (p *Provider) CancelTransfer(ctx context.Context, accountID, transferID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/accounts/"+accountID+"/transfers/"+transferID, nil)
	return err
}

func (p *Provider) DeleteACHRelationship(ctx context.Context, accountID, achID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/accounts/"+accountID+"/ach_relationships/"+achID, nil)
	return err
}

func (p *Provider) CreateRecipientBank(ctx context.Context, accountID string, req *types.CreateBankRequest) (*types.RecipientBank, error) {
	body := map[string]interface{}{
		"name":           req.Name,
		"bank_code":      req.BankCode,
		"bank_code_type": req.BankCodeType,
		"account_number": req.AccountNumber,
	}
	if req.Country != "" {
		body["country"] = req.Country
	}
	if req.StateProvince != "" {
		body["state_province"] = req.StateProvince
	}
	if req.PostalCode != "" {
		body["postal_code"] = req.PostalCode
	}
	if req.City != "" {
		body["city"] = req.City
	}
	if req.StreetAddress != "" {
		body["street_address"] = req.StreetAddress
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts/"+accountID+"/recipient_banks", body)
	if err != nil {
		return nil, err
	}
	return p.parseRecipientBank(data)
}

func (p *Provider) ListRecipientBanks(ctx context.Context, accountID string) ([]*types.RecipientBank, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+accountID+"/recipient_banks", nil)
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	banks := make([]*types.RecipientBank, 0, len(raw))
	for _, r := range raw {
		b, err := p.parseRecipientBank(r)
		if err != nil {
			continue
		}
		banks = append(banks, b)
	}
	return banks, nil
}

func (p *Provider) DeleteRecipientBank(ctx context.Context, accountID, bankID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/accounts/"+accountID+"/recipient_banks/"+bankID, nil)
	return err
}

func (p *Provider) parseRecipientBank(data []byte) (*types.RecipientBank, error) {
	var raw struct {
		ID            string `json:"id"`
		AccountID     string `json:"account_id"`
		Name          string `json:"name"`
		Status        string `json:"status"`
		Country       string `json:"country"`
		StateProvince string `json:"state_province"`
		PostalCode    string `json:"postal_code"`
		City          string `json:"city"`
		StreetAddress string `json:"street_address"`
		AccountNumber string `json:"account_number"`
		BankCode      string `json:"bank_code"`
		BankCodeType  string `json:"bank_code_type"`
		CreatedAt     string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.RecipientBank{
		ID:            raw.ID,
		AccountID:     raw.AccountID,
		Name:          raw.Name,
		Status:        raw.Status,
		Country:       raw.Country,
		StateProvince: raw.StateProvince,
		PostalCode:    raw.PostalCode,
		City:          raw.City,
		StreetAddress: raw.StreetAddress,
		AccountNumber: raw.AccountNumber,
		BankCode:      raw.BankCode,
		BankCodeType:  raw.BankCodeType,
		CreatedAt:     raw.CreatedAt,
	}, nil
}
