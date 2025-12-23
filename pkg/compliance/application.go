package compliance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

// applicationHandler manages the 5-step investor onboarding flow.
type applicationHandler struct {
	store ComplianceStore
}

// newApplicationSteps returns the 5 default onboarding steps.
func newApplicationSteps() []ApplicationStep {
	return []ApplicationStep{
		{Step: 1, Name: "Basic Info & Contact", Status: "pending"},
		{Step: 2, Name: "Identity Verification", Status: "pending"},
		{Step: 3, Name: "Document Upload", Status: "pending"},
		{Step: 4, Name: "Compliance Screening", Status: "pending"},
		{Step: 5, Name: "Review & Submit", Status: "pending"},
	}
}

// handleCreate starts a new onboarding application.
func (h *applicationHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	app := &Application{
		UserID:      req.UserID,
		Email:       req.Email,
		Status:      AppDraft,
		CurrentStep: 1,
		KYCStatus:   KYCPending,
		AMLStatus:   AMLPending,
		Steps:       newApplicationSteps(),
	}

	if err := h.store.SaveApplication(app); err != nil {
		log.Error().Err(err).Str("user_id", req.UserID).Msg("application: failed to create")
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

// handleGet returns an application by ID.
func (h *applicationHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.store.GetApplication(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// handleGetByUser returns the application for a given user.
func (h *applicationHandler) handleGetByUser(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id query parameter is required")
		return
	}
	app, err := h.store.GetApplicationByUser(userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// handleList returns all applications, optionally filtered by status.
func (h *applicationHandler) handleList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status != "" {
		writeJSON(w, http.StatusOK, h.store.ListApplicationsByStatus(ApplicationStatus(status)))
		return
	}
	writeJSON(w, http.StatusOK, h.store.ListApplications())
}

// handleStep1 updates Step 1: Basic Info + Contact.
func (h *applicationHandler) handleStep1(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.store.GetApplication(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var req struct {
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		Email        string `json:"email,omitempty"`
		Phone        string `json:"phone,omitempty"`
		DateOfBirth  string `json:"date_of_birth,omitempty"`
		SSN          string `json:"ssn,omitempty"` // will be hashed, never stored raw
		AddressLine1 string `json:"address_line1,omitempty"`
		AddressLine2 string `json:"address_line2,omitempty"`
		City         string `json:"city,omitempty"`
		State        string `json:"state,omitempty"`
		ZipCode      string `json:"zip_code,omitempty"`
		Country      string `json:"country,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.FirstName == "" {
		writeError(w, http.StatusBadRequest, "first_name is required")
		return
	}
	if req.LastName == "" {
		writeError(w, http.StatusBadRequest, "last_name is required")
		return
	}

	app.FirstName = req.FirstName
	app.LastName = req.LastName
	if req.Email != "" {
		app.Email = req.Email
	}
	app.Phone = req.Phone
	app.DateOfBirth = req.DateOfBirth
	app.AddressLine1 = req.AddressLine1
	app.AddressLine2 = req.AddressLine2
	app.City = req.City
	app.State = req.State
	app.ZipCode = req.ZipCode
	app.Country = req.Country

	// Hash SSN if provided -- NEVER store plaintext.
	if req.SSN != "" {
		hash := sha256.Sum256([]byte(req.SSN))
		app.SSNHash = hex.EncodeToString(hash[:])
		if len(req.SSN) >= 4 {
			app.SSNLast4 = req.SSN[len(req.SSN)-4:]
		}
	}

	app.Status = AppInProgress
	if app.CurrentStep < 2 {
		app.CurrentStep = 2
	}
	markStepCompleted(app, 1)

	if err := h.store.SaveApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// handleStep2 updates Step 2: Identity Verification.
func (h *applicationHandler) handleStep2(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.store.GetApplication(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var req struct {
		Provider       string `json:"provider"`        // onfido, berbix, idmerit
		VerificationID string `json:"verification_id"` // external verification ID
		Status         string `json:"status,omitempty"` // pending, verified, failed
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Provider == "" {
		writeError(w, http.StatusBadRequest, "provider is required")
		return
	}

	// Create an identity record linked to this application.
	ident := &Identity{
		UserID:   app.UserID,
		Provider: req.Provider,
		Status:   KYCPending,
		Data: map[string]interface{}{
			"application_id":  app.ID,
			"verification_id": req.VerificationID,
		},
	}
	if req.Status == "verified" {
		ident.Status = KYCVerified
	} else if req.Status == "failed" {
		ident.Status = KYCFailed
	}

	if err := h.store.SaveIdentity(ident); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	app.KYCStatus = ident.Status
	if ident.Status == KYCVerified {
		markStepCompleted(app, 2)
		if app.CurrentStep < 3 {
			app.CurrentStep = 3
		}
	} else if ident.Status == KYCFailed {
		markStepFailed(app, 2)
	}

	if err := h.store.SaveApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// handleStep3 updates Step 3: Document Upload.
func (h *applicationHandler) handleStep3(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.store.GetApplication(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var req struct {
		Documents []struct {
			Type     string `json:"type"`
			Name     string `json:"name"`
			MimeType string `json:"mime_type"`
			Size     int64  `json:"size"`
		} `json:"documents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Documents) == 0 {
		writeError(w, http.StatusBadRequest, "at least one document is required")
		return
	}

	for _, d := range req.Documents {
		if d.Type == "" {
			writeError(w, http.StatusBadRequest, "document type is required")
			return
		}
		if d.Name == "" {
			writeError(w, http.StatusBadRequest, "document name is required")
			return
		}

		doc := &DocumentUpload{
			ApplicationID: app.ID,
			UserID:        app.UserID,
			Type:          d.Type,
			Name:          d.Name,
			MimeType:      d.MimeType,
			Size:          d.Size,
			Status:        "pending",
		}
		if err := h.store.SaveDocumentUpload(doc); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	markStepCompleted(app, 3)
	if app.CurrentStep < 4 {
		app.CurrentStep = 4
	}

	if err := h.store.SaveApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// handleStep4 updates Step 4: Compliance/AML Screening.
func (h *applicationHandler) handleStep4(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.store.GetApplication(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	// Create an AML screening record for this application.
	screening := &AMLScreening{
		AccountID: app.ID,
		UserID:    app.UserID,
		Type:      "sanctions",
		Status:    AMLPending,
		RiskLevel: RiskLow,
		Provider:  "manual",
	}
	if err := h.store.SaveAMLScreening(screening); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	app.AMLStatus = AMLPending
	markStepCompleted(app, 4)
	if app.CurrentStep < 5 {
		app.CurrentStep = 5
	}

	if err := h.store.SaveApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// handleStep5 updates Step 5: Review + Submit.
func (h *applicationHandler) handleStep5(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.store.GetApplication(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	var req struct {
		Confirmed bool `json:"confirmed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !req.Confirmed {
		writeError(w, http.StatusBadRequest, "confirmed must be true to submit")
		return
	}

	markStepCompleted(app, 5)
	app.Status = AppSubmitted
	app.SubmittedAt = time.Now().UTC()

	if err := h.store.SaveApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// handleReview allows an admin to approve or reject a submitted application.
func (h *applicationHandler) handleReview(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	app, err := h.store.GetApplication(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	if app.Status != AppSubmitted && app.Status != AppUnderReview {
		writeError(w, http.StatusBadRequest, "application must be submitted or under review")
		return
	}

	var req struct {
		Decision   string `json:"decision"` // approved, rejected
		ReviewedBy string `json:"reviewed_by"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Decision != "approved" && req.Decision != "rejected" {
		writeError(w, http.StatusBadRequest, "decision must be 'approved' or 'rejected'")
		return
	}
	if req.ReviewedBy == "" {
		writeError(w, http.StatusBadRequest, "reviewed_by is required")
		return
	}

	switch req.Decision {
	case "approved":
		app.Status = AppApproved
	case "rejected":
		app.Status = AppRejected
	}
	app.ReviewedBy = req.ReviewedBy
	app.ReviewedAt = time.Now().UTC()

	if err := h.store.SaveApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// handleGetDocuments lists documents for an application.
func (h *applicationHandler) handleGetDocuments(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Verify application exists.
	if _, err := h.store.GetApplication(id); err != nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, h.store.ListDocumentUploads(id))
}

// markStepCompleted sets a step to completed.
func markStepCompleted(app *Application, step int) {
	for i := range app.Steps {
		if app.Steps[i].Step == step {
			app.Steps[i].Status = "completed"
			app.Steps[i].CompletedAt = time.Now().UTC()
			return
		}
	}
}

// markStepFailed sets a step to failed.
func markStepFailed(app *Application, step int) {
	for i := range app.Steps {
		if app.Steps[i].Step == step {
			app.Steps[i].Status = "failed"
			return
		}
	}
}
