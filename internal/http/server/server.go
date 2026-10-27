package server

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/domainerror"
	"github.com/kodmiq/TripGo-Avito-/internal/http/health"
	"github.com/kodmiq/TripGo-Avito-/internal/http/middleware"
	"github.com/kodmiq/TripGo-Avito-/internal/http/ready"
	"github.com/kodmiq/TripGo-Avito-/internal/http/respond"
	httpcreate "github.com/kodmiq/TripGo-Avito-/internal/http/trip/create"
	httpfinish "github.com/kodmiq/TripGo-Avito-/internal/http/trip/finish"
	httpget "github.com/kodmiq/TripGo-Avito-/internal/http/trip/get"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Server собирает ручки в один ServerInterface для oapi-codegen.
type Server struct {
	create *httpcreate.Handler
	get    *httpget.Handler
	finish *httpfinish.Handler
	health *health.Handler
	ready  *ready.Handler
}

func New(
	create *httpcreate.Handler,
	get *httpget.Handler,
	finish *httpfinish.Handler,
	healthHandler *health.Handler,
	readyHandler *ready.Handler,
	log *slog.Logger,
	tracer trace.TracerProvider,
	meter metric.Meter,
) (http.Handler, error) {
	metrics, err := middleware.Metrics(meter)
	if err != nil {
		return nil, err
	}
	router := chi.NewRouter()
	router.Use(middleware.Recover(log))
	router.Use(middleware.Tracing(tracer))
	router.Use(middleware.Logging(log))
	router.Use(metrics)
	api.HandlerWithOptions(Server{
		create: create,
		get:    get,
		finish: finish,
		health: healthHandler,
		ready:  readyHandler,
	}, api.ChiServerOptions{
		BaseRouter: router,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, _ error) {
			status, body := domainerror.InvalidRequest("Request validation failed")
			respond.Problem(w, r, status, body)
		},
	})
	return router, nil
}

func (s Server) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	s.create.CreateTrip(w, r, params)
}

func (s Server) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.get.GetTrip(w, r, tripID)
}

func (s Server) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.finish.FinishTrip(w, r, tripID)
}

func (s Server) Health(w http.ResponseWriter, r *http.Request) {
	s.health.Health(w, r)
}

func (s Server) Ready(w http.ResponseWriter, r *http.Request) {
	s.ready.Ready(w, r)
}
