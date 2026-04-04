package telegram_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/telegram"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendMessage(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/botTEST_TOKEN/sendMessage", r.URL.Path)
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()

	s := telegram.NewSender("TEST_TOKEN", srv.URL)
	err := s.SendMessage("123456", "Hello from Pitu")
	require.NoError(t, err)
	assert.Equal(t, "123456", gotBody["chat_id"])
	assert.Equal(t, "Hello from Pitu", gotBody["text"])
}

func TestSendMessage_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false,"description":"Bad Request"}`))
	}))
	defer srv.Close()

	s := telegram.NewSender("TOKEN", srv.URL)
	err := s.SendMessage("123", "text")
	assert.ErrorContains(t, err, "Bad Request")
}

func TestPoller_ReceivesUpdates(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Write([]byte(`{"ok":true,"result":[{"update_id":1,"message":{"message_id":10,"from":{"id":1,"first_name":"Alice"},"chat":{"id":999},"text":"hi","date":1000}}]}`))
		} else {
			// block — real poller would use long-poll timeout; for test, just return empty
			w.Write([]byte(`{"ok":true,"result":[]}`))
		}
	}))
	defer srv.Close()

	p := telegram.NewPoller("TOKEN", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	updates := make([]telegram.Update, 0)
	p.Poll(ctx, func(u telegram.Update) {
		updates = append(updates, u)
	})

	require.Len(t, updates, 1)
	assert.Equal(t, "hi", updates[0].Message.Text)
	assert.Equal(t, int64(999), updates[0].Message.Chat.ID)
}

func TestPoller_IgnoresUpdatesOn429(t *testing.T) {
	// Server always returns 429 with Retry-After: 1 — context expires before retry.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := telegram.NewPoller("TOKEN", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	var received []telegram.Update
	p.Poll(ctx, func(u telegram.Update) { received = append(received, u) })
	assert.Empty(t, received, "no updates should be delivered on 429 responses")
}

func TestPoller_RetriesAfter429(t *testing.T) {
	// First call returns 429 with Retry-After: 0 (immediate retry).
	// Second call returns a real update. Poll must deliver it.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":[{"update_id":42,"message":{"message_id":1,"from":{"id":1,"first_name":"Bob"},"chat":{"id":1},"text":"retry","date":1}}]}`))
	}))
	defer srv.Close()

	p := telegram.NewPoller("TOKEN", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var received []telegram.Update
	p.Poll(ctx, func(u telegram.Update) { received = append(received, u) })
	assert.GreaterOrEqual(t, callCount, 2, "Poll must retry after a 429")
}

func TestPoller_RetriesAfter5xx(t *testing.T) {
	// First call returns 500. Second call returns empty updates.
	// Poll must retry (callCount >= 2). Uses 3s timeout to cover the 2s backoff.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"result":[]}`))
	}))
	defer srv.Close()

	p := telegram.NewPoller("TOKEN", srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	p.Poll(ctx, func(telegram.Update) {})
	assert.GreaterOrEqual(t, callCount, 2, "Poll must retry after a 5xx")
}
