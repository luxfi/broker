// OAuth2 / JWT-bearer authentication for Alpaca Broker API (the canonical
// post-2025 auth mode). Alpaca registers JWT-P-256 clients whose public
// key they hold; we sign a short-lived client_assertion with the matching
// private key, exchange it for an access_token at authx, and use the token
// as a Bearer on broker-api.
//
// Reference: Alpaca "Authentication" docs + RFC 7523.
//
// Critical detail: the `kid` header claim MUST be omitted — Alpaca returns
// 400 invalid_request when it's present, even though the JWT is otherwise
// valid. This appears to be Alpaca-specific; RFC 7523 permits `kid` but
// their authx rejects it.
//
// Token lifetime: 15 minutes. We refresh ~60s before expiry.
//
// Endpoints:
//
//	Sandbox: https://authx.sandbox.alpaca.markets/v1/oauth2/token
//	Prod:    https://authx.alpaca.markets/v1/oauth2/token

package alpaca_omnisub

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

const (
	authxSandbox = "https://authx.sandbox.alpaca.markets/v1/oauth2/token"
	authxProd    = "https://authx.alpaca.markets/v1/oauth2/token"

	clientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"

	// Refresh the token this long before actual expiry to avoid races at the edge.
	tokenRefreshSafety = 60 * time.Second
)

// jwtSigner holds the parsed ECDSA private key and exchanges JWT assertions
// for short-lived access tokens, caching the token across calls.
type jwtSigner struct {
	clientID   string
	priv       *ecdsa.PrivateKey
	tokenURL   string
	httpClient *http.Client

	mu        sync.Mutex
	tokenVal  string
	tokenExp  time.Time
}

// newJWTSigner parses the PEM private key and returns a signer bound to the
// correct authx endpoint for the given broker base URL.
// Accepts PKCS#8 ("BEGIN PRIVATE KEY") or SEC1 ("BEGIN EC PRIVATE KEY") PEM.
func newJWTSigner(clientID, privateKeyPEM, brokerBaseURL string, hc *http.Client) (*jwtSigner, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("alpaca jwt: no PEM block found in private key")
	}

	var priv any
	var err error
	switch block.Type {
	case "PRIVATE KEY":
		priv, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		priv, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		// Some Alpaca dashboards mis-label PKCS#8 keys as "EC PRIVATE KEY".
		// Try PKCS#8 first, fall back to SEC1.
		if priv, err = x509.ParsePKCS8PrivateKey(block.Bytes); err != nil {
			priv, err = x509.ParseECPrivateKey(block.Bytes)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("alpaca jwt: parse private key: %w", err)
	}
	ec, ok := priv.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("alpaca jwt: private key is not ECDSA (got %T)", priv)
	}
	if ec.Curve.Params().Name != "P-256" {
		return nil, fmt.Errorf("alpaca jwt: expected P-256 curve, got %s", ec.Curve.Params().Name)
	}

	tokenURL := authxProd
	if strings.Contains(brokerBaseURL, "sandbox") {
		tokenURL = authxSandbox
	}

	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &jwtSigner{
		clientID:   clientID,
		priv:       ec,
		tokenURL:   tokenURL,
		httpClient: hc,
	}, nil
}

// Token returns a valid access_token, refreshing if close to expiry.
func (s *jwtSigner) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tokenVal != "" && time.Until(s.tokenExp) > tokenRefreshSafety {
		return s.tokenVal, nil
	}

	assertion, err := s.signAssertion()
	if err != nil {
		return "", err
	}

	form := url.Values{
		"grant_type":            {"client_credentials"},
		"client_id":             {s.clientID},
		"client_assertion_type": {clientAssertionType},
		"client_assertion":      {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("alpaca jwt: token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("alpaca jwt: token exchange HTTP %d: %s", resp.StatusCode, string(body))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("alpaca jwt: decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("alpaca jwt: empty access_token in response")
	}

	s.tokenVal = tr.AccessToken
	s.tokenExp = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return s.tokenVal, nil
}

// signAssertion builds and signs the JWT assertion per RFC 7523 + Alpaca quirk
// (omit `kid` from header — Alpaca rejects when present).
func (s *jwtSigner) signAssertion() (string, error) {
	now := time.Now()
	header := map[string]string{"alg": "ES256", "typ": "JWT"}
	payload := map[string]any{
		"iss": s.clientID,
		"sub": s.clientID,
		"aud": s.tokenURL,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
		"jti": newJTI(),
	}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(payload)
	signingInput := b64url(hb) + "." + b64url(pb)

	hash := sha256.Sum256([]byte(signingInput))
	r, t, err := ecdsa.Sign(rand.Reader, s.priv, hash[:])
	if err != nil {
		return "", fmt.Errorf("alpaca jwt: sign: %w", err)
	}

	// ES256 signature is raw R || S, each 32 bytes (RFC 7518 §3.4).
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	t.FillBytes(sig[32:])
	return signingInput + "." + b64url(sig), nil
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func newJTI() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// Encode as a big-int hex-ish string; it only needs to be unique per request.
	return new(big.Int).SetBytes(b[:]).Text(36)
}
