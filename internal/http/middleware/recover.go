package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/kodmiq/TripGo-Avito-/internal/http/domainerror"
	"github.com/kodmiq/TripGo-Avito-/internal/http/respond"
)

// Recover ловит панику, пишет её со стектрейсом и отдаёт 500. Процесс не падает.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				p := recover()
				if p == nil {
					return
				}
				log.ErrorContext(r.Context(), "panic recovered", "err", fmt.Sprint(p), "stack", string(debug.Stack()))
				status, body := domainerror.Internal()
				respond.Problem(w, r, status, body)
			}()
			next.ServeHTTP(w, r)
		})
	}
}
