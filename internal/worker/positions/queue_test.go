package positions

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestQueuePopsOldestFirst(t *testing.T) {
	queue := newTaskQueue(4)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000003"),
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
	}
	silent := []time.Time{now.Add(-time.Minute), now.Add(-3 * time.Minute), now.Add(-2 * time.Minute)}
	for i, id := range ids {
		if got := queue.Push(task{tripID: id, silentSince: silent[i]}); got.result != resultQueued {
			t.Fatalf("push %s = %s", id, got.result)
		}
	}

	var order []uuid.UUID
	for range ids {
		item, ok := queue.pop()
		if !ok {
			t.Fatal("missing item")
		}
		queue.done()
		order = append(order, item.tripID)
	}
	want := []uuid.UUID{ids[1], ids[2], ids[0]}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestQueueEvictsFreshestAndDropsNewer(t *testing.T) {
	queue := newTaskQueue(1)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	fresh := uuid.MustParse("00000000-0000-0000-0000-0000000000aa")
	olderID := uuid.MustParse("00000000-0000-0000-0000-0000000000bb")
	newest := uuid.MustParse("00000000-0000-0000-0000-0000000000cc")

	if got := queue.Push(task{tripID: fresh, silentSince: now}); got.result != resultQueued {
		t.Fatalf("fresh = %+v", got)
	}
	replaced := queue.Push(task{tripID: olderID, silentSince: now.Add(-time.Hour)})
	if !replaced.replaced || replaced.evicted != fresh {
		t.Fatalf("evict = %+v", replaced)
	}
	dropped := queue.Push(task{tripID: newest, silentSince: now.Add(time.Minute)})
	if dropped.replaced || dropped.result != resultDropped {
		t.Fatalf("newer = %+v", dropped)
	}

	item, ok := queue.pop()
	if !ok || item.tripID != olderID {
		t.Fatalf("pop = %v %s", ok, item.tripID)
	}
	queue.done()
}

func TestQueueDuplicateAndCancel(t *testing.T) {
	queue := newTaskQueue(2)
	id := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	item := task{tripID: id, silentSince: time.Now()}
	if got := queue.Push(item); got.result != resultQueued {
		t.Fatalf("first = %+v", got)
	}
	if got := queue.Push(item); got.result != resultDuplicate {
		t.Fatalf("second = %+v", got)
	}
	if !queue.Cancel(id) {
		t.Fatal("cancel did not find the trip")
	}
	if _, ok := queue.pop(); ok {
		t.Fatal("cancelled trip was popped")
	}
	if queue.Cancel(id) {
		t.Fatal("cancel removed a trip that was already gone")
	}
}

func TestQueueSameTimeOrdersByID(t *testing.T) {
	queue := newTaskQueue(2)
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	later := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	earlier := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	queue.Push(task{tripID: later, silentSince: now})
	queue.Push(task{tripID: earlier, silentSince: now})
	first, _ := queue.pop()
	second, _ := queue.pop()
	if first.tripID != earlier || second.tripID != later {
		t.Fatalf("order = %s, %s", first.tripID, second.tripID)
	}
}
