package mongo

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"fmt"
	"strings"
	"time"

	mongoDriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Option func(*Store) error

func WithCollectionPrefix(prefix string) Option {
	prefix = strings.TrimSpace(prefix)
	return func(s *Store) error {
		if s == nil {
			return errors.New("nil store")
		}
		if prefix == "" {
			s.prefix = ""
			return nil
		}
		if err := validateIdent(prefix); err != nil {
			return fmt.Errorf("collection prefix invalid: %w", err)
		}
		// Keep collection names short and predictable, especially when embedding into shared DBs.
		if len(prefix) > 32 {
			return fmt.Errorf("collection prefix too long (max 32)")
		}
		s.prefix = prefix
		return nil
	}
}

type OpenConfig struct {
	URI      string
	Database string
	TokenKey []byte
	Options  []Option
}

func Open(ctx context.Context, cfg OpenConfig) (*Store, error) {
	cfg.URI = strings.TrimSpace(cfg.URI)
	cfg.Database = strings.TrimSpace(cfg.Database)
	if cfg.URI == "" {
		return nil, errors.New("mongostore: uri is required")
	}
	if cfg.Database == "" {
		return nil, errors.New("mongostore: database is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	aead, err := newAEAD(cfg.TokenKey)
	if err != nil {
		return nil, err
	}

	client, err := mongoDriver.Connect(ctx, options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	st := &Store{
		client: client,
		db:     client.Database(cfg.Database),
		aead:   aead,
	}
	for _, opt := range cfg.Options {
		if opt == nil {
			continue
		}
		if err := opt(st); err != nil {
			_ = client.Disconnect(context.Background())
			return nil, err
		}
	}
	return st, nil
}

func newAEAD(tokenKey []byte) (cipher.AEAD, error) {
	if len(tokenKey) != 32 {
		return nil, errors.New("mongostore: token key must be 32 bytes")
	}
	block, err := aes.NewCipher(tokenKey)
	if err != nil {
		return nil, fmt.Errorf("mongostore: aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("mongostore: gcm: %w", err)
	}
	return aead, nil
}

func validateIdent(ident string) error {
	ident = strings.TrimSpace(ident)
	if ident == "" {
		return errors.New("empty")
	}
	if !isIdentStart(rune(ident[0])) {
		return errors.New("must start with letter or underscore")
	}
	for i := 1; i < len(ident); i++ {
		if !isIdentContinue(rune(ident[i])) {
			return errors.New("invalid character")
		}
	}
	return nil
}

func isIdentStart(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_'
}

func isIdentContinue(r rune) bool {
	return isIdentStart(r) || (r >= '0' && r <= '9')
}

