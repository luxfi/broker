package alpaca_omnisub

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/luxfi/broker/pkg/types"
)

// OmnibusSnapshot is the end-of-day reconciliation snapshot for the
// omnibus master account. It includes the aggregate cash, equity,
// buying power, and all positions held at the omnibus level.
type OmnibusSnapshot struct {
	AccountID      string           `json:"account_id"`
	Cash           string           `json:"cash"`
	Equity         string           `json:"equity"`
	BuyingPower    string           `json:"buying_power"`
	PortfolioValue string           `json:"portfolio_value"`
	Positions      []types.Position `json:"positions"`
}

// GetOmnibusSnapshot returns the current state of the omnibus master
// for end-of-day reconciliation. This is the aggregate across all
// sub-accounts and should match the sum of per-sub positions + cash.
func (p *Provider) GetOmnibusSnapshot(ctx context.Context) (*OmnibusSnapshot, error) {
	tData, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+p.cfg.OmnibusAccountID+"/account", nil)
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

	pData, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+p.cfg.OmnibusAccountID+"/positions", nil)
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

	return &OmnibusSnapshot{
		AccountID:      p.cfg.OmnibusAccountID,
		Cash:           ta.Cash,
		Equity:         ta.Equity,
		BuyingPower:    ta.BuyingPower,
		PortfolioValue: ta.PortfolioValue,
		Positions:      positions,
	}, nil
}
