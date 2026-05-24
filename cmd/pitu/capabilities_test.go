package main

import (
	"path/filepath"
	"testing"

	"github.com/pitu-dev/pitu/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCapabilities_EnableDisable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.New(dbPath)
	require.NoError(t, err)
	defer st.Close()

	require.NoError(t, runCapabilitiesWithStore(st, []string{"enable", "--chat", "c1", "--capability", "gmail"}))
	caps, _ := st.GetCapabilities("c1")
	assert.Equal(t, []string{"gmail"}, caps)

	require.NoError(t, runCapabilitiesWithStore(st, []string{"disable", "--chat", "c1", "--capability", "gmail"}))
	caps, _ = st.GetCapabilities("c1")
	assert.Empty(t, caps)
}

func TestRunCapabilities_BadArgs(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.New(dbPath)
	require.NoError(t, err)
	defer st.Close()

	assert.Error(t, runCapabilitiesWithStore(st, []string{"enable", "--chat", "c1"})) // missing --capability
	assert.Error(t, runCapabilitiesWithStore(st, []string{"bogus"}))                  // unknown action
}
