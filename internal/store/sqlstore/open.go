package sqlstore

import (
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Option func(*Store) error

func WithTablePrefix(prefix string) Option {
	prefix = strings.TrimSpace(prefix)
	return func(s *Store) error {
		if s == nil {
			return errors.New("nil store")
		}
		if prefix == "" {
			s.tablePrefix = ""
			s.rewriteMap = nil
			return nil
		}
		if err := validateIdent(prefix); err != nil {
			return fmt.Errorf("table prefix invalid: %w", err)
		}
		// Postgres identifier limit is 63 bytes; keep space for suffixes.
		if len(prefix) > 20 {
			return fmt.Errorf("table prefix too long (max 20)")
		}

		s.tablePrefix = prefix

		m := make(map[string]string, len(baseIdents))
		for _, ident := range baseIdents {
			m[ident] = prefix + "_" + ident
		}
		s.rewriteMap = m
		return nil
	}
}

func Open(dataDir string, tokenKey []byte) (*Store, error) {
	return OpenSQLite(dataDir, tokenKey)
}

func OpenSQLite(dataDir string, tokenKey []byte, opts ...Option) (*Store, error) {
	dataDir = filepath.Clean(strings.TrimSpace(dataDir))
	if dataDir == "" || dataDir == "." || dataDir == string(os.PathSeparator) {
		return nil, errors.New("sqlstore: sqlite: invalid data dir")
	}
	aead, err := newAEAD(tokenKey)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("sqlstore: sqlite: mkdir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "state.sqlite")
	dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: sqlite: open: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	st := &Store{db: db, aead: aead, dialect: sqliteDialect{}}
	if err := applyOptions(st, opts); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

func OpenPostgres(dsn string, tokenKey []byte, opts ...Option) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("sqlstore: postgres: dsn is required")
	}
	aead, err := newAEAD(tokenKey)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: postgres: open: %w", err)
	}

	st := &Store{db: db, aead: aead, dialect: postgresDialect{}}
	if err := applyOptions(st, opts); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

func OpenMySQL(dsn string, tokenKey []byte, opts ...Option) (*Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("sqlstore: mysql: dsn is required")
	}
	aead, err := newAEAD(tokenKey)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: mysql: open: %w", err)
	}

	st := &Store{db: db, aead: aead, dialect: mysqlDialect{}}
	if err := applyOptions(st, opts); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

func applyOptions(st *Store, opts []Option) error {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(st); err != nil {
			return err
		}
	}
	return nil
}

func newAEAD(tokenKey []byte) (cipher.AEAD, error) {
	if len(tokenKey) != 32 {
		return nil, errors.New("sqlstore: token key must be 32 bytes")
	}
	block, err := aes.NewCipher(tokenKey)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: gcm: %w", err)
	}
	return aead, nil
}

func decodeHex32(s string) ([]byte, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, err
	}
	if len(raw) != 32 {
		return nil, errors.New("expected 32 bytes")
	}
	return raw, nil
}

