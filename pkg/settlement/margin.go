package settlement

// MarginPolicy defines thresholds for margin monitoring on prefunded positions.
//
// When a user instant-buys an asset with pool capital, the pool is exposed to
// price risk until ACH settles. If the asset drops significantly, the pool's
// collateral (the purchased asset) may not cover the original advance.
//
// Three escalation levels:
//   - Warning: notify the user, no action yet
//   - MarginCall: demand additional funds or prepare to liquidate
//   - Liquidation: auto-sell to protect pool capital
type MarginPolicy struct {
	// WarningPct is the drawdown percentage that triggers a warning.
	// E.g., 0.20 means warn when asset drops 20% from entry.
	WarningPct float64 `json:"warning_pct"`

	// MarginCallPct is the drawdown that triggers a margin call.
	// E.g., 0.30 means margin call at 30% drop.
	MarginCallPct float64 `json:"margin_call_pct"`

	// LiquidationPct is the drawdown that triggers automatic liquidation.
	// E.g., 0.50 means force-sell at 50% drop.
	LiquidationPct float64 `json:"liquidation_pct"`

	// GracePeriod is how long after a margin call before auto-liquidation.
	// Gives the user time to deposit additional funds.
	GracePeriod string `json:"grace_period"`
}

// AlertType categorizes the severity of a margin alert.
type AlertType string

const (
	AlertWarning     AlertType = "warning"
	AlertMarginCall  AlertType = "margin_call"
	AlertLiquidation AlertType = "liquidation"
)

// MarginAlert is emitted when a prefunded position's value deteriorates.
type MarginAlert struct {
	ReservationID string    `json:"reservation_id"`
	AccountID     string    `json:"account_id"`
	Asset         string    `json:"asset"`
	CurrentPrice  float64   `json:"current_price"`
	EntryPrice    float64   `json:"entry_price"`
	DrawdownPct   float64   `json:"drawdown_pct"` // 0.0 to 1.0
	Type          AlertType `json:"alert_type"`
}

// DefaultMarginPolicy returns conservative margin thresholds suitable for
// a prefunding pool on equity and crypto positions.
//
//   - 20% drop: warning (notify user, log)
//   - 30% drop: margin call (demand deposit, freeze further instant buys)
//   - 50% drop: auto-liquidate (sell position, recover what we can)
//   - 24h grace period between margin call and forced liquidation
func DefaultMarginPolicy() *MarginPolicy {
	return &MarginPolicy{
		WarningPct:     0.20,
		MarginCallPct:  0.30,
		LiquidationPct: 0.50,
		GracePeriod:    "24h",
	}
}

// CheckMargin evaluates a reservation against current market price and
// returns an alert if any threshold is breached. Returns nil if the position
// is healthy.
//
// Drawdown is calculated as: (entryPrice - currentPrice) / entryPrice
// A positive drawdown means the asset lost value since purchase.
func CheckMargin(reservation *Reservation, currentPrice float64, policy *MarginPolicy) *MarginAlert {
	if reservation.Price <= 0 || currentPrice <= 0 {
		return nil
	}

	// Only monitor positions that are still pending settlement or already margin-called.
	if reservation.Status != StatusPendingSettlement && reservation.Status != StatusMarginCalled {
		return nil
	}

	drawdown := (reservation.Price - currentPrice) / reservation.Price

	// No alert if price hasn't dropped enough for a warning.
	if drawdown < policy.WarningPct {
		return nil
	}

	alert := &MarginAlert{
		ReservationID: reservation.ID,
		AccountID:     reservation.AccountID,
		Asset:         reservation.Asset,
		CurrentPrice:  currentPrice,
		EntryPrice:    reservation.Price,
		DrawdownPct:   drawdown,
	}

	// Escalate based on severity — most severe threshold wins.
	switch {
	case drawdown >= policy.LiquidationPct:
		alert.Type = AlertLiquidation
	case drawdown >= policy.MarginCallPct:
		alert.Type = AlertMarginCall
	default:
		alert.Type = AlertWarning
	}

	return alert
}
