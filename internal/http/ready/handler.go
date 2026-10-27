package ready

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/kodmiq/TripGo-Avito-/internal/generated"
	"github.com/kodmiq/TripGo-Avito-/internal/http/respond"
)

// Handler проверяет базу. Пока PostgreSQL недоступен, трафик на процесс слать нельзя.
type Handler struct {
	db      database
	timeout time.Duration
	log     *slog.Logger
}

func New(db database, timeout time.Duration, log *slog.Logger) *Handler {
	return &Handler{db: db, timeout: timeout, log: log}
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	if err := h.db.Ping(ctx); err != nil {
		h.log.WarnContext(r.Context(), "readiness check failed", "err", err)
		respond.JSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})
		return
	}
	respond.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
