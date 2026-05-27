// Offerings setup — TransactAPI Workstream 3.
//
// Endpoints:
//   POST /tapiv3/index.php/v1/createOffering   — new Offering record
//   POST /tapiv3/index.php/v1/getOffering      — read by offeringId
//   POST /tapiv3/index.php/v1/getAllOfferings  — directory listing, paginated
//
// Each Offering binds 1:1 to an ERC-3643 / T-REX SecurityToken on
// the Lux Network primary EVM (luxfi/captable). Token metadata is
// anchored at issuance; reconciliation between TransactAPI Offering
// state and on-chain token state runs continuously (luxfi/transfer).
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
	"strconv"
)

type wireOffering struct {
	OfferingID    string `json:"offeringId"`
	IssuerName    string `json:"issuerName"`
	OfferingName  string `json:"offeringName"`
	OfferingType  string `json:"offeringType"`
	TargetAmount  string `json:"targetAmount"`
	MinInvestment string `json:"minimumInvestment"`
	MaxInvestment string `json:"maximumInvestment"`
	UnitPrice     string `json:"unitPrice"`
	Currency      string `json:"currency"`
	Status        string `json:"offeringStatus"`
	OpenedAt      string `json:"openedDate"`
	ClosedAt      string `json:"closedDate"`
	SecurityClass string `json:"securityClass"`
	CUSIP         string `json:"cusip"`
	CreatedDate   string `json:"createdDate"`
	UpdatedDate   string `json:"updatedDate"`
}

// CreateOffering registers a new offering with TransactAPI.
func (p *Provider) CreateOffering(ctx context.Context, req *CreateOfferingRequest) (*Offering, error) {
	if req == nil {
		return nil, errors.New("northcapital: CreateOfferingRequest is required")
	}
	form := url.Values{}
	form.Set("issuerName", req.IssuerName)
	form.Set("offeringName", req.OfferingName)
	form.Set("offeringType", req.OfferingType)
	if req.TargetAmount != "" {
		form.Set("targetAmount", req.TargetAmount)
	}
	if req.MinInvestment != "" {
		form.Set("minimumInvestment", req.MinInvestment)
	}
	if req.MaxInvestment != "" {
		form.Set("maximumInvestment", req.MaxInvestment)
	}
	if req.UnitPrice != "" {
		form.Set("unitPrice", req.UnitPrice)
	}
	if req.Currency != "" {
		form.Set("currency", req.Currency)
	}
	if req.SecurityClass != "" {
		form.Set("securityClass", req.SecurityClass)
	}
	if req.CUSIP != "" {
		form.Set("cusip", req.CUSIP)
	}

	const path = "/tapiv3/index.php/v1/createOffering"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	return decodeOffering(env.Offering, "createOffering")
}

// GetOffering reads an Offering by TransactAPI offeringId.
func (p *Provider) GetOffering(ctx context.Context, offeringID string) (*Offering, error) {
	if offeringID == "" {
		return nil, errors.New("northcapital: offeringID required")
	}
	form := url.Values{}
	form.Set("offeringId", offeringID)
	const path = "/tapiv3/index.php/v1/getOffering"
	raw, err := p.doForm(ctx, "POST", path, form, "")
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	return decodeOffering(env.Offering, "getOffering")
}

// ListOfferings returns all Offerings accessible to this clientID.
// Pagination is handled internally.
func (p *Provider) ListOfferings(ctx context.Context) ([]*Offering, error) {
	return p.ListOfferingsPaged(ctx, ListOptions{})
}

// ListOfferingsPaged is the explicit-options variant of ListOfferings.
func (p *Provider) ListOfferingsPaged(ctx context.Context, opts ListOptions) ([]*Offering, error) {
	const path = "/tapiv3/index.php/v1/getAllOfferings"
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 500
	}
	var out []*Offering
	for page := 1; ; page++ {
		form := url.Values{}
		form.Set("offset", strconv.Itoa((page-1)*pageSize))
		form.Set("limit", strconv.Itoa(pageSize))
		for k, v := range opts.Filter {
			form.Set(k, v)
		}
		raw, err := p.doForm(ctx, "POST", path, form, "")
		if err != nil {
			return nil, err
		}
		env, err := decodeEnvelope(path, raw)
		if err != nil {
			return nil, err
		}
		if len(env.Offerings) == 0 {
			break
		}
		var arr []wireOffering
		if err := json.Unmarshal(env.Offerings, &arr); err != nil {
			return nil, fmt.Errorf("northcapital: listOfferings decode: %w", err)
		}
		if len(arr) == 0 {
			break
		}
		for i := range arr {
			out = append(out, wireOfferingToOffering(&arr[i]))
			if opts.MaxRecords > 0 && len(out) >= opts.MaxRecords {
				return out, nil
			}
		}
		if len(arr) < pageSize {
			break
		}
	}
	return out, nil
}

func decodeOffering(raw json.RawMessage, where string) (*Offering, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("northcapital: %s: empty offeringDetails", where)
	}
	var w wireOffering
	if err := json.Unmarshal(raw, &w); err != nil {
		var arr []wireOffering
		if err2 := json.Unmarshal(raw, &arr); err2 == nil && len(arr) > 0 {
			w = arr[0]
		} else {
			return nil, fmt.Errorf("northcapital: %s decode: %w", where, err)
		}
	}
	return wireOfferingToOffering(&w), nil
}

func wireOfferingToOffering(w *wireOffering) *Offering {
	o := &Offering{
		ID:            w.OfferingID,
		IssuerName:    w.IssuerName,
		OfferingName:  w.OfferingName,
		OfferingType:  w.OfferingType,
		TargetAmount:  w.TargetAmount,
		MinInvestment: w.MinInvestment,
		MaxInvestment: w.MaxInvestment,
		UnitPrice:     w.UnitPrice,
		Currency:      w.Currency,
		Status:        w.Status,
		SecurityClass: w.SecurityClass,
		CUSIP:         w.CUSIP,
	}
	o.CreatedAt = parseDate(w.CreatedDate)
	o.UpdatedAt = parseDate(w.UpdatedDate)
	if w.OpenedAt != "" {
		t := parseDate(w.OpenedAt)
		o.OpenedAt = &t
	}
	if w.ClosedAt != "" {
		t := parseDate(w.ClosedAt)
		o.ClosedAt = &t
	}
	return o
}
