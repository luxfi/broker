package alpaca

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/luxfi/broker/pkg/types"
)

// ACATSDisclosure is the full ACATS disclosure text that must be presented to
// and accepted by the user before initiating an ACATS transfer.
const ACATSDisclosure = `Unless otherwise indicated in the instruction above, please transfer in-kind, all assets into my account with Alpaca Securities LLC. I understand that to the extent any assets in my account are not readily transferable or without penalties such assets may not be transferred within the timeframes required by applicable regulations. I understand I will be contracted by the delivering and/or receiving firm regarding any assets that are not transferable. I authorize you to liquidate any non-transferable proprietary money market fund assets that are part of my account and transfer the resulting credit balance to Alpaca Securities LLC. I authorize the transferor to deduct any outstanding fees due you from the credit balance in my account. If my account does not contain a credit balance, or if the credit balance in the account is insufficient to satisfy any outstanding fees due you, I authorize you to liquidate the assets in my account to the extent necessary to satisfy that obligation. I understand that upon receiving a copy of this transfer instruction, for a full account transfer, transferor will freeze my account and cancel all open orders for my account on your books. I affirm that have destroyed or returned to the transferor all credit/debit cards and or unused checks issued to me in connection with my account.`

// --- ACATSManager implementation ---

// ValidateACATSAccount checks that the account exists and is ACTIVE (KYC
// approved). Only ACTIVE accounts may initiate ACATS transfers.
func (p *Provider) ValidateACATSAccount(ctx context.Context, accountID string) error {
	acct, err := p.GetAccount(ctx, accountID)
	if err != nil {
		return fmt.Errorf("validate ACATS account: %w", err)
	}
	status := strings.ToUpper(acct.Status)
	if status != "ACTIVE" && status != "APPROVED" {
		return fmt.Errorf("account %s not eligible for ACATS: status is %s, must be ACTIVE or APPROVED", accountID, acct.Status)
	}
	return nil
}

// ValidateACATSAssets rejects ineligible asset classes and fractional shares.
// ACATS does not support crypto, fixed income, or options. All quantities must
// be whole shares.
func ValidateACATSAssets(assets []ACATSAsset) error {
	if len(assets) == 0 {
		return fmt.Errorf("partial ACATS transfer requires at least one asset")
	}
	for _, a := range assets {
		sym := strings.ToUpper(a.Symbol)

		// Crypto symbols contain "/" (e.g. BTC/USD, ETH/USD).
		if strings.Contains(sym, "/") {
			return fmt.Errorf("asset %s: crypto is not eligible for ACATS transfer", a.Symbol)
		}

		// Options use OCC symbology (21-char identifiers, e.g. AAPL260418C00150000).
		if len(sym) > 15 {
			return fmt.Errorf("asset %s: options contracts are not eligible for ACATS transfer", a.Symbol)
		}

		// Validate qty is a positive whole number.
		qty, err := strconv.ParseFloat(a.Qty, 64)
		if err != nil || qty <= 0 {
			return fmt.Errorf("asset %s: qty must be a positive number, got %q", a.Symbol, a.Qty)
		}
		if math.Floor(qty) != qty {
			return fmt.Errorf("asset %s: ACATS requires whole shares, got %s", a.Symbol, a.Qty)
		}
	}
	return nil
}

// ACATSAsset is an alias for the types package ACATS asset.
type ACATSAsset = types.ACATSAsset

// CreateACATSTransfer initiates an incoming ACATS transfer from a contra
// broker. Validates account eligibility and asset constraints before submitting.
func (p *Provider) CreateACATSTransfer(ctx context.Context, accountID string, req *types.CreateACATSTransferRequest) (*types.ACATSTransfer, error) {
	if err := p.ValidateACATSAccount(ctx, accountID); err != nil {
		return nil, err
	}
	if req.Type == "PARTIAL" {
		if err := ValidateACATSAssets(req.Assets); err != nil {
			return nil, err
		}
	}

	body := map[string]interface{}{
		"type":                  "ACATS",
		"direction":             "INCOMING",
		"contra_account_number": req.ContraAccount,
		"contra_broker_number":  req.ContraBroker,
		"transfer_type":         req.Type,
	}
	if len(req.Assets) > 0 {
		assets := make([]map[string]string, len(req.Assets))
		for i, a := range req.Assets {
			assets[i] = map[string]string{
				"symbol": a.Symbol,
				"qty":    a.Qty,
			}
		}
		body["assets"] = assets
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts/"+accountID+"/transfers", body)
	if err != nil {
		return nil, err
	}
	return parseACATSTransfer(data)
}

// GetACATSTransfer retrieves a single ACATS transfer by ID. Returns the full
// transfer including status and reject_reason for rejection notification.
func (p *Provider) GetACATSTransfer(ctx context.Context, accountID, transferID string) (*types.ACATSTransfer, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+accountID+"/transfers/"+transferID, nil)
	if err != nil {
		return nil, err
	}
	return parseACATSTransfer(data)
}

// GetACATSRejection retrieves a rejected ACATS transfer and returns the
// rejection reason. Returns an error if the transfer is not in REJECTED status.
// The caller uses the reason to notify the user and allow resubmission.
func (p *Provider) GetACATSRejection(ctx context.Context, accountID, transferID string) (string, error) {
	t, err := p.GetACATSTransfer(ctx, accountID, transferID)
	if err != nil {
		return "", err
	}
	if t.Status != "REJECTED" {
		return "", fmt.Errorf("transfer %s is not rejected, status is %s", transferID, t.Status)
	}
	return t.RejectReason, nil
}

// ListACATSTransfers returns all ACATS transfers for an account, covering both
// ACATC (incoming credit) and ACATS (outgoing send) activity types.
func (p *Provider) ListACATSTransfers(ctx context.Context, accountID string) ([]*types.ACATSTransfer, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+accountID+"/transfers?type=ACATS", nil)
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	transfers := make([]*types.ACATSTransfer, 0, len(raw))
	for _, r := range raw {
		t, err := parseACATSTransfer(r)
		if err != nil {
			continue
		}
		transfers = append(transfers, t)
	}
	return transfers, nil
}

// ListACATSActivities returns account activities of type ACATC and ACATS,
// which represent ACATS credit and send events respectively.
func (p *Provider) ListACATSActivities(ctx context.Context, accountID string) ([]*types.Activity, error) {
	path := "/v1/accounts/" + accountID + "/activities?activity_type=ACATC,ACATS"
	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	activities := make([]*types.Activity, 0, len(raw))
	for _, r := range raw {
		var a types.Activity
		if json.Unmarshal(r, &a) != nil {
			continue
		}
		activities = append(activities, &a)
	}
	return activities, nil
}

// CancelACATSTransfer cancels a pending ACATS transfer.
func (p *Provider) CancelACATSTransfer(ctx context.Context, accountID, transferID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/accounts/"+accountID+"/transfers/"+transferID, nil)
	return err
}

func parseACATSTransfer(data []byte) (*types.ACATSTransfer, error) {
	var raw struct {
		ID            string `json:"id"`
		AccountID     string `json:"account_id"`
		Direction     string `json:"direction"`
		Status        string `json:"status"`
		ContraAccount string `json:"contra_account_number"`
		ContraBroker  string `json:"contra_broker_number"`
		Type          string `json:"type"`
		TransferType  string `json:"transfer_type"`
		Assets        []struct {
			Symbol string `json:"symbol"`
			Qty    string `json:"qty"`
			Status string `json:"status"`
		} `json:"assets"`
		RejectReason string `json:"reject_reason"`
		CreatedAt    string `json:"created_at"`
		UpdatedAt    string `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	t := &types.ACATSTransfer{
		ID:            raw.ID,
		AccountID:     raw.AccountID,
		Direction:     raw.Direction,
		Status:        raw.Status,
		ContraAccount: raw.ContraAccount,
		ContraBroker:  raw.ContraBroker,
		Type:          raw.Type,
		RejectReason:  raw.RejectReason,
		CreatedAt:     raw.CreatedAt,
		UpdatedAt:     raw.UpdatedAt,
	}
	// Alpaca may return the ACATS subtype as "transfer_type" instead of "type".
	if t.Type == "" && raw.TransferType != "" {
		t.Type = raw.TransferType
	}

	if len(raw.Assets) > 0 {
		t.Assets = make([]types.ACATSAsset, len(raw.Assets))
		for i, a := range raw.Assets {
			t.Assets[i] = types.ACATSAsset{
				Symbol: a.Symbol,
				Qty:    a.Qty,
				Status: a.Status,
			}
		}
	}
	return t, nil
}
