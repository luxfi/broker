package compliance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore implements ComplianceStore backed by PostgreSQL.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore creates a PostgresStore with the given connection pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// --- Identity ---

func (s *PostgresStore) SaveIdentity(id *Identity) error {
	ctx := context.Background()
	if id.ID == "" {
		id.ID = generateID()
	}
	if id.CreatedAt.IsZero() {
		id.CreatedAt = time.Now().UTC()
	}
	id.UpdatedAt = time.Now().UTC()

	dataJSON, err := json.Marshal(id.Data)
	if err != nil {
		return fmt.Errorf("marshal identity data: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO identities (id, user_id, provider, status, data, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			provider = EXCLUDED.provider,
			status = EXCLUDED.status,
			data = EXCLUDED.data,
			updated_at = EXCLUDED.updated_at
	`, id.ID, id.UserID, id.Provider, string(id.Status), dataJSON, id.CreatedAt, id.UpdatedAt)
	return err
}

func (s *PostgresStore) GetIdentity(id string) (*Identity, error) {
	ctx := context.Background()
	var ident Identity
	var dataJSON []byte
	var status string

	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, provider, status, data, created_at, updated_at
		FROM identities WHERE id = $1
	`, id).Scan(&ident.ID, &ident.UserID, &ident.Provider, &status, &dataJSON, &ident.CreatedAt, &ident.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("identity not found")
		}
		return nil, err
	}
	ident.Status = KYCStatus(status)
	if len(dataJSON) > 0 {
		json.Unmarshal(dataJSON, &ident.Data)
	}
	if ident.Data == nil {
		ident.Data = make(map[string]interface{})
	}
	return &ident, nil
}

// --- Pipeline ---

func (s *PostgresStore) SavePipeline(p *Pipeline) error {
	ctx := context.Background()
	if p.ID == "" {
		p.ID = generateID()
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	p.UpdatedAt = time.Now().UTC()

	stepsJSON, err := json.Marshal(p.Steps)
	if err != nil {
		return fmt.Errorf("marshal pipeline steps: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO pipelines (id, name, business_id, status, steps, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			business_id = EXCLUDED.business_id,
			status = EXCLUDED.status,
			steps = EXCLUDED.steps,
			updated_at = EXCLUDED.updated_at
	`, p.ID, p.Name, p.BusinessID, p.Status, stepsJSON, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *PostgresStore) GetPipeline(id string) (*Pipeline, error) {
	ctx := context.Background()
	var p Pipeline
	var stepsJSON []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, name, business_id, status, steps, created_at, updated_at
		FROM pipelines WHERE id = $1
	`, id).Scan(&p.ID, &p.Name, &p.BusinessID, &p.Status, &stepsJSON, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("pipeline not found")
		}
		return nil, err
	}
	if len(stepsJSON) > 0 {
		json.Unmarshal(stepsJSON, &p.Steps)
	}
	return &p, nil
}

func (s *PostgresStore) ListPipelines() []*Pipeline {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, business_id, status, steps, created_at, updated_at
		FROM pipelines ORDER BY created_at DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Pipeline
	for rows.Next() {
		var p Pipeline
		var stepsJSON []byte
		if err := rows.Scan(&p.ID, &p.Name, &p.BusinessID, &p.Status, &stepsJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
			continue
		}
		if len(stepsJSON) > 0 {
			json.Unmarshal(stepsJSON, &p.Steps)
		}
		out = append(out, &p)
	}
	if out == nil {
		out = make([]*Pipeline, 0)
	}
	return out
}

func (s *PostgresStore) DeletePipeline(id string) error {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx, `DELETE FROM pipelines WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("pipeline not found")
	}
	return nil
}

// --- Session ---

func (s *PostgresStore) SaveSession(sess *Session) error {
	ctx := context.Background()
	if sess.ID == "" {
		sess.ID = generateID()
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}

	stepsJSON, err := json.Marshal(sess.Steps)
	if err != nil {
		return fmt.Errorf("marshal session steps: %w", err)
	}

	var completedAt *time.Time
	if !sess.CompletedAt.IsZero() {
		completedAt = &sess.CompletedAt
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO sessions (id, pipeline_id, investor_email, investor_name, status, kyc_status, steps, created_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO UPDATE SET
			pipeline_id = EXCLUDED.pipeline_id,
			investor_email = EXCLUDED.investor_email,
			investor_name = EXCLUDED.investor_name,
			status = EXCLUDED.status,
			kyc_status = EXCLUDED.kyc_status,
			steps = EXCLUDED.steps,
			completed_at = EXCLUDED.completed_at
	`, sess.ID, sess.PipelineID, sess.InvestorEmail, sess.InvestorName,
		string(sess.Status), string(sess.KYCStatus), stepsJSON, sess.CreatedAt, completedAt)
	return err
}

func (s *PostgresStore) GetSession(id string) (*Session, error) {
	ctx := context.Background()
	var sess Session
	var stepsJSON []byte
	var status, kycStatus string
	var completedAt *time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT id, pipeline_id, investor_email, investor_name, status, kyc_status, steps, created_at, completed_at
		FROM sessions WHERE id = $1
	`, id).Scan(&sess.ID, &sess.PipelineID, &sess.InvestorEmail, &sess.InvestorName,
		&status, &kycStatus, &stepsJSON, &sess.CreatedAt, &completedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("session not found")
		}
		return nil, err
	}
	sess.Status = SessionStatus(status)
	sess.KYCStatus = KYCStatus(kycStatus)
	if completedAt != nil {
		sess.CompletedAt = *completedAt
	}
	if len(stepsJSON) > 0 {
		json.Unmarshal(stepsJSON, &sess.Steps)
	}
	return &sess, nil
}

func (s *PostgresStore) ListSessions() []*Session {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, pipeline_id, investor_email, investor_name, status, kyc_status, steps, created_at, completed_at
		FROM sessions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var sess Session
		var stepsJSON []byte
		var status, kycStatus string
		var completedAt *time.Time
		if err := rows.Scan(&sess.ID, &sess.PipelineID, &sess.InvestorEmail, &sess.InvestorName,
			&status, &kycStatus, &stepsJSON, &sess.CreatedAt, &completedAt); err != nil {
			continue
		}
		sess.Status = SessionStatus(status)
		sess.KYCStatus = KYCStatus(kycStatus)
		if completedAt != nil {
			sess.CompletedAt = *completedAt
		}
		if len(stepsJSON) > 0 {
			json.Unmarshal(stepsJSON, &sess.Steps)
		}
		out = append(out, &sess)
	}
	if out == nil {
		out = make([]*Session, 0)
	}
	return out
}

// --- Fund ---

func (s *PostgresStore) SaveFund(f *Fund) error {
	ctx := context.Background()
	if f.ID == "" {
		f.ID = generateID()
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now().UTC()
	}
	f.UpdatedAt = time.Now().UTC()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO funds (id, name, business_id, type, min_investment, total_raised, investor_count, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			business_id = EXCLUDED.business_id,
			type = EXCLUDED.type,
			min_investment = EXCLUDED.min_investment,
			total_raised = EXCLUDED.total_raised,
			investor_count = EXCLUDED.investor_count,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at
	`, f.ID, f.Name, f.BusinessID, f.Type, f.MinInvestment, f.TotalRaised, f.InvestorCount, f.Status, f.CreatedAt, f.UpdatedAt)
	return err
}

func (s *PostgresStore) GetFund(id string) (*Fund, error) {
	ctx := context.Background()
	var f Fund
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, business_id, type, min_investment, total_raised, investor_count, status, created_at, updated_at
		FROM funds WHERE id = $1
	`, id).Scan(&f.ID, &f.Name, &f.BusinessID, &f.Type, &f.MinInvestment, &f.TotalRaised, &f.InvestorCount, &f.Status, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("fund not found")
		}
		return nil, err
	}
	return &f, nil
}

func (s *PostgresStore) ListFunds() []*Fund {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, business_id, type, min_investment, total_raised, investor_count, status, created_at, updated_at
		FROM funds ORDER BY created_at DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Fund
	for rows.Next() {
		var f Fund
		if err := rows.Scan(&f.ID, &f.Name, &f.BusinessID, &f.Type, &f.MinInvestment, &f.TotalRaised, &f.InvestorCount, &f.Status, &f.CreatedAt, &f.UpdatedAt); err != nil {
			continue
		}
		out = append(out, &f)
	}
	if out == nil {
		out = make([]*Fund, 0)
	}
	return out
}

func (s *PostgresStore) DeleteFund(id string) error {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx, `DELETE FROM funds WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("fund not found")
	}
	return nil
}

func (s *PostgresStore) AddFundInvestor(inv *FundInvestor) error {
	ctx := context.Background()
	if inv.ID == "" {
		inv.ID = generateID()
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}

	// Verify fund exists.
	var fundExists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM funds WHERE id = $1)`, inv.FundID).Scan(&fundExists)
	if err != nil {
		return err
	}
	if !fundExists {
		return fmt.Errorf("fund not found")
	}

	// Insert investor and update fund totals in a transaction.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO fund_investors (id, fund_id, investor_id, name, email, amount, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, inv.ID, inv.FundID, inv.InvestorID, inv.Name, inv.Email, inv.Amount, inv.Status, inv.CreatedAt)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		UPDATE funds SET
			total_raised = total_raised + $1,
			investor_count = (SELECT COUNT(*) FROM fund_investors WHERE fund_id = $2),
			updated_at = now()
		WHERE id = $2
	`, inv.Amount, inv.FundID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *PostgresStore) ListFundInvestors(fundID string) []*FundInvestor {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, fund_id, investor_id, name, email, amount, status, created_at
		FROM fund_investors WHERE fund_id = $1 ORDER BY created_at DESC
	`, fundID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*FundInvestor
	for rows.Next() {
		var inv FundInvestor
		if err := rows.Scan(&inv.ID, &inv.FundID, &inv.InvestorID, &inv.Name, &inv.Email, &inv.Amount, &inv.Status, &inv.CreatedAt); err != nil {
			continue
		}
		out = append(out, &inv)
	}
	return out
}

// --- Envelope ---

func (s *PostgresStore) SaveEnvelope(env *Envelope) error {
	ctx := context.Background()
	if env.ID == "" {
		env.ID = generateID()
	}
	if env.CreatedAt.IsZero() {
		env.CreatedAt = time.Now().UTC()
	}
	env.UpdatedAt = time.Now().UTC()

	signersJSON, err := json.Marshal(env.Signers)
	if err != nil {
		return fmt.Errorf("marshal signers: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO envelopes (id, template_id, subject, message, status, signers, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			template_id = EXCLUDED.template_id,
			subject = EXCLUDED.subject,
			message = EXCLUDED.message,
			status = EXCLUDED.status,
			signers = EXCLUDED.signers,
			updated_at = EXCLUDED.updated_at
	`, env.ID, env.TemplateID, env.Subject, env.Message, string(env.Status), signersJSON, env.CreatedAt, env.UpdatedAt)
	return err
}

func (s *PostgresStore) GetEnvelope(id string) (*Envelope, error) {
	ctx := context.Background()
	var env Envelope
	var signersJSON []byte
	var status string

	err := s.pool.QueryRow(ctx, `
		SELECT id, template_id, subject, message, status, signers, created_at, updated_at
		FROM envelopes WHERE id = $1
	`, id).Scan(&env.ID, &env.TemplateID, &env.Subject, &env.Message, &status, &signersJSON, &env.CreatedAt, &env.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("envelope not found")
		}
		return nil, err
	}
	env.Status = EnvelopeStatus(status)
	if len(signersJSON) > 0 {
		json.Unmarshal(signersJSON, &env.Signers)
	}
	return &env, nil
}

func (s *PostgresStore) ListEnvelopes() []*Envelope {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, template_id, subject, message, status, signers, created_at, updated_at
		FROM envelopes ORDER BY created_at DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Envelope
	for rows.Next() {
		var env Envelope
		var signersJSON []byte
		var status string
		if err := rows.Scan(&env.ID, &env.TemplateID, &env.Subject, &env.Message, &status, &signersJSON, &env.CreatedAt, &env.UpdatedAt); err != nil {
			continue
		}
		env.Status = EnvelopeStatus(status)
		if len(signersJSON) > 0 {
			json.Unmarshal(signersJSON, &env.Signers)
		}
		out = append(out, &env)
	}
	if out == nil {
		out = make([]*Envelope, 0)
	}
	return out
}

// --- Template ---

func (s *PostgresStore) SaveTemplate(t *Template) error {
	ctx := context.Background()
	if t.ID == "" {
		t.ID = generateID()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}

	rolesJSON, err := json.Marshal(t.Roles)
	if err != nil {
		return fmt.Errorf("marshal template roles: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO templates (id, name, description, content, roles, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			content = EXCLUDED.content,
			roles = EXCLUDED.roles
	`, t.ID, t.Name, t.Description, t.Content, rolesJSON, t.CreatedAt)
	return err
}

func (s *PostgresStore) GetTemplate(id string) (*Template, error) {
	ctx := context.Background()
	var t Template
	var rolesJSON []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, content, roles, created_at
		FROM templates WHERE id = $1
	`, id).Scan(&t.ID, &t.Name, &t.Description, &t.Content, &rolesJSON, &t.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("template not found")
		}
		return nil, err
	}
	if len(rolesJSON) > 0 {
		json.Unmarshal(rolesJSON, &t.Roles)
	}
	return &t, nil
}

func (s *PostgresStore) ListTemplates() []*Template {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, content, roles, created_at
		FROM templates ORDER BY created_at DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Template
	for rows.Next() {
		var t Template
		var rolesJSON []byte
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Content, &rolesJSON, &t.CreatedAt); err != nil {
			continue
		}
		if len(rolesJSON) > 0 {
			json.Unmarshal(rolesJSON, &t.Roles)
		}
		out = append(out, &t)
	}
	if out == nil {
		out = make([]*Template, 0)
	}
	return out
}

// --- Role ---

func (s *PostgresStore) SaveRole(role *Role) error {
	ctx := context.Background()
	if role.ID == "" {
		role.ID = generateID()
	}
	if role.CreatedAt.IsZero() {
		role.CreatedAt = time.Now().UTC()
	}
	role.UpdatedAt = time.Now().UTC()

	permsJSON, err := json.Marshal(role.Permissions)
	if err != nil {
		return fmt.Errorf("marshal permissions: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO roles (id, name, description, permissions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			permissions = EXCLUDED.permissions,
			updated_at = EXCLUDED.updated_at
	`, role.ID, role.Name, role.Description, permsJSON, role.CreatedAt, role.UpdatedAt)
	return err
}

func (s *PostgresStore) GetRole(id string) (*Role, error) {
	ctx := context.Background()
	var role Role
	var permsJSON []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, permissions, created_at, updated_at
		FROM roles WHERE id = $1
	`, id).Scan(&role.ID, &role.Name, &role.Description, &permsJSON, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("role not found")
		}
		return nil, err
	}
	if len(permsJSON) > 0 {
		json.Unmarshal(permsJSON, &role.Permissions)
	}
	return &role, nil
}

func (s *PostgresStore) GetRoleByName(name string) (*Role, error) {
	ctx := context.Background()
	var role Role
	var permsJSON []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, permissions, created_at, updated_at
		FROM roles WHERE name = $1
	`, name).Scan(&role.ID, &role.Name, &role.Description, &permsJSON, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("role not found: %s", name)
		}
		return nil, err
	}
	if len(permsJSON) > 0 {
		json.Unmarshal(permsJSON, &role.Permissions)
	}
	return &role, nil
}

func (s *PostgresStore) ListRoles() []*Role {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, description, permissions, created_at, updated_at
		FROM roles ORDER BY created_at DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Role
	for rows.Next() {
		var role Role
		var permsJSON []byte
		if err := rows.Scan(&role.ID, &role.Name, &role.Description, &permsJSON, &role.CreatedAt, &role.UpdatedAt); err != nil {
			continue
		}
		if len(permsJSON) > 0 {
			json.Unmarshal(permsJSON, &role.Permissions)
		}
		out = append(out, &role)
	}
	if out == nil {
		out = make([]*Role, 0)
	}
	return out
}

func (s *PostgresStore) DeleteRole(id string) error {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx, `DELETE FROM roles WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("role not found")
	}
	return nil
}

// --- User ---

func (s *PostgresStore) SaveUser(u *User) error {
	ctx := context.Background()
	if u.ID == "" {
		u.ID = generateID()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO compliance_users (id, name, email, role, status, last_login, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			email = EXCLUDED.email,
			role = EXCLUDED.role,
			status = EXCLUDED.status,
			last_login = EXCLUDED.last_login
	`, u.ID, u.Name, u.Email, u.Role, u.Status, u.LastLogin, u.CreatedAt)
	return err
}

func (s *PostgresStore) GetUser(id string) (*User, error) {
	ctx := context.Background()
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, email, role, status, last_login, created_at
		FROM compliance_users WHERE id = $1
	`, id).Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Status, &u.LastLogin, &u.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}
	return &u, nil
}

func (s *PostgresStore) ListUsers() []*User {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, email, role, status, last_login, created_at
		FROM compliance_users ORDER BY created_at DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Status, &u.LastLogin, &u.CreatedAt); err != nil {
			continue
		}
		out = append(out, &u)
	}
	if out == nil {
		out = make([]*User, 0)
	}
	return out
}

// --- Transaction ---

func (s *PostgresStore) SaveTransaction(tx *Transaction) error {
	ctx := context.Background()
	if tx.ID == "" {
		tx.ID = generateID()
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO transactions (id, type, asset, amount, fee, status, date)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			type = EXCLUDED.type,
			asset = EXCLUDED.asset,
			amount = EXCLUDED.amount,
			fee = EXCLUDED.fee,
			status = EXCLUDED.status,
			date = EXCLUDED.date
	`, tx.ID, tx.Type, tx.Asset, tx.Amount, tx.Fee, tx.Status, tx.Date)
	return err
}

func (s *PostgresStore) ListTransactions() []*Transaction {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, type, asset, amount, fee, status, date
		FROM transactions ORDER BY date DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Transaction
	for rows.Next() {
		var tx Transaction
		if err := rows.Scan(&tx.ID, &tx.Type, &tx.Asset, &tx.Amount, &tx.Fee, &tx.Status, &tx.Date); err != nil {
			continue
		}
		out = append(out, &tx)
	}
	if out == nil {
		out = make([]*Transaction, 0)
	}
	return out
}

// --- Credential ---

func (s *PostgresStore) SaveCredential(c *Credential) error {
	ctx := context.Background()
	if c.ID == "" {
		c.ID = generateID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	permsJSON, err := json.Marshal(c.Permissions)
	if err != nil {
		return fmt.Errorf("marshal credential permissions: %w", err)
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO credentials (id, name, key_prefix, permissions, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			key_prefix = EXCLUDED.key_prefix,
			permissions = EXCLUDED.permissions,
			expires_at = EXCLUDED.expires_at
	`, c.ID, c.Name, c.KeyPrefix, permsJSON, c.CreatedAt, c.ExpiresAt)
	return err
}

func (s *PostgresStore) ListCredentials() []*Credential {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, key_prefix, permissions, created_at, expires_at
		FROM credentials ORDER BY created_at DESC
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []*Credential
	for rows.Next() {
		var c Credential
		var permsJSON []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.KeyPrefix, &permsJSON, &c.CreatedAt, &c.ExpiresAt); err != nil {
			continue
		}
		if len(permsJSON) > 0 {
			json.Unmarshal(permsJSON, &c.Permissions)
		}
		out = append(out, &c)
	}
	if out == nil {
		out = make([]*Credential, 0)
	}
	return out
}

func (s *PostgresStore) DeleteCredential(id string) error {
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx, `DELETE FROM credentials WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("credential not found")
	}
	return nil
}

// --- Settings ---

func (s *PostgresStore) GetSettings() *Settings {
	ctx := context.Background()
	var settings Settings
	err := s.pool.QueryRow(ctx, `
		SELECT business_name, timezone, currency, notification_email
		FROM settings WHERE id = 1
	`).Scan(&settings.BusinessName, &settings.Timezone, &settings.Currency, &settings.NotificationEmail)
	if err != nil {
		return &Settings{
			BusinessName:      "Your Company",
			Timezone:          "America/New_York",
			Currency:          "USD",
			NotificationEmail: "compliance@example.com",
		}
	}
	return &settings
}

func (s *PostgresStore) SaveSettings(settings *Settings) {
	ctx := context.Background()
	s.pool.Exec(ctx, `
		INSERT INTO settings (id, business_name, timezone, currency, notification_email)
		VALUES (1, $1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET
			business_name = EXCLUDED.business_name,
			timezone = EXCLUDED.timezone,
			currency = EXCLUDED.currency,
			notification_email = EXCLUDED.notification_email
	`, settings.BusinessName, settings.Timezone, settings.Currency, settings.NotificationEmail)
}

// --- Dashboard ---

func (s *PostgresStore) ComputeDashboard() *DashboardStats {
	ctx := context.Background()
	stats := &DashboardStats{}

	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE status IN ('pending', 'in_progress')`).Scan(&stats.ActiveSessions)
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE kyc_status = 'pending'`).Scan(&stats.PendingKYC)
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM funds`).Scan(&stats.TotalFunds)
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM transactions`).Scan(&stats.MonthlyTransactions)

	// Recent sessions (up to 5).
	stats.RecentSessions = make([]*Session, 0, 5)
	rows, err := s.pool.Query(ctx, `
		SELECT id, pipeline_id, investor_email, investor_name, status, kyc_status, steps, created_at, completed_at
		FROM sessions ORDER BY created_at DESC LIMIT 5
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sess Session
			var stepsJSON []byte
			var status, kycStatus string
			var completedAt *time.Time
			if err := rows.Scan(&sess.ID, &sess.PipelineID, &sess.InvestorEmail, &sess.InvestorName,
				&status, &kycStatus, &stepsJSON, &sess.CreatedAt, &completedAt); err != nil {
				continue
			}
			sess.Status = SessionStatus(status)
			sess.KYCStatus = KYCStatus(kycStatus)
			if completedAt != nil {
				sess.CompletedAt = *completedAt
			}
			if len(stepsJSON) > 0 {
				json.Unmarshal(stepsJSON, &sess.Steps)
			}
			stats.RecentSessions = append(stats.RecentSessions, &sess)
		}
	}

	// Recent transactions (up to 5).
	stats.RecentTransactions = make([]*Transaction, 0, 5)
	rows2, err := s.pool.Query(ctx, `
		SELECT id, type, asset, amount, fee, status, date
		FROM transactions ORDER BY date DESC LIMIT 5
	`)
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var tx Transaction
			if err := rows2.Scan(&tx.ID, &tx.Type, &tx.Asset, &tx.Amount, &tx.Fee, &tx.Status, &tx.Date); err != nil {
				continue
			}
			stats.RecentTransactions = append(stats.RecentTransactions, &tx)
		}
	}

	return stats
}

func (s *PostgresStore) ComputeESignStats() *ESignStats {
	ctx := context.Background()
	stats := &ESignStats{}

	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM envelopes WHERE status IN ('pending', 'sent', 'viewed')`).Scan(&stats.Pending)
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM envelopes WHERE status IN ('completed', 'signed')`).Scan(&stats.Completed)
	stats.Draft = stats.Pending
	s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM templates`).Scan(&stats.Templates)

	stats.Recent = make([]*Envelope, 0, 5)
	rows, err := s.pool.Query(ctx, `
		SELECT id, template_id, subject, message, status, signers, created_at, updated_at
		FROM envelopes ORDER BY created_at DESC LIMIT 5
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var env Envelope
			var signersJSON []byte
			var status string
			if err := rows.Scan(&env.ID, &env.TemplateID, &env.Subject, &env.Message, &status, &signersJSON, &env.CreatedAt, &env.UpdatedAt); err != nil {
				continue
			}
			env.Status = EnvelopeStatus(status)
			if len(signersJSON) > 0 {
				json.Unmarshal(signersJSON, &env.Signers)
			}
			stats.Recent = append(stats.Recent, &env)
		}
	}

	return stats
}

func (s *PostgresStore) ListEnvelopesByDirection(direction string) []*Envelope {
	ctx := context.Background()
	var query string
	switch direction {
	case "inbox":
		query = `SELECT id, template_id, subject, message, status, signers, created_at, updated_at
			FROM envelopes WHERE status IN ('sent', 'pending', 'viewed') ORDER BY created_at DESC`
	case "sent":
		query = `SELECT id, template_id, subject, message, status, signers, created_at, updated_at
			FROM envelopes WHERE status IN ('signed', 'completed', 'declined', 'voided') ORDER BY created_at DESC`
	default:
		return make([]*Envelope, 0)
	}

	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return make([]*Envelope, 0)
	}
	defer rows.Close()

	var out []*Envelope
	for rows.Next() {
		var env Envelope
		var signersJSON []byte
		var status string
		if err := rows.Scan(&env.ID, &env.TemplateID, &env.Subject, &env.Message, &status, &signersJSON, &env.CreatedAt, &env.UpdatedAt); err != nil {
			continue
		}
		env.Status = EnvelopeStatus(status)
		if len(signersJSON) > 0 {
			json.Unmarshal(signersJSON, &env.Signers)
		}
		out = append(out, &env)
	}
	if out == nil {
		out = make([]*Envelope, 0)
	}
	return out
}

// --- AML Screening ---

func (s *PostgresStore) SaveAMLScreening(sc *AMLScreening) error {
	ctx := context.Background()
	if sc.ID == "" {
		sc.ID = generateID()
	}
	if sc.CreatedAt.IsZero() {
		sc.CreatedAt = time.Now().UTC()
	}
	sc.UpdatedAt = time.Now().UTC()

	var reviewedAt *time.Time
	if !sc.ReviewedAt.IsZero() {
		reviewedAt = &sc.ReviewedAt
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO aml_screenings (id, account_id, user_id, type, status, risk_level, risk_score,
			provider, details, sanctions_hit, pep_match, reviewed_by, reviewed_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			risk_level = EXCLUDED.risk_level,
			risk_score = EXCLUDED.risk_score,
			details = EXCLUDED.details,
			reviewed_by = EXCLUDED.reviewed_by,
			reviewed_at = EXCLUDED.reviewed_at,
			updated_at = EXCLUDED.updated_at
	`, sc.ID, sc.AccountID, sc.UserID, sc.Type, string(sc.Status), string(sc.RiskLevel),
		sc.RiskScore, sc.Provider, sc.Details, sc.SanctionsHit, sc.PEPMatch,
		sc.ReviewedBy, reviewedAt, sc.CreatedAt, sc.UpdatedAt)
	return err
}

func (s *PostgresStore) GetAMLScreening(id string) (*AMLScreening, error) {
	ctx := context.Background()
	var sc AMLScreening
	var status, riskLevel string
	var reviewedAt *time.Time

	err := s.pool.QueryRow(ctx, `
		SELECT id, account_id, user_id, type, status, risk_level, risk_score,
			provider, details, sanctions_hit, pep_match, reviewed_by, reviewed_at, created_at, updated_at
		FROM aml_screenings WHERE id = $1
	`, id).Scan(&sc.ID, &sc.AccountID, &sc.UserID, &sc.Type, &status, &riskLevel,
		&sc.RiskScore, &sc.Provider, &sc.Details, &sc.SanctionsHit, &sc.PEPMatch,
		&sc.ReviewedBy, &reviewedAt, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("aml screening not found")
		}
		return nil, err
	}
	sc.Status = AMLStatus(status)
	sc.RiskLevel = RiskLevel(riskLevel)
	if reviewedAt != nil {
		sc.ReviewedAt = *reviewedAt
	}
	return &sc, nil
}

func (s *PostgresStore) ListAMLScreeningsByAccount(accountID string) []*AMLScreening {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, user_id, type, status, risk_level, risk_score,
			provider, details, sanctions_hit, pep_match, reviewed_by, reviewed_at, created_at, updated_at
		FROM aml_screenings WHERE account_id = $1 ORDER BY created_at DESC
	`, accountID)
	if err != nil {
		return make([]*AMLScreening, 0)
	}
	defer rows.Close()
	return scanAMLScreenings(rows)
}

func (s *PostgresStore) ListAMLScreeningsByStatus(status AMLStatus) []*AMLScreening {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, user_id, type, status, risk_level, risk_score,
			provider, details, sanctions_hit, pep_match, reviewed_by, reviewed_at, created_at, updated_at
		FROM aml_screenings WHERE status = $1 ORDER BY created_at DESC
	`, string(status))
	if err != nil {
		return make([]*AMLScreening, 0)
	}
	defer rows.Close()
	return scanAMLScreenings(rows)
}

func scanAMLScreenings(rows pgx.Rows) []*AMLScreening {
	var out []*AMLScreening
	for rows.Next() {
		var sc AMLScreening
		var status, riskLevel string
		var reviewedAt *time.Time
		if err := rows.Scan(&sc.ID, &sc.AccountID, &sc.UserID, &sc.Type, &status, &riskLevel,
			&sc.RiskScore, &sc.Provider, &sc.Details, &sc.SanctionsHit, &sc.PEPMatch,
			&sc.ReviewedBy, &reviewedAt, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			continue
		}
		sc.Status = AMLStatus(status)
		sc.RiskLevel = RiskLevel(riskLevel)
		if reviewedAt != nil {
			sc.ReviewedAt = *reviewedAt
		}
		out = append(out, &sc)
	}
	if out == nil {
		out = make([]*AMLScreening, 0)
	}
	return out
}

// --- Application ---

func (s *PostgresStore) SaveApplication(app *Application) error {
	ctx := context.Background()
	if app.ID == "" {
		app.ID = generateID()
	}
	if app.CreatedAt.IsZero() {
		app.CreatedAt = time.Now().UTC()
	}
	app.UpdatedAt = time.Now().UTC()

	stepsJSON, err := json.Marshal(app.Steps)
	if err != nil {
		return fmt.Errorf("marshal application steps: %w", err)
	}
	docsJSON, err := json.Marshal(app.Documents)
	if err != nil {
		return fmt.Errorf("marshal application documents: %w", err)
	}

	var submittedAt, reviewedAt *time.Time
	if !app.SubmittedAt.IsZero() {
		submittedAt = &app.SubmittedAt
	}
	if !app.ReviewedAt.IsZero() {
		reviewedAt = &app.ReviewedAt
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO applications (id, user_id, email, first_name, last_name, phone, date_of_birth,
			ssn_hash, ssn_last4, address_line1, address_line2, city, state, zip_code, country,
			status, current_step, kyc_status, aml_status, steps, documents,
			risk_level, risk_score, submitted_at, reviewed_by, reviewed_at,
			alpaca_account_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29)
		ON CONFLICT (id) DO UPDATE SET
			email = EXCLUDED.email,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name,
			phone = EXCLUDED.phone,
			date_of_birth = EXCLUDED.date_of_birth,
			ssn_hash = EXCLUDED.ssn_hash,
			ssn_last4 = EXCLUDED.ssn_last4,
			address_line1 = EXCLUDED.address_line1,
			address_line2 = EXCLUDED.address_line2,
			city = EXCLUDED.city,
			state = EXCLUDED.state,
			zip_code = EXCLUDED.zip_code,
			country = EXCLUDED.country,
			status = EXCLUDED.status,
			current_step = EXCLUDED.current_step,
			kyc_status = EXCLUDED.kyc_status,
			aml_status = EXCLUDED.aml_status,
			steps = EXCLUDED.steps,
			documents = EXCLUDED.documents,
			risk_level = EXCLUDED.risk_level,
			risk_score = EXCLUDED.risk_score,
			submitted_at = EXCLUDED.submitted_at,
			reviewed_by = EXCLUDED.reviewed_by,
			reviewed_at = EXCLUDED.reviewed_at,
			alpaca_account_id = EXCLUDED.alpaca_account_id,
			updated_at = EXCLUDED.updated_at
	`, app.ID, app.UserID, app.Email, app.FirstName, app.LastName, app.Phone, app.DateOfBirth,
		app.SSNHash, app.SSNLast4, app.AddressLine1, app.AddressLine2, app.City, app.State,
		app.ZipCode, app.Country, string(app.Status), app.CurrentStep,
		string(app.KYCStatus), string(app.AMLStatus), stepsJSON, docsJSON,
		string(app.RiskLevel), app.RiskScore, submittedAt, app.ReviewedBy, reviewedAt,
		app.AlpacaAccountID, app.CreatedAt, app.UpdatedAt)
	return err
}

func (s *PostgresStore) GetApplication(id string) (*Application, error) {
	ctx := context.Background()
	return s.scanApplication(ctx, `
		SELECT id, user_id, email, first_name, last_name, phone, date_of_birth,
			ssn_hash, ssn_last4, address_line1, address_line2, city, state, zip_code, country,
			status, current_step, kyc_status, aml_status, steps, documents,
			risk_level, risk_score, submitted_at, reviewed_by, reviewed_at,
			alpaca_account_id, created_at, updated_at
		FROM applications WHERE id = $1
	`, id)
}

func (s *PostgresStore) GetApplicationByUser(userID string) (*Application, error) {
	ctx := context.Background()
	return s.scanApplication(ctx, `
		SELECT id, user_id, email, first_name, last_name, phone, date_of_birth,
			ssn_hash, ssn_last4, address_line1, address_line2, city, state, zip_code, country,
			status, current_step, kyc_status, aml_status, steps, documents,
			risk_level, risk_score, submitted_at, reviewed_by, reviewed_at,
			alpaca_account_id, created_at, updated_at
		FROM applications WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1
	`, userID)
}

func (s *PostgresStore) scanApplication(ctx context.Context, query string, args ...interface{}) (*Application, error) {
	var app Application
	var status, kycStatus, amlStatus, riskLevel string
	var stepsJSON, docsJSON []byte
	var submittedAt, reviewedAt *time.Time

	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&app.ID, &app.UserID, &app.Email, &app.FirstName, &app.LastName,
		&app.Phone, &app.DateOfBirth, &app.SSNHash, &app.SSNLast4,
		&app.AddressLine1, &app.AddressLine2, &app.City, &app.State,
		&app.ZipCode, &app.Country, &status, &app.CurrentStep,
		&kycStatus, &amlStatus, &stepsJSON, &docsJSON,
		&riskLevel, &app.RiskScore, &submittedAt, &app.ReviewedBy,
		&reviewedAt, &app.AlpacaAccountID, &app.CreatedAt, &app.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("application not found")
		}
		return nil, err
	}
	app.Status = ApplicationStatus(status)
	app.KYCStatus = KYCStatus(kycStatus)
	app.AMLStatus = AMLStatus(amlStatus)
	app.RiskLevel = RiskLevel(riskLevel)
	if submittedAt != nil {
		app.SubmittedAt = *submittedAt
	}
	if reviewedAt != nil {
		app.ReviewedAt = *reviewedAt
	}
	if len(stepsJSON) > 0 {
		json.Unmarshal(stepsJSON, &app.Steps)
	}
	if len(docsJSON) > 0 {
		json.Unmarshal(docsJSON, &app.Documents)
	}
	return &app, nil
}

func (s *PostgresStore) ListApplications() []*Application {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, email, first_name, last_name, phone, date_of_birth,
			ssn_hash, ssn_last4, address_line1, address_line2, city, state, zip_code, country,
			status, current_step, kyc_status, aml_status, steps, documents,
			risk_level, risk_score, submitted_at, reviewed_by, reviewed_at,
			alpaca_account_id, created_at, updated_at
		FROM applications ORDER BY created_at DESC
	`)
	if err != nil {
		return make([]*Application, 0)
	}
	defer rows.Close()
	return s.scanApplicationRows(rows)
}

func (s *PostgresStore) ListApplicationsByStatus(status ApplicationStatus) []*Application {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, email, first_name, last_name, phone, date_of_birth,
			ssn_hash, ssn_last4, address_line1, address_line2, city, state, zip_code, country,
			status, current_step, kyc_status, aml_status, steps, documents,
			risk_level, risk_score, submitted_at, reviewed_by, reviewed_at,
			alpaca_account_id, created_at, updated_at
		FROM applications WHERE status = $1 ORDER BY created_at DESC
	`, string(status))
	if err != nil {
		return make([]*Application, 0)
	}
	defer rows.Close()
	return s.scanApplicationRows(rows)
}

func (s *PostgresStore) scanApplicationRows(rows pgx.Rows) []*Application {
	var out []*Application
	for rows.Next() {
		var app Application
		var status, kycStatus, amlStatus, riskLevel string
		var stepsJSON, docsJSON []byte
		var submittedAt, reviewedAt *time.Time

		if err := rows.Scan(
			&app.ID, &app.UserID, &app.Email, &app.FirstName, &app.LastName,
			&app.Phone, &app.DateOfBirth, &app.SSNHash, &app.SSNLast4,
			&app.AddressLine1, &app.AddressLine2, &app.City, &app.State,
			&app.ZipCode, &app.Country, &status, &app.CurrentStep,
			&kycStatus, &amlStatus, &stepsJSON, &docsJSON,
			&riskLevel, &app.RiskScore, &submittedAt, &app.ReviewedBy,
			&reviewedAt, &app.AlpacaAccountID, &app.CreatedAt, &app.UpdatedAt,
		); err != nil {
			continue
		}
		app.Status = ApplicationStatus(status)
		app.KYCStatus = KYCStatus(kycStatus)
		app.AMLStatus = AMLStatus(amlStatus)
		app.RiskLevel = RiskLevel(riskLevel)
		if submittedAt != nil {
			app.SubmittedAt = *submittedAt
		}
		if reviewedAt != nil {
			app.ReviewedAt = *reviewedAt
		}
		if len(stepsJSON) > 0 {
			json.Unmarshal(stepsJSON, &app.Steps)
		}
		if len(docsJSON) > 0 {
			json.Unmarshal(docsJSON, &app.Documents)
		}
		out = append(out, &app)
	}
	if out == nil {
		out = make([]*Application, 0)
	}
	return out
}

// --- Document Upload ---

func (s *PostgresStore) SaveDocumentUpload(doc *DocumentUpload) error {
	ctx := context.Background()
	if doc.ID == "" {
		doc.ID = generateID()
	}
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	doc.UpdatedAt = time.Now().UTC()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO document_uploads (id, application_id, user_id, type, name, mime_type, size, status, review_note, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			review_note = EXCLUDED.review_note,
			updated_at = EXCLUDED.updated_at
	`, doc.ID, doc.ApplicationID, doc.UserID, doc.Type, doc.Name, doc.MimeType,
		doc.Size, doc.Status, doc.ReviewNote, doc.CreatedAt, doc.UpdatedAt)
	return err
}

func (s *PostgresStore) GetDocumentUpload(id string) (*DocumentUpload, error) {
	ctx := context.Background()
	var doc DocumentUpload

	err := s.pool.QueryRow(ctx, `
		SELECT id, application_id, user_id, type, name, mime_type, size, status, review_note, created_at, updated_at
		FROM document_uploads WHERE id = $1
	`, id).Scan(&doc.ID, &doc.ApplicationID, &doc.UserID, &doc.Type, &doc.Name,
		&doc.MimeType, &doc.Size, &doc.Status, &doc.ReviewNote, &doc.CreatedAt, &doc.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("document not found")
		}
		return nil, err
	}
	return &doc, nil
}

func (s *PostgresStore) ListDocumentUploads(applicationID string) []*DocumentUpload {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, application_id, user_id, type, name, mime_type, size, status, review_note, created_at, updated_at
		FROM document_uploads WHERE application_id = $1 ORDER BY created_at DESC
	`, applicationID)
	if err != nil {
		return make([]*DocumentUpload, 0)
	}
	defer rows.Close()

	var out []*DocumentUpload
	for rows.Next() {
		var doc DocumentUpload
		if err := rows.Scan(&doc.ID, &doc.ApplicationID, &doc.UserID, &doc.Type, &doc.Name,
			&doc.MimeType, &doc.Size, &doc.Status, &doc.ReviewNote, &doc.CreatedAt, &doc.UpdatedAt); err != nil {
			continue
		}
		out = append(out, &doc)
	}
	if out == nil {
		out = make([]*DocumentUpload, 0)
	}
	return out
}

// --- Identity listing ---

func (s *PostgresStore) ListIdentitiesByUser(userID string) []*Identity {
	ctx := context.Background()
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, provider, status, data, created_at, updated_at
		FROM identities WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return make([]*Identity, 0)
	}
	defer rows.Close()

	var out []*Identity
	for rows.Next() {
		var ident Identity
		var status string
		var dataJSON []byte
		if err := rows.Scan(&ident.ID, &ident.UserID, &ident.Provider, &status, &dataJSON, &ident.CreatedAt, &ident.UpdatedAt); err != nil {
			continue
		}
		ident.Status = KYCStatus(status)
		if len(dataJSON) > 0 {
			json.Unmarshal(dataJSON, &ident.Data)
		}
		if ident.Data == nil {
			ident.Data = make(map[string]interface{})
		}
		out = append(out, &ident)
	}
	if out == nil {
		out = make([]*Identity, 0)
	}
	return out
}
