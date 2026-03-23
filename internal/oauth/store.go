package oauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ivan-lissitsnoi/oura-reader/internal/crypto"
	"golang.org/x/oauth2"
)

type Store struct {
	db     *sql.DB
	cipher *crypto.Cipher
}

func NewStore(db *sql.DB, cipher *crypto.Cipher) *Store {
	return &Store{db: db, cipher: cipher}
}

type storedToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	Expiry       time.Time `json:"expiry"`
}

func (s *Store) SaveToken(ctx context.Context, userID int64, token *oauth2.Token) error {
	st := storedToken{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
	}
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	encrypted, err := s.cipher.Encrypt(data)
	if err != nil {
		return fmt.Errorf("encrypt token: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO oauth_tokens (user_id, encrypted, updated_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(user_id) DO UPDATE SET
			encrypted = excluded.encrypted,
			updated_at = excluded.updated_at
	`, userID, encrypted)
	if err != nil {
		return fmt.Errorf("save token: %w", err)
	}
	return nil
}

func (s *Store) LoadToken(ctx context.Context, userID int64) (*oauth2.Token, error) {
	var encrypted []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT encrypted FROM oauth_tokens WHERE user_id = ?
	`, userID).Scan(&encrypted)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load token: %w", err)
	}

	data, err := s.cipher.Decrypt(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}

	var st storedToken
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("unmarshal token: %w", err)
	}

	return &oauth2.Token{
		AccessToken:  st.AccessToken,
		RefreshToken: st.RefreshToken,
		TokenType:    st.TokenType,
		Expiry:       st.Expiry,
	}, nil
}

func (s *Store) HasToken(ctx context.Context, userID int64) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM oauth_tokens WHERE user_id = ?
	`, userID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check token: %w", err)
	}
	return count > 0, nil
}
