package alpaca

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/luxfi/broker/pkg/types"
)

func TestACATSDisclosureNotEmpty(t *testing.T) {
	if ACATSDisclosure == "" {
		t.Fatal("ACATSDisclosure constant must not be empty")
	}
	if !strings.Contains(ACATSDisclosure, "Alpaca Securities LLC") {
		t.Fatal("ACATSDisclosure must mention Alpaca Securities LLC")
	}
}

func TestValidateACATSAssets(t *testing.T) {
	tests := []struct {
		name    string
		assets  []ACATSAsset
		wantErr string
	}{
		{
			name:    "empty assets",
			assets:  nil,
			wantErr: "at least one asset",
		},
		{
			name:    "crypto symbol",
			assets:  []ACATSAsset{{Symbol: "BTC/USD", Qty: "1"}},
			wantErr: "crypto",
		},
		{
			name:    "options OCC symbol",
			assets:  []ACATSAsset{{Symbol: "AAPL260418C00150000", Qty: "1"}},
			wantErr: "options",
		},
		{
			name:    "fractional shares",
			assets:  []ACATSAsset{{Symbol: "AAPL", Qty: "1.5"}},
			wantErr: "whole shares",
		},
		{
			name:    "zero qty",
			assets:  []ACATSAsset{{Symbol: "AAPL", Qty: "0"}},
			wantErr: "positive",
		},
		{
			name:    "negative qty",
			assets:  []ACATSAsset{{Symbol: "AAPL", Qty: "-5"}},
			wantErr: "positive",
		},
		{
			name:    "invalid qty",
			assets:  []ACATSAsset{{Symbol: "AAPL", Qty: "abc"}},
			wantErr: "positive",
		},
		{
			name:   "valid single asset",
			assets: []ACATSAsset{{Symbol: "AAPL", Qty: "10"}},
		},
		{
			name: "valid multiple assets",
			assets: []ACATSAsset{
				{Symbol: "AAPL", Qty: "10"},
				{Symbol: "MSFT", Qty: "5"},
				{Symbol: "GOOG", Qty: "1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateACATSAssets(tt.assets)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateACATSAccount_Active(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "acct-1",
				"account_number": "123",
				"status":         "ACTIVE",
				"currency":       "USD",
			})
		},
	})

	err := p.ValidateACATSAccount(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("expected no error for ACTIVE account, got: %v", err)
	}
}

func TestValidateACATSAccount_Rejected(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-2": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":             "acct-2",
				"account_number": "456",
				"status":         "SUBMITTED",
				"currency":       "USD",
			})
		},
	})

	err := p.ValidateACATSAccount(context.Background(), "acct-2")
	if err == nil {
		t.Fatal("expected error for SUBMITTED account")
	}
	if !strings.Contains(err.Error(), "not eligible") {
		t.Fatalf("error %q does not mention eligibility", err)
	}
}

func TestCreateACATSTransfer_Full(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "acct-1",
				"status": "ACTIVE",
			})
		},
		"/v1/accounts/acct-1/transfers": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":                   "xfer-1",
				"account_id":          "acct-1",
				"direction":           "INCOMING",
				"status":              "QUEUED",
				"type":                "ACATS",
				"transfer_type":       "FULL",
				"contra_account_number": "999-888",
				"contra_broker_number":  "0123",
				"created_at":          "2026-04-05T10:00:00Z",
				"updated_at":          "2026-04-05T10:00:00Z",
			})
		},
	})

	transfer, err := p.CreateACATSTransfer(context.Background(), "acct-1", &types.CreateACATSTransferRequest{
		ContraAccount: "999-888",
		ContraBroker:  "0123",
		Type:          "FULL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transfer.ID != "xfer-1" {
		t.Fatalf("ID = %q, want xfer-1", transfer.ID)
	}
	if transfer.Status != "QUEUED" {
		t.Fatalf("Status = %q, want QUEUED", transfer.Status)
	}
	if transfer.Type != "ACATS" {
		t.Fatalf("Type = %q, want ACATS", transfer.Type)
	}
}

func TestCreateACATSTransfer_PartialValidationFails(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "acct-1",
				"status": "ACTIVE",
			})
		},
	})

	_, err := p.CreateACATSTransfer(context.Background(), "acct-1", &types.CreateACATSTransferRequest{
		ContraAccount: "999-888",
		ContraBroker:  "0123",
		Type:          "PARTIAL",
		Assets:        []types.ACATSAsset{{Symbol: "BTC/USD", Qty: "1"}},
	})
	if err == nil {
		t.Fatal("expected error for crypto asset in ACATS")
	}
	if !strings.Contains(err.Error(), "crypto") {
		t.Fatalf("error %q does not mention crypto", err)
	}
}

func TestGetACATSTransfer(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1/transfers/xfer-1": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "xfer-1",
				"account_id":   "acct-1",
				"status":        "APPROVED",
				"direction":     "INCOMING",
				"type":          "ACATS",
				"reject_reason": "",
				"created_at":    "2026-04-05T10:00:00Z",
				"updated_at":    "2026-04-05T10:30:00Z",
			})
		},
	})

	transfer, err := p.GetACATSTransfer(context.Background(), "acct-1", "xfer-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transfer.Status != "APPROVED" {
		t.Fatalf("Status = %q, want APPROVED", transfer.Status)
	}
}

func TestGetACATSRejection(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1/transfers/xfer-rej": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":            "xfer-rej",
				"account_id":   "acct-1",
				"status":        "REJECTED",
				"direction":     "INCOMING",
				"type":          "ACATS",
				"reject_reason": "Account number mismatch at contra broker",
				"created_at":    "2026-04-05T10:00:00Z",
				"updated_at":    "2026-04-05T11:00:00Z",
			})
		},
	})

	reason, err := p.GetACATSRejection(context.Background(), "acct-1", "xfer-rej")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reason != "Account number mismatch at contra broker" {
		t.Fatalf("reason = %q, want account number mismatch", reason)
	}
}

func TestGetACATSRejection_NotRejected(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1/transfers/xfer-ok": func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":         "xfer-ok",
				"account_id": "acct-1",
				"status":     "APPROVED",
				"direction":  "INCOMING",
				"type":       "ACATS",
				"created_at": "2026-04-05T10:00:00Z",
				"updated_at": "2026-04-05T10:30:00Z",
			})
		},
	})

	_, err := p.GetACATSRejection(context.Background(), "acct-1", "xfer-ok")
	if err == nil {
		t.Fatal("expected error for non-rejected transfer")
	}
	if !strings.Contains(err.Error(), "not rejected") {
		t.Fatalf("error %q does not mention 'not rejected'", err)
	}
}

func TestListACATSTransfers(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1/transfers": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("type") != "ACATS" {
				http.Error(w, "missing type=ACATS", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "xfer-1", "status": "COMPLETED", "type": "ACATS", "created_at": "2026-04-01T00:00:00Z", "updated_at": "2026-04-03T00:00:00Z"},
				{"id": "xfer-2", "status": "QUEUED", "type": "ACATS", "created_at": "2026-04-05T00:00:00Z", "updated_at": "2026-04-05T00:00:00Z"},
			})
		},
	})

	transfers, err := p.ListACATSTransfers(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(transfers) != 2 {
		t.Fatalf("got %d transfers, want 2", len(transfers))
	}
	if transfers[0].ID != "xfer-1" || transfers[1].ID != "xfer-2" {
		t.Fatalf("unexpected transfer IDs: %s, %s", transfers[0].ID, transfers[1].ID)
	}
}

func TestListACATSActivities(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1/activities": func(w http.ResponseWriter, r *http.Request) {
			actType := r.URL.Query().Get("activity_type")
			if !strings.Contains(actType, "ACATC") || !strings.Contains(actType, "ACATS") {
				http.Error(w, "expected activity_type=ACATC,ACATS", http.StatusBadRequest)
				return
			}
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"id": "act-1", "activity_type": "ACATC", "account_id": "acct-1", "date": "2026-04-03"},
				{"id": "act-2", "activity_type": "ACATS", "account_id": "acct-1", "date": "2026-04-05"},
			})
		},
	})

	activities, err := p.ListACATSActivities(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activities) != 2 {
		t.Fatalf("got %d activities, want 2", len(activities))
	}
	if activities[0].ActivityType != "ACATC" {
		t.Fatalf("first activity type = %q, want ACATC", activities[0].ActivityType)
	}
	if activities[1].ActivityType != "ACATS" {
		t.Fatalf("second activity type = %q, want ACATS", activities[1].ActivityType)
	}
}

func TestCancelACATSTransfer(t *testing.T) {
	_, p := testServer(t, map[string]http.HandlerFunc{
		"/v1/accounts/acct-1/transfers/xfer-1": func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		},
	})

	err := p.CancelACATSTransfer(context.Background(), "acct-1", "xfer-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseACATSTransfer_TransferTypeFallback(t *testing.T) {
	data := `{
		"id": "xfer-3",
		"account_id": "acct-1",
		"status": "QUEUED",
		"direction": "INCOMING",
		"type": "",
		"transfer_type": "PARTIAL",
		"assets": [{"symbol": "AAPL", "qty": "10", "status": "QUEUED"}],
		"created_at": "2026-04-05T10:00:00Z",
		"updated_at": "2026-04-05T10:00:00Z"
	}`
	transfer, err := parseACATSTransfer([]byte(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if transfer.Type != "PARTIAL" {
		t.Fatalf("Type = %q, want PARTIAL (from transfer_type fallback)", transfer.Type)
	}
	if len(transfer.Assets) != 1 {
		t.Fatalf("got %d assets, want 1", len(transfer.Assets))
	}
	if transfer.Assets[0].Symbol != "AAPL" {
		t.Fatalf("asset symbol = %q, want AAPL", transfer.Assets[0].Symbol)
	}
}
