package addposition_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	domain "github.com/kodmiq/TripGo-Avito-/internal/domain/trip"
	"github.com/kodmiq/TripGo-Avito-/internal/http/trip/addposition"
	"github.com/kodmiq/TripGo-Avito-/internal/model"
)

func TestCreatePositionCreatedAndCancelled(t *testing.T) {
	ctrl := gomock.NewController(t)
	add := NewMockadder(ctrl)
	queue := NewMockcanceller(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	recorded := time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)
	add.EXPECT().Add(gomock.Any(), model.NewPosition{
		TripID: id, Latitude: 59.935, Longitude: 30.335, RecordedAt: recorded,
	}).Return(model.Position{ID: 42, TripID: id, Latitude: 59.935, Longitude: 30.335, RecordedAt: recorded}, nil)
	queue.EXPECT().Cancel(gomock.Any(), id)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/positions", strings.NewReader(
		`{"coordinates":{"latitude":59.935,"longitude":30.335},"recorded_at":"2026-08-24T12:00:05Z"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	addposition.New(add, queue, slog.New(slog.DiscardHandler)).CreateTripPosition(rec, req, id)

	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"id":42`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePositionDoesNotCancelOnError(t *testing.T) {
	ctrl := gomock.NewController(t)
	add := NewMockadder(ctrl)
	queue := NewMockcanceller(ctrl)
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	add.EXPECT().Add(gomock.Any(), gomock.Any()).Return(model.Position{}, domain.ErrTripCompleted)

	req := httptest.NewRequest(http.MethodPost, "/positions", strings.NewReader(
		`{"coordinates":{"latitude":1,"longitude":2},"recorded_at":"2026-08-24T12:00:05Z"}`,
	))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	addposition.New(add, queue, slog.New(slog.DiscardHandler)).CreateTripPosition(rec, req, id)

	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"trip_completed"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePositionRejectsBadBody(t *testing.T) {
	ctrl := gomock.NewController(t)
	add := NewMockadder(ctrl)
	queue := NewMockcanceller(ctrl)
	id := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/positions", strings.NewReader(`{"coordinates":null,"recorded_at":"2026-08-24T12:00:05Z"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	addposition.New(add, queue, slog.New(slog.DiscardHandler)).CreateTripPosition(rec, req, id)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"invalid_request"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestCreatePositionContextIsRequestContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	add := NewMockadder(ctrl)
	queue := NewMockcanceller(ctrl)
	id := uuid.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	add.EXPECT().Add(gomock.Any(), gomock.Any()).DoAndReturn(func(got context.Context, _ model.NewPosition) (model.Position, error) {
		if got != ctx {
			t.Fatal("usecase received a different context")
		}
		return model.Position{TripID: id, RecordedAt: time.Now().UTC()}, nil
	})
	queue.EXPECT().Cancel(ctx, id)

	req := httptest.NewRequest(http.MethodPost, "/positions", strings.NewReader(
		`{"coordinates":{"latitude":1,"longitude":2},"recorded_at":"2026-08-24T12:00:05Z"}`,
	)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	addposition.New(add, queue, slog.New(slog.DiscardHandler)).CreateTripPosition(rec, req, id)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
