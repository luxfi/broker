package alpaca

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/luxfi/broker/pkg/types"
)

// --- AccountManager implementation ---

func (p *Provider) UpdateAccount(ctx context.Context, accountID string, req *types.UpdateAccountRequest) (*types.Account, error) {
	body := make(map[string]interface{})
	if req.Contact != nil {
		contact := make(map[string]interface{})
		if req.Contact.Email != "" {
			contact["email_address"] = req.Contact.Email
		}
		if req.Contact.Phone != "" {
			contact["phone_number"] = req.Contact.Phone
		}
		if len(req.Contact.Street) > 0 {
			contact["street_address"] = req.Contact.Street
		}
		if req.Contact.City != "" {
			contact["city"] = req.Contact.City
		}
		if req.Contact.State != "" {
			contact["state"] = req.Contact.State
		}
		if req.Contact.PostalCode != "" {
			contact["postal_code"] = req.Contact.PostalCode
		}
		if req.Contact.Country != "" {
			contact["country"] = req.Contact.Country
		}
		body["contact"] = contact
	}
	if req.Identity != nil {
		identity := make(map[string]interface{})
		if req.Identity.GivenName != "" {
			identity["given_name"] = req.Identity.GivenName
		}
		if req.Identity.FamilyName != "" {
			identity["family_name"] = req.Identity.FamilyName
		}
		if req.Identity.DateOfBirth != "" {
			identity["date_of_birth"] = req.Identity.DateOfBirth
		}
		body["identity"] = identity
	}
	if len(req.EnabledAssets) > 0 {
		body["enabled_assets"] = req.EnabledAssets
	}

	data, _, err := p.do(ctx, http.MethodPatch, "/v1/accounts/"+accountID, body)
	if err != nil {
		return nil, err
	}
	raw, err := decode[alpacaAccount](data)
	if err != nil {
		return nil, err
	}
	return raw.toUnified(), nil
}

func (p *Provider) CloseAccount(ctx context.Context, accountID string) error {
	_, _, err := p.do(ctx, http.MethodDelete, "/v1/accounts/"+accountID, nil)
	return err
}

func (p *Provider) GetAccountActivities(ctx context.Context, accountID string, params *types.ActivityParams) ([]*types.Activity, error) {
	path := "/v1/accounts/" + accountID + "/activities"
	sep := "?"
	if params != nil {
		if len(params.ActivityTypes) > 0 {
			path += sep + "activity_type=" + strings.Join(params.ActivityTypes, ",")
			sep = "&"
		}
		if params.Date != "" {
			path += sep + "date=" + params.Date
			sep = "&"
		}
		if params.After != "" {
			path += sep + "after=" + params.After
			sep = "&"
		}
		if params.Until != "" {
			path += sep + "until=" + params.Until
			sep = "&"
		}
		if params.Direction != "" {
			path += sep + "direction=" + params.Direction
			sep = "&"
		}
		if params.PageSize > 0 {
			path += sep + "page_size=" + fmt.Sprintf("%d", params.PageSize)
			sep = "&"
		}
		if params.PageToken != "" {
			path += sep + "page_token=" + params.PageToken
		}
	}

	data, _, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var raw []struct {
		ID              string `json:"id"`
		AccountID       string `json:"account_id"`
		ActivityType    string `json:"activity_type"`
		Symbol          string `json:"symbol"`
		Side            string `json:"side"`
		Qty             string `json:"qty"`
		Price           string `json:"price"`
		CumQty          string `json:"cum_qty"`
		LeavesQty       string `json:"leaves_qty"`
		NetAmount       string `json:"net_amount"`
		PerShareAmount  string `json:"per_share_amount"`
		Description     string `json:"description"`
		Status          string `json:"status"`
		TransactionTime string `json:"transaction_time"`
		Date            string `json:"date"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	activities := make([]*types.Activity, len(raw))
	for i, r := range raw {
		activities[i] = &types.Activity{
			ID:              r.ID,
			AccountID:       r.AccountID,
			ActivityType:    r.ActivityType,
			Symbol:          r.Symbol,
			Side:            r.Side,
			Qty:             r.Qty,
			Price:           r.Price,
			CumQty:          r.CumQty,
			LeavesQty:       r.LeavesQty,
			NetAmount:       r.NetAmount,
			PerShareAmount:  r.PerShareAmount,
			Description:     r.Description,
			Status:          r.Status,
			TransactionTime: r.TransactionTime,
			Date:            r.Date,
		}
	}
	return activities, nil
}
