package positions

import (
	"bytes"
	"container/heap"
	"context"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

// taskQueue — приоритетная очередь ограниченной ёмкости.
// Первой выходит поездка, которая молчит дольше всех. При переполнении более старая
// вытесняет самую свежую. map index — «сейчас в очереди»: одна поездка не попадает дважды.
type taskQueue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	items    priority
	capacity int
	closed   bool
	inflight int
	size     atomic.Int64
	active   atomic.Int64
}

func newTaskQueue(capacity int) *taskQueue {
	queue := &taskQueue{
		capacity: capacity,
		items:    priority{index: make(map[uuid.UUID]int)},
	}
	queue.cond = sync.NewCond(&queue.mu)
	return queue
}

type pushOutcome struct {
	result   string
	evicted  uuid.UUID
	replaced bool
}

func (q *taskQueue) Push(item task) pushOutcome {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return pushOutcome{result: resultDropped}
	}
	if _, ok := q.items.index[item.tripID]; ok {
		return pushOutcome{result: resultDuplicate}
	}
	if len(q.items.tasks) >= q.capacity {
		return q.pushFull(&item)
	}
	heap.Push(&q.items, &item)
	q.storeSize()
	q.cond.Broadcast()
	return pushOutcome{result: resultQueued}
}

func (q *taskQueue) pushFull(item *task) pushOutcome {
	worst := q.items.freshest()
	if !older(*item, *worst) {
		return pushOutcome{result: resultDropped}
	}
	evicted := worst.tripID
	heap.Remove(&q.items, worst.index)
	heap.Push(&q.items, item)
	q.storeSize()
	q.cond.Broadcast()
	return pushOutcome{result: resultDropped, evicted: evicted, replaced: true}
}

// Cancel удаляет поездку, которая ещё ждёт. Уже взятую воркером задачу не трогает.
func (q *taskQueue) Cancel(id uuid.UUID) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	index, ok := q.items.index[id]
	if !ok {
		return false
	}
	heap.Remove(&q.items, index)
	q.storeSize()
	q.cond.Broadcast()
	return true
}

func (q *taskQueue) pop() (task, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items.tasks) == 0 {
		return task{}, false
	}
	item := heap.Pop(&q.items).(*task)
	q.inflight++
	q.active.Store(int64(q.inflight))
	q.storeSize()
	return *item, true
}

func (q *taskQueue) done() {
	q.mu.Lock()
	q.inflight--
	q.active.Store(int64(q.inflight))
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *taskQueue) wait(ctx context.Context) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items.tasks) > 0 || q.closed || ctx.Err() != nil {
		return
	}
	q.cond.Wait()
}

func (q *taskQueue) waitIdle(ctx context.Context) {
	stop := context.AfterFunc(ctx, q.broadcast)
	defer stop()
	q.mu.Lock()
	defer q.mu.Unlock()
	for (q.inflight > 0 || len(q.items.tasks) > 0) && ctx.Err() == nil {
		q.cond.Wait()
	}
}

func (q *taskQueue) Close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *taskQueue) broadcast() {
	q.mu.Lock()
	q.cond.Broadcast()
	q.mu.Unlock()
}

func (q *taskQueue) storeSize() {
	q.size.Store(int64(len(q.items.tasks)))
}

func older(left, right task) bool {
	if !left.silentSince.Equal(right.silentSince) {
		return left.silentSince.Before(right.silentSince)
	}
	return bytes.Compare(left.tripID[:], right.tripID[:]) < 0
}

type priority struct {
	tasks []*task
	index map[uuid.UUID]int
}

func (p priority) Len() int { return len(p.tasks) }

func (p priority) Less(i, j int) bool {
	return older(*p.tasks[i], *p.tasks[j])
}

func (p priority) Swap(i, j int) {
	p.tasks[i], p.tasks[j] = p.tasks[j], p.tasks[i]
	p.tasks[i].index = i
	p.tasks[j].index = j
	p.index[p.tasks[i].tripID] = i
	p.index[p.tasks[j].tripID] = j
}

func (p *priority) Push(x any) {
	item := x.(*task)
	item.index = len(p.tasks)
	p.tasks = append(p.tasks, item)
	p.index[item.tripID] = item.index
}

func (p *priority) Pop() any {
	last := len(p.tasks) - 1
	item := p.tasks[last]
	p.tasks[last] = nil
	p.tasks = p.tasks[:last]
	delete(p.index, item.tripID)
	item.index = -1
	return item
}

func (p priority) freshest() *task {
	worst := p.tasks[0]
	for _, item := range p.tasks[1:] {
		if older(*worst, *item) {
			worst = item
		}
	}
	return worst
}
