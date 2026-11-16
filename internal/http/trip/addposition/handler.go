package addposition

import (
	"log/slog"
	"net/http"

	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/respond"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

// Handler принимает POST /api/v1/trips/{tripId}/positions.
type Handler struct {
	add   adder
	queue canceller
	log   *slog.Logger
}

func New(add adder, queue canceller, log *slog.Logger) *Handler {
	return &Handler{add: add, queue: queue, log: log}
}

func (h *Handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	in, err := decodePosition(r, tripID)
	if err != nil {
		respond.WriteError(w, r, h.log, err)
		return
	}
	stored, err := h.add.Add(r.Context(), in)
	if err != nil {
		respond.WriteError(w, r, h.log, err)
		return
	}
	h.queue.Cancel(r.Context(), tripID)
	respond.JSON(w, http.StatusCreated, toAPI(stored))
}

func toAPI(pos model.Position) api.TripPosition {
	return api.TripPosition{
		Id:          pos.ID,
		TripId:      pos.TripID,
		Coordinates: api.Coordinates{Latitude: pos.Latitude, Longitude: pos.Longitude},
		RecordedAt:  pos.RecordedAt,
	}
}
