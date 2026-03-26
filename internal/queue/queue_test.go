package queue_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pitu-dev/pitu/internal/queue"
	"github.com/stretchr/testify/assert"
)

func TestQueue_FIFOOrder(t *testing.T) {
	q := queue.New(10)
	results := make([]int, 0)
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		n := i
		q.Enqueue("chat1", func() {
			mu.Lock()
			results = append(results, n)
			mu.Unlock()
		})
	}
	time.Sleep(50 * time.Millisecond)
	q.Stop()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{0, 1, 2, 3, 4}, results)
}

func TestQueue_GlobalConcurrencyLimit(t *testing.T) {
	const maxConcurrent = 2
	q := queue.New(maxConcurrent)
	var active atomic.Int32
	var maxSeen atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		chatID := fmt.Sprintf("chat%d", i) // each in its own chat so they can all be dispatched
		q.Enqueue(chatID, func() {
			defer wg.Done()
			cur := active.Add(1)
			if cur > maxSeen.Load() {
				maxSeen.Store(cur)
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
		})
	}
	wg.Wait()
	q.Stop()
	assert.LessOrEqual(t, maxSeen.Load(), int32(maxConcurrent))
}
