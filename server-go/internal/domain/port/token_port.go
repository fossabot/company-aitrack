package port

import "github.com/aitrack/server/internal/domain/model"

// TokenPort is the persistence port for API tokens.
type TokenPort interface {
	// Save persists a newly issued token.
	Save(t *model.Token) error
	// FindActiveByHash resolves an active token by its SHA-256 hash.
	// Returns (nil, nil) when no active token matches.
	FindActiveByHash(hash string) (*model.Token, error)
}
