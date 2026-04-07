package webhook

import "time"

// Webhook is a registered webhook endpoint.
type Webhook struct {
	ID        string   `json:"id"`
	OrgID     string   `json:"org_id"`
	URL       string   `json:"url"`
	Secret    string   `json:"-"` // HMAC-SHA256 signing secret; never serialized
	Events    []string `json:"events"`
	Active    bool     `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

// Delivery is a log entry for a single webhook delivery attempt.
type Delivery struct {
	ID           string    `json:"id"`
	WebhookID    string    `json:"webhook_id"`
	EventType    string    `json:"event_type"`
	EventID      string    `json:"event_id"`
	Status       string    `json:"status"` // pending, delivered, failed
	ResponseCode int      `json:"response_code,omitempty"`
	Error        string    `json:"error,omitempty"`
	Attempts     int       `json:"attempts"`
	CreatedAt    time.Time `json:"created_at"`
}

// Store is the persistence interface for webhook registrations and delivery logs.
type Store interface {
	// ListByEvent returns all active webhooks for the given org subscribed to the event.
	ListByEvent(orgID, event string) ([]Webhook, error)

	// GetByID returns a single webhook by ID, scoped to org.
	GetByID(orgID, id string) (*Webhook, error)

	// List returns all webhooks for the given org.
	List(orgID string) ([]Webhook, error)

	// Save creates or updates a webhook.
	Save(wh *Webhook) error

	// Delete removes a webhook by ID, scoped to org.
	Delete(orgID, id string) error

	// LogDelivery records a delivery attempt.
	LogDelivery(d *Delivery) error

	// ListDeliveries returns delivery history for a webhook.
	ListDeliveries(webhookID string, limit int) ([]Delivery, error)
}
