package sqlstore

import "strings"

type sqliteDialect struct{}

func (sqliteDialect) name() string { return "sqlite" }

func (sqliteDialect) rebind(query string) string { return query }

func (sqliteDialect) usesRowID() bool { return true }

func (sqliteDialect) schemaStmts() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS merchants (
			merchant_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			settings_invoice_ttl_seconds INTEGER NOT NULL,
			settings_required_confirmations INTEGER NOT NULL,
			settings_late_payment_policy TEXT NOT NULL,
			settings_partial_payment_policy TEXT NOT NULL,
			settings_overpayment_policy TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS merchant_wallets (
			merchant_id TEXT PRIMARY KEY,
			wallet_id TEXT NOT NULL,
			ufvk TEXT NOT NULL,
			chain TEXT NOT NULL,
			ua_hrp TEXT NOT NULL,
			coin_type INTEGER NOT NULL,
			next_address_index INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			key_id TEXT PRIMARY KEY,
			merchant_id TEXT NOT NULL,
			label TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			revoked_at INTEGER,
			created_at INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_token_hash ON api_keys(token_hash)`,
		`CREATE TABLE IF NOT EXISTS invoices (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_id TEXT NOT NULL UNIQUE,
			merchant_id TEXT NOT NULL,
			external_order_id TEXT NOT NULL,
			external_order_id_hash BLOB NOT NULL,
			wallet_id TEXT NOT NULL,
			address_index INTEGER NOT NULL,
			address TEXT NOT NULL,
			address_hash BLOB NOT NULL,
			created_after_height INTEGER NOT NULL,
			created_after_hash TEXT NOT NULL,
			amount_zat INTEGER NOT NULL,
			required_confirmations INTEGER NOT NULL,
			policy_late_payment TEXT NOT NULL,
			policy_partial_payment TEXT NOT NULL,
			policy_overpayment TEXT NOT NULL,
			received_pending_zat INTEGER NOT NULL,
			received_confirmed_zat INTEGER NOT NULL,
			status TEXT NOT NULL,
			expires_at INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE (merchant_id, external_order_id_hash),
			UNIQUE (wallet_id, address_hash)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_merchant_seq ON invoices(merchant_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_invoices_merchant_status_seq ON invoices(merchant_id, status, seq)`,
		`CREATE TABLE IF NOT EXISTS invoice_tokens (
			invoice_id TEXT PRIMARY KEY,
			token_enc BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scan_cursors (
			wallet_id TEXT PRIMARY KEY,
			cursor_id INTEGER NOT NULL,
			last_event_at INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS deposits (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			wallet_id TEXT NOT NULL,
			txid TEXT NOT NULL,
			action_index INTEGER NOT NULL,
			recipient_address TEXT NOT NULL,
			recipient_address_hash BLOB NOT NULL,
			amount_zat INTEGER NOT NULL,
			height INTEGER NOT NULL,
			status TEXT NOT NULL,
			confirmed_height INTEGER,
			invoice_id TEXT,
			detected_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE (wallet_id, txid, action_index)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_deposits_invoice_status ON deposits(invoice_id, status, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_deposits_wallet_recipient_hash ON deposits(wallet_id, recipient_address_hash)`,
		`CREATE TABLE IF NOT EXISTS refunds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			refund_id TEXT NOT NULL UNIQUE,
			merchant_id TEXT NOT NULL,
			invoice_id TEXT,
			external_refund_id TEXT,
			to_address TEXT NOT NULL,
			amount_zat INTEGER NOT NULL,
			status TEXT NOT NULL,
			sent_txid TEXT,
			notes TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_refunds_merchant_id ON refunds(merchant_id, id)`,
		`CREATE INDEX IF NOT EXISTS idx_refunds_invoice_id ON refunds(invoice_id, id)`,
		`CREATE TABLE IF NOT EXISTS review_cases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			review_id TEXT NOT NULL UNIQUE,
			merchant_id TEXT NOT NULL,
			invoice_id TEXT,
			reason TEXT NOT NULL,
			status TEXT NOT NULL,
			notes TEXT NOT NULL,
			deposit_wallet_id TEXT,
			deposit_txid TEXT,
			deposit_action_index INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_review_cases_merchant_status ON review_cases(merchant_id, status, id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_review_cases_invoice_unique ON review_cases(merchant_id, invoice_id, reason, status)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_review_cases_deposit_unique ON review_cases(deposit_wallet_id, deposit_txid, deposit_action_index, reason)`,
		`CREATE TABLE IF NOT EXISTS invoice_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_id TEXT NOT NULL,
			type TEXT NOT NULL,
			occurred_at INTEGER NOT NULL,
			deposit_wallet_id TEXT,
			deposit_txid TEXT,
			deposit_action_index INTEGER,
			deposit_amount_zat INTEGER,
			deposit_height INTEGER,
			refund_id TEXT,
			created_at INTEGER NOT NULL
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
			config_json BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_sinks_merchant_id ON event_sinks(merchant_id)`,
		`CREATE TABLE IF NOT EXISTS outbox_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL UNIQUE,
			merchant_id TEXT NOT NULL,
			envelope_json BLOB NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_outbox_events_merchant_id ON outbox_events(merchant_id, id)`,
		`CREATE TABLE IF NOT EXISTS event_deliveries (
			delivery_id TEXT PRIMARY KEY,
			merchant_id TEXT NOT NULL,
			sink_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			status TEXT NOT NULL,
			attempt INTEGER NOT NULL,
			next_retry_at INTEGER,
			last_error TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(sink_id, event_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_merchant_status_retry ON event_deliveries(merchant_id, status, next_retry_at)`,
		`CREATE INDEX IF NOT EXISTS idx_event_deliveries_sink_id ON event_deliveries(sink_id)`,
	}
}

func (sqliteDialect) isUniqueViolation(err error) bool { return isUniqueViolation(err) }

func (sqliteDialect) isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "already exists")
}
