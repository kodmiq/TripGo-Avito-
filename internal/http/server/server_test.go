package server_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/kodmiq/TripGo-Avito-/internal/http/health"
	"github.com/kodmiq/TripGo-Avito-/internal/http/ready"
	"github.com/kodmiq/TripGo-Avito-/internal/http/server"
	httpcreate "github.com/kodmiq/TripGo-Avito-/internal/http/trip/create"
	httpfinish "github.com/kodmiq/TripGo-Avito-/internal/http/trip/finish"
	httpget "github.com/kodmiq/TripGo-Avito-/internal/http/trip/get"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

type unusedCreate struct{}

func (unusedCreate) Create(context.Context, model.CreateTrip, *uuid.UUID) (model.Trip, bool, error) {
	return model.Trip{}, false, nil
}

type unusedGet struct{}

func (unusedGet) Get(context.Context, uuid.UUID) (model.Trip, error) {
	return model.Trip{}, nil
}

type unusedFinish struct{}

func (unusedFinish) Finish(context.Context, uuid.UUID) (model.Trip, error) {
	return model.Trip{}, nil
}

type unusedDB struct{}

func (unusedDB) Ping(context.Context) error { return nil }

func TestInvalidTripID(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	meter := sdkmetric.NewMeterProvider().Meter("test")
	handler, err := server.New(
		httpcreate.New(unusedCreate{}, log),
		httpget.New(unusedGet{}, log),
		httpfinish.New(unusedFinish{}, log),
		health.New(),
		ready.New(unusedDB{}, time.Second, log),
		log,
		sdktrace.NewTracerProvider(),
		meter,
	)
	if err != nil {
		t.Fatalf("server: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/not-a-uuid", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "not-a-uuid") && strings.Contains(rec.Body.String(), "invalid UUID") {
		t.Fatalf("parser details leaked: %s", rec.Body.String())
	}
}
