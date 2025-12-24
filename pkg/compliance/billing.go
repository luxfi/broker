package compliance

import "net/http"

// billingHandler holds billing HTTP handler state.
type billingHandler struct{}

func (h *billingHandler) handleGetBilling(w http.ResponseWriter, r *http.Request) {
	// Static/mock billing data. In production this would come from Stripe.
	billing := BillingInfo{
		Plan:          "professional",
		PaymentMethod: "visa_4242",
		NextBilling:   "2026-04-01",
		MonthlyUsage:  299.00,
		Invoices: []Invoice{
			{ID: "inv-001", Date: "2026-03-01", Amount: 299.00, Status: "paid"},
			{ID: "inv-002", Date: "2026-02-01", Amount: 299.00, Status: "paid"},
			{ID: "inv-003", Date: "2026-01-01", Amount: 299.00, Status: "paid"},
		},
	}
	writeJSON(w, http.StatusOK, billing)
}
