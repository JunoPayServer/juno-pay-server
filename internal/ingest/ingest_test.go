package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/domain"
	"github.com/Abdullah1738/juno-pay-server/internal/scanclient"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
	"github.com/Abdullah1738/juno-sdk-go/types"
)

func TestIngestor_Sync_AppliesDepositEvents(t *testing.T) {
	st := store.NewMem()
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
	if _, err := st.SetMerchantWallet(ctx, m.MerchantID, store.MerchantWallet{
		WalletID: "w1",
		UFVK:     "jview1test",
		Chain:    "mainnet",
		UAHRP:    "j",
		CoinType: 8133,
	}); err != nil {
		t.Fatalf("SetMerchantWallet: %v", err)
	}

	inv, _, err := st.CreateInvoice(ctx, store.InvoiceCreate{
		MerchantID:            m.MerchantID,
		ExternalOrderID:       "order-1",
		WalletID:              "w1",
		AddressIndex:          0,
		Address:               "j1addr0",
		CreatedAfterHeight:    100,
		CreatedAfterHash:      "h100",
		AmountZat:             100,
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

	now := time.Date(2025, 12, 30, 0, 0, 0, 0, time.UTC)

	var upsertCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/wallets":
			upsertCount++
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/wallets/w1/events":
			cursor := r.URL.Query().Get("cursor")
			if cursor == "0" {
				payload := types.DepositEventPayload{
					DepositEvent: types.DepositEvent{
						WalletID:       "w1",
						TxID:           "tx1",
						Height:         101,
						ActionIndex:    0,
						AmountZatoshis: 100,
					},
					RecipientAddress: "j1addr0",
				}
				pb, _ := json.Marshal(payload)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"events": []any{
						map[string]any{
							"id":         1,
							"kind":       "DepositEvent",
							"wallet_id":  "w1",
							"height":     101,
							"payload":    json.RawMessage(pb),
							"created_at": now.Format(time.RFC3339Nano),
						},
					},
					"next_cursor": 1,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"events": []any{}, "next_cursor": 1})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	sc, err := scanclient.New(srv.URL)
	if err != nil {
		t.Fatalf("scanclient.New: %v", err)
	}
	ing, err := New(st, sc, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := ing.Sync(ctx); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if upsertCount == 0 {
		t.Fatalf("expected UpsertWallet to be called")
	}

	got, ok, err := st.GetInvoice(ctx, inv.InvoiceID)
	if err != nil || !ok {
		t.Fatalf("GetInvoice: ok=%v err=%v", ok, err)
	}
	if got.ReceivedPendingZat != 100 {
		t.Fatalf("expected pending 100, got %d", got.ReceivedPendingZat)
	}
}
