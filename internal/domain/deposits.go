package domain

import "time"

type DepositStatus string

const (
	DepositDetected    DepositStatus = "detected"
	DepositConfirmed   DepositStatus = "confirmed"
	DepositUnconfirmed DepositStatus = "unconfirmed"
	DepositOrphaned    DepositStatus = "orphaned"
)

type Deposit struct {
	WalletID         string
	TxID             string
	ActionIndex      int32
	RecipientAddress string
	AmountZat        int64
	Height           int64
	Status           DepositStatus
	ConfirmedHeight  *int64
	InvoiceID        *string
	DetectedAt       time.Time
	UpdatedAt        time.Time
}
