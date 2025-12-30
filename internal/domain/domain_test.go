package domain

import (
	"testing"
)

func TestMerchantSettings_Validate_Success(t *testing.T) {
	s := MerchantSettings{
		InvoiceTTLSeconds:     1800,
		RequiredConfirmations: 100,
		Policies: InvoicePolicies{
			LatePayment:    LatePaymentMarkPaidLate,
			PartialPayment: PartialPaymentAccept,
			Overpayment:    OverpaymentMarkOverpaid,
		},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestMerchantSettings_Validate_InvalidTTL(t *testing.T) {
	s := MerchantSettings{
		InvoiceTTLSeconds:     -1,
		RequiredConfirmations: 0,
		Policies: InvoicePolicies{
			LatePayment:    LatePaymentMarkPaidLate,
			PartialPayment: PartialPaymentAccept,
			Overpayment:    OverpaymentMarkOverpaid,
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestMerchantSettings_Validate_InvalidConfirmations(t *testing.T) {
	s := MerchantSettings{
		InvoiceTTLSeconds:     0,
		RequiredConfirmations: -1,
		Policies: InvoicePolicies{
			LatePayment:    LatePaymentMarkPaidLate,
			PartialPayment: PartialPaymentAccept,
			Overpayment:    OverpaymentMarkOverpaid,
		},
	}
	if err := s.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestMerchantSettings_Validate_InvalidPolicies(t *testing.T) {
	t.Run("late", func(t *testing.T) {
		s := MerchantSettings{
			InvoiceTTLSeconds:     0,
			RequiredConfirmations: 0,
			Policies: InvoicePolicies{
				LatePayment:    "nope",
				PartialPayment: PartialPaymentAccept,
				Overpayment:    OverpaymentMarkOverpaid,
			},
		}
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("partial", func(t *testing.T) {
		s := MerchantSettings{
			InvoiceTTLSeconds:     0,
			RequiredConfirmations: 0,
			Policies: InvoicePolicies{
				LatePayment:    LatePaymentMarkPaidLate,
				PartialPayment: "nope",
				Overpayment:    OverpaymentMarkOverpaid,
			},
		}
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("overpay", func(t *testing.T) {
		s := MerchantSettings{
			InvoiceTTLSeconds:     0,
			RequiredConfirmations: 0,
			Policies: InvoicePolicies{
				LatePayment:    LatePaymentMarkPaidLate,
				PartialPayment: PartialPaymentAccept,
				Overpayment:    "nope",
			},
		}
		if err := s.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestInvoice_CanApplyDeposit(t *testing.T) {
	inv := Invoice{CreatedAfterHeight: 100}
	if inv.CanApplyDeposit(100) {
		t.Fatalf("expected false when deposit height == created_after_height")
	}
	if !inv.CanApplyDeposit(101) {
		t.Fatalf("expected true when deposit height > created_after_height")
	}
	if inv.CanApplyDeposit(99) {
		t.Fatalf("expected false when deposit height < created_after_height")
	}
}

