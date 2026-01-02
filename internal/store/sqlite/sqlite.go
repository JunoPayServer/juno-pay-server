package sqlite

import (
	_ "modernc.org/sqlite"

	"github.com/Abdullah1738/juno-pay-server/internal/store/sqlstore"
)

type Store = sqlstore.Store

func Open(dataDir string, tokenKey []byte) (*Store, error) {
	return sqlstore.Open(dataDir, tokenKey)
}

