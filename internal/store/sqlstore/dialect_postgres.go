package sqlstore

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type postgresDialect struct{}

func (postgresDialect) name() string { return "postgres" }

func (postgresDialect) rebind(query string) string { return rebindDollar(query) }

func (postgresDialect) usesRowID() bool { return false }

func (postgresDialect) schemaStmts() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS merchants (
			merchant_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			settings_invoice_ttl_seconds BIGINT NOT NULL,
			settings_required_confirmations INTEGER NOT NULL,
			settings_late_payment_policy TEXT NOT NULL,
			settings_partial_payment_policy TEXT NOT NULL,
			settings_overpayment_policy TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS merchant_wallets (
			merchant_id TEXT PRIMARY KEY,
			wallet_id TEXT NOT NULL,
			ufvk TEXT NOT NULL,
			chain TEXT NOT NULL,
			ua_hrp TEXT NOT NULL,
			coin_type INTEGER NOT NULL,
			next_address_index BIGINT NOT NULL,
			created_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			key_id TEXT PRIMARY KEY,
			merchant_id TEXT NOT NULL,
			label TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			revoked_at BIGINT,
			created_at BIGINT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_token_hash ON api_keys(token_hash)`,
		`CREATE TABLE IF NOT EXISTS invoices (
			seq BIGSERIAL PRIMARY KEY,
			invoice_id TEXT NOT NULL UNIQUE,
			merchant_id TEXT NOT NULL,
			external_order_id TEXT NOT NULL,
			external_order_id_hash BYTEA NOT NULL,
			wallet_id TEXT NOT NULL,
			address_index BIGINT NOT NULL,
			address TEXT NOT NULL,
			address_hash BYTEA NOT NULL,
			created_after_height BIGINT NOT NULL,
			created_after_hash TEXT NOT NULL,
			amount_zat BIGINT NOT NULL,
			required_confirmations INTEGER NOT NULL,
			policy_late_payment TEXT NOT NULL,
			policy_partial_payment TEXT NOT NULL,
			policy_overpayment TEXT NOT NULL,
			received_pending_zat BIGINT NOT NULL,
			received_confirmed_zat BIGINT NOT NULL,
			status TEXT NOT NULL,
			expires_at BIGINT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE (merchant_id, external_order_id_hash),
			UNIQUE (wallet_id, address_hash)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_merchant_seq ON invoices(merchant_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_merchant_status_seq ON invoices(merchant_id, status, seq)`,
		`CREATE TABLE IF NOT EXISTS invoice_tokens (
			invoice_id TEXT PRIMARY KEY,
			token_enc BYTEA NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scan_cursors (
			wallet_id TEXT PRIMARY KEY,
			cursor BIGINT NOT NULL,
			last_event_at BIGINT
		)`,
		`CREATE TABLE IF NOT EXISTS deposits (
			seq BIGSERIAL PRIMARY KEY,
			wallet_id TEXT NOT NULL,
			txid TEXT NOT NULL,
			action_index INTEGER NOT NULL,
			recipient_address TEXT NOT NULL,
			recipient_address_hash BYTEA NOT NULL,
			amount_zat BIGINT NOT NULL,
			height BIGINT NOT NULL,
			status TEXT NOT NULL,
			confirmed_height BIGINT,
			invoice_id TEXT,
			detected_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE (wallet_id, txid, action_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_deposits_invoice_status ON deposits(invoice_id, status, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_deposits_wallet_recipient_hash ON deposits(wallet_id, recipient_address_hash)`,
		`CREATE TABLE IF NOT EXISTS refunds (
			id BIGSERIAL PRIMARY KEY,
			refund_id TEXT NOT NULL UNIQUE,
			merchant_id TEXT NOT NULL,
			invoice_id TEXT,
			external_refund_id TEXT,
			to_address TEXT NOT NULL,
			amount_zat BIGINT NOT NULL,
			status TEXT NOT NULL,
			sent_txid TEXT,
			notes TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_refunds_merchant_id ON refunds(merchant_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_refunds_invoice_id ON refunds(invoice_id, id)`,
		`CREATE TABLE IF NOT EXISTS review_cases (
			id BIGSERIAL PRIMARY KEY,
			review_id TEXT NOT NULL UNIQUE,
			merchant_id TEXT NOT NULL,
			invoice_id TEXT,
			reason TEXT NOT NULL,
			status TEXT NOT NULL,
			notes TEXT NOT NULL,
			deposit_wallet_id TEXT,
			deposit_txid TEXT,
			deposit_action_index INTEGER,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_review_cases_merchant_status ON review_cases(merchant_id, status, id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_review_cases_invoice_unique ON review_cases(merchant_id, invoice_id, reason, status)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_review_cases_deposit_unique ON review_cases(deposit_wallet_id, deposit_txid, deposit_action_index, reason)`,
		`CREATE TABLE IF NOT EXISTS invoice_events (
			id BIGSERIAL PRIMARY KEY,
			invoice_id TEXT NOT NULL,
			type TEXT NOT NULL,
			occurred_at BIGINT NOT NULL,
			deposit_wallet_id TEXT,
			deposit_txid TEXT,
			deposit_action_index INTEGER,
			deposit_amount_zat BIGINT,
			deposit_height BIGINT,
			refund_id TEXT,
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_invoice_events_invoice_id ON invoice_events(invoice_id, id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_invoice_events_deposit_unique
		 ON invoice_events(invoice_id, type, deposit_wallet_id, deposit_txid, deposit_action_index)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_invoice_events_refund_unique
		 ON invoice_events(invoice_id, type, refund_id)`,
		`CREATE TABLE IF NOT EXISTS event_sinks (
			sink_id TEXT PRIMARY KEY,
			merchant_id TEXT NOT NULL,
			kind TEXT NOT NULL,
			status TEXT NOT NULL,
			config_json BYTEA NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_sinks_merchant_id ON event_sinks(merchant_id)`,
		`CREATE TABLE IF NOT EXISTS outbox_events (
			id BIGSERIAL PRIMARY KEY,
			event_id TEXT NOT NULL UNIQUE,
			merchant_id TEXT NOT NULL,
			envelope_json BYTEA NOT NULL,
			created_at BIGINT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_outbox_events_merchant_id ON outbox_events(merchant_id, id)`,
		`CREATE TABLE IF NOT EXISTS event_deliveries (
			delivery_id TEXT PRIMARY KEY,
			merchant_id TEXT NOT NULL,
			sink_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			status TEXT NOT NULL,
			attempt INTEGER NOT NULL,
			next_retry_at BIGINT,
			last_error TEXT,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE(sink_id, event_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_merchant_status_retry ON event_deliveries(merchant_id, status, next_retry_at)`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_sink_id ON event_deliveries(sink_id)`,
	}
}

func (postgresDialect) isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key")
}

func (postgresDialect) isAlreadyExists(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P07" || pgErr.Code == "42710"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate")
}
