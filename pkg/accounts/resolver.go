package accounts

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/broker/pkg/provider"
)

// AccountMapping links a user ID (from IAM) to a provider account ID.
type AccountMapping struct {
	UserID    string    `json:"user_id"`
	OrgID     string    `json:"org_id"`
	Provider  string    `json:"provider"`
	AccountID string    `json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
}

// mappingKey is the composite key for user+provider lookups.
type mappingKey struct {
	userID   string
	provider string
}

// Resolver is a thread-safe store of user-to-provider account mappings.
type Resolver struct {
	mu       sync.RWMutex
	byKey    map[mappingKey]AccountMapping   // user+provider -> mapping
	byUser   map[string][]AccountMapping     // user -> all mappings
}

// NewResolver creates an empty Resolver.
func NewResolver() *Resolver {
	return &Resolver{
		byKey:  make(map[mappingKey]AccountMapping),
		byUser: make(map[string][]AccountMapping),
	}
}

// SetMapping registers or updates a user-to-provider account mapping.
func (r *Resolver) SetMapping(userID, orgID, providerName, accountID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := mappingKey{userID: userID, provider: providerName}
	m := AccountMapping{
		UserID:    userID,
		OrgID:     orgID,
		Provider:  providerName,
		AccountID: accountID,
		CreatedAt: time.Now(),
	}

	// If updating an existing mapping, replace it in the byUser slice too.
	if _, exists := r.byKey[key]; exists {
		r.removeFromUserSlice(userID, providerName)
	}

	r.byKey[key] = m
	r.byUser[userID] = append(r.byUser[userID], m)
}

// RemoveMapping removes the mapping for a user+provider pair.
func (r *Resolver) RemoveMapping(userID, providerName string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := mappingKey{userID: userID, provider: providerName}
	if _, exists := r.byKey[key]; !exists {
		return
	}
	delete(r.byKey, key)
	r.removeFromUserSlice(userID, providerName)
}

// removeFromUserSlice removes the entry for providerName from byUser[userID].
// Caller must hold mu.
func (r *Resolver) removeFromUserSlice(userID, providerName string) {
	mappings := r.byUser[userID]
	for i, m := range mappings {
		if m.Provider == providerName {
			r.byUser[userID] = append(mappings[:i], mappings[i+1:]...)
			if len(r.byUser[userID]) == 0 {
				delete(r.byUser, userID)
			}
			return
		}
	}
}

// ResolveAccount returns the provider account ID for a user+provider pair.
func (r *Resolver) ResolveAccount(userID, providerName string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, ok := r.byKey[mappingKey{userID: userID, provider: providerName}]
	if !ok {
		return "", false
	}
	return m.AccountID, true
}

// ResolveAnyAccount returns the first available provider and account ID for a user.
func (r *Resolver) ResolveAnyAccount(userID string) (providerName, accountID string, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mappings := r.byUser[userID]
	if len(mappings) == 0 {
		return "", "", false
	}
	return mappings[0].Provider, mappings[0].AccountID, true
}

// ListMappings returns all account mappings for a user.
func (r *Resolver) ListMappings(userID string) []AccountMapping {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mappings := r.byUser[userID]
	if len(mappings) == 0 {
		return nil
	}
	out := make([]AccountMapping, len(mappings))
	copy(out, mappings)
	return out
}

// AutoDiscover queries all providers in the registry for accounts belonging to
// the given user/org and registers any discovered mappings.
func (r *Resolver) AutoDiscover(ctx context.Context, registry *provider.Registry, userID, orgID string) error {
	names := registry.List()
	var errs []error

	for _, name := range names {
		p, err := registry.Get(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("provider %s: %w", name, err))
			continue
		}

		accounts, err := p.ListAccounts(ctx)
		if err != nil {
			errs = append(errs, fmt.Errorf("provider %s list accounts: %w", name, err))
			continue
		}

		for _, acct := range accounts {
			if acct.UserID == userID || (orgID != "" && acct.OrgID == orgID) {
				r.SetMapping(userID, orgID, name, acct.ProviderID)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("auto-discover encountered %d errors: %v", len(errs), errs)
	}
	return nil
}

// ImportFromJWT extracts user identity from JWT claims and registers mappings
// for any accounts found in the token payload.
func (r *Resolver) ImportFromJWT(tokenPayload map[string]interface{}) error {
	sub, _ := tokenPayload["sub"].(string)
	if sub == "" {
		return fmt.Errorf("JWT missing 'sub' claim")
	}

	orgID, _ := tokenPayload["owner"].(string)

	// If the JWT carries provider account mappings (e.g. from IAM enrichment),
	// register them directly.
	if accts, ok := tokenPayload["accounts"].(map[string]interface{}); ok {
		for providerName, v := range accts {
			if accountID, ok := v.(string); ok {
				r.SetMapping(sub, orgID, providerName, accountID)
			}
		}
	}

	return nil
}
