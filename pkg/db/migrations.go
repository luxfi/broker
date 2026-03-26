package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migration is a single schema migration with a name and SQL statement.
type migration struct {
	name string
	sql  string
}

// migrations is the ordered list of schema migrations.
// Each runs inside a transaction and is tracked in the schema_migrations table.
var migrations = []migration{
	{
		name: "001_create_identities",
		sql: `CREATE TABLE IF NOT EXISTS identities (
			id          TEXT PRIMARY KEY,
			user_id     TEXT NOT NULL,
			provider    TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'pending',
			data        JSONB DEFAULT '{}',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "002_create_pipelines",
		sql: `CREATE TABLE IF NOT EXISTS pipelines (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			business_id TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'draft',
			steps       JSONB DEFAULT '[]',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "003_create_sessions",
		sql: `CREATE TABLE IF NOT EXISTS sessions (
			id              TEXT PRIMARY KEY,
			pipeline_id     TEXT NOT NULL DEFAULT '',
			investor_email  TEXT NOT NULL DEFAULT '',
			investor_name   TEXT NOT NULL DEFAULT '',
			status          TEXT NOT NULL DEFAULT 'pending',
			kyc_status      TEXT NOT NULL DEFAULT 'pending',
			steps           JSONB DEFAULT '[]',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			completed_at    TIMESTAMPTZ
		)`,
	},
	{
		name: "004_create_funds",
		sql: `CREATE TABLE IF NOT EXISTS funds (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL,
			business_id     TEXT NOT NULL DEFAULT '',
			type            TEXT NOT NULL DEFAULT '',
			min_investment  DOUBLE PRECISION NOT NULL DEFAULT 0,
			total_raised    DOUBLE PRECISION NOT NULL DEFAULT 0,
			investor_count  INTEGER NOT NULL DEFAULT 0,
			status          TEXT NOT NULL DEFAULT 'raising',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "005_create_fund_investors",
		sql: `CREATE TABLE IF NOT EXISTS fund_investors (
			id          TEXT PRIMARY KEY,
			fund_id     TEXT NOT NULL REFERENCES funds(id) ON DELETE CASCADE,
			investor_id TEXT NOT NULL DEFAULT '',
			name        TEXT NOT NULL DEFAULT '',
			email       TEXT NOT NULL DEFAULT '',
			amount      DOUBLE PRECISION NOT NULL DEFAULT 0,
			status      TEXT NOT NULL DEFAULT 'committed',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "006_create_envelopes",
		sql: `CREATE TABLE IF NOT EXISTS envelopes (
			id          TEXT PRIMARY KEY,
			template_id TEXT NOT NULL DEFAULT '',
			subject     TEXT NOT NULL DEFAULT '',
			message     TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'pending',
			signers     JSONB DEFAULT '[]',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "007_create_templates",
		sql: `CREATE TABLE IF NOT EXISTS templates (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			content     TEXT NOT NULL DEFAULT '',
			roles       JSONB DEFAULT '[]',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "008_create_roles",
		sql: `CREATE TABLE IF NOT EXISTS roles (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			permissions JSONB DEFAULT '[]',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "009_create_users",
		sql: `CREATE TABLE IF NOT EXISTS compliance_users (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			email      TEXT NOT NULL DEFAULT '',
			role       TEXT NOT NULL DEFAULT 'agent',
			status     TEXT NOT NULL DEFAULT 'active',
			last_login TIMESTAMPTZ NOT NULL DEFAULT now(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "010_create_transactions",
		sql: `CREATE TABLE IF NOT EXISTS transactions (
			id     TEXT PRIMARY KEY,
			type   TEXT NOT NULL DEFAULT '',
			asset  TEXT NOT NULL DEFAULT '',
			amount DOUBLE PRECISION NOT NULL DEFAULT 0,
			fee    DOUBLE PRECISION NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			date   TEXT NOT NULL DEFAULT ''
		)`,
	},
	{
		name: "011_create_credentials",
		sql: `CREATE TABLE IF NOT EXISTS credentials (
			id          TEXT PRIMARY KEY,
			name        TEXT NOT NULL,
			key_prefix  TEXT NOT NULL DEFAULT '',
			permissions JSONB DEFAULT '[]',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			expires_at  TEXT NOT NULL DEFAULT ''
		)`,
	},
	{
		name: "012_create_settings",
		sql: `CREATE TABLE IF NOT EXISTS settings (
			id                 INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
			business_name      TEXT NOT NULL DEFAULT '',
			timezone           TEXT NOT NULL DEFAULT 'America/New_York',
			currency           TEXT NOT NULL DEFAULT 'USD',
			notification_email TEXT NOT NULL DEFAULT ''
		)`,
	},
	{
		name: "013_create_audit_events",
		sql: `CREATE TABLE IF NOT EXISTS audit_events (
			id          TEXT PRIMARY KEY,
			actor       TEXT NOT NULL DEFAULT '',
			action      TEXT NOT NULL DEFAULT '',
			resource    TEXT NOT NULL DEFAULT '',
			resource_id TEXT NOT NULL DEFAULT '',
			details     JSONB DEFAULT '{}',
			timestamp   TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
	},
	{
		name: "014_seed_default_settings",
		sql: `INSERT INTO settings (id, business_name, timezone, currency, notification_email)
			VALUES (1, 'Your Company', 'America/New_York', 'USD', 'compliance@example.com')
			ON CONFLICT (id) DO NOTHING`,
	},
	{
		name: "015_create_indexes",
		sql: `CREATE INDEX IF NOT EXISTS idx_identities_user_id ON identities(user_id);
			CREATE INDEX IF NOT EXISTS idx_sessions_pipeline_id ON sessions(pipeline_id);
			CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
			CREATE INDEX IF NOT EXISTS idx_funds_business_id ON funds(business_id);
			CREATE INDEX IF NOT EXISTS idx_fund_investors_fund_id ON fund_investors(fund_id);
			CREATE INDEX IF NOT EXISTS idx_envelopes_status ON envelopes(status);
			CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp ON audit_events(timestamp);
			CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events(actor)`,
	},
}

// RunMigrations creates the schema_migrations tracking table and runs all
// pending migrations in order. Each migration runs in its own transaction.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// Create the tracking table.
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		// Check if already applied.
		var exists bool
		err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name = $1)`, m.name).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", m.name, err)
		}
		if exists {
			continue
		}

		// Run migration in a transaction.
		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin transaction for %s: %w", m.name, err)
		}

		if _, err := tx.Exec(ctx, m.sql); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("run migration %s: %w", m.name, err)
		}

		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (name) VALUES ($1)`, m.name); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", m.name, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.name, err)
		}
	}

	return nil
}
