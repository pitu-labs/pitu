package ratelimit_test

import (
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/ratelimit"
	"github.com/stretchr/testify/assert"
)

func TestLimiter_AllowsFirstMessage(t *testing.T) {
	l := ratelimit.New(5 * time.Second)
	assert.True(t, l.Allow("chat1"), "first message must always be allowed")
}

func TestLimiter_DropsMessageWithinInterval(t *testing.T) {
	l := ratelimit.New(5 * time.Second)
	assert.True(t, l.Allow("chat1"))
	assert.False(t, l.Allow("chat1"), "second message within interval must be denied")
}

func TestLimiter_AllowsMessageAfterInterval(t *testing.T) {
	l := ratelimit.New(20 * time.Millisecond)
	assert.True(t, l.Allow("chat1"))
	time.Sleep(25 * time.Millisecond)
	assert.True(t, l.Allow("chat1"), "message after interval must be allowed")
}

func TestLimiter_IndependentPerChat(t *testing.T) {
	l := ratelimit.New(5 * time.Second)
	assert.True(t, l.Allow("chat1"))
	assert.True(t, l.Allow("chat2"), "different chat must have its own bucket")
}

func TestLimiter_ZeroIntervalAlwaysAllows(t *testing.T) {
	l := ratelimit.New(0)
	for i := 0; i < 10; i++ {
		assert.True(t, l.Allow("chat1"), "zero interval must always allow")
	}
}
