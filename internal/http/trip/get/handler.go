package get

import (
	"log/slog"
	"net/http"

	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/respond"
)

// Handler принимает GET /api/v1/trips/{tripId}.
type Handler struct {
	read tripReader
	log  *slog.Logger
}

func New(read tripReader, log *slog.Logger) *Handler {
	return &Handler{read: read, log: log}
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	trip, err := h.read.Get(r.Context(), tripID)
	if err != nil {
		respond.WriteError(w, r, h.log, err)
		return
	}
	respond.Trip(w, http.StatusOK, trip, "")
}
