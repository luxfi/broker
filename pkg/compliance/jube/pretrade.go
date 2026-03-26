package jube

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

// PreTradeAction describes what the broker should do with the order.
type PreTradeAction string

const (
	PreTradeAllow  PreTradeAction = "allow"
	PreTradeBlock  PreTradeAction = "block"
	PreTradeReview PreTradeAction = "review"
)

// PreTradeConfig controls how the Jube pre-trade screen behaves.
type PreTradeConfig struct {
	// ModelID is the Jube EntityAnalysisModel to invoke.
	ModelID int

	// AllowOnReview controls whether orders flagged "review" are allowed
	// to proceed (true) or held (false). Default: true (allow, flag async).
	AllowOnReview bool

	// AllowOnError controls whether orders proceed if Jube is unreachable.
	// Default: true (fail-open). Set false for fail-closed.
	AllowOnError bool

	// WebhookURL is the target for aml.flagged webhook events. Empty = no webhook.
	WebhookURL string

	// WebhookHMACSecret is the HMAC-SHA256 secret for signing webhooks.
	// Must come from KMS, never hardcoded.
	WebhookHMACSecret string
}

// PreTradeResult is the outcome of a Jube AML screen on an order.
type PreTradeResult struct {
	Action   PreTradeAction `json:"action"`
	Allowed  bool           `json:"allowed"`
	Score    float64        `json:"score"`
	Alerts   []Alert        `json:"alerts,omitempty"`
	Errors   []string       `json:"errors,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
}

// PreTradeScreen wraps a Jube client and provides pre-trade AML screening
// for the broker's order flow. Call Screen() before executing any trade.
type PreTradeScreen struct {
	client *Client
	cfg    PreTradeConfig
}

// NewPreTradeScreen creates a pre-trade screener using the given Jube client.
func NewPreTradeScreen(client *Client, cfg PreTradeConfig) *PreTradeScreen {
	if cfg.ModelID == 0 {
		cfg.ModelID = 1 // default model
	}
	return &PreTradeScreen{
		client: client,
		cfg:    cfg,
	}
}

// Screen submits a transaction to Jube for AML/fraud scoring and returns
// a decision. This should be called in the broker's order creation path
// after risk checks pass but before the order reaches the provider.
func (s *PreTradeScreen) Screen(ctx context.Context, req ScreenRequest) PreTradeResult {
	txReq := TransactionRequest{
		EntityAnalysisModelID: s.cfg.ModelID,
		EntityInstanceEntryPayload: map[string]interface{}{
			"AccountId":     req.AccountID,
			"TransactionId": req.OrderID,
			"Amount":        req.Amount(),
			"Currency":      req.Currency,
			"Symbol":        req.Symbol,
			"Side":          req.Side,
			"Provider":      req.Provider,
			"IP":            req.IP,
		},
	}

	resp, err := s.client.ScreenTransaction(ctx, txReq)
	if err != nil {
		log.Error().Err(err).
			Str("account", req.AccountID).
			Str("symbol", req.Symbol).
			Msg("jube: pre-trade screen failed")

		if s.cfg.AllowOnError {
			return PreTradeResult{
				Action:   PreTradeAllow,
				Allowed:  true,
				Warnings: []string{fmt.Sprintf("jube unavailable: %v", err)},
			}
		}
		return PreTradeResult{
			Action:  PreTradeBlock,
			Allowed: false,
			Errors:  []string{fmt.Sprintf("compliance check unavailable: %v", err)},
		}
	}

	result := PreTradeResult{
		Score:  resp.Score,
		Alerts: resp.Alerts,
	}

	switch resp.Action {
	case ActionBlock:
		result.Action = PreTradeBlock
		result.Allowed = false
		result.Errors = append(result.Errors, "transaction blocked by AML screening")
		for _, a := range resp.Alerts {
			result.Errors = append(result.Errors, fmt.Sprintf("[%s] %s (score=%.2f)", a.Severity, a.RuleName, a.Score))
		}

	case ActionReview:
		result.Action = PreTradeReview
		result.Allowed = s.cfg.AllowOnReview
		result.Warnings = append(result.Warnings, "transaction flagged for manual review")
		for _, a := range resp.Alerts {
			result.Warnings = append(result.Warnings, fmt.Sprintf("[%s] %s (score=%.2f)", a.Severity, a.RuleName, a.Score))
		}

	default: // allow
		result.Action = PreTradeAllow
		result.Allowed = true
	}

	// Fire webhook async for flagged/blocked transactions.
	if resp.Action == ActionBlock || resp.Action == ActionReview {
		if s.cfg.WebhookURL != "" && s.cfg.WebhookHMACSecret != "" {
			go s.fireAMLWebhook(req, resp)
		}

		// Publish to NATS if connected.
		if s.client.natsConn != nil && len(resp.Alerts) > 0 {
			go s.publishNATSAlert(req, resp)
		}
	}

	return result
}

// ScreenRequest contains the order details for AML screening.
type ScreenRequest struct {
	AccountID string
	OrderID   string
	Provider  string
	Symbol    string
	Side      string
	Qty       string
	Price     string
	Currency  string
	IP        string
}

// Amount returns the estimated USD value of the order.
func (r ScreenRequest) Amount() float64 {
	q, _ := strconv.ParseFloat(r.Qty, 64)
	p, _ := strconv.ParseFloat(r.Price, 64)
	if p == 0 {
		p = 1
	}
	return q * p
}

// fireAMLWebhook sends an aml.flagged webhook event.
func (s *PreTradeScreen) fireAMLWebhook(req ScreenRequest, resp *TransactionResponse) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	event := WebhookEvent{
		Event:     EventAMLFlagged,
		Timestamp: time.Now().UTC(),
		Data: AMLFlaggedData{
			AccountID:     req.AccountID,
			TransactionID: req.OrderID,
			RiskScore:     resp.Score,
			Alerts:        resp.Alerts,
			Action:        resp.Action,
		},
	}

	if err := FireWebhook(ctx, event, s.cfg.WebhookURL, s.cfg.WebhookHMACSecret); err != nil {
		log.Error().Err(err).
			Str("account", req.AccountID).
			Msg("jube: failed to fire aml.flagged webhook")
	}
}

// publishNATSAlert publishes an alert to NATS for async processing.
func (s *PreTradeScreen) publishNATSAlert(req ScreenRequest, resp *TransactionResponse) {
	for _, alert := range resp.Alerts {
		data, err := json.Marshal(alert)
		if err != nil {
			log.Error().Err(err).Msg("jube: failed to marshal alert for NATS")
			continue
		}
		if err := s.client.natsConn.Publish(NATSSubjectAlerts, data); err != nil {
			log.Error().Err(err).
				Str("account", req.AccountID).
				Str("alert", alert.ID).
				Msg("jube: failed to publish NATS alert")
		}
	}
}
