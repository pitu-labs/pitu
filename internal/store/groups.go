package store

type Group struct {
	ChatID      string
	Name        string
	Description string
}

func (s *Store) RegisterGroup(chatID, name, description string) error {
	_, err := s.db.Exec(
		`INSERT INTO groups (chat_id, name, description) VALUES (?,?,?)
		 ON CONFLICT(name) DO UPDATE SET chat_id=excluded.chat_id, description=excluded.description`,
		chatID, name, description,
	)
	return err
}

func (s *Store) GetGroup(name string) (*Group, error) {
	row := s.db.QueryRow(`SELECT chat_id, name, description FROM groups WHERE name=?`, name)
	var g Group
	if err := row.Scan(&g.ChatID, &g.Name, &g.Description); err != nil {
		return nil, err
	}
	return &g, nil
}
