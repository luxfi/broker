package alpaca

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/luxfi/broker/pkg/types"
)

// --- PortfolioAnalyzer implementation ---

func (p *Provider) GetPortfolioHistory(ctx context.Context, accountID string, params *types.HistoryParams) (*types.PortfolioHistory, error) {
	path := "/v1/trading/accounts/" + accountID + "/account/portfolio/history"
	sep := "?"
	if params != nil {
		if params.Period != "" {
			path += sep + "period=" + params.Period
			sep = "&"
		}
		if params.Timeframe != "" {
			path += sep + "timeframe=" + params.Timeframe
			sep = "&"
		}
		if params.DateEnd != "" {
			path += sep + "date_end=" + params.DateEnd
			sep = "&"
		}
		if params.ExtendedHours {
			path += sep + "extended_hours=true"
		}
	}

	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Timestamp     []int64   `json:"timestamp"`
		Equity        []float64 `json:"equity"`
		ProfitLoss    []float64 `json:"profit_loss"`
		ProfitLossPct []float64 `json:"profit_loss_pct"`
		BaseValue     float64   `json:"base_value"`
		Timeframe     string    `json:"timeframe"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	return &types.PortfolioHistory{
		Timestamp:     raw.Timestamp,
		Equity:        raw.Equity,
		ProfitLoss:    raw.ProfitLoss,
		ProfitLossPct: raw.ProfitLossPct,
		BaseValue:     raw.BaseValue,
		Timeframe:     raw.Timeframe,
	}, nil
}
