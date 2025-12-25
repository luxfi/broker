// Package compliance provides KYC/KYB verification, investor onboarding
// pipelines, fund management, eSign, and role-based access control for
// broker-dealer compliance workflows.
//
// All domain types and the store interface are re-exported from
// github.com/luxfi/compliance. This package adds only HTTP handlers
// and broker-specific wiring.
package compliance

import (
	"github.com/luxfi/compliance/pkg/store"
	"github.com/luxfi/compliance/pkg/types"
)

// --- Type aliases: re-export all types from the compliance library ---

type KYCStatus = types.KYCStatus

const (
	KYCPending  = types.KYCPending
	KYCVerified = types.KYCVerified
	KYCFailed   = types.KYCFailed
	KYCExpired  = types.KYCExpired
)

type KYBStatus = types.KYBStatus

const (
	KYBPending  = types.KYBPending
	KYBApproved = types.KYBApproved
	KYBRejected = types.KYBRejected
)

type SessionStatus = types.SessionStatus

const (
	SessionPending    = types.SessionPending
	SessionInProgress = types.SessionInProgress
	SessionCompleted  = types.SessionCompleted
	SessionFailed     = types.SessionFailed
	SessionArchived   = types.SessionArchived
)

type AMLStatus = types.AMLStatus

const (
	AMLPending = types.AMLPending
	AMLCleared = types.AMLCleared
	AMLFlagged = types.AMLFlagged
	AMLBlocked = types.AMLBlocked
	AMLExpired = types.AMLExpired
)

type RiskLevel = types.RiskLevel

const (
	RiskLow      = types.RiskLow
	RiskMedium   = types.RiskMedium
	RiskHigh     = types.RiskHigh
	RiskCritical = types.RiskCritical
)

type ApplicationStatus = types.ApplicationStatus

const (
	AppDraft       = types.AppDraft
	AppInProgress  = types.AppInProgress
	AppSubmitted   = types.AppSubmitted
	AppUnderReview = types.AppUnderReview
	AppApproved    = types.AppApproved
	AppRejected    = types.AppRejected
)

type EnvelopeStatus = types.EnvelopeStatus

const (
	EnvelopePending   = types.EnvelopePending
	EnvelopeSent      = types.EnvelopeSent
	EnvelopeViewed    = types.EnvelopeViewed
	EnvelopeSigned    = types.EnvelopeSigned
	EnvelopeCompleted = types.EnvelopeCompleted
	EnvelopeDeclined  = types.EnvelopeDeclined
	EnvelopeVoided    = types.EnvelopeVoided
)

type (
	Identity        = types.Identity
	Document        = types.Document
	BusinessKYB     = types.BusinessKYB
	PipelineStep    = types.PipelineStep
	Pipeline        = types.Pipeline
	SessionStep     = types.SessionStep
	Session         = types.Session
	Fund            = types.Fund
	FundInvestor    = types.FundInvestor
	Signer          = types.Signer
	Envelope        = types.Envelope
	Template        = types.Template
	Permission      = types.Permission
	Role            = types.Role
	Module          = types.Module
	User            = types.User
	Transaction     = types.Transaction
	Settings        = types.Settings
	Credential      = types.Credential
	AMLScreening    = types.AMLScreening
	ApplicationStep = types.ApplicationStep
	Application     = types.Application
	DocumentUpload  = types.DocumentUpload
	Invoice         = types.Invoice
	BillingInfo     = types.BillingInfo
	DashboardStats  = types.DashboardStats
	ESignStats      = types.ESignStats
)

// --- Store: re-export from the compliance library ---

type ComplianceStore = store.ComplianceStore
type MemoryStore = store.MemoryStore

// NewMemoryStore creates an in-memory compliance store.
func NewMemoryStore() *store.MemoryStore {
	return store.NewMemoryStore()
}

// NewStore is an alias for NewMemoryStore, preserving backward compatibility.
func NewStore() *store.MemoryStore {
	return store.NewMemoryStore()
}

// generateID delegates to the library's GenerateID.
func generateID() string {
	return store.GenerateID()
}
