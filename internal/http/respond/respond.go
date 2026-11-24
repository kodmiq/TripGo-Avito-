package respond

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/domainerror"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func JSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

func Problem(w http.ResponseWriter, r *http.Request, status int, body api.Problem) {
	instance := r.URL.Path
	body.Instance = &instance
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		return
	}
}

// WriteError логирует причину только для 5xx: один раз и с текстом, которого нет в ответе.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	status, body := domainerror.Problem(err)
	if status >= http.StatusInternalServerError {
		log.ErrorContext(r.Context(), "request failed", "err", err)
	}
	Problem(w, r, status, body)
}

// Trip пишет поездку. finished_at и last_position_at всегда в теле, даже когда значения нет:
// у сгенерированных тегов omitempty спрятал бы null.
func Trip(w http.ResponseWriter, status int, trip model.Trip, location string) {
	if location != "" {
		w.Header().Set("Location", location)
	}
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
	JSON(w, status, body)
}
