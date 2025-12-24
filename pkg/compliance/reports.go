package compliance

import (
	"net/http"
	"time"
)

// reportsHandler holds reports HTTP handler state.
type reportsHandler struct{}

// reportType describes an available report.
type reportType struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	LastGenerated string `json:"lastGenerated"`
}

func (h *reportsHandler) handleListReports(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC().Format(time.RFC3339)
	reports := []reportType{
		{
			ID:            "monthly-statement",
			Name:          "Monthly Statement",
			Description:   "Comprehensive monthly account statement with all activity",
			LastGenerated: now,
		},
		{
			ID:            "form-8949",
			Name:          "Form 8949",
			Description:   "IRS Form 8949 for reporting capital gains and losses",
			LastGenerated: now,
		},
		{
			ID:            "form-1099b",
			Name:          "1099-B",
			Description:   "Broker proceeds from securities and barter exchange transactions",
			LastGenerated: now,
		},
		{
			ID:            "transaction-history",
			Name:          "Transaction History",
			Description:   "Complete transaction history export for audit and compliance",
			LastGenerated: now,
		},
	}
	writeJSON(w, http.StatusOK, reports)
}
