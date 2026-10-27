package create

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/respond"
)

// Handler принимает POST /api/v1/trips.
type Handler struct {
	create creator
	log    *slog.Logger
}

func New(create creator, log *slog.Logger) *Handler {
	return &Handler{create: create, log: log}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	in, err := decodeCreateTrip(r)
	if err != nil {
		respond.WriteError(w, r, h.log, err)
		return
	}

	trip, created, err := h.create.Create(r.Context(), in, idempotencyKey(params))
	if err != nil {
		respond.WriteError(w, r, h.log, err)
		return
	}
	if !created {
		respond.Trip(w, http.StatusOK, trip, "")
		return
	}
	respond.Trip(w, http.StatusCreated, trip, "/api/v1/trips/"+trip.ID.String())
}

func idempotencyKey(params api.CreateTripParams) *uuid.UUID {
	if params.IdempotencyKey == nil {
		return nil
	}
	id := uuid.UUID(*params.IdempotencyKey)
	return &id
}
