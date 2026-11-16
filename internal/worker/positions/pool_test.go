package positions

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/goleak"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestScannerSkipsDuplicateAndStopsOnDrop(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	first := stale(now.Add(-time.Hour))
	second := stale(now.Add(-time.Minute))
	metrics := &recordMetrics{}
	lister := &scriptLister{pages: [][]model.StaleTrip{{first, second}, {first}}}
	pool := testPool(t, Config{QueueSize: 1, BatchSize: 10}, lister, metrics, okPush{})

	pool.runTick(context.Background())
	if metrics.count(resultQueued) != 1 || metrics.count(resultDropped) != 1 {
		t.Fatalf("first tick = %+v", metrics.stale)
	}
	if lister.calls.Load() != 1 {
		t.Fatalf("scanner kept reading after drop: %d", lister.calls.Load())
	}

	pool.runTick(context.Background())
	if metrics.count(resultDuplicate) != 1 {
		t.Fatalf("duplicate = %+v", metrics.stale)
	}
	if len(metrics.rows) != 2 || metrics.rows[0] != 2 {
		t.Fatalf("rows = %v", metrics.rows)
	}
}

func TestScannerReadsAllBatches(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	pages := [][]model.StaleTrip{page(now, 100), page(now, 100), page(now, 50)}
	lister := &scriptLister{pages: pages}
	metrics := &recordMetrics{}
	pool := testPool(t, Config{QueueSize: 300, BatchSize: 100}, lister, metrics, okPush{})

	pool.runTick(context.Background())
	if lister.calls.Load() != 3 {
		t.Fatalf("queries = %d", lister.calls.Load())
	}
	if metrics.count(resultQueued) != 250 {
		t.Fatalf("queued = %d", metrics.count(resultQueued))
	}
	if lister.cursors[0] != nil {
		t.Fatal("first page had a cursor")
	}
	if lister.cursors[1] == nil || lister.cursors[1].ID != pages[0][99].ID {
		t.Fatalf("second cursor = %+v", lister.cursors[1])
	}
}

func TestCancelSkipsPush(t *testing.T) {
	trip := stale(time.Now())
	push := &countingPush{}
	metrics := &recordMetrics{}
	pool := testPool(t, Config{QueueSize: 4, BatchSize: 10}, &scriptLister{pages: [][]model.StaleTrip{{trip}}}, metrics, push)
	pool.runTick(context.Background())
	pool.Cancel(context.Background(), trip.ID)
	if metrics.requests[resultCancelled] != 1 {
		t.Fatalf("cancelled = %+v", metrics.requests)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v", err)
	}
	if push.calls.Load() != 0 {
		t.Fatalf("push calls = %d", push.calls.Load())
	}
}

func TestPoolUsesFixedWorkersAndSurvivesPushError(t *testing.T) {
	gate := &gatePush{entered: make(chan struct{}, 8), release: make(chan struct{}), failFirst: 1}
	metrics := &recordMetrics{}
	pool := testPool(t, Config{Workers: 2, QueueSize: 8, BatchSize: 10}, &scriptLister{}, metrics, gate)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)
	t.Cleanup(func() {
		cancel()
		<-pool.done
	})

	for range 4 {
		pool.queue.Push(task{tripID: uuid.New(), driverID: uuid.New(), silentSince: time.Now()})
	}
	<-gate.entered
	<-gate.entered
	if gate.calls.Load() != 2 || pool.ActiveWorkers() != 2 || pool.QueueSize() != 2 {
		t.Fatalf("calls=%d active=%d queued=%d", gate.calls.Load(), pool.ActiveWorkers(), pool.QueueSize())
	}
	close(gate.release)

	wait, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	for gate.calls.Load() != 4 {
		select {
		case <-wait.Done():
			t.Fatalf("calls = %d", gate.calls.Load())
		case <-gate.entered:
		}
	}
	if gate.errors.Load() != 1 || gate.calls.Load() != 4 {
		t.Fatalf("calls=%d errors=%d results=%+v", gate.calls.Load(), gate.errors.Load(), metrics.requests)
	}
}

func TestTicksDoNotOverlap(t *testing.T) {
	ticks := make(chan time.Time, 1)
	lister := &blockingLister{entered: make(chan struct{}), release: make(chan struct{})}
	pool := testPool(t, Config{}, lister, &recordMetrics{}, okPush{})
	pool.ticker = func(time.Duration) Ticker { return fakeTicker{c: ticks} }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)

	ticks <- time.Now()
	<-lister.entered
	ticks <- time.Now()
	if lister.inflight.Load() != 1 {
		t.Fatalf("overlapping scans: %d", lister.inflight.Load())
	}
	lister.release <- struct{}{}
	<-lister.entered
	if lister.max.Load() != 1 {
		t.Fatalf("max inflight = %d", lister.max.Load())
	}
	lister.release <- struct{}{}
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() = %v", err)
	}
}

func TestShutdownDrainsAndContextCancelStops(t *testing.T) {
	t.Run("drain", func(t *testing.T) {
		gate := &gatePush{entered: make(chan struct{}, 1), release: make(chan struct{})}
		pool := testPool(t, Config{Workers: 1, QueueSize: 2, BatchSize: 10}, &scriptLister{}, &recordMetrics{}, gate)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		pool.Start(ctx)
		pool.queue.Push(task{tripID: uuid.New(), driverID: uuid.New(), silentSince: time.Now()})
		<-gate.entered
		finished := make(chan error, 1)
		go func() { finished <- pool.Shutdown(context.Background()) }()
		gate.release <- struct{}{}
		if err := <-finished; err != nil {
			t.Fatalf("Shutdown() = %v", err)
		}
		if gate.calls.Load() != 1 {
			t.Fatalf("drained calls = %d", gate.calls.Load())
		}
	})

	t.Run("cancel", func(t *testing.T) {
		gate := &gatePush{entered: make(chan struct{}, 1), release: make(chan struct{})}
		pool := testPool(t, Config{Workers: 1, QueueSize: 2, BatchSize: 10}, &scriptLister{}, &recordMetrics{}, gate)
		ctx, cancel := context.WithCancel(context.Background())
		pool.Start(ctx)
		pool.queue.Push(task{tripID: uuid.New(), driverID: uuid.New(), silentSince: time.Now()})
		<-gate.entered
		cancel()
		wait, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		select {
		case <-pool.done:
		case <-wait.Done():
			t.Fatal("workers stayed up after cancel")
		}
	})
}

func testPool(t *testing.T, cfg Config, trips tripLister, metrics metrics, push pushGateway) *Pool {
	t.Helper()
	if cfg.Interval == 0 {
		cfg.Interval = time.Second
	}
	if cfg.StaleAfter == 0 {
		cfg.StaleAfter = time.Second
	}
	if cfg.Workers == 0 {
		cfg.Workers = 1
	}
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 10
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 10
	}
	if cfg.PushTimeout == 0 {
		cfg.PushTimeout = time.Second
	}
	pool, err := New(cfg, Dependencies{
		Trips: trips, Push: push, Metrics: metrics, Log: slog.New(slog.DiscardHandler),
		Now: func() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	return pool
}

func stale(silent time.Time) model.StaleTrip {
	return model.StaleTrip{ID: uuid.New(), DriverID: uuid.New(), SilentSince: silent}
}

func page(now time.Time, n int) []model.StaleTrip {
	out := make([]model.StaleTrip, n)
	for i := range out {
		out[i] = model.StaleTrip{
			ID:          uuid.New(),
			DriverID:    uuid.New(),
			SilentSince: now.Add(-time.Duration(n-i) * time.Second),
		}
	}
	return out
}

type recordMetrics struct {
	mu       sync.Mutex
	stale    map[string]int
	requests map[string]int
	rows     []int
}

func (m *recordMetrics) StaleCheck(_ context.Context, result string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stale == nil {
		m.stale = map[string]int{}
	}
	m.stale[result]++
}

func (m *recordMetrics) RowsRead(_ context.Context, n int) {
	m.mu.Lock()
	m.rows = append(m.rows, n)
	m.mu.Unlock()
}

func (m *recordMetrics) Request(_ context.Context, result string, _ time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.requests == nil {
		m.requests = map[string]int{}
	}
	m.requests[result]++
}

func (m *recordMetrics) count(result string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stale[result]
}

type scriptLister struct {
	calls   atomic.Int32
	mu      sync.Mutex
	pages   [][]model.StaleTrip
	cursors []*model.StaleCursor
}

func (s *scriptLister) ListStale(_ context.Context, _ time.Time, _ time.Duration, _ int, cursor *model.StaleCursor) ([]model.StaleTrip, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := int(s.calls.Add(1))
	s.cursors = append(s.cursors, cursor)
	if n > len(s.pages) {
		return nil, nil
	}
	return s.pages[n-1], nil
}

type blockingLister struct {
	entered  chan struct{}
	release  chan struct{}
	inflight atomic.Int32
	max      atomic.Int32
}

func (b *blockingLister) ListStale(ctx context.Context, _ time.Time, _ time.Duration, _ int, _ *model.StaleCursor) ([]model.StaleTrip, error) {
	n := b.inflight.Add(1)
	for {
		old := b.max.Load()
		if n <= old || b.max.CompareAndSwap(old, n) {
			break
		}
	}
	select {
	case b.entered <- struct{}{}:
	case <-ctx.Done():
		b.inflight.Add(-1)
		return nil, ctx.Err()
	}
	select {
	case <-b.release:
		b.inflight.Add(-1)
		return nil, nil
	case <-ctx.Done():
		b.inflight.Add(-1)
		return nil, ctx.Err()
	}
}

type okPush struct{}

func (okPush) RequestPosition(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type countingPush struct {
	calls atomic.Int32
}

func (p *countingPush) RequestPosition(context.Context, uuid.UUID, uuid.UUID) error {
	p.calls.Add(1)
	return nil
}

type gatePush struct {
	entered   chan struct{}
	release   chan struct{}
	calls     atomic.Int32
	errors    atomic.Int32
	failFirst int32
}

func (g *gatePush) RequestPosition(ctx context.Context, _, _ uuid.UUID) error {
	n := g.calls.Add(1)
	select {
	case g.entered <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-g.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	if n <= g.failFirst {
		g.errors.Add(1)
		return errors.New("push unavailable")
	}
	return nil
}

type fakeTicker struct{ c chan time.Time }

func (f fakeTicker) C() <-chan time.Time { return f.c }
func (f fakeTicker) Stop()               {}
