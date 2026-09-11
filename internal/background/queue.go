// Package background owns bounded, cancelable best-effort work. Durable work
// (such as verification mail) belongs in a database outbox instead.
package background

import (
	"context"
	"log"
	"sync"
)

type Queue struct {
	ctx   context.Context
	tasks chan func()
}

func New(ctx context.Context, wg *sync.WaitGroup, workers, capacity int) *Queue {
	q := &Queue{ctx: ctx, tasks: make(chan func(), capacity)}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case task := <-q.tasks:
					if ctx.Err() != nil {
						return
					}
					task()
				}
			}
		}()
	}
	return q
}
func (q *Queue) Submit(task func()) {
	if q.ctx.Err() != nil {
		return
	}
	select {
	case q.tasks <- task:
	default:
		log.Print("background: notification queue full; notification skipped")
	}
}
