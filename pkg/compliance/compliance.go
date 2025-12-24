// Package compliance provides KYC/KYB verification, investor onboarding
// pipelines, fund management, eSign, and role-based access control for
// broker-dealer compliance workflows.
package compliance

import "time"

// --- KYC/KYB Status Types ---

// KYCStatus tracks identity verification state.
type KYCStatus string

const (
	KYCPending  KYCStatus = "pending"
	KYCVerified KYCStatus = "verified"
	KYCFailed   KYCStatus = "failed"
	KYCExpired  KYCStatus = "expired"
)

// KYBStatus tracks business verification state.
type KYBStatus string

const (
	KYBPending  KYBStatus = "pending"
	KYBApproved KYBStatus = "approved"
	KYBRejected KYBStatus = "rejected"
)

// SessionStatus tracks onboarding session state.
type SessionStatus string

const (
	SessionPending    SessionStatus = "pending"
	SessionInProgress SessionStatus = "in_progress"
	SessionCompleted  SessionStatus = "completed"
	SessionFailed     SessionStatus = "failed"
	SessionArchived   SessionStatus = "archived"
)

// --- Core Types ---

// Identity represents a KYC-verified individual identity.
type Identity struct {
	ID        string                 `json:"id"`
	UserID    string                 `json:"user_id"`
	Provider  string                 `json:"provider"` // onfido, berbix, idmerit, etc.
	Status    KYCStatus              `json:"status"`
	Data      map[string]interface{} `json:"data,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Document is a verification document (passport, license, etc.).
type Document struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // passport, drivers_license, utility_bill, ein_letter
	Name      string `json:"name,omitempty"`
	MimeType  string `json:"mime_type,omitempty"`
	Content   string `json:"content,omitempty"` // base64
	Status    string `json:"status,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// BusinessKYB represents a business verification record.
type BusinessKYB struct {
	ID         string     `json:"id"`
	BusinessID string     `json:"business_id"`
	Name       string     `json:"name"`
	EIN        string     `json:"ein"`
	Status     KYBStatus  `json:"status"`
	Documents  []Document `json:"documents,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// --- Pipeline / Session Types ---

// PipelineStep defines a single step in an onboarding pipeline.
type PipelineStep struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // kyc, accreditation, esign, payment, review
	Required bool   `json:"required"`
	Order    int    `json:"order"`
}

// Pipeline is a configurable investor onboarding flow.
type Pipeline struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	BusinessID string         `json:"business_id"`
	Steps      []PipelineStep `json:"steps"`
	Status     string         `json:"status"` // active, draft, archived
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// SessionStep tracks completion of a single pipeline step within a session.
type SessionStep struct {
	StepID      string    `json:"step_id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Status      string    `json:"status"` // pending, completed, failed, skipped
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// Session is an investor's progress through an onboarding pipeline.
type Session struct {
	ID            string        `json:"id"`
	PipelineID    string        `json:"pipeline_id"`
	InvestorEmail string        `json:"investor_email"`
	InvestorName  string        `json:"investor_name"`
	Status        SessionStatus `json:"status"`
	KYCStatus     KYCStatus     `json:"kyc_status"`
	Steps         []SessionStep `json:"steps,omitempty"`
	CreatedAt     time.Time     `json:"created_at"`
	CompletedAt   time.Time     `json:"completed_at,omitempty"`
}

// --- Fund Types ---

// Fund represents an investment fund.
type Fund struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	BusinessID    string  `json:"business_id"`
	Type          string  `json:"type"` // equity, debt, real_estate
	MinInvestment float64 `json:"min_investment"`
	TotalRaised   float64 `json:"total_raised"`
	InvestorCount int     `json:"investor_count"`
	Status        string  `json:"status"` // open, closed, raising
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// FundInvestor represents an investor's participation in a fund.
type FundInvestor struct {
	ID         string    `json:"id"`
	FundID     string    `json:"fund_id"`
	InvestorID string    `json:"investor_id"`
	Name       string    `json:"name"`
	Email      string    `json:"email"`
	Amount     float64   `json:"amount"`
	Status     string    `json:"status"` // committed, funded, withdrawn
	CreatedAt  time.Time `json:"created_at"`
}

// --- eSign Types ---

// EnvelopeStatus tracks document signing state.
type EnvelopeStatus string

const (
	EnvelopePending   EnvelopeStatus = "pending"
	EnvelopeSent      EnvelopeStatus = "sent"
	EnvelopeViewed    EnvelopeStatus = "viewed"
	EnvelopeSigned    EnvelopeStatus = "signed"
	EnvelopeCompleted EnvelopeStatus = "completed"
	EnvelopeDeclined  EnvelopeStatus = "declined"
	EnvelopeVoided    EnvelopeStatus = "voided"
)

// Signer represents a person who must sign an envelope.
type Signer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role,omitempty"` // investor, issuer, witness
	Status    string `json:"status"`         // pending, signed, declined
	SignedAt  string `json:"signed_at,omitempty"`
}

// Envelope is a document package sent for signatures.
type Envelope struct {
	ID         string         `json:"id"`
	TemplateID string         `json:"template_id,omitempty"`
	Subject    string         `json:"subject"`
	Message    string         `json:"message,omitempty"`
	Status     EnvelopeStatus `json:"status"`
	Signers    []Signer       `json:"signers"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Template is a reusable eSign document template.
type Template struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Content     string    `json:"content,omitempty"` // base64 PDF or URL
	Roles       []string  `json:"roles,omitempty"`   // expected signer roles
	CreatedAt   time.Time `json:"created_at"`
}

// --- RBAC Types ---

// Permission defines a single allowed action on a module.
type Permission struct {
	Module string `json:"module"` // kyc, funds, esign, pipelines, sessions, roles
	Action string `json:"action"` // read, write, delete, admin
}

// Role is a named set of permissions.
type Role struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Permissions []Permission `json:"permissions"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Module describes a compliance module for the permission matrix.
type Module struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Actions     []string `json:"actions"` // available actions
}

// --- User Management Types ---

// User represents an admin/compliance platform user.
type User struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`   // owner, admin, manager, developer, agent
	Status    string    `json:"status"` // active, inactive, suspended
	LastLogin time.Time `json:"last_login"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Transaction Types ---

// Transaction represents a financial transaction record.
type Transaction struct {
	ID     string  `json:"id"`
	Type   string  `json:"type"`  // deposit, withdrawal, trade, dividend
	Asset  string  `json:"asset"` // USD, BTC, ETH, etc.
	Amount float64 `json:"amount"`
	Fee    float64 `json:"fee"`
	Status string  `json:"status"` // pending, completed, failed, cancelled
	Date   string  `json:"date"`
}

// --- Settings Types ---

// Settings holds business configuration for the compliance tenant.
type Settings struct {
	BusinessName      string `json:"business_name"`
	Timezone          string `json:"timezone"`
	Currency          string `json:"currency"`
	NotificationEmail string `json:"notification_email"`
}

// --- Credential Types ---

// Credential represents an API key visible to the compliance admin.
type Credential struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	KeyPrefix   string    `json:"key_prefix"` // first 8 chars, for display only
	Permissions []string  `json:"permissions"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   string    `json:"expires_at,omitempty"`
}

// --- Billing Types ---

// Invoice represents a billing invoice.
type Invoice struct {
	ID     string  `json:"id"`
	Date   string  `json:"date"`
	Amount float64 `json:"amount"`
	Status string  `json:"status"` // paid, pending, overdue
}

// BillingInfo holds billing and subscription details.
type BillingInfo struct {
	Plan          string    `json:"plan"`           // starter, professional, enterprise
	PaymentMethod string    `json:"payment_method"` // visa_4242, etc.
	NextBilling   string    `json:"next_billing"`
	MonthlyUsage  float64   `json:"monthly_usage"`
	Invoices      []Invoice `json:"invoices"`
}

// --- Dashboard Types ---

// DashboardStats holds aggregate compliance dashboard statistics.
type DashboardStats struct {
	ActiveSessions      int            `json:"activeSessions"`
	PendingKYC          int            `json:"pendingKYC"`
	TotalFunds          int            `json:"totalFunds"`
	MonthlyTransactions int            `json:"monthlyTransactions"`
	RecentSessions      []*Session     `json:"recentSessions"`
	RecentTransactions  []*Transaction `json:"recentTransactions"`
}

// --- eSign Dashboard Types ---

// ESignStats holds aggregate eSign statistics.
type ESignStats struct {
	Pending   int         `json:"pending"`
	Completed int         `json:"completed"`
	Draft     int         `json:"draft"`
	Templates int         `json:"templates"`
	Recent    []*Envelope `json:"recent"`
}
