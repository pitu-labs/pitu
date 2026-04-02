package store

type Task struct {
	ID       string
	ChatID   string
	Name     string
	Schedule string
	Prompt   string
	Paused   bool
}

func (s *Store) SaveTask(t Task) error {
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, chat_id, name, schedule, prompt, paused) VALUES (?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, schedule=excluded.schedule, prompt=excluded.prompt`,
		t.ID, t.ChatID, t.Name, t.Schedule, t.Prompt, boolToInt(t.Paused),
	)
	return err
}

func (s *Store) GetTasksByChatID(chatID string) ([]Task, error) {
	return s.queryTasks(`SELECT id, chat_id, name, schedule, prompt, paused FROM tasks WHERE chat_id=?`, chatID)
}

func (s *Store) GetAllActiveTasks() ([]Task, error) {
	return s.queryTasks(`SELECT id, chat_id, name, schedule, prompt, paused FROM tasks WHERE paused=0`)
}

func (s *Store) PauseTask(id string) error {
	_, err := s.db.Exec(`UPDATE tasks SET paused=1 WHERE id=?`, id)
	return err
}

func (s *Store) DeleteTask(id string) error {
	_, err := s.db.Exec(`DELETE FROM tasks WHERE id=?`, id)
	return err
}

func (s *Store) queryTasks(query string, args ...any) ([]Task, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		var t Task
		var paused int
		if err := rows.Scan(&t.ID, &t.ChatID, &t.Name, &t.Schedule, &t.Prompt, &paused); err != nil {
			return nil, err
		}
		t.Paused = paused == 1
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
