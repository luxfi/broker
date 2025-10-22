package alpaca

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/luxfi/broker/pkg/types"
)

// --- WatchlistManager implementation ---

func (p *Provider) CreateWatchlist(ctx context.Context, accountID string, req *types.CreateWatchlistRequest) (*types.Watchlist, error) {
	body := map[string]interface{}{
		"name": req.Name,
	}
	if len(req.Symbols) > 0 {
		body["symbols"] = req.Symbols
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+accountID+"/watchlists", body)
	if err != nil {
		return nil, err
	}
	return p.parseWatchlist(data)
}

func (p *Provider) ListWatchlists(ctx context.Context, accountID string) ([]*types.Watchlist, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+accountID+"/watchlists", nil)
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	watchlists := make([]*types.Watchlist, 0, len(raw))
	for _, r := range raw {
		w, err := p.parseWatchlist(r)
		if err != nil {
			continue
		}
		watchlists = append(watchlists, w)
	}
	return watchlists, nil
}

func (p *Provider) GetWatchlist(ctx context.Context, accountID, watchlistID string) (*types.Watchlist, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+accountID+"/watchlists/"+watchlistID, nil)
	if err != nil {
		return nil, err
	}
	return p.parseWatchlist(data)
}

func (p *Provider) UpdateWatchlist(ctx context.Context, accountID, watchlistID string, req *types.UpdateWatchlistRequest) (*types.Watchlist, error) {
	body := make(map[string]interface{})
	if req.Name != "" {
		body["name"] = req.Name
	}
	if len(req.Symbols) > 0 {
		body["symbols"] = req.Symbols
	}

	data, _, err := p.do(ctx, http.MethodPut, "/v1/trading/accounts/"+accountID+"/watchlists/"+watchlistID, body)
	if err != nil {
		return nil, err
	}
	return p.parseWatchlist(data)
}

func (p *Provider) DeleteWatchlist(ctx context.Context, accountID, watchlistID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/trading/accounts/"+accountID+"/watchlists/"+watchlistID, nil)
	return err
}

func (p *Provider) AddWatchlistAsset(ctx context.Context, accountID, watchlistID, symbol string) (*types.Watchlist, error) {
	body := map[string]interface{}{
		"symbol": symbol,
	}
	data, _, err := p.do(ctx, http.MethodPost, "/v1/trading/accounts/"+accountID+"/watchlists/"+watchlistID, body)
	if err != nil {
		return nil, err
	}
	return p.parseWatchlist(data)
}

func (p *Provider) RemoveWatchlistAsset(ctx context.Context, accountID, watchlistID, symbol string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/trading/accounts/"+accountID+"/watchlists/"+watchlistID+"/"+symbol, nil)
	return err
}

func (p *Provider) parseWatchlist(data []byte) (*types.Watchlist, error) {
	var raw struct {
		ID        string `json:"id"`
		AccountID string `json:"account_id"`
		Name      string `json:"name"`
		Assets    []struct {
			ID     string `json:"id"`
			Symbol string `json:"symbol"`
			Name   string `json:"name"`
			Class  string `json:"class"`
		} `json:"assets"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	w := &types.Watchlist{
		ID:        raw.ID,
		AccountID: raw.AccountID,
		Name:      raw.Name,
		CreatedAt: raw.CreatedAt,
		UpdatedAt: raw.UpdatedAt,
	}
	for _, a := range raw.Assets {
		w.Assets = append(w.Assets, types.WatchlistAsset{
			ID:     a.ID,
			Symbol: a.Symbol,
			Name:   a.Name,
			Class:  a.Class,
		})
	}
	return w, nil
}
