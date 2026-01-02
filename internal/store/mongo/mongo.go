package mongo

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
	"github.com/Abdullah1738/juno-sdk-go/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Store struct {
	client *mongo.Client
	db     *mongo.Database
	aead   cipher.AEAD

	prefix string
}

var _ store.Store = (*Store)(nil)

func (s *Store) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.client.Disconnect(ctx)
}

func (s *Store) Init(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("mongostore: nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	ensure := func(c *mongo.Collection, models ...mongo.IndexModel) error {
		if len(models) == 0 {
			return nil
		}
		_, err := c.Indexes().CreateMany(ctx, models)
		return err
	}

	// Collections.
	merchants := s.c("merchants")
	merchantWallets := s.c("merchant_wallets")
	apiKeys := s.c("api_keys")
	invoices := s.c("invoices")
	invoiceTokens := s.c("invoice_tokens")
	scanCursors := s.c("scan_cursors")
	deposits := s.c("deposits")
	refunds := s.c("refunds")
	reviewCases := s.c("review_cases")
	invoiceEvents := s.c("invoice_events")
	eventSinks := s.c("event_sinks")
	outboxEvents := s.c("outbox_events")
	eventDeliveries := s.c("event_deliveries")
	counters := s.c("counters")

	// Indexes.
	if err := ensure(merchants); err != nil {
		return err
	}
	if err := ensure(merchantWallets,
		mongo.IndexModel{Keys: bson.D{{Key: "wallet_id", Value: 1}}, Options: options.Index().SetUnique(true)},
	); err != nil {
		return err
	}
	if err := ensure(apiKeys,
		mongo.IndexModel{Keys: bson.D{{Key: "token_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
	); err != nil {
		return err
	}
	if err := ensure(invoices,
		mongo.IndexModel{Keys: bson.D{{Key: "seq", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}, {Key: "seq", Value: 1}}},
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}, {Key: "status", Value: 1}, {Key: "seq", Value: 1}}},
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}, {Key: "external_order_id_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongo.IndexModel{Keys: bson.D{{Key: "wallet_id", Value: 1}, {Key: "address_hash", Value: 1}}, Options: options.Index().SetUnique(true)},
	); err != nil {
		return err
	}
	if err := ensure(invoiceTokens); err != nil {
		return err
	}
	if err := ensure(scanCursors); err != nil {
		return err
	}
	if err := ensure(deposits,
		mongo.IndexModel{Keys: bson.D{{Key: "seq", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongo.IndexModel{Keys: bson.D{{Key: "wallet_id", Value: 1}, {Key: "txid", Value: 1}, {Key: "action_index", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongo.IndexModel{Keys: bson.D{{Key: "invoice_id", Value: 1}, {Key: "status", Value: 1}, {Key: "seq", Value: 1}}},
		mongo.IndexModel{Keys: bson.D{{Key: "wallet_id", Value: 1}, {Key: "recipient_address_hash", Value: 1}}},
	); err != nil {
		return err
	}
	if err := ensure(refunds,
		mongo.IndexModel{Keys: bson.D{{Key: "seq", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}, {Key: "seq", Value: 1}}},
		mongo.IndexModel{Keys: bson.D{{Key: "invoice_id", Value: 1}, {Key: "seq", Value: 1}}},
	); err != nil {
		return err
	}
	if err := ensure(reviewCases,
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}, {Key: "status", Value: 1}, {Key: "created_at", Value: 1}}},
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}, {Key: "invoice_id", Value: 1}, {Key: "reason", Value: 1}, {Key: "status", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongo.IndexModel{Keys: bson.D{{Key: "deposit_wallet_id", Value: 1}, {Key: "deposit_txid", Value: 1}, {Key: "deposit_action_index", Value: 1}, {Key: "reason", Value: 1}}, Options: options.Index().SetUnique(true)},
	); err != nil {
		return err
	}
	if err := ensure(invoiceEvents,
		mongo.IndexModel{Keys: bson.D{{Key: "invoice_id", Value: 1}, {Key: "_id", Value: 1}}},
		mongo.IndexModel{Keys: bson.D{{Key: "invoice_id", Value: 1}, {Key: "type", Value: 1}, {Key: "deposit_wallet_id", Value: 1}, {Key: "deposit_txid", Value: 1}, {Key: "deposit_action_index", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongo.IndexModel{Keys: bson.D{{Key: "invoice_id", Value: 1}, {Key: "type", Value: 1}, {Key: "refund_id", Value: 1}}, Options: options.Index().SetUnique(true)},
	); err != nil {
		return err
	}
	if err := ensure(eventSinks,
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}}},
	); err != nil {
		return err
	}
	if err := ensure(outboxEvents,
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}, {Key: "_id", Value: 1}}},
		mongo.IndexModel{Keys: bson.D{{Key: "event_id", Value: 1}}, Options: options.Index().SetUnique(true)},
	); err != nil {
		return err
	}
	if err := ensure(eventDeliveries,
		mongo.IndexModel{Keys: bson.D{{Key: "sink_id", Value: 1}, {Key: "event_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		mongo.IndexModel{Keys: bson.D{{Key: "merchant_id", Value: 1}, {Key: "status", Value: 1}, {Key: "next_retry_at", Value: 1}}},
		mongo.IndexModel{Keys: bson.D{{Key: "sink_id", Value: 1}}},
	); err != nil {
		return err
	}
	if err := ensure(counters); err != nil {
		return err
	}

	return nil
}

func (s *Store) c(name string) *mongo.Collection {
	if s == nil || s.db == nil {
		return nil
	}
	if s.prefix == "" {
		return s.db.Collection(name)
	}
	return s.db.Collection(s.prefix + "_" + name)
}

func (s *Store) nextSeq(ctx context.Context, sessCtx mongo.SessionContext, key string) (int64, error) {
	c := s.c("counters")

	filter := bson.M{"_id": key}
	update := bson.M{"$inc": bson.M{"v": int64(1)}}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	var doc struct {
		V int64 `bson:"v"`
	}
	var err error
	if sessCtx != nil {
		err = c.FindOneAndUpdate(sessCtx, filter, update, opts).Decode(&doc)
	} else {
		err = c.FindOneAndUpdate(ctx, filter, update, opts).Decode(&doc)
	}
	if err != nil {
		return 0, err
	}
	if doc.V < 1 {
		return 1, nil
	}
	return doc.V, nil
}

func hash32Bytes(v string) []byte {
	sum := sha256.Sum256([]byte(v))
	return sum[:]
}

func newID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
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
		return "", errors.New("mongostore: token ciphertext too short")
	}
	nonce := enc[:ns]
	ct := enc[ns:]
	pt, err := s.aead.Open(nil, nonce, ct, []byte(invoiceID))
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

func (s *Store) CreateMerchant(ctx context.Context, name string, settings domain.MerchantSettings) (domain.Merchant, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Merchant{}, domain.NewError(domain.ErrInvalidArgument, "name is required")
	}
	if err := settings.Validate(); err != nil {
		return domain.Merchant{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	id, err := newID("m")
	if err != nil {
		return domain.Merchant{}, err
	}
	now := time.Now().UTC()
	nowUnix := now.Unix()

	doc := bson.M{
		"_id":    id,
		"name":   name,
		"status": string(domain.MerchantActive),
		"settings": bson.M{
			"invoice_ttl_seconds":     settings.InvoiceTTLSeconds,
			"required_confirmations":  settings.RequiredConfirmations,
			"late_payment_policy":     string(settings.Policies.LatePayment),
			"partial_payment_policy":  string(settings.Policies.PartialPayment),
			"overpayment_policy":      string(settings.Policies.Overpayment),
		},
		"created_at": nowUnix,
		"updated_at": nowUnix,
	}

	if _, err := s.c("merchants").InsertOne(ctx, doc); err != nil {
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
	if ctx == nil {
		ctx = context.Background()
	}

	var doc struct {
		ID     string `bson:"_id"`
		Name   string `bson:"name"`
		Status string `bson:"status"`
		Settings struct {
			InvoiceTTLSeconds    int64  `bson:"invoice_ttl_seconds"`
			RequiredConfirmations int32 `bson:"required_confirmations"`
			LatePaymentPolicy    string `bson:"late_payment_policy"`
			PartialPaymentPolicy string `bson:"partial_payment_policy"`
			OverpaymentPolicy    string `bson:"overpayment_policy"`
		} `bson:"settings"`
		CreatedAt int64 `bson:"created_at"`
		UpdatedAt int64 `bson:"updated_at"`
	}

	err := s.c("merchants").FindOne(ctx, bson.M{"_id": merchantID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Merchant{}, false, nil
	}
	if err != nil {
		return domain.Merchant{}, false, err
	}

	m := domain.Merchant{
		MerchantID: doc.ID,
		Name:       doc.Name,
		Status:     domain.MerchantStatus(doc.Status),
		Settings: domain.MerchantSettings{
			InvoiceTTLSeconds:     doc.Settings.InvoiceTTLSeconds,
			RequiredConfirmations: doc.Settings.RequiredConfirmations,
			Policies: domain.InvoicePolicies{
				LatePayment:    domain.LatePaymentPolicy(doc.Settings.LatePaymentPolicy),
				PartialPayment: domain.PartialPaymentPolicy(doc.Settings.PartialPaymentPolicy),
				Overpayment:    domain.OverpaymentPolicy(doc.Settings.OverpaymentPolicy),
			},
		},
		CreatedAt: time.Unix(doc.CreatedAt, 0).UTC(),
		UpdatedAt: time.Unix(doc.UpdatedAt, 0).UTC(),
	}
	return m, true, nil
}

func (s *Store) ListMerchants(ctx context.Context) ([]domain.Merchant, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cur, err := s.c("merchants").Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []domain.Merchant
	for cur.Next(ctx) {
		var doc struct {
			ID     string `bson:"_id"`
			Name   string `bson:"name"`
			Status string `bson:"status"`
			Settings struct {
				InvoiceTTLSeconds    int64  `bson:"invoice_ttl_seconds"`
				RequiredConfirmations int32 `bson:"required_confirmations"`
				LatePaymentPolicy    string `bson:"late_payment_policy"`
				PartialPaymentPolicy string `bson:"partial_payment_policy"`
				OverpaymentPolicy    string `bson:"overpayment_policy"`
			} `bson:"settings"`
			CreatedAt int64 `bson:"created_at"`
			UpdatedAt int64 `bson:"updated_at"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, domain.Merchant{
			MerchantID: doc.ID,
			Name:       doc.Name,
			Status:     domain.MerchantStatus(doc.Status),
			Settings: domain.MerchantSettings{
				InvoiceTTLSeconds:     doc.Settings.InvoiceTTLSeconds,
				RequiredConfirmations: doc.Settings.RequiredConfirmations,
				Policies: domain.InvoicePolicies{
					LatePayment:    domain.LatePaymentPolicy(doc.Settings.LatePaymentPolicy),
					PartialPayment: domain.PartialPaymentPolicy(doc.Settings.PartialPaymentPolicy),
					Overpayment:    domain.OverpaymentPolicy(doc.Settings.OverpaymentPolicy),
				},
			},
			CreatedAt: time.Unix(doc.CreatedAt, 0).UTC(),
			UpdatedAt: time.Unix(doc.UpdatedAt, 0).UTC(),
		})
	}
	if err := cur.Err(); err != nil {
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
	if ctx == nil {
		ctx = context.Background()
	}

	nowUnix := time.Now().UTC().Unix()

	update := bson.M{
		"$set": bson.M{
			"settings": bson.M{
				"invoice_ttl_seconds":     settings.InvoiceTTLSeconds,
				"required_confirmations":  settings.RequiredConfirmations,
				"late_payment_policy":     string(settings.Policies.LatePayment),
				"partial_payment_policy":  string(settings.Policies.PartialPayment),
				"overpayment_policy":      string(settings.Policies.Overpayment),
			},
			"updated_at": nowUnix,
		},
	}
	res, err := s.c("merchants").UpdateOne(ctx, bson.M{"_id": merchantID}, update)
	if err != nil {
		return domain.Merchant{}, err
	}
	if res.MatchedCount == 0 {
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
	if ctx == nil {
		ctx = context.Background()
	}

	// Validate merchant exists.
	if _, ok, err := s.GetMerchant(ctx, merchantID); err != nil {
		return store.MerchantWallet{}, err
	} else if !ok {
		return store.MerchantWallet{}, store.ErrNotFound
	}

	now := time.Now().UTC()
	nowUnix := now.Unix()

	doc := bson.M{
		"_id":               merchantID,
		"wallet_id":         w.WalletID,
		"ufvk":              w.UFVK,
		"chain":             w.Chain,
		"ua_hrp":            w.UAHRP,
		"coin_type":         w.CoinType,
		"next_address_index": int64(0),
		"created_at":        nowUnix,
	}

	if _, err := s.c("merchant_wallets").InsertOne(ctx, doc); err != nil {
		if isDuplicateKey(err) {
			return store.MerchantWallet{}, store.ErrConflict
		}
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
	if ctx == nil {
		ctx = context.Background()
	}

	var doc struct {
		ID              string `bson:"_id"`
		WalletID        string `bson:"wallet_id"`
		UFVK            string `bson:"ufvk"`
		Chain           string `bson:"chain"`
		UAHRP           string `bson:"ua_hrp"`
		CoinType        int32  `bson:"coin_type"`
		CreatedAtUnix   int64  `bson:"created_at"`
	}
	err := s.c("merchant_wallets").FindOne(ctx, bson.M{"_id": merchantID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return store.MerchantWallet{}, false, nil
	}
	if err != nil {
		return store.MerchantWallet{}, false, err
	}
	return store.MerchantWallet{
		MerchantID: merchantID,
		WalletID:   doc.WalletID,
		UFVK:       doc.UFVK,
		Chain:      doc.Chain,
		UAHRP:      doc.UAHRP,
		CoinType:   doc.CoinType,
		CreatedAt:  time.Unix(doc.CreatedAtUnix, 0).UTC(),
	}, true, nil
}

func (s *Store) ListMerchantWallets(ctx context.Context) ([]store.MerchantWallet, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cur, err := s.c("merchant_wallets").Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []store.MerchantWallet
	for cur.Next(ctx) {
		var doc struct {
			ID            string `bson:"_id"`
			WalletID      string `bson:"wallet_id"`
			UFVK          string `bson:"ufvk"`
			Chain         string `bson:"chain"`
			UAHRP         string `bson:"ua_hrp"`
			CoinType      int32  `bson:"coin_type"`
			CreatedAtUnix int64  `bson:"created_at"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, store.MerchantWallet{
			MerchantID: doc.ID,
			WalletID:   doc.WalletID,
			UFVK:       doc.UFVK,
			Chain:      doc.Chain,
			UAHRP:      doc.UAHRP,
			CoinType:   doc.CoinType,
			CreatedAt:  time.Unix(doc.CreatedAtUnix, 0).UTC(),
		})
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) NextAddressIndex(ctx context.Context, merchantID string) (uint32, error) {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return 0, domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.Before)
	update := bson.M{"$inc": bson.M{"next_address_index": int64(1)}}

	var doc struct {
		Next int64 `bson:"next_address_index"`
	}
	err := s.c("merchant_wallets").FindOneAndUpdate(ctx, bson.M{"_id": merchantID}, update, opts).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, store.ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	if doc.Next < 0 {
		doc.Next = 0
	}
	return uint32(doc.Next), nil
}

func (s *Store) CreateMerchantAPIKey(ctx context.Context, merchantID, label string) (keyID string, apiKey string, err error) {
	merchantID = strings.TrimSpace(merchantID)
	label = strings.TrimSpace(label)
	if merchantID == "" {
		return "", "", domain.NewError(domain.ErrInvalidArgument, "merchant_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if _, ok, err := s.GetMerchant(ctx, merchantID); err != nil {
		return "", "", err
	} else if !ok {
		return "", "", store.ErrNotFound
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
	doc := bson.M{
		"_id":        keyID,
		"merchant_id": merchantID,
		"label":      label,
		"token_hash": hashHex,
		"created_at": nowUnix,
	}
	if _, err := s.c("api_keys").InsertOne(ctx, doc); err != nil {
		return "", "", err
	}
	return keyID, apiKey, nil
}

func (s *Store) RevokeMerchantAPIKey(ctx context.Context, keyID string) error {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "key_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	nowUnix := time.Now().UTC().Unix()
	filter := bson.M{"_id": keyID, "revoked_at": bson.M{"$exists": false}}
	update := bson.M{"$set": bson.M{"revoked_at": nowUnix}}
	res, err := s.c("api_keys").UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		// Either already revoked or not found; mimic sqlstore behavior: ErrNotFound only if missing.
		var exists struct{ ID string `bson:"_id"` }
		err := s.c("api_keys").FindOne(ctx, bson.M{"_id": keyID}).Decode(&exists)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return store.ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Store) LookupMerchantIDByAPIKey(ctx context.Context, apiKey string) (merchantID string, ok bool, err error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sum := sha256.Sum256([]byte(apiKey))
	hashHex := hex.EncodeToString(sum[:])

	var doc struct {
		MerchantID string `bson:"merchant_id"`
		RevokedAt  *int64 `bson:"revoked_at,omitempty"`
	}
	err = s.c("api_keys").FindOne(ctx, bson.M{"token_hash": hashHex}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if doc.RevokedAt != nil {
		return "", false, nil
	}
	return doc.MerchantID, true, nil
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
	if ctx == nil {
		ctx = context.Background()
	}

	id, err := newID("inv")
	if err != nil {
		return domain.Invoice{}, false, err
	}
	now := time.Now().UTC()
	nowUnix := now.Unix()

	var expiresUnix *int64
	if req.ExpiresAt != nil {
		v := req.ExpiresAt.UTC().Unix()
		expiresUnix = &v
	}

	extHash := hash32Bytes(req.ExternalOrderID)
	addrHash := hash32Bytes(req.Address)

	session, err := s.client.StartSession()
	if err != nil {
		return domain.Invoice{}, false, err
	}
	defer session.EndSession(ctx)

	var created bool
	var out domain.Invoice

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (any, error) {
		seq, err := s.nextSeq(ctx, sessCtx, "invoice_seq")
		if err != nil {
			return nil, err
		}

		invDoc := bson.M{
			"_id":                  id,
			"seq":                  seq,
			"merchant_id":          req.MerchantID,
			"external_order_id":    req.ExternalOrderID,
			"external_order_id_hash": extHash,
			"wallet_id":            req.WalletID,
			"address_index":        int64(req.AddressIndex),
			"address":              req.Address,
			"address_hash":         addrHash,
			"created_after_height": req.CreatedAfterHeight,
			"created_after_hash":   strings.TrimSpace(req.CreatedAfterHash),
			"amount_zat":           req.AmountZat,
			"required_confirmations": req.RequiredConfirmations,
			"policies": bson.M{
				"late_payment":    string(req.Policies.LatePayment),
				"partial_payment": string(req.Policies.PartialPayment),
				"overpayment":     string(req.Policies.Overpayment),
			},
			"received_pending_zat":   int64(0),
			"received_confirmed_zat": int64(0),
			"status":                 string(domain.InvoiceOpen),
			"expires_at":             expiresUnix,
			"created_at":             nowUnix,
			"updated_at":             nowUnix,
		}

		if _, err := s.c("invoices").InsertOne(sessCtx, invDoc); err != nil {
			return nil, err
		}

		if err := s.insertInvoiceEvent(sessCtx, id, domain.InvoiceEventInvoiceCreated, now, nil, nil); err != nil {
			return nil, err
		}

		created = true
		out = domain.Invoice{
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
		return nil, nil
	})
	if err == nil {
		return out, created, nil
	}
	if !isDuplicateKey(err) {
		return domain.Invoice{}, false, err
	}

	existing, ok, err := s.findInvoiceByExternal(ctx, req.MerchantID, req.ExternalOrderID, extHash)
	if err != nil {
		return domain.Invoice{}, false, err
	}
	if !ok {
		return domain.Invoice{}, false, store.ErrConflict
	}
	if existing.AmountZat != req.AmountZat || existing.WalletID != req.WalletID || existing.Address != req.Address {
		return domain.Invoice{}, false, store.ErrConflict
	}
	return existing, false, nil
}

func (s *Store) findInvoiceByExternal(ctx context.Context, merchantID, externalOrderID string, externalOrderIDHash []byte) (domain.Invoice, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{
		"merchant_id":            merchantID,
		"external_order_id_hash": externalOrderIDHash,
		"external_order_id":      externalOrderID,
	}
	var doc invoiceDoc
	err := s.c("invoices").FindOne(ctx, filter).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Invoice{}, false, nil
	}
	if err != nil {
		return domain.Invoice{}, false, err
	}
	return doc.toDomain(), true, nil
}

type invoiceDoc struct {
	ID                string `bson:"_id"`
	Seq               int64  `bson:"seq"`
	MerchantID        string `bson:"merchant_id"`
	ExternalOrderID   string `bson:"external_order_id"`
	WalletID          string `bson:"wallet_id"`
	AddressIndex      int64  `bson:"address_index"`
	Address           string `bson:"address"`
	CreatedAfterHeight int64 `bson:"created_after_height"`
	CreatedAfterHash  string `bson:"created_after_hash"`
	AmountZat         int64  `bson:"amount_zat"`
	RequiredConfirmations int32 `bson:"required_confirmations"`
	Policies struct {
		LatePayment    string `bson:"late_payment"`
		PartialPayment string `bson:"partial_payment"`
		Overpayment    string `bson:"overpayment"`
	} `bson:"policies"`
	ReceivedPendingZat   int64  `bson:"received_pending_zat"`
	ReceivedConfirmedZat int64  `bson:"received_confirmed_zat"`
	Status               string `bson:"status"`
	ExpiresAt            *int64 `bson:"expires_at"`
	CreatedAtUnix        int64  `bson:"created_at"`
	UpdatedAtUnix        int64  `bson:"updated_at"`
}

func (d invoiceDoc) toDomain() domain.Invoice {
	var expiresAt *time.Time
	if d.ExpiresAt != nil {
		t := time.Unix(*d.ExpiresAt, 0).UTC()
		expiresAt = &t
	}
	return domain.Invoice{
		InvoiceID:             d.ID,
		MerchantID:            d.MerchantID,
		ExternalOrderID:       d.ExternalOrderID,
		WalletID:              d.WalletID,
		AddressIndex:          uint32(d.AddressIndex),
		Address:               d.Address,
		CreatedAfterHeight:    d.CreatedAfterHeight,
		CreatedAfterHash:      d.CreatedAfterHash,
		AmountZat:             d.AmountZat,
		RequiredConfirmations: d.RequiredConfirmations,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentPolicy(d.Policies.LatePayment),
			PartialPayment: domain.PartialPaymentPolicy(d.Policies.PartialPayment),
			Overpayment:    domain.OverpaymentPolicy(d.Policies.Overpayment),
		},
		ReceivedPendingZat:   d.ReceivedPendingZat,
		ReceivedConfirmedZat: d.ReceivedConfirmedZat,
		Status:               domain.InvoiceStatus(d.Status),
		ExpiresAt:            expiresAt,
		CreatedAt:            time.Unix(d.CreatedAtUnix, 0).UTC(),
		UpdatedAt:            time.Unix(d.UpdatedAtUnix, 0).UTC(),
	}
}

func (s *Store) GetInvoice(ctx context.Context, invoiceID string) (domain.Invoice, bool, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return domain.Invoice{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var doc invoiceDoc
	err := s.c("invoices").FindOne(ctx, bson.M{"_id": invoiceID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Invoice{}, false, nil
	}
	if err != nil {
		return domain.Invoice{}, false, err
	}
	return doc.toDomain(), true, nil
}

func (s *Store) FindInvoiceByExternalOrderID(ctx context.Context, merchantID, externalOrderID string) (domain.Invoice, bool, error) {
	merchantID = strings.TrimSpace(merchantID)
	externalOrderID = strings.TrimSpace(externalOrderID)
	if merchantID == "" || externalOrderID == "" {
		return domain.Invoice{}, false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	extHash := hash32Bytes(externalOrderID)
	return s.findInvoiceByExternal(ctx, merchantID, externalOrderID, extHash)
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
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{"seq": bson.M{"$gt": f.AfterID}}
	if f.MerchantID != "" {
		filter["merchant_id"] = f.MerchantID
	}
	if f.Status != "" {
		filter["status"] = string(f.Status)
	}
	if f.ExternalOrderID != "" {
		filter["external_order_id_hash"] = hash32Bytes(f.ExternalOrderID)
		filter["external_order_id"] = f.ExternalOrderID
	}

	opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}}).SetLimit(int64(f.Limit))
	cur, err := s.c("invoices").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	out := make([]domain.Invoice, 0, f.Limit)
	var next int64
	for cur.Next(ctx) {
		var doc invoiceDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, 0, err
		}
		out = append(out, doc.toDomain())
		next = doc.Seq
	}
	if err := cur.Err(); err != nil {
		return nil, 0, err
	}
	return out, next, nil
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
	if ctx == nil {
		ctx = context.Background()
	}

	if _, ok, err := s.GetInvoice(ctx, invoiceID); err != nil {
		return err
	} else if !ok {
		return store.ErrNotFound
	}

	enc, err := s.encryptToken(invoiceID, token)
	if err != nil {
		return err
	}

	_, err = s.c("invoice_tokens").UpdateOne(
		ctx,
		bson.M{"_id": invoiceID},
		bson.M{"$set": bson.M{"token_enc": enc}},
		options.Update().SetUpsert(true),
	)
	return err
}

func (s *Store) GetInvoiceToken(ctx context.Context, invoiceID string) (token string, ok bool, err error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return "", false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var doc struct {
		Enc []byte `bson:"token_enc"`
	}
	err = s.c("invoice_tokens").FindOne(ctx, bson.M{"_id": invoiceID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	tok, err := s.decryptToken(invoiceID, doc.Enc)
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
	if ctx == nil {
		ctx = context.Background()
	}

	var doc struct {
		Cursor int64 `bson:"cursor"`
	}
	err = s.c("scan_cursors").FindOne(ctx, bson.M{"_id": walletID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return doc.Cursor, nil
}

func (s *Store) ScannerStatus(ctx context.Context) (lastCursor int64, lastEventAt *time.Time, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":        nil,
			"max_cursor": bson.M{"$max": "$cursor"},
			"max_event":  bson.M{"$max": "$last_event_at"},
		}}},
	}

	cur, err := s.c("scan_cursors").Aggregate(ctx, pipeline)
	if err != nil {
		return 0, nil, err
	}
	defer cur.Close(ctx)

	if !cur.Next(ctx) {
		return 0, nil, nil
	}
	var doc struct {
		MaxCursor int64  `bson:"max_cursor"`
		MaxEvent  *int64 `bson:"max_event"`
	}
	if err := cur.Decode(&doc); err != nil {
		return 0, nil, err
	}
	if doc.MaxEvent != nil {
		t := time.Unix(*doc.MaxEvent, 0).UTC()
		lastEventAt = &t
	}
	if doc.MaxCursor < 0 {
		doc.MaxCursor = 0
	}
	return doc.MaxCursor, lastEventAt, nil
}

func (s *Store) PendingDeliveries(ctx context.Context) (count int64, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.c("event_deliveries").CountDocuments(ctx, bson.M{"status": string(domain.EventDeliveryPending)})
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
	if ctx == nil {
		ctx = context.Background()
	}

	session, err := s.client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (any, error) {
		var curDoc struct {
			Cursor int64 `bson:"cursor"`
		}
		err := s.c("scan_cursors").FindOne(sessCtx, bson.M{"_id": ev.WalletID}).Decode(&curDoc)
		if errors.Is(err, mongo.ErrNoDocuments) {
			curDoc.Cursor = 0
			err = nil
		}
		if err != nil {
			return nil, err
		}
		if ev.Cursor <= curDoc.Cursor {
			return nil, nil
		}

		switch types.WalletEventKind(ev.Kind) {
		case types.WalletEventKindDepositEvent,
			types.WalletEventKindDepositConfirmed,
			types.WalletEventKindDepositUnconfirmed,
			types.WalletEventKindDepositOrphaned:
			if err := s.applyDepositEvent(sessCtx, ev); err != nil {
				return nil, err
			}
		default:
			// ignore
		}

		lastEventAt := ev.OccurredAt.UTC()
		if ev.OccurredAt.IsZero() {
			lastEventAt = time.Now().UTC()
		}
		lastUnix := lastEventAt.Unix()

		filter := bson.M{"_id": ev.WalletID}
		update := bson.M{"$set": bson.M{"cursor": ev.Cursor, "last_event_at": lastUnix}}
		_, err = s.c("scan_cursors").UpdateOne(sessCtx, filter, update, options.Update().SetUpsert(true))
		return nil, err
	})
	return err
}

func (s *Store) applyDepositEvent(sessCtx mongo.SessionContext, ev store.ScanEvent) error {
	now := time.Now().UTC()

	var (
		walletID         string
		txid             string
		actionIndex      int32
		recipientAddress string
		amountZat        int64
		height           int64
		status           string
		confirmedHeight  *int64
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
		status = "confirmed"
		ch := p.ConfirmedHeight
		confirmedHeight = &ch
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
	if addr == "" {
		return nil
	}
	addrHash := hash32Bytes(addr)

	// Find invoice by address.
	var invRef struct {
		ID                 string `bson:"_id"`
		CreatedAfterHeight int64  `bson:"created_after_height"`
	}
	err := s.c("invoices").FindOne(sessCtx, bson.M{"wallet_id": walletID, "address_hash": addrHash}).Decode(&invRef)
	applyInvoiceID := ""
	if err == nil && height > invRef.CreatedAfterHeight {
		applyInvoiceID = invRef.ID
	}
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return err
	}

	detectedAtUnix := ev.OccurredAt.UTC().Unix()
	if ev.OccurredAt.IsZero() {
		detectedAtUnix = now.Unix()
	}
	updatedAtUnix := now.Unix()

	filter := bson.M{"wallet_id": walletID, "txid": txid, "action_index": actionIndex}

	depSeq, err := s.nextSeq(sessCtx, sessCtx, "deposit_seq")
	if err != nil {
		return err
	}

	update := bson.M{
		"$set": bson.M{
			"recipient_address":      addr,
			"recipient_address_hash": addrHash,
			"amount_zat":             amountZat,
			"height":                 height,
			"status":                 status,
			"confirmed_height":       confirmedHeight,
			"updated_at":             updatedAtUnix,
		},
		"$setOnInsert": bson.M{
			"seq":         depSeq,
			"detected_at": detectedAtUnix,
		},
	}
	if applyInvoiceID != "" {
		update["$setOnInsert"].(bson.M)["invoice_id"] = applyInvoiceID
	}

	// Upsert deposit.
	if _, err := s.c("deposits").UpdateOne(sessCtx, filter, update, options.Update().SetUpsert(true)); err != nil {
		return err
	}

	// If we can now attribute this deposit to an invoice, set invoice_id once (COALESCE-like).
	if applyInvoiceID != "" {
		_, err := s.c("deposits").UpdateOne(
			sessCtx,
			bson.M{
				"wallet_id":     walletID,
				"txid":          txid,
				"action_index":  actionIndex,
				"$or": []bson.M{
					{"invoice_id": bson.M{"$exists": false}},
					{"invoice_id": nil},
					{"invoice_id": ""},
				},
			},
			bson.M{"$set": bson.M{"invoice_id": applyInvoiceID}},
		)
		if err != nil {
			return err
		}
	}

	if applyInvoiceID == "" {
		// Unknown address deposit: create review case if wallet maps to a merchant.
		var mw struct {
			MerchantID string `bson:"_id"`
		}
		err := s.c("merchant_wallets").FindOne(sessCtx, bson.M{"wallet_id": walletID}).Decode(&mw)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil
		}
		if err != nil {
			return err
		}

		reviewID, err := newID("rev")
		if err != nil {
			return err
		}
		notes := fmt.Sprintf("wallet_id=%s txid=%s action_index=%d recipient_address=%s amount_zat=%d height=%d",
			walletID, txid, actionIndex, addr, amountZat, height,
		)

		doc := bson.M{
			"_id":                 reviewID,
			"merchant_id":         mw.MerchantID,
			"invoice_id":          nil,
			"reason":              string(domain.ReviewUnknownAddress),
			"status":              string(domain.ReviewOpen),
			"notes":               notes,
			"deposit_wallet_id":   walletID,
			"deposit_txid":        txid,
			"deposit_action_index": actionIndex,
			"created_at":          updatedAtUnix,
			"updated_at":          updatedAtUnix,
		}
		if _, err := s.c("review_cases").InsertOne(sessCtx, doc); err != nil {
			if isDuplicateKey(err) {
				return nil
			}
			return err
		}
		return nil
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
		if err := s.insertInvoiceEvent(sessCtx, applyInvoiceID, domain.InvoiceEventDepositDetected, ev.OccurredAt.UTC(), depRef, nil); err != nil {
			return err
		}
	case "confirmed":
		if err := s.insertInvoiceEvent(sessCtx, applyInvoiceID, domain.InvoiceEventDepositConfirmed, ev.OccurredAt.UTC(), depRef, nil); err != nil {
			return err
		}
	}

	// Recompute invoice aggregates from deposits.
	pendingSum, err := s.sumDeposits(sessCtx, applyInvoiceID, []string{"detected", "unconfirmed"})
	if err != nil {
		return err
	}
	confirmedSum, err := s.sumDeposits(sessCtx, applyInvoiceID, []string{"confirmed"})
	if err != nil {
		return err
	}

	var invDoc struct {
		MerchantID string `bson:"merchant_id"`
		AmountZat  int64  `bson:"amount_zat"`
		Status     string `bson:"status"`
		ExpiresAt  *int64 `bson:"expires_at"`
		Policies   struct {
			LatePayment    string `bson:"late_payment"`
			PartialPayment string `bson:"partial_payment"`
			Overpayment    string `bson:"overpayment"`
		} `bson:"policies"`
	}
	if err := s.c("invoices").FindOne(sessCtx, bson.M{"_id": applyInvoiceID}).Decode(&invDoc); err != nil {
		return err
	}

	expired := invDoc.ExpiresAt != nil && now.Unix() > *invDoc.ExpiresAt
	newStatus := computeInvoiceStatusSQL(invDoc.AmountZat, confirmedSum, expired, domain.LatePaymentPolicy(invDoc.Policies.LatePayment))

	_, err = s.c("invoices").UpdateOne(sessCtx, bson.M{"_id": applyInvoiceID}, bson.M{
		"$set": bson.M{
			"received_pending_zat":   pendingSum,
			"received_confirmed_zat": confirmedSum,
			"status":                 string(newStatus),
			"updated_at":             updatedAtUnix,
		},
	})
	if err != nil {
		return err
	}

	if newStatus != domain.InvoiceStatus(invDoc.Status) {
		switch newStatus {
		case domain.InvoiceExpired:
			if err := s.insertInvoiceEvent(sessCtx, applyInvoiceID, domain.InvoiceEventInvoiceExpired, now, nil, nil); err != nil {
				return err
			}
		case domain.InvoicePaid, domain.InvoicePaidLate:
			if err := s.insertInvoiceEvent(sessCtx, applyInvoiceID, domain.InvoiceEventInvoicePaid, now, nil, nil); err != nil {
				return err
			}
		case domain.InvoiceOverpaid:
			if err := s.insertInvoiceEvent(sessCtx, applyInvoiceID, domain.InvoiceEventInvoiceOverpaid, now, nil, nil); err != nil {
				return err
			}
		}

		invID := applyInvoiceID
		switch {
		case newStatus == domain.InvoicePartial && domain.PartialPaymentPolicy(invDoc.Policies.PartialPayment) == domain.PartialPaymentReject:
			if err := s.createReviewCase(sessCtx, invDoc.MerchantID, &invID, domain.ReviewPartialPayment, "partial payment requires review", "", "", 0); err != nil {
				return err
			}
		case newStatus == domain.InvoiceOverpaid && domain.OverpaymentPolicy(invDoc.Policies.Overpayment) == domain.OverpaymentManualReview:
			if err := s.createReviewCase(sessCtx, invDoc.MerchantID, &invID, domain.ReviewOverpayment, "overpayment requires review", "", "", 0); err != nil {
				return err
			}
		case (newStatus == domain.InvoicePaid || newStatus == domain.InvoicePaidLate) &&
			expired && confirmedSum == invDoc.AmountZat && domain.LatePaymentPolicy(invDoc.Policies.LatePayment) == domain.LatePaymentManualReview:
			if err := s.createReviewCase(sessCtx, invDoc.MerchantID, &invID, domain.ReviewLatePayment, "late payment requires review", "", "", 0); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Store) sumDeposits(ctx context.Context, invoiceID string, statuses []string) (int64, error) {
	if invoiceID == "" {
		return 0, nil
	}
	filter := bson.M{
		"invoice_id": invoiceID,
		"status":     bson.M{"$in": statuses},
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$group", Value: bson.M{"_id": nil, "sum": bson.M{"$sum": "$amount_zat"}}}},
	}
	cur, err := s.c("deposits").Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cur.Close(ctx)
	if !cur.Next(ctx) {
		return 0, nil
	}
	var out struct {
		Sum int64 `bson:"sum"`
	}
	if err := cur.Decode(&out); err != nil {
		return 0, err
	}
	return out.Sum, nil
}

func computeInvoiceStatusSQL(amountZat int64, confirmedZat int64, expired bool, latePolicy domain.LatePaymentPolicy) domain.InvoiceStatus {
	switch {
	case confirmedZat == 0 && expired:
		return domain.InvoiceExpired
	case confirmedZat == 0:
		return domain.InvoiceOpen
	case confirmedZat < amountZat:
		return domain.InvoicePartial
	case confirmedZat == amountZat:
		if expired && latePolicy == domain.LatePaymentMarkPaidLate {
			return domain.InvoicePaidLate
		}
		return domain.InvoicePaid
	default:
		return domain.InvoiceOverpaid
	}
}

func (s *Store) insertInvoiceEvent(ctx mongo.SessionContext, invoiceID string, typ domain.InvoiceEventType, occurredAt time.Time, dep *domain.DepositRef, refundID *string) error {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return nil
	}

	now := time.Now().UTC()
	occurredAt = occurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = now
	}

	// Dedupe events with no deposit/refund explicitly.
	if dep == nil && (refundID == nil || strings.TrimSpace(*refundID) == "") {
		filter := bson.M{"invoice_id": invoiceID, "type": string(typ), "deposit_txid": bson.M{"$exists": false}, "refund_id": bson.M{"$exists": false}}
		err := s.c("invoice_events").FindOne(ctx, filter).Err()
		if err == nil {
			return nil
		}
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return err
		}
	}

	id, err := s.nextSeq(ctx, ctx, "invoice_event_seq")
	if err != nil {
		return err
	}

	doc := bson.M{
		"_id":         id,
		"invoice_id":  invoiceID,
		"type":        string(typ),
		"occurred_at": occurredAt.Unix(),
		"created_at":  now.Unix(),
	}
	if dep != nil {
		doc["deposit_wallet_id"] = dep.WalletID
		doc["deposit_txid"] = dep.TxID
		doc["deposit_action_index"] = dep.ActionIndex
		doc["deposit_amount_zat"] = dep.AmountZat
		doc["deposit_height"] = dep.Height
	}
	if refundID != nil && strings.TrimSpace(*refundID) != "" {
		doc["refund_id"] = strings.TrimSpace(*refundID)
	}

	if _, err := s.c("invoice_events").InsertOne(ctx, doc); err != nil {
		if isDuplicateKey(err) {
			return nil
		}
		return err
	}

	return s.enqueueOutbox(ctx, invoiceID, typ, occurredAt, dep, refundID)
}

func (s *Store) enqueueOutbox(ctx mongo.SessionContext, invoiceID string, typ domain.InvoiceEventType, occurredAt time.Time, dep *domain.DepositRef, refundID *string) error {
	var inv struct {
		MerchantID      string `bson:"merchant_id"`
		ExternalOrderID string `bson:"external_order_id"`
	}
	if err := s.c("invoices").FindOne(ctx, bson.M{"_id": invoiceID}).Decode(&inv); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil
		}
		return err
	}

	data := map[string]any{
		"merchant_id":       inv.MerchantID,
		"invoice_id":        invoiceID,
		"external_order_id": inv.ExternalOrderID,
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
		ref, ok, err := s.getRefund(ctx, strings.TrimSpace(*refundID))
		if err != nil {
			return err
		}
		if ok {
			data["refund"] = map[string]any{
				"refund_id":  ref.RefundID,
				"to_address": ref.ToAddress,
				"amount_zat": ref.AmountZat,
				"status":     string(ref.Status),
				"sent_txid":  ref.SentTxID,
				"notes":      ref.Notes,
			}
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

	seq, err := s.nextSeq(ctx, ctx, "outbox_seq")
	if err != nil {
		return err
	}

	outDoc := bson.M{
		"_id":          seq,
		"event_id":     eventID,
		"merchant_id":  inv.MerchantID,
		"envelope_json": envBytes,
		"created_at":   time.Now().UTC().Unix(),
	}
	if _, err := s.c("outbox_events").InsertOne(ctx, outDoc); err != nil {
		if !isDuplicateKey(err) {
			return err
		}
		// Extremely unlikely (event_id collision); treat as best-effort and continue.
	}

	// Create deliveries for active sinks.
	sinksCur, err := s.c("event_sinks").Find(ctx, bson.M{"merchant_id": inv.MerchantID, "status": string(domain.EventSinkActive)})
	if err != nil {
		return err
	}
	defer sinksCur.Close(ctx)

	now := time.Now().UTC()
	nowUnix := now.Unix()
	for sinksCur.Next(ctx) {
		var sink struct {
			ID string `bson:"_id"`
		}
		if err := sinksCur.Decode(&sink); err != nil {
			return err
		}
		deliveryID, err := newID("del")
		if err != nil {
			return err
		}
		delDoc := bson.M{
			"_id":         deliveryID,
			"merchant_id": inv.MerchantID,
			"sink_id":     sink.ID,
			"event_id":    eventID,
			"status":      string(domain.EventDeliveryPending),
			"attempt":     int32(0),
			"created_at":  nowUnix,
			"updated_at":  nowUnix,
		}
		if _, err := s.c("event_deliveries").InsertOne(ctx, delDoc); err != nil {
			if isDuplicateKey(err) {
				continue
			}
			return err
		}
	}
	if err := sinksCur.Err(); err != nil {
		return err
	}

	return nil
}

func (s *Store) getRefund(ctx context.Context, refundID string) (domain.Refund, bool, error) {
	var doc struct {
		ID              string `bson:"_id"`
		MerchantID      string `bson:"merchant_id"`
		InvoiceID       *string `bson:"invoice_id"`
		ExternalRefundID *string `bson:"external_refund_id"`
		ToAddress       string `bson:"to_address"`
		AmountZat       int64  `bson:"amount_zat"`
		Status          string `bson:"status"`
		SentTxID        *string `bson:"sent_txid"`
		Notes           string `bson:"notes"`
		CreatedAtUnix   int64  `bson:"created_at"`
		UpdatedAtUnix   int64  `bson:"updated_at"`
	}
	err := s.c("refunds").FindOne(ctx, bson.M{"_id": refundID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.Refund{}, false, nil
	}
	if err != nil {
		return domain.Refund{}, false, err
	}

	return domain.Refund{
		RefundID:         doc.ID,
		MerchantID:       doc.MerchantID,
		InvoiceID:        doc.InvoiceID,
		ExternalRefundID: doc.ExternalRefundID,
		ToAddress:        doc.ToAddress,
		AmountZat:        doc.AmountZat,
		Status:           domain.RefundStatus(doc.Status),
		SentTxID:         doc.SentTxID,
		Notes:            doc.Notes,
		CreatedAt:        time.Unix(doc.CreatedAtUnix, 0).UTC(),
		UpdatedAt:        time.Unix(doc.UpdatedAtUnix, 0).UTC(),
	}, true, nil
}

func (s *Store) ListInvoiceEvents(ctx context.Context, invoiceID string, afterID int64, limit int) (events []domain.InvoiceEvent, nextCursor int64, err error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return nil, 0, domain.NewError(domain.ErrInvalidArgument, "invoice_id is required")
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{
		"invoice_id": invoiceID,
		"_id":        bson.M{"$gt": afterID},
	}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit))
	cur, err := s.c("invoice_events").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var doc struct {
			ID             int64  `bson:"_id"`
			Type           string `bson:"type"`
			OccurredAtUnix int64  `bson:"occurred_at"`

			DepositWalletID    *string `bson:"deposit_wallet_id"`
			DepositTxID        *string `bson:"deposit_txid"`
			DepositActionIndex *int32  `bson:"deposit_action_index"`
			DepositAmountZat   *int64  `bson:"deposit_amount_zat"`
			DepositHeight      *int64  `bson:"deposit_height"`

			RefundID *string `bson:"refund_id"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, 0, err
		}

		var dep *domain.DepositRef
		if doc.DepositTxID != nil && strings.TrimSpace(*doc.DepositTxID) != "" {
			dep = &domain.DepositRef{
				WalletID:    deref(doc.DepositWalletID),
				TxID:        deref(doc.DepositTxID),
				ActionIndex: derefInt32(doc.DepositActionIndex),
				AmountZat:   derefInt64(doc.DepositAmountZat),
				Height:      derefInt64(doc.DepositHeight),
			}
		}

		var refund *domain.Refund
		if doc.RefundID != nil && strings.TrimSpace(*doc.RefundID) != "" {
			r, ok, err := s.getRefund(ctx, strings.TrimSpace(*doc.RefundID))
			if err != nil {
				return nil, 0, err
			}
			if ok {
				c := r
				refund = &c
			}
		}

		events = append(events, domain.InvoiceEvent{
			EventID:    strconv.FormatInt(doc.ID, 10),
			Type:       domain.InvoiceEventType(doc.Type),
			OccurredAt: time.Unix(doc.OccurredAtUnix, 0).UTC(),
			InvoiceID:  invoiceID,
			Deposit:    dep,
			Refund:     refund,
		})
		nextCursor = doc.ID
	}
	if err := cur.Err(); err != nil {
		return nil, 0, err
	}
	return events, nextCursor, nil
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
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
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{"seq": bson.M{"$gt": f.AfterID}}
	if f.TxID != "" {
		filter["txid"] = f.TxID
	}
	if f.InvoiceID != "" {
		filter["invoice_id"] = f.InvoiceID
	}
	if f.MerchantID != "" {
		var mw struct {
			WalletID string `bson:"wallet_id"`
		}
		err := s.c("merchant_wallets").FindOne(ctx, bson.M{"_id": f.MerchantID}).Decode(&mw)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, 0, nil
		}
		if err != nil {
			return nil, 0, err
		}
		filter["wallet_id"] = mw.WalletID
	}

	opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}}).SetLimit(int64(f.Limit))
	cur, err := s.c("deposits").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var doc struct {
			Seq             int64  `bson:"seq"`
			WalletID        string `bson:"wallet_id"`
			TxID            string `bson:"txid"`
			ActionIndex     int32  `bson:"action_index"`
			RecipientAddress string `bson:"recipient_address"`
			AmountZat       int64  `bson:"amount_zat"`
			Height          int64  `bson:"height"`
			Status          string `bson:"status"`
			ConfirmedHeight *int64 `bson:"confirmed_height"`
			InvoiceID       *string `bson:"invoice_id"`
			DetectedAt      int64  `bson:"detected_at"`
			UpdatedAt       int64  `bson:"updated_at"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, 0, err
		}

		var invoiceID *string
		if doc.InvoiceID != nil && strings.TrimSpace(*doc.InvoiceID) != "" {
			v := strings.TrimSpace(*doc.InvoiceID)
			invoiceID = &v
		}

		deposits = append(deposits, domain.Deposit{
			WalletID:         doc.WalletID,
			TxID:             doc.TxID,
			ActionIndex:      doc.ActionIndex,
			RecipientAddress: doc.RecipientAddress,
			AmountZat:        doc.AmountZat,
			Height:           doc.Height,
			Status:           domain.DepositStatus(doc.Status),
			ConfirmedHeight:  doc.ConfirmedHeight,
			InvoiceID:        invoiceID,
			DetectedAt:       time.Unix(doc.DetectedAt, 0).UTC(),
			UpdatedAt:        time.Unix(doc.UpdatedAt, 0).UTC(),
		})
		nextCursor = doc.Seq
	}
	if err := cur.Err(); err != nil {
		return nil, 0, err
	}
	return deposits, nextCursor, nil
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
	if ctx == nil {
		ctx = context.Background()
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

	session, err := s.client.StartSession()
	if err != nil {
		return domain.Refund{}, err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (any, error) {
		if _, ok, err := s.GetMerchant(sessCtx, req.MerchantID); err != nil {
			return nil, err
		} else if !ok {
			return nil, store.ErrNotFound
		}

		if req.InvoiceID != "" {
			inv, ok, err := s.GetInvoice(sessCtx, req.InvoiceID)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, store.ErrNotFound
			}
			if inv.MerchantID != req.MerchantID {
				return nil, store.ErrForbidden
			}
		}

		seq, err := s.nextSeq(ctx, sessCtx, "refund_seq")
		if err != nil {
			return nil, err
		}

		doc := bson.M{
			"_id":         refundID,
			"seq":         seq,
			"merchant_id": req.MerchantID,
			"invoice_id":  nil,
			"external_refund_id": nil,
			"to_address":  req.ToAddress,
			"amount_zat":  req.AmountZat,
			"status":      string(status),
			"sent_txid":   nil,
			"notes":       req.Notes,
			"created_at":  nowUnix,
			"updated_at":  nowUnix,
		}
		if req.InvoiceID != "" {
			doc["invoice_id"] = req.InvoiceID
		}
		if req.ExternalRefundID != "" {
			doc["external_refund_id"] = req.ExternalRefundID
		}
		if req.SentTxID != "" {
			doc["sent_txid"] = req.SentTxID
		}

		if _, err := s.c("refunds").InsertOne(sessCtx, doc); err != nil {
			return nil, err
		}

		if req.InvoiceID != "" {
			typ := domain.InvoiceEventRefundRequested
			if status == domain.RefundSent {
				typ = domain.InvoiceEventRefundSent
			}
			if err := s.insertInvoiceEvent(sessCtx, req.InvoiceID, typ, now, nil, &refundID); err != nil {
				return nil, err
			}
		}
		return nil, nil
	})
	if err != nil {
		return domain.Refund{}, err
	}

	var invID *string
	if req.InvoiceID != "" {
		v := req.InvoiceID
		invID = &v
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
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{"seq": bson.M{"$gt": f.AfterID}}
	if f.MerchantID != "" {
		filter["merchant_id"] = f.MerchantID
	}
	if f.InvoiceID != "" {
		filter["invoice_id"] = f.InvoiceID
	}
	if f.Status != "" {
		filter["status"] = string(f.Status)
	}

	opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}}).SetLimit(int64(f.Limit))
	cur, err := s.c("refunds").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var doc struct {
			ID              string `bson:"_id"`
			Seq             int64  `bson:"seq"`
			MerchantID      string `bson:"merchant_id"`
			InvoiceID       *string `bson:"invoice_id"`
			ExternalRefundID *string `bson:"external_refund_id"`
			ToAddress       string `bson:"to_address"`
			AmountZat       int64  `bson:"amount_zat"`
			Status          string `bson:"status"`
			SentTxID        *string `bson:"sent_txid"`
			Notes           string `bson:"notes"`
			CreatedAtUnix   int64  `bson:"created_at"`
			UpdatedAtUnix   int64  `bson:"updated_at"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, 0, err
		}

		refunds = append(refunds, domain.Refund{
			RefundID:         doc.ID,
			MerchantID:       doc.MerchantID,
			InvoiceID:        doc.InvoiceID,
			ExternalRefundID: doc.ExternalRefundID,
			ToAddress:        doc.ToAddress,
			AmountZat:        doc.AmountZat,
			Status:           domain.RefundStatus(doc.Status),
			SentTxID:         doc.SentTxID,
			Notes:            doc.Notes,
			CreatedAt:        time.Unix(doc.CreatedAtUnix, 0).UTC(),
			UpdatedAt:        time.Unix(doc.UpdatedAtUnix, 0).UTC(),
		})
		nextCursor = doc.Seq
	}
	if err := cur.Err(); err != nil {
		return nil, 0, err
	}
	return refunds, nextCursor, nil
}

func (s *Store) ListReviewCases(ctx context.Context, f store.ReviewCaseFilter) ([]domain.ReviewCase, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{}
	if f.MerchantID != "" {
		filter["merchant_id"] = f.MerchantID
	}
	if f.Status != "" {
		filter["status"] = string(f.Status)
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(1000)
	cur, err := s.c("review_cases").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []domain.ReviewCase
	for cur.Next(ctx) {
		var doc struct {
			ID         string  `bson:"_id"`
			MerchantID string  `bson:"merchant_id"`
			InvoiceID  *string `bson:"invoice_id"`
			Reason     string  `bson:"reason"`
			Status     string  `bson:"status"`
			Notes      string  `bson:"notes"`
			CreatedAt  int64   `bson:"created_at"`
			UpdatedAt  int64   `bson:"updated_at"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		var invID *string
		if doc.InvoiceID != nil && strings.TrimSpace(*doc.InvoiceID) != "" {
			v := strings.TrimSpace(*doc.InvoiceID)
			invID = &v
		}
		out = append(out, domain.ReviewCase{
			ReviewID:   doc.ID,
			MerchantID: doc.MerchantID,
			InvoiceID:  invID,
			Reason:     domain.ReviewReason(doc.Reason),
			Status:     domain.ReviewStatus(doc.Status),
			Notes:      doc.Notes,
			CreatedAt:  time.Unix(doc.CreatedAt, 0).UTC(),
			UpdatedAt:  time.Unix(doc.UpdatedAt, 0).UTC(),
		})
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ResolveReviewCase(ctx context.Context, reviewID string, notes string) error {
	return s.updateReviewCaseStatus(ctx, reviewID, domain.ReviewResolved, notes)
}

func (s *Store) RejectReviewCase(ctx context.Context, reviewID string, notes string) error {
	return s.updateReviewCaseStatus(ctx, reviewID, domain.ReviewRejected, notes)
}

func (s *Store) updateReviewCaseStatus(ctx context.Context, reviewID string, status domain.ReviewStatus, notes string) error {
	reviewID = strings.TrimSpace(reviewID)
	notes = strings.TrimSpace(notes)
	if reviewID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "review_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	nowUnix := time.Now().UTC().Unix()
	update := bson.M{"$set": bson.M{"status": string(status), "notes": notes, "updated_at": nowUnix}}
	res, err := s.c("review_cases").UpdateOne(ctx, bson.M{"_id": reviewID}, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
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
	cfgBytes, _ := json.Marshal(cfgAny)

	if ctx == nil {
		ctx = context.Background()
	}

	if _, ok, err := s.GetMerchant(ctx, req.MerchantID); err != nil {
		return domain.EventSink{}, err
	} else if !ok {
		return domain.EventSink{}, store.ErrNotFound
	}

	sinkID, err := newID("sink")
	if err != nil {
		return domain.EventSink{}, err
	}
	now := time.Now().UTC()
	nowUnix := now.Unix()

	doc := bson.M{
		"_id":         sinkID,
		"merchant_id": req.MerchantID,
		"kind":        string(req.Kind),
		"status":      string(domain.EventSinkActive),
		"config_json": cfgBytes,
		"created_at":  nowUnix,
		"updated_at":  nowUnix,
	}
	if _, err := s.c("event_sinks").InsertOne(ctx, doc); err != nil {
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
	if ctx == nil {
		ctx = context.Background()
	}

	var doc struct {
		ID         string `bson:"_id"`
		MerchantID string `bson:"merchant_id"`
		Kind       string `bson:"kind"`
		Status     string `bson:"status"`
		Config     []byte `bson:"config_json"`
		CreatedAt  int64  `bson:"created_at"`
		UpdatedAt  int64  `bson:"updated_at"`
	}
	err := s.c("event_sinks").FindOne(ctx, bson.M{"_id": sinkID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return domain.EventSink{}, false, nil
	}
	if err != nil {
		return domain.EventSink{}, false, err
	}
	return domain.EventSink{
		SinkID:     doc.ID,
		MerchantID: doc.MerchantID,
		Kind:       domain.EventSinkKind(doc.Kind),
		Status:     domain.EventSinkStatus(doc.Status),
		Config:     doc.Config,
		CreatedAt:  time.Unix(doc.CreatedAt, 0).UTC(),
		UpdatedAt:  time.Unix(doc.UpdatedAt, 0).UTC(),
	}, true, nil
}

func (s *Store) ListEventSinks(ctx context.Context, merchantID string) ([]domain.EventSink, error) {
	merchantID = strings.TrimSpace(merchantID)
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{}
	if merchantID != "" {
		filter["merchant_id"] = merchantID
	}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(1000)
	cur, err := s.c("event_sinks").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []domain.EventSink
	for cur.Next(ctx) {
		var doc struct {
			ID         string `bson:"_id"`
			MerchantID string `bson:"merchant_id"`
			Kind       string `bson:"kind"`
			Status     string `bson:"status"`
			Config     []byte `bson:"config_json"`
			CreatedAt  int64  `bson:"created_at"`
			UpdatedAt  int64  `bson:"updated_at"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}
		out = append(out, domain.EventSink{
			SinkID:     doc.ID,
			MerchantID: doc.MerchantID,
			Kind:       domain.EventSinkKind(doc.Kind),
			Status:     domain.EventSinkStatus(doc.Status),
			Config:     doc.Config,
			CreatedAt:  time.Unix(doc.CreatedAt, 0).UTC(),
			UpdatedAt:  time.Unix(doc.UpdatedAt, 0).UTC(),
		})
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListOutboundEvents(ctx context.Context, merchantID string, afterID int64, limit int) (events []domain.CloudEvent, nextCursor int64, err error) {
	merchantID = strings.TrimSpace(merchantID)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{"_id": bson.M{"$gt": afterID}}
	if merchantID != "" {
		filter["merchant_id"] = merchantID
	}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(int64(limit))
	cur, err := s.c("outbox_events").Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	for cur.Next(ctx) {
		var doc struct {
			Seq      int64  `bson:"_id"`
			Envelope []byte `bson:"envelope_json"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, 0, err
		}
		var ce domain.CloudEvent
		if err := json.Unmarshal(doc.Envelope, &ce); err != nil {
			return nil, 0, err
		}
		events = append(events, ce)
		nextCursor = doc.Seq
	}
	if err := cur.Err(); err != nil {
		return nil, 0, err
	}
	return events, nextCursor, nil
}

func (s *Store) ListEventDeliveries(ctx context.Context, f store.EventDeliveryFilter) ([]domain.EventDelivery, error) {
	f.MerchantID = strings.TrimSpace(f.MerchantID)
	f.SinkID = strings.TrimSpace(f.SinkID)
	if f.Limit <= 0 || f.Limit > 1000 {
		f.Limit = 100
	}
	if ctx == nil {
		ctx = context.Background()
	}

	filter := bson.M{}
	if f.MerchantID != "" {
		filter["merchant_id"] = f.MerchantID
	}
	if f.SinkID != "" {
		filter["sink_id"] = f.SinkID
	}
	if f.Status != "" {
		filter["status"] = string(f.Status)
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}}).SetLimit(int64(f.Limit))
	cur, err := s.c("event_deliveries").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []domain.EventDelivery
	for cur.Next(ctx) {
		var doc struct {
			ID          string  `bson:"_id"`
			SinkID      string  `bson:"sink_id"`
			EventID     string  `bson:"event_id"`
			Status      string  `bson:"status"`
			Attempt     int32   `bson:"attempt"`
			NextRetryAt *int64  `bson:"next_retry_at"`
			LastError   *string `bson:"last_error"`
			CreatedAt   int64   `bson:"created_at"`
			UpdatedAt   int64   `bson:"updated_at"`
		}
		if err := cur.Decode(&doc); err != nil {
			return nil, err
		}

		var next *time.Time
		if doc.NextRetryAt != nil {
			t := time.Unix(*doc.NextRetryAt, 0).UTC()
			next = &t
		}

		out = append(out, domain.EventDelivery{
			DeliveryID:  doc.ID,
			SinkID:      doc.SinkID,
			EventID:     doc.EventID,
			Status:      domain.EventDeliveryStatus(doc.Status),
			Attempt:     doc.Attempt,
			NextRetryAt: next,
			LastError:   doc.LastError,
			CreatedAt:   time.Unix(doc.CreatedAt, 0).UTC(),
			UpdatedAt:   time.Unix(doc.UpdatedAt, 0).UTC(),
		})
	}
	if err := cur.Err(); err != nil {
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
	if ctx == nil {
		ctx = context.Background()
	}

	nowUnix := now.UTC().Unix()
	filter := bson.M{
		"status": string(domain.EventDeliveryPending),
		"$or": []bson.M{
			{"next_retry_at": bson.M{"$exists": false}},
			{"next_retry_at": nil},
			{"next_retry_at": bson.M{"$lte": nowUnix}},
		},
	}
	opts := options.Find().SetSort(bson.D{{Key: "next_retry_at", Value: 1}, {Key: "created_at", Value: 1}}).SetLimit(int64(limit))
	cur, err := s.c("event_deliveries").Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var out []store.DueDelivery
	for cur.Next(ctx) {
		var d struct {
			ID          string  `bson:"_id"`
			MerchantID  string  `bson:"merchant_id"`
			SinkID      string  `bson:"sink_id"`
			EventID     string  `bson:"event_id"`
			Status      string  `bson:"status"`
			Attempt     int32   `bson:"attempt"`
			NextRetryAt *int64  `bson:"next_retry_at"`
			LastError   *string `bson:"last_error"`
			CreatedAt   int64   `bson:"created_at"`
			UpdatedAt   int64   `bson:"updated_at"`
		}
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}

		sink, ok, err := s.GetEventSink(ctx, d.SinkID)
		if err != nil {
			return nil, err
		}
		if !ok || sink.Status != domain.EventSinkActive {
			continue
		}

		var oe struct {
			Envelope []byte `bson:"envelope_json"`
		}
		err = s.c("outbox_events").FindOne(ctx, bson.M{"event_id": d.EventID}).Decode(&oe)
		if errors.Is(err, mongo.ErrNoDocuments) {
			continue
		}
		if err != nil {
			return nil, err
		}

		var ce domain.CloudEvent
		if err := json.Unmarshal(oe.Envelope, &ce); err != nil {
			return nil, err
		}

		var next *time.Time
		if d.NextRetryAt != nil {
			t := time.Unix(*d.NextRetryAt, 0).UTC()
			next = &t
		}

		out = append(out, store.DueDelivery{
			Delivery: domain.EventDelivery{
				DeliveryID:  d.ID,
				SinkID:      d.SinkID,
				EventID:     d.EventID,
				Status:      domain.EventDeliveryStatus(d.Status),
				Attempt:     d.Attempt,
				NextRetryAt: next,
				LastError:   d.LastError,
				CreatedAt:   time.Unix(d.CreatedAt, 0).UTC(),
				UpdatedAt:   time.Unix(d.UpdatedAt, 0).UTC(),
			},
			Sink:  sink,
			Event: ce,
		})
		if len(out) >= limit {
			break
		}
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) UpdateEventDelivery(ctx context.Context, deliveryID string, status domain.EventDeliveryStatus, attempt int32, nextRetryAt *time.Time, lastError *string) error {
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return domain.NewError(domain.ErrInvalidArgument, "delivery_id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	nowUnix := time.Now().UTC().Unix()
	var nextUnix any = nil
	if nextRetryAt != nil {
		nextUnix = nextRetryAt.UTC().Unix()
	}
	update := bson.M{
		"$set": bson.M{
			"status":        string(status),
			"attempt":       attempt,
			"next_retry_at": nextUnix,
			"last_error":    lastError,
			"updated_at":    nowUnix,
		},
	}

	res, err := s.c("event_deliveries").UpdateOne(ctx, bson.M{"_id": deliveryID}, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (s *Store) createReviewCase(ctx mongo.SessionContext, merchantID string, invoiceID *string, reason domain.ReviewReason, notes string, depWalletID, depTxID string, depActionIndex int32) error {
	merchantID = strings.TrimSpace(merchantID)
	if merchantID == "" {
		return nil
	}

	reviewID, err := newID("rev")
	if err != nil {
		return err
	}

	nowUnix := time.Now().UTC().Unix()
	doc := bson.M{
		"_id":         reviewID,
		"merchant_id": merchantID,
		"invoice_id":  invoiceID,
		"reason":      string(reason),
		"status":      string(domain.ReviewOpen),
		"notes":       strings.TrimSpace(notes),
		"created_at":  nowUnix,
		"updated_at":  nowUnix,
	}
	if depWalletID != "" && depTxID != "" {
		doc["deposit_wallet_id"] = depWalletID
		doc["deposit_txid"] = depTxID
		doc["deposit_action_index"] = depActionIndex
	}

	if _, err := s.c("review_cases").InsertOne(ctx, doc); err != nil {
		if isDuplicateKey(err) {
			return nil
		}
		return err
	}
	return nil
}

func isDuplicateKey(err error) bool {
	var we mongo.WriteException
	if errors.As(err, &we) {
		for _, e := range we.WriteErrors {
			if e.Code == 11000 {
				return true
			}
		}
	}
	var bwe mongo.BulkWriteException
	if errors.As(err, &bwe) {
		for _, e := range bwe.WriteErrors {
			if e.Code == 11000 {
				return true
			}
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "e11000")
}
