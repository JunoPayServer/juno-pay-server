package sqlite

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
)

type Store struct {
	db   *sql.DB
	aead cipher.AEAD
}

func Open(dataDir string, tokenKey []byte) (*Store, error) {
	dataDir = filepath.Clean(strings.TrimSpace(dataDir))
	if dataDir == "" || dataDir == "." || dataDir == string(os.PathSeparator) {
		return nil, errors.New("sqlite: invalid data dir")
	}
	if len(tokenKey) != 32 {
		return nil, errors.New("sqlite: token key must be 32 bytes")
	}

	block, err := aes.NewCipher(tokenKey)
	if err != nil {
		return nil, fmt.Errorf("sqlite: aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("sqlite: gcm: %w", err)
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("sqlite: mkdir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "state.sqlite")
	dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	return &Store{db: db, aead: aead}, nil
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

	stmts := []string{
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
			invoice_id TEXT PRIMARY KEY,
			merchant_id TEXT NOT NULL,
			external_order_id TEXT NOT NULL,
			wallet_id TEXT NOT NULL,
			address_index INTEGER NOT NULL,
			address TEXT NOT NULL,
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
			UNIQUE (merchant_id, external_order_id),
			UNIQUE (wallet_id, address)
		)`,
		`CREATE TABLE IF NOT EXISTS invoice_tokens (
			invoice_id TEXT PRIMARY KEY,
			token_enc BLOB NOT NULL
		)`,
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return tx.Commit()
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

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO merchants (
			merchant_id, name, status,
			settings_invoice_ttl_seconds, settings_required_confirmations,
			settings_late_payment_policy, settings_partial_payment_policy, settings_overpayment_policy,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
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
	err := s.db.QueryRowContext(ctx, `
		SELECT name, status,
		       settings_invoice_ttl_seconds, settings_required_confirmations,
		       settings_late_payment_policy, settings_partial_payment_policy, settings_overpayment_policy,
		       created_at, updated_at
		FROM merchants
		WHERE merchant_id = ?
	`, merchantID).Scan(
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT merchant_id, name, status,
		       settings_invoice_ttl_seconds, settings_required_confirmations,
		       settings_late_payment_policy, settings_partial_payment_policy, settings_overpayment_policy,
		       created_at, updated_at
		FROM merchants
		ORDER BY merchant_id
	`)
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
	res, err := s.db.ExecContext(ctx, `
		UPDATE merchants
		SET settings_invoice_ttl_seconds = ?,
		    settings_required_confirmations = ?,
		    settings_late_payment_policy = ?,
		    settings_partial_payment_policy = ?,
		    settings_overpayment_policy = ?,
		    updated_at = ?
		WHERE merchant_id = ?
	`,
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
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM merchants WHERE merchant_id = ?`, merchantID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.MerchantWallet{}, store.ErrNotFound
		}
		return store.MerchantWallet{}, err
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO merchant_wallets (
			merchant_id, wallet_id, ufvk, chain, ua_hrp, coin_type, next_address_index, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, merchantID, w.WalletID, w.UFVK, w.Chain, w.UAHRP, w.CoinType, 0, nowUnix)
	if err != nil {
		if isUniqueViolation(err) {
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
	err := s.db.QueryRowContext(ctx, `
		SELECT wallet_id, ufvk, chain, ua_hrp, coin_type, created_at
		FROM merchant_wallets
		WHERE merchant_id = ?
	`, merchantID).Scan(&w.WalletID, &w.UFVK, &w.Chain, &w.UAHRP, &w.CoinType, &createdAtUnix)
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
	err = tx.QueryRowContext(ctx, `SELECT next_address_index FROM merchant_wallets WHERE merchant_id = ?`, merchantID).Scan(&idx)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, store.ErrNotFound
	}
	if err != nil {
		return 0, err
	}

	_, err = tx.ExecContext(ctx, `UPDATE merchant_wallets SET next_address_index = next_address_index + 1 WHERE merchant_id = ?`, merchantID)
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
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM merchants WHERE merchant_id = ?`, merchantID).Scan(&exists); err != nil {
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
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_keys (key_id, merchant_id, label, token_hash, revoked_at, created_at)
		VALUES (?, ?, ?, ?, NULL, ?)
	`, keyID, merchantID, label, hashHex, nowUnix)
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
	res, err := s.db.ExecContext(ctx, `
		UPDATE api_keys
		SET revoked_at = ?
		WHERE key_id = ? AND revoked_at IS NULL
	`, nowUnix, keyID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM api_keys WHERE key_id = ?`, keyID).Scan(&exists); err != nil {
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
	err = s.db.QueryRowContext(ctx, `
		SELECT merchant_id, revoked_at
		FROM api_keys
		WHERE token_hash = ?
	`, hashHex).Scan(&merchantID, &revokedAt)
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (
			invoice_id, merchant_id, external_order_id,
			wallet_id, address_index, address,
			created_after_height, created_after_hash,
			amount_zat, required_confirmations,
			policy_late_payment, policy_partial_payment, policy_overpayment,
			received_pending_zat, received_confirmed_zat,
			status, expires_at,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, ?, ?, ?, ?)
	`,
		id, req.MerchantID, req.ExternalOrderID,
		req.WalletID, req.AddressIndex, req.Address,
		req.CreatedAfterHeight, strings.TrimSpace(req.CreatedAfterHash),
		req.AmountZat, req.RequiredConfirmations,
		string(req.Policies.LatePayment), string(req.Policies.PartialPayment), string(req.Policies.Overpayment),
		string(domain.InvoiceOpen), expiresUnix,
		nowUnix, nowUnix,
	)
	if err == nil {
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

	if !isUniqueViolation(err) {
		return domain.Invoice{}, false, err
	}

	// Idempotent replay: fetch existing invoice by (merchant_id, external_order_id)
	existing, ok, err := s.findInvoiceTx(ctx, tx, req.MerchantID, req.ExternalOrderID)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	if !ok {
		return domain.Invoice{}, false, store.ErrConflict
	}
	if existing.AmountZat != req.AmountZat {
		return domain.Invoice{}, false, store.ErrConflict
	}
	return existing, false, nil
}

func (s *Store) GetInvoice(ctx context.Context, invoiceID string) (domain.Invoice, bool, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return domain.Invoice{}, false, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT invoice_id, merchant_id, external_order_id, wallet_id, address_index, address, created_after_height, created_after_hash,
		       amount_zat, required_confirmations,
		       policy_late_payment, policy_partial_payment, policy_overpayment,
		       received_pending_zat, received_confirmed_zat,
		       status, expires_at, created_at, updated_at
		FROM invoices
		WHERE invoice_id = ?
		LIMIT 1
	`, invoiceID)
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

	rows, err := s.db.QueryContext(ctx, `
		SELECT invoice_id, merchant_id, external_order_id, wallet_id, address_index, address, created_after_height, created_after_hash,
		       amount_zat, required_confirmations,
		       policy_late_payment, policy_partial_payment, policy_overpayment,
		       received_pending_zat, received_confirmed_zat,
		       status, expires_at, created_at, updated_at
		FROM invoices
		WHERE merchant_id = ? AND external_order_id = ?
		LIMIT 1
	`, merchantID, externalOrderID)
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
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM invoices WHERE invoice_id = ?`, invoiceID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.ErrNotFound
		}
		return err
	}

	enc, err := s.encryptToken(invoiceID, token)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO invoice_tokens (invoice_id, token_enc)
		VALUES (?, ?)
		ON CONFLICT(invoice_id) DO UPDATE SET token_enc = excluded.token_enc
	`, invoiceID, enc)
	if err != nil {
		return err
	}
	return nil
}

func (s *Store) GetInvoiceToken(ctx context.Context, invoiceID string) (token string, ok bool, err error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return "", false, nil
	}
	var enc []byte
	err = s.db.QueryRowContext(ctx, `SELECT token_enc FROM invoice_tokens WHERE invoice_id = ?`, invoiceID).Scan(&enc)
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

func (s *Store) findInvoiceTx(ctx context.Context, tx *sql.Tx, merchantID, externalOrderID string) (domain.Invoice, bool, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT invoice_id, merchant_id, external_order_id, wallet_id, address_index, address, created_after_height, created_after_hash,
		       amount_zat, required_confirmations,
		       policy_late_payment, policy_partial_payment, policy_overpayment,
		       received_pending_zat, received_confirmed_zat,
		       status, expires_at, created_at, updated_at
		FROM invoices
		WHERE merchant_id = ? AND external_order_id = ?
		LIMIT 1
	`, merchantID, externalOrderID)
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

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// modernc.org/sqlite reports errors as strings; keep it simple and robust.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}
