package listpositions

import (
	"log/slog"
	"net/http"

	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/respond"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

// Handler принимает GET /api/v1/trips/{tripId}/positions.
type Handler struct {
	list lister
	log  *slog.Logger
}

func New(list lister, log *slog.Logger) *Handler {
	return &Handler{list: list, log: log}
}

func (h *Handler) ListTripPositions(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	positions, err := h.list.List(r.Context(), tripID)
	if err != nil {
		respond.WriteError(w, r, h.log, err)
		return
	}
	respond.JSON(w, http.StatusOK, api.TripPositionsResponse{Positions: toAPI(positions)})
}

func toAPI(positions []model.Position) []api.TripPosition {
	out := make([]api.TripPosition, 0, len(positions))
	for _, pos := range positions {
		out = append(out, api.TripPosition{
			Id:          pos.ID,
			TripId:      pos.TripID,
			Coordinates: api.Coordinates{Latitude: pos.Latitude, Longitude: pos.Longitude},
			RecordedAt:  pos.RecordedAt,
		})
	}
	return out
}
