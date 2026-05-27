// Custody account opening — TransactAPI Workstream 6.
//
// Endpoints:
//   POST /tapiv3/index.php/v1/createCustodyAccountRequest
//   POST /tapiv3/index.php/v1/getCustodyAccountRequest
//
// The custody-account-opening workflow per the documented Custody
// Account Opening Workflow guide proceeds in three steps:
//
//  1. Investor onboards as a Party (or Entity) — CreateParty /
//     CreateEntity. KYC / AML / Accredited verification runs.
//  2. Custody-account request is submitted with the desired
//     accountType (individual, joint, entity, ira). NCPS-side
//     compliance review begins. Initial status is "pending".
//  3. NCPS compliance review completes; the account either opens
//     (status "open") or is rejected (status "rejected"). Outcome is
//     pushed via the encrypted webhook channel
//     (account.opened / account.rejected); the adapter also exposes
//     GetCustodyAccount for direct polling.
//
// Once open, the custody account is reconciled against the Lux
// captable identity registry; subsequent trades may settle into the
// custody account directly.
//
// Source-of-design: Public-Spec
// Source-ref: https://transactapi.readme.io/reference

package northcapital

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
)

type wireCustody struct {
	RequestID    string `json:"requestId"`
	AccountID    string `json:"accountId"`
	PartyID      string `json:"partyId"`
	EntityID     string `json:"entityId"`
	AccountType  string `json:"accountType"`
	Status       string `json:"status"`
	OpenedDate   string `json:"openedDate"`
	ClosedDate   string `json:"closedDate"`
}

// OpenCustodyAccount opens a custody account for the given Party (or
// Entity), of the requested account_type (individual / joint / entity / ira).
// The returned CustodyAccount carries the initial request ID; final
// open / rejected status arrives via webhook and may be polled via
// GetCustodyAccount.
func (p *Provider) OpenCustodyAccount(ctx context.Context, partyID string, req *CustodyOpenRequest) (*CustodyAccount, error) {
	if partyID == "" {
		return nil, errors.New("northcapital: partyID required")
	}
	if req == nil {
		return nil, errors.New("northcapital: CustodyOpenRequest is required")
	}
	form := url.Values{}
	form.Set("partyId", partyID)
	form.Set("accountType", req.AccountType)

	const path = "/tapiv3/index.php/v1/createCustodyAccountRequest"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	return decodeCustody(env.Custody, "openCustodyAccount")
}

// GetCustodyAccount reads the status of a previously-submitted custody
// account request. Use this to poll the request between webhook events
// or to backfill state on adapter restart.
func (p *Provider) GetCustodyAccount(ctx context.Context, requestID string) (*CustodyAccount, error) {
	if requestID == "" {
		return nil, errors.New("northcapital: requestID required")
	}
	form := url.Values{}
	form.Set("requestId", requestID)
	const path = "/tapiv3/index.php/v1/getCustodyAccountRequest"
	raw, err := p.doForm(ctx, "POST", path, form, "")
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	return decodeCustody(env.Custody, "getCustodyAccount")
}

func decodeCustody(raw json.RawMessage, where string) (*CustodyAccount, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("northcapital: %s: empty custodyAccountDetails", where)
	}
	var w wireCustody
	if err := json.Unmarshal(raw, &w); err != nil {
		var arr []wireCustody
		if err2 := json.Unmarshal(raw, &arr); err2 == nil && len(arr) > 0 {
			w = arr[0]
		} else {
			return nil, fmt.Errorf("northcapital: %s decode: %w", where, err)
		}
	}
	ca := &CustodyAccount{
		ID:          w.AccountID,
		RequestID:   w.RequestID,
		PartyID:     w.PartyID,
		EntityID:    w.EntityID,
		AccountType: w.AccountType,
		Status:      normalizeStatus(w.Status),
	}
	if w.Status == "" {
		ca.Status = ""
	}
	if w.OpenedDate != "" {
		t := parseDate(w.OpenedDate)
		ca.OpenedAt = &t
	}
	if w.ClosedDate != "" {
		t := parseDate(w.ClosedDate)
		ca.ClosedAt = &t
	}
	return ca, nil
}
