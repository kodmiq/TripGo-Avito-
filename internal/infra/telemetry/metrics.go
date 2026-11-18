package telemetry

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Trips считает созданные и завершённые поездки. Бизнес-ошибки сюда не входят:
// их видно по статусу HTTP.
type Trips struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func newTrips(meter metric.Meter) (Trips, error) {
	created, err := meter.Int64Counter(
		"tripgo.trips.created",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Created trips"),
	)
	if err != nil {
		return Trips{}, err
	}
	completed, err := meter.Int64Counter(
		"tripgo.trips.completed",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Completed trips"),
	)
	if err != nil {
		return Trips{}, err
	}
	return Trips{created: created, completed: completed}, nil
}

func (t Trips) TripCreated(ctx context.Context) {
	t.created.Add(ctx, 1)
}

func (t Trips) TripCompleted(ctx context.Context) {
	t.completed.Add(ctx, 1)
}

// Positions считает координаты, решения сканера и вызовы Push Service.
type Positions struct {
	saved    metric.Int64Counter
	stale    metric.Int64Counter
	requests metric.Int64Counter
	duration metric.Float64Histogram
	rows     metric.Int64Histogram
}

func newPositions(meter metric.Meter) (Positions, error) {
	saved, err := meter.Int64Counter(
		"tripgo.positions.saved",
		metric.WithUnit("{position}"),
		metric.WithDescription("Saved coordinates"),
	)
	if err != nil {
		return Positions{}, err
	}
	stale, err := meter.Int64Counter(
		"tripgo.stale_checks",
		metric.WithUnit("{trip}"),
		metric.WithDescription("What the scanner did with a stale trip"),
	)
	if err != nil {
		return Positions{}, err
	}
	requests, err := meter.Int64Counter(
		"tripgo.position_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Outcome of a position request"),
	)
	if err != nil {
		return Positions{}, err
	}
	duration, err := meter.Float64Histogram(
		"tripgo.position_requests.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Push Service call duration"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
	)
	if err != nil {
		return Positions{}, err
	}
	rows, err := meter.Int64Histogram(
		"tripgo.stale_checks.rows_read",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Rows read by the stale-trip scan during one tick"),
		metric.WithExplicitBucketBoundaries(1, 10, 25, 50, 100, 250, 500, 1000),
	)
	if err != nil {
		return Positions{}, err
	}
	return Positions{saved: saved, stale: stale, requests: requests, duration: duration, rows: rows}, nil
}

func (p Positions) PositionSaved(ctx context.Context) {
	p.saved.Add(ctx, 1)
}

func (p Positions) StaleCheck(ctx context.Context, result string) {
	p.stale.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (p Positions) RowsRead(ctx context.Context, n int) {
	if n < 0 {
		n = 0
	}
	p.rows.Record(ctx, int64(n))
}

func (p Positions) Request(ctx context.Context, result string, duration time.Duration) {
	p.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
	if result == "cancelled" {
		return
	}
	p.duration.Record(ctx, duration.Seconds(), metric.WithAttributes(attribute.String("result", result)))
}

// WatchPositionPool снимает размер очереди и число занятых воркеров в момент сбора.
func WatchPositionPool(meter metric.Meter, queueSize, activeWorkers func() int64) error {
	queue, err := meter.Int64ObservableGauge(
		"tripgo.position_requests.queue.size",
		metric.WithUnit("{task}"),
		metric.WithDescription("Position requests waiting in the queue"),
	)
	if err != nil {
		return err
	}
	active, err := meter.Int64ObservableGauge(
		"tripgo.position_requests.workers.active",
		metric.WithUnit("{worker}"),
		metric.WithDescription("Position workers currently busy"),
	)
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(queue, queueSize())
		observer.ObserveInt64(active, activeWorkers())
		return nil
	}, queue, active)
	return err
}

// WatchActiveTrips снимает число активных поездок в момент сбора метрик.
func WatchActiveTrips(meter metric.Meter, log *slog.Logger, count func(context.Context) (int64, error)) error {
	gauge, err := meter.Int64ObservableGauge(
		"tripgo.trips.active",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Trips currently in status active"),
	)
	if err != nil {
		return err
	}
	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		n, err := count(ctx)
		if err != nil {
			log.WarnContext(ctx, "active trips gauge", "err", err)
			return nil
		}
		observer.ObserveInt64(gauge, n)
		return nil
	}, gauge)
	return err
}
