package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Logging пишет одну запись, когда ответ уже готов. 2xx и 4xx — Info, 5xx — Error.
// Тела запроса и ответа в запись не попадают.
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			defer func() {
				recovered := recover()
				status := finishStatus(rec, recovered != nil)
				level := slog.LevelInfo
				if status >= http.StatusInternalServerError {
					level = slog.LevelError
				}
				log.Log(r.Context(), level, "request handled",
					"method", r.Method,
					"route", routePattern(r),
					"status", status,
					"duration_ms", time.Since(start).Milliseconds(),
				)
				if recovered != nil {
					panic(recovered)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
