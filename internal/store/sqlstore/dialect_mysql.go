package sqlstore

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type mysqlDialect struct{}

func (mysqlDialect) name() string { return "mysql" }

func (mysqlDialect) rebind(query string) string { return query }

func (mysqlDialect) usesRowID() bool { return false }

func (mysqlDialect) schemaStmts() []string {
	return []string{
		`CREATE TABLE IF NOT EXISTS merchants (
			merchant_id VARCHAR(64) PRIMARY KEY,
			name TEXT NOT NULL,
			status VARCHAR(32) NOT NULL,
			settings_invoice_ttl_seconds BIGINT NOT NULL,
			settings_required_confirmations INT NOT NULL,
			settings_late_payment_policy VARCHAR(32) NOT NULL,
			settings_partial_payment_policy VARCHAR(32) NOT NULL,
			settings_overpayment_policy VARCHAR(32) NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS merchant_wallets (
			merchant_id VARCHAR(64) PRIMARY KEY,
			wallet_id VARCHAR(64) NOT NULL,
			ufvk TEXT NOT NULL,
			chain VARCHAR(32) NOT NULL,
			ua_hrp VARCHAR(16) NOT NULL,
			coin_type INT NOT NULL,
			next_address_index BIGINT NOT NULL,
			created_at BIGINT NOT NULL
		) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			key_id VARCHAR(64) PRIMARY KEY,
			merchant_id VARCHAR(64) NOT NULL,
			label VARCHAR(255) NOT NULL,
			token_hash VARCHAR(64) NOT NULL,
			revoked_at BIGINT NULL,
			created_at BIGINT NOT NULL
		) ENGINE=InnoDB`,
		`CREATE UNIQUE INDEX idx_api_keys_token_hash ON api_keys(token_hash)`,

		`CREATE TABLE IF NOT EXISTS invoices (
			seq BIGINT AUTO_INCREMENT PRIMARY KEY,
			invoice_id VARCHAR(64) NOT NULL,
			merchant_id VARCHAR(64) NOT NULL,
			external_order_id TEXT NOT NULL,
			external_order_id_hash BINARY(32) NOT NULL,
			wallet_id VARCHAR(64) NOT NULL,
			address_index BIGINT NOT NULL,
			address TEXT NOT NULL,
			address_hash BINARY(32) NOT NULL,
			created_after_height BIGINT NOT NULL,
			created_after_hash TEXT NOT NULL,
			amount_zat BIGINT NOT NULL,
			required_confirmations INT NOT NULL,
			policy_late_payment VARCHAR(32) NOT NULL,
			policy_partial_payment VARCHAR(32) NOT NULL,
			policy_overpayment VARCHAR(32) NOT NULL,
			received_pending_zat BIGINT NOT NULL,
			received_confirmed_zat BIGINT NOT NULL,
			status VARCHAR(32) NOT NULL,
			expires_at BIGINT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE (invoice_id),
			UNIQUE (merchant_id, external_order_id_hash),
			UNIQUE (wallet_id, address_hash)
		) ENGINE=InnoDB`,
		`CREATE INDEX idx_invoices_merchant_seq ON invoices(merchant_id, seq)`,
		`CREATE INDEX idx_invoices_merchant_status_seq ON invoices(merchant_id, status, seq)`,

		`CREATE TABLE IF NOT EXISTS invoice_tokens (
			invoice_id VARCHAR(64) PRIMARY KEY,
			token_enc BLOB NOT NULL
		) ENGINE=InnoDB`,

		`CREATE TABLE IF NOT EXISTS scan_cursors (
			wallet_id VARCHAR(64) PRIMARY KEY,
			cursor BIGINT NOT NULL,
			last_event_at BIGINT NULL
		) ENGINE=InnoDB`,

		`CREATE TABLE IF NOT EXISTS deposits (
			seq BIGINT AUTO_INCREMENT PRIMARY KEY,
			wallet_id VARCHAR(64) NOT NULL,
			txid VARCHAR(128) NOT NULL,
			action_index INT NOT NULL,
			recipient_address TEXT NOT NULL,
			recipient_address_hash BINARY(32) NOT NULL,
			amount_zat BIGINT NOT NULL,
			height BIGINT NOT NULL,
			status VARCHAR(32) NOT NULL,
			confirmed_height BIGINT NULL,
			invoice_id VARCHAR(64) NULL,
			detected_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE (wallet_id, txid, action_index)
		) ENGINE=InnoDB`,
		`CREATE INDEX idx_deposits_invoice_status ON deposits(invoice_id, status, seq)`,
		`CREATE INDEX idx_deposits_wallet_recipient_hash ON deposits(wallet_id, recipient_address_hash)`,

		`CREATE TABLE IF NOT EXISTS refunds (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			refund_id VARCHAR(64) NOT NULL,
			merchant_id VARCHAR(64) NOT NULL,
			invoice_id VARCHAR(64) NULL,
			external_refund_id TEXT NULL,
			to_address TEXT NOT NULL,
			amount_zat BIGINT NOT NULL,
			status VARCHAR(32) NOT NULL,
			sent_txid TEXT NULL,
			notes TEXT NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE (refund_id)
		) ENGINE=InnoDB`,
		`CREATE INDEX idx_refunds_merchant_id ON refunds(merchant_id, id)`,
		`CREATE INDEX idx_refunds_invoice_id ON refunds(invoice_id, id)`,

		`CREATE TABLE IF NOT EXISTS review_cases (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			review_id VARCHAR(64) NOT NULL,
			merchant_id VARCHAR(64) NOT NULL,
			invoice_id VARCHAR(64) NULL,
			reason VARCHAR(64) NOT NULL,
			status VARCHAR(32) NOT NULL,
			notes TEXT NOT NULL,
			deposit_wallet_id VARCHAR(64) NULL,
			deposit_txid VARCHAR(128) NULL,
			deposit_action_index INT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE (review_id)
		) ENGINE=InnoDB`,
		`CREATE INDEX idx_review_cases_merchant_status ON review_cases(merchant_id, status, id)`,
		`CREATE UNIQUE INDEX idx_review_cases_invoice_unique ON review_cases(merchant_id, invoice_id, reason, status)`,
		`CREATE UNIQUE INDEX idx_review_cases_deposit_unique ON review_cases(deposit_wallet_id, deposit_txid, deposit_action_index, reason)`,

		`CREATE TABLE IF NOT EXISTS invoice_events (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			invoice_id VARCHAR(64) NOT NULL,
			type VARCHAR(64) NOT NULL,
			occurred_at BIGINT NOT NULL,
			deposit_wallet_id VARCHAR(64) NULL,
			deposit_txid VARCHAR(128) NULL,
			deposit_action_index INT NULL,
			deposit_amount_zat BIGINT NULL,
			deposit_height BIGINT NULL,
			refund_id VARCHAR(64) NULL,
			created_at BIGINT NOT NULL
		) ENGINE=InnoDB`,
		`CREATE INDEX idx_invoice_events_invoice_id ON invoice_events(invoice_id, id)`,
		`CREATE UNIQUE INDEX idx_invoice_events_deposit_unique
		 ON invoice_events(invoice_id, type, deposit_wallet_id, deposit_txid, deposit_action_index)`,
		`CREATE UNIQUE INDEX idx_invoice_events_refund_unique
		 ON invoice_events(invoice_id, type, refund_id)`,

		`CREATE TABLE IF NOT EXISTS event_sinks (
			sink_id VARCHAR(64) PRIMARY KEY,
			merchant_id VARCHAR(64) NOT NULL,
			kind VARCHAR(32) NOT NULL,
			status VARCHAR(32) NOT NULL,
			config_json LONGBLOB NOT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL
		) ENGINE=InnoDB`,
		`CREATE INDEX idx_event_sinks_merchant_id ON event_sinks(merchant_id)`,

		`CREATE TABLE IF NOT EXISTS outbox_events (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			event_id VARCHAR(64) NOT NULL,
			merchant_id VARCHAR(64) NOT NULL,
			envelope_json LONGBLOB NOT NULL,
			created_at BIGINT NOT NULL,
			UNIQUE (event_id)
		) ENGINE=InnoDB`,
		`CREATE INDEX idx_outbox_events_merchant_id ON outbox_events(merchant_id, id)`,

		`CREATE TABLE IF NOT EXISTS event_deliveries (
			delivery_id VARCHAR(64) PRIMARY KEY,
			merchant_id VARCHAR(64) NOT NULL,
			sink_id VARCHAR(64) NOT NULL,
			event_id VARCHAR(64) NOT NULL,
			status VARCHAR(32) NOT NULL,
			attempt INT NOT NULL,
			next_retry_at BIGINT NULL,
			last_error TEXT NULL,
			created_at BIGINT NOT NULL,
			updated_at BIGINT NOT NULL,
			UNIQUE(sink_id, event_id)
		) ENGINE=InnoDB`,
		`CREATE INDEX idx_event_deliveries_merchant_status_retry ON event_deliveries(merchant_id, status, next_retry_at)`,
		`CREATE INDEX idx_event_deliveries_sink_id ON event_deliveries(sink_id)`,
	}
}

func (mysqlDialect) isUniqueViolation(err error) bool {
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		return myErr.Number == 1062
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique")
}

func (mysqlDialect) isAlreadyExists(err error) bool {
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		// 1050: Table already exists
		// 1060: Duplicate column name
		// 1061: Duplicate key name
		return myErr.Number == 1050 || myErr.Number == 1060 || myErr.Number == 1061
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "duplicate")
}

