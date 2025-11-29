package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func TestStoreAddAndValidate(t *testing.T) {
	s := NewStore()
	s.Add(&APIKey{Key: "test-key", Name: "test", OrgID: "org1", Permissions: []string{"read"}})

	ak, ok := s.Validate("test-key")
	if !ok {
		t.Fatal("expected valid key")
	}
	if ak.Name != "test" {
		t.Fatalf("expected name 'test', got %q", ak.Name)
	}
}

func TestStoreValidateUnknownKey(t *testing.T) {
	s := NewStore()
	s.Add(&APIKey{Key: "real-key"})

	_, ok := s.Validate("wrong-key")
	if ok {
		t.Fatal("expected invalid for unknown key")
	}
}

func TestStoreValidateExpiredKey(t *testing.T) {
	s := NewStore()
	past := time.Now().Add(-time.Hour)
	s.Add(&APIKey{Key: "expired", ExpiresAt: &past})

	_, ok := s.Validate("expired")
	if ok {
		t.Fatal("expected invalid for expired key")
	}
}

func TestStoreValidateNotYetExpiredKey(t *testing.T) {
	s := NewStore()
	future := time.Now().Add(time.Hour)
	s.Add(&APIKey{Key: "valid", Name: "ok", ExpiresAt: &future})

	ak, ok := s.Validate("valid")
	if !ok {
		t.Fatal("expected valid for non-expired key")
	}
	if ak.Name != "ok" {
		t.Fatalf("expected name 'ok', got %q", ak.Name)
	}
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name  string
		perms []string
		check string
		want  bool
	}{
		{"direct match", []string{"read", "trade"}, "trade", true},
		{"admin grants all", []string{"admin"}, "trade", true},
		{"missing perm", []string{"read"}, "trade", false},
		{"empty perms", nil, "read", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ak := &APIKey{Permissions: tt.perms}
			got := ak.HasPermission(tt.check)
			if got != tt.want {
				t.Fatalf("HasPermission(%q) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}

func TestMiddlewareValidKey(t *testing.T) {
	s := NewStore()
	s.Add(&APIKey{Key: "good-key", Name: "test", OrgID: "org-abc"})

	handler := Middleware(s)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer good-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestMiddlewareXAPIKeyHeader(t *testing.T) {
	s := NewStore()
	s.Add(&APIKey{Key: "x-key", Name: "xtest", OrgID: "org-x"})

	handler := Middleware(s)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-API-Key", "x-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 via X-API-Key, got %d", rr.Code)
	}
}

func TestMiddlewareSetsOrgHeader(t *testing.T) {
	s := NewStore()
	s.Add(&APIKey{Key: "hdr-key", Name: "hdr-test", OrgID: "org-hdr"})

	var gotOrgID, gotName string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotOrgID = r.Header.Get("X-Org-ID")
		gotName = r.Header.Get("X-API-Key-Name")
		w.WriteHeader(http.StatusOK)
	})

	handler := Middleware(s)(inner)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer hdr-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if gotOrgID != "org-hdr" {
		t.Fatalf("expected X-Org-ID 'org-hdr', got %q", gotOrgID)
	}
	if gotName != "hdr-test" {
		t.Fatalf("expected X-API-Key-Name 'hdr-test', got %q", gotName)
	}
}

func TestMiddlewareEmptyKeyRejects(t *testing.T) {
	s := NewStore()
	handler := Middleware(s)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestMiddlewareEmptyKeyDevMode(t *testing.T) {
	os.Setenv("BROKER_DEV_MODE", "true")
	defer os.Unsetenv("BROKER_DEV_MODE")

	s := NewStore()
	handler := Middleware(s)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 in dev mode, got %d", rr.Code)
	}
}

func TestMiddlewareWrongKeyRejects(t *testing.T) {
	s := NewStore()
	s.Add(&APIKey{Key: "real-key"})

	handler := Middleware(s)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong key, got %d", rr.Code)
	}
}

func TestMiddlewareHealthzSkipsAuth(t *testing.T) {
	s := NewStore() // empty store, no keys
	handler := Middleware(s)(http.HandlerFunc(okHandler))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for /healthz, got %d", rr.Code)
	}
}

func TestRequirePermissionGranted(t *testing.T) {
	s := NewStore()
	s.Add(&APIKey{Key: "perm-key", Permissions: []string{"trade"}})

	handler := RequirePermission(s, "trade")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/order", nil)
	req.Header.Set("Authorization", "Bearer perm-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 with correct perm, got %d", rr.Code)
	}
}

func TestRequirePermissionDenied(t *testing.T) {
	s := NewStore()
	s.Add(&APIKey{Key: "read-key", Permissions: []string{"read"}})

	handler := RequirePermission(s, "trade")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/order", nil)
	req.Header.Set("Authorization", "Bearer read-key")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

func TestRequirePermissionNoKey(t *testing.T) {
	s := NewStore()
	handler := RequirePermission(s, "trade")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/order", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestRequirePermissionDevModeNoKey(t *testing.T) {
	os.Setenv("BROKER_DEV_MODE", "true")
	defer os.Unsetenv("BROKER_DEV_MODE")

	s := NewStore()
	handler := RequirePermission(s, "trade")(http.HandlerFunc(okHandler))
	req := httptest.NewRequest(http.MethodGet, "/api/order", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 in dev mode, got %d", rr.Code)
	}
}
