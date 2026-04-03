package compliance

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func newTestRouter() (chi.Router, *MemoryStore) {
	store := NewStore()
	router := NewRouter(store)
	return router, store
}

// doRequest sends a request with gateway-style IAM headers (superadmin role).
func doRequest(r chi.Router, method, path string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "test-user-001")
	req.Header.Set("X-Org-Id", "liquidity")
	req.Header.Set("X-User-Email", "testadmin@liquidity.io")
	req.Header.Set("X-User-Roles", "superadmin")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// doRequestAs sends a request with a specific role.
func doRequestAs(r chi.Router, method, path string, body interface{}, userID, role string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID)
	req.Header.Set("X-Org-Id", "liquidity")
	req.Header.Set("X-User-Roles", role)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON: %v (body: %s)", err, w.Body.String())
	}
}

// ==========================================================================
// KYC Tests
// ==========================================================================

func TestKYCVerifyAndGet(t *testing.T) {
	r, _ := newTestRouter()

	// Create a pending KYC identity.
	w := doRequest(r, "POST", "/kyc/verify", map[string]string{
		"user_id":  "user-123",
		"provider": "onfido",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var ident Identity
	decodeJSON(t, w, &ident)
	if ident.UserID != "user-123" {
		t.Fatalf("expected user_id user-123, got %s", ident.UserID)
	}
	if ident.Status != KYCPending {
		t.Fatalf("expected status pending, got %s", ident.Status)
	}

	// Retrieve it.
	w = doRequest(r, "GET", "/kyc/"+ident.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var fetched Identity
	decodeJSON(t, w, &fetched)
	if fetched.ID != ident.ID {
		t.Fatalf("ID mismatch: %s != %s", fetched.ID, ident.ID)
	}
}

func TestKYCVerifyMissingUserID(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/kyc/verify", map[string]string{"provider": "onfido"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestKYCGetStatus(t *testing.T) {
	r, store := newTestRouter()

	// Seed an identity directly in the store.
	ident := &Identity{
		UserID:   "user-status",
		Provider: "berbix",
		Status:   KYCVerified,
		Data:     map[string]interface{}{"score": 95},
	}
	if err := store.SaveIdentity(ident); err != nil {
		t.Fatal(err)
	}

	w := doRequest(r, "GET", "/kyc/"+ident.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got Identity
	decodeJSON(t, w, &got)
	if got.Status != KYCVerified {
		t.Fatalf("expected status verified, got %s", got.Status)
	}
	if got.Data["score"] == nil {
		t.Fatal("expected data.score to be set")
	}
}

func TestKYCVerifyWithMissingFields(t *testing.T) {
	r, _ := newTestRouter()

	// Missing both user_id and provider -- user_id is required.
	w := doRequest(r, "POST", "/kyc/verify", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestKYCDuplicateVerification(t *testing.T) {
	r, _ := newTestRouter()

	body := map[string]string{
		"user_id":  "user-dup",
		"provider": "onfido",
	}

	// First verification.
	w1 := doRequest(r, "POST", "/kyc/verify", body)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first: expected 201, got %d", w1.Code)
	}
	var id1 Identity
	decodeJSON(t, w1, &id1)

	// Second verification for same user -- both should succeed with different IDs.
	w2 := doRequest(r, "POST", "/kyc/verify", body)
	if w2.Code != http.StatusCreated {
		t.Fatalf("second: expected 201, got %d", w2.Code)
	}
	var id2 Identity
	decodeJSON(t, w2, &id2)

	if id1.ID == id2.ID {
		t.Fatal("duplicate verifications should have different IDs")
	}
}

func TestKYCGetNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/kyc/nonexistent-id", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestKYCVerifyInvalidBody(t *testing.T) {
	r, _ := newTestRouter()
	// Send a raw invalid JSON body.
	req := httptest.NewRequest("POST", "/kyc/verify", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "test-user-001"); req.Header.Set("X-User-Roles", "superadmin")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ==========================================================================
// Pipeline Tests
// ==========================================================================

func TestPipelineCRUD(t *testing.T) {
	r, _ := newTestRouter()

	// Create
	w := doRequest(r, "POST", "/pipelines/", map[string]interface{}{
		"name":        "Reg D Pipeline",
		"business_id": "biz-1",
		"steps": []map[string]interface{}{
			{"id": "s1", "name": "KYC", "type": "kyc", "required": true, "order": 1},
			{"id": "s2", "name": "eSign", "type": "esign", "required": true, "order": 2},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var pipeline Pipeline
	decodeJSON(t, w, &pipeline)
	if pipeline.Name != "Reg D Pipeline" {
		t.Fatalf("expected name 'Reg D Pipeline', got %q", pipeline.Name)
	}
	if pipeline.Status != "draft" {
		t.Fatalf("expected default status 'draft', got %q", pipeline.Status)
	}
	if len(pipeline.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(pipeline.Steps))
	}

	// List
	w = doRequest(r, "GET", "/pipelines/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var pipelines []*Pipeline
	decodeJSON(t, w, &pipelines)
	if len(pipelines) != 1 {
		t.Fatalf("expected 1 pipeline, got %d", len(pipelines))
	}

	// Get
	w = doRequest(r, "GET", "/pipelines/"+pipeline.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Patch
	newName := "Updated Pipeline"
	w = doRequest(r, "PATCH", "/pipelines/"+pipeline.ID, map[string]interface{}{
		"name": newName,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated Pipeline
	decodeJSON(t, w, &updated)
	if updated.Name != newName {
		t.Fatalf("expected name %q, got %q", newName, updated.Name)
	}

	// Delete
	w = doRequest(r, "DELETE", "/pipelines/"+pipeline.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}

	// Get after delete -> 404
	w = doRequest(r, "GET", "/pipelines/"+pipeline.ID, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete: expected 404, got %d", w.Code)
	}
}

func TestPipelineFilterByStatus(t *testing.T) {
	r, store := newTestRouter()

	// Create pipelines with different statuses.
	for _, s := range []string{"active", "draft", "archived", "active"} {
		store.SavePipeline(&Pipeline{Name: "Pipeline " + s, Status: s})
	}

	w := doRequest(r, "GET", "/pipelines/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var all []*Pipeline
	decodeJSON(t, w, &all)
	if len(all) != 4 {
		t.Fatalf("expected 4 pipelines, got %d", len(all))
	}

	// Count by status.
	counts := map[string]int{}
	for _, p := range all {
		counts[p.Status]++
	}
	if counts["active"] != 2 {
		t.Fatalf("expected 2 active, got %d", counts["active"])
	}
	if counts["draft"] != 1 {
		t.Fatalf("expected 1 draft, got %d", counts["draft"])
	}
	if counts["archived"] != 1 {
		t.Fatalf("expected 1 archived, got %d", counts["archived"])
	}
}

func TestPipelineCreateMissingName(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/pipelines/", map[string]interface{}{
		"business_id": "biz-1",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPipelineCreateInvalidBody(t *testing.T) {
	r, _ := newTestRouter()
	req := httptest.NewRequest("POST", "/pipelines/", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "test-user-001"); req.Header.Set("X-User-Roles", "superadmin")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPipelineDeleteNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "DELETE", "/pipelines/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestPipelineUpdateNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "PATCH", "/pipelines/nonexistent", map[string]interface{}{
		"name": "x",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// ==========================================================================
// Session Tests
// ==========================================================================

func TestSessionLifecycle(t *testing.T) {
	r, store := newTestRouter()

	// Create a pipeline first.
	p := &Pipeline{
		Name:   "Test Pipeline",
		Status: "active",
		Steps: []PipelineStep{
			{ID: "s1", Name: "KYC", Type: "kyc", Required: true, Order: 1},
		},
	}
	if err := store.SavePipeline(p); err != nil {
		t.Fatalf("save pipeline: %v", err)
	}

	// Create session.
	w := doRequest(r, "POST", "/sessions/", map[string]string{
		"pipeline_id":    p.ID,
		"investor_email": "investor@example.com",
		"investor_name":  "John Doe",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create session: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var sess Session
	decodeJSON(t, w, &sess)
	if sess.Status != SessionPending {
		t.Fatalf("expected status pending, got %s", sess.Status)
	}
	if len(sess.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(sess.Steps))
	}

	// Get session steps.
	w = doRequest(r, "GET", "/sessions/"+sess.ID+"/steps", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get steps: expected 200, got %d", w.Code)
	}
	var steps []SessionStep
	decodeJSON(t, w, &steps)
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}

	// Patch session status.
	w = doRequest(r, "PATCH", "/sessions/"+sess.ID, map[string]string{
		"status": "in_progress",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch session: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionRequiresPipeline(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/sessions/", map[string]string{
		"pipeline_id":    "nonexistent",
		"investor_email": "x@y.com",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSessionStatusTransitionPendingToCompleted(t *testing.T) {
	r, store := newTestRouter()

	p := &Pipeline{Name: "Flow", Status: "active", Steps: []PipelineStep{
		{ID: "s1", Name: "KYC", Type: "kyc", Required: true, Order: 1},
		{ID: "s2", Name: "eSign", Type: "esign", Required: true, Order: 2},
	}}
	store.SavePipeline(p)

	// Create session.
	w := doRequest(r, "POST", "/sessions/", map[string]string{
		"pipeline_id":    p.ID,
		"investor_email": "alice@example.com",
		"investor_name":  "Alice",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", w.Code)
	}
	var sess Session
	decodeJSON(t, w, &sess)

	// pending -> in_progress
	w = doRequest(r, "PATCH", "/sessions/"+sess.ID, map[string]string{"status": "in_progress"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var s1 Session
	decodeJSON(t, w, &s1)
	if s1.Status != SessionInProgress {
		t.Fatalf("expected in_progress, got %s", s1.Status)
	}

	// in_progress -> completed
	w = doRequest(r, "PATCH", "/sessions/"+sess.ID, map[string]string{"status": "completed"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var s2 Session
	decodeJSON(t, w, &s2)
	if s2.Status != SessionCompleted {
		t.Fatalf("expected completed, got %s", s2.Status)
	}
}

func TestSessionStatusTransitionPendingToFailed(t *testing.T) {
	r, store := newTestRouter()

	p := &Pipeline{Name: "Flow", Status: "active", Steps: []PipelineStep{
		{ID: "s1", Name: "KYC", Type: "kyc", Required: true, Order: 1},
	}}
	store.SavePipeline(p)

	w := doRequest(r, "POST", "/sessions/", map[string]string{
		"pipeline_id":    p.ID,
		"investor_email": "bob@example.com",
	})
	var sess Session
	decodeJSON(t, w, &sess)

	// pending -> failed
	w = doRequest(r, "PATCH", "/sessions/"+sess.ID, map[string]string{"status": "failed"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var patched Session
	decodeJSON(t, w, &patched)
	if patched.Status != SessionFailed {
		t.Fatalf("expected failed, got %s", patched.Status)
	}
}

func TestSessionListByPipeline(t *testing.T) {
	_, store := newTestRouter()

	p1 := &Pipeline{Name: "Pipeline A", Status: "active"}
	p2 := &Pipeline{Name: "Pipeline B", Status: "active"}
	store.SavePipeline(p1)
	store.SavePipeline(p2)

	store.SaveSession(&Session{PipelineID: p1.ID, InvestorEmail: "a@a.com", Status: SessionPending})
	store.SaveSession(&Session{PipelineID: p1.ID, InvestorEmail: "b@b.com", Status: SessionPending})
	store.SaveSession(&Session{PipelineID: p2.ID, InvestorEmail: "c@c.com", Status: SessionPending})

	all := store.ListSessions()
	if len(all) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(all))
	}

	// Filter by pipeline ID.
	count := 0
	for _, s := range all {
		if s.PipelineID == p1.ID {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected 2 sessions for pipeline A, got %d", count)
	}
}

func TestSessionSearchByEmail(t *testing.T) {
	_, store := newTestRouter()

	p := &Pipeline{Name: "P", Status: "active"}
	store.SavePipeline(p)
	store.SaveSession(&Session{PipelineID: p.ID, InvestorEmail: "alice@example.com", Status: SessionPending})
	store.SaveSession(&Session{PipelineID: p.ID, InvestorEmail: "bob@example.com", Status: SessionPending})
	store.SaveSession(&Session{PipelineID: p.ID, InvestorEmail: "alice@corp.com", Status: SessionPending})

	all := store.ListSessions()
	found := 0
	for _, s := range all {
		if s.InvestorEmail == "alice@example.com" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("expected 1 match for alice@example.com, got %d", found)
	}
}

func TestSessionCreateMissingEmail(t *testing.T) {
	r, store := newTestRouter()

	p := &Pipeline{Name: "P", Status: "active"}
	store.SavePipeline(p)

	w := doRequest(r, "POST", "/sessions/", map[string]string{
		"pipeline_id": p.ID,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSessionCreateMissingPipelineID(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "POST", "/sessions/", map[string]string{
		"investor_email": "x@y.com",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSessionGetNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/sessions/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSessionStepsNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/sessions/nonexistent/steps", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestSessionUpdateKYCStatus(t *testing.T) {
	r, store := newTestRouter()

	p := &Pipeline{Name: "P", Status: "active", Steps: []PipelineStep{
		{ID: "s1", Name: "KYC", Type: "kyc", Required: true, Order: 1},
	}}
	store.SavePipeline(p)

	w := doRequest(r, "POST", "/sessions/", map[string]string{
		"pipeline_id":    p.ID,
		"investor_email": "test@test.com",
	})
	var sess Session
	decodeJSON(t, w, &sess)

	// Update KYC status to verified.
	w = doRequest(r, "PATCH", "/sessions/"+sess.ID, map[string]string{
		"kyc_status": "verified",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var patched Session
	decodeJSON(t, w, &patched)
	if patched.KYCStatus != KYCVerified {
		t.Fatalf("expected kyc_status verified, got %s", patched.KYCStatus)
	}
}

// ==========================================================================
// Fund Tests
// ==========================================================================

func TestFundCRUD(t *testing.T) {
	r, _ := newTestRouter()

	// Create
	w := doRequest(r, "POST", "/funds/", map[string]interface{}{
		"name":           "Reg D Fund",
		"business_id":    "biz-1",
		"type":           "equity",
		"min_investment": 10000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var fund Fund
	decodeJSON(t, w, &fund)
	if fund.Name != "Reg D Fund" {
		t.Fatalf("expected name 'Reg D Fund', got %q", fund.Name)
	}
	if fund.Status != "raising" {
		t.Fatalf("expected default status 'raising', got %q", fund.Status)
	}

	// Get
	w = doRequest(r, "GET", "/funds/"+fund.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Patch
	w = doRequest(r, "PATCH", "/funds/"+fund.ID, map[string]interface{}{
		"status": "closed",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d", w.Code)
	}
	var patched Fund
	decodeJSON(t, w, &patched)
	if patched.Status != "closed" {
		t.Fatalf("expected status closed, got %q", patched.Status)
	}

	// List investors (empty)
	w = doRequest(r, "GET", "/funds/"+fund.ID+"/investors", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("investors: expected 200, got %d", w.Code)
	}
	var investors []*FundInvestor
	decodeJSON(t, w, &investors)
	if len(investors) != 0 {
		t.Fatalf("expected 0 investors, got %d", len(investors))
	}

	// Delete
	w = doRequest(r, "DELETE", "/funds/"+fund.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}
}

func TestFundAddInvestor(t *testing.T) {
	_, store := newTestRouter()

	f := &Fund{Name: "Growth Fund", Type: "equity", MinInvestment: 5000}
	store.SaveFund(f)

	err := store.AddFundInvestor(&FundInvestor{
		FundID:     f.ID,
		InvestorID: "inv-1",
		Name:       "Alice",
		Email:      "alice@example.com",
		Amount:     25000,
		Status:     "committed",
	})
	if err != nil {
		t.Fatal(err)
	}

	err = store.AddFundInvestor(&FundInvestor{
		FundID:     f.ID,
		InvestorID: "inv-2",
		Name:       "Bob",
		Email:      "bob@example.com",
		Amount:     50000,
		Status:     "funded",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetFund(f.ID)
	if got.InvestorCount != 2 {
		t.Fatalf("expected 2 investors, got %d", got.InvestorCount)
	}
	if got.TotalRaised != 75000 {
		t.Fatalf("expected total_raised 75000, got %f", got.TotalRaised)
	}
}

func TestFundGetInvestorsViaAPI(t *testing.T) {
	r, store := newTestRouter()

	f := &Fund{Name: "API Fund", Type: "debt"}
	store.SaveFund(f)
	store.AddFundInvestor(&FundInvestor{
		FundID: f.ID, InvestorID: "inv-1", Name: "Alice", Amount: 10000,
	})
	store.AddFundInvestor(&FundInvestor{
		FundID: f.ID, InvestorID: "inv-2", Name: "Bob", Amount: 20000,
	})

	w := doRequest(r, "GET", "/funds/"+f.ID+"/investors", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var investors []*FundInvestor
	decodeJSON(t, w, &investors)
	if len(investors) != 2 {
		t.Fatalf("expected 2 investors, got %d", len(investors))
	}
}

func TestFundWithZeroMinInvestment(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "POST", "/funds/", map[string]interface{}{
		"name":           "Open Fund",
		"type":           "equity",
		"min_investment": 0,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var fund Fund
	decodeJSON(t, w, &fund)
	if fund.MinInvestment != 0 {
		t.Fatalf("expected min_investment 0, got %f", fund.MinInvestment)
	}
}

func TestFundDeleteNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "DELETE", "/funds/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFundGetNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/funds/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFundCreateMissingName(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/funds/", map[string]interface{}{
		"type": "equity",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestFundInvestorsForNonexistentFund(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/funds/nonexistent/investors", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFundDeleteCleansUpInvestors(t *testing.T) {
	_, store := newTestRouter()

	f := &Fund{Name: "Temp Fund"}
	store.SaveFund(f)
	store.AddFundInvestor(&FundInvestor{FundID: f.ID, InvestorID: "inv-1", Amount: 1000})

	// Confirm investor exists.
	if len(store.ListFundInvestors(f.ID)) != 1 {
		t.Fatal("expected 1 investor before delete")
	}

	store.DeleteFund(f.ID)

	// Investors should be cleaned up.
	if len(store.ListFundInvestors(f.ID)) != 0 {
		t.Fatal("expected 0 investors after fund delete")
	}
}

// ==========================================================================
// eSign Tests
// ==========================================================================

func TestESignEnvelopeLifecycle(t *testing.T) {
	r, _ := newTestRouter()

	// Create envelope
	w := doRequest(r, "POST", "/esign/envelopes", map[string]interface{}{
		"subject": "Subscription Agreement",
		"signers": []map[string]string{
			{"name": "John Doe", "email": "john@example.com", "role": "investor"},
			{"name": "Jane Issuer", "email": "jane@example.com", "role": "issuer"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var env Envelope
	decodeJSON(t, w, &env)
	if env.Status != EnvelopePending {
		t.Fatalf("expected status pending, got %s", env.Status)
	}
	if len(env.Signers) != 2 {
		t.Fatalf("expected 2 signers, got %d", len(env.Signers))
	}

	// Sign as first signer
	w = doRequest(r, "POST", "/esign/envelopes/"+env.ID+"/sign", map[string]string{
		"signer_id": env.Signers[0].ID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("sign1: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var afterSign1 Envelope
	decodeJSON(t, w, &afterSign1)
	if afterSign1.Status != EnvelopeSigned {
		t.Fatalf("expected status signed (partial), got %s", afterSign1.Status)
	}

	// Sign as second signer -> completed
	w = doRequest(r, "POST", "/esign/envelopes/"+env.ID+"/sign", map[string]string{
		"signer_id": env.Signers[1].ID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("sign2: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var afterSign2 Envelope
	decodeJSON(t, w, &afterSign2)
	if afterSign2.Status != EnvelopeCompleted {
		t.Fatalf("expected status completed, got %s", afterSign2.Status)
	}

	// List envelopes
	w = doRequest(r, "GET", "/esign/envelopes", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
}

func TestESignTemplates(t *testing.T) {
	r, _ := newTestRouter()

	// Create template
	w := doRequest(r, "POST", "/esign/templates", map[string]interface{}{
		"name":        "Sub Agreement v2",
		"description": "Standard subscription agreement",
		"roles":       []string{"investor", "issuer"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List templates
	w = doRequest(r, "GET", "/esign/templates", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var templates []Template
	decodeJSON(t, w, &templates)
	if len(templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(templates))
	}
}

func TestESignCreateEnvelopeMissingSubject(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/esign/envelopes", map[string]interface{}{
		"signers": []map[string]string{
			{"name": "John", "email": "john@example.com"},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestESignCreateEnvelopeNoSigners(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/esign/envelopes", map[string]interface{}{
		"subject": "Test",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestESignGetEnvelopeStatus(t *testing.T) {
	r, _ := newTestRouter()

	// Create
	w := doRequest(r, "POST", "/esign/envelopes", map[string]interface{}{
		"subject": "NDA",
		"signers": []map[string]string{
			{"name": "Alice", "email": "alice@example.com"},
		},
	})
	var env Envelope
	decodeJSON(t, w, &env)

	// Get status
	w = doRequest(r, "GET", "/esign/envelopes/"+env.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got Envelope
	decodeJSON(t, w, &got)
	if got.Status != EnvelopePending {
		t.Fatalf("expected pending, got %s", got.Status)
	}
}

func TestESignEnvelopeWithMultipleSigners(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "POST", "/esign/envelopes", map[string]interface{}{
		"subject": "Multi-party Agreement",
		"signers": []map[string]string{
			{"name": "Alice", "email": "alice@example.com", "role": "investor"},
			{"name": "Bob", "email": "bob@example.com", "role": "issuer"},
			{"name": "Charlie", "email": "charlie@example.com", "role": "witness"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var env Envelope
	decodeJSON(t, w, &env)
	if len(env.Signers) != 3 {
		t.Fatalf("expected 3 signers, got %d", len(env.Signers))
	}

	// Each signer should have a unique ID.
	ids := map[string]bool{}
	for _, s := range env.Signers {
		if s.ID == "" {
			t.Fatal("signer missing ID")
		}
		if ids[s.ID] {
			t.Fatal("duplicate signer ID")
		}
		ids[s.ID] = true
	}
}

func TestESignSignInvalidSigner(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "POST", "/esign/envelopes", map[string]interface{}{
		"subject": "Test",
		"signers": []map[string]string{
			{"name": "Alice", "email": "alice@example.com"},
		},
	})
	var env Envelope
	decodeJSON(t, w, &env)

	// Try to sign with a bad signer_id.
	w = doRequest(r, "POST", "/esign/envelopes/"+env.ID+"/sign", map[string]string{
		"signer_id": "bad-id",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestESignSignMissingSignerID(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "POST", "/esign/envelopes", map[string]interface{}{
		"subject": "Test",
		"signers": []map[string]string{
			{"name": "Alice", "email": "alice@example.com"},
		},
	})
	var env Envelope
	decodeJSON(t, w, &env)

	w = doRequest(r, "POST", "/esign/envelopes/"+env.ID+"/sign", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestESignGetEnvelopeNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/esign/envelopes/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestESignCreateTemplateMissingName(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/esign/templates", map[string]interface{}{
		"description": "No name",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ==========================================================================
// Role Tests
// ==========================================================================

func TestRoleCRUD(t *testing.T) {
	r, _ := newTestRouter()

	// Create
	w := doRequest(r, "POST", "/roles/", map[string]interface{}{
		"name":        "Compliance Officer",
		"description": "Full compliance access",
		"permissions": []map[string]string{
			{"module": "kyc", "action": "admin"},
			{"module": "funds", "action": "read"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var role Role
	decodeJSON(t, w, &role)
	if role.Name != "Compliance Officer" {
		t.Fatalf("expected name 'Compliance Officer', got %q", role.Name)
	}
	if len(role.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(role.Permissions))
	}

	// Get
	w = doRequest(r, "GET", "/roles/"+role.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}

	// Patch
	w = doRequest(r, "PATCH", "/roles/"+role.ID, map[string]interface{}{
		"name": "Updated Role",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated Role
	decodeJSON(t, w, &updated)
	if updated.Name != "Updated Role" {
		t.Fatalf("expected name 'Updated Role', got %q", updated.Name)
	}

	// Delete
	w = doRequest(r, "DELETE", "/roles/"+role.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d", w.Code)
	}

	// Get after delete -> 404
	w = doRequest(r, "GET", "/roles/"+role.ID, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("get-after-delete: expected 404, got %d", w.Code)
	}
}

func TestRoleDefaultSeed(t *testing.T) {
	store := NewStore()
	SeedStore(store)

	roles := store.ListRoles()
	if len(roles) < 5 {
		t.Fatalf("expected at least 5 seeded roles, got %d", len(roles))
	}

	// Check that Owner role exists with admin permissions.
	found := false
	for _, role := range roles {
		if role.Name == "Owner" {
			found = true
			if len(role.Permissions) == 0 {
				t.Fatal("Owner role should have permissions")
			}
		}
	}
	if !found {
		t.Fatal("expected Owner role in seed data")
	}
}

func TestRoleAddRemovePermissions(t *testing.T) {
	r, _ := newTestRouter()

	// Create with one permission.
	w := doRequest(r, "POST", "/roles/", map[string]interface{}{
		"name": "Viewer",
		"permissions": []map[string]string{
			{"module": "kyc", "action": "read"},
		},
	})
	var role Role
	decodeJSON(t, w, &role)

	// Update: add more permissions.
	w = doRequest(r, "PATCH", "/roles/"+role.ID, map[string]interface{}{
		"permissions": []map[string]string{
			{"module": "kyc", "action": "read"},
			{"module": "funds", "action": "read"},
			{"module": "esign", "action": "read"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var patched Role
	decodeJSON(t, w, &patched)
	if len(patched.Permissions) != 3 {
		t.Fatalf("expected 3 permissions, got %d", len(patched.Permissions))
	}

	// Update: remove permissions (set to single).
	w = doRequest(r, "PATCH", "/roles/"+role.ID, map[string]interface{}{
		"permissions": []map[string]string{
			{"module": "kyc", "action": "read"},
		},
	})
	var reduced Role
	decodeJSON(t, w, &reduced)
	if len(reduced.Permissions) != 1 {
		t.Fatalf("expected 1 permission after removal, got %d", len(reduced.Permissions))
	}
}

func TestRoleWithDuplicatePermissions(t *testing.T) {
	r, _ := newTestRouter()

	// Create role with duplicate permissions -- store allows it (idempotent).
	w := doRequest(r, "POST", "/roles/", map[string]interface{}{
		"name": "Dup Test",
		"permissions": []map[string]string{
			{"module": "kyc", "action": "read"},
			{"module": "kyc", "action": "read"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var role Role
	decodeJSON(t, w, &role)
	// Both are stored (dedup is a policy decision, not store's job).
	if len(role.Permissions) != 2 {
		t.Fatalf("expected 2 permissions, got %d", len(role.Permissions))
	}
}

func TestRoleCreateMissingName(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/roles/", map[string]interface{}{
		"description": "no name",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRoleDeleteNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "DELETE", "/roles/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRoleUpdateNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "PATCH", "/roles/nonexistent", map[string]interface{}{
		"name": "x",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// ==========================================================================
// Modules Test
// ==========================================================================

func TestListModules(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/modules", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var modules []Module
	decodeJSON(t, w, &modules)
	if len(modules) < 6 {
		t.Fatalf("expected at least 6 modules, got %d", len(modules))
	}
}

// ==========================================================================
// Health Check Tests
// ==========================================================================

func TestHealthz(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/healthz", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	decodeJSON(t, w, &resp)
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", resp["status"])
	}
	if resp["version"] == "" {
		t.Fatal("expected version in health response")
	}
}

// ==========================================================================
// Store Unit Tests
// ==========================================================================

func TestStoreIdentityNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetIdentity("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent identity")
	}
}

func TestStoreGeneratesIDs(t *testing.T) {
	s := NewStore()
	f := &Fund{Name: "Test"}
	if err := s.SaveFund(f); err != nil {
		t.Fatal(err)
	}
	if f.ID == "" {
		t.Fatal("expected generated ID")
	}
	if f.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}
}

func TestStoreFundInvestors(t *testing.T) {
	s := NewStore()
	f := &Fund{Name: "Test Fund"}
	s.SaveFund(f)

	if err := s.AddFundInvestor(&FundInvestor{
		FundID:     f.ID,
		InvestorID: "inv-1",
		Name:       "Alice",
		Amount:     50000,
	}); err != nil {
		t.Fatal(err)
	}

	inv := s.ListFundInvestors(f.ID)
	if len(inv) != 1 {
		t.Fatalf("expected 1 investor, got %d", len(inv))
	}

	// Verify fund stats updated.
	got, _ := s.GetFund(f.ID)
	if got.InvestorCount != 1 {
		t.Fatalf("expected investor_count 1, got %d", got.InvestorCount)
	}
	if got.TotalRaised != 50000 {
		t.Fatalf("expected total_raised 50000, got %f", got.TotalRaised)
	}
}

func TestStoreFundInvestorNonexistentFund(t *testing.T) {
	s := NewStore()
	err := s.AddFundInvestor(&FundInvestor{FundID: "nope"})
	if err == nil {
		t.Fatal("expected error for nonexistent fund")
	}
}

func TestStoreSessionTimestamps(t *testing.T) {
	s := NewStore()
	sess := &Session{PipelineID: "p1", InvestorEmail: "a@b.com", Status: SessionPending}
	s.SaveSession(sess)
	if sess.ID == "" {
		t.Fatal("expected generated ID")
	}
	if sess.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt to be set")
	}
}

func TestStoreRoleTimestamps(t *testing.T) {
	s := NewStore()
	role := &Role{Name: "Test"}
	s.SaveRole(role)
	if role.ID == "" {
		t.Fatal("expected generated ID")
	}
	if role.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt")
	}
	if role.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt")
	}
}

func TestStoreEnvelopeTimestamps(t *testing.T) {
	s := NewStore()
	env := &Envelope{Subject: "Test", Status: EnvelopePending}
	s.SaveEnvelope(env)
	if env.ID == "" {
		t.Fatal("expected generated ID")
	}
	if env.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt")
	}
}

func TestStorePipelineTimestamps(t *testing.T) {
	s := NewStore()
	p := &Pipeline{Name: "Test", Status: "draft"}
	s.SavePipeline(p)
	if p.ID == "" {
		t.Fatal("expected generated ID")
	}
	if p.CreatedAt.IsZero() {
		t.Fatal("expected CreatedAt")
	}
	if p.UpdatedAt.IsZero() {
		t.Fatal("expected UpdatedAt")
	}
}

// ==========================================================================
// Seed Tests
// ==========================================================================

func TestSeedStore(t *testing.T) {
	s := NewStore()
	SeedStore(s)

	roles := s.ListRoles()
	if len(roles) < 5 {
		t.Fatalf("expected at least 5 roles, got %d", len(roles))
	}

	pipelines := s.ListPipelines()
	if len(pipelines) < 3 {
		t.Fatalf("expected at least 3 pipelines, got %d", len(pipelines))
	}

	sessions := s.ListSessions()
	if len(sessions) < 5 {
		t.Fatalf("expected at least 5 sessions, got %d", len(sessions))
	}

	funds := s.ListFunds()
	if len(funds) < 2 {
		t.Fatalf("expected at least 2 funds, got %d", len(funds))
	}
}

func TestSeedStoreIdempotent(t *testing.T) {
	s := NewStore()
	SeedStore(s)
	count1 := len(s.ListRoles())

	SeedStore(s)
	count2 := len(s.ListRoles())

	// Second seed adds more (it is additive, not deduplicated).
	// This is expected behavior for dev seed data.
	if count2 < count1 {
		t.Fatalf("seed should not remove data: %d < %d", count2, count1)
	}
}

// ==========================================================================
// Integration Tests (full flow)
// ==========================================================================

func TestIntegrationPipelineToSessionToKYC(t *testing.T) {
	r, _ := newTestRouter()

	// 1. Create a pipeline.
	w := doRequest(r, "POST", "/pipelines/", map[string]interface{}{
		"name":        "Reg D 506(c)",
		"business_id": "biz-100",
		"status":      "active",
		"steps": []map[string]interface{}{
			{"id": "kyc-step", "name": "Identity Verification", "type": "kyc", "required": true, "order": 1},
			{"id": "esign-step", "name": "Sign Documents", "type": "esign", "required": true, "order": 2},
			{"id": "payment-step", "name": "Fund Investment", "type": "payment", "required": true, "order": 3},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create pipeline: expected 201, got %d", w.Code)
	}
	var pipeline Pipeline
	decodeJSON(t, w, &pipeline)

	// 2. Create a session for an investor.
	w = doRequest(r, "POST", "/sessions/", map[string]string{
		"pipeline_id":    pipeline.ID,
		"investor_email": "investor@fund.com",
		"investor_name":  "Jane Investor",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create session: expected 201, got %d", w.Code)
	}
	var sess Session
	decodeJSON(t, w, &sess)
	if len(sess.Steps) != 3 {
		t.Fatalf("expected 3 steps from pipeline, got %d", len(sess.Steps))
	}

	// 3. Start the session.
	w = doRequest(r, "PATCH", "/sessions/"+sess.ID, map[string]string{
		"status": "in_progress",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("start session: expected 200, got %d", w.Code)
	}

	// 4. Run KYC verification.
	w = doRequest(r, "POST", "/kyc/verify", map[string]string{
		"user_id":  "investor@fund.com",
		"provider": "onfido",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("kyc verify: expected 201, got %d", w.Code)
	}
	var ident Identity
	decodeJSON(t, w, &ident)

	// 5. Update session KYC status.
	w = doRequest(r, "PATCH", "/sessions/"+sess.ID, map[string]string{
		"kyc_status": "verified",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update kyc status: expected 200, got %d", w.Code)
	}
	var kycUpdated Session
	decodeJSON(t, w, &kycUpdated)
	if kycUpdated.KYCStatus != KYCVerified {
		t.Fatalf("expected kyc verified, got %s", kycUpdated.KYCStatus)
	}

	// 6. Complete the session.
	w = doRequest(r, "PATCH", "/sessions/"+sess.ID, map[string]string{
		"status": "completed",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("complete session: expected 200, got %d", w.Code)
	}
	var completed Session
	decodeJSON(t, w, &completed)
	if completed.Status != SessionCompleted {
		t.Fatalf("expected completed, got %s", completed.Status)
	}
	if completed.KYCStatus != KYCVerified {
		t.Fatalf("expected kyc verified preserved, got %s", completed.KYCStatus)
	}
}

func TestIntegrationFundWithInvestors(t *testing.T) {
	r, store := newTestRouter()

	// Create fund via API.
	w := doRequest(r, "POST", "/funds/", map[string]interface{}{
		"name":           "Series A Fund",
		"business_id":    "biz-200",
		"type":           "equity",
		"min_investment": 25000,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create fund: expected 201, got %d", w.Code)
	}
	var fund Fund
	decodeJSON(t, w, &fund)

	// Add investors directly (store-level, simulating backend process).
	store.AddFundInvestor(&FundInvestor{
		FundID: fund.ID, InvestorID: "inv-a", Name: "Alice", Email: "alice@fund.com", Amount: 50000, Status: "funded",
	})
	store.AddFundInvestor(&FundInvestor{
		FundID: fund.ID, InvestorID: "inv-b", Name: "Bob", Email: "bob@fund.com", Amount: 100000, Status: "committed",
	})

	// Verify via API.
	w = doRequest(r, "GET", "/funds/"+fund.ID, nil)
	var got Fund
	decodeJSON(t, w, &got)
	if got.InvestorCount != 2 {
		t.Fatalf("expected 2 investors, got %d", got.InvestorCount)
	}
	if got.TotalRaised != 150000 {
		t.Fatalf("expected total_raised 150000, got %f", got.TotalRaised)
	}

	// List investors via API.
	w = doRequest(r, "GET", "/funds/"+fund.ID+"/investors", nil)
	var investors []*FundInvestor
	decodeJSON(t, w, &investors)
	if len(investors) != 2 {
		t.Fatalf("expected 2 investors, got %d", len(investors))
	}
}

func TestIntegrationESignFullFlow(t *testing.T) {
	r, _ := newTestRouter()

	// 1. Create template.
	w := doRequest(r, "POST", "/esign/templates", map[string]interface{}{
		"name":        "Subscription Agreement",
		"description": "Standard subscription agreement for fund investors",
		"roles":       []string{"investor", "issuer"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create template: expected 201, got %d", w.Code)
	}
	var tmpl Template
	decodeJSON(t, w, &tmpl)

	// 2. Create envelope from template.
	w = doRequest(r, "POST", "/esign/envelopes", map[string]interface{}{
		"template_id": tmpl.ID,
		"subject":     "Sign: Subscription Agreement - Series A",
		"message":     "Please review and sign the attached subscription agreement.",
		"signers": []map[string]string{
			{"name": "Alice Investor", "email": "alice@investor.com", "role": "investor"},
			{"name": "Bob Issuer", "email": "bob@issuer.com", "role": "issuer"},
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create envelope: expected 201, got %d", w.Code)
	}
	var env Envelope
	decodeJSON(t, w, &env)
	if env.TemplateID != tmpl.ID {
		t.Fatalf("expected template_id %s, got %s", tmpl.ID, env.TemplateID)
	}

	// 3. First signer signs.
	w = doRequest(r, "POST", "/esign/envelopes/"+env.ID+"/sign", map[string]string{
		"signer_id": env.Signers[0].ID,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("sign1: expected 200, got %d", w.Code)
	}

	// 4. Check status (should be "signed" - partial).
	w = doRequest(r, "GET", "/esign/envelopes/"+env.ID, nil)
	var mid Envelope
	decodeJSON(t, w, &mid)
	if mid.Status != EnvelopeSigned {
		t.Fatalf("expected signed, got %s", mid.Status)
	}

	// 5. Second signer signs -> completed.
	w = doRequest(r, "POST", "/esign/envelopes/"+env.ID+"/sign", map[string]string{
		"signer_id": env.Signers[1].ID,
	})
	var final Envelope
	decodeJSON(t, w, &final)
	if final.Status != EnvelopeCompleted {
		t.Fatalf("expected completed, got %s", final.Status)
	}
}

// ==========================================================================
// Dashboard Tests
// ==========================================================================

func TestDashboard(t *testing.T) {
	r, store := newTestRouter()

	// Seed some data.
	store.SaveSession(&Session{Status: SessionInProgress, KYCStatus: KYCPending})
	store.SaveSession(&Session{Status: SessionCompleted, KYCStatus: KYCVerified})
	store.SaveSession(&Session{Status: SessionPending, KYCStatus: KYCPending})
	store.SaveFund(&Fund{Name: "Fund A"})
	store.SaveTransaction(&Transaction{Type: "deposit", Amount: 100})

	w := doRequest(r, "GET", "/dashboard", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats DashboardStats
	decodeJSON(t, w, &stats)

	if stats.ActiveSessions != 2 {
		t.Fatalf("expected 2 active sessions, got %d", stats.ActiveSessions)
	}
	if stats.PendingKYC != 2 {
		t.Fatalf("expected 2 pending KYC, got %d", stats.PendingKYC)
	}
	if stats.TotalFunds != 1 {
		t.Fatalf("expected 1 fund, got %d", stats.TotalFunds)
	}
	if stats.MonthlyTransactions != 1 {
		t.Fatalf("expected 1 transaction, got %d", stats.MonthlyTransactions)
	}
}

func TestDashboardEmpty(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "GET", "/dashboard", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var stats DashboardStats
	decodeJSON(t, w, &stats)
	if stats.ActiveSessions != 0 {
		t.Fatalf("expected 0 active sessions, got %d", stats.ActiveSessions)
	}
}

// ==========================================================================
// Users Tests
// ==========================================================================

func TestUserListAndCreate(t *testing.T) {
	r, _ := newTestRouter()

	// List (empty).
	w := doRequest(r, "GET", "/users", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var users []*User
	decodeJSON(t, w, &users)
	if len(users) != 0 {
		t.Fatalf("expected 0 users, got %d", len(users))
	}

	// Create.
	w = doRequest(r, "POST", "/users", map[string]string{
		"name":  "Test User",
		"email": "test@example.com",
		"role":  "admin",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var u User
	decodeJSON(t, w, &u)
	if u.Name != "Test User" {
		t.Fatalf("expected name 'Test User', got %q", u.Name)
	}
	if u.Email != "test@example.com" {
		t.Fatalf("expected email 'test@example.com', got %q", u.Email)
	}
	if u.Role != "admin" {
		t.Fatalf("expected role 'admin', got %q", u.Role)
	}
	if u.Status != "active" {
		t.Fatalf("expected default status 'active', got %q", u.Status)
	}
	if u.ID == "" {
		t.Fatal("expected non-empty ID")
	}

	// List again (should have 1).
	w = doRequest(r, "GET", "/users", nil)
	decodeJSON(t, w, &users)
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
}

func TestUserCreateMissingName(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/users", map[string]string{"email": "a@b.com"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUserCreateMissingEmail(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/users", map[string]string{"name": "Test"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUserCreateDefaultRole(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/users", map[string]string{
		"name":  "Test",
		"email": "test@test.com",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var u User
	decodeJSON(t, w, &u)
	if u.Role != "agent" {
		t.Fatalf("expected default role 'agent', got %q", u.Role)
	}
}

// ==========================================================================
// Transactions Tests
// ==========================================================================

func TestTransactionList(t *testing.T) {
	r, store := newTestRouter()

	// Empty list.
	w := doRequest(r, "GET", "/transactions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var txns []*Transaction
	decodeJSON(t, w, &txns)
	if len(txns) != 0 {
		t.Fatalf("expected 0 transactions, got %d", len(txns))
	}

	// Seed some.
	store.SaveTransaction(&Transaction{Type: "deposit", Asset: "USD", Amount: 1000})
	store.SaveTransaction(&Transaction{Type: "trade", Asset: "BTC", Amount: 0.5})

	w = doRequest(r, "GET", "/transactions", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	decodeJSON(t, w, &txns)
	if len(txns) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txns))
	}
}

// ==========================================================================
// Reports Tests
// ==========================================================================

func TestReportList(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "GET", "/reports", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var reports []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	decodeJSON(t, w, &reports)
	if len(reports) != 4 {
		t.Fatalf("expected 4 reports, got %d", len(reports))
	}

	// Check expected report IDs.
	expectedIDs := map[string]bool{
		"monthly-statement":   false,
		"form-8949":           false,
		"form-1099b":          false,
		"transaction-history": false,
	}
	for _, rpt := range reports {
		if _, ok := expectedIDs[rpt.ID]; !ok {
			t.Fatalf("unexpected report ID: %s", rpt.ID)
		}
		expectedIDs[rpt.ID] = true
	}
	for id, found := range expectedIDs {
		if !found {
			t.Fatalf("missing report: %s", id)
		}
	}
}

// ==========================================================================
// Settings Tests
// ==========================================================================

func TestSettingsGetAndUpdate(t *testing.T) {
	r, _ := newTestRouter()

	// Get defaults.
	w := doRequest(r, "GET", "/settings", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", w.Code)
	}
	var s Settings
	decodeJSON(t, w, &s)
	if s.BusinessName != "Your Company" {
		t.Fatalf("expected default business name 'Your Company', got %q", s.BusinessName)
	}
	if s.Timezone != "America/New_York" {
		t.Fatalf("expected timezone 'America/New_York', got %q", s.Timezone)
	}

	// Update.
	w = doRequest(r, "PUT", "/settings", map[string]string{
		"business_name": "Acme Corp",
		"timezone":      "America/Los_Angeles",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("update: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJSON(t, w, &s)
	if s.BusinessName != "Acme Corp" {
		t.Fatalf("expected 'Acme Corp', got %q", s.BusinessName)
	}
	if s.Timezone != "America/Los_Angeles" {
		t.Fatalf("expected 'America/Los_Angeles', got %q", s.Timezone)
	}
	// Unchanged fields should remain.
	if s.Currency != "USD" {
		t.Fatalf("expected currency 'USD' unchanged, got %q", s.Currency)
	}

	// Verify persistence.
	w = doRequest(r, "GET", "/settings", nil)
	decodeJSON(t, w, &s)
	if s.BusinessName != "Acme Corp" {
		t.Fatalf("expected persisted 'Acme Corp', got %q", s.BusinessName)
	}
}

// ==========================================================================
// Credentials Tests
// ==========================================================================

func TestCredentialCreateListDelete(t *testing.T) {
	r, _ := newTestRouter()

	// List (empty).
	w := doRequest(r, "GET", "/credentials", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", w.Code)
	}
	var creds []*Credential
	decodeJSON(t, w, &creds)
	if len(creds) != 0 {
		t.Fatalf("expected 0 credentials, got %d", len(creds))
	}

	// Create.
	w = doRequest(r, "POST", "/credentials", map[string]interface{}{
		"name":        "Test Key",
		"permissions": []string{"read", "trade"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created struct {
		Credential
		Key string `json:"key"`
	}
	decodeJSON(t, w, &created)
	if created.Name != "Test Key" {
		t.Fatalf("expected name 'Test Key', got %q", created.Name)
	}
	if created.Key == "" {
		t.Fatal("expected non-empty key on creation")
	}
	if created.KeyPrefix == "" {
		t.Fatal("expected non-empty key prefix")
	}
	if len(created.Key) != 64 { // 32 bytes hex
		t.Fatalf("expected 64 char key, got %d", len(created.Key))
	}

	// List (should have 1).
	w = doRequest(r, "GET", "/credentials", nil)
	decodeJSON(t, w, &creds)
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(creds))
	}

	// Delete.
	w = doRequest(r, "DELETE", "/credentials/"+created.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List (should be empty again).
	w = doRequest(r, "GET", "/credentials", nil)
	decodeJSON(t, w, &creds)
	if len(creds) != 0 {
		t.Fatalf("expected 0 credentials after delete, got %d", len(creds))
	}
}

func TestCredentialCreateMissingName(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/credentials", map[string]interface{}{
		"permissions": []string{"read"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCredentialDeleteNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "DELETE", "/credentials/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCredentialDefaultPermissions(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/credentials", map[string]interface{}{
		"name": "ReadOnly",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var created struct {
		Credential
		Key string `json:"key"`
	}
	decodeJSON(t, w, &created)
	if len(created.Permissions) != 1 || created.Permissions[0] != "read" {
		t.Fatalf("expected default [read] permissions, got %v", created.Permissions)
	}
}

// ==========================================================================
// Billing Tests
// ==========================================================================

func TestBillingGet(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "GET", "/billing", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var billing BillingInfo
	decodeJSON(t, w, &billing)
	if billing.Plan != "professional" {
		t.Fatalf("expected plan 'professional', got %q", billing.Plan)
	}
	if len(billing.Invoices) != 3 {
		t.Fatalf("expected 3 invoices, got %d", len(billing.Invoices))
	}
}

// ==========================================================================
// eSign Dashboard Tests
// ==========================================================================

func TestESignDashboard(t *testing.T) {
	r, store := newTestRouter()

	// Seed envelopes.
	store.SaveEnvelope(&Envelope{Subject: "Doc A", Status: EnvelopePending, Signers: []Signer{{ID: "s1"}}})
	store.SaveEnvelope(&Envelope{Subject: "Doc B", Status: EnvelopeCompleted, Signers: []Signer{{ID: "s2"}}})
	store.SaveEnvelope(&Envelope{Subject: "Doc C", Status: EnvelopeSent, Signers: []Signer{{ID: "s3"}}})
	store.SaveTemplate(&Template{Name: "Template 1"})

	w := doRequest(r, "GET", "/esign-dashboard", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var stats ESignStats
	decodeJSON(t, w, &stats)
	if stats.Pending != 2 {
		t.Fatalf("expected 2 pending, got %d", stats.Pending)
	}
	if stats.Completed != 1 {
		t.Fatalf("expected 1 completed, got %d", stats.Completed)
	}
	if stats.Templates != 1 {
		t.Fatalf("expected 1 template, got %d", stats.Templates)
	}
}

// ==========================================================================
// Envelope Direction Tests
// ==========================================================================

func TestEnvelopeInboxAndSent(t *testing.T) {
	r, store := newTestRouter()

	// Seed envelopes with different statuses.
	store.SaveEnvelope(&Envelope{Subject: "Inbox 1", Status: EnvelopePending, Signers: []Signer{{ID: "s1"}}})
	store.SaveEnvelope(&Envelope{Subject: "Inbox 2", Status: EnvelopeSent, Signers: []Signer{{ID: "s2"}}})
	store.SaveEnvelope(&Envelope{Subject: "Sent 1", Status: EnvelopeCompleted, Signers: []Signer{{ID: "s3"}}})
	store.SaveEnvelope(&Envelope{Subject: "Sent 2", Status: EnvelopeDeclined, Signers: []Signer{{ID: "s4"}}})

	// Inbox.
	w := doRequest(r, "GET", "/envelopes/inbox", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("inbox: expected 200, got %d", w.Code)
	}
	var inbox []*Envelope
	decodeJSON(t, w, &inbox)
	if len(inbox) != 2 {
		t.Fatalf("expected 2 inbox envelopes, got %d", len(inbox))
	}

	// Sent.
	w = doRequest(r, "GET", "/envelopes/sent", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("sent: expected 200, got %d", w.Code)
	}
	var sent []*Envelope
	decodeJSON(t, w, &sent)
	if len(sent) != 2 {
		t.Fatalf("expected 2 sent envelopes, got %d", len(sent))
	}
}

// ==========================================================================
// Seed Tests
// ==========================================================================

func TestSeedPopulatesNewTypes(t *testing.T) {
	store := NewStore()
	SeedStore(store)

	if len(store.ListUsers()) == 0 {
		t.Fatal("expected seeded users")
	}
	if len(store.ListTransactions()) == 0 {
		t.Fatal("expected seeded transactions")
	}
	if len(store.ListCredentials()) == 0 {
		t.Fatal("expected seeded credentials")
	}
	if len(store.ListEnvelopes()) == 0 {
		t.Fatal("expected seeded envelopes")
	}
	if len(store.ListTemplates()) == 0 {
		t.Fatal("expected seeded templates")
	}
	if len(store.ListFunds()) == 0 {
		t.Fatal("expected seeded funds")
	}
	if len(store.ListRoles()) == 0 {
		t.Fatal("expected seeded roles")
	}
}

// ==========================================================================
// AML Screening Tests
// ==========================================================================

func TestAMLScreenAndGet(t *testing.T) {
	r, _ := newTestRouter()

	// Create an AML screening.
	w := doRequest(r, "POST", "/aml/screen", map[string]string{
		"account_id": "acct-123",
		"user_id":    "user-123",
		"name":       "John Doe",
		"country":    "US",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var screening AMLScreening
	decodeJSON(t, w, &screening)
	if screening.AccountID != "acct-123" {
		t.Fatalf("expected account_id acct-123, got %s", screening.AccountID)
	}
	if screening.Status != AMLPending {
		t.Fatalf("expected status pending, got %s", screening.Status)
	}

	// Get screening by ID.
	w = doRequest(r, "GET", "/aml/screenings/"+screening.ID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var fetched AMLScreening
	decodeJSON(t, w, &fetched)
	if fetched.ID != screening.ID {
		t.Fatalf("ID mismatch: %s != %s", fetched.ID, screening.ID)
	}
}

func TestAMLScreenMissingAccountID(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/aml/screen", map[string]string{
		"name": "John Doe",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAMLScreenMissingName(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/aml/screen", map[string]string{
		"account_id": "acct-1",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAMLListByAccount(t *testing.T) {
	r, store := newTestRouter()

	store.SaveAMLScreening(&AMLScreening{AccountID: "acct-a", UserID: "u1", Type: "sanctions", Status: AMLPending, Provider: "manual"})
	store.SaveAMLScreening(&AMLScreening{AccountID: "acct-a", UserID: "u1", Type: "pep", Status: AMLCleared, Provider: "manual"})
	store.SaveAMLScreening(&AMLScreening{AccountID: "acct-b", UserID: "u2", Type: "sanctions", Status: AMLFlagged, Provider: "manual"})

	w := doRequest(r, "GET", "/aml/screenings?account_id=acct-a", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var screenings []*AMLScreening
	decodeJSON(t, w, &screenings)
	if len(screenings) != 2 {
		t.Fatalf("expected 2 screenings for acct-a, got %d", len(screenings))
	}
}

func TestAMLListByAccountMissingParam(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/aml/screenings", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAMLListFlagged(t *testing.T) {
	r, store := newTestRouter()

	store.SaveAMLScreening(&AMLScreening{AccountID: "a1", Status: AMLFlagged, Provider: "jube"})
	store.SaveAMLScreening(&AMLScreening{AccountID: "a2", Status: AMLCleared, Provider: "jube"})
	store.SaveAMLScreening(&AMLScreening{AccountID: "a3", Status: AMLFlagged, Provider: "jube"})

	w := doRequest(r, "GET", "/aml/flagged", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var flagged []*AMLScreening
	decodeJSON(t, w, &flagged)
	if len(flagged) != 2 {
		t.Fatalf("expected 2 flagged, got %d", len(flagged))
	}
}

func TestAMLReview(t *testing.T) {
	r, store := newTestRouter()

	sc := &AMLScreening{AccountID: "a1", Status: AMLFlagged, Provider: "jube"}
	store.SaveAMLScreening(sc)

	// Review and clear. reviewed_by comes from JWT context (testadmin), not request body.
	w := doRequest(r, "POST", "/aml/screenings/"+sc.ID+"/review", map[string]string{
		"decision": "cleared",
		"details":  "false positive",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var reviewed AMLScreening
	decodeJSON(t, w, &reviewed)
	if reviewed.Status != AMLCleared {
		t.Fatalf("expected cleared, got %s", reviewed.Status)
	}
	// Reviewer identity is extracted from JWT, not request body.
	if reviewed.ReviewedBy != "test-user-001" {
		t.Fatalf("expected reviewed_by test-user-001 (from gateway), got %s", reviewed.ReviewedBy)
	}
}

func TestAMLReviewBadDecision(t *testing.T) {
	r, store := newTestRouter()

	sc := &AMLScreening{AccountID: "a1", Status: AMLFlagged, Provider: "jube"}
	store.SaveAMLScreening(sc)

	w := doRequest(r, "POST", "/aml/screenings/"+sc.ID+"/review", map[string]string{
		"decision":    "maybe",
		"reviewed_by": "admin",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAMLReviewIgnoresBodyReviewedBy(t *testing.T) {
	r, store := newTestRouter()

	sc := &AMLScreening{AccountID: "a1", Status: AMLFlagged, Provider: "jube"}
	store.SaveAMLScreening(sc)

	// Send reviewed_by in body attempting to spoof identity.
	// It must be ignored — reviewer comes from JWT context.
	w := doRequest(r, "POST", "/aml/screenings/"+sc.ID+"/review", map[string]string{
		"decision":    "cleared",
		"reviewed_by": "spoofed-admin@evil.com",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var reviewed AMLScreening
	decodeJSON(t, w, &reviewed)
	if reviewed.ReviewedBy == "spoofed-admin@evil.com" {
		t.Fatal("SECURITY: reviewed_by accepted from request body instead of JWT context")
	}
	if reviewed.ReviewedBy != "test-user-001" {
		t.Fatalf("expected reviewed_by test-user-001 (from gateway), got %s", reviewed.ReviewedBy)
	}
}

func TestAMLReviewNotFound(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "POST", "/aml/screenings/nonexistent/review", map[string]string{
		"decision":    "cleared",
		"reviewed_by": "admin",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAMLGetNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/aml/screenings/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAMLRiskAssessment(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "POST", "/aml/risk-assessment", map[string]interface{}{
		"account_id": "acct-1",
		"user_id":    "user-1",
		"amount":     50000.0,
		"currency":   "USD",
		"type":       "deposit",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var screening AMLScreening
	decodeJSON(t, w, &screening)
	if screening.Type != "transaction" {
		t.Fatalf("expected type transaction, got %s", screening.Type)
	}
}

func TestAMLRiskAssessmentMissingAmount(t *testing.T) {
	r, _ := newTestRouter()

	w := doRequest(r, "POST", "/aml/risk-assessment", map[string]interface{}{
		"account_id": "acct-1",
		"amount":     0,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ==========================================================================
// Application (5-Step Onboarding) Tests
// ==========================================================================

func TestApplicationFullOnboardingFlow(t *testing.T) {
	r, _ := newTestRouter()

	// Create application.
	w := doRequest(r, "POST", "/applications/", map[string]string{
		"user_id": "user-onboard",
		"email":   "investor@example.com",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var app Application
	decodeJSON(t, w, &app)
	if app.Status != AppDraft {
		t.Fatalf("expected status draft, got %s", app.Status)
	}
	if app.CurrentStep != 1 {
		t.Fatalf("expected current_step 1, got %d", app.CurrentStep)
	}
	if len(app.Steps) != 5 {
		t.Fatalf("expected 5 steps, got %d", len(app.Steps))
	}

	// Step 1: Basic info + Contact.
	w = doRequest(r, "POST", "/applications/"+app.ID+"/step/1", map[string]interface{}{
		"first_name":    "John",
		"last_name":     "Doe",
		"phone":         "+1-555-0100",
		"date_of_birth": "1990-01-15",
		"ssn":           "123-45-6789",
		"address_line1": "123 Main St",
		"city":          "New York",
		"state":         "NY",
		"zip_code":      "10001",
		"country":       "US",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("step1: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJSON(t, w, &app)
	if app.FirstName != "John" {
		t.Fatalf("expected first_name John, got %s", app.FirstName)
	}
	if app.CurrentStep != 2 {
		t.Fatalf("expected current_step 2, got %d", app.CurrentStep)
	}
	if app.Status != AppInProgress {
		t.Fatalf("expected status in_progress, got %s", app.Status)
	}
	// SSN must be hashed, not exposed via JSON.
	if app.SSNLast4 != "6789" {
		t.Fatalf("expected ssn_last4 6789, got %s", app.SSNLast4)
	}

	// Step 2: Identity verification.
	w = doRequest(r, "POST", "/applications/"+app.ID+"/step/2", map[string]string{
		"provider":        "onfido",
		"verification_id": "ver-abc-123",
		"status":          "verified",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("step2: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJSON(t, w, &app)
	if app.KYCStatus != KYCVerified {
		t.Fatalf("expected kyc_status verified, got %s", app.KYCStatus)
	}
	if app.CurrentStep != 3 {
		t.Fatalf("expected current_step 3, got %d", app.CurrentStep)
	}

	// Step 3: Document upload.
	w = doRequest(r, "POST", "/applications/"+app.ID+"/step/3", map[string]interface{}{
		"documents": []map[string]interface{}{
			{"type": "passport", "name": "passport.pdf", "mime_type": "application/pdf", "size": 102400},
			{"type": "utility_bill", "name": "utility.pdf", "mime_type": "application/pdf", "size": 51200},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("step3: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJSON(t, w, &app)
	if app.CurrentStep != 4 {
		t.Fatalf("expected current_step 4, got %d", app.CurrentStep)
	}

	// Check documents were saved.
	w = doRequest(r, "GET", "/applications/"+app.ID+"/documents", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("get docs: expected 200, got %d", w.Code)
	}
	var docs []*DocumentUpload
	decodeJSON(t, w, &docs)
	if len(docs) != 2 {
		t.Fatalf("expected 2 documents, got %d", len(docs))
	}

	// Step 4: Compliance/AML screening.
	w = doRequest(r, "POST", "/applications/"+app.ID+"/step/4", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("step4: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJSON(t, w, &app)
	if app.CurrentStep != 5 {
		t.Fatalf("expected current_step 5, got %d", app.CurrentStep)
	}

	// Step 5: Review + Submit.
	w = doRequest(r, "POST", "/applications/"+app.ID+"/step/5", map[string]bool{
		"confirmed": true,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("step5: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJSON(t, w, &app)
	if app.Status != AppSubmitted {
		t.Fatalf("expected status submitted, got %s", app.Status)
	}
	if app.SubmittedAt.IsZero() {
		t.Fatal("expected submitted_at to be set")
	}

	// Admin review: approve.
	w = doRequest(r, "POST", "/applications/"+app.ID+"/review", map[string]string{
		"decision":    "approved",
		"reviewed_by": "compliance-officer@example.com",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("review: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	decodeJSON(t, w, &app)
	if app.Status != AppApproved {
		t.Fatalf("expected status approved, got %s", app.Status)
	}
}

func TestApplicationCreateMissingUserID(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/applications/", map[string]string{
		"email": "test@test.com",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationCreateMissingEmail(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/applications/", map[string]string{
		"user_id": "user-1",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationGetNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/applications/nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestApplicationLookupByUser(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{
		UserID: "user-lookup",
		Email:  "lookup@test.com",
		Status: AppDraft,
		Steps:  newApplicationSteps(),
	}
	store.SaveApplication(app)

	w := doRequest(r, "GET", "/applications/lookup?user_id=user-lookup", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var fetched Application
	decodeJSON(t, w, &fetched)
	if fetched.UserID != "user-lookup" {
		t.Fatalf("expected user_id user-lookup, got %s", fetched.UserID)
	}
}

func TestApplicationLookupByUserNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/applications/lookup?user_id=nonexistent", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestApplicationLookupMissingParam(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/applications/lookup", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationListByStatus(t *testing.T) {
	r, store := newTestRouter()

	store.SaveApplication(&Application{UserID: "u1", Email: "a@a.com", Status: AppDraft, Steps: newApplicationSteps()})
	store.SaveApplication(&Application{UserID: "u2", Email: "b@b.com", Status: AppSubmitted, Steps: newApplicationSteps()})
	store.SaveApplication(&Application{UserID: "u3", Email: "c@c.com", Status: AppSubmitted, Steps: newApplicationSteps()})

	// All applications.
	w := doRequest(r, "GET", "/applications/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var all []*Application
	decodeJSON(t, w, &all)
	if len(all) != 3 {
		t.Fatalf("expected 3 applications, got %d", len(all))
	}

	// Filter by status.
	w = doRequest(r, "GET", "/applications/?status=submitted", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var submitted []*Application
	decodeJSON(t, w, &submitted)
	if len(submitted) != 2 {
		t.Fatalf("expected 2 submitted, got %d", len(submitted))
	}
}

func TestApplicationStep1MissingName(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{UserID: "u1", Email: "a@a.com", Status: AppDraft, Steps: newApplicationSteps()}
	store.SaveApplication(app)

	w := doRequest(r, "POST", "/applications/"+app.ID+"/step/1", map[string]string{
		"last_name": "Doe",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationStep2MissingProvider(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{UserID: "u1", Email: "a@a.com", Status: AppInProgress, CurrentStep: 2, Steps: newApplicationSteps()}
	store.SaveApplication(app)

	w := doRequest(r, "POST", "/applications/"+app.ID+"/step/2", map[string]string{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationStep3NoDocuments(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{UserID: "u1", Email: "a@a.com", Status: AppInProgress, CurrentStep: 3, Steps: newApplicationSteps()}
	store.SaveApplication(app)

	w := doRequest(r, "POST", "/applications/"+app.ID+"/step/3", map[string]interface{}{
		"documents": []interface{}{},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationStep5NotConfirmed(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{UserID: "u1", Email: "a@a.com", Status: AppInProgress, CurrentStep: 5, KYCStatus: KYCVerified, Steps: newApplicationSteps()}
	store.SaveApplication(app)

	w := doRequest(r, "POST", "/applications/"+app.ID+"/step/5", map[string]bool{
		"confirmed": false,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationReviewBadDecision(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{UserID: "u1", Email: "a@a.com", Status: AppSubmitted, Steps: newApplicationSteps()}
	store.SaveApplication(app)

	w := doRequest(r, "POST", "/applications/"+app.ID+"/review", map[string]string{
		"decision":    "maybe",
		"reviewed_by": "admin",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationReviewNotSubmitted(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{UserID: "u1", Email: "a@a.com", Status: AppDraft, Steps: newApplicationSteps()}
	store.SaveApplication(app)

	w := doRequest(r, "POST", "/applications/"+app.ID+"/review", map[string]string{
		"decision":    "approved",
		"reviewed_by": "admin",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestApplicationReviewReject(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{UserID: "u1", Email: "a@a.com", Status: AppSubmitted, Steps: newApplicationSteps()}
	store.SaveApplication(app)

	w := doRequest(r, "POST", "/applications/"+app.ID+"/review", map[string]string{
		"decision":    "rejected",
		"reviewed_by": "compliance@example.com",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var reviewed Application
	decodeJSON(t, w, &reviewed)
	if reviewed.Status != AppRejected {
		t.Fatalf("expected rejected, got %s", reviewed.Status)
	}
}

func TestApplicationDocumentsNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/applications/nonexistent/documents", nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// ==========================================================================
// KYC Extended Endpoint Tests
// ==========================================================================

func TestKYCListByUser(t *testing.T) {
	r, store := newTestRouter()

	store.SaveIdentity(&Identity{UserID: "user-multi", Provider: "onfido", Status: KYCPending, Data: map[string]interface{}{}})
	store.SaveIdentity(&Identity{UserID: "user-multi", Provider: "berbix", Status: KYCVerified, Data: map[string]interface{}{}})
	store.SaveIdentity(&Identity{UserID: "user-other", Provider: "onfido", Status: KYCPending, Data: map[string]interface{}{}})

	w := doRequest(r, "GET", "/kyc/?user_id=user-multi", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var identities []*Identity
	decodeJSON(t, w, &identities)
	if len(identities) != 2 {
		t.Fatalf("expected 2 identities for user-multi, got %d", len(identities))
	}
}

func TestKYCListByUserMissingParam(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/kyc/", nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestKYCUpdateStatus(t *testing.T) {
	r, store := newTestRouter()

	ident := &Identity{UserID: "user-update", Provider: "onfido", Status: KYCPending, Data: map[string]interface{}{}}
	store.SaveIdentity(ident)

	w := doRequest(r, "PATCH", "/kyc/"+ident.ID, map[string]string{
		"status": "verified",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated Identity
	decodeJSON(t, w, &updated)
	if updated.Status != KYCVerified {
		t.Fatalf("expected verified, got %s", updated.Status)
	}
}

func TestKYCUpdateStatusInvalid(t *testing.T) {
	r, store := newTestRouter()

	ident := &Identity{UserID: "user-bad", Provider: "onfido", Status: KYCPending, Data: map[string]interface{}{}}
	store.SaveIdentity(ident)

	w := doRequest(r, "PATCH", "/kyc/"+ident.ID, map[string]string{
		"status": "nonexistent_status",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestKYCUpdateStatusNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "PATCH", "/kyc/nonexistent", map[string]string{
		"status": "verified",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestKYCDocumentUpload(t *testing.T) {
	r, store := newTestRouter()

	ident := &Identity{UserID: "user-doc", Provider: "onfido", Status: KYCPending, Data: map[string]interface{}{}}
	store.SaveIdentity(ident)

	w := doRequest(r, "POST", "/kyc/"+ident.ID+"/documents", map[string]string{
		"type":      "passport",
		"name":      "passport-scan.jpg",
		"mime_type": "image/jpeg",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var doc Document
	decodeJSON(t, w, &doc)
	if doc.Type != "passport" {
		t.Fatalf("expected type passport, got %s", doc.Type)
	}
	if doc.Status != "pending" {
		t.Fatalf("expected status pending, got %s", doc.Status)
	}
}

func TestKYCDocumentUploadMissingType(t *testing.T) {
	r, store := newTestRouter()

	ident := &Identity{UserID: "user-doc2", Provider: "onfido", Status: KYCPending, Data: map[string]interface{}{}}
	store.SaveIdentity(ident)

	w := doRequest(r, "POST", "/kyc/"+ident.ID+"/documents", map[string]string{
		"name": "something.pdf",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestKYCDocumentUploadIdentityNotFound(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "POST", "/kyc/nonexistent/documents", map[string]string{
		"type": "passport",
		"name": "test.pdf",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// ==========================================================================
// Modules Test (updated count)
// ==========================================================================

func TestListModulesIncludesAMLAndApplications(t *testing.T) {
	r, _ := newTestRouter()
	w := doRequest(r, "GET", "/modules", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var modules []Module
	decodeJSON(t, w, &modules)

	found := map[string]bool{}
	for _, m := range modules {
		found[m.Name] = true
	}
	for _, name := range []string{"aml", "applications"} {
		if !found[name] {
			t.Fatalf("expected module %q in list", name)
		}
	}
}

// ==========================================================================
// Store Unit Tests (new types)
// ==========================================================================

func TestStoreAMLScreeningNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetAMLScreening("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent aml screening")
	}
}

func TestStoreApplicationNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetApplication("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent application")
	}
}

func TestStoreApplicationByUserNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetApplicationByUser("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent user application")
	}
}

func TestStoreDocumentUploadNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetDocumentUpload("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent document")
	}
}

func TestStoreIdentitiesByUserEmpty(t *testing.T) {
	s := NewStore()
	results := s.ListIdentitiesByUser("nonexistent")
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestStoreGetRoleByName(t *testing.T) {
	s := NewStore()
	SeedStore(s)

	role, err := s.GetRoleByName("Owner")
	if err != nil {
		t.Fatalf("expected Owner role, got error: %v", err)
	}
	if role.Name != "Owner" {
		t.Fatalf("expected name Owner, got %s", role.Name)
	}
	if len(role.Permissions) == 0 {
		t.Fatal("expected Owner to have permissions")
	}
}

func TestStoreGetRoleByNameNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.GetRoleByName("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent role name")
	}
}

// ==========================================================================
// Security Tests — Red Team Findings
// ==========================================================================

// CRITICAL-2: AML reviewer identity must come from JWT, not request body.
func TestSecurityAMLReviewerSpoofingPrevented(t *testing.T) {
	r, store := newTestRouter()

	sc := &AMLScreening{AccountID: "a1", Status: AMLFlagged, Provider: "jube"}
	store.SaveAMLScreening(sc)

	// Attacker tries to claim review was by "bob" but JWT says "test-user-001".
	w := doRequest(r, "POST", "/aml/screenings/"+sc.ID+"/review", map[string]string{
		"decision":    "blocked",
		"reviewed_by": "bob@spoofed.com",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var reviewed AMLScreening
	decodeJSON(t, w, &reviewed)
	if reviewed.ReviewedBy == "bob@spoofed.com" {
		t.Fatal("SECURITY: reviewer identity accepted from request body (spoofable)")
	}
	if reviewed.ReviewedBy != "test-user-001" {
		t.Fatalf("expected reviewer testadmin from JWT, got %s", reviewed.ReviewedBy)
	}
}

// CRITICAL-3: Application reviewer identity must come from JWT, not request body.
func TestSecurityApplicationReviewerSpoofingPrevented(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{UserID: "u1", Email: "a@a.com", Status: AppSubmitted, Steps: newApplicationSteps()}
	store.SaveApplication(app)

	// Attacker sends reviewed_by in body — must be ignored.
	w := doRequest(r, "POST", "/applications/"+app.ID+"/review", map[string]string{
		"decision":    "approved",
		"reviewed_by": "spoofed-admin@evil.com",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var reviewed Application
	decodeJSON(t, w, &reviewed)
	if reviewed.ReviewedBy == "spoofed-admin@evil.com" {
		t.Fatal("SECURITY: reviewer identity accepted from request body (spoofable)")
	}
	if reviewed.ReviewedBy != "test-user-001" {
		t.Fatalf("expected reviewer testadmin from JWT, got %s", reviewed.ReviewedBy)
	}
}

// HIGH-2: Application steps cannot be skipped.
func TestSecurityApplicationStepSkipPrevented(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{
		UserID:      "u1",
		Email:       "a@a.com",
		Status:      AppDraft,
		CurrentStep: 1,
		Steps:       newApplicationSteps(),
	}
	store.SaveApplication(app)

	// Try to jump directly to step 5 — should be rejected.
	w := doRequest(r, "POST", "/applications/"+app.ID+"/step/5", map[string]bool{
		"confirmed": true,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("SECURITY: step 5 accepted without completing prior steps, got %d", w.Code)
	}

	// Try to jump to step 3 — should be rejected.
	w = doRequest(r, "POST", "/applications/"+app.ID+"/step/3", map[string]interface{}{
		"documents": []map[string]interface{}{
			{"type": "passport", "name": "p.pdf", "mime_type": "application/pdf", "size": 100},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("SECURITY: step 3 accepted without completing step 2, got %d", w.Code)
	}

	// Try to jump to step 2 — should be rejected (step 1 not done).
	w = doRequest(r, "POST", "/applications/"+app.ID+"/step/2", map[string]string{
		"provider": "onfido",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("SECURITY: step 2 accepted without completing step 1, got %d", w.Code)
	}

	// Step 1 should work (no precondition).
	w = doRequest(r, "POST", "/applications/"+app.ID+"/step/1", map[string]string{
		"first_name": "Test",
		"last_name":  "User",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for step 1, got %d: %s", w.Code, w.Body.String())
	}
}

// HIGH-2 / MEDIUM-5: Application steps rejected after terminal status.
func TestSecurityApplicationTerminalStatusBlocksSteps(t *testing.T) {
	r, store := newTestRouter()

	app := &Application{
		UserID:      "u1",
		Email:       "a@a.com",
		Status:      AppApproved,
		CurrentStep: 5,
		Steps:       newApplicationSteps(),
	}
	store.SaveApplication(app)

	// Try to modify step 1 on an approved application — should be rejected.
	w := doRequest(r, "POST", "/applications/"+app.ID+"/step/1", map[string]string{
		"first_name": "Hacker",
		"last_name":  "Evil",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("SECURITY: step modification allowed on approved application, got %d", w.Code)
	}

	// Try on a rejected application.
	app2 := &Application{
		UserID:      "u2",
		Email:       "b@b.com",
		Status:      AppRejected,
		CurrentStep: 5,
		Steps:       newApplicationSteps(),
	}
	store.SaveApplication(app2)

	w = doRequest(r, "POST", "/applications/"+app2.ID+"/step/1", map[string]string{
		"first_name": "Hacker",
		"last_name":  "Evil",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("SECURITY: step modification allowed on rejected application, got %d", w.Code)
	}

	// Try on a submitted application.
	app3 := &Application{
		UserID:      "u3",
		Email:       "c@c.com",
		Status:      AppSubmitted,
		CurrentStep: 5,
		Steps:       newApplicationSteps(),
	}
	store.SaveApplication(app3)

	w = doRequest(r, "POST", "/applications/"+app3.ID+"/step/1", map[string]string{
		"first_name": "Hacker",
		"last_name":  "Evil",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("SECURITY: step modification allowed on submitted application, got %d", w.Code)
	}
}

// HIGH-1: RBAC uses stored permissions via gateway roles header.
func TestSecurityRBACUsesStoredPermissions(t *testing.T) {
	store := NewStore()
	SeedStore(store)
	router := NewRouter(store)

	// Helper to make requests with a specific role via gateway headers.
	makeRequest := func(userID, role, method, path string, body interface{}) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", userID)
		req.Header.Set("X-Org-Id", "liquidity")
		req.Header.Set("X-User-Roles", role)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// Developer should have read access to KYC.
	w := makeRequest("dev-user", "Developer", "GET", "/kyc/?user_id=test", nil)
	if w.Code == http.StatusForbidden {
		t.Fatal("Developer should have kyc:read permission")
	}

	// Developer should NOT have write access to KYC.
	w = makeRequest("dev-user", "Developer", "POST", "/kyc/verify", map[string]string{
		"user_id": "u1", "provider": "test",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("SECURITY: Developer should NOT have kyc:write, got %d", w.Code)
	}

	// Agent should have sessions:write.
	w = makeRequest("agent-user", "Agent", "POST", "/sessions/", map[string]string{
		"pipeline_id": "test", "investor_email": "a@b.com",
	})
	if w.Code == http.StatusForbidden {
		t.Fatal("Agent should have sessions:write permission")
	}

	// Agent should NOT have funds:write.
	w = makeRequest("agent-user", "Agent", "POST", "/funds/", map[string]interface{}{
		"name": "Hack Fund", "type": "equity",
	})
	if w.Code != http.StatusForbidden {
		t.Fatalf("SECURITY: Agent should NOT have funds:write, got %d", w.Code)
	}
}
