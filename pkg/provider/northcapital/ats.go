// ATS / Secondary trades — TransactAPI Workstream 7.
//
// Endpoints / channels:
//   POST /tapiv3/index.php/v1/publishAtsEvent       — Lux-originated ATS event
//   POST /tapiv3/index.php/v1/getSecondaryTrades    — Secondary Trades Directory
//
// The ATS webhook channel is the inbound complement, handled by the
// Provider.ConsumeWebhook entry point (see webhook.go). Events flow
// outbound (Lux → TransactAPI) on order placement / cancellation /
// match / clear / settle so the NCPS-side ATS record stays in lockstep
// with the Lux on-chain compliance-module state.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/reference

package northcapital

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

type wireSecondaryTrade struct {
	TradeID    string `json:"tradeId"`
	OfferingID string `json:"offeringId"`
	BuyerID    string `json:"buyerPartyId"`
	SellerID   string `json:"sellerPartyId"`
	Units      string `json:"transactionUnits"`
	UnitPrice  string `json:"unitPrice"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
	TradedAt   string `json:"tradedAt"`
	SettledAt  string `json:"settledAt"`
}

// PublishATSEvent posts an ATS lifecycle event to TransactAPI's ATS
// channel. The EventID is the idempotency-key input so retries of the
// same logical event collapse on the NCPS side.
func (p *Provider) PublishATSEvent(ctx context.Context, evt *ATSEvent) error {
	if evt == nil {
		return errors.New("northcapital: ATSEvent is required")
	}
	if evt.EventID == "" {
		return errors.New("northcapital: ATSEvent.EventID is required for idempotency")
	}
	form := url.Values{}
	form.Set("eventId", evt.EventID)
	form.Set("eventType", evt.EventType)
	form.Set("offeringId", evt.OfferingID)
	if evt.TradeID != "" {
		form.Set("tradeId", evt.TradeID)
	}
	if evt.PartyID != "" {
		form.Set("partyId", evt.PartyID)
	}
	if evt.EntityID != "" {
		form.Set("entityId", evt.EntityID)
	}
	if evt.Side != "" {
		form.Set("transactionType", evt.Side)
	}
	if evt.Units != "" {
		form.Set("transactionUnits", evt.Units)
	}
	if evt.UnitPrice != "" {
		form.Set("unitPrice", evt.UnitPrice)
	}
	if !evt.Timestamp.IsZero() {
		form.Set("timestamp", evt.Timestamp.UTC().Format("2006-01-02T15:04:05Z"))
	}

	const path = "/tapiv3/index.php/v1/publishAtsEvent"
	// EventID is the natural idempotency key — same event sent twice
	// must collapse, regardless of caller-supplied IdempotencyKey.
	raw, err := p.doForm(ctx, "POST", path, form, evt.EventID)
	if err != nil {
		return err
	}
	if _, err := decodeEnvelope(path, raw); err != nil {
		return err
	}
	return nil
}

// GetSecondaryTrades reads the Secondary Trades Directory for an Offering.
func (p *Provider) GetSecondaryTrades(ctx context.Context, offeringID string) ([]*SecondaryTrade, error) {
	if offeringID == "" {
		return nil, errors.New("northcapital: offeringID required")
	}
	form := url.Values{}
	form.Set("offeringId", offeringID)
	const path = "/tapiv3/index.php/v1/getSecondaryTrades"
	raw, err := p.doForm(ctx, "POST", path, form, "")
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.Secondary) == 0 {
		return nil, nil
	}
	var arr []wireSecondaryTrade
	if err := json.Unmarshal(env.Secondary, &arr); err != nil {
		var one wireSecondaryTrade
		if err2 := json.Unmarshal(env.Secondary, &one); err2 == nil {
			arr = []wireSecondaryTrade{one}
		} else {
			return nil, fmt.Errorf("northcapital: getSecondaryTrades decode: %w", err)
		}
	}
	out := make([]*SecondaryTrade, 0, len(arr))
	for i := range arr {
		st := &SecondaryTrade{
			ID:         arr[i].TradeID,
			OfferingID: arr[i].OfferingID,
			BuyerID:    arr[i].BuyerID,
			SellerID:   arr[i].SellerID,
			Units:      arr[i].Units,
			UnitPrice:  arr[i].UnitPrice,
			Currency:   arr[i].Currency,
			Status:     arr[i].Status,
			TradedAt:   parseDate(arr[i].TradedAt),
		}
		if arr[i].SettledAt != "" {
			t := parseDate(arr[i].SettledAt)
			st.SettledAt = &t
		}
		out = append(out, st)
	}
	return out, nil
}
