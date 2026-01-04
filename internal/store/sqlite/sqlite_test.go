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

	paid, _, err := s.ListInvoices(ctx, store.InvoiceFilter{MerchantID: m.MerchantID, Status: domain.InvoiceConfirmed, AfterID: 0, Limit: 10})
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

func TestStore_ListDeposits_FilterAndCursor(t *testing.T) {
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
	if _, err := s.SetMerchantWallet(ctx, m.MerchantID, store.MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	inv, _, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1a",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             10,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	p1 := types.DepositEventPayload{
		DepositEvent: types.DepositEvent{
			WalletID:       "w1",
			TxID:           "tx1",
			Height:         1,
			ActionIndex:    0,
			AmountZatoshis: 10,
		},
		RecipientAddress: "j1a",
	}
	b1, _ := json.Marshal(p1)
	if err := s.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       string(types.WalletEventKindDepositEvent),
		Payload:    b1,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent 1: %v", err)
	}

	p2 := types.DepositEventPayload{
		DepositEvent: types.DepositEvent{
			WalletID:       "w1",
			TxID:           "tx2",
			Height:         2,
			ActionIndex:    0,
			AmountZatoshis: 1,
		},
		RecipientAddress: "j1unknown",
	}
	b2, _ := json.Marshal(p2)
	if err := s.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     2,
		Kind:       string(types.WalletEventKindDepositEvent),
		Payload:    b2,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent 2: %v", err)
	}

	page1, cur1, err := s.ListDeposits(ctx, store.DepositFilter{MerchantID: m.MerchantID, AfterID: 0, Limit: 1})
	if err != nil {
		t.Fatalf("ListDeposits page1: %v", err)
	}
	if len(page1) != 1 || page1[0].TxID != "tx1" {
		t.Fatalf("expected first deposit tx1")
	}
	if page1[0].InvoiceID == nil || *page1[0].InvoiceID != inv.InvoiceID {
		t.Fatalf("expected tx1 invoice_id to match invoice")
	}

	page2, _, err := s.ListDeposits(ctx, store.DepositFilter{MerchantID: m.MerchantID, AfterID: cur1, Limit: 10})
	if err != nil {
		t.Fatalf("ListDeposits page2: %v", err)
	}
	if len(page2) != 1 || page2[0].TxID != "tx2" {
		t.Fatalf("expected second deposit tx2")
	}
	if page2[0].InvoiceID != nil {
		t.Fatalf("expected tx2 invoice_id to be nil")
	}

	onlyInv, _, err := s.ListDeposits(ctx, store.DepositFilter{MerchantID: m.MerchantID, InvoiceID: inv.InvoiceID, AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListDeposits onlyInv: %v", err)
	}
	if len(onlyInv) != 1 || onlyInv[0].TxID != "tx1" {
		t.Fatalf("expected only tx1 for invoice_id filter")
	}
}

func TestStore_Refunds_CreateListAndInvoiceEvents(t *testing.T) {
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

	inv, _, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1a",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             10,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	r1, err := s.CreateRefund(ctx, store.RefundCreate{
		MerchantID: m.MerchantID,
		InvoiceID:  inv.InvoiceID,
		ToAddress:  "j1dest",
		AmountZat:  1,
		Notes:      "n1",
	})
	if err != nil {
		t.Fatalf("CreateRefund r1: %v", err)
	}
	if r1.Status != domain.RefundRequested {
		t.Fatalf("expected requested status")
	}

	r2, err := s.CreateRefund(ctx, store.RefundCreate{
		MerchantID: m.MerchantID,
		InvoiceID:  inv.InvoiceID,
		ToAddress:  "j1dest2",
		AmountZat:  2,
		SentTxID:   "txsent",
	})
	if err != nil {
		t.Fatalf("CreateRefund r2: %v", err)
	}
	if r2.Status != domain.RefundSent {
		t.Fatalf("expected sent status")
	}

	refunds, _, err := s.ListRefunds(ctx, store.RefundFilter{MerchantID: m.MerchantID, AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListRefunds: %v", err)
	}
	if len(refunds) != 2 {
		t.Fatalf("expected 2 refunds")
	}

	events, _, err := s.ListInvoiceEvents(ctx, inv.InvoiceID, 0, 100)
	if err != nil {
		t.Fatalf("ListInvoiceEvents: %v", err)
	}
	var gotRequested, gotSent int
	for _, e := range events {
		switch e.Type {
		case domain.InvoiceEventRefundRequested:
			gotRequested++
			if e.Refund == nil || e.Refund.RefundID != r1.RefundID {
				t.Fatalf("expected refund.requested to include r1")
			}
		case domain.InvoiceEventRefundSent:
			gotSent++
			if e.Refund == nil || e.Refund.RefundID != r2.RefundID {
				t.Fatalf("expected refund.sent to include r2")
			}
		}
	}
	if gotRequested != 1 || gotSent != 1 {
		t.Fatalf("expected refund events requested=1 sent=1, got requested=%d sent=%d", gotRequested, gotSent)
	}

	outbox, _, err := s.ListOutboundEvents(ctx, m.MerchantID, 0, 100)
	if err != nil {
		t.Fatalf("ListOutboundEvents: %v", err)
	}
	var outboxRefunds int
	for _, e := range outbox {
		if e.Type == string(domain.InvoiceEventRefundRequested) || e.Type == string(domain.InvoiceEventRefundSent) {
			outboxRefunds++
		}
	}
	if outboxRefunds != 2 {
		t.Fatalf("expected 2 refund events in outbox, got %d", outboxRefunds)
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

func TestStore_ReviewCases_UnknownAddressDeposit(t *testing.T) {
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
	if _, err := s.SetMerchantWallet(ctx, m.MerchantID, store.MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	p1 := types.DepositEventPayload{
		DepositEvent: types.DepositEvent{
			WalletID:       "w1",
			TxID:           "tx1",
			Height:         1,
			ActionIndex:    0,
			AmountZatoshis: 123,
		},
		RecipientAddress: "j1unknown",
	}
	b1, _ := json.Marshal(p1)
	if err := s.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       string(types.WalletEventKindDepositEvent),
		Payload:    b1,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent deposit: %v", err)
	}

	p2 := types.DepositConfirmedPayload{
		DepositEventPayload: types.DepositEventPayload{
			DepositEvent: types.DepositEvent{
				WalletID:       "w1",
				TxID:           "tx1",
				Height:         1,
				ActionIndex:    0,
				AmountZatoshis: 123,
			},
			RecipientAddress: "j1unknown",
		},
		ConfirmedHeight:       1,
		RequiredConfirmations: 0,
	}
	b2, _ := json.Marshal(p2)
	if err := s.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     2,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    b2,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent confirmed: %v", err)
	}

	cs, err := s.ListReviewCases(ctx, store.ReviewCaseFilter{MerchantID: m.MerchantID, Status: domain.ReviewOpen})
	if err != nil {
		t.Fatalf("ListReviewCases: %v", err)
	}
	if len(cs) != 1 {
		t.Fatalf("expected 1 review case, got %d", len(cs))
	}
	if cs[0].Reason != domain.ReviewUnknownAddress {
		t.Fatalf("expected reason unknown_address, got %q", cs[0].Reason)
	}
	if cs[0].InvoiceID != nil {
		t.Fatalf("expected invoice_id nil")
	}
}

func TestStore_ReviewCases_FromInvoicePolicies(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	m, err := s.CreateMerchant(ctx, "acme", domain.MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentReject,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}
	if _, err := s.SetMerchantWallet(ctx, m.MerchantID, store.MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	expired := time.Now().UTC().Add(-1 * time.Second)
	invPartial, _, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-partial",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1partial",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             10,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentReject,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice partial: %v", err)
	}
	invOverpaid, _, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-overpaid",
		WalletID:              "w1",
		AddressIndex:          1,
		Address:               "j1overpaid",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             10,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentManualReview,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice overpaid: %v", err)
	}
	invLate, _, err := s.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-late",
		WalletID:              "w1",
		AddressIndex:          2,
		Address:               "j1late",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             10,
		RequiredConfirmations: 0,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentManualReview,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
		ExpiresAt: &expired,
	})
	if err != nil {
		t.Fatalf("CreateInvoice late: %v", err)
	}

	pp := types.DepositConfirmedPayload{
		DepositEventPayload: types.DepositEventPayload{
			DepositEvent: types.DepositEvent{
				WalletID:       "w1",
				TxID:           "tx_partial",
				Height:         1,
				ActionIndex:    0,
				AmountZatoshis: 5,
			},
			RecipientAddress: "j1partial",
		},
		ConfirmedHeight:       1,
		RequiredConfirmations: 0,
	}
	bpp, _ := json.Marshal(pp)
	if err := s.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    bpp,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent partial: %v", err)
	}

	op := types.DepositConfirmedPayload{
		DepositEventPayload: types.DepositEventPayload{
			DepositEvent: types.DepositEvent{
				WalletID:       "w1",
				TxID:           "tx_overpaid",
				Height:         1,
				ActionIndex:    0,
				AmountZatoshis: 11,
			},
			RecipientAddress: "j1overpaid",
		},
		ConfirmedHeight:       1,
		RequiredConfirmations: 0,
	}
	bop, _ := json.Marshal(op)
	if err := s.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     2,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    bop,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent overpaid: %v", err)
	}

	lp := types.DepositConfirmedPayload{
		DepositEventPayload: types.DepositEventPayload{
			DepositEvent: types.DepositEvent{
				WalletID:       "w1",
				TxID:           "tx_late",
				Height:         1,
				ActionIndex:    0,
				AmountZatoshis: 10,
			},
			RecipientAddress: "j1late",
		},
		ConfirmedHeight:       1,
		RequiredConfirmations: 0,
	}
	blp, _ := json.Marshal(lp)
	if err := s.ApplyScanEvent(ctx, store.ScanEvent{
		WalletID:   "w1",
		Cursor:     3,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    blp,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent late: %v", err)
	}

	cs, err := s.ListReviewCases(ctx, store.ReviewCaseFilter{MerchantID: m.MerchantID, Status: domain.ReviewOpen})
	if err != nil {
		t.Fatalf("ListReviewCases: %v", err)
	}
	if len(cs) != 3 {
		t.Fatalf("expected 3 review cases, got %d", len(cs))
	}

	byInv := map[string]domain.ReviewReason{}
	for _, c := range cs {
		if c.InvoiceID == nil {
			t.Fatalf("expected invoice_id for policy review cases")
		}
		byInv[*c.InvoiceID] = c.Reason
	}
	if byInv[invPartial.InvoiceID] != domain.ReviewPartialPayment {
		t.Fatalf("expected partial invoice to have partial_payment review case")
	}
	if byInv[invOverpaid.InvoiceID] != domain.ReviewOverpayment {
		t.Fatalf("expected overpaid invoice to have overpayment review case")
	}
	if byInv[invLate.InvoiceID] != domain.ReviewLatePayment {
		t.Fatalf("expected late invoice to have late_payment review case")
	}
}
