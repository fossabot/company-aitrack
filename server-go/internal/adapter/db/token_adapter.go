// Package dbadapter contains the driven-side persistence adapters. Each type
// implements a domain port (TokenPort, EditRecordPort, DevicePort) on top of a
// *sql.DB, isolating SQL from the domain and application layers.
package dbadapter

import (
	"database/sql"
	"time"

	"github.com/aitrack/server/internal/domain/model"
	"github.com/aitrack/server/internal/domain/port"
)

// TokenAdapter persists tokens. It implements port.TokenPort.
type TokenAdapter struct {
	db *sql.DB
}

// NewTokenAdapter constructs a TokenAdapter over the given database.
func NewTokenAdapter(db *sql.DB) *TokenAdapter {
	return &TokenAdapter{db: db}
}

var _ port.TokenPort = (*TokenAdapter)(nil)

// Save persists a newly issued token.
func (r *TokenAdapter) Save(t *model.Token) error {
	_, err := r.db.Exec(
		`INSERT INTO tokens (token_hash, token_key, hmac_secret, owner, note, active, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		t.TokenHash, t.TokenKey, t.HmacSecret, t.Owner, t.Note,
		boolToInt(t.Active), t.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// FindActiveByHash resolves an active token by its SHA-256 hash.
func (r *TokenAdapter) FindActiveByHash(hash string) (*model.Token, error) {
	row := r.db.QueryRow(
		`SELECT id, token_hash, token_key, hmac_secret, owner, note, active, created_at
		 FROM tokens WHERE token_hash = $1 AND active = 1`, hash)
	return scanToken(row)
}

func scanToken(row *sql.Row) (*model.Token, error) {
	var t model.Token
	var createdAt string
	var active int
	err := row.Scan(&t.ID, &t.TokenHash, &t.TokenKey, &t.HmacSecret,
		&t.Owner, &t.Note, &active, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.Active = active == 1
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
