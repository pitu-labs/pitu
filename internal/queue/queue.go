package queue

import "sync"

// Queue dispatches tasks per chat in FIFO order, bounded by a global semaphore.
// Each chat gets a dedicated worker goroutine that lives until Stop is called.
type Queue struct {
	sem   chan struct{}
	mu    sync.Mutex
	chats map[string]chan func()
	wg    sync.WaitGroup
	once  sync.Once
	done  chan struct{}
}

func New(maxConcurrent int) *Queue {
	return &Queue{
		sem:   make(chan struct{}, maxConcurrent),
		chats: make(map[string]chan func()),
		done:  make(chan struct{}),
	}
}

// Enqueue adds a task for chatID. Tasks for the same chat run sequentially;
// tasks across different chats are bounded by maxConcurrent.
func (q *Queue) Enqueue(chatID string, fn func()) {
	q.mu.Lock()
	ch, ok := q.chats[chatID]
	if !ok {
		ch = make(chan func(), 256)
		q.chats[chatID] = ch
		q.wg.Add(1)
		go q.worker(ch)
	}
	q.mu.Unlock()
	select {
	case ch <- fn:
	case <-q.done:
	}
}

func (q *Queue) worker(ch chan func()) {
	defer q.wg.Done()
	for fn := range ch {
		q.sem <- struct{}{} // acquire slot
		fn()
		<-q.sem // release slot
	}
}

// Stop drains in-flight tasks and shuts down all workers.
func (q *Queue) Stop() {
	q.once.Do(func() {
		close(q.done)
		q.mu.Lock()
		for _, ch := range q.chats {
			close(ch)
		}
		q.mu.Unlock()
		q.wg.Wait()
	})
}
