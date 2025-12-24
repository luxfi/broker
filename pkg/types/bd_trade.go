package types

import "time"

// CrossBDTradeWebhook is the v2 cross-broker-dealer trade webhook payload.
// Sent to buyer BD, seller BD, and transfer agent when a trade executes.
type CrossBDTradeWebhook struct {
	WebhookEvent WebhookEvent       `json:"webhook_event"`
	Transaction  CrossBDTransaction `json:"transaction"`
	Buyer        CrossBDParty       `json:"buyer"`
	Seller       CrossBDParty       `json:"seller"`
	TransferAgent TransferAgentInfo `json:"transfer_agent"`
}

type WebhookEvent struct {
	EventID                 string            `json:"event_id"`
	EventType               string            `json:"event_type"`
	Endpoint                string            `json:"endpoint"`
	Timestamp               time.Time         `json:"timestamp"`
	Version                 string            `json:"version"`
	TransactionType         string            `json:"transaction_type"`
	BlockchainTransactionID string            `json:"blockchain_transaction_id"`
	Recipients              []WebhookRecipient `json:"recipients"`
}

type WebhookRecipient struct {
	RecipientID    string    `json:"recipient_id"`
	Name           string    `json:"name"`
	Role           string    `json:"role"` // buyer_broker_dealer, seller_broker_dealer, transfer_agent
	DeliveredAt    time.Time `json:"delivered_at"`
	DeliveryStatus string    `json:"delivery_status"`
}

type CrossBDTransaction struct {
	TransactionID   string              `json:"transaction_id"`
	TransactionType string              `json:"transaction_type"` // secondary_market_transfer, return_of_capital
	Status          string              `json:"status"`           // pending_compliance_clearance, cleared, settled
	InitiatedAt     time.Time           `json:"initiated_at"`
	Description     string              `json:"description"`
	Security        CrossBDSecurity     `json:"security"`
	Settlement      CrossBDSettlement   `json:"settlement"`
	Restrictions    CrossBDRestrictions `json:"restrictions"`
}

type CrossBDSecurity struct {
	AssetID                string              `json:"asset_id"`
	AssetName              string              `json:"asset_name"`
	AssetType              string              `json:"asset_type"`  // private_security
	SecurityClass          string              `json:"security_class"`
	ShareClass             string              `json:"share_class"`
	CUSIP                  *string             `json:"cusip"`
	ISIN                   *string             `json:"isin"`
	IssuerID               string              `json:"issuer_id"`
	IssuerName             string              `json:"issuer_name"`
	IssuerType             string              `json:"issuer_type"` // Private Corporation, LLC, Trust, Partnership, SPV, Public Company, Sole Proprietorship
	NumberOfShares         float64             `json:"number_of_shares"`
	PricePerShare          float64             `json:"price_per_share"`
	Currency               string              `json:"currency"`
	GrossTradeAmount       float64             `json:"gross_trade_amount"`
	AccruedInterest        float64             `json:"accrued_interest"`
	Commissions            CrossBDCommissions  `json:"commissions"`
	NetTradeAmount         float64             `json:"net_trade_amount"`
	TradeExecutionDatetime time.Time           `json:"trade_execution_datetime"`
	PriceDeterminationMethod string            `json:"price_determination_method"` // negotiated, auction, market
	BidPrice               float64             `json:"bid_price"`
	AskPrice               float64             `json:"ask_price"`
	LastValuationPrice     float64             `json:"last_valuation_price"`
	LastValuationDate      string              `json:"last_valuation_date"`
}

type CrossBDCommissions struct {
	BuyerBrokerDealer  BDCommission `json:"buyer_broker_dealer"`
	SellerBrokerDealer BDCommission `json:"seller_broker_dealer"`
	TotalCommissions   float64      `json:"total_commissions"`
}

type BDCommission struct {
	FirmName         string   `json:"firm_name"`
	CRDNumber        string   `json:"crd_number"`
	CommissionType   string   `json:"commission_type"` // flat_fee, percentage
	CommissionRate   *float64 `json:"commission_rate"`
	CommissionAmount float64  `json:"commission_amount"`
	Currency         string   `json:"currency"`
}

type CrossBDSettlement struct {
	SettlementDate     time.Time `json:"settlement_date"`
	SettlementType     string    `json:"settlement_type"` // bilateral, dvp, free_delivery
	SettlementStatus   string    `json:"settlement_status"`
	SettlementCurrency string    `json:"settlement_currency"`
}

type CrossBDRestrictions struct {
	LegendRequired         bool    `json:"legend_required"`
	Rule144HoldingPeriodMet bool   `json:"rule_144_holding_period_met"`
	TransferRestrictions   string  `json:"transfer_restrictions"`
	LockUpExpiryDate       *string `json:"lock_up_expiry_date"`
}

type CrossBDParty struct {
	InvestorID    string         `json:"investor_id"`
	AccountID     string         `json:"account_id"`
	AccountType   string         `json:"account_type"` // entity, individual
	BrokerDealer  BrokerDealerRef `json:"broker_dealer"`
	ComplianceRef ComplianceRef  `json:"compliance_ref"`
}

type BrokerDealerRef struct {
	FirmName    string `json:"firm_name"`
	CRDNumber   string `json:"crd_number"`
	FINRAMember bool   `json:"finra_member"`
}

type ComplianceRef struct {
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
}

type TransferAgentInfo struct {
	FirmName              string                `json:"firm_name"`
	SECRegistered         bool                  `json:"sec_registered"`
	SECRegistrationNumber string                `json:"sec_registration_number"`
	Acknowledgment        TAAgentAcknowledgment `json:"acknowledgment"`
}

type TAAgentAcknowledgment struct {
	Acknowledged          bool      `json:"acknowledged"`
	AcknowledgedAt        time.Time `json:"acknowledged_at"`
	TransferInstructionID string    `json:"transfer_instruction_id"`
	RecordDate            time.Time `json:"record_date"`
	UnitsToTransfer       float64   `json:"units_to_transfer"`
	Status                string    `json:"status"` // pending_transfer, completed, rejected
}
