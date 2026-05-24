package store

import "time"

// EnableCapability marks a capability enabled for a chat. Idempotent.
func (s *Store) EnableCapability(chatID, capability string) error {
	_, err := s.db.Exec(
		`INSERT INTO chat_capabilities (chat_id, capability, enabled_at) VALUES (?,?,?)
		 ON CONFLICT(chat_id, capability) DO NOTHING`,
		chatID, capability, time.Now().UTC(),
	)
	return err
}

// DisableCapability removes a capability from a chat. No error if absent.
func (s *Store) DisableCapability(chatID, capability string) error {
	_, err := s.db.Exec(
		`DELETE FROM chat_capabilities WHERE chat_id=? AND capability=?`,
		chatID, capability,
	)
	return err
}

// GetCapabilities returns the enabled capability names for a chat (may be empty).
func (s *Store) GetCapabilities(chatID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT capability FROM chat_capabilities WHERE chat_id=? ORDER BY capability`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var caps []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		caps = append(caps, c)
	}
	return caps, rows.Err()
}

// ListCapabilitiesByChat returns a map of chat_id → enabled capability names.
func (s *Store) ListCapabilitiesByChat() (map[string][]string, error) {
	rows, err := s.db.Query(`SELECT chat_id, capability FROM chat_capabilities ORDER BY chat_id, capability`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var chatID, capability string
		if err := rows.Scan(&chatID, &capability); err != nil {
			return nil, err
		}
		out[chatID] = append(out[chatID], capability)
	}
	return out, rows.Err()
}
