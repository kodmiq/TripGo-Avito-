package middleware

import (
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Metrics считает длительность и число запросов в работе. В http.route кладётся шаблон.
func Metrics(meter metric.Meter) (func(http.Handler) http.Handler, error) {
	duration, err := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("HTTP server request duration"),
	)
	if err != nil {
		return nil, fmt.Errorf("http.server.request.duration: %w", err)
	}
	active, err := meter.Int64UpDownCounter(
		"http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("HTTP requests currently in flight"),
	)
	if err != nil {
		return nil, fmt.Errorf("http.server.active_requests: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := routePattern(r)
			labels := []attribute.KeyValue{
				attribute.String("http.request.method", r.Method),
				attribute.String("http.route", route),
			}
			ctx := r.Context()
			active.Add(ctx, 1, metric.WithAttributes(labels...))
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				active.Add(ctx, -1, metric.WithAttributes(labels...))
				duration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(append(labels,
					attribute.Int("http.response.status_code", rec.status),
				)...))
			}()
			next.ServeHTTP(rec, r)
		})
	}, nil
}
