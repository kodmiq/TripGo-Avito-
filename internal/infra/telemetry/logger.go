package telemetry

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel/trace"
)

const serviceNameField = "service"

// NewLogger пишет JSON в stdout. trace_id и span_id добавляются, если запись
// сделана внутри спана. Глобальный slog.Default бизнес-код не использует.
func NewLogger(level slog.Level, serviceName string) *slog.Logger {
	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(traceHandler{next: base}).With(serviceNameField, serviceName)
}

type traceHandler struct {
	next slog.Handler
}

func (h traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h traceHandler) Handle(ctx context.Context, record slog.Record) error {
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if spanCtx.IsValid() {
		record.Add("trace_id", spanCtx.TraceID().String())
		record.Add("span_id", spanCtx.SpanID().String())
	}
	return h.next.Handle(ctx, record)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{next: h.next.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{next: h.next.WithGroup(name)}
}
