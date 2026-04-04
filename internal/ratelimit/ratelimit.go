package ratelimit

import (
	"sync"
	"time"
)

// Limiter enforces a minimum interval between accepted messages per chat.
// A zero interval disables rate limiting (all messages are allowed).
type Limiter struct {
	interval time.Duration
	mu       sync.Mutex
	last     map[string]time.Time
}

// New returns a Limiter that accepts at most one message per interval per chat.
// Pass zero to disable rate limiting.
func New(interval time.Duration) *Limiter {
	return &Limiter{
		interval: interval,
		last:     make(map[string]time.Time),
	}
}

// Allow reports whether the message from chatID should be processed.
// It returns true for the first message from a chat, and for messages
// that arrive at least interval after the previous accepted message.
func (l *Limiter) Allow(chatID string) bool {
	if l.interval == 0 {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if t, ok := l.last[chatID]; ok && now.Sub(t) < l.interval {
		return false
	}
	l.last[chatID] = now
	return true
}
