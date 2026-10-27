package ready_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kodmiq/TripGo-Avito-/internal/http/ready"
	"go.uber.org/mock/gomock"
)

func TestReadyUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := NewMockdatabase(ctrl)
	db.EXPECT().Ping(gomock.Any()).Return(errors.New("connection refused"))

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	ready.New(db, time.Second, slog.New(slog.DiscardHandler)).Ready(rec, req)

	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"unavailable"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatalf("body leaked ping error: %s", rec.Body.String())
	}
}

func TestReadyOK(t *testing.T) {
	ctrl := gomock.NewController(t)
	db := NewMockdatabase(ctrl)
	db.EXPECT().Ping(gomock.Any()).DoAndReturn(func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("ready ping has no deadline")
		}
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()
	ready.New(db, time.Second, slog.New(slog.DiscardHandler)).Ready(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
