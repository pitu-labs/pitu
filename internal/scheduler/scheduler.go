package scheduler

import (
	"context"
	"fmt"
	"sync"

	"github.com/pitu-dev/pitu/internal/store"
	"github.com/robfig/cron/v3"
)

// schedulerParser is the single source of truth for accepted cron syntax.
var schedulerParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type Scheduler struct {
	dispatch func(chatID, prompt string)
	cr       *cron.Cron
	entryIDs sync.Map // taskID → cron.EntryID
	mu       sync.Mutex
}

func New(dispatch func(chatID, prompt string)) *Scheduler {
	return &Scheduler{
		dispatch: dispatch,
		cr:       cron.New(cron.WithParser(schedulerParser)),
	}
}

// Add registers a task with the scheduler.
func (s *Scheduler) Add(t store.Task) error {
	if t.Paused {
		return nil
	}
	id, err := s.cr.AddFunc(t.Schedule, func() {
		s.dispatch(t.ChatID, t.Prompt)
	})
	if err != nil {
		return fmt.Errorf("scheduler: add task %s: %w", t.ID, err)
	}
	s.entryIDs.Store(t.ID, id)
	return nil
}

// Pause stops a running task without removing it from the store.
func (s *Scheduler) Pause(taskID string) {
	if v, ok := s.entryIDs.LoadAndDelete(taskID); ok {
		s.cr.Remove(v.(cron.EntryID))
	}
}

// Validate checks whether schedule is a valid cron expression or descriptor
// without registering anything. Uses the same parser as cron.New().
func (s *Scheduler) Validate(schedule string) error {
	if _, err := schedulerParser.Parse(schedule); err != nil {
		return fmt.Errorf("scheduler: invalid schedule %q: %w", schedule, err)
	}
	return nil
}

// Run starts the cron scheduler and blocks until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	s.cr.Start()
	<-ctx.Done()
	s.cr.Stop()
}
