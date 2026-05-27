// Party / Entity onboarding — TransactAPI Workstream 1.
//
// Endpoints (per the integration brief §1 row 1):
//   POST /tapiv3/index.php/v1/createParty    — natural-person / joint onboarding
//   POST /tapiv3/index.php/v1/createEntity   — legal-entity onboarding
//   GET  /tapiv3/index.php/v1/getParty       — read by partyId
//   GET  /tapiv3/index.php/v1/getAllParties  — directory listing, capped at 500/page
//
// All four use form-encoded bodies with clientID + developerAPIKey
// injected by the form transport. Response envelopes are decoded by
// the local helpers and projected into the package-local Party /
// Entity types so downstream consumers stay independent of the wire
// shape.
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
	"time"
)

// Wire shapes for the form-encoded party endpoints. These mirror the
// documented field names exactly so the form encoding round-trips
// without translation tables in the call path.

type wireParty struct {
	PartyID         string `json:"partyId"`
	FirstName       string `json:"firstName"`
	LastName        string `json:"lastName"`
	EmailAddress    string `json:"emailAddress"`
	Phone           string `json:"phone"`
	DOB             string `json:"dob"`
	SocialSecurityNumber string `json:"socialSecurityNumber"`
	TaxIDNumber     string `json:"taxIdNumber"`
	Domicile        string `json:"domicile"`
	Citizenship     string `json:"citizenship"`
	PrimAddress1    string `json:"primAddress1"`
	PrimAddress2    string `json:"primAddress2"`
	PrimCity        string `json:"primCity"`
	PrimState       string `json:"primState"`
	PrimZip         string `json:"primZip"`
	PrimCountry     string `json:"primCountry"`
	KYCStatus       string `json:"kycStatus"`
	AMLStatus       string `json:"amlStatus"`
	CreatedDate     string `json:"createdDate"`
	UpdatedDate     string `json:"updatedDate"`
}

type wireEntity struct {
	EntityID         string `json:"entityId"`
	EntityName       string `json:"entityName"`
	EntityType       string `json:"entityType"`
	DomicileCountry  string `json:"domicileCountry"`
	DomicileState    string `json:"domicileState"`
	EIN              string `json:"ein"`
	PrimAddress1     string `json:"primAddress1"`
	PrimAddress2     string `json:"primAddress2"`
	PrimCity         string `json:"primCity"`
	PrimState        string `json:"primState"`
	PrimZip          string `json:"primZip"`
	PrimCountry      string `json:"primCountry"`
	KYCStatus        string `json:"kycStatus"`
	AMLStatus        string `json:"amlStatus"`
	CreatedDate      string `json:"createdDate"`
	UpdatedDate      string `json:"updatedDate"`
}

// envelope is the TransactAPI standard reply wrapper. statusCode "101"
// is success; everything else is an application-level error surfaced
// as APIError via decodeEnvelope.
type envelope struct {
	StatusCode    string          `json:"statusCode"`
	StatusDesc    string          `json:"statusDesc"`
	PartyDetails  json.RawMessage `json:"partyDetails"`
	EntityDetails json.RawMessage `json:"entityDetails"`
	Offering      json.RawMessage `json:"offeringDetails"`
	Offerings     json.RawMessage `json:"offeringsDetails"`
	Trade         json.RawMessage `json:"tradeDetails"`
	Trades        json.RawMessage `json:"tradesDetails"`
	Custody       json.RawMessage `json:"custodyAccountDetails"`
	Secondary     json.RawMessage `json:"secondaryTradesDetails"`
	Documents     json.RawMessage `json:"documentDetails"`
	KYC           json.RawMessage `json:"kycDetails"`
	AML           json.RawMessage `json:"amlDetails"`
	Accredited    json.RawMessage `json:"accreditedDetails"`
	Suitability   json.RawMessage `json:"suitabilityDetails"`
	BulkResult    json.RawMessage `json:"bulkResult"`
	ATSResult     json.RawMessage `json:"atsResult"`
	Refund        json.RawMessage `json:"refundDetails"`
	WebhookKey    json.RawMessage `json:"webhookKeyDetails"`
}

// decodeEnvelope unmarshals raw bytes and verifies the TransactAPI
// status code. Returns the envelope (so the caller can pull the
// type-specific raw segment) or a wrapped APIError on failure.
func decodeEnvelope(path string, raw []byte) (*envelope, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("northcapital: decode envelope %s: %w", path, err)
	}
	if env.StatusCode != "" && env.StatusCode != "101" {
		return nil, &APIError{
			StatusCode: 400,
			Endpoint:   path,
			Body:       fmt.Sprintf("statusCode=%s statusDesc=%s", env.StatusCode, env.StatusDesc),
		}
	}
	return &env, nil
}

// CreateParty onboards a natural-person (or joint) investor.
func (p *Provider) CreateParty(ctx context.Context, req *CreatePartyRequest) (*Party, error) {
	if req == nil {
		return nil, errors.New("northcapital: CreatePartyRequest is required")
	}
	form := url.Values{}
	form.Set("firstName", req.GivenName)
	form.Set("lastName", req.FamilyName)
	form.Set("emailAddress", req.Email)
	if req.Phone != "" {
		form.Set("phone", req.Phone)
	}
	if req.DateOfBirth != "" {
		form.Set("dob", req.DateOfBirth)
	}
	if req.TaxID != "" {
		switch req.TaxIDType {
		case "ssn":
			form.Set("socialSecurityNumber", req.TaxID)
		default:
			form.Set("taxIdNumber", req.TaxID)
		}
	}
	if req.Domicile != "" {
		form.Set("domicile", req.Domicile)
	}
	if req.Citizenship != "" {
		form.Set("citizenship", req.Citizenship)
	}
	if req.Address != nil {
		form.Set("primAddress1", req.Address.Street1)
		if req.Address.Street2 != "" {
			form.Set("primAddress2", req.Address.Street2)
		}
		form.Set("primCity", req.Address.City)
		form.Set("primState", req.Address.State)
		form.Set("primZip", req.Address.PostalCode)
		form.Set("primCountry", req.Address.Country)
	}

	const path = "/tapiv3/index.php/v1/createParty"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.PartyDetails) == 0 {
		return nil, fmt.Errorf("northcapital: createParty: empty partyDetails")
	}
	// partyDetails is sometimes a single object, sometimes a single-element array
	// depending on the legacy endpoint. Try array first.
	var arr []wireParty
	if err := json.Unmarshal(env.PartyDetails, &arr); err == nil && len(arr) > 0 {
		return wirePartyToParty(req.Type, &arr[0]), nil
	}
	var one wireParty
	if err := json.Unmarshal(env.PartyDetails, &one); err != nil {
		return nil, fmt.Errorf("northcapital: createParty: decode party: %w", err)
	}
	return wirePartyToParty(req.Type, &one), nil
}

// CreateEntity onboards a legal-entity (LLC / corp / trust / partnership /
// SPV) investor. Requires beneficial-owner + control-person partyIds.
func (p *Provider) CreateEntity(ctx context.Context, req *CreateEntityRequest) (*Entity, error) {
	if req == nil {
		return nil, errors.New("northcapital: CreateEntityRequest is required")
	}
	form := url.Values{}
	form.Set("entityName", req.LegalName)
	form.Set("entityType", req.EntityType)
	form.Set("domicileCountry", req.FormationCountry)
	if req.FormationState != "" {
		form.Set("domicileState", req.FormationState)
	}
	if req.EIN != "" {
		form.Set("ein", req.EIN)
	}
	if req.Address != nil {
		form.Set("primAddress1", req.Address.Street1)
		if req.Address.Street2 != "" {
			form.Set("primAddress2", req.Address.Street2)
		}
		form.Set("primCity", req.Address.City)
		form.Set("primState", req.Address.State)
		form.Set("primZip", req.Address.PostalCode)
		form.Set("primCountry", req.Address.Country)
	}
	for i, bo := range req.Beneficials {
		form.Set("beneficialOwner"+strconv.Itoa(i+1), bo)
	}
	for i, cp := range req.ControlPersons {
		form.Set("controlPerson"+strconv.Itoa(i+1), cp)
	}

	const path = "/tapiv3/index.php/v1/createEntity"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.EntityDetails) == 0 {
		return nil, fmt.Errorf("northcapital: createEntity: empty entityDetails")
	}
	var arr []wireEntity
	if err := json.Unmarshal(env.EntityDetails, &arr); err == nil && len(arr) > 0 {
		return wireEntityToEntity(&arr[0], req.Beneficials, req.ControlPersons), nil
	}
	var one wireEntity
	if err := json.Unmarshal(env.EntityDetails, &one); err != nil {
		return nil, fmt.Errorf("northcapital: createEntity: decode entity: %w", err)
	}
	return wireEntityToEntity(&one, req.Beneficials, req.ControlPersons), nil
}

// GetParty reads a Party by TransactAPI partyId.
func (p *Provider) GetParty(ctx context.Context, partyID string) (*Party, error) {
	if partyID == "" {
		return nil, errors.New("northcapital: partyID required")
	}
	form := url.Values{}
	form.Set("partyId", partyID)
	const path = "/tapiv3/index.php/v1/getParty"
	raw, err := p.doForm(ctx, "POST", path, form, "")
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.PartyDetails) == 0 {
		return nil, fmt.Errorf("northcapital: getParty: not found")
	}
	var arr []wireParty
	if err := json.Unmarshal(env.PartyDetails, &arr); err == nil && len(arr) > 0 {
		return wirePartyToParty(PartyIndividual, &arr[0]), nil
	}
	var one wireParty
	if err := json.Unmarshal(env.PartyDetails, &one); err != nil {
		return nil, fmt.Errorf("northcapital: getParty: decode: %w", err)
	}
	return wirePartyToParty(PartyIndividual, &one), nil
}

// ListParties returns all Parties accessible to this clientID. The
// adapter handles pagination internally per the documented
// "getAllParties" cap (500 records/page since the Jan 13 change). No
// caller writes pagination logic.
func (p *Provider) ListParties(ctx context.Context) ([]*Party, error) {
	return p.ListPartiesPaged(ctx, ListOptions{})
}

// ListPartiesPaged is the explicit-option variant of ListParties.
func (p *Provider) ListPartiesPaged(ctx context.Context, opts ListOptions) ([]*Party, error) {
	const path = "/tapiv3/index.php/v1/getAllParties"
	pageSize := opts.PageSize
	if pageSize == 0 {
		pageSize = 500 // upstream cap for legacy "get all" endpoints
	}

	var out []*Party
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
		if len(env.PartyDetails) == 0 {
			break
		}
		var arr []wireParty
		if err := json.Unmarshal(env.PartyDetails, &arr); err != nil {
			return nil, fmt.Errorf("northcapital: listParties: decode page %d: %w", page, err)
		}
		if len(arr) == 0 {
			break
		}
		for i := range arr {
			out = append(out, wirePartyToParty(PartyIndividual, &arr[i]))
			if opts.MaxRecords > 0 && len(out) >= opts.MaxRecords {
				return out, nil
			}
		}
		if len(arr) < pageSize {
			break // short page → no more results
		}
	}
	return out, nil
}

// --- wire ↔ domain projections ---

func wirePartyToParty(typ PartyType, w *wireParty) *Party {
	p := &Party{
		ID:          w.PartyID,
		Type:        typ,
		GivenName:   w.FirstName,
		FamilyName:  w.LastName,
		Email:       w.EmailAddress,
		Phone:       w.Phone,
		DateOfBirth: w.DOB,
		Domicile:    w.Domicile,
		Citizenship: w.Citizenship,
		KYCStatus:   w.KYCStatus,
		AMLStatus:   w.AMLStatus,
	}
	if w.SocialSecurityNumber != "" {
		p.TaxIDType = "ssn"
		p.TaxIDLast4 = lastN(w.SocialSecurityNumber, 4)
	} else if w.TaxIDNumber != "" {
		p.TaxIDType = "ein"
		p.TaxIDLast4 = lastN(w.TaxIDNumber, 4)
	}
	if w.PrimAddress1 != "" || w.PrimCity != "" {
		p.Address = &PartyAddress{
			Street1:    w.PrimAddress1,
			Street2:    w.PrimAddress2,
			City:       w.PrimCity,
			State:      w.PrimState,
			PostalCode: w.PrimZip,
			Country:    w.PrimCountry,
		}
	}
	p.CreatedAt = parseDate(w.CreatedDate)
	p.UpdatedAt = parseDate(w.UpdatedDate)
	return p
}

func wireEntityToEntity(w *wireEntity, beneficials, controls []string) *Entity {
	e := &Entity{
		ID:               w.EntityID,
		LegalName:        w.EntityName,
		EntityType:       w.EntityType,
		FormationCountry: w.DomicileCountry,
		FormationState:   w.DomicileState,
		EIN:              w.EIN,
		KYCStatus:        w.KYCStatus,
		AMLStatus:        w.AMLStatus,
		Beneficials:      beneficials,
		ControlPersons:   controls,
	}
	if w.PrimAddress1 != "" || w.PrimCity != "" {
		e.Address = &PartyAddress{
			Street1:    w.PrimAddress1,
			Street2:    w.PrimAddress2,
			City:       w.PrimCity,
			State:      w.PrimState,
			PostalCode: w.PrimZip,
			Country:    w.PrimCountry,
		}
	}
	e.CreatedAt = parseDate(w.CreatedDate)
	e.UpdatedAt = parseDate(w.UpdatedDate)
	return e
}

// parseDate accepts both RFC3339 and TransactAPI's MM/DD/YYYY form.
func parseDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "01/02/2006", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
