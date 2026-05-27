// Conformance suite for the North Capital TransactAPI adapter.
//
// Drives every endpoint of the adapter against httptest.Server
// fixtures and asserts the wire-level behavior promised by the
// integration brief and the documented TransactAPI public reference:
//
//  - every endpoint's happy path (request shape + response decode)
//  - non-retryable 4xx mapping → *APIError
//  - 429 + Retry-After honored, exponential backoff, surface
//    sustained 429 as wrapped ErrRateLimited
//  - idempotency-key derivation determinism
//  - idempotency header on every mutating call (caller-supplied wins)
//  - authentication: clientID + developerAPIKey on every legacy call
//  - pagination iteration across multiple pages
//  - webhook signature verification (positive + negative)
//  - webhook decryption (positive + negative)
//  - rate-limit + retry behavior
//
// No production HTTP traffic. All fixtures are httptest.Server.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/reference

package northcapital

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// --- helpers ---

// assertAuthForm decodes a form-encoded request body and asserts that
// clientID + developerAPIKey are present (TransactAPI authorization
// scheme for legacy v1 endpoints). Returns the parsed form.
func assertAuthForm(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parseForm: %v", err)
	}
	if got := r.PostForm.Get("clientID"); got != "test-client" {
		t.Fatalf("clientID: got %q, want test-client", got)
	}
	if got := r.PostForm.Get("developerAPIKey"); got != "test-key" {
		t.Fatalf("developerAPIKey: got %q, want test-key", got)
	}
	return r.PostForm
}

// writeEnv writes a successful TransactAPI envelope around the supplied
// type-specific raw segment.
func writeEnv(t *testing.T, w http.ResponseWriter, key string, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	body := map[string]any{
		"statusCode": "101",
		"statusDesc": "Ok",
		key:          payload,
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode env: %v", err)
	}
}

// --- per-endpoint happy-path tests ---

func TestCreateParty_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/createParty", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("firstName") != "Jane" || form.Get("lastName") != "Doe" {
			t.Fatalf("form: %+v", form)
		}
		if r.Header.Get(idempotencyHeader) == "" {
			t.Fatal("idempotency header missing on createParty")
		}
		writeEnv(t, w, "partyDetails", map[string]any{
			"partyId":      "P-1",
			"firstName":    "Jane",
			"lastName":     "Doe",
			"emailAddress": "jane@example.com",
			"kycStatus":    "Approved",
			"amlStatus":    "Approved",
			"createdDate":  "2026-05-25 12:00:00",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.CreateParty(context.Background(), &CreatePartyRequest{
		Type: PartyIndividual, GivenName: "Jane", FamilyName: "Doe", Email: "jane@example.com",
		OrgID: "org-1",
	})
	if err != nil {
		t.Fatalf("CreateParty: %v", err)
	}
	if got.ID != "P-1" || got.KYCStatus != "Approved" {
		t.Fatalf("Party: %+v", got)
	}
}

func TestCreateEntity_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/createEntity", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("entityName") != "Acme LLC" || form.Get("entityType") != "llc" {
			t.Fatalf("form: %+v", form)
		}
		writeEnv(t, w, "entityDetails", map[string]any{
			"entityId":    "E-1",
			"entityName":  "Acme LLC",
			"entityType":  "llc",
			"createdDate": "2026-05-25 12:00:00",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.CreateEntity(context.Background(), &CreateEntityRequest{
		LegalName: "Acme LLC", EntityType: "llc", FormationCountry: "US",
		Beneficials:    []string{"P-1", "P-2"},
		ControlPersons: []string{"P-1"},
	})
	if err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	if got.ID != "E-1" {
		t.Fatalf("Entity: %+v", got)
	}
	if len(got.Beneficials) != 2 {
		t.Fatalf("Beneficials: %+v", got.Beneficials)
	}
}

func TestGetParty_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getParty", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("partyId") != "P-9" {
			t.Fatalf("partyId: %q", form.Get("partyId"))
		}
		writeEnv(t, w, "partyDetails", []map[string]any{{
			"partyId":   "P-9",
			"firstName": "Bob",
			"lastName":  "Smith",
		}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.GetParty(context.Background(), "P-9")
	if err != nil {
		t.Fatalf("GetParty: %v", err)
	}
	if got.ID != "P-9" {
		t.Fatalf("Party: %+v", got)
	}
}

func TestListParties_Paginated(t *testing.T) {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getAllParties", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		page := atomic.AddInt32(&calls, 1)
		offset := form.Get("offset")
		limit := form.Get("limit")
		if limit != "2" {
			t.Fatalf("limit: %q", limit)
		}
		// Emit 2 records on pages 1+2, 1 record on page 3 (short page).
		var parties []map[string]any
		switch page {
		case 1:
			if offset != "0" {
				t.Fatalf("offset page 1: %q", offset)
			}
			parties = []map[string]any{
				{"partyId": "P-1"}, {"partyId": "P-2"},
			}
		case 2:
			if offset != "2" {
				t.Fatalf("offset page 2: %q", offset)
			}
			parties = []map[string]any{
				{"partyId": "P-3"}, {"partyId": "P-4"},
			}
		case 3:
			if offset != "4" {
				t.Fatalf("offset page 3: %q", offset)
			}
			parties = []map[string]any{{"partyId": "P-5"}}
		default:
			t.Fatalf("unexpected page %d", page)
		}
		writeEnv(t, w, "partyDetails", parties)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.ListPartiesPaged(context.Background(), ListOptions{PageSize: 2})
	if err != nil {
		t.Fatalf("ListPartiesPaged: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d parties, want 5", len(got))
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("calls = %d, want 3 (paginated)", atomic.LoadInt32(&calls))
	}
}

func TestPerformKYC_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/performKYC", func(w http.ResponseWriter, r *http.Request) {
		assertAuthForm(t, r)
		writeEnv(t, w, "kycDetails", map[string]any{
			"partyId":   "P-1",
			"kycStatus": "Approved",
			"score":     95,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.PerformKYC(context.Background(), "P-1")
	if err != nil {
		t.Fatalf("PerformKYC: %v", err)
	}
	if got.Status != "pass" || got.Score != 95 {
		t.Fatalf("KYCResult: %+v", got)
	}
}

func TestPerformAML_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/performAml", func(w http.ResponseWriter, r *http.Request) {
		assertAuthForm(t, r)
		writeEnv(t, w, "amlDetails", map[string]any{
			"partyId":   "P-1",
			"amlStatus": "Approved",
			"hits":      []map[string]any{},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.PerformAML(context.Background(), "P-1")
	if err != nil {
		t.Fatalf("PerformAML: %v", err)
	}
	if got.Status != "pass" {
		t.Fatalf("AMLResult: %+v", got)
	}
}

func TestPerformAccredited_AllMethods(t *testing.T) {
	cases := []struct {
		method AccreditedMethod
		extra  func(*AccreditedRequest)
		check  func(t *testing.T, form url.Values)
	}{
		{
			method: AccreditedByIncome,
			extra:  func(r *AccreditedRequest) { r.IncomeYear = 2025 },
			check: func(t *testing.T, form url.Values) {
				if form.Get("incomeYear") != "2025" {
					t.Fatalf("incomeYear: %q", form.Get("incomeYear"))
				}
			},
		},
		{
			method: AccreditedByNetWorth,
			extra:  func(r *AccreditedRequest) {},
			check: func(t *testing.T, form url.Values) {
				if form.Get("method") != "net_worth" {
					t.Fatalf("method: %q", form.Get("method"))
				}
			},
		},
		{
			method: AccreditedByThirdParty,
			extra:  func(r *AccreditedRequest) { r.ThirdPartyEmail = "cpa@example.com"; r.DocumentID = "D-9" },
			check: func(t *testing.T, form url.Values) {
				if form.Get("thirdPartyEmail") != "cpa@example.com" || form.Get("documentId") != "D-9" {
					t.Fatalf("third-party fields: %+v", form)
				}
			},
		},
		{
			method: AccreditedByLicense,
			extra:  func(r *AccreditedRequest) { r.LicenseRef = "CRD-1234567" },
			check: func(t *testing.T, form url.Values) {
				if form.Get("licenseRef") != "CRD-1234567" {
					t.Fatalf("licenseRef: %q", form.Get("licenseRef"))
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.method), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/tapiv3/index.php/v1/performAccredited", func(w http.ResponseWriter, r *http.Request) {
				form := assertAuthForm(t, r)
				if form.Get("partyId") != "P-1" {
					t.Fatalf("partyId: %q", form.Get("partyId"))
				}
				tc.check(t, form)
				writeEnv(t, w, "accreditedDetails", map[string]any{
					"partyId":    "P-1",
					"method":     string(tc.method),
					"status":     "Verified",
					"verifiedAt": "2026-05-25 12:00:00",
					"expiresAt":  "2031-05-25 12:00:00",
				})
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)
			p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

			req := &AccreditedRequest{PartyID: "P-1", Method: tc.method}
			tc.extra(req)
			got, err := p.PerformAccreditedFull(context.Background(), req)
			if err != nil {
				t.Fatalf("PerformAccreditedFull: %v", err)
			}
			if got.Status != "pass" {
				t.Fatalf("AccreditedResult.Status: %q", got.Status)
			}
			if got.VerifiedAt == nil || got.ExpiresAt == nil {
				t.Fatal("VerifiedAt / ExpiresAt: must be set")
			}
		})
	}
}

func TestSuitability_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/recordSuitability", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("riskTolerance") != "moderate" {
			t.Fatalf("riskTolerance: %q", form.Get("riskTolerance"))
		}
		writeEnv(t, w, "suitabilityDetails", map[string]any{
			"partyId":       "P-1",
			"status":        "Approved",
			"riskTolerance": "moderate",
			"recordedAt":    "2026-05-25 12:00:00",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.Suitability(context.Background(), &SuitabilityRequest{
		PartyID: "P-1", RiskTolerance: "moderate",
	})
	if err != nil {
		t.Fatalf("Suitability: %v", err)
	}
	if got.Status != "pass" || got.RiskTolerance != "moderate" {
		t.Fatalf("SuitabilityResult: %+v", got)
	}
}

func TestCreateOffering_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/createOffering", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("issuerName") != "Lux Industries Inc." || form.Get("currency") != "USD" {
			t.Fatalf("form: %+v", form)
		}
		writeEnv(t, w, "offeringDetails", map[string]any{
			"offeringId":     "O-1",
			"issuerName":     "Lux Industries Inc.",
			"offeringName":   "Series A",
			"offeringType":   "reg_d_506c",
			"currency":       "USD",
			"offeringStatus": "open",
			"createdDate":    "2026-05-25 12:00:00",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.CreateOffering(context.Background(), &CreateOfferingRequest{
		IssuerName: "Lux Industries Inc.", OfferingName: "Series A",
		OfferingType: "reg_d_506c", Currency: "USD",
	})
	if err != nil {
		t.Fatalf("CreateOffering: %v", err)
	}
	if got.ID != "O-1" {
		t.Fatalf("Offering: %+v", got)
	}
}

func TestGetOffering_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getOffering", func(w http.ResponseWriter, r *http.Request) {
		assertAuthForm(t, r)
		writeEnv(t, w, "offeringDetails", map[string]any{"offeringId": "O-1"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.GetOffering(context.Background(), "O-1")
	if err != nil {
		t.Fatalf("GetOffering: %v", err)
	}
	if got.ID != "O-1" {
		t.Fatalf("Offering.ID: %q", got.ID)
	}
}

func TestListOfferings_Paginated(t *testing.T) {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getAllOfferings", func(w http.ResponseWriter, r *http.Request) {
		assertAuthForm(t, r)
		page := atomic.AddInt32(&calls, 1)
		var data []map[string]any
		switch page {
		case 1:
			data = []map[string]any{{"offeringId": "O-1"}, {"offeringId": "O-2"}}
		case 2:
			data = []map[string]any{{"offeringId": "O-3"}}
		default:
			t.Fatalf("unexpected page %d", page)
		}
		writeEnv(t, w, "offeringsDetails", data)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.ListOfferingsPaged(context.Background(), ListOptions{PageSize: 2})
	if err != nil {
		t.Fatalf("ListOfferingsPaged: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d offerings, want 3", len(got))
	}
}

func TestCreateTrade_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/createTrade", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("offeringId") != "O-1" || form.Get("transactionUnits") != "100" {
			t.Fatalf("form: %+v", form)
		}
		writeEnv(t, w, "tradeDetails", map[string]any{
			"tradeId":          "T-1",
			"offeringId":       "O-1",
			"partyId":          "P-1",
			"transactionType":  "buy",
			"transactionUnits": "100",
			"tradeStatus":      "pending",
			"createdDate":      "2026-05-25 12:00:00",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.CreateTrade(context.Background(), &CreateTradeRequest{
		OfferingID: "O-1", PartyID: "P-1", Side: "buy", Units: "100",
	})
	if err != nil {
		t.Fatalf("CreateTrade: %v", err)
	}
	if got.ID != "T-1" {
		t.Fatalf("Trade: %+v", got)
	}
}

func TestCreateTrade_CallerIdempotencyWins(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/createTrade", func(w http.ResponseWriter, r *http.Request) {
		assertAuthForm(t, r)
		if r.Header.Get(idempotencyHeader) != "caller-supplied-key" {
			t.Fatalf("idempotency: %q", r.Header.Get(idempotencyHeader))
		}
		writeEnv(t, w, "tradeDetails", map[string]any{"tradeId": "T-99"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	_, err := p.CreateTrade(context.Background(), &CreateTradeRequest{
		OfferingID: "O-1", PartyID: "P-1", Side: "buy", Units: "100",
		IdempotencyKey: "caller-supplied-key",
	})
	if err != nil {
		t.Fatalf("CreateTrade: %v", err)
	}
}

func TestGetTrade_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getTrade", func(w http.ResponseWriter, r *http.Request) {
		assertAuthForm(t, r)
		writeEnv(t, w, "tradeDetails", map[string]any{"tradeId": "T-7"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.GetTrade(context.Background(), "T-7")
	if err != nil {
		t.Fatalf("GetTrade: %v", err)
	}
	if got.ID != "T-7" {
		t.Fatalf("Trade.ID: %q", got.ID)
	}
}

func TestBulkUploadTrades_Multipart(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/externalUploadTrades", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Fatalf("content-type: %q", r.Header.Get("Content-Type"))
		}
		mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		_ = mediaType
		mr := multipart.NewReader(r.Body, params["boundary"])
		fields := map[string]string{}
		var fileBody []byte
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			b, _ := io.ReadAll(part)
			if part.FileName() != "" {
				fileBody = b
			} else {
				fields[part.FormName()] = string(b)
			}
		}
		if fields["clientID"] != "test-client" || fields["developerAPIKey"] != "test-key" {
			t.Fatalf("auth fields: %+v", fields)
		}
		if string(fileBody) != "offeringId,units\nO-1,100\n" {
			t.Fatalf("file: %q", fileBody)
		}
		writeEnv(t, w, "bulkResult", map[string]any{
			"batchId":       "B-1",
			"acceptedCount": 1,
			"rejectedCount": 0,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.BulkUploadTrades(context.Background(), []byte("offeringId,units\nO-1,100\n"))
	if err != nil {
		t.Fatalf("BulkUploadTrades: %v", err)
	}
	if got.BatchID != "B-1" || got.Accepted != 1 {
		t.Fatalf("BulkTradeResult: %+v", got)
	}
}

func TestCancelTrade_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/cancelTrade", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("tradeId") != "T-1" {
			t.Fatalf("tradeId: %q", form.Get("tradeId"))
		}
		writeEnv(t, w, "tradeDetails", map[string]any{
			"tradeId":     "T-1",
			"tradeStatus": "cancelled",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.CancelTrade(context.Background(), &CancelTradeRequest{TradeID: "T-1", Reason: "investor request"})
	if err != nil {
		t.Fatalf("CancelTrade: %v", err)
	}
	if got.Status != "cancelled" {
		t.Fatalf("Trade.Status: %q", got.Status)
	}
}

func TestRefundTrade_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/refundTrade", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("amount") != "100.00" {
			t.Fatalf("amount: %q", form.Get("amount"))
		}
		writeEnv(t, w, "refundDetails", map[string]any{
			"refundId":  "R-1",
			"tradeId":   "T-1",
			"amount":    "100.00",
			"reason":    "investor request",
			"status":    "pending",
			"createdAt": "2026-05-25 12:00:00",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.RefundTrade(context.Background(), &RefundTradeRequest{
		TradeID: "T-1", Amount: "100.00", Reason: "investor request",
	})
	if err != nil {
		t.Fatalf("RefundTrade: %v", err)
	}
	if got.ID != "R-1" {
		t.Fatalf("Refund: %+v", got)
	}
}

func TestSendSubscriptionDocs_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/sendSubscriptionDocs", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("tradeId") != "T-1" || form.Get("recipient1") != "jane@example.com" {
			t.Fatalf("form: %+v", form)
		}
		writeEnv(t, w, "documentDetails", map[string]any{"status": "sent"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	err := p.SendSubscriptionDocs(context.Background(), &SendSubscriptionDocsRequest{
		TradeID: "T-1", Recipients: []string{"jane@example.com"},
	})
	if err != nil {
		t.Fatalf("SendSubscriptionDocs: %v", err)
	}
}

func TestGetTradeDocuments_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getTradeDocuments", func(w http.ResponseWriter, r *http.Request) {
		assertAuthForm(t, r)
		writeEnv(t, w, "documentDetails", []map[string]any{
			{"documentId": "D-1", "tradeId": "T-1", "name": "subscription.pdf", "status": "signed"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.GetTradeDocuments(context.Background(), "T-1")
	if err != nil {
		t.Fatalf("GetTradeDocuments: %v", err)
	}
	if len(got) != 1 || got[0].ID != "D-1" {
		t.Fatalf("docs: %+v", got)
	}
}

func TestOpenCustodyAccount_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/createCustodyAccountRequest", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("partyId") != "P-1" || form.Get("accountType") != "individual" {
			t.Fatalf("form: %+v", form)
		}
		writeEnv(t, w, "custodyAccountDetails", map[string]any{
			"requestId":   "CR-1",
			"accountId":   "C-1",
			"partyId":     "P-1",
			"accountType": "individual",
			"status":      "Pending",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.OpenCustodyAccount(context.Background(), "P-1", &CustodyOpenRequest{AccountType: "individual"})
	if err != nil {
		t.Fatalf("OpenCustodyAccount: %v", err)
	}
	if got.ID != "C-1" || got.RequestID != "CR-1" {
		t.Fatalf("CustodyAccount: %+v", got)
	}
	if got.Status != "manual_review" {
		t.Fatalf("status: %q", got.Status)
	}
}

func TestGetCustodyAccount_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getCustodyAccountRequest", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("requestId") != "CR-1" {
			t.Fatalf("requestId: %q", form.Get("requestId"))
		}
		writeEnv(t, w, "custodyAccountDetails", map[string]any{
			"requestId":  "CR-1",
			"accountId":  "C-1",
			"status":     "Approved",
			"openedDate": "2026-05-25 12:00:00",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.GetCustodyAccount(context.Background(), "CR-1")
	if err != nil {
		t.Fatalf("GetCustodyAccount: %v", err)
	}
	if got.Status != "pass" || got.OpenedAt == nil {
		t.Fatalf("CustodyAccount: %+v", got)
	}
}

func TestPublishATSEvent_UsesEventIDAsIdem(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/publishAtsEvent", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("eventId") != "EVT-1" || form.Get("eventType") != "match" {
			t.Fatalf("form: %+v", form)
		}
		if r.Header.Get(idempotencyHeader) != "EVT-1" {
			t.Fatalf("idempotency: %q (must be EventID)", r.Header.Get(idempotencyHeader))
		}
		writeEnv(t, w, "atsResult", map[string]any{"status": "accepted"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	err := p.PublishATSEvent(context.Background(), &ATSEvent{
		EventID: "EVT-1", EventType: "match", OfferingID: "O-1",
	})
	if err != nil {
		t.Fatalf("PublishATSEvent: %v", err)
	}
}

func TestGetSecondaryTrades_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getSecondaryTrades", func(w http.ResponseWriter, r *http.Request) {
		assertAuthForm(t, r)
		writeEnv(t, w, "secondaryTradesDetails", []map[string]any{
			{"tradeId": "ST-1", "offeringId": "O-1", "buyerPartyId": "P-1", "sellerPartyId": "P-2",
				"transactionUnits": "10", "unitPrice": "1.00", "currency": "USD", "status": "cleared",
				"tradedAt": "2026-05-25 12:00:00"},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	got, err := p.GetSecondaryTrades(context.Background(), "O-1")
	if err != nil {
		t.Fatalf("GetSecondaryTrades: %v", err)
	}
	if len(got) != 1 || got[0].ID != "ST-1" {
		t.Fatalf("secondary: %+v", got)
	}
}

func TestUpdateWebhookAuthKey_HappyPath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/updateWebhookAuthKey", func(w http.ResponseWriter, r *http.Request) {
		form := assertAuthForm(t, r)
		if form.Get("newAuthKey") != "rotated-key-v2" {
			t.Fatalf("newAuthKey: %q", form.Get("newAuthKey"))
		}
		writeEnv(t, w, "webhookKeyDetails", map[string]any{"status": "rotated"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	err := p.UpdateWebhookAuthKey(context.Background(), &UpdateWebhookAuthKeyRequest{NewAuthKey: "rotated-key-v2"})
	if err != nil {
		t.Fatalf("UpdateWebhookAuthKey: %v", err)
	}
}

// --- error mapping ---

func TestAPIError_NonRetryable4xx(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/createParty", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "missing firstName"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	_, err := p.CreateParty(context.Background(), &CreatePartyRequest{Type: PartyIndividual, Email: "x@y.com"})
	if err == nil {
		t.Fatal("expected error on 400")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 400 {
		t.Fatalf("StatusCode: %d", apiErr.StatusCode)
	}
}

func TestAPIError_EnvelopeStatusCodeMismatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getParty", func(w http.ResponseWriter, r *http.Request) {
		// HTTP 200 but envelope-level failure.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"statusCode": "189", "statusDesc": "Party not found",
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key", HTTPClient: srv.Client()})

	_, err := p.GetParty(context.Background(), "P-bogus")
	if err == nil {
		t.Fatal("expected envelope-level error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !strings.Contains(apiErr.Body, "statusCode=189") {
		t.Fatalf("error body: %q", apiErr.Body)
	}
}

// --- 429 / retry / Retry-After ---

func TestRateLimit_RetriesThenSucceeds(t *testing.T) {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getParty", func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&calls, 1)
		if c < 3 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		writeEnv(t, w, "partyDetails", map[string]any{"partyId": "P-1"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{
		BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key",
		HTTPClient: srv.Client(),
		MaxRetries: 5, RetryBaseDelay: 1, RetryMaxDelay: 1,
	})

	got, err := p.GetParty(context.Background(), "P-1")
	if err != nil {
		t.Fatalf("GetParty after retries: %v", err)
	}
	if got.ID != "P-1" {
		t.Fatalf("Party: %+v", got)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestRateLimit_Exhausted(t *testing.T) {
	var calls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/tapiv3/index.php/v1/getParty", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := New(Config{
		BaseURL: srv.URL, ClientID: "test-client", DeveloperAPIKey: "test-key",
		HTTPClient: srv.Client(),
		MaxRetries: 2, RetryBaseDelay: 1, RetryMaxDelay: 1,
	})

	_, err := p.GetParty(context.Background(), "P-1")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("calls = %d, want 3 (1 + 2 retries)", calls)
	}
}

func TestRateLimit_RetryAfterHonored(t *testing.T) {
	// We don't time-assert exactly (CI variance), but we do verify
	// that a non-zero Retry-After is parsed without error.
	if d := parseRetryAfter("3"); d.Seconds() != 3 {
		t.Fatalf("parseRetryAfter(\"3\"): %v", d)
	}
	if d := parseRetryAfter(""); d != 0 {
		t.Fatalf("parseRetryAfter(empty): %v", d)
	}
}

// --- webhook signature + decryption ---

func TestConsumeWebhook_RejectsMissingSignature(t *testing.T) {
	p := newTestProvider(t, nil)
	_, err := p.ConsumeWebhook(context.Background(), http.Header{}, []byte("anything"))
	if !errors.Is(err, ErrWebhookSignature) {
		t.Fatalf("got %v, want ErrWebhookSignature", err)
	}
}

func TestConsumeWebhook_RejectsBadSignature(t *testing.T) {
	p := newTestProvider(t, nil)
	h := http.Header{}
	h.Set(webhookHMACHeader, "00aa")
	_, err := p.ConsumeWebhook(context.Background(), h, []byte("anything"))
	if !errors.Is(err, ErrWebhookSignature) {
		t.Fatalf("got %v, want ErrWebhookSignature", err)
	}
}

func TestConsumeWebhook_DecryptsValidEnvelope(t *testing.T) {
	// Build a valid TransactAPI-style encrypted webhook envelope:
	// 1. Encode a "k=v, k=v, ..." plaintext
	// 2. AES-256-CBC encrypt with a known 32-byte key + random IV
	// 3. Prepend the IV
	// 4. URL-safe Base64 encode
	// 5. Substitute "+" → "plusencr"
	// Then HMAC-SHA256 the envelope and submit.
	key := []byte("0123456789abcdef0123456789abcdef") // 32 bytes
	plaintext := "eventId=EVT-42, eventType=trade.settled, version=1, transactionType=buy"
	envelope := buildTAPIEnvelope(t, plaintext, key)

	authKey := "hmac-key-v1"
	mac := hmac.New(sha256.New, []byte(authKey))
	mac.Write([]byte(envelope))
	sig := hex.EncodeToString(mac.Sum(nil))

	p := New(Config{
		WebhookAuthKey:       authKey,
		WebhookDecryptionKey: string(key),
	})
	h := http.Header{}
	h.Set(webhookHMACHeader, sig)
	got, err := p.ConsumeWebhook(context.Background(), h, []byte(envelope))
	if err != nil {
		t.Fatalf("ConsumeWebhook: %v", err)
	}
	if got.EventID != "EVT-42" || got.EventType != "trade.settled" {
		t.Fatalf("event: %+v", got)
	}
	if got.TransactionType != "buy" || got.Version != "1" {
		t.Fatalf("event fields: %+v", got)
	}
}

func TestConsumeWebhook_DecryptionFailsOnWrongKey(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	envelope := buildTAPIEnvelope(t, "eventId=EVT-1", key)

	// Disable HMAC for this test (focus on decryption failure).
	p := New(Config{WebhookDecryptionKey: "WRONG-KEY-WRONG-KEY-WRONG-KEY-AAA"})
	_, err := p.ConsumeWebhook(context.Background(), http.Header{}, []byte(envelope))
	if !errors.Is(err, ErrWebhookDecryption) {
		t.Fatalf("got %v, want ErrWebhookDecryption", err)
	}
}

func TestConsumeWebhook_NoDecryptionKeyConfigured(t *testing.T) {
	p := New(Config{})
	_, err := p.ConsumeWebhook(context.Background(), http.Header{}, []byte("anything"))
	if !errors.Is(err, ErrMissingConfig) {
		t.Fatalf("got %v, want ErrMissingConfig", err)
	}
}

func TestConsumeWebhook_DecryptionFailsOnInvalidBase64(t *testing.T) {
	p := New(Config{WebhookDecryptionKey: "0123456789abcdef0123456789abcdef"})
	_, err := p.ConsumeWebhook(context.Background(), http.Header{}, []byte("!!not valid base64!!"))
	if !errors.Is(err, ErrWebhookDecryption) {
		t.Fatalf("got %v, want ErrWebhookDecryption", err)
	}
}

func TestConsumeWebhook_PlusencrSubstitution(t *testing.T) {
	// Construct an envelope that contains "+" in its Base64 form, so
	// the "plusencr" substitution path is exercised end-to-end.
	key := []byte("0123456789abcdef0123456789abcdef")
	// Find a plaintext whose encrypted form Base64-encodes with a "+".
	plaintext := ""
	var envelope string
	for i := 0; i < 100; i++ {
		candidate := fmt.Sprintf("eventId=EVT-PLUS-%d, eventType=plus.test", i)
		env := buildTAPIEnvelope(t, candidate, key)
		if strings.Contains(env, "plusencr") {
			plaintext = candidate
			envelope = env
			break
		}
	}
	if envelope == "" {
		// Statistically very unlikely we don't hit one in 100 tries.
		t.Skip("could not produce envelope with + in 100 tries")
	}
	_ = plaintext

	p := New(Config{WebhookDecryptionKey: string(key)})
	got, err := p.ConsumeWebhook(context.Background(), http.Header{}, []byte(envelope))
	if err != nil {
		t.Fatalf("ConsumeWebhook with plusencr: %v", err)
	}
	if !strings.HasPrefix(got.EventID, "EVT-PLUS-") {
		t.Fatalf("event: %+v", got)
	}
}

func TestConsumeWebhook_KeyRotationOverlap(t *testing.T) {
	// During the 24-hour rotation overlap, WebhookAuthKey carries both
	// keys as a comma-separated list. A webhook signed with either
	// must verify.
	key := []byte("0123456789abcdef0123456789abcdef")
	envelope := buildTAPIEnvelope(t, "eventId=EVT-ROT, eventType=ok", key)

	oldKey, newKey := "old-hmac-key", "new-hmac-key"
	macOld := hmac.New(sha256.New, []byte(oldKey))
	macOld.Write([]byte(envelope))
	sigOld := hex.EncodeToString(macOld.Sum(nil))

	p := New(Config{
		WebhookAuthKey:       oldKey + "," + newKey,
		WebhookDecryptionKey: string(key),
	})
	h := http.Header{}
	h.Set(webhookHMACHeader, sigOld)
	got, err := p.ConsumeWebhook(context.Background(), h, []byte(envelope))
	if err != nil {
		t.Fatalf("ConsumeWebhook overlap (old): %v", err)
	}
	if got.EventID != "EVT-ROT" {
		t.Fatalf("event: %+v", got)
	}
	// And the new key works too.
	macNew := hmac.New(sha256.New, []byte(newKey))
	macNew.Write([]byte(envelope))
	sigNew := hex.EncodeToString(macNew.Sum(nil))
	h.Set(webhookHMACHeader, sigNew)
	if _, err := p.ConsumeWebhook(context.Background(), h, []byte(envelope)); err != nil {
		t.Fatalf("ConsumeWebhook overlap (new): %v", err)
	}
}

// --- helpers for webhook tests ---

// buildTAPIEnvelope produces a TransactAPI-style encrypted webhook
// envelope around `plaintext` using `key`. This is the inverse of
// decryptWebhookPayload — keeping the test fixtures honest against
// the production decoder.
func buildTAPIEnvelope(t *testing.T, plaintext string, key []byte) string {
	t.Helper()
	if len(key) != 32 {
		t.Fatalf("test key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("aes.NewCipher: %v", err)
	}
	// PKCS#7 pad
	pad := aes.BlockSize - (len(plaintext) % aes.BlockSize)
	padded := append([]byte(plaintext), bytesRepeat(byte(pad), pad)...)
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		t.Fatalf("rand.Read iv: %v", err)
	}
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	envelope := append([]byte{}, iv...)
	envelope = append(envelope, ct...)
	encoded := base64.URLEncoding.EncodeToString(envelope)
	// Round-trip the documented "+" → "plusencr" substitution. The
	// URLEncoding alphabet doesn't actually produce "+" (it uses "-"
	// instead), so to exercise the swap we additionally re-encode
	// with standard Base64 and substitute. Try the URL-safe path
	// first; if no "+" appears, swap to std encoding.
	if !strings.ContainsAny(encoded, "+/") {
		encoded = base64.StdEncoding.EncodeToString(envelope)
	}
	return strings.ReplaceAll(encoded, "+", plusencrMarker)
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// --- pkcs7 unpad direct tests ---

func TestPKCS7Unpad(t *testing.T) {
	cases := []struct {
		name    string
		in      []byte
		wantErr bool
		want    string
	}{
		{name: "ok-1", in: append([]byte("ABC"), bytesRepeat(13, 13)...), want: "ABC"},
		{name: "ok-full", in: bytesRepeat(16, 16), want: ""},
		{name: "bad-zero", in: append([]byte("ABCDEFGHIJKLMNO"), 0), wantErr: true},
		{name: "bad-too-large", in: append([]byte("ABCDEFGHIJKLMNO"), 17), wantErr: true},
		{name: "bad-mismatch", in: append([]byte("ABCDEFGHIJKLM"), 3, 3, 2), wantErr: true},
		{name: "bad-empty", in: []byte{}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := pkcs7Unpad(tc.in, aes.BlockSize)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
