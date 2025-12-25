package compliance

import (
	"time"

	"github.com/luxfi/compliance/pkg/rbac"
)

// SeedStore populates the store with demo data for development.
// Call this only when BROKER_ENV != "production".
func SeedStore(s ComplianceStore) {
	seedRoles(s)
	seedPipelines(s)
	seedFunds(s)
	seedUsers(s)
	seedTransactions(s)
	seedCredentials(s)
	seedEnvelopes(s)
}

// seedRoles uses the canonical role definitions from the compliance library
// to prevent drift between seed data and the library's RBAC defaults.
func seedRoles(s ComplianceStore) {
	for _, role := range rbac.DefaultRoles() {
		s.SaveRole(role)
	}
}

func seedPipelines(s ComplianceStore) {
	p1 := &Pipeline{
		Name:       "Reg D 506(c) Accredited",
		BusinessID: "demo-biz-1",
		Status:     "active",
		Steps: []PipelineStep{
			{ID: "step-kyc", Name: "Identity Verification", Type: "kyc", Required: true, Order: 1},
			{ID: "step-accred", Name: "Accreditation Check", Type: "accreditation", Required: true, Order: 2},
			{ID: "step-esign", Name: "Subscription Agreement", Type: "esign", Required: true, Order: 3},
			{ID: "step-payment", Name: "Fund Investment", Type: "payment", Required: true, Order: 4},
		},
	}
	s.SavePipeline(p1)

	p2 := &Pipeline{
		Name:       "Reg A+ Public Offering",
		BusinessID: "demo-biz-1",
		Status:     "active",
		Steps: []PipelineStep{
			{ID: "step-kyc", Name: "Identity Verification", Type: "kyc", Required: true, Order: 1},
			{ID: "step-esign", Name: "Subscription Agreement", Type: "esign", Required: true, Order: 2},
			{ID: "step-payment", Name: "Investment Payment", Type: "payment", Required: true, Order: 3},
		},
	}
	s.SavePipeline(p2)

	p3 := &Pipeline{
		Name:       "KYB Business Onboarding",
		BusinessID: "demo-biz-2",
		Status:     "draft",
		Steps: []PipelineStep{
			{ID: "step-kyb", Name: "Business Verification", Type: "kyb", Required: true, Order: 1},
			{ID: "step-docs", Name: "Document Upload", Type: "documents", Required: true, Order: 2},
			{ID: "step-review", Name: "Manual Review", Type: "review", Required: true, Order: 3},
		},
	}
	s.SavePipeline(p3)

	// Seed sessions linked to pipelines.
	sessions := []struct {
		pipelineID string
		email      string
		name       string
		status     SessionStatus
		kycStatus  KYCStatus
	}{
		{p1.ID, "alice@investor.com", "Alice Johnson", SessionCompleted, KYCVerified},
		{p1.ID, "bob@investor.com", "Bob Smith", SessionInProgress, KYCPending},
		{p1.ID, "carol@investor.com", "Carol Williams", SessionPending, KYCPending},
		{p2.ID, "dave@investor.com", "Dave Brown", SessionCompleted, KYCVerified},
		{p2.ID, "eve@investor.com", "Eve Davis", SessionFailed, KYCFailed},
	}
	for _, ss := range sessions {
		s.SaveSession(&Session{
			PipelineID:    ss.pipelineID,
			InvestorEmail: ss.email,
			InvestorName:  ss.name,
			Status:        ss.status,
			KYCStatus:     ss.kycStatus,
		})
	}
}

func seedFunds(s ComplianceStore) {
	f1 := &Fund{
		Name:          "Series A Growth Fund",
		BusinessID:    "demo-biz-1",
		Type:          "equity",
		MinInvestment: 25000,
		Status:        "raising",
	}
	s.SaveFund(f1)

	f2 := &Fund{
		Name:          "Real Estate Income Fund",
		BusinessID:    "demo-biz-1",
		Type:          "real_estate",
		MinInvestment: 50000,
		Status:        "open",
	}
	s.SaveFund(f2)

	// Add sample investors to fund 1.
	s.AddFundInvestor(&FundInvestor{
		FundID: f1.ID, InvestorID: "inv-alice", Name: "Alice Johnson",
		Email: "alice@investor.com", Amount: 50000, Status: "funded",
	})
	s.AddFundInvestor(&FundInvestor{
		FundID: f1.ID, InvestorID: "inv-dave", Name: "Dave Brown",
		Email: "dave@investor.com", Amount: 100000, Status: "committed",
	})
}

func seedUsers(s ComplianceStore) {
	now := time.Now().UTC()
	users := []User{
		{Name: "Sarah Chen", Email: "sarah@example.com", Role: "owner", Status: "active", LastLogin: now},
		{Name: "Mike Torres", Email: "mike@example.com", Role: "admin", Status: "active", LastLogin: now.Add(-2 * time.Hour)},
		{Name: "Lisa Park", Email: "lisa@example.com", Role: "manager", Status: "active", LastLogin: now.Add(-24 * time.Hour)},
		{Name: "James Wilson", Email: "james@example.com", Role: "developer", Status: "active", LastLogin: now.Add(-48 * time.Hour)},
		{Name: "Ana Rodriguez", Email: "ana@example.com", Role: "agent", Status: "inactive", LastLogin: now.Add(-720 * time.Hour)},
	}
	for i := range users {
		s.SaveUser(&users[i])
	}
}

func seedTransactions(s ComplianceStore) {
	txns := []Transaction{
		{Type: "deposit", Asset: "USD", Amount: 50000, Fee: 0, Status: "completed", Date: "2026-03-15"},
		{Type: "trade", Asset: "BTC", Amount: 1.5, Fee: 15.00, Status: "completed", Date: "2026-03-16"},
		{Type: "withdrawal", Asset: "USD", Amount: 10000, Fee: 25.00, Status: "completed", Date: "2026-03-17"},
		{Type: "dividend", Asset: "USD", Amount: 2500, Fee: 0, Status: "completed", Date: "2026-03-18"},
		{Type: "deposit", Asset: "ETH", Amount: 10.0, Fee: 0, Status: "pending", Date: "2026-03-19"},
		{Type: "trade", Asset: "ETH", Amount: 5.0, Fee: 12.50, Status: "completed", Date: "2026-03-20"},
	}
	for i := range txns {
		s.SaveTransaction(&txns[i])
	}
}

func seedCredentials(s ComplianceStore) {
	creds := []Credential{
		{Name: "Production API", KeyPrefix: "sk_live_", Permissions: []string{"read", "trade"}, CreatedAt: time.Now().UTC().Add(-720 * time.Hour)},
		{Name: "Staging API", KeyPrefix: "sk_test_", Permissions: []string{"read"}, CreatedAt: time.Now().UTC().Add(-360 * time.Hour)},
	}
	for i := range creds {
		s.SaveCredential(&creds[i])
	}
}

func seedEnvelopes(s ComplianceStore) {
	now := time.Now().UTC()
	envelopes := []Envelope{
		{
			Subject: "Subscription Agreement - Series A",
			Status:  EnvelopePending,
			Signers: []Signer{
				{ID: generateID(), Name: "Alice Johnson", Email: "alice@investor.com", Role: "investor", Status: "pending"},
			},
			CreatedAt: now.Add(-48 * time.Hour),
		},
		{
			Subject: "NDA - Confidential Offering",
			Status:  EnvelopeCompleted,
			Signers: []Signer{
				{ID: generateID(), Name: "Dave Brown", Email: "dave@investor.com", Role: "investor", Status: "signed", SignedAt: now.Add(-24 * time.Hour).Format(time.RFC3339)},
			},
			CreatedAt: now.Add(-72 * time.Hour),
		},
		{
			Subject: "Operating Agreement Amendment",
			Status:  EnvelopeSent,
			Signers: []Signer{
				{ID: generateID(), Name: "Bob Smith", Email: "bob@investor.com", Role: "investor", Status: "pending"},
				{ID: generateID(), Name: "Carol Williams", Email: "carol@investor.com", Role: "witness", Status: "pending"},
			},
			CreatedAt: now.Add(-12 * time.Hour),
		},
	}
	for i := range envelopes {
		s.SaveEnvelope(&envelopes[i])
	}

	// Seed templates.
	templates := []Template{
		{Name: "Subscription Agreement", Description: "Standard subscription agreement for fund investments", Roles: []string{"investor", "issuer"}},
		{Name: "NDA", Description: "Non-disclosure agreement for confidential offerings", Roles: []string{"investor"}},
		{Name: "Operating Agreement", Description: "LLC operating agreement template", Roles: []string{"member", "manager"}},
	}
	for i := range templates {
		s.SaveTemplate(&templates[i])
	}
}
