package alpaca

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/luxfi/broker/pkg/types"
)

// --- EventStreamer implementation ---
// Alpaca Broker API provides SSE endpoints at /v1/events/{type}.

func (p *Provider) StreamTradeEvents(ctx context.Context, since string) (<-chan *types.TradeEvent, error) {
	path := "/v1/events/trades/account"
	if since != "" {
		path += "?since=" + since
	}
	ch := make(chan *types.TradeEvent, 64)
	err := p.streamSSE(ctx, path, func(data []byte) {
		var raw struct {
			EventType string `json:"event"`
			EventID   string `json:"event_id"`
			AccountID string `json:"account_id"`
			Order     *struct {
				ID             string  `json:"id"`
				Symbol         string  `json:"symbol"`
				Side           string  `json:"side"`
				Type           string  `json:"type"`
				Qty            string  `json:"qty"`
				FilledQty      string  `json:"filled_qty"`
				FilledAvgPrice string  `json:"filled_avg_price"`
				Status         string  `json:"status"`
				CreatedAt      string  `json:"created_at"`
			} `json:"order"`
			Timestamp string `json:"timestamp"`
		}
		if json.Unmarshal(data, &raw) != nil {
			return
		}
		evt := &types.TradeEvent{
			EventType: raw.EventType,
			EventID:   raw.EventID,
			AccountID: raw.AccountID,
			Timestamp: raw.Timestamp,
		}
		if raw.Order != nil {
			evt.Order = &types.Order{
				Provider:       "alpaca",
				ProviderID:     raw.Order.ID,
				Symbol:         raw.Order.Symbol,
				Side:           raw.Order.Side,
				Type:           raw.Order.Type,
				Qty:            raw.Order.Qty,
				FilledQty:      raw.Order.FilledQty,
				FilledAvgPrice: raw.Order.FilledAvgPrice,
				Status:         raw.Order.Status,
			}
		}
		select {
		case ch <- evt:
		default:
		}
	})
	if err != nil {
		close(ch)
		return nil, err
	}
	return ch, nil
}

func (p *Provider) StreamAccountEvents(ctx context.Context, since string) (<-chan *types.AccountEvent, error) {
	path := "/v1/events/accounts/status"
	if since != "" {
		path += "?since=" + since
	}
	ch := make(chan *types.AccountEvent, 64)
	err := p.streamSSE(ctx, path, func(data []byte) {
		var raw struct {
			EventType      string `json:"event"`
			EventID        string `json:"event_id"`
			AccountID      string `json:"account_id"`
			TradingBlocked bool   `json:"trading_blocked"`
			Timestamp      string `json:"at"`
		}
		if json.Unmarshal(data, &raw) != nil {
			return
		}
		select {
		case ch <- &types.AccountEvent{
			EventType:      raw.EventType,
			EventID:        raw.EventID,
			AccountID:      raw.AccountID,
			TradingBlocked: raw.TradingBlocked,
			Timestamp:      raw.Timestamp,
		}:
		default:
		}
	})
	if err != nil {
		close(ch)
		return nil, err
	}
	return ch, nil
}

func (p *Provider) StreamTransferEvents(ctx context.Context, since string) (<-chan *types.TransferEvent, error) {
	path := "/v1/events/transfers/status"
	if since != "" {
		path += "?since=" + since
	}
	ch := make(chan *types.TransferEvent, 64)
	err := p.streamSSE(ctx, path, func(data []byte) {
		var raw struct {
			EventType string `json:"event"`
			EventID   string `json:"event_id"`
			AccountID string `json:"account_id"`
			Timestamp string `json:"at"`
		}
		if json.Unmarshal(data, &raw) != nil {
			return
		}
		select {
		case ch <- &types.TransferEvent{
			EventType: raw.EventType,
			EventID:   raw.EventID,
			AccountID: raw.AccountID,
			Timestamp: raw.Timestamp,
		}:
		default:
		}
	})
	if err != nil {
		close(ch)
		return nil, err
	}
	return ch, nil
}

func (p *Provider) StreamJournalEvents(ctx context.Context, since string) (<-chan *types.JournalEvent, error) {
	path := "/v1/events/journals/status"
	if since != "" {
		path += "?since=" + since
	}
	ch := make(chan *types.JournalEvent, 64)
	err := p.streamSSE(ctx, path, func(data []byte) {
		var raw struct {
			EventType string `json:"event"`
			EventID   string `json:"event_id"`
			Timestamp string `json:"at"`
		}
		if json.Unmarshal(data, &raw) != nil {
			return
		}
		select {
		case ch <- &types.JournalEvent{
			EventType: raw.EventType,
			EventID:   raw.EventID,
			Timestamp: raw.Timestamp,
		}:
		default:
		}
	})
	if err != nil {
		close(ch)
		return nil, err
	}
	return ch, nil
}

// streamSSE connects to an Alpaca SSE endpoint and calls handler for each data line.
// It runs in a goroutine and closes the channel when the context is cancelled.
func (p *Provider) streamSSE(ctx context.Context, path string, handler func(data []byte)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.cfg.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create SSE request: %w", err)
	}
	req.SetBasicAuth(p.cfg.APIKey, p.cfg.APISecret)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connect: %w", err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return fmt.Errorf("SSE %d: %s", resp.StatusCode, string(body))
	}

	go func() {
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				handler([]byte(strings.TrimPrefix(line, "data: ")))
			}
		}
	}()

	return nil
}
