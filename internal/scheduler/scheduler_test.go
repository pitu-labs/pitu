package scheduler_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/scheduler"
	"github.com/pitu-dev/pitu/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_FiresTask(t *testing.T) {
	var fired atomic.Int32
	dispatch := func(chatID, prompt string) {
		fired.Add(1)
	}

	s := scheduler.New(dispatch)
	// "@every 2s" fires reliably within robfig/cron's second-resolution scheduler
	require.NoError(t, s.Add(store.Task{
		ID:       "t1",
		ChatID:   "c1",
		Schedule: "@every 2s",
		Prompt:   "test prompt",
	}))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go s.Run(ctx)

	time.Sleep(7 * time.Second)
	assert.GreaterOrEqual(t, fired.Load(), int32(3))
}

func TestScheduler_PausedTaskDoesNotFire(t *testing.T) {
	var fired atomic.Int32
	s := scheduler.New(func(string, string) { fired.Add(1) })
	require.NoError(t, s.Add(store.Task{ID: "t2", ChatID: "c2", Schedule: "@every 1s", Prompt: "p"}))
	s.Pause("t2")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go s.Run(ctx)

	time.Sleep(2 * time.Second)
	assert.Zero(t, fired.Load())
}

func TestValidate_AcceptsValidSchedules(t *testing.T) {
	s := scheduler.New(func(string, string) {})
	assert.NoError(t, s.Validate("0 8 * * *"))
	assert.NoError(t, s.Validate("@daily"))
	assert.NoError(t, s.Validate("@every 30m"))
	assert.NoError(t, s.Validate("*/5 * * * *"))
}

func TestValidate_RejectsInvalidSchedules(t *testing.T) {
	s := scheduler.New(func(string, string) {})
	err := s.Validate("not-a-cron")
	require.Error(t, err)
	assert.ErrorContains(t, err, `"not-a-cron"`)

	assert.Error(t, s.Validate("99 * * * *"))
	assert.Error(t, s.Validate(""))
}
