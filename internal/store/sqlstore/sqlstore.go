package sqlstore

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
	"github.com/Abdullah1738/juno-sdk-go/types"
)

type Store struct {
	db   *sql.DB
	aead cipher.AEAD

	dialect dialect

	rewriteMap map[string]string

	tablePrefix string
}

type dialect interface {
	name() string
	rebind(query string) string
	usesRowID() bool
	schemaStmts() []string
	isUniqueViolation(err error) bool
	isAlreadyExists(err error) bool
}

func hash32Bytes(s string) []byte {
	sum := sha256.Sum256([]byte(s))
	return sum[:]
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Init(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite: nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	stmts := s.dialect.schemaStmts()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, s.q(stmt)); err != nil {
			if s.dialect.isAlreadyExists(err) {
				continue
			}
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// Migrations for older sqlite DBs.
	if s.dialect.name() == "sqlite" {
		if _, err := s.db.ExecContext(ctx, s.q(`ALTER TABLE scan_cursors ADD COLUMN last_event_at INTEGER`)); err != nil {
			// Ignore "duplicate column name" errors.
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "duplicate") && !strings.Contains(msg, "already exists") {
				return err
			}
		}
	}

	// Backfill invoice statuses to match the current status model.
	nowUnix := time.Now().UTC().Unix()
	_, err = s.db.ExecContext(ctx, s.q(`
		UPDATE invoices
		SET status = CASE
			WHEN (received_pending_zat + received_confirmed_zat) = 0
			  AND expires_at IS NOT NULL
			  AND expires_at < ?
				THEN ?
			WHEN (received_pending_zat + received_confirmed_zat) = 0
				THEN ?
			WHEN received_confirmed_zat > amount_zat
				THEN ?
			WHEN received_confirmed_zat = amount_zat
				THEN CASE
					WHEN expires_at IS NOT NULL
					  AND expires_at < ?
					  AND policy_late_payment = ?
						THEN ?
					ELSE ?
				END
			WHEN (received_pending_zat + received_confirmed_zat) >= amount_zat
				THEN ?
			WHEN received_confirmed_zat > 0
				THEN ?
			ELSE ?
		END
		WHERE status <> ?
	`),
		nowUnix, string(domain.InvoiceExpired),
		string(domain.InvoiceOpen),
		string(domain.InvoiceOverpaid),
		nowUnix, string(domain.LatePaymentMarkPaidLate), string(domain.InvoicePaidLate), string(domain.InvoiceConfirmed),
		string(domain.InvoicePending),
		string(domain.InvoicePartialConfirmed),
		string(domain.InvoicePartialPending),
		string(domain.InvoiceCanceled),
	)
	if err != nil {
		return err
	}

	return nil
}

func (s *Store) CreateMerchant(ctx context.Context, name string, settings domain.MerchantSettings) (domain.Merchant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Merchant{}, domain.NewError(domain.ErrInvalidArgument, "name is required")
	}
	if err := settings.Validate(); err != nil {
		return domain.Merchant{}, err
	}

	id, err := newID("m")
	if err != nil {
		return domain.Merchant{}, err
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()

	_, err = s.db.ExecContext(ctx, s.q(`
		INSERT INTO merchants (
			merchant_id, name, status,
			settings_invoice_ttl_seconds, settings_required_confirmations,
			settings_late_payment_policy, settings_partial_payment_policy, settings_overpayment_policy,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		id, name, string(domain.MerchantActive),
		settings.InvoiceTTLSeconds, settings.RequiredConfirmations,
		string(settings.Policies.LatePayment), string(settings.Policies.PartialPayment), string(settings.Policies.Overpayment),
		nowUnix, nowUnix,
	)
	if err != nil {
		return domain.Merchant{}, err
	}

	return domain.Merchant{
		MerchantID: id,
		Name:       name,
		Status:     domain.MerchantActive,
		Settings:   settings,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (s *Store) GetMerchant(ctx context.Context, merchantID string) (domain.Merchant, bool, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return domain.Merchant{}, false, nil
	}

	var (
		name                          string
		status                        string
		invoiceTTLSeconds             int64
		requiredConfirmations         int32
		latePolicy, partialPolicy, op string
		createdAtUnix, updatedAtUnix  int64
	)
	err := s.db.QueryRowContext(ctx, s.q(`
		SELECT name, status,
		       settings_invoice_ttl_seconds, settings_required_confirmations,
		       settings_late_payment_policy, settings_partial_payment_policy, settings_overpayment_policy,
		       created_at, updated_at
		FROM merchants
		WHERE merchant_id = ?
	`), merchantID).Scan(
		&name, &status,
		&invoiceTTLSeconds, &requiredConfirmations,
		&latePolicy, &partialPolicy, &op,
		&createdAtUnix, &updatedAtUnix,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Merchant{}, false, nil
	}
	if err != nil {
		return domain.Merchant{}, false, err
	}

	m := domain.Merchant{
		MerchantID: merchantID,
		Name:       name,
		Status:     domain.MerchantStatus(status),
		Settings: domain.MerchantSettings{
			InvoiceTTLSeconds:     invoiceTTLSeconds,
			RequiredConfirmations: requiredConfirmations,
			Policies: domain.InvoicePolicies{
				LatePayment:    domain.LatePaymentPolicy(latePolicy),
				PartialPayment: domain.PartialPaymentPolicy(partialPolicy),
				Overpayment:    domain.OverpaymentPolicy(op),
			},
		},
		CreatedAt: time.Unix(createdAtUnix, 0).UTC(),
		UpdatedAt: time.Unix(updatedAtUnix, 0).UTC(),
	}

	return m, true, nil
}

func (s *Store) ListMerchants(ctx context.Context) ([]domain.Merchant, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT merchant_id, name, status,
		       settings_invoice_ttl_seconds, settings_required_confirmations,
		       settings_late_payment_policy, settings_partial_payment_policy, settings_overpayment_policy,
		       created_at, updated_at
		FROM merchants
		ORDER BY merchant_id
	`))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Merchant
	for rows.Next() {
		var (
			id                            string
			name                          string
			status                        string
			invoiceTTLSeconds             int64
			requiredConfirmations         int32
			latePolicy, partialPolicy, op string
			createdAtUnix, updatedAtUnix  int64
		)
		if err := rows.Scan(
			&id, &name, &status,
			&invoiceTTLSeconds, &requiredConfirmations,
			&latePolicy, &partialPolicy, &op,
			&createdAtUnix, &updatedAtUnix,
		); err != nil {
			return nil, err
		}
		out = append(out, domain.Merchant{
			MerchantID: id,
			Name:       name,
			Status:     domain.MerchantStatus(status),
			Settings: domain.MerchantSettings{
				InvoiceTTLSeconds:     invoiceTTLSeconds,
				RequiredConfirmations: requiredConfirmations,
				Policies: domain.InvoicePolicies{
					LatePayment:    domain.LatePaymentPolicy(latePolicy),
					PartialPayment: domain.PartialPaymentPolicy(partialPolicy),
					Overpayment:    domain.OverpaymentPolicy(op),
				},
			},
			CreatedAt: time.Unix(createdAtUnix, 0).UTC(),
			UpdatedAt: time.Unix(updatedAtUnix, 0).UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateMerchantSettings(ctx context.Context, merchantID string, settings domain.MerchantSettings) (domain.Merchant, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return domain.Merchant{}, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if err := settings.Validate(); err != nil {
		return domain.Merchant{}, err
	}

	nowUnix := time.Now().UTC().Unix()
	res, err := s.db.ExecContext(ctx, s.q(`
		UPDATE merchants
		SET settings_invoice_ttl_seconds = ?,
		    settings_required_confirmations = ?,
		    settings_late_payment_policy = ?,
		    settings_partial_payment_policy = ?,
		    settings_overpayment_policy = ?,
		    updated_at = ?
		WHERE merchant_id = ?
	`),
		settings.InvoiceTTLSeconds,
		settings.RequiredConfirmations,
		string(settings.Policies.LatePayment),
		string(settings.Policies.PartialPayment),
		string(settings.Policies.Overpayment),
		nowUnix,
		merchantID,
	)
	if err != nil {
		return domain.Merchant{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.Merchant{}, store.ErrNotFound
	}

	m, ok, err := s.GetMerchant(ctx, merchantID)
	if err != nil {
		return domain.Merchant{}, err
	}
	if !ok {
		return domain.Merchant{}, store.ErrNotFound
	}
	return m, nil
}

func (s *Store) SetMerchantWallet(ctx context.Context, merchantID string, w store.MerchantWallet) (store.MerchantWallet, error) {
	merchantID = strings.TrimSpace(merchantID)
	w.WalletID = strings.TrimSpace(w.WalletID)
	w.UFVK = strings.TrimSpace(w.UFVK)
	w.Chain = strings.TrimSpace(w.Chain)
	w.UAHRP = strings.TrimSpace(w.UAHRP)

	if merchantID == "" {
		return store.MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if w.WalletID == "" {
		return store.MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "wallet_id is required")
	}
	if w.UFVK == "" {
		return store.MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "ufvk is required")
	}
	if w.Chain == "" {
		return store.MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "chain is required")
	}
	if w.UAHRP == "" {
		return store.MerchantWallet{}, domain.NewError(domain.ErrInvalidArgument, "ua_hrp is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return store.MerchantWallet{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var exists int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT 1 FROM merchants WHERE merchant_id = ?`), merchantID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.MerchantWallet{}, store.ErrNotFound
		}
		return store.MerchantWallet{}, err
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()
	_, err = tx.ExecContext(ctx, s.q(`
		INSERT INTO merchant_wallets (
			merchant_id, wallet_id, ufvk, chain, ua_hrp, coin_type, next_address_index, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), merchantID, w.WalletID, w.UFVK, w.Chain, w.UAHRP, w.CoinType, 0, nowUnix)
	if err != nil {
		if s.dialect.isUniqueViolation(err) {
			return store.MerchantWallet{}, store.ErrConflict
		}
		return store.MerchantWallet{}, err
	}
	if err := tx.Commit(); err != nil {
		return store.MerchantWallet{}, err
	}

	w.MerchantID = merchantID
	w.CreatedAt = now
	return w, nil
}

func (s *Store) GetMerchantWallet(ctx context.Context, merchantID string) (store.MerchantWallet, bool, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return store.MerchantWallet{}, false, nil
	}
	var w store.MerchantWallet
	var createdAtUnix int64
	err := s.db.QueryRowContext(ctx, s.q(`
		SELECT wallet_id, ufvk, chain, ua_hrp, coin_type, created_at
		FROM merchant_wallets
		WHERE merchant_id = ?
	`), merchantID).Scan(&w.WalletID, &w.UFVK, &w.Chain, &w.UAHRP, &w.CoinType, &createdAtUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return store.MerchantWallet{}, false, nil
	}
	if err != nil {
		return store.MerchantWallet{}, false, err
	}
	w.MerchantID = merchantID
	w.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
	return w, true, nil
}

func (s *Store) ListMerchantWallets(ctx context.Context) ([]store.MerchantWallet, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT merchant_id, wallet_id, ufvk, chain, ua_hrp, coin_type, created_at
		FROM merchant_wallets
		ORDER BY merchant_id
	`))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.MerchantWallet
	for rows.Next() {
		var w store.MerchantWallet
		var createdAtUnix int64
		if err := rows.Scan(&w.MerchantID, &w.WalletID, &w.UFVK, &w.Chain, &w.UAHRP, &w.CoinType, &createdAtUnix); err != nil {
			return nil, err
		}
		w.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) NextAddressIndex(ctx context.Context, merchantID string) (uint32, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return 0, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var idx int64
	err = tx.QueryRowContext(ctx, s.q(`SELECT next_address_index FROM merchant_wallets WHERE merchant_id = ?`), merchantID).Scan(&idx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, store.ErrNotFound
	}
	if err != nil {
		return 0, err
	}

	_, err = tx.ExecContext(ctx, s.q(`UPDATE merchant_wallets SET next_address_index = next_address_index + 1 WHERE merchant_id = ?`), merchantID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return uint32(idx), nil
}

func (s *Store) CreateMerchantAPIKey(ctx context.Context, merchantID, label string) (keyID string, apiKey string, err error) {
	merchantID = strings.TrimSpace(merchantID)
	label = strings.TrimSpace(label)
	if merchantID == "" {
		return "", "", domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}

	var exists int
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT 1 FROM merchants WHERE merchant_id = ?`), merchantID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", store.ErrNotFound
		}
		return "", "", err
	}

	keyID, err = newID("key")
	if err != nil {
		return "", "", err
	}

	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	apiKey = "jps_" + hex.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(apiKey))
	hashHex := hex.EncodeToString(sum[:])

	nowUnix := time.Now().UTC().Unix()
	_, err = s.db.ExecContext(ctx, s.q(`
		INSERT INTO api_keys (key_id, merchant_id, label, token_hash, revoked_at, created_at)
		VALUES (?, ?, ?, ?, NULL, ?)
	`), keyID, merchantID, label, hashHex, nowUnix)
	if err != nil {
		return "", "", err
	}

	return keyID, apiKey, nil
}

func (s *Store) RevokeMerchantAPIKey(ctx context.Context, keyID string) error {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "key_id is required")
	}
	nowUnix := time.Now().UTC().Unix()
	res, err := s.db.ExecContext(ctx, s.q(`
		UPDATE api_keys
		SET revoked_at = ?
		WHERE key_id = ? AND revoked_at IS NULL
	`), nowUnix, keyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists int
		if err := s.db.QueryRowContext(ctx, s.q(`SELECT 1 FROM api_keys WHERE key_id = ?`), keyID).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return store.ErrNotFound
			}
			return err
		}
		return nil
	}
	return nil
}

func (s *Store) LookupMerchantIDByAPIKey(ctx context.Context, apiKey string) (merchantID string, ok bool, err error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", false, nil
	}
	sum := sha256.Sum256([]byte(apiKey))
	hashHex := hex.EncodeToString(sum[:])

	var revokedAt sql.NullInt64
	err = s.db.QueryRowContext(ctx, s.q(`
		SELECT merchant_id, revoked_at
		FROM api_keys
		WHERE token_hash = ?
	`), hashHex).Scan(&merchantID, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if revokedAt.Valid {
		return "", false, nil
	}
	return merchantID, true, nil
}

func (s *Store) ListMerchantAPIKeys(ctx context.Context, merchantID string) ([]store.MerchantAPIKey, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return nil, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}

	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT key_id, merchant_id, label, revoked_at, created_at
		FROM api_keys
		WHERE merchant_id = ?
		ORDER BY created_at DESC, key_id DESC
	`), merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.MerchantAPIKey
	for rows.Next() {
		var rec store.MerchantAPIKey
		var revokedAt sql.NullInt64
		var createdAt int64
		if err := rows.Scan(&rec.KeyID, &rec.MerchantID, &rec.Label, &revokedAt, &createdAt); err != nil {
			return nil, err
		}
		rec.CreatedAt = time.Unix(createdAt, 0).UTC()
		if revokedAt.Valid {
			t := time.Unix(revokedAt.Int64, 0).UTC()
			rec.RevokedAt = &t
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) CreateInvoice(ctx context.Context, req store.InvoiceCreate) (domain.Invoice, bool, error) {
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	req.ExternalOrderID = strings.TrimSpace(req.ExternalOrderID)
	req.WalletID = strings.TrimSpace(req.WalletID)
	req.Address = strings.ToLower(strings.TrimSpace(req.Address))
	if req.MerchantID == "" {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if req.ExternalOrderID == "" {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "external_order_id is required")
	}
	if req.AmountZat <= 0 {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "amount_zat must be > 0")
	}
	if req.Address == "" {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "address is required")
	}
	if req.WalletID == "" {
		return domain.Invoice{}, false, domain.NewError(domain.ErrInvalidArgument, "wallet_id is required")
	}

	id, err := newID("inv")
	if err != nil {
		return domain.Invoice{}, false, err
	}
	now := time.Now().UTC()
	nowUnix := now.Unix()

	var expiresUnix any = nil
	if req.ExpiresAt != nil {
		expiresUnix = req.ExpiresAt.UTC().Unix()
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	extOrderHash := hash32Bytes(req.ExternalOrderID)
	addrHash := hash32Bytes(req.Address)

	_, err = tx.ExecContext(ctx, s.q(`
		INSERT INTO invoices (
			invoice_id, merchant_id, external_order_id, external_order_id_hash,
			wallet_id, address_index, address, address_hash,
			created_after_height, created_after_hash,
			amount_zat, required_confirmations,
			policy_late_payment, policy_partial_payment, policy_overpayment,
			received_pending_zat, received_confirmed_zat,
			status, expires_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?)
	`),
		id, req.MerchantID, req.ExternalOrderID, extOrderHash,
		req.WalletID, req.AddressIndex, req.Address, addrHash,
		req.CreatedAfterHeight, strings.TrimSpace(req.CreatedAfterHash),
		req.AmountZat, req.RequiredConfirmations,
		string(req.Policies.LatePayment), string(req.Policies.PartialPayment), string(req.Policies.Overpayment),
		string(domain.InvoiceOpen), expiresUnix,
		nowUnix, nowUnix,
	)
	if err == nil {
		if err := s.insertInvoiceEventTx(ctx, tx, id, domain.InvoiceEventInvoiceCreated, now, nil, nil); err != nil {
			return domain.Invoice{}, false, err
		}
		if err := tx.Commit(); err != nil {
			return domain.Invoice{}, false, err
		}
		inv := domain.Invoice{
			InvoiceID:             id,
			MerchantID:            req.MerchantID,
			ExternalOrderID:       req.ExternalOrderID,
			WalletID:              req.WalletID,
			AddressIndex:          req.AddressIndex,
			Address:               req.Address,
			CreatedAfterHeight:    req.CreatedAfterHeight,
			CreatedAfterHash:      strings.TrimSpace(req.CreatedAfterHash),
			AmountZat:             req.AmountZat,
			RequiredConfirmations: req.RequiredConfirmations,
			Policies:              req.Policies,
			Status:                domain.InvoiceOpen,
			ExpiresAt:             req.ExpiresAt,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		return inv, true, nil
	}

	if !s.dialect.isUniqueViolation(err) {
		return domain.Invoice{}, false, err
	}

	// Idempotent replay: fetch existing invoice by (merchant_id, external_order_id_hash).
	// Note: on Postgres, once a statement errors the transaction is aborted; do not reuse `tx`.
	_ = tx.Rollback()
	existing, ok, err := s.FindInvoiceByExternalOrderID(ctx, req.MerchantID, req.ExternalOrderID)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	if !ok {
		return domain.Invoice{}, false, store.ErrConflict
	}
	if existing.ExternalOrderID != req.ExternalOrderID {
		return domain.Invoice{}, false, store.ErrConflict
	}
	if existing.AmountZat != req.AmountZat || existing.WalletID != req.WalletID || existing.Address != req.Address {
		return domain.Invoice{}, false, store.ErrConflict
	}
	return existing, false, nil
}

func (s *Store) GetInvoice(ctx context.Context, invoiceID string) (domain.Invoice, bool, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return domain.Invoice{}, false, nil
	}

	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT invoice_id, merchant_id, external_order_id, wallet_id, address_index, address, created_after_height, created_after_hash,
		       amount_zat, required_confirmations,
		       policy_late_payment, policy_partial_payment, policy_overpayment,
		       received_pending_zat, received_confirmed_zat,
		       status, expires_at, created_at, updated_at
		FROM invoices
		WHERE invoice_id = ?
		LIMIT 1
	`), invoiceID)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return domain.Invoice{}, false, nil
	}
	inv, err := scanInvoiceFull(rows)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	return inv, true, nil
}

func (s *Store) FindInvoiceByExternalOrderID(ctx context.Context, merchantID, externalOrderID string) (domain.Invoice, bool, error) {
	merchantID = strings.TrimSpace(merchantID)
	externalOrderID = strings.TrimSpace(externalOrderID)
	if merchantID == "" || externalOrderID == "" {
		return domain.Invoice{}, false, nil
	}

	extHash := hash32Bytes(externalOrderID)
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT invoice_id, merchant_id, external_order_id, wallet_id, address_index, address, created_after_height, created_after_hash,
		       amount_zat, required_confirmations,
		       policy_late_payment, policy_partial_payment, policy_overpayment,
		       received_pending_zat, received_confirmed_zat,
		       status, expires_at, created_at, updated_at
		FROM invoices
		WHERE merchant_id = ? AND external_order_id_hash = ? AND external_order_id = ?
		LIMIT 1
	`), merchantID, extHash, externalOrderID)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return domain.Invoice{}, false, nil
	}
	inv, err := scanInvoiceFull(rows)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	return inv, true, nil
}

func (s *Store) ListInvoices(ctx context.Context, f store.InvoiceFilter) ([]domain.Invoice, int64, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.ExternalOrderID = strings.TrimSpace(f.ExternalOrderID)
	if f.AfterID < 0 {
		f.AfterID = 0
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	where := []string{"seq > ?"}
	args := []any{f.AfterID}

	if f.MerchantID != "" {
		where = append(where, "merchant_id = ?")
		args = append(args, f.MerchantID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(f.Status))
	}
	if f.ExternalOrderID != "" {
		where = append(where, "external_order_id_hash = ? AND external_order_id = ?")
		args = append(args, hash32Bytes(f.ExternalOrderID), f.ExternalOrderID)
	}

	args = append(args, f.Limit)
	q := `
		SELECT seq,
		       invoice_id, merchant_id, external_order_id, wallet_id, address_index, address, created_after_height, created_after_hash,
		       amount_zat, required_confirmations,
		       policy_late_payment, policy_partial_payment, policy_overpayment,
		       received_pending_zat, received_confirmed_zat,
		       status, expires_at, created_at, updated_at
		FROM invoices
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY seq
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, s.q(q), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]domain.Invoice, 0, f.Limit)
	var nextCursor int64
	for rows.Next() {
		var (
			seq              int64
			invoiceID        string
			merchantID       string
			externalOrderID  string
			walletID         string
			addressIndex     uint32
			address          string
			createdAfterH    int64
			createdAfterHash string
			amountZat        int64
			requiredConfs    int32
			latePolicy       string
			partialPolicy    string
			overpayPolicy    string
			recvPending      int64
			recvConfirmed    int64
			status           string
			expiresAtUnix    sql.NullInt64
			createdAtUnix    int64
			updatedAtUnix    int64
		)
		if err := rows.Scan(
			&seq,
			&invoiceID, &merchantID, &externalOrderID,
			&walletID, &addressIndex, &address,
			&createdAfterH, &createdAfterHash,
			&amountZat, &requiredConfs,
			&latePolicy, &partialPolicy, &overpayPolicy,
			&recvPending, &recvConfirmed,
			&status, &expiresAtUnix, &createdAtUnix, &updatedAtUnix,
		); err != nil {
			return nil, 0, err
		}

		var expiresAt *time.Time
		if expiresAtUnix.Valid {
			t := time.Unix(expiresAtUnix.Int64, 0).UTC()
			expiresAt = &t
		}

		out = append(out, domain.Invoice{
			InvoiceID:             invoiceID,
			MerchantID:            merchantID,
			ExternalOrderID:       externalOrderID,
			WalletID:              walletID,
			AddressIndex:          addressIndex,
			Address:               address,
			CreatedAfterHeight:    createdAfterH,
			CreatedAfterHash:      createdAfterHash,
			AmountZat:             amountZat,
			RequiredConfirmations: requiredConfs,
			Policies: domain.InvoicePolicies{
				LatePayment:    domain.LatePaymentPolicy(latePolicy),
				PartialPayment: domain.PartialPaymentPolicy(partialPolicy),
				Overpayment:    domain.OverpaymentPolicy(overpayPolicy),
			},
			ReceivedPendingZat:   recvPending,
			ReceivedConfirmedZat: recvConfirmed,
			Status:               domain.InvoiceStatus(status),
			ExpiresAt:            expiresAt,
			CreatedAt:            time.Unix(createdAtUnix, 0).UTC(),
			UpdatedAt:            time.Unix(updatedAtUnix, 0).UTC(),
		})
		nextCursor = seq
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, nextCursor, nil
}

func (s *Store) PutInvoiceToken(ctx context.Context, invoiceID string, token string) error {
	invoiceID = strings.TrimSpace(invoiceID)
	token = strings.TrimSpace(token)
	if invoiceID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "invoice_id is required")
	}
	if token == "" {
		return domain.NewError(domain.ErrInvalidArgument, "token is required")
	}

	var exists int
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT 1 FROM invoices WHERE invoice_id = ?`), invoiceID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ErrNotFound
		}
		return err
	}

	enc, err := s.encryptToken(invoiceID, token)
	if err != nil {
		return err
	}

	res, err := s.db.ExecContext(ctx, s.q(`UPDATE invoice_tokens SET token_enc = ? WHERE invoice_id = ?`), enc, invoiceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = s.db.ExecContext(ctx, s.q(`INSERT INTO invoice_tokens (invoice_id, token_enc) VALUES (?, ?)`), invoiceID, enc)
	if err == nil {
		return nil
	}
	if s.dialect.isUniqueViolation(err) {
		_, err = s.db.ExecContext(ctx, s.q(`UPDATE invoice_tokens SET token_enc = ? WHERE invoice_id = ?`), enc, invoiceID)
	}
	return err
}

func (s *Store) GetInvoiceToken(ctx context.Context, invoiceID string) (token string, ok bool, err error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return "", false, nil
	}
	var enc []byte
	err = s.db.QueryRowContext(ctx, s.q(`SELECT token_enc FROM invoice_tokens WHERE invoice_id = ?`), invoiceID).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	tok, err := s.decryptToken(invoiceID, enc)
	if err != nil {
		return "", false, err
	}
	return tok, true, nil
}

func (s *Store) ScanCursor(ctx context.Context, walletID string) (cursor int64, err error) {
	walletID = strings.TrimSpace(walletID)
	if walletID == "" {
		return 0, domain.NewError(domain.ErrInvalidArgument, "wallet_id is required")
	}
	err = s.db.QueryRowContext(ctx, s.q(`SELECT cursor_id FROM scan_cursors WHERE wallet_id = ?`), walletID).Scan(&cursor)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return cursor, nil
}

func (s *Store) ScannerStatus(ctx context.Context) (lastCursor int64, lastEventAt *time.Time, err error) {
	var (
		maxCursor int64
		maxEvent  sql.NullInt64
	)
	if err := s.db.QueryRowContext(ctx, s.q(`
		SELECT COALESCE(MAX(cursor_id), 0), MAX(last_event_at)
		FROM scan_cursors
	`)).Scan(&maxCursor, &maxEvent); err != nil {
		return 0, nil, err
	}
	if maxEvent.Valid {
		t := time.Unix(maxEvent.Int64, 0).UTC()
		lastEventAt = &t
	}
	return maxCursor, lastEventAt, nil
}

func (s *Store) PendingDeliveries(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, s.q(`
		SELECT COUNT(*)
		FROM event_deliveries
		WHERE status = ?
	`), string(domain.EventDeliveryPending)).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) ApplyScanEvent(ctx context.Context, ev store.ScanEvent) error {
	ev.WalletID = strings.TrimSpace(ev.WalletID)
	ev.Kind = strings.TrimSpace(ev.Kind)
	if ev.WalletID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "wallet_id is required")
	}
	if ev.Cursor <= 0 {
		return domain.NewError(domain.ErrInvalidArgument, "cursor must be > 0")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var cur int64
	err = tx.QueryRowContext(ctx, s.q(`SELECT cursor_id FROM scan_cursors WHERE wallet_id = ?`), ev.WalletID).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		cur = 0
		err = nil
	}
	if err != nil {
		return err
	}
	if ev.Cursor <= cur {
		return nil
	}

	switch types.WalletEventKind(ev.Kind) {
	case types.WalletEventKindDepositEvent,
		types.WalletEventKindDepositConfirmed,
		types.WalletEventKindDepositUnconfirmed,
		types.WalletEventKindDepositOrphaned:
		if err := s.applyDepositEventTx(ctx, tx, ev); err != nil {
			return err
		}
	default:
		// Ignore other event kinds.
	}

	lastEventAt := ev.OccurredAt.UTC()
	if ev.OccurredAt.IsZero() {
		lastEventAt = time.Now().UTC()
	}
	lastEventAtUnix := lastEventAt.Unix()

	res, err := tx.ExecContext(ctx, s.q(`
		UPDATE scan_cursors
		SET cursor_id = ?, last_event_at = ?
		WHERE wallet_id = ? AND cursor_id < ?
	`), ev.Cursor, lastEventAtUnix, ev.WalletID, ev.Cursor)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		inserted, err := s.execTxIgnoreUnique(ctx, tx, s.q(`
			INSERT INTO scan_cursors (wallet_id, cursor_id, last_event_at)
			VALUES (?, ?, ?)
		`), ev.WalletID, ev.Cursor, lastEventAtUnix)
		if err != nil {
			return err
		}
		if !inserted {
			// Race: retry update.
			if _, err := tx.ExecContext(ctx, s.q(`
				UPDATE scan_cursors
				SET cursor_id = ?, last_event_at = ?
				WHERE wallet_id = ? AND cursor_id < ?
			`), ev.Cursor, lastEventAtUnix, ev.WalletID, ev.Cursor); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *Store) UpdateInvoiceConfirmations(ctx context.Context, bestHeight int64) error {
	if s == nil || s.db == nil {
		return errors.New("sqlite: nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx, s.q(`
		SELECT d.wallet_id, d.txid, d.action_index, d.amount_zat, d.height, d.invoice_id, i.required_confirmations
		FROM deposits d
		JOIN invoices i ON i.invoice_id = d.invoice_id
		WHERE d.status = 'detected'
		  AND d.invoice_id IS NOT NULL
		  AND d.invoice_id <> ''
		  AND d.height <= ?
	`), bestHeight)
	if err != nil {
		return err
	}
	defer rows.Close()

	type dep struct {
		InvoiceID   string
		WalletID    string
		TxID        string
		ActionIndex int32
		AmountZat   int64
		Height      int64
		ConfHeight  int64
	}

	toConfirm := make([]dep, 0, 128)
	for rows.Next() {
		var (
			walletID    string
			txid        string
			actionIndex int32
			amountZat   int64
			height      int64
			invoiceID   string
			reqConfs    int32
		)
		if err := rows.Scan(&walletID, &txid, &actionIndex, &amountZat, &height, &invoiceID, &reqConfs); err != nil {
			return err
		}

		required := int64(reqConfs)
		if required <= 0 {
			required = 1
		}
		if (bestHeight-height+1) < required {
			continue
		}

		toConfirm = append(toConfirm, dep{
			InvoiceID:   invoiceID,
			WalletID:    walletID,
			TxID:        txid,
			ActionIndex: actionIndex,
			AmountZat:   amountZat,
			Height:      height,
			ConfHeight:  height + required - 1,
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	affectedInvoices := make(map[string]struct{}, 128)
	for _, d := range toConfirm {
		res, err := tx.ExecContext(ctx, s.q(`
			UPDATE deposits
			SET status = 'confirmed',
			    confirmed_height = ?,
			    updated_at = ?
			WHERE wallet_id = ? AND txid = ? AND action_index = ? AND status = 'detected'
		`), d.ConfHeight, nowUnix, d.WalletID, d.TxID, d.ActionIndex)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			continue
		}

		depRef := &domain.DepositRef{
			WalletID:    d.WalletID,
			TxID:        d.TxID,
			ActionIndex: d.ActionIndex,
			AmountZat:   d.AmountZat,
			Height:      d.Height,
		}
		if err := s.insertInvoiceEventTx(ctx, tx, d.InvoiceID, domain.InvoiceEventDepositConfirmed, now, depRef, nil); err != nil {
			return err
		}

		affectedInvoices[d.InvoiceID] = struct{}{}
	}

	for invoiceID := range affectedInvoices {
		if err := s.recomputeInvoiceAggregatesTx(ctx, tx, invoiceID, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) ListInvoiceEvents(ctx context.Context, invoiceID string, afterID int64, limit int) (events []domain.InvoiceEvent, nextCursor int64, err error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return nil, 0, domain.NewError(domain.ErrInvalidArgument, "invoice_id is required")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT ie.id, ie.type, ie.occurred_at,
		       ie.deposit_wallet_id, ie.deposit_txid, ie.deposit_action_index, ie.deposit_amount_zat, ie.deposit_height,
		       r.refund_id, r.merchant_id, r.invoice_id, r.external_refund_id, r.to_address, r.amount_zat, r.status, r.sent_txid, r.notes, r.created_at, r.updated_at
		FROM invoice_events ie
		LEFT JOIN refunds r ON r.refund_id = ie.refund_id
		WHERE ie.invoice_id = ? AND ie.id > ?
		ORDER BY ie.id
		LIMIT ?
	`), invoiceID, afterID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id             int64
			typ            string
			occurredAtUnix int64
			depWalletID    sql.NullString
			depTxID        sql.NullString
			depActionIndex sql.NullInt64
			depAmountZat   sql.NullInt64
			depHeight      sql.NullInt64
			refundID       sql.NullString
			refundMerchant sql.NullString
			refundInvoice  sql.NullString
			refundExternal sql.NullString
			refundToAddr   sql.NullString
			refundAmount   sql.NullInt64
			refundStatus   sql.NullString
			refundSentTxID sql.NullString
			refundNotes    sql.NullString
			refundCreated  sql.NullInt64
			refundUpdated  sql.NullInt64
		)
		if err := rows.Scan(
			&id, &typ, &occurredAtUnix,
			&depWalletID, &depTxID, &depActionIndex, &depAmountZat, &depHeight,
			&refundID, &refundMerchant, &refundInvoice, &refundExternal, &refundToAddr, &refundAmount, &refundStatus, &refundSentTxID, &refundNotes, &refundCreated, &refundUpdated,
		); err != nil {
			return nil, 0, err
		}

		var dep *domain.DepositRef
		if depTxID.Valid {
			dep = &domain.DepositRef{
				WalletID:    depWalletID.String,
				TxID:        depTxID.String,
				ActionIndex: int32(depActionIndex.Int64),
				AmountZat:   depAmountZat.Int64,
				Height:      depHeight.Int64,
			}
		}

		var refund *domain.Refund
		if refundID.Valid && strings.TrimSpace(refundID.String) != "" {
			var invID *string
			if refundInvoice.Valid && strings.TrimSpace(refundInvoice.String) != "" {
				v := refundInvoice.String
				invID = &v
			}
			var extID *string
			if refundExternal.Valid && strings.TrimSpace(refundExternal.String) != "" {
				v := refundExternal.String
				extID = &v
			}
			var sentTxID *string
			if refundSentTxID.Valid && strings.TrimSpace(refundSentTxID.String) != "" {
				v := refundSentTxID.String
				sentTxID = &v
			}

			refund = &domain.Refund{
				RefundID:         refundID.String,
				MerchantID:       refundMerchant.String,
				InvoiceID:        invID,
				ExternalRefundID: extID,
				ToAddress:        refundToAddr.String,
				AmountZat:        refundAmount.Int64,
				Status:           domain.RefundStatus(refundStatus.String),
				SentTxID:         sentTxID,
				Notes:            refundNotes.String,
				CreatedAt:        time.Unix(refundCreated.Int64, 0).UTC(),
				UpdatedAt:        time.Unix(refundUpdated.Int64, 0).UTC(),
			}
		}

		events = append(events, domain.InvoiceEvent{
			EventID:    strconv.FormatInt(id, 10),
			Type:       domain.InvoiceEventType(typ),
			OccurredAt: time.Unix(occurredAtUnix, 0).UTC(),
			InvoiceID:  invoiceID,
			Deposit:    dep,
			Refund:     refund,
		})
		nextCursor = id
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return events, nextCursor, nil
}

func (s *Store) ListDeposits(ctx context.Context, f store.DepositFilter) (deposits []domain.Deposit, nextCursor int64, err error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.InvoiceID = strings.TrimSpace(f.InvoiceID)
	f.TxID = strings.TrimSpace(f.TxID)
	if f.AfterID < 0 {
		f.AfterID = 0
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	base := `
		SELECT d.seq,
		       d.wallet_id, d.txid, d.action_index, d.recipient_address, d.amount_zat, d.height,
		       d.status, d.confirmed_height, d.invoice_id,
		       d.detected_at, d.updated_at
		FROM deposits d
	`

	where := []string{"d.seq > ?"}
	args := []any{f.AfterID}

	if f.MerchantID != "" {
		base += ` JOIN merchant_wallets mw ON mw.wallet_id = d.wallet_id `
		where = append(where, "mw.merchant_id = ?")
		args = append(args, f.MerchantID)
	}
	if f.InvoiceID != "" {
		where = append(where, "d.invoice_id = ?")
		args = append(args, f.InvoiceID)
	}
	if f.TxID != "" {
		where = append(where, "d.txid = ?")
		args = append(args, f.TxID)
	}

	args = append(args, f.Limit)
	q := base + `
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY d.seq
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, s.q(q), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]domain.Deposit, 0, f.Limit)
	var next int64
	for rows.Next() {
		var (
			seq         int64
			walletID    string
			txid        string
			actionIndex int32
			addr        string
			amountZat   int64
			height      int64
			status      string
			confHeight  sql.NullInt64
			invoiceID   sql.NullString
			detectedAt  int64
			updatedAt   int64
		)
		if err := rows.Scan(&seq, &walletID, &txid, &actionIndex, &addr, &amountZat, &height, &status, &confHeight, &invoiceID, &detectedAt, &updatedAt); err != nil {
			return nil, 0, err
		}

		var confirmedHeight *int64
		if confHeight.Valid {
			v := confHeight.Int64
			confirmedHeight = &v
		}
		var invID *string
		if invoiceID.Valid && strings.TrimSpace(invoiceID.String) != "" {
			v := invoiceID.String
			invID = &v
		}

		out = append(out, domain.Deposit{
			WalletID:         walletID,
			TxID:             txid,
			ActionIndex:      actionIndex,
			RecipientAddress: addr,
			AmountZat:        amountZat,
			Height:           height,
			Status:           domain.DepositStatus(status),
			ConfirmedHeight:  confirmedHeight,
			InvoiceID:        invID,
			DetectedAt:       time.Unix(detectedAt, 0).UTC(),
			UpdatedAt:        time.Unix(updatedAt, 0).UTC(),
		})
		next = seq
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, next, nil
}

func (s *Store) CreateRefund(ctx context.Context, req store.RefundCreate) (domain.Refund, error) {
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	req.InvoiceID = strings.TrimSpace(req.InvoiceID)
	req.ExternalRefundID = strings.TrimSpace(req.ExternalRefundID)
	req.ToAddress = strings.TrimSpace(req.ToAddress)
	req.SentTxID = strings.TrimSpace(req.SentTxID)
	req.Notes = strings.TrimSpace(req.Notes)
	if req.MerchantID == "" {
		return domain.Refund{}, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if req.ToAddress == "" {
		return domain.Refund{}, domain.NewError(domain.ErrInvalidArgument, "to_address is required")
	}
	if req.AmountZat <= 0 {
		return domain.Refund{}, domain.NewError(domain.ErrInvalidArgument, "amount_zat must be > 0")
	}

	refundID, err := newID("refund")
	if err != nil {
		return domain.Refund{}, err
	}

	status := domain.RefundRequested
	if req.SentTxID != "" {
		status = domain.RefundSent
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()

	var invoiceIDAny any = nil
	if req.InvoiceID != "" {
		invoiceIDAny = req.InvoiceID
	}
	var extAny any = nil
	if req.ExternalRefundID != "" {
		extAny = req.ExternalRefundID
	}
	var sentAny any = nil
	if req.SentTxID != "" {
		sentAny = req.SentTxID
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Refund{}, err
	}
	defer func() { _ = tx.Rollback() }()

	// Validate merchant exists.
	var exists int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT 1 FROM merchants WHERE merchant_id = ?`), req.MerchantID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Refund{}, store.ErrNotFound
		}
		return domain.Refund{}, err
	}

	// Validate invoice belongs to merchant (if provided).
	if req.InvoiceID != "" {
		var invMerchantID string
		if err := tx.QueryRowContext(ctx, s.q(`SELECT merchant_id FROM invoices WHERE invoice_id = ?`), req.InvoiceID).Scan(&invMerchantID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.Refund{}, store.ErrNotFound
			}
			return domain.Refund{}, err
		}
		if invMerchantID != req.MerchantID {
			return domain.Refund{}, store.ErrForbidden
		}
	}

	if _, err := tx.ExecContext(ctx, s.q(`
		INSERT INTO refunds (
			refund_id, merchant_id, invoice_id, external_refund_id,
			to_address, amount_zat, status, sent_txid, notes,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		refundID, req.MerchantID, invoiceIDAny, extAny,
		req.ToAddress, req.AmountZat, string(status), sentAny, req.Notes,
		nowUnix, nowUnix,
	); err != nil {
		return domain.Refund{}, err
	}

	var invID *string
	if req.InvoiceID != "" {
		v := req.InvoiceID
		invID = &v

		typ := domain.InvoiceEventRefundRequested
		if status == domain.RefundSent {
			typ = domain.InvoiceEventRefundSent
		}
		if err := s.insertInvoiceEventTx(ctx, tx, req.InvoiceID, typ, now, nil, &refundID); err != nil {
			return domain.Refund{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return domain.Refund{}, err
	}

	var extID *string
	if req.ExternalRefundID != "" {
		v := req.ExternalRefundID
		extID = &v
	}
	var sentTxID *string
	if req.SentTxID != "" {
		v := req.SentTxID
		sentTxID = &v
	}

	return domain.Refund{
		RefundID:         refundID,
		MerchantID:       req.MerchantID,
		InvoiceID:        invID,
		ExternalRefundID: extID,
		ToAddress:        req.ToAddress,
		AmountZat:        req.AmountZat,
		Status:           status,
		SentTxID:         sentTxID,
		Notes:            req.Notes,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func (s *Store) ListRefunds(ctx context.Context, f store.RefundFilter) (refunds []domain.Refund, nextCursor int64, err error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.InvoiceID = strings.TrimSpace(f.InvoiceID)
	if f.AfterID < 0 {
		f.AfterID = 0
	}
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	where := []string{"id > ?"}
	args := []any{f.AfterID}

	if f.MerchantID != "" {
		where = append(where, "merchant_id = ?")
		args = append(args, f.MerchantID)
	}
	if f.InvoiceID != "" {
		where = append(where, "invoice_id = ?")
		args = append(args, f.InvoiceID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(f.Status))
	}

	args = append(args, f.Limit)

	q := `
		SELECT id, refund_id, merchant_id, invoice_id, external_refund_id, to_address, amount_zat, status, sent_txid, notes, created_at, updated_at
		FROM refunds
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY id
		LIMIT ?
	`

	rows, err := s.db.QueryContext(ctx, s.q(q), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]domain.Refund, 0, f.Limit)
	var next int64
	for rows.Next() {
		var (
			seq        int64
			refundID   string
			merchantID string
			invoiceID  sql.NullString
			externalID sql.NullString
			toAddress  string
			amountZat  int64
			status     string
			sentTxID   sql.NullString
			notes      string
			createdAt  int64
			updatedAt  int64
		)
		if err := rows.Scan(&seq, &refundID, &merchantID, &invoiceID, &externalID, &toAddress, &amountZat, &status, &sentTxID, &notes, &createdAt, &updatedAt); err != nil {
			return nil, 0, err
		}

		var invID *string
		if invoiceID.Valid && strings.TrimSpace(invoiceID.String) != "" {
			v := invoiceID.String
			invID = &v
		}
		var extID *string
		if externalID.Valid && strings.TrimSpace(externalID.String) != "" {
			v := externalID.String
			extID = &v
		}
		var sentID *string
		if sentTxID.Valid && strings.TrimSpace(sentTxID.String) != "" {
			v := sentTxID.String
			sentID = &v
		}

		out = append(out, domain.Refund{
			RefundID:         refundID,
			MerchantID:       merchantID,
			InvoiceID:        invID,
			ExternalRefundID: extID,
			ToAddress:        toAddress,
			AmountZat:        amountZat,
			Status:           domain.RefundStatus(status),
			SentTxID:         sentID,
			Notes:            notes,
			CreatedAt:        time.Unix(createdAt, 0).UTC(),
			UpdatedAt:        time.Unix(updatedAt, 0).UTC(),
		})
		next = seq
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, next, nil
}

func (s *Store) ListReviewCases(ctx context.Context, f store.ReviewCaseFilter) ([]domain.ReviewCase, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	where := []string{"1=1"}
	args := []any{}

	if f.MerchantID != "" {
		where = append(where, "merchant_id = ?")
		args = append(args, f.MerchantID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(f.Status))
	}

	q := `
		SELECT review_id, merchant_id, invoice_id, reason, status, notes, created_at, updated_at
		FROM review_cases
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY id DESC
	`

	rows, err := s.db.QueryContext(ctx, s.q(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.ReviewCase{}
	for rows.Next() {
		var (
			reviewID   string
			merchantID string
			invoiceID  sql.NullString
			reason     string
			status     string
			notes      string
			createdAt  int64
			updatedAt  int64
		)
		if err := rows.Scan(&reviewID, &merchantID, &invoiceID, &reason, &status, &notes, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var invID *string
		if invoiceID.Valid && strings.TrimSpace(invoiceID.String) != "" {
			v := invoiceID.String
			invID = &v
		}
		out = append(out, domain.ReviewCase{
			ReviewID:   reviewID,
			MerchantID: merchantID,
			InvoiceID:  invID,
			Reason:     domain.ReviewReason(reason),
			Status:     domain.ReviewStatus(status),
			Notes:      notes,
			CreatedAt:  time.Unix(createdAt, 0).UTC(),
			UpdatedAt:  time.Unix(updatedAt, 0).UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ResolveReviewCase(ctx context.Context, reviewID string, notes string) error {
	reviewID = strings.TrimSpace(reviewID)
	notes = strings.TrimSpace(notes)
	if reviewID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "review_id is required")
	}
	if notes == "" {
		return domain.NewError(domain.ErrInvalidArgument, "notes is required")
	}

	now := time.Now().UTC().Unix()
	res, err := s.db.ExecContext(ctx, s.q(`
		UPDATE review_cases
		SET status = ?, notes = ?, updated_at = ?
		WHERE review_id = ?
	`), string(domain.ReviewResolved), notes, now, reviewID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) RejectReviewCase(ctx context.Context, reviewID string, notes string) error {
	reviewID = strings.TrimSpace(reviewID)
	notes = strings.TrimSpace(notes)
	if reviewID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "review_id is required")
	}
	if notes == "" {
		return domain.NewError(domain.ErrInvalidArgument, "notes is required")
	}

	now := time.Now().UTC().Unix()
	res, err := s.db.ExecContext(ctx, s.q(`
		UPDATE review_cases
		SET status = ?, notes = ?, updated_at = ?
		WHERE review_id = ?
	`), string(domain.ReviewRejected), notes, now, reviewID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) CreateEventSink(ctx context.Context, req store.EventSinkCreate) (domain.EventSink, error) {
	req.MerchantID = strings.TrimSpace(req.MerchantID)
	if req.MerchantID == "" {
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if len(req.Config) == 0 {
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "config is required")
	}

	switch req.Kind {
	case domain.EventSinkWebhook, domain.EventSinkKafka, domain.EventSinkNATS, domain.EventSinkRabbitMQ:
	default:
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "kind invalid")
	}

	var cfgAny any
	if err := json.Unmarshal(req.Config, &cfgAny); err != nil {
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "config invalid json")
	}
	if _, ok := cfgAny.(map[string]any); !ok {
		return domain.EventSink{}, domain.NewError(domain.ErrInvalidArgument, "config must be an object")
	}
	cfgBytes, err := json.Marshal(cfgAny)
	if err != nil {
		return domain.EventSink{}, err
	}

	var exists int
	if err := s.db.QueryRowContext(ctx, s.q(`SELECT 1 FROM merchants WHERE merchant_id = ?`), req.MerchantID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.EventSink{}, store.ErrNotFound
		}
		return domain.EventSink{}, err
	}

	sinkID, err := newID("sink")
	if err != nil {
		return domain.EventSink{}, err
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()
	_, err = s.db.ExecContext(ctx, s.q(`
		INSERT INTO event_sinks (sink_id, merchant_id, kind, status, config_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), sinkID, req.MerchantID, string(req.Kind), string(domain.EventSinkActive), cfgBytes, nowUnix, nowUnix)
	if err != nil {
		return domain.EventSink{}, err
	}

	return domain.EventSink{
		SinkID:     sinkID,
		MerchantID: req.MerchantID,
		Kind:       req.Kind,
		Status:     domain.EventSinkActive,
		Config:     cfgBytes,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

func (s *Store) GetEventSink(ctx context.Context, sinkID string) (domain.EventSink, bool, error) {
	sinkID = strings.TrimSpace(sinkID)
	if sinkID == "" {
		return domain.EventSink{}, false, nil
	}

	var (
		merchantID  string
		kind        string
		status      string
		configBytes []byte
		createdUnix int64
		updatedUnix int64
	)
	err := s.db.QueryRowContext(ctx, s.q(`
		SELECT merchant_id, kind, status, config_json, created_at, updated_at
		FROM event_sinks
		WHERE sink_id = ?
	`), sinkID).Scan(&merchantID, &kind, &status, &configBytes, &createdUnix, &updatedUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.EventSink{}, false, nil
	}
	if err != nil {
		return domain.EventSink{}, false, err
	}

	return domain.EventSink{
		SinkID:     sinkID,
		MerchantID: merchantID,
		Kind:       domain.EventSinkKind(kind),
		Status:     domain.EventSinkStatus(status),
		Config:     configBytes,
		CreatedAt:  time.Unix(createdUnix, 0).UTC(),
		UpdatedAt:  time.Unix(updatedUnix, 0).UTC(),
	}, true, nil
}

func (s *Store) ListEventSinks(ctx context.Context, merchantID string) ([]domain.EventSink, error) {
	merchantID = strings.TrimSpace(merchantID)

	var (
		rows *sql.Rows
		err  error
	)
	if merchantID == "" {
		rows, err = s.db.QueryContext(ctx, s.q(`
			SELECT sink_id, merchant_id, kind, status, config_json, created_at, updated_at
			FROM event_sinks
			ORDER BY created_at, sink_id
		`))
	} else {
		rows, err = s.db.QueryContext(ctx, s.q(`
			SELECT sink_id, merchant_id, kind, status, config_json, created_at, updated_at
			FROM event_sinks
			WHERE merchant_id = ?
			ORDER BY created_at, sink_id
		`), merchantID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.EventSink
	for rows.Next() {
		var (
			sinkID      string
			merchantID0 string
			kind        string
			status      string
			configBytes []byte
			createdUnix int64
			updatedUnix int64
		)
		if err := rows.Scan(&sinkID, &merchantID0, &kind, &status, &configBytes, &createdUnix, &updatedUnix); err != nil {
			return nil, err
		}
		out = append(out, domain.EventSink{
			SinkID:     sinkID,
			MerchantID: merchantID0,
			Kind:       domain.EventSinkKind(kind),
			Status:     domain.EventSinkStatus(status),
			Config:     configBytes,
			CreatedAt:  time.Unix(createdUnix, 0).UTC(),
			UpdatedAt:  time.Unix(updatedUnix, 0).UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListOutboundEvents(ctx context.Context, merchantID string, afterID int64, limit int) ([]domain.CloudEvent, int64, error) {
	merchantID = strings.TrimSpace(merchantID)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var (
		rows *sql.Rows
		err  error
	)
	if merchantID == "" {
		rows, err = s.db.QueryContext(ctx, s.q(`
			SELECT id, envelope_json
			FROM outbox_events
			WHERE id > ?
			ORDER BY id
			LIMIT ?
		`), afterID, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, s.q(`
			SELECT id, envelope_json
			FROM outbox_events
			WHERE merchant_id = ? AND id > ?
			ORDER BY id
			LIMIT ?
		`), merchantID, afterID, limit)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []domain.CloudEvent
	var next int64
	for rows.Next() {
		var id int64
		var b []byte
		if err := rows.Scan(&id, &b); err != nil {
			return nil, 0, err
		}
		var ce domain.CloudEvent
		if err := json.Unmarshal(b, &ce); err != nil {
			return nil, 0, err
		}
		out = append(out, ce)
		next = id
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, next, nil
}

func (s *Store) ListEventDeliveries(ctx context.Context, f store.EventDeliveryFilter) ([]domain.EventDelivery, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.SinkID = strings.TrimSpace(f.SinkID)
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}

	where := []string{"1=1"}
	var args []any
	if f.MerchantID != "" {
		where = append(where, "merchant_id = ?")
		args = append(args, f.MerchantID)
	}
	if f.SinkID != "" {
		where = append(where, "sink_id = ?")
		args = append(args, f.SinkID)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, string(f.Status))
	}
	args = append(args, f.Limit)

	q := `
		SELECT delivery_id, sink_id, event_id, status, attempt, next_retry_at, last_error, created_at, updated_at
		FROM event_deliveries
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY updated_at DESC
		LIMIT ?
	`
	rows, err := s.db.QueryContext(ctx, s.q(q), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.EventDelivery
	for rows.Next() {
		var (
			deliveryID  string
			sinkID      string
			eventID     string
			status      string
			attempt     int32
			nextUnix    sql.NullInt64
			lastErr     sql.NullString
			createdUnix int64
			updatedUnix int64
		)
		if err := rows.Scan(&deliveryID, &sinkID, &eventID, &status, &attempt, &nextUnix, &lastErr, &createdUnix, &updatedUnix); err != nil {
			return nil, err
		}
		var nextRetry *time.Time
		if nextUnix.Valid {
			tm := time.Unix(nextUnix.Int64, 0).UTC()
			nextRetry = &tm
		}
		var lastErrPtr *string
		if lastErr.Valid {
			s := lastErr.String
			lastErrPtr = &s
		}
		out = append(out, domain.EventDelivery{
			DeliveryID:  deliveryID,
			SinkID:      sinkID,
			EventID:     eventID,
			Status:      domain.EventDeliveryStatus(status),
			Attempt:     attempt,
			NextRetryAt: nextRetry,
			LastError:   lastErrPtr,
			CreatedAt:   time.Unix(createdUnix, 0).UTC(),
			UpdatedAt:   time.Unix(updatedUnix, 0).UTC(),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListDueDeliveries(ctx context.Context, now time.Time, limit int) ([]store.DueDelivery, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowUnix := now.UTC().Unix()

	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT d.delivery_id, d.merchant_id, d.sink_id, d.event_id, d.status, d.attempt, d.next_retry_at, d.last_error, d.created_at, d.updated_at,
		       s.kind, s.status, s.config_json, s.created_at, s.updated_at,
		       e.envelope_json
		FROM event_deliveries d
		JOIN event_sinks s ON s.sink_id = d.sink_id
		JOIN outbox_events e ON e.event_id = d.event_id
		WHERE d.status = ? AND s.status = ? AND (d.next_retry_at IS NULL OR d.next_retry_at <= ?)
		ORDER BY d.updated_at, d.delivery_id
		LIMIT ?
	`), string(domain.EventDeliveryPending), string(domain.EventSinkActive), nowUnix, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.DueDelivery
	for rows.Next() {
		var (
			deliveryID   string
			merchantID   string
			sinkID       string
			eventID      string
			dStatus      string
			attempt      int32
			nextUnix     sql.NullInt64
			lastErr      sql.NullString
			dCreatedUnix int64
			dUpdatedUnix int64

			sKind        string
			sStatus      string
			sConfig      []byte
			sCreatedUnix int64
			sUpdatedUnix int64

			envBytes []byte
		)
		if err := rows.Scan(
			&deliveryID, &merchantID, &sinkID, &eventID, &dStatus, &attempt, &nextUnix, &lastErr, &dCreatedUnix, &dUpdatedUnix,
			&sKind, &sStatus, &sConfig, &sCreatedUnix, &sUpdatedUnix,
			&envBytes,
		); err != nil {
			return nil, err
		}

		var nextRetry *time.Time
		if nextUnix.Valid {
			tm := time.Unix(nextUnix.Int64, 0).UTC()
			nextRetry = &tm
		}
		var lastErrPtr *string
		if lastErr.Valid {
			s := lastErr.String
			lastErrPtr = &s
		}

		var ce domain.CloudEvent
		if err := json.Unmarshal(envBytes, &ce); err != nil {
			return nil, err
		}

		out = append(out, store.DueDelivery{
			Delivery: domain.EventDelivery{
				DeliveryID:  deliveryID,
				SinkID:      sinkID,
				EventID:     eventID,
				Status:      domain.EventDeliveryStatus(dStatus),
				Attempt:     attempt,
				NextRetryAt: nextRetry,
				LastError:   lastErrPtr,
				CreatedAt:   time.Unix(dCreatedUnix, 0).UTC(),
				UpdatedAt:   time.Unix(dUpdatedUnix, 0).UTC(),
			},
			Sink: domain.EventSink{
				SinkID:     sinkID,
				MerchantID: merchantID,
				Kind:       domain.EventSinkKind(sKind),
				Status:     domain.EventSinkStatus(sStatus),
				Config:     sConfig,
				CreatedAt:  time.Unix(sCreatedUnix, 0).UTC(),
				UpdatedAt:  time.Unix(sUpdatedUnix, 0).UTC(),
			},
			Event: ce,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateEventDelivery(ctx context.Context, deliveryID string, status domain.EventDeliveryStatus, attempt int32, nextRetryAt *time.Time, lastError *string) error {
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "delivery_id is required")
	}

	nowUnix := time.Now().UTC().Unix()
	var nextUnix any = nil
	if nextRetryAt != nil {
		nextUnix = nextRetryAt.UTC().Unix()
	}
	var lastErrAny any = nil
	if lastError != nil {
		lastErrAny = *lastError
	}

	res, err := s.db.ExecContext(ctx, s.q(`
		UPDATE event_deliveries
		SET status = ?, attempt = ?, next_retry_at = ?, last_error = ?, updated_at = ?
		WHERE delivery_id = ?
	`), string(status), attempt, nextUnix, lastErrAny, nowUnix, deliveryID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func scanInvoiceFull(rows *sql.Rows) (domain.Invoice, error) {
	var (
		invoiceID          string
		merchantID         string
		externalOrderID    string
		walletID           string
		addressIndex       uint32
		address            string
		createdAfterHeight int64
		createdAfterHash   string
		amountZat          int64
		requiredConfs      int32
		latePolicy         string
		partialPolicy      string
		overpayPolicy      string
		recvPending        int64
		recvConfirmed      int64
		status             string
		expiresAtUnix      sql.NullInt64
		createdAtUnix      int64
		updatedAtUnix      int64
	)

	if err := rows.Scan(
		&invoiceID, &merchantID, &externalOrderID,
		&walletID, &addressIndex, &address,
		&createdAfterHeight, &createdAfterHash,
		&amountZat, &requiredConfs,
		&latePolicy, &partialPolicy, &overpayPolicy,
		&recvPending, &recvConfirmed,
		&status, &expiresAtUnix, &createdAtUnix, &updatedAtUnix,
	); err != nil {
		return domain.Invoice{}, err
	}

	var expiresAt *time.Time
	if expiresAtUnix.Valid {
		t := time.Unix(expiresAtUnix.Int64, 0).UTC()
		expiresAt = &t
	}

	return domain.Invoice{
		InvoiceID:             invoiceID,
		MerchantID:            merchantID,
		ExternalOrderID:       externalOrderID,
		WalletID:              walletID,
		AddressIndex:          addressIndex,
		Address:               address,
		CreatedAfterHeight:    createdAfterHeight,
		CreatedAfterHash:      createdAfterHash,
		AmountZat:             amountZat,
		RequiredConfirmations: requiredConfs,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentPolicy(latePolicy),
			PartialPayment: domain.PartialPaymentPolicy(partialPolicy),
			Overpayment:    domain.OverpaymentPolicy(overpayPolicy),
		},
		ReceivedPendingZat:   recvPending,
		ReceivedConfirmedZat: recvConfirmed,
		Status:               domain.InvoiceStatus(status),
		ExpiresAt:            expiresAt,
		CreatedAt:            time.Unix(createdAtUnix, 0).UTC(),
		UpdatedAt:            time.Unix(updatedAtUnix, 0).UTC(),
	}, nil
}

func (s *Store) insertInvoiceEventTx(ctx context.Context, tx *sql.Tx, invoiceID string, typ domain.InvoiceEventType, occurredAt time.Time, dep *domain.DepositRef, refundID *string) error {
	if dep == nil && refundID == nil {
		var exists int
		if err := tx.QueryRowContext(ctx, s.q(`
			SELECT 1
			FROM invoice_events
			WHERE invoice_id = ? AND type = ? AND deposit_txid IS NULL AND refund_id IS NULL
			LIMIT 1
		`), invoiceID, string(typ)).Scan(&exists); err == nil {
			return nil
		}
	}

	nowUnix := time.Now().UTC().Unix()
	occurredUnix := occurredAt.UTC().Unix()

	var depWalletID any = nil
	var depTxID any = nil
	var depActionIndex any = nil
	var depAmountZat any = nil
	var depHeight any = nil
	if dep != nil {
		depWalletID = dep.WalletID
		depTxID = dep.TxID
		depActionIndex = dep.ActionIndex
		depAmountZat = dep.AmountZat
		depHeight = dep.Height
	}

	var refundIDAny any = nil
	if refundID != nil && strings.TrimSpace(*refundID) != "" {
		refundIDAny = strings.TrimSpace(*refundID)
	}

	inserted, err := s.execTxIgnoreUnique(ctx, tx, s.q(`
		INSERT INTO invoice_events (
			invoice_id, type, occurred_at,
			deposit_wallet_id, deposit_txid, deposit_action_index, deposit_amount_zat, deposit_height,
			refund_id,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`),
		invoiceID, string(typ), occurredUnix,
		depWalletID, depTxID, depActionIndex, depAmountZat, depHeight,
		refundIDAny,
		nowUnix,
	)
	if err != nil {
		return err
	}
	if !inserted {
		return nil
	}

	// Note: for events where both deposit and refund are NULL, we pre-check for duplicates above,
	// because unique indexes do not dedupe NULLs across all SQL dialects.
	return s.enqueueOutboxForInvoiceEventTx(ctx, tx, invoiceID, typ, occurredAt, dep, refundID)
}

func (s *Store) enqueueOutboxForInvoiceEventTx(ctx context.Context, tx *sql.Tx, invoiceID string, typ domain.InvoiceEventType, occurredAt time.Time, dep *domain.DepositRef, refundID *string) error {
	var merchantID string
	var externalOrderID string
	err := tx.QueryRowContext(ctx, s.q(`
		SELECT merchant_id, external_order_id
		FROM invoices
		WHERE invoice_id = ?
		LIMIT 1
	`), invoiceID).Scan(&merchantID, &externalOrderID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	data := map[string]any{
		"merchant_id":       merchantID,
		"invoice_id":        invoiceID,
		"external_order_id": externalOrderID,
	}
	if dep != nil {
		data["deposit"] = map[string]any{
			"wallet_id":    dep.WalletID,
			"txid":         dep.TxID,
			"action_index": dep.ActionIndex,
			"amount_zat":   dep.AmountZat,
			"height":       dep.Height,
		}
	}
	if refundID != nil && strings.TrimSpace(*refundID) != "" {
		var (
			rID            string
			rMerchantID    string
			rInvoiceID     sql.NullString
			rExternalID    sql.NullString
			rToAddress     string
			rAmountZat     int64
			rStatus        string
			rSentTxID      sql.NullString
			rNotes         string
			rCreatedAtUnix int64
			rUpdatedAtUnix int64
		)
		err := tx.QueryRowContext(ctx, s.q(`
			SELECT refund_id, merchant_id, invoice_id, external_refund_id, to_address, amount_zat, status, sent_txid, notes, created_at, updated_at
			FROM refunds
			WHERE refund_id = ?
			LIMIT 1
		`), strings.TrimSpace(*refundID)).Scan(&rID, &rMerchantID, &rInvoiceID, &rExternalID, &rToAddress, &rAmountZat, &rStatus, &rSentTxID, &rNotes, &rCreatedAtUnix, &rUpdatedAtUnix)
		if err == nil {
			refund := map[string]any{
				"refund_id":   rID,
				"merchant_id": rMerchantID,
				"to_address":  rToAddress,
				"amount_zat":  rAmountZat,
				"status":      rStatus,
				"notes":       rNotes,
				"created_at":  time.Unix(rCreatedAtUnix, 0).UTC().Format(time.RFC3339Nano),
				"updated_at":  time.Unix(rUpdatedAtUnix, 0).UTC().Format(time.RFC3339Nano),
			}
			if rInvoiceID.Valid && strings.TrimSpace(rInvoiceID.String) != "" {
				refund["invoice_id"] = rInvoiceID.String
			} else {
				refund["invoice_id"] = nil
			}
			if rExternalID.Valid && strings.TrimSpace(rExternalID.String) != "" {
				refund["external_refund_id"] = rExternalID.String
			} else {
				refund["external_refund_id"] = nil
			}
			if rSentTxID.Valid && strings.TrimSpace(rSentTxID.String) != "" {
				refund["sent_txid"] = rSentTxID.String
			} else {
				refund["sent_txid"] = nil
			}
			data["refund"] = refund
		} else {
			data["refund"] = map[string]any{"refund_id": strings.TrimSpace(*refundID)}
		}
	}
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}

	eventID, err := newID("evt")
	if err != nil {
		return err
	}
	ce := domain.CloudEvent{
		SpecVersion:     "1.0",
		ID:              eventID,
		Source:          "juno-pay-server",
		Type:            string(typ),
		Subject:         "invoice/" + invoiceID,
		Time:            occurredAt.UTC(),
		DataContentType: "application/json",
		Data:            dataBytes,
	}
	envBytes, err := json.Marshal(ce)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()

	if _, err := tx.ExecContext(ctx, s.q(`
		INSERT INTO outbox_events (event_id, merchant_id, envelope_json, created_at)
		VALUES (?, ?, ?, ?)
	`), eventID, merchantID, envBytes, nowUnix); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx, s.q(`
		SELECT sink_id
		FROM event_sinks
		WHERE merchant_id = ? AND status = ?
	`), merchantID, string(domain.EventSinkActive))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var sinkID string
		if err := rows.Scan(&sinkID); err != nil {
			return err
		}
		deliveryID, err := newID("del")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, s.q(`
			INSERT INTO event_deliveries (
				delivery_id, merchant_id, sink_id, event_id,
				status, attempt, next_retry_at, last_error,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`),
			deliveryID, merchantID, sinkID, eventID,
			string(domain.EventDeliveryPending), 0, nowUnix, nil,
			nowUnix, nowUnix,
		); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	return nil
}

func (s *Store) applyDepositEventTx(ctx context.Context, tx *sql.Tx, ev store.ScanEvent) error {
	now := time.Now().UTC()

	var (
		walletID         string
		txid             string
		actionIndex      int32
		recipientAddress string
		amountZat        int64
		height           int64
		status           string
	)

	switch types.WalletEventKind(ev.Kind) {
	case types.WalletEventKindDepositEvent:
		var p types.DepositEventPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		walletID = p.WalletID
		txid = p.TxID
		actionIndex = int32(p.ActionIndex)
		recipientAddress = p.RecipientAddress
		amountZat = int64(p.AmountZatoshis)
		height = p.Height
		status = "detected"
	case types.WalletEventKindDepositConfirmed:
		var p types.DepositConfirmedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		walletID = p.WalletID
		txid = p.TxID
		actionIndex = int32(p.ActionIndex)
		recipientAddress = p.RecipientAddress
		amountZat = int64(p.AmountZatoshis)
		height = p.Height
		// Note: do not mark deposits as confirmed here. Confirmation is per-invoice (required_confirmations)
		// and is handled by UpdateInvoiceConfirmations based on chain tip height.
		status = "detected"
	case types.WalletEventKindDepositUnconfirmed:
		var p types.DepositUnconfirmedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		walletID = p.WalletID
		txid = p.TxID
		actionIndex = int32(p.ActionIndex)
		recipientAddress = p.RecipientAddress
		amountZat = int64(p.AmountZatoshis)
		height = p.Height
		status = "unconfirmed"
	case types.WalletEventKindDepositOrphaned:
		var p types.DepositOrphanedPayload
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			return err
		}
		walletID = p.WalletID
		txid = p.TxID
		actionIndex = int32(p.ActionIndex)
		recipientAddress = p.RecipientAddress
		amountZat = int64(p.AmountZatoshis)
		height = p.Height
		status = "orphaned"
	default:
		return nil
	}

	addr := strings.ToLower(strings.TrimSpace(recipientAddress))
	addrHash := hash32Bytes(addr)
	clearConfirmedHeight := status == "unconfirmed" || status == "orphaned"

	// Find invoice by address (if any).
	var invoiceID sql.NullString
	var invoiceCreatedAfterHeight int64
	err := tx.QueryRowContext(ctx, s.q(`
		SELECT invoice_id, created_after_height
		FROM invoices
		WHERE wallet_id = ? AND address_hash = ?
		LIMIT 1
	`), walletID, addrHash).Scan(&invoiceID, &invoiceCreatedAfterHeight)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	applyInvoiceID := ""
	if err == nil && invoiceID.Valid && height > invoiceCreatedAfterHeight {
		applyInvoiceID = invoiceID.String
	}

	detectedAtUnix := ev.OccurredAt.UTC().Unix()
	if ev.OccurredAt.IsZero() {
		detectedAtUnix = now.Unix()
	}
	updatedAtUnix := now.Unix()

	var invoiceIDAny any = nil
	if applyInvoiceID != "" {
		invoiceIDAny = applyInvoiceID
	}

	res, err := tx.ExecContext(ctx, s.q(`
		UPDATE deposits
		SET recipient_address = ?,
		    recipient_address_hash = ?,
		    amount_zat = ?,
		    height = ?,
		    status = CASE
		      WHEN status = 'confirmed' AND ? = 'detected' THEN status
		      ELSE ?
		    END,
		    confirmed_height = CASE
		      WHEN ? THEN NULL
		      ELSE confirmed_height
		    END,
		    invoice_id = COALESCE(invoice_id, ?),
		    updated_at = ?
		WHERE wallet_id = ? AND txid = ? AND action_index = ?
	`),
		addr, addrHash, amountZat, height,
		status, status,
		clearConfirmedHeight,
		invoiceIDAny, updatedAtUnix,
		walletID, txid, actionIndex,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		inserted, err := s.execTxIgnoreUnique(ctx, tx, s.q(`
			INSERT INTO deposits (
				wallet_id, txid, action_index,
				recipient_address, recipient_address_hash, amount_zat, height,
				status, confirmed_height, invoice_id,
				detected_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), walletID, txid, actionIndex, addr, addrHash, amountZat, height, status, nil, invoiceIDAny, detectedAtUnix, updatedAtUnix)
		if err != nil {
			return err
		}
		if !inserted {
			// Race: retry update.
			if _, err := tx.ExecContext(ctx, s.q(`
				UPDATE deposits
				SET recipient_address = ?,
				    recipient_address_hash = ?,
				    amount_zat = ?,
				    height = ?,
				    status = CASE
				      WHEN status = 'confirmed' AND ? = 'detected' THEN status
				      ELSE ?
				    END,
				    confirmed_height = CASE
				      WHEN ? THEN NULL
				      ELSE confirmed_height
				    END,
				    invoice_id = COALESCE(invoice_id, ?),
				    updated_at = ?
				WHERE wallet_id = ? AND txid = ? AND action_index = ?
			`),
				addr, addrHash, amountZat, height,
				status, status,
				clearConfirmedHeight,
				invoiceIDAny, updatedAtUnix,
				walletID, txid, actionIndex,
			); err != nil {
				return err
			}
		}
	}

	if applyInvoiceID == "" {
		// Unknown address deposit (unattributed): create a manual review case if we can map wallet_id -> merchant.
		rows, err := tx.QueryContext(ctx, s.q(`
			SELECT merchant_id
			FROM merchant_wallets
			WHERE wallet_id = ?
		`), walletID)
		if err != nil {
			return err
		}
		defer rows.Close()

		merchantID := ""
		for rows.Next() {
			var mid string
			if err := rows.Scan(&mid); err != nil {
				return err
			}
			if merchantID != "" && merchantID != mid {
				return fmt.Errorf("wallet_id %q is mapped to multiple merchants", walletID)
			}
			merchantID = mid
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if merchantID == "" {
			return nil
		}

		reviewID, err := newID("rev")
		if err != nil {
			return err
		}
		notes := fmt.Sprintf("wallet_id=%s txid=%s action_index=%d recipient_address=%s amount_zat=%d height=%d",
			walletID, txid, actionIndex, addr, amountZat, height,
		)

		_, err = s.execTxIgnoreUnique(ctx, tx, s.q(`
			INSERT INTO review_cases (
				review_id, merchant_id, invoice_id,
				reason, status, notes,
				deposit_wallet_id, deposit_txid, deposit_action_index,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), reviewID, merchantID, nil,
			string(domain.ReviewUnknownAddress), string(domain.ReviewOpen), notes,
			walletID, txid, actionIndex,
			updatedAtUnix, updatedAtUnix,
		)
		return err
	}

	depRef := &domain.DepositRef{
		WalletID:    walletID,
		TxID:        txid,
		ActionIndex: actionIndex,
		AmountZat:   amountZat,
		Height:      height,
	}

	switch status {
	case "detected":
		if err := s.insertInvoiceEventTx(ctx, tx, applyInvoiceID, domain.InvoiceEventDepositDetected, ev.OccurredAt.UTC(), depRef, nil); err != nil {
			return err
		}
	case "confirmed":
		if err := s.insertInvoiceEventTx(ctx, tx, applyInvoiceID, domain.InvoiceEventDepositConfirmed, ev.OccurredAt.UTC(), depRef, nil); err != nil {
			return err
		}
	}

	return s.recomputeInvoiceAggregatesTx(ctx, tx, applyInvoiceID, now)
}

func (s *Store) recomputeInvoiceAggregatesTx(ctx context.Context, tx *sql.Tx, invoiceID string, now time.Time) error {
	if tx == nil {
		return errors.New("sqlite: nil tx")
	}
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	updatedAtUnix := now.Unix()

	// Recompute invoice aggregates.
	var pendingSum int64
	if err := tx.QueryRowContext(ctx, s.q(`
		SELECT COALESCE(SUM(amount_zat), 0)
		FROM deposits
		WHERE invoice_id = ? AND status IN ('detected','unconfirmed')
	`), invoiceID).Scan(&pendingSum); err != nil {
		return err
	}
	var confirmedSum int64
	if err := tx.QueryRowContext(ctx, s.q(`
		SELECT COALESCE(SUM(amount_zat), 0)
		FROM deposits
		WHERE invoice_id = ? AND status = 'confirmed'
	`), invoiceID).Scan(&confirmedSum); err != nil {
		return err
	}

	var (
		invMerchantID string
		invAmount     int64
		invStatus     string
		invExpires    sql.NullInt64
		invLatePolicy string
		invPartial    string
		invOverpay    string
	)
	if err := tx.QueryRowContext(ctx, s.q(`
		SELECT merchant_id, amount_zat, status, expires_at, policy_late_payment, policy_partial_payment, policy_overpayment
		FROM invoices
		WHERE invoice_id = ?
	`), invoiceID).Scan(&invMerchantID, &invAmount, &invStatus, &invExpires, &invLatePolicy, &invPartial, &invOverpay); err != nil {
		return err
	}

	expired := invExpires.Valid && now.Unix() > invExpires.Int64
	newStatus := computeInvoiceStatusSQL(invAmount, pendingSum, confirmedSum, expired, domain.LatePaymentPolicy(invLatePolicy))

	_, err := tx.ExecContext(ctx, s.q(`
		UPDATE invoices
		SET received_pending_zat = ?,
		    received_confirmed_zat = ?,
		    status = ?,
		    updated_at = ?
		WHERE invoice_id = ?
	`), pendingSum, confirmedSum, string(newStatus), updatedAtUnix, invoiceID)
	if err != nil {
		return err
	}

	if newStatus != domain.InvoiceStatus(invStatus) {
		switch newStatus {
		case domain.InvoiceExpired:
			if err := s.insertInvoiceEventTx(ctx, tx, invoiceID, domain.InvoiceEventInvoiceExpired, now, nil, nil); err != nil {
				return err
			}
		case domain.InvoiceConfirmed, domain.InvoicePaidLate:
			if err := s.insertInvoiceEventTx(ctx, tx, invoiceID, domain.InvoiceEventInvoicePaid, now, nil, nil); err != nil {
				return err
			}
		case domain.InvoiceOverpaid:
			if err := s.insertInvoiceEventTx(ctx, tx, invoiceID, domain.InvoiceEventInvoiceOverpaid, now, nil, nil); err != nil {
				return err
			}
		}

		invID := invoiceID
		switch {
		case newStatus == domain.InvoicePartialConfirmed && domain.PartialPaymentPolicy(invPartial) == domain.PartialPaymentReject:
			reviewID, err := newID("rev")
			if err != nil {
				return err
			}
			_, err = s.execTxIgnoreUnique(ctx, tx, s.q(`
				INSERT INTO review_cases (
					review_id, merchant_id, invoice_id,
					reason, status, notes,
					created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`), reviewID, invMerchantID, invID,
				string(domain.ReviewPartialPayment), string(domain.ReviewOpen), "partial payment requires review",
				updatedAtUnix, updatedAtUnix,
			)
			if err != nil {
				return err
			}
		case newStatus == domain.InvoiceOverpaid && domain.OverpaymentPolicy(invOverpay) == domain.OverpaymentManualReview:
			reviewID, err := newID("rev")
			if err != nil {
				return err
			}
			_, err = s.execTxIgnoreUnique(ctx, tx, s.q(`
				INSERT INTO review_cases (
					review_id, merchant_id, invoice_id,
					reason, status, notes,
					created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`), reviewID, invMerchantID, invID,
				string(domain.ReviewOverpayment), string(domain.ReviewOpen), "overpayment requires review",
				updatedAtUnix, updatedAtUnix,
			)
			if err != nil {
				return err
			}
		case (newStatus == domain.InvoiceConfirmed || newStatus == domain.InvoicePaidLate) &&
			expired && confirmedSum == invAmount && domain.LatePaymentPolicy(invLatePolicy) == domain.LatePaymentManualReview:
			reviewID, err := newID("rev")
			if err != nil {
				return err
			}
			_, err = s.execTxIgnoreUnique(ctx, tx, s.q(`
				INSERT INTO review_cases (
					review_id, merchant_id, invoice_id,
					reason, status, notes,
					created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			`), reviewID, invMerchantID, invID,
				string(domain.ReviewLatePayment), string(domain.ReviewOpen), "late payment requires review",
				updatedAtUnix, updatedAtUnix,
			)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func computeInvoiceStatusSQL(amountZat int64, pendingZat int64, confirmedZat int64, expired bool, latePolicy domain.LatePaymentPolicy) domain.InvoiceStatus {
	total := pendingZat + confirmedZat
	switch {
	case total == 0 && expired:
		return domain.InvoiceExpired
	case total == 0:
		return domain.InvoiceOpen
	case confirmedZat > amountZat:
		return domain.InvoiceOverpaid
	case confirmedZat == amountZat:
		if expired && latePolicy == domain.LatePaymentMarkPaidLate {
			return domain.InvoicePaidLate
		}
		return domain.InvoiceConfirmed
	case total >= amountZat:
		return domain.InvoicePending
	case confirmedZat > 0:
		return domain.InvoicePartialConfirmed
	default:
		return domain.InvoicePartialPending
	}
}

func (s *Store) encryptToken(invoiceID string, token string) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := s.aead.Seal(nil, nonce, []byte(token), []byte(invoiceID))
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func (s *Store) decryptToken(invoiceID string, enc []byte) (string, error) {
	ns := s.aead.NonceSize()
	if len(enc) < ns {
		return "", errors.New("sqlite: token ciphertext too short")
	}
	nonce := enc[:ns]
	ct := enc[ns:]
	pt, err := s.aead.Open(nil, nonce, ct, []byte(invoiceID))
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func newID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

var savepointSeq uint64

func (s *Store) execTxIgnoreUnique(ctx context.Context, tx *sql.Tx, query string, args ...any) (inserted bool, err error) {
	if tx == nil {
		return false, errors.New("sqlite: nil tx")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sp := fmt.Sprintf("sp_%d", atomic.AddUint64(&savepointSeq, 1))
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+sp); err != nil {
		return false, err
	}

	_, err = tx.ExecContext(ctx, query, args...)
	if err == nil {
		_, _ = tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp)
		return true, nil
	}

	if s != nil && s.dialect != nil && s.dialect.isUniqueViolation(err) {
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+sp); rbErr != nil {
			return false, rbErr
		}
		_, _ = tx.ExecContext(ctx, "RELEASE SAVEPOINT "+sp)
		return false, nil
	}

	return false, err
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// modernc.org/sqlite reports errors as strings; keep it simple and robust.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}

var baseIdents = []string{
	// Tables.
	"merchants",
	"merchant_wallets",
	"api_keys",
	"invoices",
	"invoice_tokens",
	"scan_cursors",
	"deposits",
	"refunds",
	"review_cases",
	"invoice_events",
	"event_sinks",
	"outbox_events",
	"event_deliveries",

	// Indexes.
	"idx_api_keys_token_hash",
	"idx_deposits_invoice_status",
	"idx_deposits_wallet_recipient_hash",
	"idx_invoices_merchant_seq",
	"idx_invoices_merchant_status_seq",
	"idx_refunds_merchant_id",
	"idx_refunds_invoice_id",
	"idx_review_cases_merchant_status",
	"idx_review_cases_invoice_unique",
	"idx_review_cases_deposit_unique",
	"idx_invoice_events_invoice_id",
	"idx_invoice_events_deposit_unique",
	"idx_invoice_events_refund_unique",
	"idx_event_sinks_merchant_id",
	"idx_outbox_events_merchant_id",
	"idx_event_deliveries_merchant_status_retry",
	"idx_event_deliveries_sink_id",
}

func (s *Store) q(query string) string {
	if s == nil || s.dialect == nil {
		return query
	}
	if s.rewriteMap != nil {
		query = rewriteSQLIdents(query, s.rewriteMap)
	}
	return s.dialect.rebind(query)
}

func rewriteSQLIdents(sqlText string, repl map[string]string) string {
	if len(repl) == 0 {
		return sqlText
	}
	var b strings.Builder
	b.Grow(len(sqlText) + 32)

	inSingle := false
	inDouble := false

	for i := 0; i < len(sqlText); {
		c := sqlText[i]
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			b.WriteByte(c)
			i++
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			b.WriteByte(c)
			i++
			continue
		}
		if inSingle || inDouble {
			b.WriteByte(c)
			i++
			continue
		}

		if isIdentStart(c) {
			j := i + 1
			for j < len(sqlText) && isIdentContinue(sqlText[j]) {
				j++
			}
			ident := sqlText[i:j]
			if v, ok := repl[ident]; ok {
				b.WriteString(v)
			} else {
				b.WriteString(ident)
			}
			i = j
			continue
		}

		b.WriteByte(c)
		i++
	}

	return b.String()
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentContinue(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func validateIdent(ident string) error {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return errors.New("empty")
	}
	if len(ident) > 63 {
		return errors.New("too long")
	}
	if !isIdentStart(ident[0]) {
		return errors.New("must start with letter or underscore")
	}
	for i := 1; i < len(ident); i++ {
		if !isIdentContinue(ident[i]) {
			return errors.New("invalid character")
		}
	}
	return nil
}
