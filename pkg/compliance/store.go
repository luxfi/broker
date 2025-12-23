package compliance

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// ComplianceStore is the interface for compliance data persistence.
// MemoryStore and PostgresStore both implement this interface.
type ComplianceStore interface {
	// Identity
	SaveIdentity(id *Identity) error
	GetIdentity(id string) (*Identity, error)

	// Pipeline
	SavePipeline(p *Pipeline) error
	GetPipeline(id string) (*Pipeline, error)
	ListPipelines() []*Pipeline
	DeletePipeline(id string) error

	// Session
	SaveSession(sess *Session) error
	GetSession(id string) (*Session, error)
	ListSessions() []*Session

	// Fund
	SaveFund(f *Fund) error
	GetFund(id string) (*Fund, error)
	ListFunds() []*Fund
	DeleteFund(id string) error
	AddFundInvestor(inv *FundInvestor) error
	ListFundInvestors(fundID string) []*FundInvestor

	// Envelope
	SaveEnvelope(env *Envelope) error
	GetEnvelope(id string) (*Envelope, error)
	ListEnvelopes() []*Envelope

	// Template
	SaveTemplate(t *Template) error
	GetTemplate(id string) (*Template, error)
	ListTemplates() []*Template

	// Role
	SaveRole(role *Role) error
	GetRole(id string) (*Role, error)
	ListRoles() []*Role
	DeleteRole(id string) error

	// User
	SaveUser(u *User) error
	GetUser(id string) (*User, error)
	ListUsers() []*User

	// Transaction
	SaveTransaction(tx *Transaction) error
	ListTransactions() []*Transaction

	// Credential
	SaveCredential(c *Credential) error
	ListCredentials() []*Credential
	DeleteCredential(id string) error

	// AML Screening
	SaveAMLScreening(s *AMLScreening) error
	GetAMLScreening(id string) (*AMLScreening, error)
	ListAMLScreeningsByAccount(accountID string) []*AMLScreening
	ListAMLScreeningsByStatus(status AMLStatus) []*AMLScreening

	// Application (onboarding)
	SaveApplication(app *Application) error
	GetApplication(id string) (*Application, error)
	GetApplicationByUser(userID string) (*Application, error)
	ListApplications() []*Application
	ListApplicationsByStatus(status ApplicationStatus) []*Application

	// Document Upload
	SaveDocumentUpload(doc *DocumentUpload) error
	GetDocumentUpload(id string) (*DocumentUpload, error)
	ListDocumentUploads(applicationID string) []*DocumentUpload

	// Identity listing
	ListIdentitiesByUser(userID string) []*Identity

	// Settings
	GetSettings() *Settings
	SaveSettings(settings *Settings)

	// Dashboard
	ComputeDashboard() *DashboardStats
	ComputeESignStats() *ESignStats
	ListEnvelopesByDirection(direction string) []*Envelope
}

// MemoryStore is an in-memory compliance data store.
// Use PostgresStore in production.
type MemoryStore struct {
	mu              sync.RWMutex
	identities      map[string]*Identity        // id -> Identity
	businesses      map[string]*BusinessKYB      // id -> BusinessKYB
	pipelines       map[string]*Pipeline         // id -> Pipeline
	sessions        map[string]*Session          // id -> Session
	funds           map[string]*Fund             // id -> Fund
	investors       map[string][]*FundInvestor   // fundID -> investors
	envelopes       map[string]*Envelope         // id -> Envelope
	templates       map[string]*Template         // id -> Template
	roles           map[string]*Role             // id -> Role
	users           map[string]*User             // id -> User
	transactions    map[string]*Transaction      // id -> Transaction
	credentials     map[string]*Credential       // id -> Credential
	amlScreenings   map[string]*AMLScreening     // id -> AMLScreening
	applications    map[string]*Application      // id -> Application
	documentUploads map[string]*DocumentUpload   // id -> DocumentUpload
	settings        *Settings
}

// NewMemoryStore creates an empty in-memory compliance store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		identities:      make(map[string]*Identity),
		businesses:      make(map[string]*BusinessKYB),
		pipelines:       make(map[string]*Pipeline),
		sessions:        make(map[string]*Session),
		funds:           make(map[string]*Fund),
		investors:       make(map[string][]*FundInvestor),
		envelopes:       make(map[string]*Envelope),
		templates:       make(map[string]*Template),
		roles:           make(map[string]*Role),
		users:           make(map[string]*User),
		transactions:    make(map[string]*Transaction),
		credentials:     make(map[string]*Credential),
		amlScreenings:   make(map[string]*AMLScreening),
		applications:    make(map[string]*Application),
		documentUploads: make(map[string]*DocumentUpload),
		settings: &Settings{
			BusinessName:      "Your Company",
			Timezone:          "America/New_York",
			Currency:          "USD",
			NotificationEmail: "compliance@example.com",
		},
	}
}

// NewStore is an alias for NewMemoryStore, preserving backward compatibility.
func NewStore() *MemoryStore {
	return NewMemoryStore()
}

// --- Identity ---

func (s *MemoryStore) SaveIdentity(id *Identity) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id.ID == "" {
		id.ID = generateID()
	}
	if id.CreatedAt.IsZero() {
		id.CreatedAt = time.Now().UTC()
	}
	id.UpdatedAt = time.Now().UTC()
	s.identities[id.ID] = id
	return nil
}

func (s *MemoryStore) GetIdentity(id string) (*Identity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ident, ok := s.identities[id]
	if !ok {
		return nil, fmt.Errorf("identity not found")
	}
	return ident, nil
}

// --- Pipeline ---

func (s *MemoryStore) SavePipeline(p *Pipeline) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.ID == "" {
		p.ID = generateID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	p.UpdatedAt = time.Now().UTC()
	s.pipelines[p.ID] = p
	return nil
}

func (s *MemoryStore) GetPipeline(id string) (*Pipeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.pipelines[id]
	if !ok {
		return nil, fmt.Errorf("pipeline not found")
	}
	return p, nil
}

func (s *MemoryStore) ListPipelines() []*Pipeline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Pipeline, 0, len(s.pipelines))
	for _, p := range s.pipelines {
		out = append(out, p)
	}
	return out
}

func (s *MemoryStore) DeletePipeline(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.pipelines[id]; !ok {
		return fmt.Errorf("pipeline not found")
	}
	delete(s.pipelines, id)
	return nil
}

// --- Session ---

func (s *MemoryStore) SaveSession(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess.ID == "" {
		sess.ID = generateID()
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}
	s.sessions[sess.ID] = sess
	return nil
}

func (s *MemoryStore) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return sess, nil
}

func (s *MemoryStore) ListSessions() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	return out
}

// --- Fund ---

func (s *MemoryStore) SaveFund(f *Fund) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.ID == "" {
		f.ID = generateID()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now().UTC()
	}
	f.UpdatedAt = time.Now().UTC()
	s.funds[f.ID] = f
	return nil
}

func (s *MemoryStore) GetFund(id string) (*Fund, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.funds[id]
	if !ok {
		return nil, fmt.Errorf("fund not found")
	}
	return f, nil
}

func (s *MemoryStore) ListFunds() []*Fund {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Fund, 0, len(s.funds))
	for _, f := range s.funds {
		out = append(out, f)
	}
	return out
}

func (s *MemoryStore) DeleteFund(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.funds[id]; !ok {
		return fmt.Errorf("fund not found")
	}
	delete(s.funds, id)
	delete(s.investors, id)
	return nil
}

func (s *MemoryStore) AddFundInvestor(inv *FundInvestor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if inv.ID == "" {
		inv.ID = generateID()
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}
	f, ok := s.funds[inv.FundID]
	if !ok {
		return fmt.Errorf("fund not found")
	}
	s.investors[inv.FundID] = append(s.investors[inv.FundID], inv)
	f.InvestorCount = len(s.investors[inv.FundID])
	f.TotalRaised += inv.Amount
	return nil
}

func (s *MemoryStore) ListFundInvestors(fundID string) []*FundInvestor {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.investors[fundID]
}

// --- Envelope ---

func (s *MemoryStore) SaveEnvelope(env *Envelope) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if env.ID == "" {
		env.ID = generateID()
	}
	if env.CreatedAt.IsZero() {
		env.CreatedAt = time.Now().UTC()
	}
	env.UpdatedAt = time.Now().UTC()
	s.envelopes[env.ID] = env
	return nil
}

func (s *MemoryStore) GetEnvelope(id string) (*Envelope, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	env, ok := s.envelopes[id]
	if !ok {
		return nil, fmt.Errorf("envelope not found")
	}
	return env, nil
}

func (s *MemoryStore) ListEnvelopes() []*Envelope {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Envelope, 0, len(s.envelopes))
	for _, env := range s.envelopes {
		out = append(out, env)
	}
	return out
}

// --- Template ---

func (s *MemoryStore) SaveTemplate(t *Template) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.ID == "" {
		t.ID = generateID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	s.templates[t.ID] = t
	return nil
}

func (s *MemoryStore) GetTemplate(id string) (*Template, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.templates[id]
	if !ok {
		return nil, fmt.Errorf("template not found")
	}
	return t, nil
}

func (s *MemoryStore) ListTemplates() []*Template {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Template, 0, len(s.templates))
	for _, t := range s.templates {
		out = append(out, t)
	}
	return out
}

// --- Role ---

func (s *MemoryStore) SaveRole(role *Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if role.ID == "" {
		role.ID = generateID()
	}
	if role.CreatedAt.IsZero() {
		role.CreatedAt = time.Now().UTC()
	}
	role.UpdatedAt = time.Now().UTC()
	s.roles[role.ID] = role
	return nil
}

func (s *MemoryStore) GetRole(id string) (*Role, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	role, ok := s.roles[id]
	if !ok {
		return nil, fmt.Errorf("role not found")
	}
	return role, nil
}

func (s *MemoryStore) ListRoles() []*Role {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Role, 0, len(s.roles))
	for _, role := range s.roles {
		out = append(out, role)
	}
	return out
}

func (s *MemoryStore) DeleteRole(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.roles[id]; !ok {
		return fmt.Errorf("role not found")
	}
	delete(s.roles, id)
	return nil
}

// --- User ---

func (s *MemoryStore) SaveUser(u *User) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u.ID == "" {
		u.ID = generateID()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	s.users[u.ID] = u
	return nil
}

func (s *MemoryStore) GetUser(id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[id]
	if !ok {
		return nil, fmt.Errorf("user not found")
	}
	return u, nil
}

func (s *MemoryStore) ListUsers() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, u)
	}
	return out
}

// --- Transaction ---

func (s *MemoryStore) SaveTransaction(tx *Transaction) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if tx.ID == "" {
		tx.ID = generateID()
	}
	s.transactions[tx.ID] = tx
	return nil
}

func (s *MemoryStore) ListTransactions() []*Transaction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Transaction, 0, len(s.transactions))
	for _, tx := range s.transactions {
		out = append(out, tx)
	}
	return out
}

// --- Credential ---

func (s *MemoryStore) SaveCredential(c *Credential) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ID == "" {
		c.ID = generateID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	s.credentials[c.ID] = c
	return nil
}

func (s *MemoryStore) ListCredentials() []*Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Credential, 0, len(s.credentials))
	for _, c := range s.credentials {
		out = append(out, c)
	}
	return out
}

func (s *MemoryStore) DeleteCredential(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.credentials[id]; !ok {
		return fmt.Errorf("credential not found")
	}
	delete(s.credentials, id)
	return nil
}

// --- Settings ---

func (s *MemoryStore) GetSettings() *Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := *s.settings
	return &cp
}

func (s *MemoryStore) SaveSettings(settings *Settings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settings = settings
}

// --- Dashboard ---

// ComputeDashboard aggregates store data into dashboard statistics.
func (s *MemoryStore) ComputeDashboard() *DashboardStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &DashboardStats{
		TotalFunds:          len(s.funds),
		MonthlyTransactions: len(s.transactions),
	}

	for _, sess := range s.sessions {
		if sess.Status == SessionPending || sess.Status == SessionInProgress {
			stats.ActiveSessions++
		}
		if sess.KYCStatus == KYCPending {
			stats.PendingKYC++
		}
	}

	stats.RecentSessions = make([]*Session, 0, 5)
	count := 0
	for _, sess := range s.sessions {
		if count >= 5 {
			break
		}
		stats.RecentSessions = append(stats.RecentSessions, sess)
		count++
	}

	stats.RecentTransactions = make([]*Transaction, 0, 5)
	count = 0
	for _, tx := range s.transactions {
		if count >= 5 {
			break
		}
		stats.RecentTransactions = append(stats.RecentTransactions, tx)
		count++
	}

	return stats
}

// --- eSign Dashboard ---

// ComputeESignStats aggregates eSign envelope/template data.
func (s *MemoryStore) ComputeESignStats() *ESignStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &ESignStats{
		Templates: len(s.templates),
	}

	for _, env := range s.envelopes {
		switch env.Status {
		case EnvelopePending, EnvelopeSent, EnvelopeViewed:
			stats.Pending++
		case EnvelopeCompleted, EnvelopeSigned:
			stats.Completed++
		}
	}
	stats.Draft = stats.Pending

	stats.Recent = make([]*Envelope, 0, 5)
	count := 0
	for _, env := range s.envelopes {
		if count >= 5 {
			break
		}
		stats.Recent = append(stats.Recent, env)
		count++
	}

	return stats
}

// ListEnvelopesByDirection returns envelopes filtered by direction.
// "inbox" returns envelopes with status sent/pending (received for signing).
// "sent" returns envelopes that were created/sent out.
func (s *MemoryStore) ListEnvelopesByDirection(direction string) []*Envelope {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Envelope, 0)
	for _, env := range s.envelopes {
		switch direction {
		case "inbox":
			if env.Status == EnvelopeSent || env.Status == EnvelopePending || env.Status == EnvelopeViewed {
				out = append(out, env)
			}
		case "sent":
			if env.Status == EnvelopeSigned || env.Status == EnvelopeCompleted || env.Status == EnvelopeDeclined || env.Status == EnvelopeVoided {
				out = append(out, env)
			}
		}
	}
	return out
}

// --- AML Screening ---

func (s *MemoryStore) SaveAMLScreening(sc *AMLScreening) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sc.ID == "" {
		sc.ID = generateID()
	}
	if sc.CreatedAt.IsZero() {
		sc.CreatedAt = time.Now().UTC()
	}
	sc.UpdatedAt = time.Now().UTC()
	s.amlScreenings[sc.ID] = sc
	return nil
}

func (s *MemoryStore) GetAMLScreening(id string) (*AMLScreening, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sc, ok := s.amlScreenings[id]
	if !ok {
		return nil, fmt.Errorf("aml screening not found")
	}
	return sc, nil
}

func (s *MemoryStore) ListAMLScreeningsByAccount(accountID string) []*AMLScreening {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*AMLScreening
	for _, sc := range s.amlScreenings {
		if sc.AccountID == accountID {
			out = append(out, sc)
		}
	}
	if out == nil {
		out = make([]*AMLScreening, 0)
	}
	return out
}

func (s *MemoryStore) ListAMLScreeningsByStatus(status AMLStatus) []*AMLScreening {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*AMLScreening
	for _, sc := range s.amlScreenings {
		if sc.Status == status {
			out = append(out, sc)
		}
	}
	if out == nil {
		out = make([]*AMLScreening, 0)
	}
	return out
}

// --- Application ---

func (s *MemoryStore) SaveApplication(app *Application) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if app.ID == "" {
		app.ID = generateID()
	}
	if app.CreatedAt.IsZero() {
		app.CreatedAt = time.Now().UTC()
	}
	app.UpdatedAt = time.Now().UTC()
	s.applications[app.ID] = app
	return nil
}

func (s *MemoryStore) GetApplication(id string) (*Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	app, ok := s.applications[id]
	if !ok {
		return nil, fmt.Errorf("application not found")
	}
	return app, nil
}

func (s *MemoryStore) GetApplicationByUser(userID string) (*Application, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, app := range s.applications {
		if app.UserID == userID {
			return app, nil
		}
	}
	return nil, fmt.Errorf("application not found")
}

func (s *MemoryStore) ListApplications() []*Application {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Application, 0, len(s.applications))
	for _, app := range s.applications {
		out = append(out, app)
	}
	return out
}

func (s *MemoryStore) ListApplicationsByStatus(status ApplicationStatus) []*Application {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Application
	for _, app := range s.applications {
		if app.Status == status {
			out = append(out, app)
		}
	}
	if out == nil {
		out = make([]*Application, 0)
	}
	return out
}

// --- Document Upload ---

func (s *MemoryStore) SaveDocumentUpload(doc *DocumentUpload) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if doc.ID == "" {
		doc.ID = generateID()
	}
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	doc.UpdatedAt = time.Now().UTC()
	s.documentUploads[doc.ID] = doc
	return nil
}

func (s *MemoryStore) GetDocumentUpload(id string) (*DocumentUpload, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	doc, ok := s.documentUploads[id]
	if !ok {
		return nil, fmt.Errorf("document not found")
	}
	return doc, nil
}

func (s *MemoryStore) ListDocumentUploads(applicationID string) []*DocumentUpload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*DocumentUpload
	for _, doc := range s.documentUploads {
		if doc.ApplicationID == applicationID {
			out = append(out, doc)
		}
	}
	if out == nil {
		out = make([]*DocumentUpload, 0)
	}
	return out
}

// --- Identity listing ---

func (s *MemoryStore) ListIdentitiesByUser(userID string) []*Identity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Identity
	for _, ident := range s.identities {
		if ident.UserID == userID {
			out = append(out, ident)
		}
	}
	if out == nil {
		out = make([]*Identity, 0)
	}
	return out
}

// generateID returns a random hex ID. Panics if the system CSPRNG fails,
// which indicates a catastrophic OS-level problem.
func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b)
}
