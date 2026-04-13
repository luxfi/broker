package alpaca_omnisub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/luxfi/broker/pkg/types"
)

type alpacaAccount struct {
	ID            string `json:"id"`
	AccountNumber string `json:"account_number"`
	Status        string `json:"status"`
	Currency      string `json:"currency"`
	AccountType   string `json:"account_type"`
	CreatedAt     string `json:"created_at"`
	Contact       *struct {
		EmailAddress  string   `json:"email_address"`
		PhoneNumber   string   `json:"phone_number"`
		StreetAddress []string `json:"street_address"`
		City          string   `json:"city"`
		State         string   `json:"state"`
		PostalCode    string   `json:"postal_code"`
		Country       string   `json:"country"`
	} `json:"contact"`
	Identity *struct {
		GivenName             string `json:"given_name"`
		FamilyName            string `json:"family_name"`
		DateOfBirth           string `json:"date_of_birth"`
		TaxIDType             string `json:"tax_id_type"`
		CountryOfTaxResidence string `json:"country_of_tax_residence"`
	} `json:"identity"`
	EnabledAssets []string `json:"enabled_assets"`
}

func (a *alpacaAccount) toUnified() *types.Account {
	acct := &types.Account{
		ID:            a.ID,
		Provider:      "alpaca_omnisub",
		ProviderID:    a.ID,
		AccountNumber: a.AccountNumber,
		Status:        a.Status,
		Currency:      a.Currency,
		AccountType:   a.AccountType,
		EnabledAssets: a.EnabledAssets,
	}
	if t, err := time.Parse(time.RFC3339Nano, a.CreatedAt); err == nil {
		acct.CreatedAt = t
	}
	if a.Identity != nil {
		acct.Identity = &types.Identity{
			GivenName:    a.Identity.GivenName,
			FamilyName:   a.Identity.FamilyName,
			DateOfBirth:  a.Identity.DateOfBirth,
			TaxIDType:    a.Identity.TaxIDType,
			CountryOfTax: a.Identity.CountryOfTaxResidence,
		}
	}
	if a.Contact != nil {
		acct.Contact = &types.Contact{
			Email:      a.Contact.EmailAddress,
			Phone:      a.Contact.PhoneNumber,
			Street:     a.Contact.StreetAddress,
			City:       a.Contact.City,
			State:      a.Contact.State,
			PostalCode: a.Contact.PostalCode,
			Country:    a.Contact.Country,
		}
	}
	return acct
}

// CreateAccount creates an omnibus sub-account under the configured master.
// The Alpaca API requires account_type=omnibus_sub_account and the
// omnibus_master_id field referencing the omnibus master account.
func (p *Provider) CreateAccount(ctx context.Context, req *types.CreateAccountRequest) (*types.Account, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	ipAddress := req.IPAddress
	if ipAddress == "" {
		return nil, errMissingIP
	}

	body := map[string]interface{}{
		"account_type":      "omnibus_sub_account",
		"omnibus_master_id": p.cfg.OmnibusAccountID,
		"contact": map[string]interface{}{
			"email_address":  req.Contact.Email,
			"phone_number":   req.Contact.Phone,
			"street_address": req.Contact.Street,
			"city":           req.Contact.City,
			"state":          req.Contact.State,
			"postal_code":    req.Contact.PostalCode,
			"country":        req.Contact.Country,
		},
		"identity": map[string]interface{}{
			"given_name":                req.Identity.GivenName,
			"family_name":              req.Identity.FamilyName,
			"date_of_birth":            req.Identity.DateOfBirth,
			"tax_id":                   req.Identity.TaxID,
			"tax_id_type":              req.Identity.TaxIDType,
			"country_of_tax_residence": req.Identity.CountryOfTax,
			"funding_source":           fundingSources(req.FundingSources),
		},
		"disclosures": disclosuresBody(req.Disclosures),
		"agreements": []map[string]interface{}{
			{"agreement": "margin_agreement", "signed_at": now, "ip_address": ipAddress},
			{"agreement": "account_agreement", "signed_at": now, "ip_address": ipAddress},
			{"agreement": "customer_agreement", "signed_at": now, "ip_address": ipAddress},
		},
	}
	if len(req.EnabledAssets) > 0 {
		body["enabled_assets"] = req.EnabledAssets
	}

	data, _, err := p.do(ctx, http.MethodPost, "/v1/accounts", body)
	if err != nil {
		return nil, err
	}
	var raw alpacaAccount
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw.toUnified(), nil
}

// GetAccount retrieves a sub-account by its Alpaca ID.
func (p *Provider) GetAccount(ctx context.Context, providerAccountID string) (*types.Account, error) {
	data, _, err := p.do(ctx, http.MethodGet, "/v1/accounts/"+providerAccountID, nil)
	if err != nil {
		return nil, err
	}
	var raw alpacaAccount
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw.toUnified(), nil
}

// ListAccounts lists all sub-accounts under this omnibus master.
// Alpaca filters by query=omnibus_master_id.
func (p *Provider) ListAccounts(ctx context.Context) ([]*types.Account, error) {
	path := "/v1/accounts?query=" + p.cfg.OmnibusAccountID
	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var raw []alpacaAccount
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	accts := make([]*types.Account, len(raw))
	for i := range raw {
		accts[i] = raw[i].toUnified()
	}
	return accts, nil
}

// --- helpers ---

var errMissingIP = fmt.Errorf("client IP address is required for agreement signing")

func fundingSources(src []string) []string {
	if len(src) == 0 {
		return []string{"employment_income"}
	}
	return src
}

func disclosuresBody(d *types.AccountDisclosures) map[string]interface{} {
	cp, af, pe, fe := false, false, false, false
	if d != nil {
		if d.IsControlPerson != nil {
			cp = *d.IsControlPerson
		}
		if d.IsAffiliatedExchangeFinra != nil {
			af = *d.IsAffiliatedExchangeFinra
		}
		if d.IsPoliticallyExposed != nil {
			pe = *d.IsPoliticallyExposed
		}
		if d.ImmediateFamilyExposed != nil {
			fe = *d.ImmediateFamilyExposed
		}
	}
	return map[string]interface{}{
		"is_control_person":               cp,
		"is_affiliated_exchange_or_finra": af,
		"is_politically_exposed":          pe,
		"immediate_family_exposed":        fe,
	}
}
