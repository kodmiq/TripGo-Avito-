package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type queryMetaKey struct{}

type queryMeta struct {
	at  time.Time
	sql string
}

// DBTracer ставится на пул: спан запроса создаёт otelpgx, а гистограмма
// db.client.operation.duration пишется здесь с лейблами из задания.
type DBTracer struct {
	inner *otelpgx.Tracer
	hist  metric.Float64Histogram
}

func NewDBTracer(tp *sdktrace.TracerProvider, meter metric.Meter) (*DBTracer, error) {
	hist, err := meter.Float64Histogram(
		"db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Database client operation duration"),
	)
	if err != nil {
		return nil, err
	}
	return &DBTracer{
		inner: otelpgx.NewTracer(otelpgx.WithTracerProvider(tp)),
		hist:  hist,
	}, nil
}

func (t *DBTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.inner.TraceQueryStart(ctx, conn, data)
	return context.WithValue(ctx, queryMetaKey{}, queryMeta{at: time.Now(), sql: data.SQL})
}

func (t *DBTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	meta, _ := ctx.Value(queryMetaKey{}).(queryMeta)
	span := trace.SpanFromContext(ctx)
	table := collectionName(meta.sql)
	if table != "" {
		span.SetAttributes(attribute.String("db.collection.name", table))
	}
	errType := ""
	if data.Err != nil && !errors.Is(data.Err, sql.ErrNoRows) {
		errType = dbErrorType(data.Err)
		span.SetAttributes(attribute.String("error.type", errType))
	}
	t.inner.TraceQueryEnd(ctx, conn, data)

	if meta.at.IsZero() {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("db.operation.name", operationName(meta.sql)),
		attribute.String("db.collection.name", table),
	}
	if errType != "" {
		attrs = append(attrs, attribute.String("error.type", errType))
	}
	t.hist.Record(ctx, time.Since(meta.at).Seconds(), metric.WithAttributes(attrs...))
}

func operationName(query string) string {
	fields := strings.Fields(query)
	if len(fields) == 0 {
		return "UNKNOWN"
	}
	return strings.ToUpper(strings.Trim(fields[0], `"`))
}

func collectionName(query string) string {
	fields := strings.Fields(query)
	for i, field := range fields {
		switch strings.ToUpper(strings.Trim(field, `"`)) {
		case "FROM", "INTO", "UPDATE":
			if i+1 < len(fields) {
				return strings.Trim(strings.Trim(fields[i+1], `"`), ",")
			}
		}
	}
	return ""
}

func dbErrorType(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code != "" {
		return pgErr.Code
	}
	return "other"
}
