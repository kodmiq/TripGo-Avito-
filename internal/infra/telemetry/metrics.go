package telemetry

import (
	"context"
	"log/slog"

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
