// Package jube provides an HTTP client for the Jube AML/fraud detection sidecar.
// Jube runs as a sidecar service and exposes REST endpoints for real-time
// transaction risk scoring, sanctions screening, and case management.
package jube

import "time"

// Action constants returned by Jube risk scoring.
const (
	ActionAllow  = "allow"
	ActionBlock  = "block"
	ActionReview = "review"
)

// TransactionRequest is sent to Jube for real-time risk scoring.
type TransactionRequest struct {
	EntityAnalysisModelID      int                    `json:"entityAnalysisModelId"`
	EntityInstanceEntryPayload map[string]interface{} `json:"entityInstanceEntryPayload"`
}

// TransactionResponse is the risk assessment returned by Jube.
type TransactionResponse struct {
	Score     float64           `json:"score"`
	Alerts    []Alert           `json:"alerts"`
	Action    string            `json:"action"` // allow, block, review
	ModelTags map[string]string `json:"modelTags,omitempty"`
}

// Alert represents a single AML/fraud alert from Jube.
type Alert struct {
	ID          string    `json:"id"`
	RuleName    string    `json:"ruleName"`
	Severity    string    `json:"severity"` // low, medium, high, critical
	Description string    `json:"description"`
	Score       float64   `json:"score"`
	CreatedAt   time.Time `json:"createdAt"`
}

// SanctionCheckRequest contains the parameters for a sanctions screening query.
type SanctionCheckRequest struct {
	Name    string `json:"name"`
	Country string `json:"country,omitempty"`
}

// SanctionResult is the response from a sanctions screening query.
type SanctionResult struct {
	Hit     bool            `json:"hit"`
	Matches []SanctionMatch `json:"matches,omitempty"`
}

// SanctionMatch is a single match from the sanctions list.
type SanctionMatch struct {
	ListName   string  `json:"listName"`
	EntityName string  `json:"entityName"`
	Score      float64 `json:"score"`
	Country    string  `json:"country,omitempty"`
}

// CaseRequest is used to create a new compliance case.
type CaseRequest struct {
	AccountID     string `json:"accountId"`
	TransactionID string `json:"transactionId,omitempty"`
	Type          string `json:"type"` // aml, fraud, sanctions
	Severity      string `json:"severity"`
	Description   string `json:"description"`
}

// Case represents a compliance case in Jube.
type Case struct {
	ID            string    `json:"id"`
	AccountID     string    `json:"accountId"`
	TransactionID string    `json:"transactionId,omitempty"`
	Type          string    `json:"type"`
	Severity      string    `json:"severity"`
	Status        string    `json:"status"` // open, investigating, escalated, closed
	Description   string    `json:"description"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// CaseFilter contains query parameters for listing cases.
type CaseFilter struct {
	AccountID string `json:"accountId,omitempty"`
	Type      string `json:"type,omitempty"`
	Status    string `json:"status,omitempty"`
}

// SearchRequest is sent to the exhaustive search endpoint.
type SearchRequest struct {
	Query   string            `json:"query"`
	Filters map[string]string `json:"filters,omitempty"`
	Limit   int               `json:"limit,omitempty"`
}

// SearchResult is a single result from the exhaustive search.
type SearchResult struct {
	EntityID   string                 `json:"entityId"`
	EntityType string                 `json:"entityType"`
	Score      float64                `json:"score"`
	Data       map[string]interface{} `json:"data,omitempty"`
}
