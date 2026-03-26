package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id    TEXT NOT NULL,
			from_user  TEXT NOT NULL,
			text       TEXT NOT NULL,
			message_id TEXT NOT NULL,
			created_at DATETIME NOT NULL
		);
		CREATE TABLE IF NOT EXISTS sessions (
			chat_id    TEXT PRIMARY KEY,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE TABLE IF NOT EXISTS tasks (
			id         TEXT PRIMARY KEY,
			chat_id    TEXT NOT NULL,
			name       TEXT NOT NULL,
			schedule   TEXT NOT NULL,
			prompt     TEXT NOT NULL,
			paused     INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS groups (
			chat_id     TEXT NOT NULL,
			name        TEXT PRIMARY KEY,
			description TEXT NOT NULL
		);
	`)
	return err
}
