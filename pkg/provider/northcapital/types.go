// Package northcapital — local type definitions for the North Capital
// TransactAPI broker-side adapter (party onboarding, KYC/AML/accredited,
// offerings, trades, custody, ATS, webhook).
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/
//
// Where a sibling type already exists in `broker/pkg/types/`, this file
// aliases to it rather than re-declaring (per the local idiom — one
// shape per concept across the broker package). New shapes specific
// to TransactAPI's surface (Party / Entity / KYCResult / AMLResult /
// AccreditedResult / Offering / Trade / BulkTradeResult /
// CustodyAccount / ATSEvent / SecondaryTrade) live here.
package northcapital

import (
	"time"

	"github.com/luxfi/broker/pkg/types"
)

// --- Aliases to existing broker/pkg/types so the rest of the stack
// (smart-order-routing, captable binding, AML overlay, reporting)
// treats TransactAPI's objects identically to every other adapter. ---

// WebhookEvent is the broker-wide normalized webhook event shape
// already defined in broker/pkg/types/bd_trade.go. The TransactAPI
// adapter decrypts and maps the upstream encrypted-webhook payload
// into this shape so downstream consumers do not know it came from
// TransactAPI specifically.
type WebhookEvent = types.WebhookEvent

// --- Party / Entity (TransactAPI Investor Onboarding) ---

// PartyType discriminates individual / joint / entity onboarding.
type PartyType string

const (
	PartyIndividual PartyType = "individual"
	PartyJoint      PartyType = "joint"
	PartyEntity     PartyType = "entity"
)

// Party is a natural-person (or joint) investor on TransactAPI.
// Maps to TransactAPI `partyDetails` records, identified by `partyId`.
type Party struct {
	ID            string            `json:"id"`             // TransactAPI partyId
	Type          PartyType         `json:"type"`
	GivenName     string            `json:"given_name,omitempty"`
	FamilyName    string            `json:"family_name,omitempty"`
	Email         string            `json:"email,omitempty"`
	Phone         string            `json:"phone,omitempty"`
	DateOfBirth   string            `json:"date_of_birth,omitempty"`   // ISO 8601
	TaxIDType     string            `json:"tax_id_type,omitempty"`     // ssn, ein
	TaxIDLast4    string            `json:"tax_id_last4,omitempty"`    // never store full
	Domicile      string            `json:"domicile,omitempty"`        // ISO 3166-1 alpha-2
	Citizenship   string            `json:"citizenship,omitempty"`
	Address       *PartyAddress     `json:"address,omitempty"`
	KYCStatus     string            `json:"kyc_status,omitempty"`
	AMLStatus     string            `json:"aml_status,omitempty"`
	AccreditedAt  *time.Time        `json:"accredited_at,omitempty"`
	Meta          map[string]string `json:"meta,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// PartyAddress is the postal address attached to a Party / Entity.
type PartyAddress struct {
	Street1    string `json:"street_1"`
	Street2    string `json:"street_2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	Country    string `json:"country"` // ISO 3166-1 alpha-2
}

// Entity is a legal-entity (LLC / corp / trust / partnership / SPV)
// investor on TransactAPI. Maps to TransactAPI `entityDetails`.
type Entity struct {
	ID            string            `json:"id"` // TransactAPI entityId
	LegalName     string            `json:"legal_name"`
	EntityType    string            `json:"entity_type"` // llc, corp, trust, partnership, spv
	FormationCountry string         `json:"formation_country"`
	FormationState   string         `json:"formation_state,omitempty"`
	EIN              string         `json:"ein,omitempty"`
	Address          *PartyAddress  `json:"address,omitempty"`
	Beneficials      []string       `json:"beneficial_owner_party_ids,omitempty"`
	ControlPersons   []string       `json:"control_person_party_ids,omitempty"`
	KYCStatus        string         `json:"kyc_status,omitempty"`
	AMLStatus        string         `json:"aml_status,omitempty"`
	Meta             map[string]string `json:"meta,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

// --- KYC / AML / Accredited (TransactAPI verification) ---

// KYCResult is the result of a TransactAPI `performKYC` call.
type KYCResult struct {
	PartyID   string    `json:"party_id"`
	Status    string    `json:"status"` // pass, fail, manual_review
	Score     int       `json:"score,omitempty"`
	Reasons   []string  `json:"reasons,omitempty"`
	RunAt     time.Time `json:"run_at"`
}

// AMLResult is the result of a TransactAPI `performAML` call
// (sanctions / OFAC / SDN / PEP screening).
type AMLResult struct {
	PartyID   string    `json:"party_id"`
	Status    string    `json:"status"` // pass, fail, manual_review
	Hits      []AMLHit  `json:"hits,omitempty"`
	RunAt     time.Time `json:"run_at"`
}

// AMLHit is a single sanctions/PEP match returned by performAML.
type AMLHit struct {
	List       string  `json:"list"`        // OFAC SDN, EU Consolidated, UN, etc.
	Name       string  `json:"name"`
	MatchScore float64 `json:"match_score"`
	Reference  string  `json:"reference,omitempty"`
}

// AccreditedMethod is the documentation pathway used for Reg D 506(c)
// accredited-investor verification.
type AccreditedMethod string

const (
	AccreditedByIncome    AccreditedMethod = "income"
	AccreditedByNetWorth  AccreditedMethod = "net_worth"
	AccreditedByThirdParty AccreditedMethod = "third_party_letter"
	AccreditedByLicense   AccreditedMethod = "professional_license"
)

// AccreditedResult is the result of a TransactAPI `performAccredited` call.
type AccreditedResult struct {
	PartyID    string           `json:"party_id"`
	Method     AccreditedMethod `json:"method"`
	Status     string           `json:"status"` // pass, fail, manual_review, expired
	VerifiedAt *time.Time       `json:"verified_at,omitempty"`
	ExpiresAt  *time.Time       `json:"expires_at,omitempty"` // 506(c) verification expires
}

// --- Offerings ---

// Offering is a TransactAPI Offering — a single securities issuance
// (Reg D 506(b)/(c), Reg CF, Reg A+, private placement).
type Offering struct {
	ID             string            `json:"id"` // TransactAPI offeringId
	IssuerName     string            `json:"issuer_name"`
	OfferingName   string            `json:"offering_name"`
	OfferingType   string            `json:"offering_type"` // reg_d_506b, reg_d_506c, reg_cf, reg_a_plus, ppm
	TargetAmount   string            `json:"target_amount,omitempty"`
	MinInvestment  string            `json:"min_investment,omitempty"`
	MaxInvestment  string            `json:"max_investment,omitempty"`
	UnitPrice      string            `json:"unit_price,omitempty"`
	Currency       string            `json:"currency"` // typically USD
	Status         string            `json:"status"`   // draft, open, closed, cancelled
	OpenedAt       *time.Time        `json:"opened_at,omitempty"`
	ClosedAt       *time.Time        `json:"closed_at,omitempty"`
	SecurityClass  string            `json:"security_class,omitempty"`
	CUSIP          string            `json:"cusip,omitempty"`
	Meta           map[string]string `json:"meta,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// --- Trades / Subscriptions ---

// Trade is a TransactAPI subscription / secondary-market trade record.
// Distinct from broker/pkg/types.Trade (which is a market-data trade
// print). This is the BD-side booked trade ticket.
type Trade struct {
	ID             string            `json:"id"`            // TransactAPI tradeId
	OfferingID     string            `json:"offering_id"`
	PartyID        string            `json:"party_id"`      // investor
	EntityID       string            `json:"entity_id,omitempty"` // entity buyer
	Side           string            `json:"side"`          // buy, sell
	Units          string            `json:"units"`
	UnitPrice      string            `json:"unit_price"`
	GrossAmount    string            `json:"gross_amount"`
	NetAmount      string            `json:"net_amount,omitempty"`
	Currency       string            `json:"currency"`
	Status         string            `json:"status"` // pending, funded, settled, cancelled, returned
	PaymentMethod  string            `json:"payment_method,omitempty"` // ach, wire, credit_card, check, ira
	SignedAt       *time.Time        `json:"signed_at,omitempty"`      // e-sign callback timestamp
	SettledAt      *time.Time        `json:"settled_at,omitempty"`
	Meta           map[string]string `json:"meta,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// BulkTradeResult is the response to a TransactAPI bulk-trade CSV upload.
type BulkTradeResult struct {
	BatchID   string         `json:"batch_id"`
	Accepted  int            `json:"accepted"`
	Rejected  int            `json:"rejected"`
	Errors    []BulkTradeErr `json:"errors,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// BulkTradeErr is a per-row error in a bulk-trade upload.
type BulkTradeErr struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// --- Custody ---

// CustodyAccount is a TransactAPI custody account opened for a Party
// or Entity (held at NCPS's custody provider, opened under the
// TransactAPI custody-opening workflow).
type CustodyAccount struct {
	ID            string            `json:"id"` // TransactAPI custodyAccountId
	PartyID       string            `json:"party_id,omitempty"`
	EntityID      string            `json:"entity_id,omitempty"`
	AccountType   string            `json:"account_type"` // individual, joint, entity, ira
	Status        string            `json:"status"`       // pending, open, closed
	OpenedAt      *time.Time        `json:"opened_at,omitempty"`
	ClosedAt      *time.Time        `json:"closed_at,omitempty"`
	Meta          map[string]string `json:"meta,omitempty"`
}

// --- ATS / Secondary trades ---

// ATSEvent is a Lux-side ATS event published to TransactAPI's ATS
// webhook channel (order placed / cancelled / matched / cleared).
type ATSEvent struct {
	EventID    string            `json:"event_id"`
	EventType  string            `json:"event_type"` // order_placed, order_cancelled, match, clear, settle
	OfferingID string            `json:"offering_id"`
	TradeID    string            `json:"trade_id,omitempty"`
	PartyID    string            `json:"party_id,omitempty"`
	EntityID   string            `json:"entity_id,omitempty"`
	Side       string            `json:"side,omitempty"`
	Units      string            `json:"units,omitempty"`
	UnitPrice  string            `json:"unit_price,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
}

// SecondaryTrade is a TransactAPI Secondary Trades Directory record.
type SecondaryTrade struct {
	ID         string    `json:"id"`
	OfferingID string    `json:"offering_id"`
	BuyerID    string    `json:"buyer_party_id"`
	SellerID   string    `json:"seller_party_id"`
	Units      string    `json:"units"`
	UnitPrice  string    `json:"unit_price"`
	Currency   string    `json:"currency"`
	Status     string    `json:"status"` // cleared, pending, cancelled, settled
	TradedAt   time.Time `json:"traded_at"`
	SettledAt  *time.Time `json:"settled_at,omitempty"`
}

// --- Request shapes (per-method) ---

// CreatePartyRequest is the input to CreateParty.
type CreatePartyRequest struct {
	Type          PartyType     `json:"type"`
	GivenName     string        `json:"given_name"`
	FamilyName    string        `json:"family_name"`
	Email         string        `json:"email"`
	Phone         string        `json:"phone,omitempty"`
	DateOfBirth   string        `json:"date_of_birth,omitempty"`
	TaxIDType     string        `json:"tax_id_type,omitempty"`
	TaxID         string        `json:"tax_id,omitempty"`
	Domicile      string        `json:"domicile,omitempty"`
	Citizenship   string        `json:"citizenship,omitempty"`
	Address       *PartyAddress `json:"address,omitempty"`

	// IdempotencyKey — see treasury/pkg/types.CreatePaymentRequest.IdempotencyKey.
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	// OrgID folds into the deterministic-fallback idempotency key.
	OrgID string `json:"org_id,omitempty"`
}

// CreateEntityRequest is the input to CreateEntity.
type CreateEntityRequest struct {
	LegalName        string        `json:"legal_name"`
	EntityType       string        `json:"entity_type"`
	FormationCountry string        `json:"formation_country"`
	FormationState   string        `json:"formation_state,omitempty"`
	EIN              string        `json:"ein,omitempty"`
	Address          *PartyAddress `json:"address,omitempty"`
	Beneficials      []string      `json:"beneficial_owner_party_ids,omitempty"`
	ControlPersons   []string      `json:"control_person_party_ids,omitempty"`

	IdempotencyKey string `json:"idempotency_key,omitempty"`
	OrgID          string `json:"org_id,omitempty"`
}

// CreateOfferingRequest is the input to CreateOffering.
type CreateOfferingRequest struct {
	IssuerName    string `json:"issuer_name"`
	OfferingName  string `json:"offering_name"`
	OfferingType  string `json:"offering_type"`
	TargetAmount  string `json:"target_amount,omitempty"`
	MinInvestment string `json:"min_investment,omitempty"`
	MaxInvestment string `json:"max_investment,omitempty"`
	UnitPrice     string `json:"unit_price,omitempty"`
	Currency      string `json:"currency"`
	SecurityClass string `json:"security_class,omitempty"`
	CUSIP         string `json:"cusip,omitempty"`

	IdempotencyKey string `json:"idempotency_key,omitempty"`
	OrgID          string `json:"org_id,omitempty"`
}

// CreateTradeRequest is the input to CreateTrade.
type CreateTradeRequest struct {
	OfferingID    string `json:"offering_id"`
	PartyID       string `json:"party_id,omitempty"`
	EntityID      string `json:"entity_id,omitempty"`
	Side          string `json:"side"`
	Units         string `json:"units"`
	UnitPrice     string `json:"unit_price,omitempty"`
	PaymentMethod string `json:"payment_method,omitempty"`

	IdempotencyKey string `json:"idempotency_key,omitempty"`
	OrgID          string `json:"org_id,omitempty"`
}

// CustodyOpenRequest is the input to OpenCustodyAccount.
type CustodyOpenRequest struct {
	AccountType string `json:"account_type"` // individual, joint, entity, ira

	IdempotencyKey string `json:"idempotency_key,omitempty"`
	OrgID          string `json:"org_id,omitempty"`
}

// --- Capability shape ---
//
// BrokerCapability is the broker-side capability descriptor returned
// by Provider.Capabilities(). It mirrors the shape of
// treasury/pkg/types.ProviderCapability (the existing on-the-shelf
// idiom). Promoted to broker/pkg/types in a follow-up commit so every
// broker adapter declares its surface uniformly; defined here locally
// in the scaffolding pass so this package compiles independently.
type BrokerCapability struct {
	Name            string   `json:"name"`
	PaymentTypes    []string `json:"payment_types"`    // ach, wire, credit_card, check, ira
	Features        []string `json:"features"`         // bd, ta, ats, custody, kyc, aml, accredited, offerings, secondary_trades
	Countries       []string `json:"countries"`        // ISO 3166-1 alpha-2
	SettlementSpeed string   `json:"settlement_speed"` // t+0_to_t+2, t+1, t+2
	Status          string   `json:"status"`           // active, beta, disabled
}
