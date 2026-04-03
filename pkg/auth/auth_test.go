package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func TestMiddlewareHealthzSkipsAuth(t *testing.T) {
	handler := Middleware("http://localhost:9999")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", rr.Code)
	}
}

func TestMiddlewareStripsInjectedHeaders(t *testing.T) {
	// Verify that externally-set identity headers are stripped and the
	// request is rejected (no valid Bearer token).
	handler := Middleware("http://localhost:9999")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-User-Id", "injected-user")
	req.Header.Set("X-Org-Id", "injected-org")
	req.Header.Set("X-User-Email", "injected@example.com")
	req.Header.Set("X-Account-Id", "injected-acct")
	req.Header.Set("X-Gateway-User-Id", "injected-gw")
	req.Header.Set("X-Hanzo-User-Id", "injected-hanzo")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Should be rejected because the injected headers are stripped and
	// no Bearer token is present.
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when identity headers are injected without Bearer token, got %d", rr.Code)
	}
}

func TestMiddlewareHealthzStripsHeaders(t *testing.T) {
	// Even /healthz should strip identity headers to prevent leaking
	// injected state into downstream handlers.
	var gotUserID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-Id")
		w.WriteHeader(http.StatusOK)
	})
	handler := Middleware("http://localhost:9999")(inner)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-User-Id", "injected-user")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotUserID != "" {
		t.Fatalf("expected X-User-Id to be stripped on /healthz, got %q", gotUserID)
	}
}

func TestMiddlewareRejectsNoHeaders(t *testing.T) {
	handler := Middleware("http://localhost:9999")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMiddlewareRejectsInvalidBearerToken(t *testing.T) {
	handler := Middleware("http://localhost:9999")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("Authorization", "Bearer some-jwt")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with invalid Bearer token, got %d", rr.Code)
	}
}

func TestRequireOrgAllowed(t *testing.T) {
	handler := RequireOrg("built-in")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-Org-Id", "built-in")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequireOrgDenied(t *testing.T) {
	handler := RequireOrg("built-in")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-Org-Id", "liquidity")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestHasRoleCommaSeparated(t *testing.T) {
	tests := []struct {
		roles  string
		role   string
		expect bool
	}{
		{"admin,trade,read", "trade", true},
		{"admin, trade, read", "trade", true},
		{"read", "trade", false},
		{"", "trade", false},
		{"superadmin", "superadmin", true},
		{"viewer,editor", "admin", false},
	}
	for _, tt := range tests {
		got := HasRole(tt.roles, tt.role)
		if got != tt.expect {
			t.Errorf("HasRole(%q, %q) = %v, want %v", tt.roles, tt.role, got, tt.expect)
		}
	}
}

func TestWriteErrJSONSafe(t *testing.T) {
	rr := httptest.NewRecorder()
	writeErr(rr, http.StatusBadRequest, `injection"attempt`)
	body := rr.Body.String()
	// json.Marshal should properly escape the quote
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	// Verify it's valid JSON by checking it doesn't contain unescaped injection
	if !contains(body, `injection\"attempt`) && !contains(body, `injection\u0022attempt`) {
		// Either escaped form is acceptable from json.Marshal
		// Just verify the raw injection doesn't appear unescaped
		if contains(body, `injection"attempt"`) {
			t.Fatalf("writeErr did not escape JSON: %s", body)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
