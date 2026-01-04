package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-sdk-go/types"
)

func TestMemStore_MerchantLifecycle(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	settings := domain.MerchantSettings{
		InvoiceTTLSeconds:     1800,
		RequiredConfirmations: 100,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	}

	m, err := st.CreateMerchant(ctx, "acme", settings)
	if err != nil {
		t.Fatalf("CreateMerchant: %v", err)
	}
	if m.MerchantID == "" {
		t.Fatalf("expected merchant_id")
	}

	got, ok, err := st.GetMerchant(ctx, m.MerchantID)
	if err != nil {
		t.Fatalf("GetMerchant: %v", err)
	}
	if !ok {
		t.Fatalf("expected merchant to exist")
	}
	if got.MerchantID != m.MerchantID {
		t.Fatalf("merchant_id mismatch")
	}

	updatedSettings := settings
	updatedSettings.InvoiceTTLSeconds = 60
	got2, err := st.UpdateMerchantSettings(ctx, m.MerchantID, updatedSettings)
	if err != nil {
		t.Fatalf("UpdateMerchantSettings: %v", err)
	}
	if got2.Settings.InvoiceTTLSeconds != 60 {
		t.Fatalf("expected settings to update")
	}
}

func TestMemStore_SetMerchantWallet_Immutable(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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

	if _, err := st.SetMerchantWallet(ctx, m.MerchantID, MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet first: %v", err)
	}

	if idx, err := st.NextAddressIndex(ctx, m.MerchantID); err != nil {
		t.Fatalf("NextAddressIndex: %v", err)
	} else if idx != 0 {
		t.Fatalf("expected idx=0, got %d", idx)
	}
	if idx, err := st.NextAddressIndex(ctx, m.MerchantID); err != nil {
		t.Fatalf("NextAddressIndex: %v", err)
	} else if idx != 1 {
		t.Fatalf("expected idx=1, got %d", idx)
	}

	if _, err := st.SetMerchantWallet(ctx, m.MerchantID, MerchantWallet{
		WalletID: "w2",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != ErrConflict {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestMemStore_CreateInvoice_IdempotentByExternalOrderID(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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

	req := InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1abc",
		CreatedAfterHeight:    100,
		CreatedAfterHash:      "hash100",
		AmountZat:             10,
		RequiredConfirmations: 100,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	}

	inv1, created, err := st.CreateInvoice(ctx, req)
	if err != nil {
		t.Fatalf("CreateInvoice first: %v", err)
	}
	if !created {
		t.Fatalf("expected created=true")
	}

	inv2, created, err := st.CreateInvoice(ctx, req)
	if err != nil {
		t.Fatalf("CreateInvoice retry: %v", err)
	}
	if created {
		t.Fatalf("expected created=false on retry")
	}
	if inv2.InvoiceID != inv1.InvoiceID {
		t.Fatalf("expected same invoice_id on retry")
	}

	req2 := req
	req2.AmountZat = 11
	if _, _, err := st.CreateInvoice(ctx, req2); err != ErrConflict {
		t.Fatalf("expected ErrConflict for different amount, got %v", err)
	}
}

func TestMemStore_CreateInvoice_AddressUniquePerWallet(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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

	req1 := InvoiceCreate{
		MerchantID:      m.MerchantID,
		ExternalOrderID: "order-1",
		WalletID:        "w1",
		AddressIndex:    1,
		Address:         "j1same",
		AmountZat:       10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	}
	if _, _, err := st.CreateInvoice(ctx, req1); err != nil {
		t.Fatalf("CreateInvoice first: %v", err)
	}

	req2 := req1
	req2.ExternalOrderID = "order-2"
	req2.AddressIndex = 2
	if _, _, err := st.CreateInvoice(ctx, req2); err != ErrConflict {
		t.Fatalf("expected ErrConflict for reused address, got %v", err)
	}
}

func TestMemStore_APIKey_Lifecycle(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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

	keyID, apiKey, err := st.CreateMerchantAPIKey(ctx, m.MerchantID, "default")
	if err != nil {
		t.Fatalf("CreateMerchantAPIKey: %v", err)
	}
	if keyID == "" || apiKey == "" {
		t.Fatalf("expected keyID and apiKey")
	}

	merchantID, ok, err := st.LookupMerchantIDByAPIKey(ctx, apiKey)
	if err != nil {
		t.Fatalf("LookupMerchantIDByAPIKey: %v", err)
	}
	if !ok || merchantID != m.MerchantID {
		t.Fatalf("expected api key to resolve to merchant")
	}

	if err := st.RevokeMerchantAPIKey(ctx, keyID); err != nil {
		t.Fatalf("RevokeMerchantAPIKey: %v", err)
	}
	_, ok, err = st.LookupMerchantIDByAPIKey(ctx, apiKey)
	if err != nil {
		t.Fatalf("LookupMerchantIDByAPIKey after revoke: %v", err)
	}
	if ok {
		t.Fatalf("expected revoked api key to be invalid")
	}
}

func TestMemStore_InvoiceToken(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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

	inv, _, err := st.CreateInvoice(ctx, InvoiceCreate{
		MerchantID:      m.MerchantID,
		ExternalOrderID: "order-1",
		WalletID:        "w1",
		AddressIndex:    0,
		Address:         "j1abc",
		AmountZat:       10,
		Policies: domain.InvoicePolicies{
			LatePayment:    domain.LatePaymentMarkPaidLate,
			PartialPayment: domain.PartialPaymentAccept,
			Overpayment:    domain.OverpaymentMarkOverpaid,
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	if err := st.PutInvoiceToken(ctx, inv.InvoiceID, "tok"); err != nil {
		t.Fatalf("PutInvoiceToken: %v", err)
	}
	tok, ok, err := st.GetInvoiceToken(ctx, inv.InvoiceID)
	if err != nil {
		t.Fatalf("GetInvoiceToken: %v", err)
	}
	if !ok || tok != "tok" {
		t.Fatalf("unexpected token result")
	}
}

func TestMemStore_ListInvoices_FilterAndCursor(t *testing.T) {
	st := NewMem()
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

	m1, err := st.CreateMerchant(ctx, "m1", settings)
	if err != nil {
		t.Fatalf("CreateMerchant m1: %v", err)
	}
	m2, err := st.CreateMerchant(ctx, "m2", settings)
	if err != nil {
		t.Fatalf("CreateMerchant m2: %v", err)
	}

	i1, _, err := st.CreateInvoice(ctx, InvoiceCreate{
		MerchantID:            m1.MerchantID,
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
	i2, _, err := st.CreateInvoice(ctx, InvoiceCreate{
		MerchantID:            m1.MerchantID,
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
	if _, _, err := st.CreateInvoice(ctx, InvoiceCreate{
		MerchantID:            m2.MerchantID,
		ExternalOrderID:       "order-3",
		WalletID:              "w2",
		AddressIndex:          0,
		Address:               "j1c",
		CreatedAfterHeight:    0,
		CreatedAfterHash:      "h0",
		AmountZat:             7,
		RequiredConfirmations: 0,
		Policies:              settings.Policies,
	}); err != nil {
		t.Fatalf("CreateInvoice i3: %v", err)
	}

	// Mark i2 as paid.
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
	if err := st.ApplyScanEvent(ctx, ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    b,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent: %v", err)
	}

	// Cursor paging (m1 only).
	page1, cur1, err := st.ListInvoices(ctx, InvoiceFilter{MerchantID: m1.MerchantID, AfterID: 0, Limit: 1})
	if err != nil {
		t.Fatalf("ListInvoices page1: %v", err)
	}
	if len(page1) != 1 || page1[0].InvoiceID != i1.InvoiceID {
		t.Fatalf("expected first invoice i1")
	}
	page2, cur2, err := st.ListInvoices(ctx, InvoiceFilter{MerchantID: m1.MerchantID, AfterID: cur1, Limit: 10})
	if err != nil {
		t.Fatalf("ListInvoices page2: %v", err)
	}
	if len(page2) != 1 || page2[0].InvoiceID != i2.InvoiceID {
		t.Fatalf("expected second invoice i2")
	}
	if cur2 <= cur1 {
		t.Fatalf("expected cursor to advance")
	}

	// Status filter.
	paid, _, err := st.ListInvoices(ctx, InvoiceFilter{MerchantID: m1.MerchantID, Status: domain.InvoiceConfirmed, AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListInvoices paid: %v", err)
	}
	if len(paid) != 1 || paid[0].InvoiceID != i2.InvoiceID {
		t.Fatalf("expected only i2 to be paid")
	}

	// External order ID filter.
	byExt, _, err := st.ListInvoices(ctx, InvoiceFilter{MerchantID: m1.MerchantID, ExternalOrderID: "order-1", AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListInvoices byExt: %v", err)
	}
	if len(byExt) != 1 || byExt[0].InvoiceID != i1.InvoiceID {
		t.Fatalf("expected only i1 for external_order_id")
	}
}

func TestMemStore_ListDeposits_FilterAndCursor(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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
	if _, err := st.SetMerchantWallet(ctx, m.MerchantID, MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	inv, _, err := st.CreateInvoice(ctx, InvoiceCreate{
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

	// Deposit 1 matches invoice.
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
	if err := st.ApplyScanEvent(ctx, ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       string(types.WalletEventKindDepositEvent),
		Payload:    b1,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent 1: %v", err)
	}

	// Deposit 2 is an unknown address (no invoice mapping).
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
	if err := st.ApplyScanEvent(ctx, ScanEvent{
		WalletID:   "w1",
		Cursor:     2,
		Kind:       string(types.WalletEventKindDepositEvent),
		Payload:    b2,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent 2: %v", err)
	}

	page1, cur1, err := st.ListDeposits(ctx, DepositFilter{MerchantID: m.MerchantID, AfterID: 0, Limit: 1})
	if err != nil {
		t.Fatalf("ListDeposits page1: %v", err)
	}
	if len(page1) != 1 || page1[0].TxID != "tx1" {
		t.Fatalf("expected first deposit tx1")
	}
	if page1[0].InvoiceID == nil || *page1[0].InvoiceID != inv.InvoiceID {
		t.Fatalf("expected tx1 invoice_id to match invoice")
	}

	page2, _, err := st.ListDeposits(ctx, DepositFilter{MerchantID: m.MerchantID, AfterID: cur1, Limit: 10})
	if err != nil {
		t.Fatalf("ListDeposits page2: %v", err)
	}
	if len(page2) != 1 || page2[0].TxID != "tx2" {
		t.Fatalf("expected second deposit tx2")
	}
	if page2[0].InvoiceID != nil {
		t.Fatalf("expected tx2 invoice_id to be nil")
	}

	onlyInv, _, err := st.ListDeposits(ctx, DepositFilter{MerchantID: m.MerchantID, InvoiceID: inv.InvoiceID, AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListDeposits onlyInv: %v", err)
	}
	if len(onlyInv) != 1 || onlyInv[0].TxID != "tx1" {
		t.Fatalf("expected only tx1 for invoice_id filter")
	}
}

func TestMemStore_Refunds_CreateListAndInvoiceEvents(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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

	inv, _, err := st.CreateInvoice(ctx, InvoiceCreate{
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

	r1, err := st.CreateRefund(ctx, RefundCreate{
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
	if r1.InvoiceID == nil || *r1.InvoiceID != inv.InvoiceID {
		t.Fatalf("expected invoice_id on refund")
	}

	r2, err := st.CreateRefund(ctx, RefundCreate{
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
	if r2.SentTxID == nil || *r2.SentTxID != "txsent" {
		t.Fatalf("expected sent_txid")
	}

	refunds, _, err := st.ListRefunds(ctx, RefundFilter{MerchantID: m.MerchantID, AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListRefunds: %v", err)
	}
	if len(refunds) != 2 {
		t.Fatalf("expected 2 refunds")
	}

	sentOnly, _, err := st.ListRefunds(ctx, RefundFilter{MerchantID: m.MerchantID, Status: domain.RefundSent, AfterID: 0, Limit: 10})
	if err != nil {
		t.Fatalf("ListRefunds sentOnly: %v", err)
	}
	if len(sentOnly) != 1 || sentOnly[0].RefundID != r2.RefundID {
		t.Fatalf("expected only r2 when filtering by status=sent")
	}

	events, _, err := st.ListInvoiceEvents(ctx, inv.InvoiceID, 0, 100)
	if err != nil {
		t.Fatalf("ListInvoiceEvents: %v", err)
	}
	var gotRequested, gotSent int
	for _, e := range events {
		switch e.Type {
		case domain.InvoiceEventRefundRequested:
			gotRequested++
		case domain.InvoiceEventRefundSent:
			gotSent++
		}
	}
	if gotRequested != 1 || gotSent != 1 {
		t.Fatalf("expected refund events requested=1 sent=1, got requested=%d sent=%d", gotRequested, gotSent)
	}

	outbox, _, err := st.ListOutboundEvents(ctx, m.MerchantID, 0, 100)
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

func TestMemStore_ReviewCases_UnknownAddressDeposit(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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
	if _, err := st.SetMerchantWallet(ctx, m.MerchantID, MerchantWallet{
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
	if err := st.ApplyScanEvent(ctx, ScanEvent{
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
	if err := st.ApplyScanEvent(ctx, ScanEvent{
		WalletID:   "w1",
		Cursor:     2,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    b2,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent confirmed: %v", err)
	}

	cs, err := st.ListReviewCases(ctx, ReviewCaseFilter{MerchantID: m.MerchantID, Status: domain.ReviewOpen})
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

func TestMemStore_ReviewCases_FromInvoicePolicies(t *testing.T) {
	st := NewMem()
	ctx := context.Background()

	m, err := st.CreateMerchant(ctx, "acme", domain.MerchantSettings{
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
	if _, err := st.SetMerchantWallet(ctx, m.MerchantID, MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	expired := time.Now().UTC().Add(-1 * time.Second)
	invPartial, _, err := st.CreateInvoice(ctx, InvoiceCreate{
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
	invOverpaid, _, err := st.CreateInvoice(ctx, InvoiceCreate{
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
	invLate, _, err := st.CreateInvoice(ctx, InvoiceCreate{
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

	// Partial payment creates partial_payment review case when policy is reject.
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
	if err := st.ApplyScanEvent(ctx, ScanEvent{
		WalletID:   "w1",
		Cursor:     1,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    bpp,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent partial: %v", err)
	}

	// Overpayment creates overpayment review case when policy is manual_review.
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
	if err := st.ApplyScanEvent(ctx, ScanEvent{
		WalletID:   "w1",
		Cursor:     2,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    bop,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent overpaid: %v", err)
	}

	// Late payment creates late_payment review case when policy is manual_review.
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
	if err := st.ApplyScanEvent(ctx, ScanEvent{
		WalletID:   "w1",
		Cursor:     3,
		Kind:       string(types.WalletEventKindDepositConfirmed),
		Payload:    blp,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("ApplyScanEvent late: %v", err)
	}

	cs, err := st.ListReviewCases(ctx, ReviewCaseFilter{MerchantID: m.MerchantID, Status: domain.ReviewOpen})
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
