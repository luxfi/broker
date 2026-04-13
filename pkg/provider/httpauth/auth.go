// Package httpauth provides pluggable HTTP authentication strategies for
// any provider client. One interface, interchangeable implementations.
//
// Usage in a provider:
//
//	type Provider struct {
//	    auth httpauth.Authenticator
//	    ...
//	}
//
//	func (p *Provider) do(ctx context.Context, ...) {
//	    req, _ := http.NewRequestWithContext(...)
//	    if err := p.auth.Apply(ctx, req); err != nil {
//	        return nil, err
//	    }
//	    return p.client.Do(req)
//	}
//
// Auth concerns (caching, signing, refreshing) belong here — providers stay
// dumb HTTP proxies. This is how every broker client should plug auth.
package httpauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Authenticator decorates an outbound HTTP request with whatever auth the
// upstream API requires. Implementations should be goroutine-safe.
type Authenticator interface {
	// Apply sets the Authorization header (or equivalent) on req. Called
	// before every outbound HTTP call. Implementations may cache tokens,
	// refresh as needed, and use ctx for deadlines on the refresh call.
	Apply(ctx context.Context, req *http.Request) error

	// Name returns a short identifier for logs / metrics.
	Name() string
}

// ─── 1. Basic — static HTTP Basic auth (legacy Alpaca, Kraken, many others) ───

type Basic struct {
	Username, Password string
}

func (b *Basic) Apply(_ context.Context, req *http.Request) error {
	req.SetBasicAuth(b.Username, b.Password)
	return nil
}
func (b *Basic) Name() string { return "basic" }

// ─── 2. ClientSecret — OAuth2 client_credentials with plain secret in body ───
// (Alpaca "Client Secret" credential type)

type ClientSecret struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Scope        string // optional

	HTTPClient *http.Client

	mu       sync.Mutex
	tokenVal string
	tokenExp time.Time
}

func (c *ClientSecret) Apply(ctx context.Context, req *http.Request) error {
	tok, err := c.token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}
func (c *ClientSecret) Name() string { return "client_secret" }

func (c *ClientSecret) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokenVal != "" && time.Until(c.tokenExp) > tokenSafety {
		return c.tokenVal, nil
	}
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.ClientID},
		"client_secret": {c.ClientSecret},
	}
	if c.Scope != "" {
		form.Set("scope", c.Scope)
	}
	return c.exchange(ctx, form)
}

func (c *ClientSecret) exchange(ctx context.Context, form url.Values) (string, error) {
	hc := c.HTTPClient
	if hc == nil {
		hc = defaultHTTPClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: token exchange: %w", c.Name(), err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: token HTTP %d: %s", c.Name(), resp.StatusCode, string(body))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("%s: decode token: %w", c.Name(), err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("%s: empty access_token", c.Name())
	}
	c.tokenVal = tr.AccessToken
	c.tokenExp = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.tokenVal, nil
}

// ─── 3. JWT — RFC 7523 private_key_jwt client assertion (Alpaca "JWT P-256") ───

type JWT struct {
	TokenURL   string
	ClientID   string
	PrivateKey *ecdsa.PrivateKey // must be P-256

	// If true, include `kid` header claim (standards-compliant).
	// Some servers (Alpaca's authx) reject `kid` → leave false. Default false.
	IncludeKID bool

	HTTPClient *http.Client

	mu       sync.Mutex
	tokenVal string
	tokenExp time.Time
}

// NewJWTFromPEM is a convenience constructor that parses PKCS#8 or SEC1 PEM.
func NewJWTFromPEM(tokenURL, clientID, privateKeyPEM string) (*JWT, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("jwt: no PEM block")
	}
	var priv any
	var err error
	// Try PKCS#8 first (the most common container), fall back to SEC1.
	if priv, err = x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
		priv, err = x509.ParseECPrivateKey(block.Bytes)
	}
	if err != nil {
		return nil, fmt.Errorf("jwt: parse: %w", err)
	}
	ec, ok := priv.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("jwt: not an ECDSA key (got %T)", priv)
	}
	if ec.Curve.Params().Name != "P-256" {
		return nil, fmt.Errorf("jwt: expected P-256, got %s", ec.Curve.Params().Name)
	}
	return &JWT{TokenURL: tokenURL, ClientID: clientID, PrivateKey: ec}, nil
}

func (j *JWT) Apply(ctx context.Context, req *http.Request) error {
	tok, err := j.token(ctx)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return nil
}
func (j *JWT) Name() string { return "jwt_p256" }

func (j *JWT) token(ctx context.Context) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.tokenVal != "" && time.Until(j.tokenExp) > tokenSafety {
		return j.tokenVal, nil
	}

	assertion, err := j.signAssertion()
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type":            {"client_credentials"},
		"client_id":             {j.ClientID},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {assertion},
	}

	hc := j.HTTPClient
	if hc == nil {
		hc = defaultHTTPClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: token exchange: %w", j.Name(), err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: token HTTP %d: %s", j.Name(), resp.StatusCode, string(body))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("%s: decode: %w", j.Name(), err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("%s: empty access_token", j.Name())
	}
	j.tokenVal = tr.AccessToken
	j.tokenExp = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return j.tokenVal, nil
}

func (j *JWT) signAssertion() (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "ES256", "typ": "JWT"}
	if j.IncludeKID {
		header["kid"] = j.ClientID
	}
	payload := map[string]any{
		"iss": j.ClientID,
		"sub": j.ClientID,
		"aud": j.TokenURL,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"jti": newJTI(),
	}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	signingInput := b64url(hb) + "." + b64url(pb)

	hash := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, j.PrivateKey, hash[:])
	if err != nil {
		return "", fmt.Errorf("%s: sign: %w", j.Name(), err)
	}
	// ES256 raw R||S per RFC 7518 §3.4, each 32 bytes.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + b64url(sig), nil
}

// ─── helpers ───

// Refresh this long before actual expiry to avoid races at the edge.
const tokenSafety = 60 * time.Second

var defaultHTTPClient = &http.Client{Timeout: 10 * time.Second}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func newJTI() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return new(big.Int).SetBytes(b[:]).Text(36)
}
