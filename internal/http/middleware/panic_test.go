package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const tripRoute = "/api/v1/trips/{tripId}"

func TestPanicUsesOneFinalStatus(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	reader := metric.NewManualReader()
	meter := metric.NewMeterProvider(metric.WithReader(reader)).Meter("test")
	metrics, err := Metrics(meter)
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	spans := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))

	router := chi.NewRouter()
	router.Use(Recover(log))
	router.Use(Tracing(tp))
	router.Use(Logging(log))
	router.Use(metrics)
	router.Get(tripRoute, func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/6cb72c04-7650-45c9-a79b-bcdba0631e0c", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("boom")) {
		t.Fatalf("response leaked the panic: %s", rec.Body.String())
	}

	var handled map[string]any
	for line := range bytes.SplitSeq(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("log %q: %v", line, err)
		}
		if record["msg"] == "request handled" {
			handled = record
		}
	}
	if handled["route"] != tripRoute || handled["status"] != float64(http.StatusInternalServerError) {
		t.Fatalf("access log = %#v", handled)
	}
	if _, ok := handled["duration_ms"]; !ok {
		t.Fatalf("access log has no duration: %#v", handled)
	}
	if bytes.Contains(buf.Bytes(), []byte("6cb72c04-7650-45c9-a79b-bcdba0631e0c")) {
		t.Fatalf("log contains trip id: %s", buf.String())
	}

	ended := spans.Ended()
	if len(ended) != 1 {
		t.Fatalf("spans = %d", len(ended))
	}
	span := ended[0]
	if span.Name() != "GET "+tripRoute || span.Status().Code != codes.Error {
		t.Fatalf("span = %s status %s", span.Name(), span.Status().Code)
	}
	if got := spanAttr(t, span, "http.route"); got.AsString() != tripRoute {
		t.Fatalf("http.route = %s", got.AsString())
	}
	if got := spanAttr(t, span, "http.response.status_code"); got.AsInt64() != http.StatusInternalServerError {
		t.Fatalf("http.response.status_code = %d", got.AsInt64())
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	if got := histogramStatus(t, rm, "http.server.request.duration"); got != http.StatusInternalServerError {
		t.Fatalf("metric status = %d", got)
	}
}

func spanAttr(t *testing.T, span interface{ Attributes() []attribute.KeyValue }, key string) attribute.Value {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			return kv.Value
		}
	}
	t.Fatalf("span attribute %s is missing", key)
	return attribute.Value{}
}

func histogramStatus(t *testing.T, rm metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	for _, scope := range rm.ScopeMetrics {
		for _, item := range scope.Metrics {
			if item.Name != name {
				continue
			}
			hist, ok := item.Data.(metricdata.Histogram[float64])
			if !ok || len(hist.DataPoints) != 1 {
				t.Fatalf("histogram %s = %#v", name, item.Data)
			}
			for _, kv := range hist.DataPoints[0].Attributes.ToSlice() {
				if string(kv.Key) == "http.response.status_code" {
					return kv.Value.AsInt64()
				}
			}
		}
	}
	t.Fatalf("histogram %s is missing", name)
	return 0
}
