// Trades / Subscriptions / Documents — TransactAPI Workstream 4.
//
// Endpoints:
//   POST /tapiv3/index.php/v1/createTrade            — book a subscription / trade
//   POST /tapiv3/index.php/v1/getTrade               — read by tradeId
//   POST /tapiv3/index.php/v1/externalUploadTrades   — CSV bulk upload (multipart)
//   POST /tapiv3/index.php/v1/cancelTrade            — cancel a pending trade
//   POST /tapiv3/index.php/v1/refundTrade            — refund / return on a settled trade
//   POST /tapiv3/index.php/v1/sendSubscriptionDocs   — deliver e-sign packet
//   POST /tapiv3/index.php/v1/getTradeDocuments      — read trade-attached documents
//
// A Trade settled means the corresponding ERC-3643 SecurityToken
// movement settled on Lux Network (mint for primary, transfer for
// secondary). Parity is enforced by luxfi/transfer; divergence
// triggers an amld alert and freezes new transfers (brief §5).
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/reference

package northcapital

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
)

type wireTrade struct {
	TradeID       string `json:"tradeId"`
	OfferingID    string `json:"offeringId"`
	PartyID       string `json:"partyId"`
	EntityID      string `json:"entityId"`
	Side          string `json:"transactionType"`
	Units         string `json:"transactionUnits"`
	UnitPrice     string `json:"unitPrice"`
	GrossAmount   string `json:"totalAmount"`
	NetAmount     string `json:"netAmount"`
	Currency      string `json:"currency"`
	Status        string `json:"tradeStatus"`
	PaymentMethod string `json:"paymentMethod"`
	SignedDate    string `json:"signedDate"`
	SettledDate   string `json:"settledDate"`
	CreatedDate   string `json:"createdDate"`
	UpdatedDate   string `json:"updatedDate"`
}

type wireBulkResult struct {
	BatchID   string            `json:"batchId"`
	Accepted  int               `json:"acceptedCount"`
	Rejected  int               `json:"rejectedCount"`
	Errors    []wireBulkErr     `json:"errors"`
	CreatedAt string            `json:"createdAt"`
}

type wireBulkErr struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type wireRefund struct {
	RefundID  string `json:"refundId"`
	TradeID   string `json:"tradeId"`
	Amount    string `json:"amount"`
	Reason    string `json:"reason"`
	Status    string `json:"status"`
	CreatedAt string `json:"createdAt"`
}

type wireTradeDoc struct {
	DocumentID string `json:"documentId"`
	TradeID    string `json:"tradeId"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	URL        string `json:"url"`
	UploadedAt string `json:"uploadedAt"`
}

// CreateTrade books a subscription / trade ticket against an Offering.
// Caller-supplied IdempotencyKey wins; the deterministic fallback
// makes duplicate POSTs collapse to the same tradeId on retry.
func (p *Provider) CreateTrade(ctx context.Context, req *CreateTradeRequest) (*Trade, error) {
	if req == nil {
		return nil, errors.New("northcapital: CreateTradeRequest is required")
	}
	form := url.Values{}
	form.Set("offeringId", req.OfferingID)
	if req.PartyID != "" {
		form.Set("partyId", req.PartyID)
	}
	if req.EntityID != "" {
		form.Set("entityId", req.EntityID)
	}
	form.Set("transactionType", req.Side)
	form.Set("transactionUnits", req.Units)
	if req.UnitPrice != "" {
		form.Set("unitPrice", req.UnitPrice)
	}
	if req.PaymentMethod != "" {
		form.Set("paymentMethod", req.PaymentMethod)
	}

	const path = "/tapiv3/index.php/v1/createTrade"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	return decodeTrade(env.Trade, "createTrade")
}

// GetTrade reads a Trade by TransactAPI tradeId.
func (p *Provider) GetTrade(ctx context.Context, tradeID string) (*Trade, error) {
	if tradeID == "" {
		return nil, errors.New("northcapital: tradeID required")
	}
	form := url.Values{}
	form.Set("tradeId", tradeID)
	const path = "/tapiv3/index.php/v1/getTrade"
	raw, err := p.doForm(ctx, "POST", path, form, "")
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	return decodeTrade(env.Trade, "getTrade")
}

// BulkUploadTrades posts a CSV batch of trades via multipart/form-data.
// Returns a batch result with accepted / rejected counts and per-row
// errors. The CSV format is the documented TransactAPI bulk-trade
// upload schema (offeringId, partyId, transactionUnits, unitPrice,
// paymentMethod, …).
func (p *Provider) BulkUploadTrades(ctx context.Context, csv []byte) (*BulkTradeResult, error) {
	if len(csv) == 0 {
		return nil, errors.New("northcapital: bulk upload: empty CSV")
	}
	const path = "/tapiv3/index.php/v1/externalUploadTrades"

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("clientID", p.cfg.ClientID); err != nil {
		return nil, fmt.Errorf("northcapital: bulk: write field: %w", err)
	}
	if err := writer.WriteField("developerAPIKey", p.cfg.DeveloperAPIKey); err != nil {
		return nil, fmt.Errorf("northcapital: bulk: write field: %w", err)
	}
	part, err := writer.CreateFormFile("file", "trades.csv")
	if err != nil {
		return nil, fmt.Errorf("northcapital: bulk: create form file: %w", err)
	}
	if _, err := part.Write(csv); err != nil {
		return nil, fmt.Errorf("northcapital: bulk: write CSV: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("northcapital: bulk: close writer: %w", err)
	}

	idemKey := p.resolveIdemKey("", "", path, csv)

	endpoint := p.cfg.BaseURL + path
	doOnce := func() (*http.Response, []byte, error) {
		req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body.Bytes()))
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		req.Header.Set(idempotencyHeader, idemKey)
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, nil, err
		}
		defer resp.Body.Close()
		b, rerr := io.ReadAll(resp.Body)
		return resp, b, rerr
	}
	raw, err := p.doWithRetry(ctx, path, doOnce)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.BulkResult) == 0 {
		return nil, fmt.Errorf("northcapital: bulk: empty bulkResult")
	}
	var w wireBulkResult
	if err := json.Unmarshal(env.BulkResult, &w); err != nil {
		return nil, fmt.Errorf("northcapital: bulk decode: %w", err)
	}
	errs := make([]BulkTradeErr, 0, len(w.Errors))
	for _, e := range w.Errors {
		errs = append(errs, BulkTradeErr{Row: e.Row, Message: e.Message})
	}
	return &BulkTradeResult{
		BatchID:   w.BatchID,
		Accepted:  w.Accepted,
		Rejected:  w.Rejected,
		Errors:    errs,
		CreatedAt: parseDate(w.CreatedAt),
	}, nil
}

// CancelTrade cancels a pending (unfunded) trade.
func (p *Provider) CancelTrade(ctx context.Context, req *CancelTradeRequest) (*Trade, error) {
	if req == nil || req.TradeID == "" {
		return nil, errors.New("northcapital: tradeID required")
	}
	form := url.Values{}
	form.Set("tradeId", req.TradeID)
	if req.Reason != "" {
		form.Set("reason", req.Reason)
	}
	const path = "/tapiv3/index.php/v1/cancelTrade"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	return decodeTrade(env.Trade, "cancelTrade")
}

// RefundTrade issues a refund / return on a settled trade. Partial
// refunds supply the partial Amount; an empty Amount means full refund.
func (p *Provider) RefundTrade(ctx context.Context, req *RefundTradeRequest) (*Refund, error) {
	if req == nil || req.TradeID == "" {
		return nil, errors.New("northcapital: tradeID required")
	}
	form := url.Values{}
	form.Set("tradeId", req.TradeID)
	if req.Amount != "" {
		form.Set("amount", req.Amount)
	}
	if req.Reason != "" {
		form.Set("reason", req.Reason)
	}
	const path = "/tapiv3/index.php/v1/refundTrade"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.Refund) == 0 {
		return nil, fmt.Errorf("northcapital: refundTrade: empty refundDetails")
	}
	var w wireRefund
	if err := json.Unmarshal(env.Refund, &w); err != nil {
		var arr []wireRefund
		if err2 := json.Unmarshal(env.Refund, &arr); err2 == nil && len(arr) > 0 {
			w = arr[0]
		} else {
			return nil, fmt.Errorf("northcapital: refundTrade decode: %w", err)
		}
	}
	return &Refund{
		ID:        w.RefundID,
		TradeID:   w.TradeID,
		Amount:    w.Amount,
		Reason:    w.Reason,
		Status:    w.Status,
		CreatedAt: parseDate(w.CreatedAt),
	}, nil
}

// SendSubscriptionDocs delivers the subscription document packet to
// the trade's signers via TransactAPI's documented e-sign integration.
func (p *Provider) SendSubscriptionDocs(ctx context.Context, req *SendSubscriptionDocsRequest) error {
	if req == nil || req.TradeID == "" {
		return errors.New("northcapital: tradeID required")
	}
	form := url.Values{}
	form.Set("tradeId", req.TradeID)
	for i, r := range req.Recipients {
		form.Set(fmt.Sprintf("recipient%d", i+1), r)
	}
	const path = "/tapiv3/index.php/v1/sendSubscriptionDocs"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return err
	}
	if _, err := decodeEnvelope(path, raw); err != nil {
		return err
	}
	return nil
}

// GetTradeDocuments reads the document attachments on a trade
// (subscription agreement, PPM, accredited-investor letter, etc.).
func (p *Provider) GetTradeDocuments(ctx context.Context, tradeID string) ([]*TradeDocument, error) {
	if tradeID == "" {
		return nil, errors.New("northcapital: tradeID required")
	}
	form := url.Values{}
	form.Set("tradeId", tradeID)
	const path = "/tapiv3/index.php/v1/getTradeDocuments"
	raw, err := p.doForm(ctx, "POST", path, form, "")
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.Documents) == 0 {
		return nil, nil
	}
	var arr []wireTradeDoc
	if err := json.Unmarshal(env.Documents, &arr); err != nil {
		// Try single-object form.
		var one wireTradeDoc
		if err2 := json.Unmarshal(env.Documents, &one); err2 == nil {
			arr = []wireTradeDoc{one}
		} else {
			return nil, fmt.Errorf("northcapital: getTradeDocuments decode: %w", err)
		}
	}
	out := make([]*TradeDocument, 0, len(arr))
	for i := range arr {
		out = append(out, &TradeDocument{
			ID:         arr[i].DocumentID,
			TradeID:    arr[i].TradeID,
			Name:       arr[i].Name,
			Type:       arr[i].Type,
			Status:     arr[i].Status,
			URL:        arr[i].URL,
			UploadedAt: parseDate(arr[i].UploadedAt),
		})
	}
	return out, nil
}

func decodeTrade(raw json.RawMessage, where string) (*Trade, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("northcapital: %s: empty tradeDetails", where)
	}
	var w wireTrade
	if err := json.Unmarshal(raw, &w); err != nil {
		var arr []wireTrade
		if err2 := json.Unmarshal(raw, &arr); err2 == nil && len(arr) > 0 {
			w = arr[0]
		} else {
			return nil, fmt.Errorf("northcapital: %s decode: %w", where, err)
		}
	}
	t := &Trade{
		ID:            w.TradeID,
		OfferingID:    w.OfferingID,
		PartyID:       w.PartyID,
		EntityID:      w.EntityID,
		Side:          w.Side,
		Units:         w.Units,
		UnitPrice:     w.UnitPrice,
		GrossAmount:   w.GrossAmount,
		NetAmount:     w.NetAmount,
		Currency:      w.Currency,
		Status:        w.Status,
		PaymentMethod: w.PaymentMethod,
	}
	t.CreatedAt = parseDate(w.CreatedDate)
	t.UpdatedAt = parseDate(w.UpdatedDate)
	if w.SignedDate != "" {
		s := parseDate(w.SignedDate)
		t.SignedAt = &s
	}
	if w.SettledDate != "" {
		s := parseDate(w.SettledDate)
		t.SettledAt = &s
	}
	return t, nil
}
