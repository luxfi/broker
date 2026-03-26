package jube

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
)

// DefaultBaseURL is the default Jube sidecar address.
const DefaultBaseURL = "http://jube:5001"

// DefaultTimeout is the HTTP request timeout for Jube API calls.
const DefaultTimeout = 10 * time.Second

// NATSSubjectAlerts is the NATS subject for async AML alerts from Jube.
const NATSSubjectAlerts = "compliance.jube.alerts"

// Config holds Jube client configuration.
type Config struct {
	BaseURL  string        // Jube sidecar URL (default: http://jube:5001)
	Timeout  time.Duration // HTTP timeout (default: 10s)
	NATSAddr string        // NATS server address for async alerts (e.g. nats://localhost:4222)
}

// Client is an HTTP client for the Jube AML/fraud detection sidecar.
type Client struct {
	baseURL    string
	httpClient *http.Client
	natsConn   *nats.Conn
	natsSub    *nats.Subscription
}

// New creates a new Jube client. If cfg.NATSAddr is set, it connects to NATS
// for async alert subscriptions. Returns an error if NATS connection fails.
func New(cfg Config) (*Client, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	if cfg.NATSAddr != "" {
		nc, err := nats.Connect(cfg.NATSAddr)
		if err != nil {
			return nil, fmt.Errorf("jube: nats connect %s: %w", cfg.NATSAddr, err)
		}
		c.natsConn = nc
	}

	return c, nil
}

// Close releases resources held by the client.
func (c *Client) Close() error {
	if c.natsSub != nil {
		if err := c.natsSub.Unsubscribe(); err != nil {
			return fmt.Errorf("jube: nats unsubscribe: %w", err)
		}
	}
	if c.natsConn != nil {
		c.natsConn.Close()
	}
	return nil
}

// ScreenTransaction submits a transaction to Jube for real-time risk scoring.
// Calls POST /api/EntityAnalysisModel/Invoke.
func (c *Client) ScreenTransaction(ctx context.Context, req TransactionRequest) (*TransactionResponse, error) {
	var resp TransactionResponse
	if err := c.post(ctx, "/api/EntityAnalysisModel/Invoke", req, &resp); err != nil {
		return nil, fmt.Errorf("jube: screen transaction: %w", err)
	}
	return &resp, nil
}

// CheckSanctions performs sanctions screening against Jube's lists.
// Calls GET /api/Sanction with name and country query params.
func (c *Client) CheckSanctions(ctx context.Context, name, country string) (*SanctionResult, error) {
	params := url.Values{}
	params.Set("name", name)
	if country != "" {
		params.Set("country", country)
	}

	var resp SanctionResult
	if err := c.get(ctx, "/api/Sanction?"+params.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("jube: check sanctions: %w", err)
	}
	return &resp, nil
}

// CreateCase creates a new compliance case in Jube.
// Calls POST /api/CaseManagement.
func (c *Client) CreateCase(ctx context.Context, req CaseRequest) (*Case, error) {
	var resp Case
	if err := c.post(ctx, "/api/CaseManagement", req, &resp); err != nil {
		return nil, fmt.Errorf("jube: create case: %w", err)
	}
	return &resp, nil
}

// GetCases retrieves compliance cases matching the given filters.
// Calls GET /api/CaseManagement with optional query params.
func (c *Client) GetCases(ctx context.Context, filter CaseFilter) ([]Case, error) {
	params := url.Values{}
	if filter.AccountID != "" {
		params.Set("accountId", filter.AccountID)
	}
	if filter.Type != "" {
		params.Set("type", filter.Type)
	}
	if filter.Status != "" {
		params.Set("status", filter.Status)
	}

	path := "/api/CaseManagement"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	var resp []Case
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, fmt.Errorf("jube: get cases: %w", err)
	}
	return resp, nil
}

// Search performs an exhaustive search across Jube entities.
// Calls POST /api/ExhaustiveSearchInstance.
func (c *Client) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	var resp []SearchResult
	if err := c.post(ctx, "/api/ExhaustiveSearchInstance", req, &resp); err != nil {
		return nil, fmt.Errorf("jube: search: %w", err)
	}
	return resp, nil
}

// SubscribeAlerts subscribes to async AML/fraud alerts via NATS.
// The handler is called for each alert received. Returns an error if no
// NATS connection was configured.
func (c *Client) SubscribeAlerts(handler func(Alert)) error {
	if c.natsConn == nil {
		return fmt.Errorf("jube: nats not configured, cannot subscribe to alerts")
	}

	sub, err := c.natsConn.Subscribe(NATSSubjectAlerts, func(msg *nats.Msg) {
		var alert Alert
		if err := json.Unmarshal(msg.Data, &alert); err != nil {
			log.Error().Err(err).Msg("jube: failed to unmarshal NATS alert")
			return
		}
		handler(alert)
	})
	if err != nil {
		return fmt.Errorf("jube: nats subscribe: %w", err)
	}
	c.natsSub = sub
	return nil
}

// post sends a POST request with a JSON body and decodes the response.
func (c *Client) post(ctx context.Context, path string, body, dst interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return c.doJSON(req, dst)
}

// get sends a GET request and decodes the JSON response.
func (c *Client) get(ctx context.Context, path string, dst interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	return c.doJSON(req, dst)
}

// doJSON executes an HTTP request and decodes the JSON response into dst.
func (c *Client) doJSON(req *http.Request, dst interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("http %s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, string(body))
	}

	if dst != nil {
		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
