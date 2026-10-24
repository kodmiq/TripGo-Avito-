package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
)

type Handler struct {
	svc          *TripService
	ping         func(ctx context.Context) error
	readyTimeout time.Duration
}

func NewHandler(svc *TripService, ping func(ctx context.Context) error, readyTimeout time.Duration) *Handler {
	return &Handler{svc: svc, ping: ping, readyTimeout: readyTimeout}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	in, err := decodeCreateTrip(r)
	if err != nil {
		h.writeDecodeError(w, r, err)
		return
	}

	trip, created, err := h.svc.Create(r.Context(), in, idempotencyKey(params))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	if !created {
		slog.Info("idempotent replay", "trip_id", trip.ID.String(), "status", trip.Status)
		writeTrip(w, http.StatusOK, trip, "")
		return
	}
	slog.Info("trip created", "trip_id", trip.ID.String(), "status", trip.Status)
	writeTrip(w, http.StatusCreated, trip, "/api/v1/trips/"+trip.ID.String())
}

func idempotencyKey(params api.CreateTripParams) *uuid.UUID {
	if params.IdempotencyKey == nil {
		return nil
	}
	id := uuid.UUID(*params.IdempotencyKey)
	return &id
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := h.svc.Get(r.Context(), uuid.UUID(tripID))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	writeTrip(w, http.StatusOK, trip, "")
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	if err := rejectUnexpectedBody(r); err != nil {
		h.writeDecodeError(w, r, err)
		return
	}
	trip, err := h.svc.Finish(r.Context(), uuid.UUID(tripID))
	if err != nil {
		h.writeServiceError(w, r, err)
		return
	}
	slog.Info("trip finished", "trip_id", trip.ID.String(), "status", trip.Status)
	writeTrip(w, http.StatusOK, trip, "")
}

// Health не ходит в базу: ручку читает оркестратор, чтобы понять, жив ли процесс.
func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeHealth(w, http.StatusOK, api.Ok)
}

// Ready проверяет базу. Пока PostgreSQL недоступен, трафик на процесс слать нельзя.
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := withTimeout(r.Context(), h.readyTimeout)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		slog.Warn("readiness check failed", "err", err)
		writeHealth(w, http.StatusServiceUnavailable, api.Unavailable)
		return
	}
	writeHealth(w, http.StatusOK, api.Ok)
}

func (h *Handler) InvalidParam(w http.ResponseWriter, r *http.Request, _ error) {
	writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request",
		"https://tripgo.example/problems/invalid-request", "Request validation failed")
}

func (h *Handler) writeDecodeError(w http.ResponseWriter, r *http.Request, err error) {
	var validation *ValidationError
	if errors.As(err, &validation) {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Invalid request",
			"https://tripgo.example/problems/invalid-request", validation.Detail)
		return
	}
	h.writeServiceError(w, r, err)
}

func (h *Handler) writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrDriverBusy):
		writeProblem(w, r, http.StatusConflict, "driver_busy", "Driver busy",
			"https://tripgo.example/problems/driver-busy", "Driver already has an active trip")
	case errors.Is(err, ErrTripCompleted):
		writeProblem(w, r, http.StatusConflict, "trip_completed", "Trip completed",
			"https://tripgo.example/problems/trip-completed", "Operation is not allowed for a completed trip")
	case errors.Is(err, ErrIdempotencyConflict):
		writeProblem(w, r, http.StatusConflict, "idempotency_conflict", "Idempotency conflict",
			"https://tripgo.example/problems/idempotency-conflict", "Idempotency-Key was already used with a different request body")
	case errors.Is(err, ErrTripNotFound):
		writeProblem(w, r, http.StatusNotFound, "trip_not_found", "Trip not found",
			"https://tripgo.example/problems/trip-not-found", "Trip was not found")
	default:
		slog.Error("request failed", "err", err, "method", r.Method, "path", r.URL.Path)
		writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error",
			"https://tripgo.example/problems/internal-error", "Internal server error")
	}
}

func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				slog.Error("panic recovered", "err", fmt.Sprint(p), "stack", string(debug.Stack()), "path", r.URL.Path)
				writeProblem(w, r, http.StatusInternalServerError, "internal_error", "Internal Server Error",
					"https://tripgo.example/problems/internal-error", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeTrip(w http.ResponseWriter, status int, trip Trip, location string) {
	if location != "" {
		w.Header().Set("Location", location)
	}
	// Сгенерированные теги omitempty прячут null. В контракте finished_at и
	// last_position_at приходят в теле и равны null, пока значения нет.
	body := struct {
		ID             uuid.UUID       `json:"id"`
		UserID         uuid.UUID       `json:"user_id"`
		DriverID       uuid.UUID       `json:"driver_id"`
		StartPoint     api.Coordinates `json:"start_point"`
		EndPoint       api.Coordinates `json:"end_point"`
		Price          int64           `json:"price"`
		Status         api.TripStatus  `json:"status"`
		StartedAt      time.Time       `json:"started_at"`
		FinishedAt     *time.Time      `json:"finished_at"`
		LastPositionAt *time.Time      `json:"last_position_at"`
	}{
		ID:         trip.ID,
		UserID:     trip.UserID,
		DriverID:   trip.DriverID,
		StartPoint: api.Coordinates{Latitude: trip.StartLat, Longitude: trip.StartLon},
		EndPoint:   api.Coordinates{Latitude: trip.EndLat, Longitude: trip.EndLon},
		Price:      trip.Price,
		Status:     api.TripStatus(trip.Status),
		StartedAt:  trip.StartedAt,
		FinishedAt: trip.FinishedAt,
	}
	writeJSON(w, status, body)
}

func writeHealth(w http.ResponseWriter, status int, value api.HealthResponseStatus) {
	writeJSON(w, status, api.HealthResponse{Status: value})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeProblem(w http.ResponseWriter, r *http.Request, status int, code, title, typeURL, detail string) {
	instance := r.URL.Path
	problem := api.Problem{
		Type:     typeURL,
		Title:    title,
		Status:   int32(status),
		Detail:   &detail,
		Instance: &instance,
		Code:     code,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}
