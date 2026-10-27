package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestRoutePatternIsKnownBeforeHandler(t *testing.T) {
	router := chi.NewRouter()
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := routePattern(r); got != "/api/v1/trips/{tripId}" {
				t.Errorf("route before handler = %q", got)
			}
			next.ServeHTTP(w, r)
		})
	})
	router.Get("/api/v1/trips/{tripId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/6cb72c04-7650-45c9-a79b-bcdba0631e0c", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)
}

func TestLoggingUsesRouteTemplate(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	router := chi.NewRouter()
	router.Use(Logging(log))
	router.Get("/api/v1/trips/{tripId}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/6cb72c04-7650-45c9-a79b-bcdba0631e0c", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("log %q: %v", buf.String(), err)
	}
	if record["msg"] != "request handled" || record["route"] != "/api/v1/trips/{tripId}" || record["status"] != float64(201) {
		t.Fatalf("log = %#v", record)
	}
	if bytes.Contains(buf.Bytes(), []byte("6cb72c04-7650-45c9-a79b-bcdba0631e0c")) {
		t.Fatalf("log contains trip id: %s", buf.String())
	}
}
