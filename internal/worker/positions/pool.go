package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

// Config — параметры опроса. Ноль в размере очереди или числе воркеров не подставляется молча.
type Config struct {
	Interval    time.Duration
	StaleAfter  time.Duration
	Workers     int
	QueueSize   int
	BatchSize   int
	PushTimeout time.Duration
}

// Dependencies — то, что пул не создаёт сам. Now и NewTicker можно подменить в тестах.
type Dependencies struct {
	Trips     tripLister
	Push      pushGateway
	Metrics   metrics
	Log       *slog.Logger
	Tracer    trace.Tracer
	Now       func() time.Time
	NewTicker func(time.Duration) Ticker
}

// Pool — сканер, приоритетная очередь и фиксированное число воркеров.
type Pool struct {
	cfg     Config
	trips   tripLister
	push    pushGateway
	metrics metrics
	log     *slog.Logger
	tracer  trace.Tracer
	now     func() time.Time
	ticker  func(time.Duration) Ticker
	queue   *taskQueue

	hardCtx    context.Context
	cancelHard context.CancelFunc
	scanCtx    context.Context
	cancelScan context.CancelFunc

	scanWG   sync.WaitGroup
	workerWG sync.WaitGroup
	done     chan struct{}
}

func New(cfg Config, deps Dependencies) (*Pool, error) {
	if cfg.Workers < 1 || cfg.QueueSize < 1 || cfg.BatchSize < 1 {
		return nil, fmt.Errorf("position pool: workers, queue size and batch size must be positive")
	}
	if cfg.Interval <= 0 || cfg.StaleAfter <= 0 || cfg.PushTimeout <= 0 {
		return nil, fmt.Errorf("position pool: interval, stale after and push timeout must be greater than 0")
	}
	if deps.Trips == nil || deps.Push == nil || deps.Metrics == nil || deps.Log == nil {
		return nil, fmt.Errorf("position pool: trips, push, metrics and logger are required")
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	if deps.NewTicker == nil {
		deps.NewTicker = func(interval time.Duration) Ticker {
			return clockTicker{time.NewTicker(interval)}
		}
	}
	if deps.Tracer == nil {
		deps.Tracer = noop.NewTracerProvider().Tracer("trip-service")
	}
	return &Pool{
		cfg:     cfg,
		trips:   deps.Trips,
		push:    deps.Push,
		metrics: deps.Metrics,
		log:     deps.Log,
		tracer:  deps.Tracer,
		now:     deps.Now,
		ticker:  deps.NewTicker,
		queue:   newTaskQueue(cfg.QueueSize),
	}, nil
}

// QueueSize и ActiveWorkers читают наблюдаемые величины для gauge.
func (p *Pool) QueueSize() int64 { return p.queue.size.Load() }

func (p *Pool) ActiveWorkers() int64 { return p.queue.active.Load() }

// Start запускает один сканер и POSITION_WORKERS воркеров.
// Отмена ctx останавливает их сразу, не дожидаясь очереди.
func (p *Pool) Start(ctx context.Context) {
	p.hardCtx, p.cancelHard = context.WithCancel(ctx)
	p.scanCtx, p.cancelScan = context.WithCancel(p.hardCtx)
	p.done = make(chan struct{})
	context.AfterFunc(p.hardCtx, p.queue.broadcast)

	p.log.Info("position scanner started", "interval", p.cfg.Interval.String())
	p.scanWG.Add(1)
	go p.scan()

	p.log.Info("position workers started", "workers", p.cfg.Workers)
	for range p.cfg.Workers {
		p.workerWG.Add(1)
		go p.work()
	}
	go func() {
		p.scanWG.Wait()
		p.workerWG.Wait()
		close(p.done)
	}()
}

// Shutdown останавливает сканер, даёт воркерам доработать очередь в пределах ctx
// и только потом будит их на выход.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.cancelScan()
	p.scanWG.Wait()
	p.queue.waitIdle(ctx)
	if ctx.Err() != nil {
		p.cancelHard()
		<-p.done
		return fmt.Errorf("position workers shutdown: %w", ctx.Err())
	}
	p.queue.Close()
	p.cancelHard()
	<-p.done
	p.log.Info("position workers stopped")
	return nil
}

// Cancel убирает поездку из очереди: координата уже пришла, пуш не нужен.
func (p *Pool) Cancel(ctx context.Context, tripID uuid.UUID) {
	if p.queue.Cancel(tripID) {
		p.metrics.Request(ctx, resultCancelled, 0)
	}
}

func (p *Pool) scan() {
	defer p.scanWG.Done()
	ticker := p.ticker(p.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-p.scanCtx.Done():
			p.log.Info("position scanner stopped")
			return
		case <-ticker.C():
			p.runTick(p.scanCtx)
		}
	}
}

func (p *Pool) runTick(ctx context.Context) {
	ctx, span := p.tracer.Start(ctx, "positions.scan")
	defer span.End()

	var (
		cursor   *model.StaleCursor
		rowsRead int
	)
	defer func() { p.metrics.RowsRead(ctx, rowsRead) }()

	for {
		if ctx.Err() != nil {
			return
		}
		batch, err := p.trips.ListStale(ctx, p.now(), p.cfg.StaleAfter, p.cfg.BatchSize, cursor)
		if err != nil {
			if ctx.Err() == nil {
				p.log.ErrorContext(ctx, "stale trip scan failed", "err", err)
			}
			return
		}
		rowsRead += len(batch)
		if len(batch) == 0 {
			return
		}
		if !p.enqueueBatch(ctx, batch) {
			return
		}
		if len(batch) < p.cfg.BatchSize {
			return
		}
		last := batch[len(batch)-1]
		cursor = &model.StaleCursor{SilentSince: last.SilentSince, ID: last.ID}
	}
}

// enqueueBatch ставит порцию в очередь. false — первый dropped, следующие порции не читаем.
func (p *Pool) enqueueBatch(ctx context.Context, batch []model.StaleTrip) bool {
	for _, trip := range batch {
		if ctx.Err() != nil {
			return false
		}
		if !p.enqueue(ctx, trip) {
			return false
		}
	}
	return true
}

func (p *Pool) enqueue(ctx context.Context, trip model.StaleTrip) bool {
	_, span := p.tracer.Start(ctx, "position.enqueue", trace.WithAttributes(
		attribute.String("trip_id", trip.ID.String()),
	))
	outcome := p.queue.Push(task{
		tripID:      trip.ID,
		driverID:    trip.DriverID,
		silentSince: trip.SilentSince,
		link:        span.SpanContext(),
	})
	span.End()

	switch {
	case outcome.replaced:
		p.metrics.StaleCheck(ctx, resultDropped)
		p.log.WarnContext(ctx, "position request dropped", "trip_id", outcome.evicted.String(), "reason", "evicted by an older trip")
		p.metrics.StaleCheck(ctx, resultQueued)
		return false
	case outcome.result == resultQueued:
		p.metrics.StaleCheck(ctx, resultQueued)
		return true
	case outcome.result == resultDuplicate:
		p.metrics.StaleCheck(ctx, resultDuplicate)
		return true
	default:
		p.metrics.StaleCheck(ctx, resultDropped)
		p.log.WarnContext(ctx, "position request dropped", "trip_id", trip.ID.String(), "reason", "queue full")
		return false
	}
}

func (p *Pool) work() {
	defer p.workerWG.Done()
	for {
		if p.hardCtx.Err() != nil {
			return
		}
		item, ok := p.queue.pop()
		if ok {
			p.handle(item)
			continue
		}
		if p.queue.closedLocked() {
			return
		}
		p.queue.wait(p.hardCtx)
	}
}

func (q *taskQueue) closedLocked() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.closed
}

func (p *Pool) handle(item task) {
	defer p.queue.done()
	ctx, span := p.startRequestSpan(item)
	defer span.End()

	reqCtx, cancel := context.WithTimeout(ctx, p.cfg.PushTimeout)
	defer cancel()
	started := time.Now()
	err := p.push.RequestPosition(reqCtx, item.driverID, item.tripID)
	elapsed := time.Since(started)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "push failed")
		p.metrics.Request(ctx, resultError, elapsed)
		p.log.WarnContext(ctx, "push service unavailable", "trip_id", item.tripID.String(), "err", err)
		return
	}
	p.metrics.Request(ctx, resultSuccess, elapsed)
}

func (p *Pool) startRequestSpan(item task) (context.Context, trace.Span) {
	options := []trace.SpanStartOption{
		trace.WithAttributes(attribute.String("trip_id", item.tripID.String())),
	}
	if item.link.IsValid() {
		options = append(options, trace.WithLinks(trace.Link{SpanContext: item.link}))
	}
	return p.tracer.Start(p.hardCtx, "position.request", options...)
}
