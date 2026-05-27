// KYC / AML / Accredited / Suitability verification — Workstream 2.
//
// Endpoints:
//   POST /tapiv3/index.php/v1/performKYC          — identity + document
//   POST /tapiv3/index.php/v1/performAml          — sanctions / OFAC / SDN / PEP
//   POST /tapiv3/index.php/v1/performAccredited   — Reg D 506(c) accredited
//   POST /tapiv3/index.php/v1/recordSuitability   — suitability questionnaire
//
// Lux's amld overlay (luxfi/amld) layers an independent screen on top
// of these results, per §6 of the integration brief. Where the two
// disagree, the conservative path (block + escalate) wins.
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

// wireKYC mirrors the documented kycDetails block (status / reasons /
// score), independent of the Lux-side normalization.
type wireKYC struct {
	PartyID    string   `json:"partyId"`
	Status     string   `json:"kycStatus"`
	Score      int      `json:"score"`
	Reasons    []string `json:"reasons"`
	RunAt      string   `json:"runAt"`
	ResponseID string   `json:"responseId"`
}

type wireAML struct {
	PartyID string      `json:"partyId"`
	Status  string      `json:"amlStatus"`
	Hits    []wireAMLHit `json:"hits"`
	RunAt   string      `json:"runAt"`
}

type wireAMLHit struct {
	List       string  `json:"list"`
	Name       string  `json:"name"`
	MatchScore float64 `json:"matchScore"`
	Reference  string  `json:"reference"`
}

type wireAccredited struct {
	PartyID    string `json:"partyId"`
	Method     string `json:"method"`
	Status     string `json:"status"`
	VerifiedAt string `json:"verifiedAt"`
	ExpiresAt  string `json:"expiresAt"`
}

type wireSuitability struct {
	PartyID           string   `json:"partyId"`
	Status            string   `json:"status"`
	RiskTolerance     string   `json:"riskTolerance"`
	InvestmentHorizon string   `json:"investmentHorizon"`
	NetWorth          string   `json:"netWorth"`
	AnnualIncome      string   `json:"annualIncome"`
	LiquidityNeeds    string   `json:"liquidityNeeds"`
	Reasons           []string `json:"reasons"`
	RecordedAt        string   `json:"recordedAt"`
}

// PerformKYC runs identity + document verification on a Party.
func (p *Provider) PerformKYC(ctx context.Context, partyID string) (*KYCResult, error) {
	if partyID == "" {
		return nil, errors.New("northcapital: partyID required")
	}
	form := url.Values{}
	form.Set("partyId", partyID)
	const path = "/tapiv3/index.php/v1/performKYC"
	idemKey := p.resolveIdemKey("", partyID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.KYC) == 0 {
		return nil, fmt.Errorf("northcapital: performKYC: empty kycDetails")
	}
	var w wireKYC
	if err := json.Unmarshal(env.KYC, &w); err != nil {
		// Try array form (legacy variant).
		var arr []wireKYC
		if err2 := json.Unmarshal(env.KYC, &arr); err2 == nil && len(arr) > 0 {
			w = arr[0]
		} else {
			return nil, fmt.Errorf("northcapital: performKYC decode: %w", err)
		}
	}
	return &KYCResult{
		PartyID: w.PartyID,
		Status:  normalizeStatus(w.Status),
		Score:   w.Score,
		Reasons: w.Reasons,
		RunAt:   firstParseable(w.RunAt),
	}, nil
}

// PerformAML runs sanctions / OFAC / SDN / PEP screening on a Party.
func (p *Provider) PerformAML(ctx context.Context, partyID string) (*AMLResult, error) {
	if partyID == "" {
		return nil, errors.New("northcapital: partyID required")
	}
	form := url.Values{}
	form.Set("partyId", partyID)
	const path = "/tapiv3/index.php/v1/performAml"
	idemKey := p.resolveIdemKey("", partyID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.AML) == 0 {
		return nil, fmt.Errorf("northcapital: performAML: empty amlDetails")
	}
	var w wireAML
	if err := json.Unmarshal(env.AML, &w); err != nil {
		var arr []wireAML
		if err2 := json.Unmarshal(env.AML, &arr); err2 == nil && len(arr) > 0 {
			w = arr[0]
		} else {
			return nil, fmt.Errorf("northcapital: performAML decode: %w", err)
		}
	}
	hits := make([]AMLHit, 0, len(w.Hits))
	for _, h := range w.Hits {
		hits = append(hits, AMLHit{
			List:       h.List,
			Name:       h.Name,
			MatchScore: h.MatchScore,
			Reference:  h.Reference,
		})
	}
	return &AMLResult{
		PartyID: w.PartyID,
		Status:  normalizeStatus(w.Status),
		Hits:    hits,
		RunAt:   firstParseable(w.RunAt),
	}, nil
}

// PerformAccredited runs Reg D 506(c) accredited-investor verification.
// The five documented methods (income / net-worth / third-party letter
// / professional license / joint accredited) all flow through the
// single endpoint with the appropriate supporting fields.
func (p *Provider) PerformAccredited(ctx context.Context, partyID string, method AccreditedMethod) (*AccreditedResult, error) {
	return p.PerformAccreditedFull(ctx, &AccreditedRequest{
		PartyID: partyID,
		Method:  method,
	})
}

// PerformAccreditedFull is the explicit-options variant. Use this when
// supplying documentation refs (third-party letter document ID, income
// year, etc.).
func (p *Provider) PerformAccreditedFull(ctx context.Context, req *AccreditedRequest) (*AccreditedResult, error) {
	if req == nil || req.PartyID == "" {
		return nil, errors.New("northcapital: partyID required")
	}
	form := url.Values{}
	form.Set("partyId", req.PartyID)
	form.Set("method", string(req.Method))
	if req.DocumentID != "" {
		form.Set("documentId", req.DocumentID)
	}
	if req.LicenseRef != "" {
		form.Set("licenseRef", req.LicenseRef)
	}
	if req.IncomeYear != 0 {
		form.Set("incomeYear", strconv.Itoa(req.IncomeYear))
	}
	if req.JointSpouseID != "" {
		form.Set("jointSpousePartyId", req.JointSpouseID)
	}
	if req.ThirdPartyEmail != "" {
		form.Set("thirdPartyEmail", req.ThirdPartyEmail)
	}
	const path = "/tapiv3/index.php/v1/performAccredited"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.Accredited) == 0 {
		return nil, fmt.Errorf("northcapital: performAccredited: empty accreditedDetails")
	}
	var w wireAccredited
	if err := json.Unmarshal(env.Accredited, &w); err != nil {
		var arr []wireAccredited
		if err2 := json.Unmarshal(env.Accredited, &arr); err2 == nil && len(arr) > 0 {
			w = arr[0]
		} else {
			return nil, fmt.Errorf("northcapital: performAccredited decode: %w", err)
		}
	}
	r := &AccreditedResult{
		PartyID: w.PartyID,
		Method:  AccreditedMethod(w.Method),
		Status:  normalizeStatus(w.Status),
	}
	if w.VerifiedAt != "" {
		t := firstParseable(w.VerifiedAt)
		r.VerifiedAt = &t
	}
	if w.ExpiresAt != "" {
		t := firstParseable(w.ExpiresAt)
		r.ExpiresAt = &t
	}
	return r, nil
}

// Suitability records a suitability questionnaire result for a Party.
// Suitability is a discrete workstream from KYC/AML/Accredited — it
// captures the risk-tolerance / investment-horizon / net-worth /
// liquidity-needs profile used for trade-level appropriateness checks.
func (p *Provider) Suitability(ctx context.Context, req *SuitabilityRequest) (*SuitabilityResult, error) {
	if req == nil || req.PartyID == "" {
		return nil, errors.New("northcapital: partyID required")
	}
	form := url.Values{}
	form.Set("partyId", req.PartyID)
	if req.RiskTolerance != "" {
		form.Set("riskTolerance", req.RiskTolerance)
	}
	if req.InvestmentHorizon != "" {
		form.Set("investmentHorizon", req.InvestmentHorizon)
	}
	if req.NetWorth != "" {
		form.Set("netWorth", req.NetWorth)
	}
	if req.AnnualIncome != "" {
		form.Set("annualIncome", req.AnnualIncome)
	}
	if req.LiquidityNeeds != "" {
		form.Set("liquidityNeeds", req.LiquidityNeeds)
	}
	const path = "/tapiv3/index.php/v1/recordSuitability"
	idemKey := p.resolveIdemKey(req.IdempotencyKey, req.OrgID, path, canonicalForm(form))
	raw, err := p.doForm(ctx, "POST", path, form, idemKey)
	if err != nil {
		return nil, err
	}
	env, err := decodeEnvelope(path, raw)
	if err != nil {
		return nil, err
	}
	if len(env.Suitability) == 0 {
		return nil, fmt.Errorf("northcapital: suitability: empty suitabilityDetails")
	}
	var w wireSuitability
	if err := json.Unmarshal(env.Suitability, &w); err != nil {
		var arr []wireSuitability
		if err2 := json.Unmarshal(env.Suitability, &arr); err2 == nil && len(arr) > 0 {
			w = arr[0]
		} else {
			return nil, fmt.Errorf("northcapital: suitability decode: %w", err)
		}
	}
	return &SuitabilityResult{
		PartyID:           w.PartyID,
		Status:            normalizeStatus(w.Status),
		RiskTolerance:     w.RiskTolerance,
		InvestmentHorizon: w.InvestmentHorizon,
		NetWorth:          w.NetWorth,
		AnnualIncome:      w.AnnualIncome,
		LiquidityNeeds:    w.LiquidityNeeds,
		Reasons:           w.Reasons,
		RecordedAt:        firstParseable(w.RecordedAt),
	}, nil
}

// firstParseable runs parseDate but returns the zero value if every
// layout fails. Public helper to keep the per-endpoint files small.
func firstParseable(s string) time.Time { return parseDate(s) }

// normalizeStatus maps TransactAPI's varied status strings into the
// adapter's canonical (pass / fail / manual_review / pending) set so
// downstream consumers don't write per-endpoint matchers.
func normalizeStatus(s string) string {
	switch s {
	case "Approved", "approved", "Pass", "pass", "Verified", "verified":
		return "pass"
	case "Disapproved", "disapproved", "Failed", "failed", "Fail", "fail":
		return "fail"
	case "Pending", "pending", "Manual Review", "manual_review", "manualReview", "Review":
		return "manual_review"
	case "Expired", "expired":
		return "expired"
	case "":
		return ""
	default:
		return s
	}
}
