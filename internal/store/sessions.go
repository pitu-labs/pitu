package store

import "time"

type Session struct {
	ChatID    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) SaveSession(chatID string) error {
	now := time.Now().UTC()
	_, err := s.db.Exec(`
		INSERT INTO sessions (chat_id, created_at, updated_at) VALUES (?,?,?)
		ON CONFLICT(chat_id) DO UPDATE SET updated_at=excluded.updated_at`,
		chatID, now, now,
	)
	return err
}

func (s *Store) GetSession(chatID string) (*Session, error) {
	row := s.db.QueryRow(`SELECT chat_id, created_at, updated_at FROM sessions WHERE chat_id=?`, chatID)
	var sess Session
	if err := row.Scan(&sess.ChatID, &sess.CreatedAt, &sess.UpdatedAt); err != nil {
		return nil, err
	}
	return &sess, nil
}
