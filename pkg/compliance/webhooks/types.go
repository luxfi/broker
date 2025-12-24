// Package webhooks defines the cross-BD trade webhook payload types.
// These types match the JSON schemas in schemas/ for all entity types:
// corporation, LLC, trust, partnership, sole proprietorship, public company, SPV.
package webhooks

import "time"

// TradeWebhookPayload is the top-level envelope for trade.executed webhook events
// sent to cross-listed broker-dealers and transfer agents.
type TradeWebhookPayload struct {
	WebhookEvent  WebhookEvent  `json:"webhook_event"`
	Transaction   Transaction   `json:"transaction"`
	Buyer         Party         `json:"buyer"`
	Seller        Party         `json:"seller"`
	TransferAgent TransferAgent `json:"transfer_agent"`
}

// WebhookEvent is the event metadata envelope.
type WebhookEvent struct {
	EventID               string      `json:"event_id"`
	EventType             string      `json:"event_type"`
	Endpoint              string      `json:"endpoint"`
	Timestamp             time.Time   `json:"timestamp"`
	Version               string      `json:"version"`
	TransactionType       string      `json:"transaction_type"`
	BlockchainTxID        string      `json:"blockchain_transaction_id"`
	Recipients            []Recipient `json:"recipients"`
}

// Recipient is a webhook delivery target (broker-dealer or transfer agent).
type Recipient struct {
	RecipientID    string `json:"recipient_id"`
	Name           string `json:"name"`
	Role           string `json:"role"` // buyer_broker_dealer, seller_broker_dealer, transfer_agent
	DeliveredAt    string `json:"delivered_at,omitempty"`
	DeliveryStatus string `json:"delivery_status,omitempty"`
}

// Transaction describes the trade being executed.
type Transaction struct {
	TransactionID   string       `json:"transaction_id"`
	TransactionType string       `json:"transaction_type"`
	Status          string       `json:"status"`
	InitiatedAt     time.Time    `json:"initiated_at"`
	Description     string       `json:"description"`
	Security        Security     `json:"security"`
	Settlement      Settlement   `json:"settlement"`
	Restrictions    Restrictions `json:"restrictions"`
}

// Security describes the asset being traded.
type Security struct {
	AssetID                string      `json:"asset_id"`
	AssetName              string      `json:"asset_name"`
	AssetType              string      `json:"asset_type"`
	SecurityClass          string      `json:"security_class"`
	ShareClass             string      `json:"share_class"`
	CUSIP                  *string     `json:"cusip"`
	ISIN                   *string     `json:"isin"`
	IssuerID               string      `json:"issuer_id"`
	IssuerName             string      `json:"issuer_name"`
	IssuerType             string      `json:"issuer_type"`
	NumberOfShares         int         `json:"number_of_shares"`
	PricePerShare          float64     `json:"price_per_share"`
	Currency               string      `json:"currency"`
	GrossTradeAmount       float64     `json:"gross_trade_amount"`
	AccruedInterest        float64     `json:"accrued_interest"`
	Commissions            Commissions `json:"commissions"`
	NetTradeAmount         float64     `json:"net_trade_amount"`
	TradeExecutionDatetime time.Time   `json:"trade_execution_datetime"`
	PriceDeterminationMethod string   `json:"price_determination_method"`
	BidPrice               float64     `json:"bid_price"`
	AskPrice               float64     `json:"ask_price"`
	LastValuationPrice     float64     `json:"last_valuation_price"`
	LastValuationDate      string      `json:"last_valuation_date"`
}

// Commissions holds fee information for both sides of the trade.
type Commissions struct {
	BuyerBrokerDealer  BDCommission `json:"buyer_broker_dealer"`
	SellerBrokerDealer BDCommission `json:"seller_broker_dealer"`
	TotalCommissions   float64      `json:"total_commissions"`
}

// BDCommission is a single broker-dealer's commission on a trade.
type BDCommission struct {
	FirmName       string  `json:"firm_name"`
	CRDNumber      string  `json:"crd_number"`
	Commission     float64 `json:"commission"`
	CommissionType string  `json:"commission_type"`
	Currency       string  `json:"currency"`
}

// Settlement describes the trade settlement terms.
type Settlement struct {
	SettlementDate     time.Time `json:"settlement_date"`
	SettlementType     string    `json:"settlement_type"`
	SettlementStatus   string    `json:"settlement_status"`
	SettlementCurrency string    `json:"settlement_currency"`
}

// Restrictions describes transfer restrictions on the security.
type Restrictions struct {
	LegendRequired          bool    `json:"legend_required"`
	Rule144HoldingPeriodMet *bool   `json:"rule_144_holding_period_met"`
	TransferRestrictions    string  `json:"transfer_restrictions"`
	LockUpExpiryDate        *string `json:"lock_up_expiry_date"`
}

// Party represents a buyer or seller in the trade.
type Party struct {
	InvestorID    string        `json:"investor_id"`
	AccountID     string        `json:"account_id"`
	AccountType   string        `json:"account_type"`
	BrokerDealer  BrokerDealer  `json:"broker_dealer"`
	ComplianceRef ComplianceRef `json:"compliance_ref"`
}

// BrokerDealer identifies the broker-dealer for a trade party.
type BrokerDealer struct {
	FirmName    string `json:"firm_name"`
	CRDNumber   string `json:"crd_number"`
	FINRAMember bool   `json:"finra_member"`
}

// ComplianceRef is a reference to the compliance data endpoint for a party.
type ComplianceRef struct {
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
}

// TransferAgent represents the transfer agent handling the security transfer.
type TransferAgent struct {
	FirmName              string                `json:"firm_name"`
	SECRegistered         bool                  `json:"sec_registered"`
	SECRegistrationNumber string                `json:"sec_registration_number"`
	Acknowledgment        TAcknowledgment       `json:"acknowledgment"`
}

// TAcknowledgment is the transfer agent's acknowledgment of the transfer instruction.
type TAcknowledgment struct {
	Acknowledged          bool      `json:"acknowledged"`
	AcknowledgedAt        time.Time `json:"acknowledged_at"`
	TransferInstructionID string    `json:"transfer_instruction_id"`
	RecordDate            time.Time `json:"record_date"`
	UnitsToTransfer       int       `json:"units_to_transfer"`
	Status                string    `json:"status"`
}
