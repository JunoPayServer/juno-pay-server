package store

import (
	"context"
	"testing"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
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
		MerchantID:       m.MerchantID,
		ExternalOrderID:  "order-1",
		WalletID:         "w1",
		AddressIndex:     0,
		Address:          "j1abc",
		CreatedAfterHeight: 100,
		CreatedAfterHash:   "hash100",
		AmountZat:          10,
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

