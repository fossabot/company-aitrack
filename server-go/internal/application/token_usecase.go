// Package application holds the use cases that orchestrate domain services and
// driven ports. Use cases depend only on domain/model, domain/service and
// domain/port — never on HTTP or SQL directly.
package application

import (
	"fmt"
	"time"

	"github.com/aitrack/server/internal/domain/model"
	"github.com/aitrack/server/internal/domain/port"
	"github.com/aitrack/server/internal/domain/service"
)

// TokenService issues and resolves API tokens.
type TokenService struct {
	repo      port.TokenPort
	sig       *service.SignatureService
	encryptor *service.HmacSecretEncryptor
}

// NewTokenService constructs the token use case.
func NewTokenService(repo port.TokenPort, sig *service.SignatureService, enc *service.HmacSecretEncryptor) *TokenService {
	return &TokenService{repo: repo, sig: sig, encryptor: enc}
}

// CreateToken issues a new credential, persisting an encrypted hmac_secret.
func (s *TokenService) CreateToken(req *model.CreateTokenRequest) (*model.CreateTokenResponse, error) {
	rawToken := service.NewRawToken()
	hmacSecret := service.RandomHex(32)
	tokenHash := s.sig.SHA256HexStr(rawToken)
	tokenKey := service.ComputeTokenKey(rawToken)

	encrypted, err := s.encryptor.Encrypt(hmacSecret)
	if err != nil {
		return nil, fmt.Errorf("encrypt hmac_secret: %w", err)
	}

	t := &model.Token{
		TokenHash:  tokenHash,
		TokenKey:   tokenKey,
		HmacSecret: encrypted,
		Owner:      req.Owner,
		Note:       req.Note,
		Active:     true,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.repo.Save(t); err != nil {
		return nil, fmt.Errorf("save token: %w", err)
	}
	credential := rawToken + "-" + hmacSecret
	return &model.CreateTokenResponse{
		Credential: credential,
		TokenKey:   tokenKey,
	}, nil
}

// FindActiveToken resolves a raw Bearer token to a Token with decrypted hmac_secret.
func (s *TokenService) FindActiveToken(rawToken string) (*model.Token, error) {
	hash := s.sig.SHA256HexStr(rawToken)
	t, err := s.repo.FindActiveByHash(hash)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	plain, err := s.encryptor.Decrypt(t.HmacSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt hmac_secret: %w", err)
	}
	t.HmacSecret = plain
	return t, nil
}
