package alpaca_omnisub

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/luxfi/broker/pkg/types"
)

// GetPortfolio returns account + positions for a sub-account.
func (p *Provider) GetPortfolio(ctx context.Context, subAccountID string) (*types.Portfolio, error) {
	tData, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+subAccountID+"/account", nil)
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

	pData, _, err := p.do(ctx, http.MethodGet, "/v1/trading/accounts/"+subAccountID+"/positions", nil)
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
		AccountID:      subAccountID,
		Cash:           ta.Cash,
		Equity:         ta.Equity,
		BuyingPower:    ta.BuyingPower,
		PortfolioValue: ta.PortfolioValue,
		Positions:      positions,
	}, nil
}
