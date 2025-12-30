package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
	"github.com/Abdullah1738/juno-sdk-go/types"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	s, err := Open(t.TempDir(), key)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if err := s.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return s
}

func TestStore_MerchantWalletAndIndex(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	settings := domain.MerchantSettings{
		InvoiceTTLSeconds:     600,
		RequiredConfirmations: 10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	}

	m, err := s.CreateMerchant(ctx, "acme", settings)
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	if _, err := s.SetMerchantWallet(ctx, m.MerchantID, store.MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1js32zyfmmd4yzqy04pf9qwqrj47w3uvekjzs7pzfh2ars2v0ggzg74cd39lw9px0tr0nq7e86xevgx7fqxzslmlfqcaw28wj75prfgd0xdae7fywxl99n035kejzpj9upard7kegh3epjna7efmzy392cyr7a2hs4khc00zq0j2jqnnnz0usmuc92r5un",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	i0, err := s.NextAddressIndex(ctx, m.MerchantID)
	if err != nil {
		t.Fatalf("NextAddressIndex: %v", err)
	}
	if i0 != 0 {
		t.Fatalf("expected index 0, got %d", i0)
	}
	i1, err := s.NextAddressIndex(ctx, m.MerchantID)
	if err != nil {
		t.Fatalf("NextAddressIndex2: %v", err)
	}
	if i1 != 1 {
		t.Fatalf("expected index 1, got %d", i1)
	}
}

func TestStore_APIKeyLifecycle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	m, err := s.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	keyID, apiKey, err := s.CreateMerchantAPIKey(ctx, m.MerchantID, "default")
	if err != nil {
		t.Fatalf("CreateMerchantAPIKey: %v", err)
	}
	if keyID == "" || apiKey == "" {
		t.Fatalf("expected keyID and apiKey")
	}

	merchantID, ok, err := s.LookupMerchantIDByAPIKey(ctx, apiKey)
	if err != nil {
		t.Fatalf("LookupMerchantIDByAPIKey: %v", err)
	}
	if !ok || merchantID != m.MerchantID {
		t.Fatalf("lookup mismatch")
	}

	if err := s.RevokeMerchantAPIKey(ctx, keyID); err != nil {
		t.Fatalf("RevokeMerchantAPIKey: %v", err)
	}
	_, ok, err = s.LookupMerchantIDByAPIKey(ctx, apiKey)
	if err != nil {
		t.Fatalf("Lookup after revoke: %v", err)
	}
	if ok {
		t.Fatalf("expected revoked key to be invalid")
	}
}

func TestStore_InvoiceIdempotencyAndTokenRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	m, err := s.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     600,
		RequiredConfirmations: 10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}
	if _, err := s.SetMerchantWallet(ctx, m.MerchantID, store.MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1js32zyfmmd4yzqy04pf9qwqrj47w3uvekjzs7pzfh2ars2v0ggzg74cd39lw9px0tr0nq7e86xevgx7fqxzslmlfqcaw28wj75prfgd0xdae7fywxl99n035kejzpj9upard7kegh3epjna7efmzy392cyr7a2hs4khc00zq0j2jqnnnz0usmuc92r5un",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	inv, created, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
		CreatedAfterHeight:    100,
		CreatedAfterHash:      "h100",
		AmountZat:             123,
		RequiredConfirmations: 10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if !created {
		t.Fatalf("expected created true")
	}

	if err := s.PutInvoiceToken(ctx, inv.InvoiceID, "tok"); err != nil {
		t.Fatalf("PutInvoiceToken: %v", err)
	}
	got, ok, err := s.GetInvoiceToken(ctx, inv.InvoiceID)
	if err != nil {
		t.Fatalf("GetInvoiceToken: %v", err)
	}
	if !ok || got != "tok" {
		t.Fatalf("token mismatch")
	}

	inv2, created2, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
		CreatedAfterHeight:    100,
		CreatedAfterHash:      "h100",
		AmountZat:             123,
		RequiredConfirmations: 10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice retry: %v", err)
	}
	if created2 {
		t.Fatalf("expected created false on retry")
	}
	if inv2.InvoiceID != inv.InvoiceID {
		t.Fatalf("expected same invoice_id on retry")
	}

	_, _, err = s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
		CreatedAfterHeight:    100,
		CreatedAfterHash:      "h100",
		AmountZat:             124,
		RequiredConfirmations: 10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestStore_ListInvoices_FilterAndCursor(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	settings := domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	}

	m, err := s.CreateMerchant(ctx, "acme", settings)
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}

	i1, _, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1a",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             10,
		RequiredConfirmations: 0,
		Policies:              settings.Policies,
	})
	if err != nil {
		t.Fatalf("CreateInvoice i1: %v", err)
	}
	i2, _, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-2",
		WalletID:              "w1",
		AddressIndex:          1,
		Address:               "j1b",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             5,
		RequiredConfirmations: 0,
		Policies:              settings.Policies,
	})
	if err != nil {
		t.Fatalf("CreateInvoice i2: %v", err)
	}

	// Mark i2 as paid via scan ingestion.
	p := types.DepositConfirmedPayload{
		DepositEventPayload: types.DepositEventPayload{
			DepositEvent: types.DepositEvent{
				WalletID:       "w1",
				TxID:           "tx1",
				Height:         1,
				ActionIndex:    0,
				AmountZatoshis: 5,
			},
			RecipientAddress: "j1b",
		},
		ConfirmedHeight:       1,
		RequiredConfirmations: 0,
	}
	b, _ := json.Marshal(p)
	if err := s.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    b,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent: %v", err)
	}

	page1, cur1, err := s.ListInvoices(ctx, store.InvoiceFilter{MerchantID: m.MerchantID, AfterID: 0, Limit: 1})
	if err != nil {
		t.Fatalf("ListInvoices page1: %v", err)
	}
	if len(page1) != 1 || page1[0].InvoiceID != i1.InvoiceID {
		t.Fatalf("expected first invoice i1")
	}
	page2, cur2, err := s.ListInvoices(ctx, store.InvoiceFilter{MerchantID: m.MerchantID, AfterID: cur1, Limit: 10})
	if err != nil {
		t.Fatalf("ListInvoices page2: %v", err)
	}
	if len(page2) != 1 || page2[0].InvoiceID != i2.InvoiceID {
		t.Fatalf("expected second invoice i2")
	}
	if cur2 <= cur1 {
		t.Fatalf("expected cursor to advance")
	}

	paid, _, err := s.ListInvoices(ctx, store.InvoiceFilter{MerchantID: m.MerchantID, Status: domain.InvoicePaid, AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListInvoices paid: %v", err)
	}
	if len(paid) != 1 || paid[0].InvoiceID != i2.InvoiceID {
		t.Fatalf("expected only i2 to be paid")
	}

	byExt, _, err := s.ListInvoices(ctx, store.InvoiceFilter{MerchantID: m.MerchantID, ExternalOrderID: "order-1", AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListInvoices byExt: %v", err)
	}
	if len(byExt) != 1 || byExt[0].InvoiceID != i1.InvoiceID {
		t.Fatalf("expected only i1 for external_order_id")
	}
}

func TestStore_OutboxFromInvoiceEvents(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	m, err := s.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     600,
		RequiredConfirmations: 10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}
	if _, err := s.SetMerchantWallet(ctx, m.MerchantID, store.MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1js32zyfmmd4yzqy04pf9qwqrj47w3uvekjzs7pzfh2ars2v0ggzg74cd39lw9px0tr0nq7e86xevgx7fqxzslmlfqcaw28wj75prfgd0xdae7fywxl99n035kejzpj9upard7kegh3epjna7efmzy392cyr7a2hs4khc00zq0j2jqnnnz0usmuc92r5un",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	sink, err := s.CreateEventSink(ctx, store.EventSinkCreate{
		MerchantID: m.MerchantID,
		Kind:       domain.EventSinkWebhook,
		Config:     json.RawMessage(`{"url":"https://example.com/webhook","secret":"s"}`),
	})
	if err != nil {
		t.Fatalf("CreateEventSink: %v", err)
	}
	if sink.MerchantID != m.MerchantID || sink.SinkID == "" {
		t.Fatalf("unexpected sink: %+v", sink)
	}

	_, created, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
		CreatedAfterHeight:    100,
		CreatedAfterHash:      "h100",
		AmountZat:             123,
		RequiredConfirmations: 10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if !created {
		t.Fatalf("expected created true")
	}

	evs, _, err := s.ListOutboundEvents(ctx, m.MerchantID, 0, 100)
	if err != nil {
		t.Fatalf("ListOutboundEvents: %v", err)
	}
	if len(evs) == 0 {
		t.Fatalf("expected at least 1 outbound event")
	}
	if evs[0].Type != "invoice.created" {
		t.Fatalf("expected type invoice.created, got %q", evs[0].Type)
	}

	ds, err := s.ListEventDeliveries(ctx, store.EventDeliveryFilter{
		MerchantID: m.MerchantID,
		Limit:      100,
	})
	if err != nil {
		t.Fatalf("ListEventDeliveries: %v", err)
	}
	if len(ds) == 0 {
		t.Fatalf("expected at least 1 delivery")
	}
	if ds[0].SinkID != sink.SinkID {
		t.Fatalf("expected delivery sink_id=%q, got %q", sink.SinkID, ds[0].SinkID)
	}
	if ds[0].Status != domain.EventDeliveryPending {
		t.Fatalf("expected pending, got %q", ds[0].Status)
	}
}
