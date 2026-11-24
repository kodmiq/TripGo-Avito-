package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestTraceHandlerAddsIDs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(traceHandler{next: slog.NewJSONHandler(&buf, nil)})

	tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.AlwaysSample()))
	ctx, span := tp.Tracer("test").Start(context.Background(), "parent")
	defer span.End()

	log.InfoContext(ctx, "trip created")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("log %q: %v", buf.String(), err)
	}
	traceID, _ := record["trace_id"].(string)
	spanID, _ := record["span_id"].(string)
	if traceID == "" || spanID == "" || traceID != span.SpanContext().TraceID().String() {
		t.Fatalf("trace_id = %q span_id = %q", traceID, spanID)
	}
}
