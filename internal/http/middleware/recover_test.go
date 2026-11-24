package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverTurnsPanicIntoInternalError(t *testing.T) {
	handler := Recover(slog.New(slog.DiscardHandler))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom secret stack")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"code":"internal_error"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "boom") || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("panic text leaked: %s", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
}
