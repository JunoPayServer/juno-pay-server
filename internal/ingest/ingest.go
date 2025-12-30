package ingest

import (
	"context"
	"errors"
	"time"

	"github.com/Abdullah1738/juno-pay-server/internal/scanclient"
	"github.com/Abdullah1738/juno-pay-server/internal/store"
)

type Ingestor struct {
	st   store.Store
	scan *scanclient.Client

	pollInterval time.Duration
}

func New(st store.Store, scan *scanclient.Client, pollInterval time.Duration) (*Ingestor, error) {
	if st == nil {
		return nil, errors.New("ingest: store is nil")
	}
	if scan == nil {
		return nil, errors.New("ingest: scan client is nil")
	}
	if pollInterval <= 0 {
		pollInterval = 1 * time.Second
	}
	return &Ingestor{
		st:           st,
		scan:         scan,
		pollInterval: pollInterval,
	}, nil
}

// Sync performs one full scan sync pass for all configured merchant wallets.
func (i *Ingestor) Sync(ctx context.Context) error {
	wallets, err := i.st.ListMerchantWallets(ctx)
	if err != nil {
		return err
	}

	for _, w := range wallets {
		if err := i.scan.UpsertWallet(ctx, w.WalletID, w.UFVK); err != nil {
			return err
		}

		cursor, err := i.st.ScanCursor(ctx, w.WalletID)
		if err != nil {
			return err
		}

		for {
			evs, nextCursor, err := i.scan.ListWalletEvents(ctx, w.WalletID, cursor, 200)
			if err != nil {
				return err
			}
			if len(evs) == 0 {
				break
			}

			for _, e := range evs {
				if err := i.st.ApplyScanEvent(ctx, store.ScanEvent{
					WalletID:   w.WalletID,
					Cursor:     e.ID,
					Kind:       e.Kind,
					Height:     e.Height,
					Payload:    e.Payload,
					OccurredAt: e.CreatedAt,
				}); err != nil {
					return err
				}
				cursor = e.ID
			}
			cursor = nextCursor
		}
	}

	return nil
}

func (i *Ingestor) Run(ctx context.Context) error {
	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()

	for {
		if err := i.Sync(ctx); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
