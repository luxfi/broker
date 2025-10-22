package alpaca

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/luxfi/broker/pkg/types"
)

// --- JournalManager implementation ---

func (p *Provider) CreateJournal(ctx context.Context, req *types.CreateJournalRequest) (*types.Journal, error) {
	body := map[string]interface{}{
		"entry_type":   req.EntryType,
		"from_account": req.FromAccount,
		"to_account":   req.ToAccount,
	}
	if req.Amount != "" {
		body["amount"] = req.Amount
	}
	if req.Symbol != "" {
		body["symbol"] = req.Symbol
	}
	if req.Qty != "" {
		body["qty"] = req.Qty
	}
	if req.Description != "" {
		body["description"] = req.Description
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/journals", body)
	if err != nil {
		return nil, err
	}
	return p.parseJournal(data)
}

func (p *Provider) ListJournals(ctx context.Context, params *types.JournalParams) ([]*types.Journal, error) {
	path := "/v1/journals"
	sep := "?"
	if params != nil {
		if params.After != "" {
			path += sep + "after=" + params.After
			sep = "&"
		}
		if params.Before != "" {
			path += sep + "before=" + params.Before
			sep = "&"
		}
		if params.Status != "" {
			path += sep + "status=" + params.Status
			sep = "&"
		}
		if params.EntryType != "" {
			path += sep + "entry_type=" + params.EntryType
			sep = "&"
		}
		if params.ToAccount != "" {
			path += sep + "to_account=" + params.ToAccount
			sep = "&"
		}
		if params.FromAccount != "" {
			path += sep + "from_account=" + params.FromAccount
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
	journals := make([]*types.Journal, 0, len(raw))
	for _, r := range raw {
		j, err := p.parseJournal(r)
		if err != nil {
			continue
		}
		journals = append(journals, j)
	}
	return journals, nil
}

func (p *Provider) GetJournal(ctx context.Context, journalID string) (*types.Journal, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/journals/"+journalID, nil)
	if err != nil {
		return nil, err
	}
	return p.parseJournal(data)
}

func (p *Provider) DeleteJournal(ctx context.Context, journalID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/journals/"+journalID, nil)
	return err
}

func (p *Provider) CreateBatchJournal(ctx context.Context, req *types.BatchJournalRequest) ([]*types.Journal, error) {
	body := map[string]interface{}{
		"entry_type":   req.EntryType,
		"from_account": req.FromAccount,
		"entries":      req.Entries,
	}
	if req.Description != "" {
		body["description"] = req.Description
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/journals/batch", body)
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	journals := make([]*types.Journal, 0, len(raw))
	for _, r := range raw {
		j, err := p.parseJournal(r)
		if err != nil {
			continue
		}
		journals = append(journals, j)
	}
	return journals, nil
}

func (p *Provider) ReverseBatchJournal(ctx context.Context, req *types.ReverseBatchJournalRequest) ([]*types.Journal, error) {
	body := map[string]interface{}{
		"entry_type":   req.EntryType,
		"from_account": req.FromAccount,
		"entries":      req.Entries,
	}
	if req.Description != "" {
		body["description"] = req.Description
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/journals/reverse_batch", body)
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	journals := make([]*types.Journal, 0, len(raw))
	for _, r := range raw {
		j, err := p.parseJournal(r)
		if err != nil {
			continue
		}
		journals = append(journals, j)
	}
	return journals, nil
}

func (p *Provider) parseJournal(data []byte) (*types.Journal, error) {
	var raw struct {
		ID          string `json:"id"`
		EntryType   string `json:"entry_type"`
		FromAccount string `json:"from_account"`
		ToAccount   string `json:"to_account"`
		Symbol      string `json:"symbol"`
		Qty         string `json:"qty"`
		Price       string `json:"price"`
		Amount      string `json:"net_amount"`
		Status      string `json:"status"`
		Description string `json:"description"`
		SettleDate  string `json:"settle_date"`
		SystemDate  string `json:"system_date"`
		CreatedAt   string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return &types.Journal{
		ID:          raw.ID,
		EntryType:   raw.EntryType,
		FromAccount: raw.FromAccount,
		ToAccount:   raw.ToAccount,
		Symbol:      raw.Symbol,
		Qty:         raw.Qty,
		Price:       raw.Price,
		Amount:      raw.Amount,
		Status:      raw.Status,
		Description: raw.Description,
		SettleDate:  raw.SettleDate,
		SystemDate:  raw.SystemDate,
		CreatedAt:   raw.CreatedAt,
	}, nil
}
