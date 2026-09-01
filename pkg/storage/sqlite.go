package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// CachedPayload represents a queued discovery JSON payload stored in SQLite.
type CachedPayload struct {
	ID        int64     `json:"id"`
	Payload   string    `json:"payload"`
	CreatedAt time.Time `json:"created_at"`
}

// SQLiteQueue provides offline persistence for discovery payloads.
type SQLiteQueue struct {
	db *sql.DB
	mu sync.Mutex
}

// NewSQLiteQueue opens or creates a local SQLite database for offline queuing.
func NewSQLiteQueue(dbPath string) (*SQLiteQueue, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db at %s: %w", dbPath, err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS offline_payloads (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		payload TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize sqlite offline_payloads schema: %w", err)
	}

	return &SQLiteQueue{db: db}, nil
}

// Enqueue serializes and persists a telemetry JSON string to the offline database.
func (q *SQLiteQueue) Enqueue(payloadJSON string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	query := `INSERT INTO offline_payloads (payload, created_at) VALUES (?, ?)`
	_, err := q.db.Exec(query, payloadJSON, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to enqueue payload to sqlite: %w", err)
	}

	return nil
}

// DequeueAll retrieves all pending cached payloads from SQLite.
func (q *SQLiteQueue) DequeueAll() ([]CachedPayload, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	query := `SELECT id, payload, created_at FROM offline_payloads ORDER BY id ASC`
	rows, err := q.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query offline_payloads: %w", err)
	}
	defer rows.Close()

	var list []CachedPayload
	for rows.Next() {
		var item CachedPayload
		if err := rows.Scan(&item.ID, &item.Payload, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan offline_payload row: %w", err)
		}
		list = append(list, item)
	}

	return list, nil
}

// Delete removes processed payload records from SQLite by ID.
func (q *SQLiteQueue) Delete(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	tx, err := q.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin sqlite delete transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("DELETE FROM offline_payloads WHERE id = ?")
	if err != nil {
		return fmt.Errorf("failed to prepare delete statement: %w", err)
	}
	defer stmt.Close()

	for _, id := range ids {
		if _, err := stmt.Exec(id); err != nil {
			return fmt.Errorf("failed to delete offline payload id %d: %w", id, err)
		}
	}

	return tx.Commit()
}

// Count returns the current number of cached items in SQLite.
func (q *SQLiteQueue) Count() (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	var count int
	err := q.db.QueryRow("SELECT COUNT(*) FROM offline_payloads").Scan(&count)
	return count, err
}

// Close closes the underlying SQLite database connection.
func (q *SQLiteQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.db.Close()
}
