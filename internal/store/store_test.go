package store_test

import (
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestNew_CreatesSchema(t *testing.T) {
	s := openTestDB(t)
	assert.NotNil(t, s)
}

func TestSaveAndGetMessage(t *testing.T) {
	s := openTestDB(t)
	msg := store.Message{
		ChatID:    "123",
		FromUser:  "alice",
		Text:      "hello",
		MessageID: "m1",
		CreatedAt: time.Now().UTC(),
	}
	require.NoError(t, s.SaveMessage(msg))
	msgs, err := s.GetMessagesByChatID("123", 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "hello", msgs[0].Text)
}

func TestSaveAndGetTask(t *testing.T) {
	s := openTestDB(t)
	task := store.Task{
		ID:       "uuid-1",
		ChatID:   "123",
		Name:     "daily",
		Schedule: "0 9 * * *",
		Prompt:   "summarise",
		Paused:   false,
	}
	require.NoError(t, s.SaveTask(task))
	tasks, err := s.GetTasksByChatID("123")
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "daily", tasks[0].Name)
}

func TestPauseTask(t *testing.T) {
	s := openTestDB(t)
	task := store.Task{ID: "uuid-1", ChatID: "123", Name: "t", Schedule: "* * * * *", Prompt: "p"}
	require.NoError(t, s.SaveTask(task))
	require.NoError(t, s.PauseTask("uuid-1"))
	tasks, _ := s.GetAllActiveTasks()
	assert.Empty(t, tasks)
}

func TestRegisterAndGetGroup(t *testing.T) {
	s := openTestDB(t)
	require.NoError(t, s.RegisterGroup("123", "my-group", "test group"))
	g, err := s.GetGroup("my-group")
	require.NoError(t, err)
	assert.Equal(t, "123", g.ChatID)
}
