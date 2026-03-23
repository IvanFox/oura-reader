package user

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
)

const apiKeyPrefix = "oura_ak_"

type Manager struct {
	db *sql.DB
}

type User struct {
	ID           int64
	Name         string
	APIKeyPrefix string
	CreatedAt    string
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// Add creates a new user and returns the raw API key (shown once).
func (m *Manager) Add(ctx context.Context, name string) (string, error) {
	rawKey, hash, prefix, err := generateAPIKey()
	if err != nil {
		return "", err
	}

	_, err = m.db.ExecContext(ctx, `
		INSERT INTO users (name, api_key_hash, api_key_prefix) VALUES (?, ?, ?)
	`, name, hash, prefix)
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}

	return rawKey, nil
}

// List returns all users.
func (m *Manager) List(ctx context.Context) ([]User, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT id, name, api_key_prefix, created_at FROM users ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.APIKeyPrefix, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// Rotate invalidates the old API key and returns a new one.
func (m *Manager) Rotate(ctx context.Context, name string) (string, error) {
	rawKey, hash, prefix, err := generateAPIKey()
	if err != nil {
		return "", err
	}

	res, err := m.db.ExecContext(ctx, `
		UPDATE users SET api_key_hash = ?, api_key_prefix = ? WHERE name = ?
	`, hash, prefix, name)
	if err != nil {
		return "", fmt.Errorf("rotate key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return "", fmt.Errorf("user %q not found", name)
	}

	return rawKey, nil
}

// Remove deletes a user and all their data (cascading).
func (m *Manager) Remove(ctx context.Context, name string) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM users WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("remove user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %q not found", name)
	}
	return nil
}

// LookupByAPIKey finds a user by their raw API key.
func (m *Manager) LookupByAPIKey(ctx context.Context, rawKey string) (*User, error) {
	hash := hashKey(rawKey)
	var u User
	err := m.db.QueryRowContext(ctx, `
		SELECT id, name, api_key_prefix, created_at FROM users WHERE api_key_hash = ?
	`, hash).Scan(&u.ID, &u.Name, &u.APIKeyPrefix, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user: %w", err)
	}
	return &u, nil
}

// GetAllWithTokens returns user IDs for all users that have OAuth tokens stored.
func (m *Manager) GetAllWithTokens(ctx context.Context) ([]int64, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT u.id FROM users u INNER JOIN oauth_tokens ot ON u.id = ot.user_id
	`)
	if err != nil {
		return nil, fmt.Errorf("get users with tokens: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func generateAPIKey() (rawKey, hash, prefix string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("generate random bytes: %w", err)
	}
	rawKey = apiKeyPrefix + base64.RawURLEncoding.EncodeToString(b)
	hash = hashKey(rawKey)
	prefix = rawKey[:len(apiKeyPrefix)+8]
	return rawKey, hash, prefix, nil
}

func hashKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return fmt.Sprintf("%x", h)
}
