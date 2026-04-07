package webhook

import (
	"fmt"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for development and testing.
type MemoryStore struct {
	mu         sync.RWMutex
	webhooks   map[string]*Webhook   // id -> webhook
	deliveries map[string]*Delivery  // id -> delivery
	counter    int
}

// NewMemoryStore returns an in-memory webhook store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		webhooks:   make(map[string]*Webhook),
		deliveries: make(map[string]*Delivery),
	}
}

func (s *MemoryStore) nextID() string {
	s.counter++
	return fmt.Sprintf("wh_%d", s.counter)
}

func (s *MemoryStore) ListByEvent(orgID, event string) ([]Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Webhook
	for _, wh := range s.webhooks {
		if wh.OrgID != orgID || !wh.Active {
			continue
		}
		for _, ev := range wh.Events {
			if ev == event || ev == "*" {
				out = append(out, *wh)
				break
			}
		}
	}
	return out, nil
}

func (s *MemoryStore) GetByID(orgID, id string) (*Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	wh, ok := s.webhooks[id]
	if !ok || wh.OrgID != orgID {
		return nil, fmt.Errorf("webhook not found")
	}
	return wh, nil
}

func (s *MemoryStore) List(orgID string) ([]Webhook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Webhook
	for _, wh := range s.webhooks {
		if wh.OrgID == orgID {
			out = append(out, *wh)
		}
	}
	return out, nil
}

func (s *MemoryStore) Save(wh *Webhook) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if wh.ID == "" {
		wh.ID = s.nextID()
		wh.CreatedAt = time.Now().UTC()
	}
	s.webhooks[wh.ID] = wh
	return nil
}

func (s *MemoryStore) Delete(orgID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	wh, ok := s.webhooks[id]
	if !ok || wh.OrgID != orgID {
		return fmt.Errorf("webhook not found")
	}
	delete(s.webhooks, id)
	return nil
}

func (s *MemoryStore) LogDelivery(d *Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if d.ID == "" {
		s.counter++
		d.ID = fmt.Sprintf("del_%d", s.counter)
		d.CreatedAt = time.Now().UTC()
	}
	s.deliveries[d.ID] = d
	return nil
}

func (s *MemoryStore) ListDeliveries(webhookID string, limit int) ([]Delivery, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []Delivery
	for _, d := range s.deliveries {
		if d.WebhookID == webhookID {
			out = append(out, *d)
			if limit > 0 && len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}
