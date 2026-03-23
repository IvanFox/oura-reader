package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

// UpsertOuraData inserts or updates a single Oura data record.
func (s *Store) UpsertOuraData(ctx context.Context, userID int64, endpoint, day, ouraID string, data json.RawMessage) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO oura_data (user_id, endpoint, day, oura_id, data, fetched_at)
		VALUES (?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(user_id, endpoint, day, oura_id) DO UPDATE SET
			data = excluded.data,
			fetched_at = excluded.fetched_at
	`, userID, endpoint, nullIfEmpty(day), nullIfEmpty(ouraID), string(data))
	if err != nil {
		return fmt.Errorf("upsert oura_data: %w", err)
	}
	return nil
}

// QueryOuraData retrieves stored data for an endpoint within a date range.
func (s *Store) QueryOuraData(ctx context.Context, userID int64, endpoint, startDate, endDate string, limit, offset int) ([]json.RawMessage, int, error) {
	var countRow *sql.Row
	var rows *sql.Rows
	var err error

	if startDate != "" && endDate != "" {
		countRow = s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM oura_data WHERE user_id = ? AND endpoint = ? AND day >= ? AND day <= ?
		`, userID, endpoint, startDate, endDate)
		rows, err = s.db.QueryContext(ctx, `
			SELECT data FROM oura_data WHERE user_id = ? AND endpoint = ? AND day >= ? AND day <= ?
			ORDER BY day ASC LIMIT ? OFFSET ?
		`, userID, endpoint, startDate, endDate, limit, offset)
	} else if startDate != "" {
		countRow = s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM oura_data WHERE user_id = ? AND endpoint = ? AND day >= ?
		`, userID, endpoint, startDate)
		rows, err = s.db.QueryContext(ctx, `
			SELECT data FROM oura_data WHERE user_id = ? AND endpoint = ? AND day >= ?
			ORDER BY day ASC LIMIT ? OFFSET ?
		`, userID, endpoint, startDate, limit, offset)
	} else {
		countRow = s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM oura_data WHERE user_id = ? AND endpoint = ?
		`, userID, endpoint)
		rows, err = s.db.QueryContext(ctx, `
			SELECT data FROM oura_data WHERE user_id = ? AND endpoint = ?
			ORDER BY day ASC LIMIT ? OFFSET ?
		`, userID, endpoint, limit, offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("query oura_data: %w", err)
	}
	defer rows.Close()

	var total int
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count oura_data: %w", err)
	}

	var results []json.RawMessage
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, fmt.Errorf("scan oura_data: %w", err)
		}
		results = append(results, json.RawMessage(raw))
	}
	return results, total, rows.Err()
}

// GetSyncState returns the last sync date for a user/endpoint.
func (s *Store) GetSyncState(ctx context.Context, userID int64, endpoint string) (lastSyncDate string, lastSyncAt time.Time, err error) {
	var syncAt string
	err = s.db.QueryRowContext(ctx, `
		SELECT last_sync_date, last_sync_at FROM sync_state WHERE user_id = ? AND endpoint = ?
	`, userID, endpoint).Scan(&lastSyncDate, &syncAt)
	if err == sql.ErrNoRows {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("get sync state: %w", err)
	}
	lastSyncAt, _ = time.Parse("2006-01-02 15:04:05", syncAt)
	return lastSyncDate, lastSyncAt, nil
}

// SetSyncState updates the last sync date for a user/endpoint.
func (s *Store) SetSyncState(ctx context.Context, userID int64, endpoint, lastSyncDate string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sync_state (user_id, endpoint, last_sync_date, last_sync_at)
		VALUES (?, ?, ?, datetime('now'))
		ON CONFLICT(user_id, endpoint) DO UPDATE SET
			last_sync_date = excluded.last_sync_date,
			last_sync_at = excluded.last_sync_at
	`, userID, endpoint, lastSyncDate)
	if err != nil {
		return fmt.Errorf("set sync state: %w", err)
	}
	return nil
}

// GetAllSyncStates returns all sync states for a user.
func (s *Store) GetAllSyncStates(ctx context.Context, userID int64) (map[string]SyncInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT endpoint, last_sync_date, last_sync_at FROM sync_state WHERE user_id = ?
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("get all sync states: %w", err)
	}
	defer rows.Close()

	result := make(map[string]SyncInfo)
	for rows.Next() {
		var endpoint, date, at string
		if err := rows.Scan(&endpoint, &date, &at); err != nil {
			return nil, fmt.Errorf("scan sync state: %w", err)
		}
		syncAt, _ := time.Parse("2006-01-02 15:04:05", at)
		result[endpoint] = SyncInfo{LastSyncDate: date, LastSyncAt: syncAt}
	}
	return result, rows.Err()
}

type SyncInfo struct {
	LastSyncDate string    `json:"last_sync_date"`
	LastSyncAt   time.Time `json:"last_sync_at"`
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
