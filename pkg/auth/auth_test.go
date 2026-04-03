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
	handler := Middleware()(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", rr.Code)
	}
}

func TestMiddlewareAcceptsGatewayHeaders(t *testing.T) {
	var gotUserID, gotOrgID, gotEmail string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID = r.Header.Get("X-User-Id")
		gotOrgID = r.Header.Get("X-Org-Id")
		gotEmail = r.Header.Get("X-User-Email")
		w.WriteHeader(http.StatusOK)
	})
	handler := Middleware()(inner)
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("X-User-Id", "user-123")
	req.Header.Set("X-Org-Id", "org-456")
	req.Header.Set("X-User-Email", "user@example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if gotUserID != "user-123" {
		t.Fatalf("expected X-User-Id 'user-123', got %q", gotUserID)
	}
	if gotOrgID != "org-456" {
		t.Fatalf("expected X-Org-Id 'org-456', got %q", gotOrgID)
	}
	if gotEmail != "user@example.com" {
		t.Fatalf("expected X-User-Email 'user@example.com', got %q", gotEmail)
	}
}

func TestMiddlewareRejectsNoHeaders(t *testing.T) {
	handler := Middleware()(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMiddlewareIgnoresBearerToken(t *testing.T) {
	handler := Middleware()(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("Authorization", "Bearer some-jwt")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with only Bearer token, got %d", rr.Code)
	}
}

func TestRequirePermissionGranted(t *testing.T) {
	handler := RequirePermission("trade")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodPost, "/v1/orders", nil)
	req.Header.Set("X-User-Roles", "trade,read")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequirePermissionAdmin(t *testing.T) {
	handler := RequirePermission("trade")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodPost, "/v1/orders", nil)
	req.Header.Set("X-User-Roles", "admin")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestRequirePermissionReadDefault(t *testing.T) {
	handler := RequirePermission("read")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for read, got %d", rr.Code)
	}
}

func TestRequirePermissionDenied(t *testing.T) {
	handler := RequirePermission("trade")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodPost, "/v1/orders", nil)
	req.Header.Set("X-User-Roles", "read")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}
