package finish

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/respond"
)

// Handler принимает POST /api/v1/trips/{tripId}/finish.
type Handler struct {
	finish tripFinisher
	log    *slog.Logger
}

func New(finish tripFinisher, log *slog.Logger) *Handler {
	return &Handler{finish: finish, log: log}
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	if err := rejectUnexpectedBody(r); err != nil {
		respond.WriteError(w, r, h.log, err)
		return
	}
	trip, err := h.finish.Finish(r.Context(), uuid.UUID(tripID))
	if err != nil {
		respond.WriteError(w, r, h.log, err)
		return
	}
	respond.Trip(w, http.StatusOK, trip, "")
}
