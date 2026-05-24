package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newCapTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCapabilities_EnableGetDisable(t *testing.T) {
	s := newCapTestStore(t)

	require.NoError(t, s.EnableCapability("chat-1", "gmail"))
	require.NoError(t, s.EnableCapability("chat-1", "gcalendar"))
	require.NoError(t, s.EnableCapability("chat-1", "gmail")) // idempotent

	caps, err := s.GetCapabilities("chat-1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"gmail", "gcalendar"}, caps)

	require.NoError(t, s.DisableCapability("chat-1", "gmail"))
	caps, err = s.GetCapabilities("chat-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"gcalendar"}, caps)
}

func TestCapabilities_GetEmpty(t *testing.T) {
	s := newCapTestStore(t)
	caps, err := s.GetCapabilities("unknown")
	require.NoError(t, err)
	assert.Empty(t, caps)
}

func TestCapabilities_ListByChat(t *testing.T) {
	s := newCapTestStore(t)
	require.NoError(t, s.EnableCapability("chat-1", "gmail"))
	require.NoError(t, s.EnableCapability("chat-2", "gcalendar"))

	all, err := s.ListCapabilitiesByChat()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"gmail"}, all["chat-1"])
	assert.ElementsMatch(t, []string{"gcalendar"}, all["chat-2"])
}
