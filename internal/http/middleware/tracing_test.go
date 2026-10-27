package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingNamesSpanByRouteTemplate(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	router := chi.NewRouter()
	router.Use(Tracing(tp))
	router.Get("/api/v1/trips/{tripId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/6cb72c04-7650-45c9-a79b-bcdba0631e0c", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	span := spans[0]
	if span.Name() != "GET /api/v1/trips/{tripId}" {
		t.Fatalf("span name = %q", span.Name())
	}
	if span.Status().Code != codes.Unset {
		t.Fatalf("status = %s", span.Status().Code)
	}
}

func TestTracingMarksServerError(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	router := chi.NewRouter()
	router.Use(Tracing(tp))
	router.Get("/missing", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Status().Code != codes.Unset {
		t.Fatalf("4xx must not mark the span as error, spans = %v", spans)
	}

	router = chi.NewRouter()
	router.Use(Tracing(tp))
	router.Get("/boom", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	spans = recorder.Ended()
	if len(spans) != 2 || spans[1].Status().Code != codes.Error {
		t.Fatalf("5xx span status = %v", spans)
	}
}
