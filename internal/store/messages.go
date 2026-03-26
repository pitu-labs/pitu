package store

import "time"

type Message struct {
	ChatID    string
	FromUser  string
	Text      string
	MessageID string
	CreatedAt time.Time
}

func (s *Store) SaveMessage(m Message) error {
	_, err := s.db.Exec(
		`INSERT INTO messages (chat_id, from_user, text, message_id, created_at) VALUES (?,?,?,?,?)`,
		m.ChatID, m.FromUser, m.Text, m.MessageID, m.CreatedAt,
	)
	return err
}

func (s *Store) GetMessagesByChatID(chatID string, limit int) ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT chat_id, from_user, text, message_id, created_at FROM messages WHERE chat_id=? ORDER BY id DESC LIMIT ?`,
		chatID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ChatID, &m.FromUser, &m.Text, &m.MessageID, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
